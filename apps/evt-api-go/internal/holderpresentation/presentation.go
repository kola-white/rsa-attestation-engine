// Package holderpresentation implements a bounded, synthetic, fully disclosed
// SD-JWT presentation contract. It has no network, database or production route.
package holderpresentation

import (
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"strings"
	"sync"

	"github.com/golang-jwt/jwt/v5"
	b "github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/portableverify"
)

const VCT = "https://cvera.app/credentials/employment-role/v1"
const Contract = "cvera.stage4c.presentation.v1"

type Verifier struct {
	ID          string `json:"id"`
	KID         string `json:"kid"`
	Key         string `json:"key"`
	ResponseURI string `json:"responseURI"`
	Allow       bool   `json:"allow"`
}
type Consent struct {
	RequestHash    string `json:"requestHash"`
	CredentialHash string `json:"credentialHash"`
	Expires        int64  `json:"expires"`
	Granted        bool   `json:"granted"`
}
type Transaction struct {
	Verifier    string `json:"verifier"`
	Nonce       string `json:"nonce"`
	RequestHash string `json:"requestHash"`
	Expires     int64  `json:"expires"`
}
type Input struct {
	Base                b.Input     `json:"base"`
	Request             string      `json:"request"`
	Verifiers           []Verifier  `json:"verifiers"`
	AllowFullDisclosure bool        `json:"allowFullDisclosure"`
	Consent             *Consent    `json:"consent,omitempty"`
	Presentation        string      `json:"presentation,omitempty"`
	Transaction         Transaction `json:"transaction"`
}
type Result struct {
	Contract       string   `json:"contract"`
	Base           b.Result `json:"credential"`
	Authentication string   `json:"verifierAuthentication"`
	Authorization  string   `json:"disclosureAuthorization"`
	Consent        string   `json:"holderAuthorization"`
	Possession     string   `json:"holderPossession"`
	Transaction    string   `json:"transactionBinding"`
	Overall        string   `json:"overall"`
}

// Store is verifier-owned, atomic, single-process state. Register only locally
// generated challenges, never challenges supplied by an incoming presentation.
// A restart invalidates all outstanding transactions. Distributed stores are deferred.
type Store struct {
	mu   sync.Mutex
	seen map[string]bool
}

