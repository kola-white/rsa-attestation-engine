package evt

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/auth"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/db"
	ec "github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/evidencecredential"
)

// IssueEvidence consumes no client eligibility/authority/manifest assertion.
// An absent migration, inactive/unconfigured credential or missing authority
// fails closed before signer invocation. Exact context comes from the database.
func (h *EmployerHandlers) IssueEvidence(c *gin.Context) {
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
		EmployerID string `json:"employer_id"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || input.EmployerID == "" || decoder.Decode(&struct{}{}) != io.EOF {
		c.JSON(400, gin.H{"error": "invalid_body"})
		return
	}
	if h.Repo == nil {
		c.JSON(503, gin.H{"error": "evidence_issuance_unavailable"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	engine := db.IssuanceEngine{DB: h.DB, Signer: h.Repo.Signer}
	result, err := engine.Issue(ctx, claims.Sub, request.String(), input.EmployerID)
	if err != nil {
		if errors.Is(err, ec.ErrUnavailable) {
			c.JSON(503, gin.H{"error": "evidence_issuance_unavailable"})
			return
		}
		var state *pgconn.PgError
		if errors.As(err, &state) {
			if state.Code == "P0002" {
				c.JSON(404, gin.H{"error": "not_found"})
				return
			}
			if state.Code == "P0001" || state.Code == "23505" {
				c.JSON(409, gin.H{"error": "evidence_issuance_ineligible"})
				return
			}
		}
		if errors.Is(err, ec.ErrIneligible) {
			c.JSON(409, gin.H{"error": "evidence_issuance_ineligible"})
			return
		}
		c.JSON(503, gin.H{"error": "evidence_issuance_unavailable"})
		return
	}
	c.JSON(200, result)
}
