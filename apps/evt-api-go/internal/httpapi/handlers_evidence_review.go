package httpapi

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/auth"
)

// All review statements run through bounded owner functions; caller assertions
// never replace the authenticated canonical principal or its current case grant.
func (s *Server) evidenceAccessTx(w http.ResponseWriter, r *http.Request) (context.Context, context.CancelFunc, *sql.Tx, string, bool) {
	w.Header().Set("Cache-Control", "private, no-store")
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		writeErr(w, 401, "unauthorized")
		return nil, nil, nil, "", false
	}
	if _, err := uuid.Parse(claims.Sub); err != nil {
		writeErr(w, 404, "not_found")
		return nil, nil, nil, "", false
	}
	if s.db == nil {
		writeErr(w, 503, "evidence_unavailable")
		return nil, nil, nil, "", false
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		cancel()
		writeErr(w, 503, "evidence_unavailable")
		return nil, nil, nil, "", false
	}
	if err = pinEvidenceSchema(ctx, tx); err != nil {
		tx.Rollback()
		cancel()
		writeErr(w, 503, "evidence_unavailable")
		return nil, nil, nil, "", false
	}
	return ctx, cancel, tx, claims.Sub, true
}
func evidenceReviewError(w http.ResponseWriter, err error) {
	var state *pgconn.PgError
	switch {
	case errors.Is(err, sql.ErrNoRows):
		writeErr(w, 404, "not_found")
	case errors.As(err, &state):
		switch state.Code {
		case "P0002":
			writeErr(w, 404, "not_found")
		case "P1001":
			writeErr(w, 403, "self_review_not_allowed")
		case "23505":
			writeErr(w, 409, "review_already_recorded")
		case "22023":
			writeErr(w, 400, "invalid_review")
		case "23514":
			writeErr(w, 409, "evidence_not_reviewable")
		default:
			writeErr(w, 503, "evidence_unavailable")
		}
	default:
		writeErr(w, 503, "evidence_unavailable")
	}
}
func evidenceReviewScope(w http.ResponseWriter, r *http.Request) bool {
	if !caseIDRe.MatchString(r.PathValue("caseId")) || !checkIDRe.MatchString(r.PathValue("checkId")) {
		writeErr(w, 400, "invalid_evidence_scope")
		return false
	}
	return true
}
func (s *Server) HandleEvidenceChecks(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, tx, principal, ok := s.evidenceAccessTx(w, r)
	if !ok {
		return
	}
	defer cancel()
	defer tx.Rollback()
	var data json.RawMessage
	if err := tx.QueryRowContext(ctx, `SELECT list_evidence_checks($1)`, principal).Scan(&data); err != nil {
		evidenceReviewError(w, err)
		return
	}
	if err := tx.Commit(); err != nil {
		evidenceReviewError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, 200, map[string]any{"items": data})
}
func (s *Server) HandleEvidenceList(w http.ResponseWriter, r *http.Request) {
	if !evidenceReviewScope(w, r) {
		return
	}
	ctx, cancel, tx, principal, ok := s.evidenceAccessTx(w, r)
	if !ok {
		return
	}
	defer cancel()
	defer tx.Rollback()
	var data json.RawMessage
	if err := tx.QueryRowContext(ctx, `SELECT list_evidence_for_review($1,$2,$3)`, principal, r.PathValue("caseId"), r.PathValue("checkId")).Scan(&data); err != nil {
		evidenceReviewError(w, err)
		return
	}
	if err := tx.Commit(); err != nil {
		evidenceReviewError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, 200, map[string]any{"caseId": r.PathValue("caseId"), "checkId": r.PathValue("checkId"), "evidence": data})
}
func (s *Server) HandleEvidenceDownload(w http.ResponseWriter, r *http.Request) {
	if !evidenceReviewScope(w, r) {
		return
	}
	id, err := uuid.Parse(r.PathValue("uploadId"))
	if err != nil {
		writeErr(w, 404, "not_found")
		return
	}
	ctx, cancel, tx, principal, ok := s.evidenceAccessTx(w, r)
	if !ok {
		return
	}
	defer cancel()
	defer tx.Rollback()
	var name, kind, key, hash, scan string
	var size int64
	err = tx.QueryRowContext(ctx, `SELECT * FROM get_evidence_for_review($1,$2,$3,$4)`, principal, r.PathValue("caseId"), r.PathValue("checkId"), id.String()).Scan(&name, &kind, &size, &key, &hash, &scan)
	if err != nil {
		evidenceReviewError(w, err)
		return
	}
	if s.s3 == nil || size <= 0 || size > s.policy.MaxBytes || !strings.HasPrefix(key, "sealed-evidence/") {
		writeErr(w, 503, "evidence_unavailable")
		return
	}
	data, observed, err := s.s3.ReadEvidence(ctx, key, s.policy.MaxBytes)
	if err != nil {
		writeErr(w, 503, "evidence_unavailable")
		return
	}
	sum := sha256.Sum256(data)
	if int64(len(data)) != size || observed != kind || hex.EncodeToString(sum[:]) != hash {
		writeErr(w, 503, "evidence_integrity_unavailable")
		return
	}
	if err = tx.Commit(); err != nil {
		evidenceReviewError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": path.Base(strings.ReplaceAll(name, `\`, "/"))}))
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Evidence-Scan-Status", scan)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
func (s *Server) HandleEvidenceReview(w http.ResponseWriter, r *http.Request) {
	if !evidenceReviewScope(w, r) {
		return
	}
	id, err := uuid.Parse(r.PathValue("uploadId"))
	if err != nil {
		writeErr(w, 404, "not_found")
		return
	}
	var input struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
	}
	if err = json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeErr(w, 400, "invalid_review")
		return
	}
	input.Reason = strings.TrimSpace(input.Reason)
	if (input.Decision != "APPROVED" && input.Decision != "REJECTED") || input.Reason == "" || len(input.Reason) > 1000 {
		writeErr(w, 400, "invalid_review")
		return
	}
	ctx, cancel, tx, principal, ok := s.evidenceAccessTx(w, r)
	if !ok {
		return
	}
	defer cancel()
	defer tx.Rollback()
	var result json.RawMessage
	err = tx.QueryRowContext(ctx, `SELECT record_evidence_review($1,$2,$3,$4,$5,$6)`, principal, r.PathValue("caseId"), r.PathValue("checkId"), id.String(), input.Decision, input.Reason).Scan(&result)
	if err != nil {
		evidenceReviewError(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		evidenceReviewError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, 200, result)
}
