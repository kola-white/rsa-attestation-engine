package httpapi

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

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
	CaseID  string               `json:"caseId"`
	CheckID string               `json:"checkId"`
	Expires string               `json:"expiresAt"`
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
	maxFiles     = 3
	maxSizeBytes = int64(5 * 1024 * 1024) // 5MB

	caseIDRe  = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)
	checkIDRe = regexp.MustCompile(`^[A-Za-z0-9._-]{1,96}$`)

	allowedMimes = map[string]bool{
		"application/pdf": true,
		"image/jpeg":      true,
		"image/png":       true,
	}
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

	// 3) Validate files array
	if len(req.Files) == 0 || len(req.Files) > maxFiles {
		writeErr(w, http.StatusBadRequest, "invalid_file_count")
		return
	}

	// 4) Build a single store for this request (DO Spaces)
	store := s.s3

	if store == nil {
		writeErr(w, http.StatusInternalServerError, "storage_not_configured")
		return
	}

	// 5) Set expiration for presigned URLs
	expiresIn := 10 * time.Minute
	expiresAt := time.Now().UTC().Add(expiresIn)

	resp := EvidenceInitResponse{
		CaseID:  req.CaseID,
		CheckID: req.CheckID,
		Expires: expiresAt.Format(time.RFC3339),
		Uploads: make([]EvidenceInitFileOut, 0, len(req.Files)),
	}

	// 6) For each requested file: validate → build key → presign PUT
	for _, f := range req.Files {
		name := strings.TrimSpace(f.Name)
		mime := strings.TrimSpace(f.MimeType)

		if name == "" || len(name) > 200 {
			writeErr(w, http.StatusBadRequest, "invalid_file_name")
			return
		}
		if !allowedMimes[mime] {
			writeErr(w, http.StatusBadRequest, "invalid_mimeType")
			return
		}
		if f.Size <= 0 || f.Size > maxSizeBytes {
			writeErr(w, http.StatusBadRequest, "invalid_file_size")
			return
		}

		// IMPORTANT: this must match the prefix policy you’ll enforce in /commit
		uploadID := uuid.NewString()
		storageKey := storage.BuildStorageKey(req.CaseID, req.CheckID, name,  uploadID)

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

	writeJSON(w, http.StatusOK, resp)
}
