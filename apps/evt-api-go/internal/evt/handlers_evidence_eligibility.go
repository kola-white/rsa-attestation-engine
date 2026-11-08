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
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/requestbinding"
)

// EvidenceEligibility prepares a manifest; it never calls Signer/Attest/Verify.
// The trusted authority source remains deliberately unconfigured in production:
// no new authority schema privileges, fabricated grants or positive default.
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
		// Same explicitly trusted application path used by existing evidence routes.
		if _, err := tx.Exec(ctx, `SELECT pg_catalog.set_config('search_path',pg_catalog.format('pg_catalog,%I,pg_temp',pg_catalog.current_schema()),true)`); err != nil {
			return err
		}
		var err error
		result, err = requestbinding.Prepare(ctx, tx, claims.Sub, request.String(), input.EmployerID, nil)
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
