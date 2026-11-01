package auth

import (
	"github.com/golang-jwt/jwt/v5"
	"testing"
	"time"
)

func TestAccessTokenValidation(t *testing.T) {
	const secret = "temporary-test-secret-used-only-in-unit-tests"
	valid := jwt.MapClaims{"sub": "test-user", "typ": "access", "iss": "test-issuer", "aud": "test-audience", "exp": time.Now().Add(time.Minute).Unix()}
	for _, tc := range []struct {
		name, field string
		value       any
		method      jwt.SigningMethod
		reject      bool
	}{
		{name: "valid", method: jwt.SigningMethodHS256},
		{name: "wrong issuer", field: "iss", value: "other", method: jwt.SigningMethodHS256, reject: true},
		{name: "wrong audience", field: "aud", value: "other", method: jwt.SigningMethodHS256, reject: true},
		{name: "wrong type", field: "typ", value: "refresh", method: jwt.SigningMethodHS256, reject: true},
		{name: "expired", field: "exp", value: time.Now().Add(-time.Minute).Unix(), method: jwt.SigningMethodHS256, reject: true},
		{name: "missing expiry", field: "exp", value: nil, method: jwt.SigningMethodHS256, reject: true},
		{name: "missing subject", field: "sub", value: nil, method: jwt.SigningMethodHS256, reject: true},
		{name: "wrong algorithm", method: jwt.SigningMethodHS384, reject: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			claims := jwt.MapClaims{}
			for k, v := range valid {
				claims[k] = v
			}
			if tc.field != "" {
				if tc.value == nil {
					delete(claims, tc.field)
				} else {
					claims[tc.field] = tc.value
				}
			}
			raw, err := jwt.NewWithClaims(tc.method, claims).SignedString([]byte(secret))
			if err != nil {
				t.Fatal(err)
			}
			_, err = VerifyAccessTokenHS256(raw, secret, "test-issuer", "test-audience")
			if (err != nil) != tc.reject {
				t.Fatalf("unexpected result: %v", err)
			}
		})
	}
	raw, err := MintAccessTokenHS256(secret, "test-issuer", "test-audience", "test-user", "", []Role{RoleHRReviewer}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyAccessTokenHS256(raw, "wrong-secret", "test-issuer", "test-audience"); err == nil {
		t.Fatal("accepted wrong signature")
	}
}
