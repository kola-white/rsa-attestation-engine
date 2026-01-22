package evt

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/auth"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/db"
)

type RecruiterHandlers struct {
	DB   *db.DB
	Repo *Repo
}

type ConsumeReq struct {
	// Optional metadata about the consuming system/session
	ConsumerRef string          `json:"consumer_ref,omitempty"`
	Context     json.RawMessage `json:"context,omitempty"`
}

type ConsumeResp struct {
	Status string `json:"status"`
}

func (h *RecruiterHandlers) Consume(c *gin.Context) {
	claims := auth.MustClaims(c)
	if claims == nil {
		return
	}
	if !auth.HasRole(claims, auth.RoleRecruiter) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	requestID := c.Param("request_id")
	recruiterPersonID := claims.Sub

	var req ConsumeReq
	_ = c.ShouldBindJSON(&req) // allow empty body

	var status string
	err := h.DB.WithTx(c.Request.Context(), func(tx pgx.Tx) error {
		// Repo expects context as string (per compiler), so serialize the raw JSON bytes.
		ctxStr := ""
		if len(req.Context) > 0 {
			ctxStr = string(req.Context)
		}

		if err := h.Repo.RecruiterConsume(
			c.Request.Context(),
			tx,
			requestID,
			recruiterPersonID,
			req.ConsumerRef,
			ctxStr,
		); err != nil {
			return err
		}

		status = "CONSUMED"
		return nil
	})

	if err != nil {
		if err == db.ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		if err == db.ErrConflict {
			c.JSON(http.StatusConflict, gin.H{"error": "conflict_or_invalid_state"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "consume_failed"})
		return
	}

	c.JSON(http.StatusOK, ConsumeResp{Status: status})
}
