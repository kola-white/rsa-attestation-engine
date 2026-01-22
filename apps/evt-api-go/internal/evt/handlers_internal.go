package evt

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/auth"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/db"
)

type InternalHandlers struct {
	DB   *db.DB
	Repo *Repo
}

type VerifyReq struct {
	TrustResult  string          `json:"trust_result" binding:"required"` // "VERIFIED" | "UNVERIFIED" (or whatever your enum is)
	TrustFlags   json.RawMessage `json:"trust_flags,omitempty"`           // json array; repo defaults to [] if empty
	TrustSummary string          `json:"trust_summary,omitempty"`         // free text summary
	EVTTokenJWS  string          `json:"evt_token_jws,omitempty"`         // optional per repo (COALESCE)
}

type VerifyResp struct {
	Status string `json:"status"`
}

func (h *InternalHandlers) Verify(c *gin.Context) {
	claims := auth.MustClaims(c)
	if claims == nil {
		return
	}
	if !auth.HasRole(claims, auth.RoleCvera) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	requestID := c.Param("request_id")

	var req VerifyReq
	_ = c.ShouldBindJSON(&req) // allow empty body

	var status string
	err := h.DB.WithTx(c.Request.Context(), func(tx pgx.Tx) error {
		if err := h.Repo.InternalVerify(
			c.Request.Context(),
			tx,
			requestID,
			req.TrustResult,
			req.TrustFlags,
			req.TrustSummary,
			req.EVTTokenJWS,
		); err != nil {
			return err
		}

		// status mirrors what repo will set based on trust_result
		status = "VERIFIED"
		if req.TrustResult == "UNVERIFIED" {
			status = "UNVERIFIED"
		}
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "verify_failed"})
		return
	}

	c.JSON(http.StatusOK, VerifyResp{Status: status})
}

type CloseReq struct {
	Reason string `json:"reason,omitempty"`
}

type CloseResp struct {
	Status string `json:"status"`
}

func (h *InternalHandlers) Close(c *gin.Context) {
	claims := auth.MustClaims(c)
	if claims == nil {
		return
	}
	if !auth.HasRole(claims, auth.RoleCvera) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	requestID := c.Param("request_id")

	var req CloseReq
	_ = c.ShouldBindJSON(&req) // allow empty body

	var status string
	err := h.DB.WithTx(c.Request.Context(), func(tx pgx.Tx) error {
		if err := h.Repo.InternalClose(c.Request.Context(), tx, requestID); err != nil {
			return err
		}
		status = "CLOSED"
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "close_failed"})
		return
	}

	c.JSON(http.StatusOK, CloseResp{Status: status})
}
