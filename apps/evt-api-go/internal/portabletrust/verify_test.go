package portabletrust

import (
	"bytes"
	"compress/zlib"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"os"
	"sync"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	ec "github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/evidencecredential"
	b "github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/portableverify"
)

func TestPortableTrustLifecycle(t *testing.T) {
	k, e := rsa.GenerateKey(rand.Reader, 3072)
	if e != nil {
		t.Fatal(e)
	}
	key := JWK{Kty: "RSA", N: base64.RawURLEncoding.EncodeToString(k.N.Bytes()), E: "AQAB", KID: "test", Alg: "RS256", Use: "sig", Ops: []string{"verify"}}
	sign := func(v any, typ string) string {
		bb, _ := json.Marshal(v)
		var c jwt.MapClaims
		if json.Unmarshal(bb, &c) != nil {
			t.Fatal("claims")
		}
		tok := jwt.NewWithClaims(jwt.SigningMethodRS256, c)
		tok.Header["typ"] = typ
		tok.Header["kid"] = "test"
		raw, e := tok.SignedString(k)
		if e != nil {
			t.Fatal(e)
		}
		return raw
	}
	data, e := os.ReadFile("../../../../interoperability/stage4b/fixtures/vectors.json")
	if e != nil {
		t.Fatal(e)
	}
	var vs []struct {
		Name  string  `json:"name"`
		Input b.Input `json:"input"`
	}
	if e = json.Unmarshal(data, &vs); e != nil {
		t.Fatal(e)
	}
	var src b.Input
	for _, v := range vs {
		if v.Name == "fresh" {
			src = v.Input
		}
	}
	parts := bytes.Split([]byte(src.Credential), []byte("."))
	cb, _ := base64.RawURLEncoding.DecodeString(string(parts[1]))
	var c jwt.MapClaims
	_ = json.Unmarshal(cb, &c)
	now := src.Policy.Now
	c["credential_spki_sha256"] = ec.Fingerprint(&k.PublicKey)
	c["status"] = map[string]any{"status_list": map[string]any{"idx": 37, "uri": "https://status.example.invalid/list"}}
	in := Input{Credential: sign(c, "dc+sd-jwt") + "~", Anchor: Anchor{Issuer: "https://root.example.invalid", KID: "test", Key: key, Synthetic: true}, Policy: Policy{Now: now, ExpectedSubject: src.Policy.ExpectedSubject, Issuer: ec.Issuer, TrustURI: "https://trust.example.invalid/bundle", AllowSynthetic: true, AuthorityMode: "CURRENT"}}
	m := Manifest{Issuer: in.Anchor.Issuer, Subject: in.Policy.TrustURI, Iat: now, Exp: now + 3600, Version: 1, Synthetic: true, Profile: Profile, CredentialIssuer: ec.Issuer, VCT: VCT, Keys: []Key{{key, 0, now + 10000}}, Authority: []Grant{{"synthetic-employer", "employment.role", ec.Policy, 0, now + 10000, "AUTHORIZED"}}, Status: StatusKey{"https://status.example.invalid", "https://status.example.invalid/list", "revocation", key}}
	emp := c["employment"].(map[string]any)
	m.Authority[0].Employer = emp["employer_id"].(string)
	in.Trust = &Artifact{"AVAILABLE", sign(m, "cvera-trust-material+jwt")}
	makeStatus := func(v int64, revoked bool) string {
		raw := make([]byte, 16384)
		if revoked {
			raw[4] |= 1 << 5
		}
		var buf bytes.Buffer
		z := zlib.NewWriter(&buf)
		_, _ = z.Write(raw)
		_ = z.Close()
		return sign(StatusToken{Issuer: m.Status.Issuer, Subject: m.Status.URI, Iat: now, Exp: now + 3600, TTL: 900, Version: v, Synthetic: true, VCT: VCT, CredentialIssuer: ec.Issuer, List: List{1, base64.RawURLEncoding.EncodeToString(buf.Bytes())}}, "statuslist+jwt")
	}
	in.Status = &Artifact{"AVAILABLE", makeStatus(1, false)}
	state := NewState()
	r, e := Verify(in, state)
	if e != nil || r.Overall != "ELIGIBLE_WITHOUT_HOLDER" {
		t.Fatalf("%+v %v", r, e)
	}
	core, e := b.VerifyCredential(b.Input{Credential: in.Credential, Trust: b.Trust{Issuer: ec.Issuer, KID: "test", PublicKey: mustPEM(t, key), Synthetic: true}, Policy: src.Policy, Authority: src.Authority, Status: src.Status})
	if e != nil || core.Crypto != "VALID" || core.Authority != "UNKNOWN" || core.Status != "UNKNOWN" || core.Eligible {
		t.Fatal("core trusted fixture assertions")
	}
	revoked := in
	revoked.Status = &Artifact{"AVAILABLE", makeStatus(2, true)}
	t.Run("concurrent-version-retention", func(t *testing.T) {
		s := NewState()
		var wg sync.WaitGroup
		for n := 0; n < 16; n++ {
			wg.Add(1)
			go func(n int) {
				defer wg.Done()
				x := in
				if n%2 == 1 {
					x = revoked
				}
				_, e := Verify(x, s)
				if e != nil {
					t.Error(e)
				}
			}(n)
		}
		wg.Wait()
		r, _ := Verify(in, s)
		if r.Credential.Status != "INVALID" {
			t.Fatal("rollback accepted")
		}
		offline := in
		offline.Trust = &Artifact{Availability: "UNAVAILABLE"}
		offline.Status = &Artifact{Availability: "UNAVAILABLE"}
		r, _ = Verify(offline, s)
		if r.Credential.Status != "REVOKED" {
			t.Fatal("known revocation overwritten")
		}
	})
	t.Run("expired-cache-no-success", func(t *testing.T) {
		s := NewState()
		_, _ = Verify(in, s)
		offline := in
		offline.Trust = &Artifact{Availability: "UNAVAILABLE"}
		offline.Status = &Artifact{Availability: "UNAVAILABLE"}
		offline.Policy.Now += 901
		r, e := Verify(offline, s)
		if e != nil || r.Material != "STALE" || r.Credential.Crypto != "VALID" || r.Credential.Eligible {
			t.Fatalf("%+v %v", r, e)
		}
	})
	t.Run("missing-status-not-good", func(t *testing.T) {
		x := in
		x.Status = nil
		r, _ := Verify(x, NewState())
		if r.Credential.Status != "UNKNOWN" || r.Credential.Eligible {
			t.Fatal("missing status accepted")
		}
	})
	t.Run("nil-state-and-synthetic-denial", func(t *testing.T) {
		if _, e := Verify(in, nil); e == nil {
			t.Fatal("missing trusted state")
		}
		x := in
		x.Policy.AllowSynthetic = false
		if _, e := Verify(x, NewState()); e == nil {
			t.Fatal("synthetic accepted as production")
		}
	})
	t.Run("private-and-null-fields", func(t *testing.T) {
		bb, _ := json.Marshal(m)
		var mm map[string]any
		_ = json.Unmarshal(bb, &mm)
		mm["reviewer_email"] = "prohibited"
		bb, _ = json.Marshal(mm)
		var decoded Manifest
		if strict(bb, &decoded) == nil {
			t.Fatal("private field accepted")
		}
		delete(mm, "reviewer_email")
		mm["authority"] = nil
		bb, _ = json.Marshal(mm)
		if strict(bb, &decoded) == nil {
			t.Fatal("null authority accepted")
		}
		var j JWK
		if strict([]byte(`{"kty":"RSA","n":"AA","e":"AQAB","kid":"test","alg":"RS256","use":"sig","key_ops":["verify"],"d":"invalid"}`), &j) == nil {
			t.Fatal("private key field accepted")
		}
	})
}
func mustPEM(t *testing.T, j JWK) string {
	s, e := PEM(j)
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func TestCanonicalBase64AndState(t *testing.T) {
	for _, v := range []string{"YQ=", "YQ\n", "YR", ""} {
		if _, e := decode64(v); e == nil {
			t.Fatalf("accepted %q", v)
		}
	}
	s := NewState()
	if !s.retain("scope", "v2", 2, 20, 0) || s.retain("scope", "v1", 1, 10, 0) || s.retain("scope", "evil", 2, 20, 0) || s.retain("scope", "v3", 3, 19, 0) {
		t.Fatal("rollback/equivocation")
	}
}

func TestDecisionSnapshotMustStillBeCurrent(t *testing.T) {
	s := NewState()
	s.retain("trust", "trust1", 1, 1, 0)
	s.retain("status", "status1", 1, 1, 0)
	r := blank(Input{})
	r.Material = "VALID"
	r.Key = "AUTHENTICATED"
	r.Credential.Crypto = "VALID"
	r.Credential.Binding = "VALID"
	r.Credential.Temporal = "VALID"
	r.Credential.Authority = "AUTHORIZED"
	r.Credential.Status = "GOOD"
	if finalize(r, s, "trust", "trust1", "status", "status1").Overall != "ELIGIBLE_WITHOUT_HOLDER" {
		t.Fatal("current snapshot denied")
	}
	s.retain("trust", "trust2", 2, 2, 0)
	s.retain("status", "status2", 2, 2, 0)
	if s.retainStatus("trust", "trust1", "status", "poison99", 99, 99, 0) != "TRUST_CHANGED" || s.cached("status") != "status2" {
		t.Fatal("superseded descriptor poisoned status cache")
	}
	v := finalize(r, s, "trust", "trust1", "status", "status1")
	if v.Credential.Eligible || v.Overall != "REJECTED" || v.Material != "ROLLED_BACK" || v.Credential.Status != "INVALID" {
		t.Fatal("superseded positive decision", v)
	}
}
