package httpapi

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/auth"
)

type AuthExchangeRequest struct {
	KratosSessionToken string `json:"kratos_session_token"`
	DeviceID           string `json:"device_id,omitempty"`
}

type AuthExchangeResponse struct {
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token"`
	User         ExchangeUser `json:"user"`
}

type ExchangeUser struct {
	ID    string `json:"id"`
	Email string `json:"email,omitempty"`
	Name  string `json:"name,omitempty"`
	Role  string `json:"role"`
}

// Minimal shape from Kratos /sessions/whoami (public endpoint).
// We only parse what we need.
type kratosWhoami struct {
	Identity struct {
		ID     string         `json:"id"`
		Traits map[string]any `json:"traits"`
	} `json:"identity"`
	Active bool `json:"active"`
}

func (s *Server) HandleAuthExchange(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		log.Printf("[auth:exchange] called method=%s path=%s remote=%s", r.Method, r.URL.Path, r.RemoteAddr)
		writeErr(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}

	var req AuthExchangeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_json")
		return
	}
	req.KratosSessionToken = strings.TrimSpace(req.KratosSessionToken)
	if req.KratosSessionToken == "" {
		writeErr(w, http.StatusBadRequest, "missing_kratos_session_token")
		return
	}

	// 1) Validate session with Kratos public whoami using X-Session-Token
	who, err := s.kratosWhoami(r.Context(), req.KratosSessionToken)
	if err != nil {
		// Treat as invalid/expired token from client’s POV
		writeErr(w, http.StatusUnauthorized, "invalid_kratos_session")
		return
	}
	if who.Identity.ID == "" {
		writeErr(w, http.StatusUnauthorized, "invalid_kratos_session")
		return
	}

	// 2) Extract traits (best-effort, no assumptions about schema beyond common keys)
	email := pickStringTrait(who.Identity.Traits, "email")
	name := pickStringTrait(who.Identity.Traits, "name")

	// 3) Map to your Phase-1 role (hard-coded here; later map from DB)
	role := "requestor"

	emailLower := strings.ToLower(strings.TrimSpace(email))

	if strings.HasSuffix(emailLower, "@protonmail.com") {
    		role = "hr_reviewer"
	}

	// 4) Mint tokens
	access, err := auth.MintAccessTokenHS256(
		s.cfg.Auth.JWTSecret,
		s.cfg.Auth.Issuer,
		s.cfg.Auth.Audience,
		who.Identity.ID,
		email,
		[]string{role},
		15*time.Minute,
	)
	if err != nil {
		log.Printf("[auth:exchange] token_mint_failed: access token: %v", err)
		writeErr(w, http.StatusInternalServerError, "token_mint_failed")
		return
	}

	rawRefresh, err := randomHex(32) // 256-bit
	if err != nil {
		log.Printf("[auth:exchange] token_mint_failed: access token: %v", err)
		writeErr(w, http.StatusInternalServerError, "token_mint_failed")
		return
	}

	tx, err := s.db.BeginTx(r.Context(), &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		log.Printf("[auth:exchange] tx begin: %v", err)
		writeErr(w, http.StatusInternalServerError, "server_error")
		return
	}
	defer tx.Rollback()

	// 1) Upsert domain user mapped to Kratos identity id
	var userID uuid.UUID
	err = tx.QueryRowContext(r.Context(), `
	INSERT INTO domain_users (kratos_identity_id, email, name, role, status)
	VALUES ($1,$2,$3,$4,'active')
	ON CONFLICT (kratos_identity_id)
	DO UPDATE SET email = EXCLUDED.email, name = EXCLUDED.name, updated_at = now()
	RETURNING id
	`, who.Identity.ID, email, name, role).Scan(&userID)
	if err != nil {
		log.Printf("[auth:exchange] upsert domain_user: %v", err)
		writeErr(w, http.StatusInternalServerError, "server_error")
		return
	}

	// 2) Create auth session
	sessionID := uuid.New()
	_, err = tx.ExecContext(r.Context(), `
	INSERT INTO auth_sessions (session_id, user_id, device_id, created_at, last_used_at)
	VALUES ($1,$2,$3,now(),now())
	`, sessionID, userID, req.DeviceID)
	if err != nil {
		log.Printf("[auth:exchange] insert auth_session: %v", err)
		writeErr(w, http.StatusInternalServerError, "server_error")
		return
	}

	// 3) Mint + store refresh token (raw returned to client, hash stored)
	rawRefresh, err = randomHex(32)
	if err != nil {
		log.Printf("[auth:exchange] mint refresh: %v", err)
		writeErr(w, http.StatusInternalServerError, "token_mint_failed")
		return
	}
	
	hash, err := s.refreshTokenHash(rawRefresh)
	if err != nil {
		log.Printf("[auth:exchange] hash refresh: %v", err)
		writeErr(w, http.StatusInternalServerError, "server_error")
		return
	}

	tokenID := uuid.New()
	refreshExp := time.Now().Add(30 * 24 * time.Hour)

	_, err = tx.ExecContext(r.Context(), `
	INSERT INTO refresh_tokens (token_id, session_id, user_id, token_hash, is_current, expires_at, last_used_at)
	VALUES ($1,$2,$3,$4,TRUE,$5,now())
	`, tokenID, sessionID, userID, hash, refreshExp)
	if err != nil {
		log.Printf("[auth:exchange] insert refresh_token: %v", err)
		writeErr(w, http.StatusInternalServerError, "server_error")
		return
	}

	if err := tx.Commit(); err != nil {
		log.Printf("[auth:exchange] commit: %v", err)
		writeErr(w, http.StatusInternalServerError, "server_error")
		return
	}

	// access token should use the *domain user UUID* as subject
	access, err = auth.MintAccessTokenHS256(
		s.cfg.Auth.JWTSecret,
		s.cfg.Auth.Issuer,
		s.cfg.Auth.Audience,
		userID.String(),
		email,
		[]string{role},
		15*time.Minute,
	)

	if err != nil {
		log.Printf("[auth:exchange] token_mint_failed: access token: %v", err)
		writeErr(w, http.StatusInternalServerError, "token_mint_failed")
		return
	}

	writeJSON(w, http.StatusOK, AuthExchangeResponse{
		AccessToken:  access,
		RefreshToken: rawRefresh,
		User: ExchangeUser{
			ID:    userID.String(),
			Email: email,
			Name:  name,
			Role:  role,
		},
	})
	}

	func (s *Server) kratosWhoami(ctx context.Context, sessionToken string) (*kratosWhoami, error) {
		base := strings.TrimRight(strings.TrimSpace(s.cfg.Kratos.PublicBaseURL), "/")
		if base == "" {
			return nil, errors.New("kratos public base url empty")
		}
		url := base + "/sessions/whoami"

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("X-Session-Token", sessionToken)
		req.Header.Set("Accept", "application/json")

		cli := &http.Client{Timeout: 10 * time.Second}
		resp, err := cli.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, errors.New("kratos whoami non-200")
		}

		var out kratosWhoami
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			return nil, err
		}
		return &out, nil
	}

	func pickStringTrait(traits map[string]any, key string) string {
		if traits == nil {
			return ""
		}
		v, ok := traits[key]
		if !ok || v == nil {
			return ""
		}
		switch t := v.(type) {
		case string:
			return strings.TrimSpace(t)
		default:
			return ""
		}
	}

	func randomHex(nBytes int) (string, error) {
		b := make([]byte, nBytes)
		if _, err := rand.Read(b); err != nil {
			return "", err
		}
		return hex.EncodeToString(b), nil
	}

	

	func (s *Server) refreshTokenHash(token string) (string, error) {
		if len(s.hmacKey) == 0 {
			return "", errors.New("hmac key not configured")
		}
		mac := hmac.New(sha256.New, s.hmacKey)
		_, _ = mac.Write([]byte(token))
		return hex.EncodeToString(mac.Sum(nil)), nil
	}
