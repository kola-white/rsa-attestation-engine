package httpapi

import (
	"net/http"
	"strings"

	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/auth"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/config"
)

func RequireAuth(authCfg config.AuthConfig, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Authorization")
		if h == "" {
			http.Error(w, "missing Authorization", http.StatusUnauthorized)
			return
		}
		parts := strings.SplitN(h, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			http.Error(w, "invalid Authorization", http.StatusUnauthorized)
			return
		}

		claims, err := auth.VerifyAccessTokenHS256(parts[1], authCfg.JWTSecret, authCfg.Issuer, authCfg.Audience)
		if err != nil {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}

		ctx := auth.WithClaims(r.Context(), claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireRole is typed: callers must pass auth.Role constants.
// This is what stops the “string drift” problem.
func RequireRole(role auth.Role, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := auth.ClaimsFromContext(r.Context())
		if !ok || claims == nil {
			http.Error(w, "missing claims", http.StatusUnauthorized)
			return
		}
		if !auth.HasRole(claims, role) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
