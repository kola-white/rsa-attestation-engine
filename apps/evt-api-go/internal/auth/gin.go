package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// Context key used to store claims in gin.Context.
const ginClaimsKey = "cvera_claims"

// MustClaims returns claims from gin.Context or writes 401 and aborts.
// This is what your EVT handlers call: claims := auth.MustClaims(c)
func MustClaims(c *gin.Context) *AccessClaims {
	v, ok := c.Get(ginClaimsKey)
	if !ok || v == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing_claims"})
		c.Abort()
		return nil
	}
	claims, ok := v.(*AccessClaims)
	if !ok || claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_claims"})
		c.Abort()
		return nil
	}
	return claims
}

// GinRequireAuth validates the JWT and stores claims on gin.Context.
// Call it once as a global middleware: phase2.Use(auth.GinRequireAuth(...))
func GinRequireAuth(jwtSecret, issuer, audience string) gin.HandlerFunc {
	secret := []byte(jwtSecret)

	return func(c *gin.Context) {
		// Require Authorization: Bearer <token>
		h := strings.TrimSpace(c.GetHeader("Authorization"))
		if h == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing_authorization"})
			c.Abort()
			return
		}

		parts := strings.SplitN(h, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_authorization"})
			c.Abort()
			return
		}
		raw := strings.TrimSpace(parts[1])
		if raw == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing_token"})
			c.Abort()
			return
		}

		claims := &AccessClaims{}
		token, err := jwt.ParseWithClaims(
			raw,
			claims,
			func(t *jwt.Token) (any, error) {
				// Strict: only HS256
				if t.Method == nil || t.Method.Alg() != jwt.SigningMethodHS256.Alg() {
					return nil, jwt.ErrSignatureInvalid
				}
				return secret, nil
			},
			jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
			jwt.WithIssuer(issuer),
			jwt.WithAudience(audience),
		)

		if err != nil || token == nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_access_token"})
			c.Abort()
			return
		}

		// Optional: enforce typ=access if you mint it
		if claims.Typ != "" && claims.Typ != "access" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "wrong_token_type"})
			c.Abort()
			return
		}

		// Basic hardening
		if strings.TrimSpace(claims.Sub) == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing_sub"})
			c.Abort()
			return
		}

		// ✅ Store where MustClaims reads
		c.Set(ginClaimsKey, claims)

		// ✅ ALSO store on request context (helps any code that reads from ctx)
		// This does NOT change MustClaims behavior; it's additive compatibility.
		c.Request = c.Request.WithContext(WithClaims(c.Request.Context(), claims))

		c.Next()
	}
}

// GinRequireRole aborts with 403 if the authenticated user lacks role.
// Use per-route: phase2.POST(..., auth.GinRequireRole(auth.RoleRequestor), handler)
func GinRequireRole(role Role) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims := MustClaims(c)
		if claims == nil {
			return
		}
		if !HasRole(claims, role) {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			c.Abort()
			return
		}
		c.Next()
	}
}
