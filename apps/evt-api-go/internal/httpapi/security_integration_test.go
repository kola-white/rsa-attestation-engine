package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/auth"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/config"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/credential"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/credential/legacyjws"
	evtdb "github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/db"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/evt"
)

// Only the existing disposable local database is allowed. Each test gets an
// isolated schema; confirmed parent metadata is separated from workflow fixtures.
func securityTestServer(t *testing.T) (*Server, *pgxpool.Pool) {
	t.Helper()
	raw := os.Getenv("CVERA_AUTHORITY_TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("CVERA_AUTHORITY_TEST_DATABASE_URL is not set")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "postgres" || u.Hostname() != "127.0.0.1" || u.Port() == "" || u.Path != "/cvera_authority_test" || u.User.Username() != "cvera_test" || u.Query().Get("sslmode") != "disable" {
		t.Fatal("refusing unexpected security test database")
	}
	admin, err := sql.Open("pgx", raw)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close() })
	schema := "security_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = admin.Exec(`CREATE SCHEMA ` + schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(`DROP SCHEMA ` + schema + ` CASCADE`); err != nil {
			t.Error(err)
		}
	})
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	database, err := sql.Open("pgx", u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })

	// Reproduce confirmed parent keys/types and apply the actual migration.
	// Omitted legacy workflow DDL remains explicitly labelled test scaffolding.
	for _, path := range []string{"testdata/proposal_a_legacy_schema.sql", "../../migrations/000014_employer_hr_assignments.sql"} {
		sqlBytes, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		tx, err := database.Begin()
		if err != nil {
			t.Fatal(err)
		}
		if _, err = tx.Exec(string(sqlBytes)); err != nil {
			tx.Rollback()
			t.Fatalf("apply %s: %v", path, err)
		}
		if err = tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	_, err = database.Exec(`
 CREATE TABLE auth_sessions(session_id uuid PRIMARY KEY, user_id uuid, device_id text, created_at timestamptz, last_used_at timestamptz, revoked_at timestamptz, revoke_reason text);
 CREATE TABLE refresh_tokens(token_id uuid PRIMARY KEY, session_id uuid REFERENCES auth_sessions, user_id uuid, token_hash text UNIQUE, is_current boolean, expires_at timestamptz, last_used_at timestamptz, revoked_at timestamptz, revoke_reason text, replaced_at timestamptz);
 CREATE UNIQUE INDEX one_current_refresh ON refresh_tokens(session_id) WHERE is_current;
 CREATE TABLE evt_request_events(request_id uuid, actor_role text, actor_personid text, event_type text, from_status text, to_status text, payload jsonb);
 `)
	if err != nil {
		t.Fatal(err)
	}

	pool, err := pgxpool.New(context.Background(), u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return &Server{db: database, hmacKey: []byte(strings.Repeat("h", 32)), cfg: &config.Config{Auth: config.AuthConfig{JWTSecret: strings.Repeat("s", 32), Issuer: "test-issuer", Audience: "test-audience"}}}, pool
}

func exchangeTestUser(t *testing.T, s *Server, identity string) *AuthExchangeResponse {
	t.Helper()
	response, err := s.issueAuthExchangeResponseForKratosIdentity(context.Background(), identity, "test@example.invalid", "Test User", "test-device")
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func callTokenHandler(handler http.HandlerFunc, token string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]string{"refresh_token": token})
	w := httptest.NewRecorder()
	handler(w, httptest.NewRequest(http.MethodPost, "/auth/test", strings.NewReader(string(body))))
	return w
}

func TestSecurityPostgresInactiveAccountCannotExchange(t *testing.T) {
	s, _ := securityTestServer(t)
	first := exchangeTestUser(t, s, "inactive-identity")
	if _, err := s.db.Exec(`UPDATE domain_users SET status='disabled' WHERE id=$1`, first.User.ID); err != nil {
		t.Fatal(err)
	}
	_, err := s.issueAuthExchangeResponseForKratosIdentity(context.Background(), "inactive-identity", "other@example.invalid", "Other", "test-device")
	if !errors.Is(err, errAccountInactive) {
		t.Fatalf("inactive account exchange: %v", err)
	}
	var count int
	if err := s.db.QueryRow(`SELECT count(*) FROM auth_sessions`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatal("inactive exchange created a session")
	}

	kratos := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"active": true, "identity": map[string]any{"id": "inactive-identity"}})
	}))
	defer kratos.Close()
	s.cfg.Kratos.PublicBaseURL = kratos.URL
	for _, handler := range []http.HandlerFunc{s.HandleAuthExchange, s.HandleAuthWebExchange} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/auth/exchange", strings.NewReader(`{"kratos_session_token":"synthetic-session"}`))
		r.Header.Set("Cookie", "ory_kratos_session=synthetic-session")
		handler(w, r)
		if w.Code != http.StatusForbidden {
			t.Fatalf("inactive account handler: %d", w.Code)
		}
	}
}