func NewStore() *Store { return &Store{seen: map[string]bool{}} }
func (s *Store) Consume(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.seen[key] {
		return false
	}
	s.seen[key] = true
	return true
}
func Hash(s string) string {
	h := sha256.Sum256([]byte(s))
	return base64.RawURLEncoding.EncodeToString(h[:])
}
func public(s string) (*rsa.PublicKey, error) {
	p, r := pem.Decode([]byte(s))
	if !strings.HasPrefix(s, "-----BEGIN PUBLIC KEY-----\n") || p == nil || p.Type != "PUBLIC KEY" || strings.TrimSpace(string(r)) != "" {
		return nil, errors.New("SPKI required")
	}
	v, e := x509.ParsePKIXPublicKey(p.Bytes)
	if e != nil {
		return nil, e
	}
	k, ok := v.(*rsa.PublicKey)
	if !ok || k.N.BitLen() < 3072 || k.E != 65537 {
		return nil, errors.New("unsupported RSA")
	}
	return k, nil
}
func JWK(k *rsa.PublicKey) map[string]any {
	return map[string]any{"kty": "RSA", "n": base64.RawURLEncoding.EncodeToString(k.N.Bytes()), "e": "AQAB"}
}
func jwk(v any) (*rsa.PublicKey, error) {
	m, ok := v.(map[string]any)
	if !ok || len(m) != 3 || m["kty"] != "RSA" || m["e"] != "AQAB" {
		return nil, errors.New("public RSA JWK required")
	}
	n, ok := m["n"].(string)
	if !ok {
		return nil, errors.New("modulus")
	}
	bb, e := base64.RawURLEncoding.DecodeString(n)
	if e != nil || len(bb) == 0 || bb[0] == 0 {
		return nil, errors.New("modulus")
	}
	k := &rsa.PublicKey{N: new(big.Int).SetBytes(bb), E: 65537}
	if k.N.BitLen() < 3072 {
		return nil, errors.New("weak key")
	}
	return k, nil
}
func payload(s string) (jwt.MapClaims, error) {
	var m jwt.MapClaims
	p := strings.Split(s, ".")
	if len(p) != 3 || len(s) > 1<<20 {
		return nil, errors.New("compact JWT")
	}
	v, e := base64.RawURLEncoding.DecodeString(p[1])
	if e == nil {
		e = json.Unmarshal(v, &m)
	}
	return m, e
}
func verify(s string, k *rsa.PublicKey, typ, kid string) (jwt.MapClaims, error) {
	t, e := jwt.Parse(s, func(t *jwt.Token) (any, error) {
		for name := range t.Header {
			if typ == "openid4vci-proof+jwt" && name == "jwk" {
				continue
			}
			if name != "typ" && name != "alg" && name != "kid" {
				return nil, errors.New("unsupported header")
			}
		}
		if t.Header["typ"] != typ || (kid != "" && t.Header["kid"] != kid) {
			return nil, errors.New("header")
		}
		if typ == "openid4vci-proof+jwt" {
			embedded, err := jwk(t.Header["jwk"])
			if err != nil || embedded.N.Cmp(k.N) != 0 || embedded.E != k.E {
				return nil, errors.New("proof key mismatch")
			}
			if _, exists := t.Header["kid"]; exists {
				return nil, errors.New("ambiguous proof key")
			}
		}
		return k, nil
	}, jwt.WithValidMethods([]string{"RS256"}), jwt.WithoutClaimsValidation())
	if e != nil || t == nil || !t.Valid {
		return nil, errors.New("signature")
	}
	return t.Claims.(jwt.MapClaims), nil
}
func str(m jwt.MapClaims, k string) string { s, _ := m[k].(string); return s }
func num(m jwt.MapClaims, k string) int64 {
	v, ok := m[k].(float64)
	if !ok || v < 0 || v > 9007199254740991 || v != float64(int64(v)) {
		return -1
	}
	return int64(v)
}
func optionalTimes(m jwt.MapClaims, now int64) bool {
	for _, name := range []string{"exp", "nbf"} {
		if _, exists := m[name]; exists {
			n := num(m, name)
			if n < 0 || (name == "exp" && n <= now) || (name == "nbf" && n > now) {
				return false
			}
		}
	}
	return true
}
func window(m jwt.MapClaims, now int64) bool {
	i, e := num(m, "iat"), num(m, "exp")
	return i >= 0 && i <= now && now-i <= 300 && e > now && e > i && e-i <= 300
}
func queryOK(v any) bool {
	expected := `{"credentials":[{"format":"dc+sd-jwt","id":"employment","meta":{"vct_values":["https://cvera.app/credentials/employment-role/v1"]},"require_cryptographic_holder_binding":true}]}`
	var want any
	_ = json.Unmarshal([]byte(expected), &want)
	a, _ := json.Marshal(v)
	bb, _ := json.Marshal(want)
	return string(a) == string(bb)
}
func authenticate(in Input) (jwt.MapClaims, Verifier, string) {
	m, e := payload(in.Request)
	if e != nil {
		return nil, Verifier{}, "INVALID"
	}
	id := str(m, "client_id")
	var found *Verifier
	for _, v := range in.Verifiers {
		if v.ID == id {
			if found != nil {
				return nil, v, "INVALID"
			}
			vv := v
			found = &vv
		}
	}
	if found == nil {
		return nil, Verifier{}, "UNKNOWN"
	}
	v := *found
	k, e := public(v.Key)
	if e != nil {
		return nil, v, "INVALID"
	}
	m, e = verify(in.Request, k, "oauth-authz-req+jwt", v.KID)
	for name := range m {
		switch name {
		case "iss", "client_id", "aud", "iat", "exp", "response_type", "response_mode", "response_uri", "nonce", "state", "dcql_query":
		default:
			return nil, v, "INVALID"
		}
	}
	if e != nil || strings.Contains(id, ":") || id == "" || m["iss"] != id || m["aud"] != "https://self-issued.me/v2" || !window(m, in.Base.Policy.Now) || m["response_type"] != "vp_token" || m["response_mode"] != "direct_post" || !strings.HasPrefix(v.ResponseURI, "https://") || m["response_uri"] != v.ResponseURI || len(str(m, "nonce")) < 22 || len(str(m, "state")) < 22 {
		return nil, v, "INVALID"
	}
	return m, v, "AUTHENTICATED"
}

