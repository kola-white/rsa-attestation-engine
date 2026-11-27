package holderpresentation

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"github.com/golang-jwt/jwt/v5"
	ec "github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/evidencecredential"
	b "github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/portableverify"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestReciprocalPresentation(t *testing.T) {
	key := func() *rsa.PrivateKey {
		k, e := rsa.GenerateKey(rand.Reader, 3072)
		if e != nil {
			t.Fatal(e)
		}
		return k
	}
	issuer, holder, verifier, wrong := key(), key(), key(), key()
	sign := func(m jwt.MapClaims, k *rsa.PrivateKey, typ, kid string) string {
		tok := jwt.NewWithClaims(jwt.SigningMethodRS256, m)
		tok.Header["typ"] = typ
		if kid != "" {
			tok.Header["kid"] = kid
		}
		if typ == "openid4vci-proof+jwt" {
			tok.Header["jwk"] = JWK(&k.PublicKey)
		}
		s, e := tok.SignedString(k)
		if e != nil {
			t.Fatal(e)
		}
		return s
	}
	pub := func(k *rsa.PrivateKey) string {
		der, e := x509.MarshalPKIXPublicKey(&k.PublicKey)
		if e != nil {
			t.Fatal(e)
		}
		return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
	}
	data, e := os.ReadFile("../../../../interoperability/stage4b/fixtures/vectors.json")
	if e != nil {
		t.Fatal(e)
	}
	var vectors []struct {
		Name  string  `json:"name"`
		Input b.Input `json:"input"`
	}
	if e = json.Unmarshal(data, &vectors); e != nil {
		t.Fatal(e)
	}
	var base b.Input
	for _, v := range vectors {
		if v.Name == "fresh" {
			base = v.Input
		}
	}
	now := base.Policy.Now
	claims, e := payload(strings.TrimSuffix(base.Credential, "~"))
	if e != nil {
		t.Fatal(e)
	}
	claims["credential_spki_sha256"] = ec.Fingerprint(&issuer.PublicKey)
	nonce := "issuer-nonce-00000000000000001"
	proof := sign(jwt.MapClaims{"aud": claims["iss"], "nonce": nonce, "iat": now}, holder, "openid4vci-proof+jwt", "")
	var calls atomic.Int32
	signIssuer := func(m jwt.MapClaims) (string, error) {
		calls.Add(1)
		return sign(m, issuer, "dc+sd-jwt", base.Trust.KID) + "~", nil
	}
	store := NewStore()
	raw, e := IssueBound(claims, &holder.PublicKey, proof, claims["iss"].(string), nonce, now, now+120, store, signIssuer)
	if e != nil {
		t.Fatal(e)
	}
	if _, exists := claims["cnf"]; exists {
		t.Fatal("caller claims mutated")
	}
	t.Run("signer-failure-no-artifact-no-retry", func(t *testing.T) {
		s := NewStore()
		failed := 0
		out, err := IssueBound(claims, &holder.PublicKey, proof, claims["iss"].(string), nonce, now, now+120, s, func(jwt.MapClaims) (string, error) { failed++; return "ambiguous", errors.New("signer failed") })
		if out != "" || err == nil {
			t.Fatal("failed signer leaked artifact")
		}
		_, err = IssueBound(claims, &holder.PublicKey, proof, claims["iss"].(string), nonce, now, now+120, s, signIssuer)
		if err == nil || failed != 1 || calls.Load() != 1 {
			t.Fatal("failed attempt retried")
		}
	})
	t.Run("issuance-replay-no-sign", func(t *testing.T) {
		_, e := IssueBound(claims, &holder.PublicKey, proof, claims["iss"].(string), nonce, now, now+120, store, signIssuer)
		if e == nil || calls.Load() != 1 {
			t.Fatal("replayed proof signed")
		}
	})
	for _, tc := range []struct{ name, proof string }{{"missing", ""}, {"wrong-key", sign(jwt.MapClaims{"aud": claims["iss"], "nonce": nonce, "iat": now}, wrong, "openid4vci-proof+jwt", "")}, {"wrong-nonce", sign(jwt.MapClaims{"aud": claims["iss"], "nonce": "wrong", "iat": now}, holder, "openid4vci-proof+jwt", "")}, {"expired", sign(jwt.MapClaims{"aud": claims["iss"], "nonce": nonce, "iat": now - 301}, holder, "openid4vci-proof+jwt", "")}} {
		t.Run("issuance-"+tc.name, func(t *testing.T) {
			_, e := IssueBound(claims, &holder.PublicKey, tc.proof, claims["iss"].(string), nonce, now, now+120, NewStore(), signIssuer)
			if e == nil || calls.Load() != 1 {
				t.Fatal("bad proof reached issuer")
			}
		})
	}
	base.Credential = raw
	base.Trust.PublicKey = pub(issuer)
	base.Trust.AuthorityKey = pub(issuer)
	base.Trust.StatusKey = pub(issuer)
	eb := claims["evidence_binding"].(map[string]any)
	base.Authority = sign(jwt.MapClaims{"iss": base.Trust.AuthorityIssuer, "iat": now, "exp": now + 600, "synthetic": true, "credential_sha256": ec.Digest([]byte(raw)), "issuer": claims["iss"], "employer_id": eb["employer_id"], "claim_type": claims["claim_type"], "decision": "AUTHORIZED"}, issuer, "cvera-stage4b-fixture-authority+jwt", base.Trust.AuthorityKID)
	base.Status = &b.Status{Availability: "AVAILABLE", JWS: sign(jwt.MapClaims{"iss": base.Trust.StatusIssuer, "iat": now, "synthetic": true, "credential_sha256": ec.Digest([]byte(raw)), "entries": []any{map[string]any{"credential_id": claims["jti"], "state": "GOOD"}}}, issuer, "cvera-stage4b-fixture-status+jwt", base.Trust.StatusKID)}
	v := Verifier{ID: "test-verifier", KID: "v", Key: pub(verifier), ResponseURI: "https://verifier.example.invalid/response", Allow: true}
	var q any
	_ = json.Unmarshal([]byte(`{"credentials":[{"id":"employment","format":"dc+sd-jwt","meta":{"vct_values":["https://cvera.app/credentials/employment-role/v1"]},"require_cryptographic_holder_binding":true}]}`), &q)
	request := sign(jwt.MapClaims{"iss": v.ID, "client_id": v.ID, "aud": "https://self-issued.me/v2", "iat": now, "exp": now + 120, "response_type": "vp_token", "response_mode": "direct_post", "response_uri": v.ResponseURI, "nonce": "verifier-nonce-000000000000001", "state": "verifier-state-000000000000001", "dcql_query": q}, verifier, "oauth-authz-req+jwt", v.KID)
	in := Input{Base: base, Request: request, Verifiers: []Verifier{v}, AllowFullDisclosure: true, Consent: &Consent{Hash(request), Hash(raw), now + 60, true}, Transaction: Transaction{v.ID, "verifier-nonce-000000000000001", Hash(request), now + 120}}
	in.Presentation, _, e = Present(in, holder)
	if e != nil {
		t.Fatal(e)
	}
	t.Run("valid-go-presentation", func(t *testing.T) {
		r := Verify(in, NewStore())
		if r.Overall != "ACCEPTED_SYNTHETIC_PRESENTATION" || r.Consent != "NOT_OBSERVABLE" {
			t.Fatalf("%+v", r)
		}
		br, _ := b.Verify(base)
		if br.Holder != "UNKNOWN" || br.Overall != "INDETERMINATE" {
			t.Fatal("old verifier acquired assurance")
		}
	})
	t.Run("malformed-holder-private-key", func(t *testing.T) {
		out, _, err := Present(in, &rsa.PrivateKey{})
		if err == nil || out != "" {
			t.Fatal("malformed key released presentation")
		}
	})
	t.Run("wrong-holder-private-key", func(t *testing.T) {
		out, _, err := Present(in, wrong)
		if err == nil || out != "" {
			t.Fatal("wrong key disclosed")
		}
	})
	t.Run("consent-denied-no-key-needed", func(t *testing.T) {
		denied := in
		denied.Consent = nil
		out, _, err := Present(denied, nil)
		if err == nil || out != "" {
			t.Fatal("disclosure without consent")
		}
	})
	t.Run("invalid-proof-does-not-consume", func(t *testing.T) {
		s := NewStore()
		bad := in
		bad.Presentation = raw
		if Verify(bad, s).Overall != "DENIED" || Verify(in, s).Overall != "ACCEPTED_SYNTHETIC_PRESENTATION" {
			t.Fatal("invalid proof consumed challenge")
		}
	})
	t.Run("concurrent-exactly-one", func(t *testing.T) {
		s := NewStore()
		var wg sync.WaitGroup
		var accepted atomic.Int32
		for n := 0; n < 32; n++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if Verify(in, s).Overall == "ACCEPTED_SYNTHETIC_PRESENTATION" {
					accepted.Add(1)
				}
			}()
		}
		wg.Wait()
		if accepted.Load() != 1 {
			t.Fatalf("accepted %d duplicates", accepted.Load())
		}
	})
	t.Run("cnf-not-subject-identity", func(t *testing.T) {
		m, e := payload(strings.TrimSuffix(raw, "~"))
		if e != nil {
			t.Fatal(e)
		}
		delete(m, "cnf")
		unbound := sign(m, issuer, "dc+sd-jwt", base.Trust.KID) + "~"
		if _, e = holderKey(unbound); e == nil {
			t.Fatal("subject inferred holder key")
		}
	})
	t.Run("reject-private-jwk", func(t *testing.T) {
		j := JWK(&holder.PublicKey)
		j["d"] = "private"
		if _, e := jwk(j); e == nil {
			t.Fatal("private JWK accepted")
		}
	})
}

func TestMalformedIntendedKey(t *testing.T) {
	if _, err := IssueBound(jwt.MapClaims{}, &rsa.PublicKey{}, "", "", "", 1, 2, NewStore(), nil); err == nil {
		t.Fatal("malformed intended key accepted")
	}
}
