package evt

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
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
	// TODO: implement using the JWS/JWT library you standardize on in this repo.
	// MUST set protected header kid = s.kid
	// MUST use RS256
	_ = time.Now()

	return "", s.kid, errors.New("not_implemented")
}

func errSignerNotConfigured() error {
	return errors.New("attestation_signer_not_configured")
}
