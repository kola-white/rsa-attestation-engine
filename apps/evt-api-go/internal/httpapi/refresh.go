package httpapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/auth"
)

type AuthRefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
	DeviceID     string `json:"device_id,omitempty"`
}

type AuthRefreshResponse struct {
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token"`
	User         ExchangeUser `json:"user"`
}

func (s *Server) HandleAuthRefresh(w http.ResponseWriter, r *http.Request) {
	var req AuthRefreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_json")
		return
	}
	req.RefreshToken = strings.TrimSpace(req.RefreshToken)
	if req.RefreshToken == "" {
		writeErr(w, http.StatusBadRequest, "missing_refresh_token")
		return
	}

	now := time.Now()
	incomingHash, err := s.refreshTokenHash(req.RefreshToken)
	if err != nil {
		log.Printf("[auth:refresh] hash: %v", err)
		writeErr(w, http.StatusInternalServerError, "server_error")
		return
	}

	tx, err := s.db.BeginTx(r.Context(), &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		log.Printf("[auth:refresh] tx begin: %v", err)
		writeErr(w, http.StatusInternalServerError, "server_error")
		return
	}
	defer tx.Rollback()

	// Lock the token row if it exists
	var (
		tokenID   uuid.UUID
		sessionID uuid.UUID
		userID    uuid.UUID
		isCurrent bool
		expiresAt time.Time
		revokedAt sql.NullTime
	)

	err = tx.QueryRowContext(r.Context(), `
    SELECT token_id, session_id, user_id, is_current, expires_at, revoked_at
    FROM refresh_tokens
    WHERE token_hash = $1
    FOR UPDATE
  `, incomingHash).Scan(&tokenID, &sessionID, &userID, &isCurrent, &expiresAt, &revokedAt)

	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, http.StatusUnauthorized, "invalid_refresh_token")
		return
	}
	if err != nil {
		log.Printf("[auth:refresh] select token: %v", err)
		writeErr(w, http.StatusInternalServerError, "server_error")
		return
	}

	if revokedAt.Valid || now.After(expiresAt) {
		writeErr(w, http.StatusUnauthorized, "refresh_token_expired_or_revoked")
		return
	}

	if !isCurrent {
		// REUSE DETECTED: revoke entire session family
		if _, err := tx.ExecContext(r.Context(), `
      UPDATE refresh_tokens
      SET revoked_at = now(), revoke_reason = 'reuse_detected', is_current = FALSE
      WHERE session_id = $1 AND revoked_at IS NULL
    `, sessionID); err != nil {
			log.Printf("[auth:refresh] revoke family: %v", err)
			writeErr(w, http.StatusInternalServerError, "server_error")
			return
		}
		if _, err := tx.ExecContext(r.Context(), `
      UPDATE auth_sessions
      SET revoked_at = now(), revoke_reason = 'reuse_detected'
      WHERE session_id = $1 AND revoked_at IS NULL
    `, sessionID); err != nil {
			log.Printf("[auth:refresh] revoke session: %v", err)
			writeErr(w, http.StatusInternalServerError, "server_error")
			return
		}

		if err := tx.Commit(); err != nil {
			log.Printf("[auth:refresh] commit reuse: %v", err)
		}
		writeErr(w, http.StatusUnauthorized, "refresh_token_reused")
		return
	}

	// Load user for response + JWT claims
	var email, name, roleStr, status string
	err = tx.QueryRowContext(r.Context(), `
    SELECT email, name, role, status
    FROM domain_users
    WHERE id = $1
  `, userID).Scan(&email, &name, &roleStr, &status)
	if err != nil {
		log.Printf("[auth:refresh] load user: %v", err)
		writeErr(w, http.StatusInternalServerError, "server_error")
		return
	}
	if status != "active" {
		_, _ = tx.ExecContext(r.Context(), `
      UPDATE refresh_tokens SET revoked_at = now(), revoke_reason='user_inactive', is_current=FALSE
      WHERE session_id=$1 AND revoked_at IS NULL
    `, sessionID)
		_, _ = tx.ExecContext(r.Context(), `
      UPDATE auth_sessions SET revoked_at=now(), revoke_reason='user_inactive'
      WHERE session_id=$1 AND revoked_at IS NULL
    `, sessionID)
		_ = tx.Commit()
		writeErr(w, http.StatusForbidden, "account_inactive")
		return
	}

	// Parse role from DB into canonical typed role
	userRole, err := auth.ParseRole(roleStr)
	if err != nil {
		log.Printf("[auth:refresh] invalid role in db user_id=%s role=%q: %v", userID.String(), roleStr, err)
		writeErr(w, http.StatusUnauthorized, "invalid_role")
		return
	}

	// Rotate: old token no longer current
	if _, err := tx.ExecContext(r.Context(), `
    UPDATE refresh_tokens
    SET is_current = FALSE, replaced_at = now(), last_used_at = now()
    WHERE token_id = $1
  `, tokenID); err != nil {
		log.Printf("[auth:refresh] update old token: %v", err)
		writeErr(w, http.StatusInternalServerError, "server_error")
		return
	}

	// Insert new current token for same session family
	newRaw, err := randomHex(32)
	if err != nil {
		log.Printf("[auth:refresh] mint new refresh: %v", err)
		writeErr(w, http.StatusInternalServerError, "server_error")
		return
	}
	newHash, err := s.refreshTokenHash(newRaw)
	if err != nil {
		log.Printf("[auth:refresh] hash new: %v", err)
		writeErr(w, http.StatusInternalServerError, "server_error")
		return
	}
	newTokenID := uuid.New()
	newExp := now.Add(30 * 24 * time.Hour)

	if _, err := tx.ExecContext(r.Context(), `
    INSERT INTO refresh_tokens (token_id, session_id, user_id, token_hash, is_current, expires_at, last_used_at)
    VALUES ($1,$2,$3,$4,TRUE,$5,now())
  `, newTokenID, sessionID, userID, newHash, newExp); err != nil {
		log.Printf("[auth:refresh] insert new token: %v", err)
		writeErr(w, http.StatusInternalServerError, "server_error")
		return
	}

	// Touch session
	if _, err := tx.ExecContext(r.Context(), `
    UPDATE auth_sessions SET last_used_at = now()
    WHERE session_id = $1
  `, sessionID); err != nil {
		log.Printf("[auth:refresh] touch session: %v", err)
		writeErr(w, http.StatusInternalServerError, "server_error")
		return
	}

	// Mint access token (sub = domain user UUID)
	access, err := auth.MintAccessTokenHS256(
		s.cfg.Auth.JWTSecret,
		s.cfg.Auth.Issuer,
		s.cfg.Auth.Audience,
		userID.String(),
		email,
		[]auth.Role{userRole},
		15*time.Minute,
	)
	if err != nil {
		log.Printf("[auth:refresh] mint access: %v", err)
		writeErr(w, http.StatusInternalServerError, "token_mint_failed")
		return
	}

	if err := tx.Commit(); err != nil {
		log.Printf("[auth:refresh] commit: %v", err)
		writeErr(w, http.StatusInternalServerError, "server_error")
		return
	}

	writeJSON(w, http.StatusOK, AuthRefreshResponse{
		AccessToken:  access,
		RefreshToken: newRaw,
		User: ExchangeUser{
			ID:    userID.String(),
			Email: email,
			Name:  name,
			Role:  userRole.String(),
		},
	})
}
