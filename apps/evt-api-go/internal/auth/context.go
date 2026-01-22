package auth

import (
	"context"
	"errors"
)

type ctxKey int

const claimsKey ctxKey = iota

var ErrNoClaims = errors.New("auth: no claims in context")

// WithClaims stores AccessClaims in a stdlib context.
func WithClaims(ctx context.Context, c *AccessClaims) context.Context {
	return context.WithValue(ctx, claimsKey, c)
}

// ClaimsFromContext retrieves AccessClaims from a stdlib context.
func ClaimsFromContext(ctx context.Context) (*AccessClaims, bool) {
	v := ctx.Value(claimsKey)
	if v == nil {
		return nil, false
	}
	c, ok := v.(*AccessClaims)
	return c, ok
}
