package httpapi

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/auth"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/storage"
)

type evidenceStore interface {
	PresignPut(context.Context, string, string, time.Duration) (string, map[string]string, error)
	ReadEvidence(context.Context, string, int64) ([]byte, string, error)
	SealEvidence(context.Context, string, string, []byte) error
}

// Keep temporary relations behind the trusted application schema, including when
// the restricted role has database TEMP privilege. Deployment already requires
// an explicit trusted application search_path (the same prerequisite as 000015).
func pinEvidenceSchema(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `SELECT pg_catalog.set_config('search_path',pg_catalog.format('pg_catalog,%I,pg_temp',pg_catalog.current_schema()),true)`)
	return err
}

// Serialize reservations/completions before acquiring the authorization row locks.
// Hash collisions only serialize unrelated checks; they never confer entitlement.
func lockEvidenceScope(ctx context.Context, tx *sql.Tx, c, check string) error {
	_, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, c+"/"+check)
	return err
}

type evidenceCompleteRequest struct {
	SessionID string `json:"uploadSessionId"`
	Uploads   []struct {
		UploadID string `json:"uploadId"`
		Size     int64  `json:"size"`
		MimeType string `json:"mimeType"`
		SHA256   string `json:"sha256,omitempty"` // Optional consistency hint; never canonical.
	} `json:"uploads"`
}
type completedEvidence struct {
	UploadID   string `json:"uploadId"`
	Status     string `json:"status"`
	ScanStatus string `json:"scanStatus"`
	Hash       string `json:"evidence_hash"`
}

