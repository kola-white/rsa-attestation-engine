package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/buildinfo"
)

func TestHandleVersion_ReturnsBuildGitSHA(t *testing.T) {
	originalGitSHA := buildinfo.GitSHA
	buildinfo.GitSHA = "0123456789abcdef0123456789abcdef01234567"
	t.Cleanup(func() {
		buildinfo.GitSHA = originalGitSHA
	})

	req := httptest.NewRequest(http.MethodGet, "/version", nil)
	rec := httptest.NewRecorder()

	handleVersion(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got struct {
		GitSHA string `json:"git_sha"`
	}

	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if got.GitSHA != buildinfo.GitSHA {
		t.Fatalf("git_sha = %q, want %q", got.GitSHA, buildinfo.GitSHA)
	}
}
