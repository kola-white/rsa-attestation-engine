package httpapi

import (
	"context"
	"log"
	"net/http"

	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/config"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/storage"
)

type Server struct {
	cfg    *config.Config
	s3     *storage.S3Store
	policy storage.EvidencePolicy
}

func NewRouter(cfg *config.Config) http.Handler {
	s3, err := storage.NewS3Store(context.Background(), cfg.Spaces)
	if err != nil {
		panic(err)
	}

	s := &Server{
		cfg:    cfg,
		s3:     s3,
		policy: storage.DefaultEvidencePolicy(),
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
	})

	// Auth (NEW)
	mux.HandleFunc("/auth/exchange", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[auth:exchange] called method=%s path=%s remote=%s", r.Method, r.URL.Path, r.RemoteAddr)
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.HandleAuthExchange(w, r)
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
