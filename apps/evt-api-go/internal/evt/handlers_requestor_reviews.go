package evt

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/auth"
)

// ReviewOutcomes discloses only bound decisions belonging to the active requestor.
// The database function returns no reviewer identities, grants, files or credentials.
func (h *CandidateHandlers) ReviewOutcomes(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	claims := auth.MustClaims(c)
	if claims == nil {
		return
	}
	if !auth.HasRole(claims, auth.RoleRequestor) {
		c.JSON(403, gin.H{"error": "forbidden"})
		return
	}
	request, err := uuid.Parse(c.Param("request_id"))
	if err != nil {
		c.JSON(404, gin.H{"error": "not_found"})
		return
	}
	if _, err = uuid.Parse(claims.Sub); err != nil {
		c.JSON(404, gin.H{"error": "not_found"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	tx, err := h.DB.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		c.JSON(503, gin.H{"error": "review_outcomes_unavailable"})
		return
	}
	defer tx.Rollback(ctx)
	var data json.RawMessage
	err = tx.QueryRow(ctx, `SELECT requestor_review_outcomes($1,$2)`, claims.Sub, request.String()).Scan(&data)
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		var state *pgconn.PgError
		if errors.As(err, &state) && state.Code == "P0002" {
			c.JSON(404, gin.H{"error": "not_found"})
		} else {
			c.JSON(503, gin.H{"error": "review_outcomes_unavailable"})
		}
		return
	}
	c.Data(200, "application/json", data)
}
