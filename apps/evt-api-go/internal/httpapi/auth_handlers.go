package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

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
	role := "hr_reviewer"

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

	refresh, err := randomHex(32) // 256-bit
	if err != nil {
		log.Printf("[auth:exchange] token_mint_failed: access token: %v", err)
		writeErr(w, http.StatusInternalServerError, "token_mint_failed")
		return
	}

	// NOTE: Persistence of refresh tokens is intentionally not added here
	// so we do NOT risk breaking your existing S3/evidence work.
	// Add DB-backed refresh rotation once your evt DB table is ready.

	writeJSON(w, http.StatusOK, AuthExchangeResponse{
		AccessToken:  access,
		RefreshToken: refresh,
		User: ExchangeUser{
			ID:    who.Identity.ID,
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
