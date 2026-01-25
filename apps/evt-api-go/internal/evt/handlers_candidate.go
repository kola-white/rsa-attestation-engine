package evt

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/auth"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/db"
)

type CandidateHandlers struct {
	DB   *db.DB
	Repo *Repo
}

type CreateDraftReq struct {
	EmployerID    string          `json:"employer_id" binding:"required"`
	ClaimSnapshot json.RawMessage `json:"claim_snapshot"`
}

type RequestorListRow struct {
	RequestID     string          `json:"request_id"`
	Status        string          `json:"status"`
	ClaimSnapshot json.RawMessage `json:"claim_snapshot"`
	CreatedAt     string          `json:"created_at"`
	UpdatedAt     string          `json:"updated_at"`
}

type RequestorListResp struct {
	Items []RequestorListRow `json:"items"`
}

func (h *CandidateHandlers) List(c *gin.Context) {
	claims := auth.MustClaims(c)
	if claims == nil {
		return
	}
	if !auth.HasRole(claims, auth.RoleRequestor) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	personID := claims.Sub

	var items []RequestorListRow
	err := h.DB.WithTx(c.Request.Context(), func(tx pgx.Tx) error {
		rows, err := h.Repo.CandidateList(c.Request.Context(), tx, personID, 100)
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

	c.JSON(http.StatusOK, RequestorListResp{Items: items})
}


type CreateDraftResp struct {
	RequestID string `json:"request_id"`
	Status    string `json:"status"`
}

func (h *CandidateHandlers) CreateDraft(c *gin.Context) {
	claims := auth.MustClaims(c)
	if claims == nil {
		return
	}
	if !auth.HasRole(claims, auth.RoleRequestor) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	var req CreateDraftReq
	if err := c.ShouldBindJSON(&req); err != nil || req.EmployerID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body"})
		return
	}

	personID := claims.Sub // ✅ sub = domain_users.id (UUID string)

	var requestID string
	err := h.DB.WithTx(c.Request.Context(), func(tx pgx.Tx) error {
		id, err := h.Repo.CandidateCreateDraft(c.Request.Context(), tx, personID, req.EmployerID, req.ClaimSnapshot)
		if err != nil {
			return err
		}
		requestID = id
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "create_failed"})
		return
	}

	c.JSON(http.StatusCreated, CreateDraftResp{RequestID: requestID, Status: "DRAFT"})
}

type PatchDraftReq struct {
	ExpectedVersion int             `json:"expected_version" binding:"required"`
	ClaimSnapshot   json.RawMessage `json:"claim_snapshot" binding:"required"`
}

func (h *CandidateHandlers) PatchDraft(c *gin.Context) {
	claims := auth.MustClaims(c)
	if claims == nil {
		return
	}
	if !auth.HasRole(claims, auth.RoleRequestor) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	requestID := c.Param("request_id")

	var req PatchDraftReq
	if err := c.ShouldBindJSON(&req); err != nil || req.ExpectedVersion < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body"})
		return
	}

	personID := claims.Sub

	err := h.DB.WithTx(c.Request.Context(), func(tx pgx.Tx) error {
		return h.Repo.CandidateUpdateDraft(c.Request.Context(), tx, requestID, personID, req.ExpectedVersion, req.ClaimSnapshot)
	})
	if err != nil {
		if err == db.ErrConflict {
			c.JSON(http.StatusConflict, gin.H{"error": "conflict_or_invalid_state"})
			return
		}
		if err == db.ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "update_failed"})
		return
	}

	c.Status(http.StatusNoContent)
}

type SubmitResp struct {
	Status string `json:"status"`
}

func (h *CandidateHandlers) Submit(c *gin.Context) {
	claims := auth.MustClaims(c)
	if claims == nil {
		return
	}
	if !auth.HasRole(claims, auth.RoleRequestor) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	requestID := c.Param("request_id")
	personID := claims.Sub

	var status string
	err := h.DB.WithTx(c.Request.Context(), func(tx pgx.Tx) error {
		s, err := h.Repo.CandidateSubmit(c.Request.Context(), tx, requestID, personID)
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "submit_failed"})
		return
	}

	c.JSON(http.StatusOK, SubmitResp{Status: status})
}

func (h *CandidateHandlers) Cancel(c *gin.Context) {
	claims := auth.MustClaims(c)
	if claims == nil {
		return
	}
	if !auth.HasRole(claims, auth.RoleRequestor) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	requestID := c.Param("request_id")
	personID := claims.Sub

	err := h.DB.WithTx(c.Request.Context(), func(tx pgx.Tx) error {
		return h.Repo.CandidateCancel(c.Request.Context(), tx, requestID, personID)
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "cancel_failed"})
		return
	}

	c.Status(http.StatusNoContent)
}