func TestSecurityPostgresRefreshRotationReuseAndLogout(t *testing.T) {
	s, _ := securityTestServer(t)
	first := exchangeTestUser(t, s, "rotate-identity")
	w := callTokenHandler(s.HandleAuthRefresh, first.RefreshToken)
	if w.Code != 200 {
		t.Fatalf("refresh: %d %s", w.Code, w.Body.String())
	}
	var second AuthRefreshResponse
	if err := json.Unmarshal(w.Body.Bytes(), &second); err != nil {
		t.Fatal(err)
	}
	if second.RefreshToken == first.RefreshToken {
		t.Fatal("refresh token was not rotated")
	}
	if w := callTokenHandler(s.HandleAuthRefresh, first.RefreshToken); w.Code != 401 {
		t.Fatalf("reuse: %d", w.Code)
	}
	if w := callTokenHandler(s.HandleAuthRefresh, second.RefreshToken); w.Code != 401 {
		t.Fatalf("reuse left descendant usable: %d", w.Code)
	}
	third := exchangeTestUser(t, s, "logout-identity")
	for i := 0; i < 2; i++ {
		if w := callTokenHandler(s.HandleAuthLogout, third.RefreshToken); w.Code != 200 {
			t.Fatalf("logout: %d", w.Code)
		}
	}
	if w := callTokenHandler(s.HandleAuthRefresh, third.RefreshToken); w.Code != 401 {
		t.Fatalf("logout left token usable: %d", w.Code)
	}
	// Independently revoked sessions reject an otherwise live token.
	fourth := exchangeTestUser(t, s, "revoked-session")
	if _, err := s.db.Exec(`UPDATE auth_sessions SET revoked_at=now() WHERE user_id=$1`, fourth.User.ID); err != nil {
		t.Fatal(err)
	}
	if w := callTokenHandler(s.HandleAuthRefresh, fourth.RefreshToken); w.Code != 401 {
		t.Fatalf("revoked session refreshed: %d", w.Code)
	}
}

func TestSecurityPostgresConcurrentRefreshLogout(t *testing.T) {
	s, _ := securityTestServer(t)
	for i := 0; i < 10; i++ {
		pair := exchangeTestUser(t, s, fmt.Sprintf("race-%d", i))
		var refresh, logout *httptest.ResponseRecorder
		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); <-start; refresh = callTokenHandler(s.HandleAuthRefresh, pair.RefreshToken) }()
		go func() { defer wg.Done(); <-start; logout = callTokenHandler(s.HandleAuthLogout, pair.RefreshToken) }()
		close(start)
		wg.Wait()
		if logout.Code != 200 {
			t.Fatalf("logout: %d", logout.Code)
		}
		if refresh.Code != 200 && refresh.Code != 401 && refresh.Code != 500 {
			t.Fatalf("refresh: %d", refresh.Code)
		}
		var live int
		if err := s.db.QueryRow(`SELECT count(*) FROM refresh_tokens WHERE user_id=$1 AND revoked_at IS NULL AND is_current`, pair.User.ID).Scan(&live); err != nil {
			t.Fatal(err)
		}
		if live != 0 {
			t.Fatal("concurrent rotation escaped logout")
		}
	}
}

