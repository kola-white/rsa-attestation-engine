package evt

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/auth"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/db"
)

type EmployerHandlers struct {
	DB   *db.DB
	Repo *Repo
}

type AttestReq struct {
	EmployerID     string          `json:"employer_id" binding:"required"`
	ResponseType   string          `json:"response_type" binding:"required"` // e.g. "approve" | "reject"
	ResponseBody   json.RawMessage `json:"response_body,omitempty"`
	AttestationJWS string          `json:"attestation_jws" binding:"required"`
}

type AttestResp struct {
	Status string `json:"status"`
}

func (h *EmployerHandlers) Attest(c *gin.Context) {
	claims := auth.MustClaims(c)
	if claims == nil {
		return
	}
	if !auth.HasRole(claims, auth.RoleHRReviewer) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	requestID := c.Param("request_id")
	hrPersonID := claims.Sub

	var req AttestReq
	if err := c.ShouldBindJSON(&req); err != nil ||
		strings.TrimSpace(req.EmployerID) == "" ||
		strings.TrimSpace(req.ResponseType) == "" ||
		strings.TrimSpace(req.AttestationJWS) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body"})
		return
	}

	var status string
	err := h.DB.WithTx(c.Request.Context(), func(tx pgx.Tx) error {
		s, err := h.Repo.HREmployerAttest(
			c.Request.Context(),
			tx,
			requestID,
			req.EmployerID,
			hrPersonID,
			req.ResponseType,
			req.ResponseBody,
			req.AttestationJWS,
		)
		if err != nil {
			return err
		}
		status = s
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "attest_failed"})
		return
	}

	c.JSON(http.StatusOK, AttestResp{Status: status})
}
