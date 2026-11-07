package httpapi

import (
	"context"
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
	SessionID string                `json:"uploadSessionId"`
	CaseID    string                `json:"caseId"`
	CheckID   string                `json:"checkId"`
	Expires   string                `json:"expiresAt"`
	Uploads   []EvidenceInitFileOut `json:"uploads"`
}

type EvidenceInitFileOut struct {
	UploadID   string            `json:"uploadId"`
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

	if req.CaseID == "" {
		req.CaseID = r.PathValue("caseId")
	}
	if req.CheckID == "" {
		req.CheckID = r.PathValue("checkId")
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

	for _, f := range req.Files {
		if strings.TrimSpace(f.Name) == "" || len(strings.TrimSpace(f.Name)) > 200 {
			writeErr(w, 400, "invalid_file_name")
			return
		}
		if f.Size <= 0 || f.Size > s.policy.MaxBytes {
			writeErr(w, 400, "invalid_file_size")
			return
		}
		if !s.policy.AllowedMimeType[strings.TrimSpace(f.MimeType)] {
			writeErr(w, 400, "invalid_mimeType")
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	r = r.WithContext(ctx)

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
	if err = pinEvidenceSchema(r.Context(), tx); err != nil {
		writeErr(w, 503, "persistence_unavailable")
		return
	}
	if err := lockEvidenceScope(r.Context(), tx, req.CaseID, req.CheckID); err != nil {
		writeErr(w, 503, "persistence_unavailable")
		return
	}
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

	sessionID := uuid.NewString()
	var reserved int
	if err := tx.QueryRowContext(r.Context(), `SELECT count(*) FROM evidence_uploads u JOIN upload_sessions s USING(upload_session_id) WHERE s.case_id=$1 AND s.check_id=$2 AND (u.status='RECEIVED' OR s.expires_at>clock_timestamp())`, req.CaseID, req.CheckID).Scan(&reserved); err != nil {
		writeErr(w, 503, "persistence_unavailable")
		return
	}
	if reserved+len(req.Files) > s.policy.MaxFiles {
		writeErr(w, 409, "evidence_limit_reached")
		return
	}
	if _, err := tx.ExecContext(r.Context(), `INSERT INTO upload_sessions(upload_session_id,principal_id,case_id,check_id,expires_at) VALUES($1,$2,$3,$4,$5)`, sessionID, claims.Sub, req.CaseID, req.CheckID, expiresAt); err != nil {
		writeErr(w, 503, "persistence_unavailable")
		return
	}
	resp := EvidenceInitResponse{
		SessionID: sessionID,
		CaseID:    req.CaseID,
		CheckID:   req.CheckID,
		Expires:   expiresAt.Format(time.RFC3339),
		Uploads:   make([]EvidenceInitFileOut, 0, len(req.Files)),
	}

	// 7) For each requested file: validate → build key → presign PUT
	for _, f := range req.Files {
		name := strings.TrimSpace(f.Name)
		mime := strings.TrimSpace(f.MimeType)

		// Completion resolves this persisted key; callers cannot choose a source key.
		uploadID := uuid.NewString()
		storageKey := storage.BuildStorageKey(req.CaseID, req.CheckID, uploadID, name)

		u, hdrs, err := store.PresignPut(r.Context(), storageKey, mime, expiresIn)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "presign_failed")
			return
		}

		if _, err := tx.ExecContext(r.Context(), `INSERT INTO evidence_uploads(upload_id,upload_session_id,staging_key,file_name,expected_mime_type,expected_size,case_id,check_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, uploadID, sessionID, storageKey, name, mime, f.Size, req.CaseID, req.CheckID); err != nil {
			writeErr(w, 503, "persistence_unavailable")
			return
		}
		resp.Uploads = append(resp.Uploads, EvidenceInitFileOut{
			UploadID:   uploadID,
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
