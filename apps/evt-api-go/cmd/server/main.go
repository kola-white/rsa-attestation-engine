package main

import (
	"log"
	"net/http"
	"time"

	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/config"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/httpapi"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           httpapi.NewRouter(&cfg),
		ReadTimeout:       10 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	log.Printf("evt-api-go listening on %s", cfg.Addr)
	log.Fatal(srv.ListenAndServe())
}
