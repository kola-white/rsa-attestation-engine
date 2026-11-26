// Package portableverify is a local interoperability boundary, not a production
// trust service. Trust is supplied by the verifier, never learned from a token.
package portableverify

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/credential"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/credential/sdjwtvc"
	ec "github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/evidencecredential"
)

const Contract = "cvera.stage4b.portable-verification.v1"

type Trust struct {
	Issuer          string `json:"issuer"`
	KID             string `json:"kid"`
	PublicKey       string `json:"publicKey"`
	AuthorityIssuer string `json:"authorityIssuer"`
	AuthorityKID    string `json:"authorityKid"`
	AuthorityKey    string `json:"authorityKey"`
	StatusIssuer    string `json:"statusIssuer"`
	StatusKID       string `json:"statusKid"`
	StatusKey       string `json:"statusKey"`
	Synthetic       bool   `json:"synthetic"`
}
type Policy struct {
	Now             int64  `json:"now"`
	ExpectedSubject string `json:"expectedSubject"`
	MaxStatusAge    *int64 `json:"maxStatusAgeSeconds,omitempty"`
}
type Status struct {
	Availability string `json:"availability"`
	JWS          string `json:"jws"`
}
type Input struct {
	Credential string  `json:"credential"`
	Trust      Trust   `json:"trust"`
	Policy     Policy  `json:"policy"`
	Authority  string  `json:"authorityJws"`
	Status     *Status `json:"status,omitempty"`
	Holder     string  `json:"holder,omitempty"`
}
type Result struct {
	Contract  string `json:"contract"`
	Synthetic bool   `json:"synthetic"`
	Crypto    string `json:"cryptographicIntegrity"`
	Binding   string `json:"claimBinding"`
	Temporal  string `json:"temporalValidity"`
	Authority string `json:"issuerAuthority"`
	Status    string `json:"credentialStatus"`
	Holder    string `json:"holderAssurance"`
	Eligible  bool   `json:"eligibleWithoutHolder"`
	Overall   string `json:"overall"`
}
type pinned struct {
	t   Trust
	key *rsa.PublicKey
}

func (p pinned) ResolveIssuerKey(_ context.Context, issuer, kid, alg string) (any, string, error) {
	if issuer != p.t.Issuer || kid != p.t.KID || alg != "RS256" {
		return nil, "", errors.New("untrusted key")
	}
	return p.key, "explicit-local-public-trust", nil
}
func key(s string) (*rsa.PublicKey, error) {
	b, rest := pem.Decode([]byte(s))
	if b == nil || b.Type != "PUBLIC KEY" || len(strings.TrimSpace(string(rest))) != 0 {
		return nil, errors.New("public SPKI required")
	}
	v, e := x509.ParsePKIXPublicKey(b.Bytes)
	if e != nil {
		return nil, e
	}
	k, ok := v.(*rsa.PublicKey)
	if !ok || k.N.BitLen() < 3072 || k.E != 65537 {
		return nil, errors.New("unsupported key")
	}
	return k, nil
}
func decode(raw string) (map[string]any, error) {
	p := strings.Split(raw, ".")
	if len(p) != 3 {
		return nil, errors.New("compact JWT required")
	}
	b, e := base64.RawURLEncoding.DecodeString(p[1])
	if e != nil {
		return nil, e
	}
	var m map[string]any
	e = json.Unmarshal(b, &m)
	return m, e
}

// Presence matters: JSON null must not silently disable a protected-header guard.
func unsupportedHeaders(h map[string]any) bool {
	_, critical := h["crit"]
	_, encoding := h["b64"]
	return critical || encoding
}

func assertion(raw, pub, issuer, kid, typ string) (jwt.MapClaims, error) {
	k, e := key(pub)
	if e != nil {
		return nil, e
	}
	t, e := jwt.Parse(raw, func(t *jwt.Token) (any, error) {
		if unsupportedHeaders(t.Header) || t.Header["typ"] != typ || t.Header["kid"] != kid {
			return nil, errors.New("header mismatch")
		}
		return k, nil
	}, jwt.WithValidMethods([]string{"RS256"}), jwt.WithoutClaimsValidation())
	if e != nil || t == nil || !t.Valid {
		return nil, errors.New("invalid assertion")
	}
	m, ok := t.Claims.(jwt.MapClaims)
	if !ok || m["iss"] != issuer || m["synthetic"] != true {
		return nil, errors.New("wrong assertion scope")
	}
	return m, nil
}
func integer(v any) (int64, bool) {
	n, ok := v.(float64)
	return int64(n), ok && n >= 0 && n <= 9007199254740991 && float64(int64(n)) == n
}
func str(m map[string]any, k string) string { s, _ := m[k].(string); return s }

