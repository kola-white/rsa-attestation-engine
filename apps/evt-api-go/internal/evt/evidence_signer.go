package evt

import (
	"context"
	"crypto/rsa"
	"github.com/golang-jwt/jwt/v5"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/evidencecredential"
	"math/big"
)

func (s *AttestationSigner) KeyID() string {
	if s == nil {
		return ""
	}
	return s.kid
}
func (s *AttestationSigner) PublicKey() *rsa.PublicKey {
	if s == nil || s.privateKey == nil {
		return nil
	}
	return &rsa.PublicKey{N: new(big.Int).Set(s.privateKey.PublicKey.N), E: s.privateKey.PublicKey.E}
}
func (s *AttestationSigner) SignEvidenceClaims(ctx context.Context, claims evidencecredential.Claims) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if s == nil || s.privateKey == nil {
		return "", errSignerNotConfigured()
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = s.kid
	token.Header["typ"] = "dc+sd-jwt"
	raw, err := token.SignedString(s.privateKey)
	if err != nil {
		return "", err
	}
	return raw + "~", nil
}