// Authorized HTTP issuance remains on the production signer/verifier paths.
func TestEmployerAssignedLocalSigning(t *testing.T) {
	s, pool := securityTestServer(t)
	key, err := rsa.GenerateKey(rand.Reader, 3072)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "weekend2-temporary-test.pem")
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}), 0600); err != nil {
		t.Fatal(err)
	}
	signer, err := evt.NewAttestationSignerFromPEMPath(path)
	if err != nil {
		t.Fatal(err)
	}

	principal := uuid.NewString()
	if _, err := s.db.Exec(`INSERT INTO domain_users(id,kratos_identity_id,role,status) VALUES($1::uuid,$1::uuid::text,'hr_reviewer','active')`, principal); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO evt_employer_config(employer_id) VALUES('employer-b')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO evt_requests(request_id,candidate_personid,employer_id,status,claim_snapshot) VALUES('00000000-0000-4000-8000-00000000000c',$1,'employer-b','ATTESTATION_PENDING','{"subject":"synthetic-only"}')`, principal); err != nil {
		t.Fatal(err)
	}

	d := &evtdb.DB{Pool: pool}
	m := &evt.Module{Employer: &evt.EmployerHandlers{DB: d, Repo: &evt.Repo{DB: d, Signer: signer}}}
	engine := gin.New()
	engine.Use(auth.GinRequireAuth(s.cfg.Auth.JWTSecret, s.cfg.Auth.Issuer, s.cfg.Auth.Audience))
	m.Register(engine.Group("/v1"))
	grantEmployer(t, pool, principal, "employer-b")
	raw, err := auth.MintAccessTokenHS256(s.cfg.Auth.JWTSecret, s.cfg.Auth.Issuer, s.cfg.Auth.Audience, principal, "", []auth.Role{auth.RoleHRReviewer}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ method, path, body string }{
		{"GET", "/v1/employer/requests?employer_id=employer-b", ""},
		{"GET", "/v1/employer/requests/00000000-0000-4000-8000-00000000000c?employer_id=employer-b", ""},
		{"POST", "/v1/employer/requests/00000000-0000-4000-8000-00000000000c/attest", `{"employer_id":"employer-b","response_type":"FULL_MATCH","response_body":{"claim":{"value":{"title":"Engineer"}}}}`},
	} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		r.Header.Set("Authorization", "Bearer "+raw)
		r.Header.Set("Content-Type", "application/json")
		engine.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("assigned endpoint %s: %d %s", tc.path, w.Code, w.Body.String())
		}
		t.Logf("assigned HR principal received HTTP 200 for %s %s", tc.method, tc.path)
	}
	var compact string
	if err := s.db.QueryRow(`SELECT attestation_jws FROM evt_requests WHERE request_id='00000000-0000-4000-8000-00000000000c'`).Scan(&compact); err != nil {
		t.Fatal(err)
	}
	enc := base64.RawURLEncoding
	jwks, _ := json.Marshal(map[string]any{"keys": []map[string]string{{"kty": "RSA", "kid": "weekend2-temporary-test", "alg": "RS256", "n": enc.EncodeToString(key.N.Bytes()), "e": enc.EncodeToString([]byte{1, 0, 1})}}})
	resolver, err := legacyjws.NewStaticJWKSResolver(jwks)
	if err != nil {
		t.Fatal(err)
	}
	profile := legacyjws.New(resolver, nil)
	wrongN := new(big.Int).Add(key.N, big.NewInt(2))
	wrongJWKS, _ := json.Marshal(map[string]any{"keys": []map[string]string{{"kty": "RSA", "kid": "weekend2-temporary-test", "alg": "RS256", "n": enc.EncodeToString(wrongN.Bytes()), "e": enc.EncodeToString([]byte{1, 0, 1})}}})
	wrongKeyResolver, err := legacyjws.NewStaticJWKSResolver(wrongJWKS)
	if err != nil {
		t.Fatal(err)
	}
	wrongEvidence, err := legacyjws.New(wrongKeyResolver, nil).VerifyAndNormalize(context.Background(), []byte(compact), credential.VerificationOptions{})
	if err != nil || wrongEvidence.Cryptographic.Signature != credential.EvidenceInvalid {
		t.Fatal("wrong public key accepted")
	}

	evidence, err := profile.VerifyAndNormalize(context.Background(), []byte(compact), credential.VerificationOptions{})
	if err != nil || evidence.Cryptographic.Signature != credential.EvidenceValid {
		t.Fatalf("local signature verification: %v %+v", err, evidence.Cryptographic)
	}
	if evidence.Status.Result != credential.EvidenceUnknown || evidence.Validity.Result != credential.EvidenceUnknown {
		t.Fatal("legacy envelope invented status or temporal validity")
	}
	// Byte-slice ResponseBody is base64 in the existing signed wire payload;
	// record the normalization limitation without changing the frozen schema.
	found := false
	for _, e := range evidence.Errors {
		if e.Code == "LEGACY_RESPONSE_BODY_INVALID" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected existing legacy response-body normalization limitation")
	}
	parts := strings.Split(compact, ".")
	parts[1] = enc.EncodeToString([]byte(`{"RequestID":"tampered","EmployerID":"employer-b"}`))
	evidence, err = profile.VerifyAndNormalize(context.Background(), []byte(strings.Join(parts, ".")), credential.VerificationOptions{})
	if err != nil || evidence.Cryptographic.Signature != credential.EvidenceInvalid {
		t.Fatal("tampered payload accepted")
	}
	emptyJWKS := strings.Replace(string(jwks), "weekend2-temporary-test", "different-key", 1)
	wrongResolver, err := legacyjws.NewStaticJWKSResolver([]byte(emptyJWKS))
	if err != nil {
		t.Fatal(err)
	}
	evidence, err = legacyjws.New(wrongResolver, nil).VerifyAndNormalize(context.Background(), []byte(compact), credential.VerificationOptions{})
	if err != nil || evidence.Cryptographic.Signature != credential.EvidenceInvalid {
		t.Fatal("unknown kid accepted")
	}
	t.Log("LOCAL ONLY: HTTP issuance -> temporary PEM signer -> database JWS -> static JWKS signature verification; trust/status/liveness not established")
}