// CredentialEvaluator is trusted, read-only integration code, never a serialized
// trust hint. It may be called again immediately before release/acceptance.
type CredentialEvaluator func(b.Input) (b.Result, error)

// EvaluateHolder is an independent holder-side decision. Consent is a trusted
// local UI decision, not a transferable proof of human consent to a verifier.
func EvaluateHolder(in Input) Result { return EvaluateHolderWithEvaluator(in, b.Verify) }
func EvaluateHolderWithEvaluator(in Input, evaluate CredentialEvaluator) Result {
	r, m, v := initial(in, evaluate)
	if r.Authentication != "AUTHENTICATED" {
		return r
	}
	if !v.Allow || !in.AllowFullDisclosure || !queryOK(m["dcql_query"]) {
		r.Authorization = "DENIED"
		return r
	}
	r.Authorization = "AUTHORIZED"
	if in.Consent == nil {
		return r
	}
	c := in.Consent
	if !c.Granted || c.Expires <= in.Base.Policy.Now || c.RequestHash != Hash(in.Request) || c.CredentialHash != Hash(in.Base.Credential) {
		r.Consent = "DENIED"
		return r
	}
	r.Consent = "AUTHORIZED"
	if r.Base.Eligible {
		r.Overall = "AUTHORIZED_TO_PRESENT"
	}
	return r
}
func initial(in Input, evaluate CredentialEvaluator) (Result, jwt.MapClaims, Verifier) {
	if evaluate == nil {
		evaluate = func(b.Input) (b.Result, error) { return b.Result{}, errors.New("missing trusted evaluator") }
	}
	br, e := evaluate(in.Base)
	r := Result{Contract: Contract, Base: br, Authentication: "UNKNOWN", Authorization: "UNKNOWN", Consent: "UNKNOWN", Possession: "UNKNOWN", Transaction: "UNKNOWN", Overall: "DENIED"}
	if e != nil {
		return r, nil, Verifier{}
	}
	m, v, a := authenticate(in)
	r.Authentication = a
	return r, m, v
}
func holderKey(raw string) (*rsa.PublicKey, error) {
	m, e := payload(strings.TrimSuffix(raw, "~"))
	if e != nil {
		return nil, e
	}
	if a, exists := m["_sd_alg"]; exists && a != "sha-256" {
		return nil, errors.New("hash algorithm")
	}
	cnf, ok := m["cnf"].(map[string]any)
	if !ok || len(cnf) != 1 {
		return nil, errors.New("cnf required")
	}
	return jwk(cnf["jwk"])
}

// Present returns no protected bytes unless all holder-side gates pass. The
// credential signature is verified by the reused Stage4B profile before cnf use.
func Present(in Input, k *rsa.PrivateKey) (string, Result, error) {
	return PresentWithEvaluator(in, k, b.Verify)
}
func PresentWithEvaluator(in Input, k *rsa.PrivateKey, evaluate CredentialEvaluator) (string, Result, error) {
	r := EvaluateHolderWithEvaluator(in, evaluate)
	if r.Overall != "AUTHORIZED_TO_PRESENT" {
		return "", r, errors.New("disclosure denied")
	}
	hk, e := holderKey(in.Base.Credential)
	if e != nil || k == nil || k.N == nil || hk.N.Cmp(k.N) != 0 || hk.E != k.E {
		return "", r, errors.New("holder key mismatch")
	}
	m, _, _ := authenticate(in)
	t := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{"iat": in.Base.Policy.Now, "aud": m["client_id"], "nonce": m["nonce"], "sd_hash": Hash(in.Base.Credential)})
	t.Header["typ"] = "kb+jwt"
	kb, e := t.SignedString(k)
	if e != nil {
		return "", r, e
	}
	current, err := evaluate(in.Base)
	r.Base = current
	if err != nil || !current.Eligible {
		r.Overall = "DENIED"
		return "", r, errors.New("credential trust changed before release")
	}
	r.Possession = "VALID"
	return in.Base.Credential + kb, r, nil
}

