package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// AccessClaims is our Phase-1/2 access token claim shape.
// IMPORTANT: Keep it stable. Sub = domain_users.id (UUID string).
type AccessClaims struct {
	Sub   string `json:"sub"`
	Email string `json:"email,omitempty"`
	Roles []Role `json:"roles,omitempty"`

	Typ string `json:"typ"` // "access"
	Iat int64  `json:"iat"`
	Exp int64  `json:"exp"`
	Iss string `json:"iss"`
	Aud string `json:"aud"`
}

// ---- jwt.Claims interface (jwt/v5) ---------------------------------------
// We implement the getters so *AccessClaims can be used with ParseWithClaims.

func (c AccessClaims) GetAudience() (jwt.ClaimStrings, error) {
	if c.Aud == "" {
		return nil, errors.New("missing aud")
	}
	return jwt.ClaimStrings{c.Aud}, nil
}

func (c AccessClaims) GetIssuer() (string, error) {
	if c.Iss == "" {
		return "", errors.New("missing iss")
	}
	return c.Iss, nil
}

func (c AccessClaims) GetSubject() (string, error) {
	if c.Sub == "" {
		return "", errors.New("missing sub")
	}
	return c.Sub, nil
}

func (c AccessClaims) GetExpirationTime() (*jwt.NumericDate, error) {
	if c.Exp == 0 {
		return nil, errors.New("missing exp")
	}
	return jwt.NewNumericDate(time.Unix(c.Exp, 0)), nil
}

func (c AccessClaims) GetIssuedAt() (*jwt.NumericDate, error) {
	if c.Iat == 0 {
		// iat is optional; return nil without error
		return nil, nil
	}
	return jwt.NewNumericDate(time.Unix(c.Iat, 0)), nil
}

func (c AccessClaims) GetNotBefore() (*jwt.NumericDate, error) {
	// not used
	return nil, nil
}
// --------------------------------------------------------------------------