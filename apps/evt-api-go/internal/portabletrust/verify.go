// Package portabletrust authenticates portable issuer keys, scoped authority and
// OAuth Status List JWTs. It has no Cvera service or database dependency.
package portabletrust

import (
	"bytes"
	"compress/zlib"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net/url"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/golang-jwt/jwt/v5"
	b "github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/portableverify"
)

const Contract = "cvera.stage4d.portable-trust.v1"
const VCT = "https://cvera.app/credentials/employment-role/v1"
const Profile = "draft-ietf-oauth-sd-jwt-vc-18"
const MaxBytes = 1 << 20
const safeMax int64 = 9007199254740991

type JWK struct {
	Kty string   `json:"kty"`
	N   string   `json:"n"`
	E   string   `json:"e"`
	KID string   `json:"kid"`
	Alg string   `json:"alg"`
	Use string   `json:"use"`
	Ops []string `json:"key_ops"`
}
type Anchor struct {
	Issuer    string `json:"issuer"`
	KID       string `json:"kid"`
	Key       JWK    `json:"key"`
	Synthetic bool   `json:"synthetic"`
}
type Policy struct {
	Now              int64  `json:"now"`
	ExpectedSubject  string `json:"expectedSubject"`
	Issuer           string `json:"issuer"`
	TrustURI         string `json:"trustURI"`
	AllowSynthetic   bool   `json:"allowSynthetic"`
	AuthorityMode    string `json:"authorityMode"`
	MaxStatusAge     *int64 `json:"maxStatusAgeSeconds,omitempty"`
	MaxTrustAge      *int64 `json:"maxTrustAgeSeconds,omitempty"`
	MinTrustVersion  int64  `json:"minTrustVersion"`
	MinStatusVersion int64  `json:"minStatusVersion"`
}
type Artifact struct {
	Availability string `json:"availability"`
	JWS          string `json:"jws"`
}
type Input struct {
	Credential string    `json:"credential"`
	Anchor     Anchor    `json:"anchor"`
	Policy     Policy    `json:"policy"`
	Trust      *Artifact `json:"trustMaterial"`
	Status     *Artifact `json:"statusMaterial"`
}
type Result struct {
	Contract          string   `json:"contract"`
	Material          string   `json:"trustMaterial"`
	Key               string   `json:"verificationKey"`
	CurrentAuthority  string   `json:"currentAuthority"`
	IssuanceAuthority string   `json:"issuanceAuthority"`
	Credential        b.Result `json:"credential"`
	Overall           string   `json:"overall"`
}
type Key struct {
	JWK   JWK   `json:"jwk"`
	From  int64 `json:"from"`
	Until int64 `json:"until"`
}
type Grant struct {
	Employer string `json:"employer_id"`
	Claim    string `json:"claim_type"`
	Policy   string `json:"policy"`
	From     int64  `json:"from"`
	Until    int64  `json:"until"`
	Decision string `json:"decision"`
}
type StatusKey struct {
	Issuer  string `json:"issuer"`
	URI     string `json:"uri"`
	Purpose string `json:"purpose"`
	JWK     JWK    `json:"jwk"`
}
type Manifest struct {
	Issuer           string    `json:"iss"`
	Subject          string    `json:"sub"`
	Iat              int64     `json:"iat"`
	Exp              int64     `json:"exp"`
	Version          int64     `json:"version"`
	Synthetic        bool      `json:"synthetic"`
	Profile          string    `json:"profile"`
	CredentialIssuer string    `json:"credential_issuer"`
	VCT              string    `json:"vct"`
	Keys             []Key     `json:"keys"`
	Authority        []Grant   `json:"authority"`
	Status           StatusKey `json:"status"`
}
type List struct {
	Bits int    `json:"bits"`
	Lst  string `json:"lst"`
}
type StatusToken struct {
	Issuer           string `json:"iss"`
	Subject          string `json:"sub"`
	Iat              int64  `json:"iat"`
	Exp              int64  `json:"exp"`
	TTL              int64  `json:"ttl"`
	Version          int64  `json:"cvera_version"`
	Synthetic        bool   `json:"synthetic"`
	VCT              string `json:"cvera_vct"`
	CredentialIssuer string `json:"cvera_credential_issuer"`
	List             List   `json:"status_list"`
}
type entry struct {
	Version, Iat int64
	Raw          string
}

