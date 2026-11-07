package httpapi

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/auth"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/storage"
)

type EvidenceInitRequest struct {
	CaseID  string               `json:"caseId"`
	CheckID string               `json:"checkId"`
	Files   []EvidenceInitFileIn `json:"files"`
}

type EvidenceInitFileIn struct {
	Name     string `json:"name"`
	MimeType string `json:"mimeType"`
	Size     int64  `json:"size"` // bytes
}

type EvidenceInitResponse struct {
	CaseID  string                `json:"caseId"`
	CheckID string                `json:"checkId"`
	Expires string                `json:"expiresAt"`
	Uploads []EvidenceInitFileOut `json:"uploads"`
}

type EvidenceInitFileOut struct {
	Name       string            `json:"name"`
	MimeType   string            `json:"mimeType"`
	Size       int64             `json:"size"`
	StorageKey string            `json:"storageKey"`
	Method     string            `json:"method"` // PUT
	URL        string            `json:"url"`
	Headers    map[string]string `json:"headers"` // caller must send
}

var (
	caseIDRe  = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)
	checkIDRe = regexp.MustCompile(`^[A-Za-z0-9._-]{1,96}$`)
)

func (s *Server) HandleEvidenceInit(w http.ResponseWriter, r *http.Request) {
	// 1) Decode JSON
	var req EvidenceInitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_json")
		return
	}

	// 2) Normalize + validate case/check IDs
	req.CaseID = strings.TrimSpace(req.CaseID)
	req.CheckID = strings.TrimSpace(req.CheckID)

	if !caseIDRe.MatchString(req.CaseID) {
		writeErr(w, http.StatusBadRequest, "invalid_caseId")
		return
	}
	if !checkIDRe.MatchString(req.CheckID) {
		writeErr(w, http.StatusBadRequest, "invalid_checkId")
		return
	}

	// 3) Enforce URL param consistency
	pathCaseID := r.PathValue("caseId")
	pathCheckID := r.PathValue("checkId")

	if pathCaseID != "" && pathCaseID != req.CaseID {
		writeErr(w, http.StatusBadRequest, "caseId_mismatch")
		return
	}
	if pathCheckID != "" && pathCheckID != req.CheckID {
		writeErr(w, http.StatusBadRequest, "checkId_mismatch")
		return
	}

	// 4) Validate files array
	if len(req.Files) == 0 || len(req.Files) > s.policy.MaxFiles {
		writeErr(w, http.StatusBadRequest, "invalid_file_count")
		return
	}

	// Authorization uses only the signed canonical principal and persisted IDs.
	// Hold the database row locks through presigning and commit before exposing
	// URLs. Revocation/state changes block on these locks, or win first and deny.
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if _, err := uuid.Parse(claims.Sub); err != nil {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	if s.db == nil {
		writeErr(w, http.StatusServiceUnavailable, "authorization_unavailable")
		return
	}
	tx, err := s.db.BeginTx(r.Context(), &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, "authorization_unavailable")
		return
	}
	defer tx.Rollback()
	var allowed bool
	if err := tx.QueryRowContext(r.Context(), `SELECT check_evidence_upload($1,$2,$3)`, claims.Sub, req.CaseID, req.CheckID).Scan(&allowed); err != nil {
		writeErr(w, http.StatusServiceUnavailable, "authorization_unavailable")
		return
	}
	if !allowed {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}

	// 5) Build a single store for this request (DO Spaces)
	store := s.s3

	if store == nil {
		writeErr(w, http.StatusInternalServerError, "storage_not_configured")
		return
	}

	// 6) Set expiration for presigned URLs
	expiresIn := 10 * time.Minute
	expiresAt := time.Now().UTC().Add(expiresIn)

	resp := EvidenceInitResponse{
		CaseID:  req.CaseID,
		CheckID: req.CheckID,
		Expires: expiresAt.Format(time.RFC3339),
		Uploads: make([]EvidenceInitFileOut, 0, len(req.Files)),
	}

	// 7) For each requested file: validate → build key → presign PUT
	for _, f := range req.Files {
		name := strings.TrimSpace(f.Name)
		mime := strings.TrimSpace(f.MimeType)

		if name == "" || len(name) > 200 {
			writeErr(w, http.StatusBadRequest, "invalid_file_name")
			return
		}
		if f.Size <= 0 || f.Size > s.policy.MaxBytes {
			writeErr(w, http.StatusBadRequest, "invalid_file_size")
			return
		}
		if !s.policy.AllowedMimeType[mime] {
			writeErr(w, http.StatusBadRequest, "invalid_mimeType")
			return
		}

		// IMPORTANT: this must match the prefix policy you’ll enforce in /commit
		uploadID := uuid.NewString()
		storageKey := storage.BuildStorageKey(req.CaseID, req.CheckID, uploadID, name)

		u, hdrs, err := store.PresignPut(r.Context(), storageKey, mime, expiresIn)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "presign_failed")
			return
		}

		resp.Uploads = append(resp.Uploads, EvidenceInitFileOut{
			Name:       name,
			MimeType:   mime,
			Size:       f.Size,
			StorageKey: storageKey,
			Method:     "PUT",
			URL:        u,
			Headers:    hdrs,
		})
	}

	if err := tx.Commit(); err != nil {
		writeErr(w, http.StatusServiceUnavailable, "authorization_unavailable")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}