var uuidRE = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
var hashRE = regexp.MustCompile(`^[0-9a-f]{64}$`)

func Verify(in Input) (Result, error) {
	r := Result{Contract: Contract, Synthetic: true, Crypto: "UNKNOWN", Binding: "UNKNOWN", Temporal: "UNKNOWN", Authority: "UNKNOWN", Status: "UNKNOWN", Holder: "UNKNOWN", Overall: "INDETERMINATE"}
	maxAge := int64(900)
	if in.Policy.MaxStatusAge != nil {
		maxAge = *in.Policy.MaxStatusAge
	}
	if in.Policy.Now <= 0 || in.Policy.Now > 9007199254740991 || maxAge < 0 || maxAge > 9007199254740991 || !in.Trust.Synthetic {
		return r, errors.New("invalid verifier policy or non-synthetic trust")
	}
	if in.Holder == "UNSUPPORTED" {
		r.Holder = "UNSUPPORTED"
	}
	finish := func() (Result, error) {
		r.Eligible = r.Crypto == "VALID" && r.Binding == "VALID" && r.Temporal == "VALID" && r.Authority == "AUTHORIZED" && r.Status == "GOOD"
		if r.Crypto == "INVALID" || r.Crypto == "UNSUPPORTED" || r.Binding == "INVALID" || r.Temporal == "INVALID" || r.Authority == "INVALID" || r.Authority == "UNAUTHORIZED" || r.Status == "INVALID" || r.Status == "REVOKED" {
			r.Overall = "REJECTED"
		}
		return r, nil
	}
	raw := in.Credential
	if len(raw) > 1<<20 || !strings.HasSuffix(raw, "~") || strings.Count(raw, "~") != 1 {
		r.Crypto = "UNSUPPORTED"
		return finish()
	}
	jwtRaw := strings.TrimSuffix(raw, "~")
	parts := strings.Split(jwtRaw, ".")
	if len(parts) != 3 {
		r.Crypto = "INVALID"
		return finish()
	}
	hb, e := base64.RawURLEncoding.DecodeString(parts[0])
	var h map[string]any
	if e != nil || json.Unmarshal(hb, &h) != nil {
		r.Crypto = "INVALID"
		return finish()
	}
	if unsupportedHeaders(h) {
		r.Crypto = "INVALID"
		return finish()
	}
	if h["alg"] != "RS256" || h["typ"] != "dc+sd-jwt" {
		r.Crypto = "UNSUPPORTED"
		return finish()
	}
	k, e := key(in.Trust.PublicKey)
	if e != nil {
		r.Crypto = "INVALID"
		return finish()
	}
	// Existing CredentialProfile performs signature checking and normalization;
	// the resolver is a pinned public key with no HTTP or database capability.
	profile := sdjwtvc.New(pinned{in.Trust, k}, sdjwtvc.WithExpectedVCT(ec.VCT))
	ev, e := profile.VerifyAndNormalize(context.Background(), []byte(raw), credential.VerificationOptions{Now: time.Unix(in.Policy.Now, 0)})
	if e != nil || ev.Cryptographic.Signature != credential.EvidenceValid {
		r.Crypto = "INVALID"
		return finish()
	}
	r.Crypto = "VALID"
	m, e := decode(jwtRaw)
	if e != nil {
		r.Binding = "INVALID"
		return finish()
	}
	if m["vct"] != ec.VCT || m["iss"] != ec.Issuer || in.Trust.Issuer != ec.Issuer || m["claim_type"] != credential.ClaimTypeEmploymentRole || m["verification_policy"] != ec.Policy {
		r.Crypto = "UNSUPPORTED"
		return finish()
	}
	var c ec.Claims
	b, _ := json.Marshal(m)
	errClaims := json.Unmarshal(b, &c)
	valid := errClaims == nil && in.Policy.ExpectedSubject != "" && c.Subject == in.Policy.ExpectedSubject && c.Subject == c.Evidence.SubjectID && c.Employment.EmployerID != "" && c.Employment.EmployerID == c.Evidence.EmployerID && strings.TrimSpace(c.Employment.Title) != "" && c.Evidence.CaseID != "" && c.Evidence.ManualPolicy == ec.ManualPolicy && c.Evidence.AuthorityGrantID != "" && c.CredentialFingerprint == ec.Fingerprint(k)
	for _, v := range []string{c.Subject, c.ID, c.Evidence.RequestID, c.Evidence.BindingID, c.Evidence.ManifestID} {
		valid = valid && uuidRE.MatchString(v)
	}
	for _, v := range []string{c.Evidence.SnapshotDigest, c.Evidence.ManifestDigest, c.CredentialFingerprint} {
		valid = valid && hashRE.MatchString(v)
	}
	eb, _ := m["evidence_binding"].(map[string]any)
	ver, vok := integer(eb["request_version"])
	pos, pok := integer(eb["authority_recording_position"])
	valid = valid && vok && ver > 0 && pok && pos > 0
	r.Binding = "INVALID"
	if valid {
		r.Binding = "VALID"
	}
	iat, iok := integer(m["iat"])
	nbf, nok := integer(m["nbf"])
	exp, eok := integer(m["exp"])
	r.Temporal = "INVALID"
	if iok && nok && eok && iat == nbf && exp > iat && in.Policy.Now >= nbf && in.Policy.Now < exp {
		r.Temporal = "VALID"
	}
	digest := ec.Digest([]byte(raw))
	if in.Authority != "" {
		a, e := assertion(in.Authority, in.Trust.AuthorityKey, in.Trust.AuthorityIssuer, in.Trust.AuthorityKID, "cvera-stage4b-fixture-authority+jwt")
		if e != nil {
			r.Authority = "INVALID"
		} else {
			ai, ao := integer(a["iat"])
			ae, ap := integer(a["exp"])
			if !ao || !ap || ai > in.Policy.Now || ae <= in.Policy.Now || a["credential_sha256"] != digest || a["issuer"] != c.Issuer || a["employer_id"] != c.Evidence.EmployerID || a["claim_type"] != c.ClaimType {
				r.Authority = "INVALID"
			} else {
				switch a["decision"] {
				case "AUTHORIZED":
					r.Authority = "AUTHORIZED"
				case "UNAUTHORIZED":
					r.Authority = "UNAUTHORIZED"
				case "UNKNOWN":
					r.Authority = "UNKNOWN"
				default:
					r.Authority = "INVALID"
				}
			}
		}
	}
	r.Status = statusEvidence(in, digest, c.ID, maxAge)
	return finish()
}

