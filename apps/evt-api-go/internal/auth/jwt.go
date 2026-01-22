package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

func MintAccessTokenHS256(secret, iss, aud, sub, email string, roles []Role, ttl time.Duration) (string, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return "", errors.New("jwt secret is empty")
	}
	if len(roles) == 0 {
		return "", errors.New("jwt roles is empty")
	}

	now := time.Now().Unix()
	claims := AccessClaims{
		Sub:   sub,
		Email: email,
		Roles: roles,
		Typ:   "access",
		Iat:   now,
		Exp:   now + int64(ttl.Seconds()),
		Iss:   iss,
		Aud:   aud,
	}

	header := map[string]any{
		"alg": "HS256",
		"typ": "JWT",
	}

	hb, _ := json.Marshal(header)
	cb, _ := json.Marshal(claims)

	enc := base64.RawURLEncoding
	h := enc.EncodeToString(hb)
	c := enc.EncodeToString(cb)

	signingInput := h + "." + c
	sig := hmacSHA256([]byte(secret), []byte(signingInput))
	s := enc.EncodeToString(sig)

	return signingInput + "." + s, nil
}

func hmacSHA256(key, msg []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(msg)
	return mac.Sum(nil)
}