// State is trusted verifier-owned retained memory. Never import it from a holder.
// Its lock atomically checks version, timestamp and same-version equivocation.
// Restart loses knowledge; configured minimum versions provide an independent floor.
type State struct {
	mu   sync.Mutex
	data map[string]entry
}

func NewState() *State { return &State{data: map[string]entry{}} }
func (s *State) cached(scope string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data[scope].Raw
}
func (s *State) retain(scope, raw string, v, at, floor int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data == nil {
		s.data = map[string]entry{}
	}
	old, ok := s.data[scope]
	if v < floor || (ok && (v < old.Version || at < old.Iat || (v == old.Version && raw != old.Raw))) {
		return false
	}
	s.data[scope] = entry{v, at, raw}
	return true
}

// Exact public schema, including required fields and types. No nulls or
// case-insensitive aliases of security-critical fields are accepted.
func shape(v any, t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Struct:
		m, ok := v.(map[string]any)
		if !ok || len(m) != t.NumField() {
			return false
		}
		for n := 0; n < t.NumField(); n++ {
			f := t.Field(n)
			name := strings.Split(f.Tag.Get("json"), ",")[0]
			x, ok := m[name]
			if !ok || !shape(x, f.Type) {
				return false
			}
		}
		return true
	case reflect.Slice:
		a, ok := v.([]any)
		if !ok {
			return false
		}
		for _, x := range a {
			if !shape(x, t.Elem()) {
				return false
			}
		}
		return true
	case reflect.String:
		_, ok := v.(string)
		return ok
	case reflect.Bool:
		_, ok := v.(bool)
		return ok
	case reflect.Int, reflect.Int64:
		n, ok := v.(json.Number)
		if !ok {
			return false
		}
		i, e := n.Int64()
		return e == nil && number(i)
	}
	return false
}
func strict(data []byte, v any) error {
	if !utf8.Valid(data) {
		return errors.New("UTF8")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	var m any
	if d.Decode(&m) != nil || d.Decode(new(any)) != io.EOF || !shape(m, reflect.TypeOf(v).Elem()) {
		return errors.New("public schema")
	}
	return json.Unmarshal(data, v)
}

var url64 = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func decode64(s string) ([]byte, error) {
	if !url64.MatchString(s) {
		return nil, errors.New("base64url")
	}
	b, e := base64.RawURLEncoding.Strict().DecodeString(s)
	if e != nil || base64.RawURLEncoding.EncodeToString(b) != s {
		return nil, errors.New("canonical base64url")
	}
	return b, nil
}

func KeyRSA(j JWK) (*rsa.PublicKey, error) {
	if j.Kty != "RSA" || j.E != "AQAB" || j.KID == "" || j.Alg != "RS256" || j.Use != "sig" || len(j.Ops) != 1 || j.Ops[0] != "verify" {
		return nil, errors.New("public signing JWK required")
	}
	v, e := decode64(j.N)
	if e != nil || len(v) == 0 || v[0] == 0 || len(v) > 1024 {
		return nil, errors.New("modulus")
	}
	k := &rsa.PublicKey{N: new(big.Int).SetBytes(v), E: 65537}
	if k.N.BitLen() < 3072 {
		return nil, errors.New("weak RSA")
	}
	return k, nil
}
func PEM(j JWK) (string, error) {
	k, e := KeyRSA(j)
	if e != nil {
		return "", e
	}
	d, e := x509.MarshalPKIXPublicKey(k)
	if e != nil {
		return "", e
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: d})), nil
}
func jwtPayload(raw string, j JWK, typ string, v any) error {
	if len(raw) > MaxBytes {
		return errors.New("size")
	}
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return errors.New("JWT")
	}
	for _, p := range parts {
		if _, e := decode64(p); e != nil {
			return e
		}
	}
	for _, p := range parts[:2] {
		v, _ := decode64(p)
		if !utf8.Valid(v) {
			return errors.New("UTF8")
		}
	}
	key, e := KeyRSA(j)
	if e != nil {
		return e
	}
	t, e := jwt.Parse(raw, func(t *jwt.Token) (any, error) {
		if len(t.Header) != 3 || t.Header["alg"] != "RS256" || t.Header["typ"] != typ || t.Header["kid"] != j.KID {
			return nil, errors.New("header")
		}
		return key, nil
	}, jwt.WithValidMethods([]string{"RS256"}), jwt.WithoutClaimsValidation())
	if e != nil || t == nil || !t.Valid {
		return errors.New("signature")
	}
	p := strings.Split(raw, ".")
	data, e := base64.RawURLEncoding.DecodeString(p[1])
	if e != nil {
		return e
	}
	return strict(data, v)
}
func uri(s string) bool {
	u, e := url.Parse(s)
	return e == nil && u.Scheme == "https" && u.Host != "" && u.User == nil && u.RawQuery == "" && u.Fragment == ""
}
func number(n int64) bool   { return n >= 0 && n <= safeMax }
func times(i, e int64) bool { return number(i) && number(e) && e > i }
func max(p *int64) int64 {
	if p == nil {
		return 900
	}
	return *p
}
func scope(in Input) string {
	v, _ := json.Marshal([]any{in.Anchor.Issuer, in.Anchor.KID, in.Anchor.Key.N, in.Anchor.Synthetic, in.Policy.Issuer, in.Policy.TrustURI})
	h := sha256.Sum256(v)
	return base64.RawURLEncoding.EncodeToString(h[:])
}
func material(a *Artifact, s *State, k string) (string, string) {
	if a == nil {
		return "", "UNKNOWN"
	}
	if a.Availability == "UNAVAILABLE" {
		return s.cached(k), "UNKNOWN"
	}
	if a.Availability != "AVAILABLE" || a.JWS == "" {
		return "", "INVALID"
	}
	return a.JWS, "VALID"
}
func manifestOK(m Manifest, in Input) bool {
	if m.Issuer != in.Anchor.Issuer || m.Subject != in.Policy.TrustURI || m.Synthetic != in.Anchor.Synthetic || m.Profile != Profile || m.CredentialIssuer != in.Policy.Issuer || m.VCT != VCT || !times(m.Iat, m.Exp) || m.Iat > in.Policy.Now || m.Version < 1 || m.Version > safeMax || len(m.Keys) == 0 || len(m.Keys) > 32 || len(m.Authority) > 128 || !uri(m.Status.URI) || m.Status.Issuer == "" || m.Status.Purpose != "revocation" {
		return false
	}
	if _, e := KeyRSA(m.Status.JWK); e != nil {
		return false
	}
	seen := map[string]bool{}
	for _, k := range m.Keys {
		if _, e := KeyRSA(k.JWK); e != nil || seen[k.JWK.KID] || !times(k.From, k.Until) {
			return false
		}
		seen[k.JWK.KID] = true
	}
	for _, g := range m.Authority {
		if g.Employer == "" || g.Claim == "" || g.Policy == "" || !times(g.From, g.Until) || (g.Decision != "AUTHORIZED" && g.Decision != "UNAUTHORIZED" && g.Decision != "UNKNOWN") {
			return false
		}
	}
	return true
}
func authority(gs []Grant, employer, claim, policy string, at int64) string {
	result := "UNKNOWN"
	matches := 0
	for _, g := range gs {
		if g.Employer == employer && g.Claim == claim && g.Policy == policy && g.From <= at && at < g.Until {
			result = g.Decision
			matches++
		}
	}
	if matches > 1 {
		return "INVALID"
	}
	return result
}
func blank(in Input) Result {
	r, _ := b.VerifyCredential(b.Input{Trust: b.Trust{Synthetic: in.Anchor.Synthetic}, Policy: b.Policy{Now: in.Policy.Now, ExpectedSubject: in.Policy.ExpectedSubject}})
	r.Crypto = "UNKNOWN"
	r.Binding = "UNKNOWN"
	r.Temporal = "UNKNOWN"
	r.Overall = "INDETERMINATE"
	return Result{Contract: Contract, Material: "UNKNOWN", Key: "UNKNOWN", CurrentAuthority: "UNKNOWN", IssuanceAuthority: "UNKNOWN", Credential: r, Overall: "INDETERMINATE"}
}
func Verify(in Input, s *State) (Result, error) {
	r := blank(in)
	p := in.Policy
	if s == nil || !number(p.Now) || p.Now == 0 || !number(max(p.MaxStatusAge)) || !number(max(p.MaxTrustAge)) || !number(p.MinTrustVersion) || !number(p.MinStatusVersion) || !uri(p.TrustURI) || p.Issuer == "" || (p.AuthorityMode != "ISSUANCE_TIME" && p.AuthorityMode != "CURRENT") || in.Anchor.Issuer == "" || in.Anchor.KID != in.Anchor.Key.KID || (in.Anchor.Synthetic && !p.AllowSynthetic) {
		return r, errors.New("invalid trusted policy")
	}
	finish := func() (Result, error) {
		r.Credential.Eligible = r.Material == "VALID" && r.Key == "AUTHENTICATED" && r.Credential.Crypto == "VALID" && r.Credential.Binding == "VALID" && r.Credential.Temporal == "VALID" && r.Credential.Authority == "AUTHORIZED" && r.Credential.Status == "GOOD"
		if r.Credential.Eligible {
			r.Overall = "ELIGIBLE_WITHOUT_HOLDER"
		} else if r.Key == "INVALID" || r.Material == "INVALID" || r.Material == "ROLLED_BACK" || r.Credential.Crypto == "INVALID" || r.Credential.Binding == "INVALID" || r.Credential.Temporal == "INVALID" || r.Credential.Status == "INVALID" || r.Credential.Status == "REVOKED" || r.Credential.Authority == "UNAUTHORIZED" || r.Credential.Authority == "INVALID" {
			r.Overall = "REJECTED"
		}
		r.Credential.Overall = "INDETERMINATE"
		if r.Overall == "REJECTED" {
			r.Credential.Overall = "REJECTED"
		}
		return r, nil
	}
	sc := scope(in)
	raw, availability := material(in.Trust, s, sc+"/trust")
	if raw == "" {
		r.Material = availability
		return finish()
	}
	var m Manifest
	if jwtPayload(raw, in.Anchor.Key, "cvera-trust-material+jwt", &m) != nil || !manifestOK(m, in) {
		r.Material = "INVALID"
		return finish()
	}
	if !s.retain(sc+"/trust", raw, m.Version, m.Iat, p.MinTrustVersion) {
		r.Material = "ROLLED_BACK"
		return finish()
	}
	r.Material = "VALID"
	if m.Exp <= p.Now || p.Now-m.Iat > max(p.MaxTrustAge) {
		r.Material = "STALE"
	}
	// Read untrusted kid only to select a key from authenticated, scoped material.
	parts := strings.Split(strings.TrimSuffix(in.Credential, "~"), ".")
	var h struct {
		KID string `json:"kid"`
	}
	if len(parts) != 3 {
		r.Credential.Crypto = "INVALID"
		return finish()
	}
	hb, e := base64.RawURLEncoding.DecodeString(parts[0])
	if e != nil || json.Unmarshal(hb, &h) != nil {
		r.Credential.Crypto = "INVALID"
		return finish()
	}
	var selected *Key
	for _, k := range m.Keys {
		if k.JWK.KID == h.KID {
			kk := k
			selected = &kk
		}
	}
	if selected == nil {
		return finish()
	}
	pub, e := PEM(selected.JWK)
	if e != nil {
		r.Material = "INVALID"
		return finish()
	}
	r.Key = "AUTHENTICATED"
	br, e := b.VerifyCredential(b.Input{Credential: in.Credential, Trust: b.Trust{Issuer: m.CredentialIssuer, KID: h.KID, PublicKey: pub, Synthetic: in.Anchor.Synthetic}, Policy: b.Policy{Now: p.Now, ExpectedSubject: p.ExpectedSubject, MaxStatusAge: p.MaxStatusAge}})
	if e != nil {
		return r, e
	}
	r.Credential = br
	if br.Crypto != "VALID" || br.Binding != "VALID" || br.Temporal != "VALID" {
		return finish()
	}
	pb, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var c struct {
		Iat        int64  `json:"iat"`
		Claim      string `json:"claim_type"`
		Policy     string `json:"verification_policy"`
		Employment struct {
			Employer string `json:"employer_id"`
		} `json:"employment"`
		Status json.RawMessage `json:"status"`
	}
	if json.Unmarshal(pb, &c) != nil {
		r.Credential.Status = "INVALID"
		return finish()
	}
	if c.Iat < selected.From || c.Iat >= selected.Until {
		r.Key = "INVALID"
		return finish()
	}
	if r.Material != "VALID" {
		return finish()
	}
	r.IssuanceAuthority = authority(m.Authority, c.Employment.Employer, c.Claim, c.Policy, c.Iat)
	r.CurrentAuthority = authority(m.Authority, c.Employment.Employer, c.Claim, c.Policy, p.Now)
	r.Credential.Authority = r.CurrentAuthority
	if p.AuthorityMode == "ISSUANCE_TIME" {
		r.Credential.Authority = r.IssuanceAuthority
	}
	if len(c.Status) == 0 || string(c.Status) == "null" {
		return finish()
	}
	var mechanism struct {
		List json.RawMessage `json:"status_list"`
	}
	if json.Unmarshal(c.Status, &mechanism) != nil {
		r.Credential.Status = "INVALID"
		return finish()
	}
	if len(mechanism.List) == 0 || string(mechanism.List) == "null" {
		return finish()
	}
	var ref struct {
		Index *int64 `json:"idx"`
		URI   string `json:"uri"`
	}
	if json.Unmarshal(mechanism.List, &ref) != nil {
		r.Credential.Status = "INVALID"
		return finish()
	}

	if ref.Index == nil || !number(*ref.Index) || ref.URI != m.Status.URI {
		r.Credential.Status = "INVALID"
		return finish()
	}
	stRaw, a := material(in.Status, s, sc+"/status/"+m.Status.URI)
	if stRaw == "" {
		r.Credential.Status = a
		return finish()
	}
	var st StatusToken
	if jwtPayload(stRaw, m.Status.JWK, "statuslist+jwt", &st) != nil || st.Issuer != m.Status.Issuer || st.Subject != ref.URI || st.Synthetic != in.Anchor.Synthetic || st.VCT != m.VCT || st.CredentialIssuer != m.CredentialIssuer || !times(st.Iat, st.Exp) || st.Iat > p.Now || st.TTL <= 0 || !number(st.TTL) || st.Version < 1 || !number(st.Version) || st.List.Bits != 1 {
		r.Credential.Status = "INVALID"
		return finish()
	}
	compressed, e := decode64(st.List.Lst)
	if e != nil || len(compressed) > MaxBytes {
		r.Credential.Status = "INVALID"
		return finish()
	}
	compressedReader := bytes.NewReader(compressed)
	zr, e := zlib.NewReader(compressedReader)
	if e != nil {
		r.Credential.Status = "INVALID"
		return finish()
	}
	list, e := io.ReadAll(io.LimitReader(zr, MaxBytes+1))
	zr.Close()
	if e != nil || compressedReader.Len() != 0 || len(list) < 16384 || len(list) > MaxBytes {
		r.Credential.Status = "INVALID"
		return finish()
	}
	if !s.retain(sc+"/status/"+m.Status.URI, stRaw, st.Version, st.Iat, p.MinStatusVersion) {
		r.Credential.Status = "INVALID"
		return finish()
	}
	if *ref.Index >= int64(len(list))*8 {
		r.Credential.Status = "UNKNOWN"
		return finish()
	}
	if st.Exp <= p.Now || p.Now-st.Iat > max(p.MaxStatusAge) || p.Now-st.Iat > st.TTL {
		r.Credential.Status = "STALE"
		return finish()
	}
	r.Credential.Status = "GOOD"
	if list[*ref.Index/8]&(1<<uint(*ref.Index%8)) != 0 {
		r.Credential.Status = "REVOKED"
	}
	return finish()
}