// Validate authenticity and binding before trusting either age or state.
func statusEvidence(in Input, digest, credentialID string, maxAge int64) string {
	if in.Status == nil || in.Status.Availability == "UNAVAILABLE" {
		return "UNKNOWN"
	}
	if in.Status.Availability != "AVAILABLE" {
		return "INVALID"
	}
	a, err := assertion(in.Status.JWS, in.Trust.StatusKey, in.Trust.StatusIssuer, in.Trust.StatusKID, "cvera-stage4b-fixture-status+jwt")
	if err != nil {
		return "INVALID"
	}
	at, ok := integer(a["iat"])
	if !ok || at > in.Policy.Now || a["credential_sha256"] != digest {
		return "INVALID"
	}
	entries, ok := a["entries"].([]any)
	if !ok {
		return "INVALID"
	}
	matches, state := 0, ""
	for _, value := range entries {
		entry, ok := value.(map[string]any)
		if !ok {
			return "INVALID"
		}
		if entry["credential_id"] == credentialID {
			matches++
			state = str(entry, "state")
		}
	}
	if matches == 0 {
		return "UNKNOWN"
	}
	if matches > 1 || (state != "GOOD" && state != "REVOKED") {
		return "INVALID"
	}
	if in.Policy.Now-at > maxAge {
		return "STALE"
	}
	return state
}
