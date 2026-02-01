package evt

import (
	"encoding/json"
	"log"
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
}

type AttestResp struct {
	Status string `json:"status"`
	Attestation AttestationMeta  `json:"attestation"`
}

type AttestationMeta struct {
	Present bool   `json:"present"`
	KID     string `json:"kid,omitempty"`
}

type HRQueueRow struct {
	RequestID     string          `json:"request_id"`
	Status        string          `json:"status"`
	ClaimSnapshot json.RawMessage `json:"claim_snapshot"`
	CreatedAt     string          `json:"created_at"`
	UpdatedAt     string          `json:"updated_at"`
}

type HRQueueResp struct {
	Items []HRQueueRow `json:"items"`
}

type HRGetResp struct {
	RequestID     string          `json:"request_id"`
	Status        string          `json:"status"`
	ClaimSnapshot json.RawMessage `json:"claim_snapshot"`
	CreatedAt     string          `json:"created_at"`
	UpdatedAt     string          `json:"updated_at"`
	Version       int             `json:"version"`
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
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body"})
		return
	}

	req.EmployerID = strings.TrimSpace(req.EmployerID)
	req.ResponseType = strings.TrimSpace(req.ResponseType)

	if req.EmployerID == "" || req.ResponseType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body"})
		return
	}

	// response_body can be empty; normalize to {} for stable DB behavior
	if len(req.ResponseBody) == 0 {
		req.ResponseBody = json.RawMessage(`{}`)
	}

	var (
		status  string
		present bool
		kid     string
		jwsLen  int
	)

	err := h.DB.WithTx(c.Request.Context(), func(tx pgx.Tx) error {
		// IMPORTANT: Repo now generates JWS when needed and returns metadata
		out, err := h.Repo.EmployerAttestServerSigned(
			c.Request.Context(),
			tx,
			requestID,
			req.EmployerID,
			hrPersonID,
			req.ResponseType,
			req.ResponseBody,
		)
		if err != nil {
			return err
		}

		status = out.Status
		present = out.AttestationPresent
		kid = out.KID
		jwsLen = out.JWSBytes

		return nil
	})

	if err != nil {
		log.Printf("[attest_failed] request_id=%s employer_id=%s hr_personid=%s response_type=%s err=%T %v",
			requestID, req.EmployerID, hrPersonID, req.ResponseType, err, err,
		)

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

	// Mandatory success log (your requested shape)
	log.Printf("[attest] request_id=%s employer_id=%s hr_personid=%s generated_jws=%t kid=%s bytes=%d response_type=%s status=%s",
		requestID, req.EmployerID, hrPersonID, present, kid, jwsLen, req.ResponseType, status,
	)

	c.JSON(http.StatusOK, AttestResp{
		Status: status,
		Attestation: AttestationMeta{
			Present: present,
			KID:     kid,
		},
	})
}

func (h *EmployerHandlers) List(c *gin.Context) {
	claims := auth.MustClaims(c)
	if claims == nil {
		return
	}
	if !auth.HasRole(claims, auth.RoleHRReviewer) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	employerID := strings.TrimSpace(c.Query("employer_id"))
	if employerID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing_employer_id"})
		return
	}

	var items []HRQueueRow
	err := h.DB.WithTx(c.Request.Context(), func(tx pgx.Tx) error {
		rows, err := h.Repo.EmployerList(c.Request.Context(), tx, employerID, 100)
		if err != nil {
			return err
		}
		items = rows
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list_failed"})
		return
	}

	c.JSON(http.StatusOK, HRQueueResp{Items: items})
}

func (h *EmployerHandlers) Get(c *gin.Context) {
	claims := auth.MustClaims(c)
	if claims == nil {
		return
	}
	if !auth.HasRole(claims, auth.RoleHRReviewer) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	employerID := strings.TrimSpace(c.Query("employer_id"))
	if employerID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing_employer_id"})
		return
	}

	requestID := c.Param("request_id")

	var out HRGetResp
	err := h.DB.WithTx(c.Request.Context(), func(tx pgx.Tx) error {
		row, err := h.Repo.EmployerGet(c.Request.Context(), tx, requestID, employerID)
		if err != nil {
			return err
		}

		out = HRGetResp{
			RequestID:     row.RequestID,
			Status:        row.Status,
			ClaimSnapshot: row.ClaimSnapshot,
			CreatedAt:     row.CreatedAt,
			UpdatedAt:     row.UpdatedAt,
			Version:       row.Version,
		}
		return nil
	})

	if err != nil {
		if err == db.ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "get_failed"})
		return
	}

	c.JSON(http.StatusOK, out)
}

