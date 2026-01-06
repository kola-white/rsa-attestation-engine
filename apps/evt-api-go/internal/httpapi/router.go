package httpapi

import (
	"context"
	"database/sql"
	"encoding/hex"
	"log"
	"net/http"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/config"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/storage"
)

	type Server struct {
		cfg    *config.Config
		db     *sql.DB
		s3     *storage.S3Store
		policy storage.EvidencePolicy
		hmacKey []byte
	}

	func NewRouter(cfg *config.Config) http.Handler {
	// --- Open DB ------------------------------------------------------------
	db, err := sql.Open("pgx", cfg.Auth.DBDSN)
	if err != nil {
		log.Fatalf("db open failed: %v", err)
	}

	if err := db.Ping(); err != nil {
		log.Fatalf("db ping failed: %v", err)
	}

	// --- Decode HMAC key ----------------------------------------------------
	hmacKey, err := hex.DecodeString(cfg.Auth.RefreshTokenHMACKey)
	if err != nil {
		log.Fatalf("invalid EVT_REFRESH_TOKEN_HMAC_KEY (must be hex): %v", err)
	}	
	if len(hmacKey) < 32 {
	log.Fatalf("EVT_REFRESH_TOKEN_HMAC_KEY too short: got %d bytes, need >= 32", len(hmacKey))
	}

	// --- S3 ---------------------------------------------------------------
	s3, err := storage.NewS3Store(context.Background(), cfg.Spaces)
	if err != nil {
		log.Fatalf("s3 init failed: %v", err)
	}

	s := &Server{
		cfg:    cfg,
		db:   	db,
		s3:     s3,
		policy: storage.DefaultEvidencePolicy(),
		hmacKey: hmacKey,
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
	})

	// Auth (EXCHANGE, REFRESH, LOGOUT)
	mux.HandleFunc("POST /auth/exchange", s.HandleAuthExchange)
	mux.HandleFunc("/auth/refresh", func(w http.ResponseWriter, r *http.Request) {
	log.Printf("[auth:refresh] called method=%s path=%s remote=%s", r.Method, r.URL.Path, r.RemoteAddr)
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.HandleAuthRefresh(w, r)
	})

	mux.HandleFunc("/auth/logout", func(w http.ResponseWriter, r *http.Request) {
	log.Printf("[auth:logout] called method=%s path=%s remote=%s", r.Method, r.URL.Path, r.RemoteAddr)
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.HandleAuthLogout(w, r)
	})

	// Contract paths:
	mux.HandleFunc("POST /v1/cases/{caseId}/checks/{checkId}/evidence:init", s.HandleEvidenceInit)
	mux.HandleFunc("POST /v1/cases/{caseId}/checks/{checkId}/evidence:complete", s.HandleEvidenceComplete)

	// Optional legacy aliases:
	mux.HandleFunc("POST /v1/evidence/init", s.HandleEvidenceInit)
	mux.HandleFunc("POST /v1/evidence/commit", s.HandleEvidenceComplete)

	return withReqLog(withCORS(mux))
}

func (s *Server) HandleEvidenceComplete(w http.ResponseWriter, r *http.Request) {
	log.Printf("[evidence:complete] called path=%s", r.URL.Path)
	writeErr(w, http.StatusNotImplemented, "evidence_complete_requires_persistence")
}

func withReqLog(next http.Handler) http.Handler {
  return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    log.Printf("[req] %s %s remote=%s", r.Method, r.URL.Path, r.RemoteAddr)
    next.ServeHTTP(w, r)
  })
}
