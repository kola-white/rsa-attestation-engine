package auth

import (
	"errors"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrInvalidToken = errors.New("invalid_token")
)

// VerifyAccessTokenHS256 verifies the HS256 access token minted by MintAccessTokenHS256.
// sub = domain_users.id (UUID string)
func VerifyAccessTokenHS256(tokenStr string, secret string, iss string, aud string) (*AccessClaims, error) {
	tokenStr = strings.TrimSpace(tokenStr)
	if tokenStr == "" {
		return nil, ErrInvalidToken
	}
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return nil, ErrInvalidToken
	}
	iss = strings.TrimSpace(iss)
	aud = strings.TrimSpace(aud)
	if iss == "" || aud == "" {
		return nil, ErrInvalidToken
	}

	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
	)

	var claims AccessClaims
	tok, err := parser.ParseWithClaims(tokenStr, &claims, func(token *jwt.Token) (any, error) {
		return []byte(secret), nil
	})
	if err != nil || tok == nil || !tok.Valid {
		return nil, ErrInvalidToken
	}

	// --- Manual validation (we own the schema) ---
	now := time.Now().Unix()
	const leeway = int64(30)

	if claims.Sub == "" {
		return nil, ErrInvalidToken
	}
	if claims.Typ != "access" {
		return nil, ErrInvalidToken
	}
	if claims.Iss != iss {
		return nil, ErrInvalidToken
	}
	if claims.Aud != aud {
		return nil, ErrInvalidToken
	}
	// Exp must be in the future (with leeway)
	if claims.Exp == 0 || (claims.Exp+leeway) < now {
		return nil, ErrInvalidToken
	}

	return &claims, nil
}