// Verify requires verifier-owned original request and transaction inputs. No
// assertion sent by a holder can register a transaction or grant authorization.
func Verify(in Input, s *Store) Result { return VerifyWithEvaluator(in, s, b.Verify) }
func VerifyWithEvaluator(in Input, s *Store, evaluate CredentialEvaluator) Result {
	r, m, v := initial(in, evaluate)
	r.Consent = "NOT_OBSERVABLE"
	if r.Authentication != "AUTHENTICATED" {
		return r
	}
	if !v.Allow || !in.AllowFullDisclosure || !queryOK(m["dcql_query"]) {
		r.Authorization = "DENIED"
		return r
	}
	r.Authorization = "AUTHORIZED"
	if !r.Base.Eligible {
		return r
	}
	r.Possession = "INVALID"
	if !strings.HasPrefix(in.Presentation, in.Base.Credential) {
		return r
	}
	kb := strings.TrimPrefix(in.Presentation, in.Base.Credential)
	k, e := holderKey(in.Base.Credential)
	if e != nil {
		return r
	}
	claims, e := verify(kb, k, "kb+jwt", "")
	if e != nil || claims["aud"] != v.ID || claims["nonce"] != m["nonce"] || claims["sd_hash"] != Hash(in.Base.Credential) {
		return r
	}
	for _, name := range []string{"exp", "nbf"} {
		if _, exists := claims[name]; exists {
			n := num(claims, name)
			if n < 0 || (name == "exp" && n <= in.Base.Policy.Now) || (name == "nbf" && n > in.Base.Policy.Now) {
				return r
			}
		}
	}
	at := num(claims, "iat")
	if at < 0 || at > in.Base.Policy.Now || in.Base.Policy.Now-at > 300 {
		return r
	}
	r.Possession = "VALID"
	r.Transaction = "INVALID"
	tx := in.Transaction
	if tx.Verifier != v.ID || tx.Nonce != str(m, "nonce") || tx.RequestHash != Hash(in.Request) || tx.Expires <= in.Base.Policy.Now || tx.Expires > num(m, "exp") || s == nil {
		return r
	}
	current, err := evaluate(in.Base)
	r.Base = current
	if err != nil || !current.Eligible {
		r.Transaction = "UNKNOWN"
		return r
	}
	if !s.Consume(tx.Verifier + "/" + tx.Nonce) {
		r.Transaction = "REPLAYED"
		return r
	}
	r.Transaction = "VALID"
	r.Overall = "ACCEPTED_SYNTHETIC_PRESENTATION"
	return r
}

// IssueBound is a local integration seam, NOT an issuance authorization route.
// The caller supplies already-authorized claims and issuer-owned challenge state.
// Possession is established before the callback can sign issuer-protected cnf.
func IssueBound(claims jwt.MapClaims, intended *rsa.PublicKey, proof string, issuer, nonce string, now, expires int64, store *Store, sign func(jwt.MapClaims) (string, error)) (string, error) {
	if intended == nil || intended.N == nil || intended.N.BitLen() < 3072 || intended.E != 65537 || now <= 0 || expires <= now || expires-now > 300 || len(nonce) < 22 || issuer == "" || claims["iss"] != issuer || store == nil || sign == nil {
		return "", errors.New("issuer context")
	}
	p, e := verify(proof, intended, "openid4vci-proof+jwt", "")
	if e != nil || !optionalTimes(p, now) || p["aud"] != issuer || p["nonce"] != nonce || num(p, "iat") < 0 || num(p, "iat") > now || now-num(p, "iat") > 300 {
		return "", errors.New("proof")
	}
	if !store.Consume(issuer + "/" + nonce) {
		return "", errors.New("proof replay")
	}
	copy := jwt.MapClaims{}
	for k, v := range claims {
		copy[k] = v
	}
	copy["cnf"] = map[string]any{"jwk": JWK(intended)}
	artifact, err := sign(copy)
	if err != nil {
		return "", err
	}
	return artifact, nil
}
