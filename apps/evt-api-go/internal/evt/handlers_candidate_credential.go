package evt

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/auth"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/db"
)

// Credential returns the subject's own committed portable artifact, not a trust
// verdict. Revocation/authority/evidence must still be checked by the verifier.
func (h *CandidateHandlers) Credential(c *gin.Context) {
	claims := auth.MustClaims(c)
	if claims == nil {
		return
	}
	c.Header("Cache-Control", "private, no-store")
	if !auth.HasRole(claims, auth.RoleRequestor) {
		c.JSON(403, gin.H{"error": "forbidden"})
		return
	}
	request, err := uuid.Parse(c.Param("request_id"))
	if err != nil {
		c.JSON(404, gin.H{"error": "not_found"})
		return
	}
	if h.DB == nil || h.DB.Pool == nil {
		c.JSON(503, gin.H{"error": "credential_unavailable"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	engine := db.IssuanceEngine{DB: h.DB}
	artifact, err := engine.SubjectCredential(ctx, claims.Sub, request.String())
	if err != nil {
		if err == pgx.ErrNoRows {
			c.JSON(404, gin.H{"error": "not_found"})
			return
		}
		c.JSON(503, gin.H{"error": "credential_unavailable"})
		return
	}
	c.JSON(200, gin.H{"credential": artifact, "mediaType": "application/dc+sd-jwt"})
}
