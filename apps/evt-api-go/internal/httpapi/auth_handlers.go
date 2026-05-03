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

var errKratosWhoamiNon200 = errors.New("kratos whoami non-200")

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

	who, err := s.kratosWhoami(r.Context(), req.KratosSessionToken)
	if err != nil {
		log.Printf("[auth:exchange] kratos whoami error: %v", err)

		if errors.Is(err, errKratosWhoamiNon200) {
			writeErr(w, http.StatusUnauthorized, "invalid_kratos_session")
			return
		}

		writeErr(w, http.StatusBadGateway, "kratos_unreachable")
		return
	}

	if who == nil || strings.TrimSpace(who.Identity.ID) == "" {
		writeErr(w, http.StatusUnauthorized, "invalid_kratos_session")
		return
	}

	resp, err := s.issueAuthExchangeResponseForKratosIdentity(
		r.Context(),
		who.Identity.ID,
		pickStringTrait(who.Identity.Traits, "email"),
		pickStringTrait(who.Identity.Traits, "name"),
		req.DeviceID,
	)

	if err != nil {
		log.Printf("[auth:exchange] issue token pair failed: %v", err)
		writeErr(w, http.StatusInternalServerError, "server_error")
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) HandleAuthWebExchange(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		log.Printf("[auth:web-exchange] called method=%s path=%s remote=%s", r.Method, r.URL.Path, r.RemoteAddr)
		writeErr(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}

	cookieHeader := strings.TrimSpace(r.Header.Get("Cookie"))
	if cookieHeader == "" {
		writeErr(w, http.StatusUnauthorized, "missing_kratos_cookie")
		return
	}

	who, err := s.kratosWhoamiWithCookie(r.Context(), cookieHeader)
	if err != nil {
		log.Printf("[auth:web-exchange] kratos whoami error: %v", err)

		if errors.Is(err, errKratosWhoamiNon200) {
			writeErr(w, http.StatusUnauthorized, "invalid_kratos_session")
			return
		}

		writeErr(w, http.StatusBadGateway, "kratos_unreachable")
		return
	}

	if who == nil || strings.TrimSpace(who.Identity.ID) == "" {
		writeErr(w, http.StatusUnauthorized, "invalid_kratos_session")
		return
	}

	resp, err := s.issueAuthExchangeResponseForKratosIdentity(
		r.Context(),
		who.Identity.ID,
		pickStringTrait(who.Identity.Traits, "email"),
		pickStringTrait(who.Identity.Traits, "name"),
		"web",
	)

	if err != nil {
		log.Printf("[auth:web-exchange] issue token pair failed: %v", err)
		writeErr(w, http.StatusInternalServerError, "server_error")
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) issueAuthExchangeResponseForKratosIdentity(
	ctx context.Context,
	kratosIdentityID string,
	email string,
	name string,
	deviceID string,
) (*AuthExchangeResponse, error) {
	kratosIdentityID = strings.TrimSpace(kratosIdentityID)
	email = strings.TrimSpace(strings.ToLower(email))
	name = strings.TrimSpace(name)
	deviceID = strings.TrimSpace(deviceID)

	if kratosIdentityID == "" {
		return nil, errors.New("missing kratos identity id")
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	defaultRole := auth.RoleRequestor
	defaultStatus := "active"

	var (
		userID    uuid.UUID
		dbRoleStr string
	)

	err = tx.QueryRowContext(ctx, `
		INSERT INTO domain_users (kratos_identity_id, email, name, role, status)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (kratos_identity_id)
		DO UPDATE SET email = EXCLUDED.email, name = EXCLUDED.name, updated_at = now()
		RETURNING id, role
	`, kratosIdentityID, email, name, defaultRole.String(), defaultStatus).Scan(&userID, &dbRoleStr)

	if err != nil {
		return nil, err
	}

	userRole, err := auth.ParseRole(dbRoleStr)
	if err != nil {
		return nil, err
	}

	sessionID := uuid.New()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO auth_sessions (session_id, user_id, device_id, created_at, last_used_at)
		VALUES ($1,$2,$3,now(),now())
	`, sessionID, userID, deviceID)

	if err != nil {
		return nil, err
	}

	rawRefresh, err := randomHex(32)
	if err != nil {
		return nil, err
	}

	hash, err := s.refreshTokenHash(rawRefresh)
	if err != nil {
		return nil, err
	}

	tokenID := uuid.New()
	refreshExp := time.Now().Add(30 * 24 * time.Hour)

	_, err = tx.ExecContext(ctx, `
		INSERT INTO refresh_tokens (token_id, session_id, user_id, token_hash, is_current, expires_at, last_used_at)
		VALUES ($1,$2,$3,$4,TRUE,$5,now())
	`, tokenID, sessionID, userID, hash, refreshExp)

	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

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
		return nil, err
	}

	return &AuthExchangeResponse{
		AccessToken:  access,
		RefreshToken: rawRefresh,
		User: ExchangeUser{
			ID:    userID.String(),
			Email: email,
			Name:  name,
			Role:  userRole.String(),
		},
	}, nil
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
		return nil, err // <-- network/DNS/TLS/timeout path
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("[auth:kratosWhoami] non-200 status=%d", resp.StatusCode)
		return nil, errKratosWhoamiNon200 // <-- non-200 path
	}

	var out kratosWhoami
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *Server) kratosWhoamiWithCookie(ctx context.Context, cookieHeader string) (*kratosWhoami, error) {
	base := strings.TrimRight(strings.TrimSpace(s.cfg.Kratos.PublicBaseURL), "/")
	if base == "" {
		return nil, errors.New("kratos public base url empty")
	}

	cookieHeader = strings.TrimSpace(cookieHeader)
	if cookieHeader == "" {
		return nil, errors.New("missing cookie header")
	}

	url := base + "/sessions/whoami"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Cookie", cookieHeader)

	cli := &http.Client{Timeout: 10 * time.Second}

	resp, err := cli.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("[auth:kratosWhoamiWithCookie] non-200 status=%d", resp.StatusCode)
		return nil, errKratosWhoamiNon200
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
