package httpapi

import (
	"net/http"

	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/buildinfo"
)

func handleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"git_sha": buildinfo.GitSHA,
	})
}
