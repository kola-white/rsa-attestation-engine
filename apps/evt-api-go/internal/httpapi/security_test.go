package httpapi

import (
	"context"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/auth"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/credential/sdjwtvc"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/storage"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/config"
)

func TestExchangeRejectsInactiveKratosSession(t *testing.T) {
	for _, web := range []bool{false, true} {
		t.Run(map[bool]string{false: "native", true: "web"}[web], func(t *testing.T) {
			kratos := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/sessions/whoami" {
					t.Error("wrong whoami path")
				}
				json.NewEncoder(w).Encode(map[string]any{"active": false, "identity": map[string]any{"id": "test-identity"}})
			}))
			defer kratos.Close()
			s := &Server{cfg: &config.Config{Kratos: config.KratosConfig{PublicBaseURL: kratos.URL}}}
			r := httptest.NewRequest(http.MethodPost, "/auth/exchange", strings.NewReader(`{"kratos_session_token":"test-token"}`))
			r.Header.Set("Cookie", "ory_kratos_session=test-cookie")
			w := httptest.NewRecorder()
			if web {
				s.HandleAuthWebExchange(w, r)
			} else {
				s.HandleAuthExchange(w, r)
			}
			if w.Code != http.StatusUnauthorized {
				t.Fatalf("inactive session returned %d", w.Code)
			}
		})
	}
}

func TestIssuerDestinationPolicy(t *testing.T) {
	for _, raw := range []string{"127.0.0.1", "10.0.0.1", "172.16.0.1", "192.168.0.1", "169.254.169.254", "100.100.100.200", "0.0.0.0", "224.0.0.1", "240.0.0.1", "::1", "::ffff:127.0.0.1", "fc00::1", "fe80::1", "64:ff9b::7f00:1", "2002:7f00:1::"} {
		if publicIssuerAddress(netip.MustParseAddr(raw)) {
			t.Errorf("accepted internal destination %s", raw)
		}
	}
	for _, raw := range []string{"8.8.8.8", "2606:4700:4700::1111"} {
		if !publicIssuerAddress(netip.MustParseAddr(raw)) {
			t.Errorf("rejected public destination %s", raw)
		}
	}
	for _, host := range []string{"127.0.0.1:443", "[::1]:443", "localhost:443"} {
		conn, err := dialPublicIssuer(context.Background(), "tcp", host)
		if conn != nil {
			conn.Close()
			t.Fatal("dialed an internal destination")
		}
		if err == nil || !strings.Contains(err.Error(), "not public") {
			t.Errorf("%s: expected policy rejection, got %v", host, err)
		}
	}
	if issuerHTTPClient().Transport.(*http.Transport).Proxy != nil {
		t.Fatal("proxy bypasses destination policy")
	}
}

func TestCORSOrigins(t *testing.T) {
	for _, origin := range []string{"https://mvp.cvera.app", "https://attacker.example", "https://mvp.cvera.app.attacker.example"} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodOptions, "/auth/web/exchange", nil)
		r.Header.Set("Origin", origin)
		withCORS(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("preflight reached handler") })).ServeHTTP(w, r)
		allowed := w.Header().Get("Access-Control-Allow-Origin")
		if (allowed == origin) != (origin == "https://mvp.cvera.app") {
			t.Errorf("unexpected origin permission for %s", origin)
		}
	}
}

func TestEvidenceRoutesRequireAuthentication(t *testing.T) {
	s := &Server{cfg: &config.Config{Auth: config.AuthConfig{JWTSecret: strings.Repeat("s", 32), Issuer: "test-issuer", Audience: "test-audience"}}}
	mux := http.NewServeMux()
	s.registerEvidenceRoutes(mux)
	for _, path := range []string{"/v1/cases/case-b/checks/check-b/evidence:init", "/v1/cases/case-b/checks/check-b/evidence:complete", "/v1/evidence/init", "/v1/evidence/commit"} {
		for _, header := range []string{"", "Bearer invalid"} {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
			r.Header.Set("Authorization", header)
			mux.ServeHTTP(w, r)
			if w.Code != http.StatusUnauthorized {
				t.Fatalf("%s without valid auth: %d", path, w.Code)
			}
		}
	}
}

// Authentication is fixed, but case authorization/persistence remains an OPEN
// release blocker. Offline presigning uses synthetic credentials and no upload.
func TestReleaseBlockerEvidenceCaseScope(t *testing.T) {
	cfg := &config.Config{Auth: config.AuthConfig{JWTSecret: strings.Repeat("s", 32), Issuer: "test-issuer", Audience: "test-audience"}}
	store, err := storage.NewS3Store(context.Background(), config.SpacesConfig{AccessKey: "synthetic-test-key", SecretKey: "synthetic-test-secret", Region: "sfo3", Endpoint: "https://storage.example.invalid", Bucket: "synthetic-test-bucket"})
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{cfg: cfg, s3: store, policy: storage.DefaultEvidencePolicy()}
	mux := http.NewServeMux()
	s.registerEvidenceRoutes(mux)
	raw, err := auth.MintAccessTokenHS256(cfg.Auth.JWTSecret, cfg.Auth.Issuer, cfg.Auth.Audience, "unrelated-user", "", []auth.Role{auth.RoleRequestor}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/cases/case-b/checks/check-b/evidence:init", strings.NewReader(`{"caseId":"case-b","checkId":"check-b","files":[{"name":"synthetic.pdf","mimeType":"application/pdf","size":1}]}`))
	r.Header.Set("Authorization", "Bearer "+raw)
	mux.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("scope-gap reproduction: %d %s", w.Code, w.Body.String())
	}
	t.Log("RELEASE BLOCKER reproduced: unrelated authenticated user obtained an offline synthetic upload URL for arbitrary case-b/check-b; no storage request was made")
}

func TestRequestBodyLimit(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/auth/exchange", strings.NewReader(strings.Repeat("x", (1<<20)+1)))
	withRequestLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err == nil {
			t.Error("oversized request accepted")
		}
		if len(body) > 1<<20 {
			t.Error("body read exceeded limit")
		}
	})).ServeHTTP(w, r)
}

func TestGinProtectedRoleBoundary(t *testing.T) {
	const secret = "temporary-application-jwt-test-secret"
	engine := gin.New()
	engine.Use(auth.GinRequireAuth(secret, "issuer", "audience"))
	engine.POST("/protected", auth.GinRequireRole(auth.RoleHRReviewer), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	for _, tc := range []struct {
		role   auth.Role
		secret string
		want   int
	}{{auth.RoleHRReviewer, secret, 204}, {auth.RoleRequestor, secret, 403}, {auth.RoleHRReviewer, "wrong-secret", 401}} {
		raw, err := auth.MintAccessTokenHS256(tc.secret, "issuer", "audience", "synthetic-user", "", []auth.Role{tc.role}, time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/protected", nil)
		r.Header.Set("Authorization", "Bearer "+raw)
		engine.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatalf("role %s: %d, want %d", tc.role, w.Code, tc.want)
		}
	}
}

func TestIssuerMetadataClientRejectsInternalIssuerBeforeTLS(t *testing.T) {
	resolver := &sdjwtvc.IssuerMetadataResolver{Client: issuerMetadataHTTPClient}
	_, _, err := resolver.ResolveIssuerKey(context.Background(), "https://127.0.0.1:443", "test-key", "RS256")
	if err == nil || !strings.Contains(err.Error(), "not public") {
		t.Fatalf("expected destination rejection before TLS, got %v", err)
	}
}
