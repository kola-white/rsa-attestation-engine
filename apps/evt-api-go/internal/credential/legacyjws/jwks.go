package legacyjws

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"strings"
	"sync"
)

var ErrKeyNotFound = errors.New("legacy_jws_key_not_found")

type jwksDocument struct {
	Keys []jwk `json:"keys"`
}

type jwk struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Alg string `json:"alg"`
	Use string `json:"use"`

	N string `json:"n"`
	E string `json:"e"`
}

// StaticJWKSResolver is a read-only in-memory resolver for the legacy JWKS.
//
// It exists only for compatibility with the current legacy-jws profile.
// SD-JWT VC issuer-key resolution will use its separately frozen issuer
// metadata / x5c profile.
type StaticJWKSResolver struct {
	mu   sync.RWMutex
	keys map[string]*rsa.PublicKey
}

// NewStaticJWKSResolver parses a JWKS document and constructs an immutable
// resolver.
//
// Only RSA keys suitable for RS256 are imported.
func NewStaticJWKSResolver(jwksJSON []byte) (*StaticJWKSResolver, error) {
	var doc jwksDocument
	if err := json.Unmarshal(jwksJSON, &doc); err != nil {
		return nil, fmt.Errorf("parse legacy JWKS: %w", err)
	}

	keys := make(map[string]*rsa.PublicKey)

	for _, raw := range doc.Keys {
		if strings.TrimSpace(raw.Kid) == "" {
			continue
		}
		if raw.Kty != "RSA" {
			continue
		}
		if raw.Alg != "" && raw.Alg != "RS256" {
			continue
		}

		pub, err := rsaPublicKeyFromJWK(raw)
		if err != nil {
			return nil, fmt.Errorf("parse legacy JWK %q: %w", raw.Kid, err)
		}

		keys[raw.Kid] = pub
	}

	if len(keys) == 0 {
		return nil, errors.New("legacy JWKS contains no usable RSA verification keys")
	}

	return &StaticJWKSResolver{
		keys: keys,
	}, nil
}

// NewStaticJWKSResolverFromFile loads the current legacy trust/jwks.json
// representation from disk.
func NewStaticJWKSResolverFromFile(path string) (*StaticJWKSResolver, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read legacy JWKS: %w", err)
	}

	return NewStaticJWKSResolver(b)
}

func (r *StaticJWKSResolver) ResolveKey(
	_ context.Context,
	kid string,
) (any, error) {
	if r == nil {
		return nil, ErrKeyNotFound
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	key, ok := r.keys[kid]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrKeyNotFound, kid)
	}

	return key, nil
}

func rsaPublicKeyFromJWK(k jwk) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
	if err != nil {
		return nil, fmt.Errorf("decode n: %w", err)
	}

	eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
	if err != nil {
		return nil, fmt.Errorf("decode e: %w", err)
	}

	if len(nBytes) == 0 || len(eBytes) == 0 {
		return nil, errors.New("missing RSA modulus or exponent")
	}

	n := new(big.Int).SetBytes(nBytes)

	e := 0
	for _, b := range eBytes {
		e = e<<8 + int(b)
	}

	if e <= 0 {
		return nil, errors.New("invalid RSA exponent")
	}

	return &rsa.PublicKey{
		N: n,
		E: e,
	}, nil
}
