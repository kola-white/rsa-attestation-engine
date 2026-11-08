package evt

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/auth"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/db"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/requestbinding"
)

// EvidenceEligibility prepares a manifest; it never calls Signer/Attest/Verify.
// A missing authority store remains explicitly unconfigured. Bounded trusted
// resolution is used when deployed; no client or fabricated authority is accepted.
func (h *EmployerHandlers) EvidenceEligibility(c *gin.Context) {
	claims := auth.MustClaims(c)
	if claims == nil {
		return
	}
	c.Header("Cache-Control", "private, no-store")
	if !auth.HasRole(claims, auth.RoleHRReviewer) {
		c.JSON(403, gin.H{"error": "forbidden"})
		return
	}
	request, err := uuid.Parse(c.Param("request_id"))
	if err != nil {
		c.JSON(404, gin.H{"error": "not_found"})
		return
	}
	var input struct {
		EmployerID string `json:"employer_id" binding:"required"`
	}
	if c.ShouldBindJSON(&input) != nil {
		c.JSON(400, gin.H{"error": "invalid_body"})
		return
	}
	if h.DB == nil || h.DB.Pool == nil {
		c.JSON(503, gin.H{"error": "evidence_eligibility_unavailable"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	var result requestbinding.Result
	err = h.DB.WithTx(ctx, func(tx pgx.Tx) error {

		source, err := db.OptionalAuthoritySource(ctx, tx)
		if err != nil {
			return err
		}
		result, err = requestbinding.Prepare(ctx, tx, claims.Sub, request.String(), input.EmployerID, source)
		if err != nil {
			return err
		}
		if result.Eligible && h.Repo != nil && h.Repo.Signer != nil {
			result.SigningEnabled, err = db.ActiveEvidenceSigningCredential(ctx, tx, h.Repo.Signer)
		}
		return err
	})
	if err != nil {
		var state *pgconn.PgError
		if errors.As(err, &state) && state.Code == "P0002" {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "evidence_eligibility_unavailable"})
		return
	}
	c.JSON(http.StatusOK, result)
}
