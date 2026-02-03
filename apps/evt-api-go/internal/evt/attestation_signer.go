package evt

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type AttestationSigner struct {
	privateKey *rsa.PrivateKey
	kid        string // must match trust/jwks.json entry kid
}

func NewAttestationSignerFromPEMPath(pemPath string) (*AttestationSigner, error) {
	b, err := os.ReadFile(pemPath)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(b)
	if block == nil {
		return nil, errors.New("invalid_pem")
	}

	// PKCS1 or PKCS8 support
	var pk any
	if strings.Contains(block.Type, "PRIVATE KEY") {
		// Try PKCS8 first
		if k, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
			pk = k
		} else if k, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
			pk = k
		} else {
			return nil, errors.New("unsupported_private_key")
		}
	} else {
		return nil, errors.New("unsupported_pem_type")
	}

	rsaKey, ok := pk.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("not_rsa_private_key")
	}

	kid := kidFromPath(pemPath) // EXACT parity with make-jwks.mjs
	return &AttestationSigner{privateKey: rsaKey, kid: kid}, nil
}

func kidFromPath(p string) string {
	base := filepath.Base(p)
	// Node script strips .pem (case-insensitive)
	if strings.HasSuffix(strings.ToLower(base), ".pem") {
		base = base[:len(base)-4]
	}
	return base
}

// Claims payload you sign and store inside the JWS
type AttestationClaims struct {
	RequestID    string
	EmployerID   string
	HRPersonID   string
	ResponseType string
	ResponseBody []byte
	IssuedAtUnix int64
}

// SignAttestationJWS returns (compactJWS, kid, error)
func (s *AttestationSigner) SignAttestationJWS(cl AttestationClaims) (string, string, error) {
	if s == nil || s.privateKey == nil || s.kid == "" {
		return "", "", errSignerNotConfigured()
	}

	// No auto-fields (iat/exp/etc). We sign exactly what the repo constructs in AttestationClaims.
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{})

	// MUST set protected header kid = s.kid
	// MUST use RS256 (SigningMethodRS256 does this)
	tok.Header["kid"] = s.kid
	tok.Header["typ"] = "JWT"

	// Convert the claims struct into a JSON object map to become the JWT payload.
	// This keeps your attestation schema fields as-is (no extra jwt.* fields).
	b, err := json.Marshal(cl)
	if err != nil {
		return "", s.kid, fmt.Errorf("marshal attestation claims: %w", err)
	}

	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return "", s.kid, fmt.Errorf("unmarshal attestation claims: %w", err)
	}
	tok.Claims = jwt.MapClaims(m)

	// Preserve your TODO’s “_ = time.Now()” intent without injecting anything.
	_ = time.Now()

	out, err := tok.SignedString(s.privateKey)
	if err != nil {
		return "", s.kid, fmt.Errorf("sign attestation jws: %w", err)
	}
	return out, s.kid, nil
}

func errSignerNotConfigured() error {
	return errors.New("attestation_signer_not_configured")
}