func (s *Server) HandleEvidenceComplete(w http.ResponseWriter, r *http.Request) {
	var req evidenceCompleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, 400, "invalid_json")
		return
	}
	if _, err := uuid.Parse(req.SessionID); err != nil || len(req.Uploads) == 0 || len(req.Uploads) > s.policy.MaxFiles {
		writeErr(w, 400, "invalid_upload_session")
		return
	}
	ids := make(map[string]bool)
	for _, u := range req.Uploads {
		id, err := uuid.Parse(u.UploadID)
		if err != nil || id.String() != u.UploadID || ids[u.UploadID] {
			writeErr(w, 400, "invalid_upload_id")
			return
		}
		ids[u.UploadID] = true
		if u.SHA256 != "" {
			b, err := hex.DecodeString(u.SHA256)
			if err != nil || len(b) != 32 || hex.EncodeToString(b) != u.SHA256 {
				writeErr(w, 400, "invalid_sha256")
				return
			}
		}
	}
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		writeErr(w, 401, "unauthorized")
		return
	}
	if _, err := uuid.Parse(claims.Sub); err != nil {
		writeErr(w, 404, "not_found")
		return
	}
	if s.db == nil {
		writeErr(w, 503, "persistence_unavailable")
		return
	}
	// Storage work holds revocation/state locks. Bound it so operators cannot be
	// blocked indefinitely by a stalled object store or client disconnect.
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		writeErr(w, 503, "persistence_unavailable")
		return
	}
	defer tx.Rollback()
	if err = pinEvidenceSchema(ctx, tx); err != nil {
		writeErr(w, 503, "persistence_unavailable")
		return
	}
	var c, check string
	err = tx.QueryRowContext(ctx, `SELECT case_id,check_id FROM upload_sessions WHERE upload_session_id=$1 AND principal_id=$2`, req.SessionID, claims.Sub).Scan(&c, &check)
	if err == sql.ErrNoRows {
		writeErr(w, 404, "not_found")
		return
	}
	if err != nil {
		writeErr(w, 503, "persistence_unavailable")
		return
	}
	if (r.PathValue("caseId") != "" && r.PathValue("caseId") != c) || (r.PathValue("checkId") != "" && r.PathValue("checkId") != check) {
		writeErr(w, 404, "not_found")
		return
	}
	if err = lockEvidenceScope(ctx, tx, c, check); err != nil {
		writeErr(w, 503, "persistence_unavailable")
		return
	}
	var allowed bool
	if err = tx.QueryRowContext(ctx, `SELECT check_evidence_upload($1,$2,$3)`, claims.Sub, c, check).Scan(&allowed); err != nil {
		writeErr(w, 503, "authorization_unavailable")
		return
	}
	if !allowed {
		writeErr(w, 404, "not_found")
		return
	}
	var expired bool
	var closed sql.NullTime
	if err = tx.QueryRowContext(ctx, `SELECT expires_at<=clock_timestamp(),closed_at FROM upload_sessions WHERE upload_session_id=$1 AND principal_id=$2 FOR UPDATE`, req.SessionID, claims.Sub).Scan(&expired, &closed); err != nil {
		writeErr(w, 503, "persistence_unavailable")
		return
	}
	if expired {
		writeErr(w, 409, "upload_session_expired")
		return
	}
	type stored struct {
		id, key, mime, status, hash string
		size                        int64
		data                        []byte
		sealed                      string
	}
	rows, err := tx.QueryContext(ctx, `SELECT upload_id,staging_key,expected_mime_type,expected_size,status,coalesce(evidence_hash,'') FROM evidence_uploads WHERE upload_session_id=$1 ORDER BY upload_id`, req.SessionID)
	if err != nil {
		writeErr(w, 503, "persistence_unavailable")
		return
	}
	var files []stored
	for rows.Next() {
		var f stored
		if err = rows.Scan(&f.id, &f.key, &f.mime, &f.size, &f.status, &f.hash); err != nil {
			break
		}
		files = append(files, f)
	}
	rowErr := rows.Err()
	rows.Close()
	if err != nil || rowErr != nil {
		writeErr(w, 503, "persistence_unavailable")
		return
	}
	if len(files) != len(req.Uploads) {
		writeErr(w, 400, "complete_entire_session")
		return
	}
	for i := range files {
		f := &files[i]
		found := false
		for _, u := range req.Uploads {
			if u.UploadID == f.id {
				found = true
				if u.Size != f.size || u.MimeType != f.mime {
					writeErr(w, 400, "metadata_mismatch")
					return
				}
				if closed.Valid && u.SHA256 != "" && u.SHA256 != f.hash {
					writeErr(w, 409, "hash_mismatch")
					return
				}
			}
		}
		if !found {
			writeErr(w, 404, "not_found")
			return
		}
		if closed.Valid {
			if f.status != "RECEIVED" {
				writeErr(w, 409, "incomplete_session")
				return
			}
			continue
		}
		if s.s3 == nil {
			writeErr(w, 503, "storage_unavailable")
			return
		}
		data, mime, readErr := s.s3.ReadEvidence(ctx, f.key, s.policy.MaxBytes)
		if readErr != nil {
			writeErr(w, 409, "uploaded_object_unavailable")
			return
		}
		if int64(len(data)) != f.size || mime != f.mime {
			writeErr(w, 409, "uploaded_object_mismatch")
			return
		}
		sum := sha256.Sum256(data)
		f.hash = hex.EncodeToString(sum[:])
		f.data = data
		for _, u := range req.Uploads {
			if u.UploadID == f.id && u.SHA256 != "" && u.SHA256 != f.hash {
				writeErr(w, 409, "hash_mismatch")
				return
			}
		}
	}
	if !closed.Valid {
		hashes := make(map[string]bool)
		for i := range files {
			f := &files[i]
			var duplicate bool
			if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM evidence_uploads WHERE case_id=$1 AND check_id=$2 AND evidence_hash=$3 AND status='RECEIVED')`, c, check, f.hash).Scan(&duplicate); err != nil {
				writeErr(w, 503, "persistence_unavailable")
				return
			}
			if duplicate || hashes[f.hash] {
				writeErr(w, 409, "duplicate_evidence")
				return
			}
			hashes[f.hash] = true
		}
		for i := range files {
			f := &files[i]
			f.sealed = storage.BuildSealedEvidenceKey(c, check, f.id, uuid.NewString())
			if err = s.s3.SealEvidence(ctx, f.sealed, f.mime, f.data); err != nil {
				writeErr(w, 503, "storage_unavailable")
				return
			}
			// A failed transaction may leave an unreferenced private object. Never delete
			// after an ambiguous commit: it might be the accepted evidence object.
			if _, err = tx.ExecContext(ctx, `UPDATE evidence_uploads SET status='RECEIVED',sealed_key=$2,evidence_hash=$3,completed_at=clock_timestamp() WHERE upload_id=$1`, f.id, f.sealed, f.hash); err != nil {
				writeErr(w, 503, "persistence_unavailable")
				return
			}
			if _, err = tx.ExecContext(ctx, `INSERT INTO evidence_scan_jobs(upload_id) VALUES($1)`, f.id); err != nil {
				writeErr(w, 503, "persistence_unavailable")
				return
			}
		}
		var valid bool
		if err = tx.QueryRowContext(ctx, `UPDATE upload_sessions SET closed_at=clock_timestamp() WHERE upload_session_id=$1 AND expires_at>clock_timestamp() RETURNING true`, req.SessionID).Scan(&valid); err == sql.ErrNoRows {
			writeErr(w, 409, "upload_session_expired")
			return
		} else if err != nil {
			writeErr(w, 503, "persistence_unavailable")
			return
		}
	}
	if err = tx.Commit(); err != nil {
		writeErr(w, 503, "persistence_unavailable")
		return
	}
	evidence := make([]completedEvidence, 0, len(files))
	for _, f := range files {
		evidence = append(evidence, completedEvidence{f.id, "RECEIVED", "PENDING", f.hash})
	}
	writeJSON(w, http.StatusOK, map[string]any{"caseId": c, "checkId": check, "evidence": evidence})
}
