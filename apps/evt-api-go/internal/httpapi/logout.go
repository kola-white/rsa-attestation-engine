package httpapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

type AuthLogoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}

func (s *Server) HandleAuthLogout(w http.ResponseWriter, r *http.Request) {
	var req AuthLogoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_json")
		return
	}
	req.RefreshToken = strings.TrimSpace(req.RefreshToken)
	if req.RefreshToken == "" {
		writeErr(w, http.StatusBadRequest, "missing_refresh_token")
		return
	}

  hash, err := s.refreshTokenHash(req.RefreshToken)
  if err != nil {
    log.Printf("[auth:logout] hash: %v", err)
    writeErr(w, http.StatusInternalServerError, "server_error")
    return
  }

	tx, err := s.db.BeginTx(r.Context(), &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		log.Printf("[auth:logout] tx begin: %v", err)
		writeErr(w, http.StatusInternalServerError, "server_error")
		return
	}
	defer tx.Rollback()

	var sessionID uuid.UUID
	err = tx.QueryRowContext(r.Context(), `
    SELECT session_id
    FROM refresh_tokens
    WHERE token_hash = $1
  `, hash).Scan(&sessionID)
	if errors.Is(err, sql.ErrNoRows) {
		// Idempotent logout: return 200 even if token not found.
		writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
		return
	}
	if err != nil {
		log.Printf("[auth:logout] select session: %v", err)
		writeErr(w, http.StatusInternalServerError, "server_error")
		return
	}

	_, _ = tx.ExecContext(r.Context(), `
    UPDATE refresh_tokens
    SET revoked_at = now(), revoke_reason = 'logout', is_current = FALSE
    WHERE session_id = $1 AND revoked_at IS NULL
  `, sessionID)

	_, _ = tx.ExecContext(r.Context(), `
    UPDATE auth_sessions
    SET revoked_at = now(), revoke_reason = 'logout'
    WHERE session_id = $1 AND revoked_at IS NULL
  `, sessionID)

	if err := tx.Commit(); err != nil {
		log.Printf("[auth:logout] commit: %v", err)
		writeErr(w, http.StatusInternalServerError, "server_error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}
