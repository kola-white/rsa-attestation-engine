package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/auth"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type memoryEvidence struct {
	mu                        sync.Mutex
	objects                   map[string][]byte
	mime                      string
	fail                      bool
	readStarted, readContinue chan struct{}
}

func (m *memoryEvidence) PresignPut(_ context.Context, key, mime string, _ time.Duration) (string, map[string]string, error) {
	return "https://storage.invalid/" + key, map[string]string{"Content-Type": mime}, nil
}
func (m *memoryEvidence) ReadEvidence(ctx context.Context, key string, limit int64) ([]byte, string, error) {
	if m.readStarted != nil {
		close(m.readStarted)
		select {
		case <-m.readContinue:
		case <-ctx.Done():
			return nil, "", ctx.Err()
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	data, ok := m.objects[key]
	if !ok || int64(len(data)) > limit {
		return nil, "", errors.New("missing or oversized")
	}
	return append([]byte(nil), data...), m.mime, nil
}
func (m *memoryEvidence) SealEvidence(_ context.Context, key, mime string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail {
		return errors.New("unavailable")
	}
	m.objects[key] = append([]byte(nil), data...)
	return nil
}
func completionInit(t *testing.T, f evidenceFixture, m *memoryEvidence) EvidenceInitResponse {
	t.Helper()
	f.s.s3 = m
	w := f.request(f.principal, "case-a", "employment.company_and_dates", false, evidenceFile)
	evidenceStatus(t, w, 200)
	var response EvidenceInitResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	m.objects[response.Uploads[0].StorageKey] = []byte("a")
	return response
}
func completionRequest(f evidenceFixture, principal string, init EvidenceInitResponse, path string) *httptest.ResponseRecorder {
	body := fmt.Sprintf(`{"uploadSessionId":%q,"uploads":[{"uploadId":%q,"size":1,"mimeType":"application/pdf"}]}`, init.SessionID, init.Uploads[0].UploadID)
	return completionBody(f, principal, path, body)
}
func completionBody(f evidenceFixture, principal, path, body string) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	f.s.registerEvidenceRoutes(mux)
	token, err := auth.MintAccessTokenHS256(f.s.cfg.Auth.JWTSecret, f.s.cfg.Auth.Issuer, f.s.cfg.Auth.Audience, principal, "", []auth.Role{auth.RoleRequestor}, time.Minute)
	if err != nil {
		panic(err)
	}
	r := httptest.NewRequest("POST", path, strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	return w
}
func TestEvidenceCompletionSealsBytesAndRetries(t *testing.T) {
	f := evidenceSetup(t)
	m := &memoryEvidence{objects: map[string][]byte{}, mime: "application/pdf"}
	init := completionInit(t, f, m)
	for _, path := range []string{"/v1/evidence/commit", "/v1/cases/case-a/checks/employment.company_and_dates/evidence:complete"} {
		evidenceStatus(t, completionRequest(f, f.principal, init, path), 200)
	}
	var key, hash, status string
	if err := f.pool.QueryRow(context.Background(), `SELECT sealed_key,evidence_hash,j.scan_status FROM evidence_uploads u JOIN evidence_scan_jobs j USING(upload_id) WHERE u.upload_id=$1`, init.Uploads[0].UploadID).Scan(&key, &hash, &status); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte("a"))
	if key == init.Uploads[0].StorageKey || hash != hex.EncodeToString(sum[:]) || status != "PENDING" {
		t.Fatal("unsealed or unhashed evidence", status)
	}
	m.objects[init.Uploads[0].StorageKey] = []byte("b")
	evidenceStatus(t, completionRequest(f, f.principal, init, "/v1/evidence/commit"), 200)
	if string(m.objects[key]) != "a" {
		t.Fatal("staging overwrite changed accepted evidence")
	}
	var count int
	f.pool.QueryRow(context.Background(), `SELECT count(*) FROM evidence_scan_jobs`).Scan(&count)
	if count != 1 {
		t.Fatal("duplicate scan job", count)
	}
}
func TestEvidenceCompletionAuthorizationAndExpiry(t *testing.T) {
	for _, condition := range []string{"other-principal", "wrong-path", "revoked", "closed", "inactive", "expired", "foreign-upload"} {
		t.Run(condition, func(t *testing.T) {
			f := evidenceSetup(t)
			m := &memoryEvidence{objects: map[string][]byte{}, mime: "application/pdf"}
			init := completionInit(t, f, m)
			principal, path, want := f.principal, "/v1/evidence/commit", 404
			switch condition {
			case "other-principal":
				principal = f.other
			case "wrong-path":
				path = "/v1/cases/case-b/checks/employment.company_and_dates/evidence:complete"
			case "revoked":
				f.admin(t, `SELECT revoke_evidence_upload($1,'fixture','revoked')`, f.grant)
			case "closed":
				f.admin(t, `SELECT set_evidence_resource_state('case-a',NULL,'CLOSED','fixture','closed')`)
			case "inactive":
				evidenceExec(t, f.pool, `UPDATE domain_users SET status='disabled' WHERE id=$1`, f.principal)
			case "expired":
				evidenceExec(t, f.pool, `UPDATE upload_sessions SET created_at=clock_timestamp()-interval '1 hour',expires_at=clock_timestamp()-interval '1 minute'`)
				want = 409
			case "foreign-upload":
				init.Uploads[0].UploadID = f.other
			}
			evidenceStatus(t, completionRequest(f, principal, init, path), want)
			var count int
			f.pool.QueryRow(context.Background(), `SELECT count(*) FROM evidence_scan_jobs`).Scan(&count)
			if count != 0 {
				t.Fatal("denied evidence queued")
			}
		})
	}
}
func TestEvidenceCompletionStorageValidation(t *testing.T) {
	for _, condition := range []string{"missing", "size", "mime", "oversized", "seal-failure", "hash"} {
		t.Run(condition, func(t *testing.T) {
			f := evidenceSetup(t)
			m := &memoryEvidence{objects: map[string][]byte{}, mime: "application/pdf"}
			init := completionInit(t, f, m)
			want := 409
			switch condition {
			case "missing":
				delete(m.objects, init.Uploads[0].StorageKey)
			case "size":
				m.objects[init.Uploads[0].StorageKey] = []byte("ab")
			case "mime":
				m.mime = "image/png"
			case "oversized":
				m.objects[init.Uploads[0].StorageKey] = make([]byte, f.s.policy.MaxBytes+1)
			case "seal-failure":
				m.fail = true
				want = 503
			}
			var w *httptest.ResponseRecorder
			if condition == "hash" {
				w = completionBody(f, f.principal, "/v1/evidence/commit", fmt.Sprintf(`{"uploadSessionId":%q,"uploads":[{"uploadId":%q,"size":1,"mimeType":"application/pdf","sha256":%q}]}`, init.SessionID, init.Uploads[0].UploadID, strings.Repeat("0", 64)))
			} else {
				w = completionRequest(f, f.principal, init, "/v1/evidence/commit")
			}
			evidenceStatus(t, w, want)
			var count int
			f.pool.QueryRow(context.Background(), `SELECT count(*) FROM evidence_uploads WHERE status='RECEIVED'`).Scan(&count)
			if count != 0 {
				t.Fatal("invalid object accepted")
			}
		})
	}
}
func TestEvidenceCompletionDedupAndReservations(t *testing.T) {
	f := evidenceSetup(t)
	m := &memoryEvidence{objects: map[string][]byte{}, mime: "application/pdf"}
	first := completionInit(t, f, m)
	second := completionInit(t, f, m)
	evidenceStatus(t, completionRequest(f, f.principal, first, "/v1/evidence/commit"), 200)
	evidenceStatus(t, completionRequest(f, f.principal, second, "/v1/evidence/commit"), 409)
	completionInit(t, f, m)
	evidenceStatus(t, f.request(f.principal, "case-a", "employment.company_and_dates", false, evidenceFile), 409)
}
func TestEvidenceCompletionRevocationWaitsForInFlightAcceptance(t *testing.T) {
	f := evidenceSetup(t)
	m := &memoryEvidence{objects: map[string][]byte{}, mime: "application/pdf", readStarted: make(chan struct{}), readContinue: make(chan struct{})}
	init := completionInit(t, f, m)
	result := make(chan *httptest.ResponseRecorder, 1)
	go func() { result <- completionRequest(f, f.principal, init, "/v1/evidence/commit") }()
	select {
	case <-m.readStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("completion never read")
	}
	revoke := make(chan error, 1)
	go func() {
		_, err := f.pool.Exec(context.Background(), `SELECT revoke_evidence_upload($1,'fixture','revocation')`, f.grant)
		revoke <- err
	}()
	select {
	case err := <-revoke:
		t.Fatalf("revocation escaped authorization lock: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(m.readContinue)
	select {
	case w := <-result:
		evidenceStatus(t, w, 200)
	case <-time.After(5 * time.Second):
		t.Fatal("completion stalled")
	}
	select {
	case err := <-revoke:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("revocation stalled")
	}
	evidenceStatus(t, completionRequest(f, f.principal, init, "/v1/evidence/commit"), 404)
}

func TestEvidenceCompletionConcurrentReservations(t *testing.T) {
	f := evidenceSetup(t)
	// Every connection starts under the runtime role; avoid accidentally testing
	// serial behavior caused by the single-connection legacy fixture.
	cfg, err := pgx.ParseConfig(f.pool.Config().ConnString())
	if err != nil {
		t.Fatal(err)
	}
	cfg.RuntimeParams["role"] = f.runtime
	db := stdlib.OpenDB(*cfg)
	defer db.Close()
	db.SetMaxOpenConns(6)
	f.s.db = db
	f.s.s3 = &memoryEvidence{objects: map[string][]byte{}, mime: "application/pdf"}
	results := make(chan int, 6)
	for i := 0; i < 6; i++ {
		go func() {
			results <- f.request(f.principal, "case-a", "employment.company_and_dates", false, evidenceFile).Code
		}()
	}
	accepted, denied := 0, 0
	for i := 0; i < 6; i++ {
		select {
		case code := <-results:
			switch code {
			case 200:
				accepted++
			case 409:
				denied++
			default:
				t.Fatal("unexpected result", code)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("reservation stalled")
		}
	}
	if accepted != 3 || denied != 3 {
		t.Fatal("quota oversubscribed", accepted, denied)
	}
	evidenceExec(t, f.pool, `UPDATE upload_sessions SET created_at=clock_timestamp()-interval '1 hour',expires_at=clock_timestamp()-interval '1 minute'`)
	evidenceStatus(t, f.request(f.principal, "case-a", "employment.company_and_dates", false, evidenceFile), 200)

}

func TestEvidenceCompletionPrivilegesAndMigration(t *testing.T) {
	f := evidenceSetup(t)
	for _, query := range []string{
		`UPDATE cases SET status='OPEN'`,
		`INSERT INTO case_evidence_upload_grants(principal_id,case_id) VALUES(NULL,'case-a')`,
		`UPDATE evidence_scan_jobs SET scan_status='CLEAN'`,
		`DELETE FROM evidence_uploads`,
		`UPDATE upload_sessions SET principal_id=gen_random_uuid()`,
		`UPDATE evidence_uploads SET staging_key='forged'`,
	} {
		_, err := f.s.db.Exec(query)
		expectEmployerSQLState(t, err, "42501")
	}
	var readable bool
	if err := f.pool.QueryRow(context.Background(), `SELECT has_table_privilege($1,'upload_sessions','SELECT')`, f.operator).Scan(&readable); err != nil || readable {
		t.Fatal("operator received lifecycle data access", readable, err)
	}
	// Once-only migration rejects reruns instead of hiding a mismatched schema.
	_, err := f.pool.Exec(context.Background(), employerSQL(t, "../../migrations/000016_evidence_completion.sql"))
	expectEmployerSQLState(t, err, "42P07")
	_, pool := securityTestServer(t)
	evidenceExec(t, pool, employerSQL(t, "../../migrations/000015_case_evidence_authorization.sql"))
	tx, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(context.Background(), employerSQL(t, "../../migrations/000016_evidence_completion.sql")); err != nil {
		t.Fatal(err)
	}
	if err = tx.Rollback(context.Background()); err != nil {
		t.Fatal(err)
	}
	var absent bool
	if err = pool.QueryRow(context.Background(), `SELECT to_regclass(current_schema()||'.upload_sessions') IS NULL`).Scan(&absent); err != nil || !absent {
		t.Fatal("rollback left lifecycle schema", absent, err)
	}
}

func TestEvidenceLifecycleFailsClosedWithoutPersistenceAndIgnoresTempShadow(t *testing.T) {
	f := evidenceSetup(t)
	evidenceExec(t, f.pool, `SELECT 1`)
	// TEMP is permitted in production; it must not shadow the actual tables.
	if _, err := f.s.db.Exec(`CREATE TEMP TABLE upload_sessions(fake text)`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.db.Exec(`CREATE TEMP TABLE evidence_uploads(fake text)`); err != nil {
		t.Fatal(err)
	}
	evidenceStatus(t, f.request(f.principal, "case-a", "employment.company_and_dates", false, evidenceFile), 200)
	evidenceExec(t, f.pool, `DROP TABLE evidence_scan_jobs,evidence_uploads,upload_sessions`)
	evidenceStatus(t, f.request(f.principal, "case-a", "employment.company_and_dates", true, evidenceFile), 503)
}

func TestEvidenceInitPathIdentifiersAndBatchAtomicity(t *testing.T) {
	f := evidenceSetup(t)
	m := &memoryEvidence{objects: map[string][]byte{}, mime: "application/pdf"}
	f.s.s3 = m
	w := completionBody(f, f.principal, "/v1/cases/case-a/checks/employment.company_and_dates/evidence:init", `{"files":[{"name":"a.pdf","mimeType":"application/pdf","size":1},{"name":"b.pdf","mimeType":"application/pdf","size":1}]}`)
	evidenceStatus(t, w, 200)
	var init EvidenceInitResponse
	if err := json.Unmarshal(w.Body.Bytes(), &init); err != nil {
		t.Fatal(err)
	}
	// Completing only one file cannot partially accept the session.
	evidenceStatus(t, completionRequest(f, f.principal, init, "/v1/evidence/commit"), 400)
	m.objects[init.Uploads[0].StorageKey] = []byte("a")
	m.objects[init.Uploads[1].StorageKey] = []byte("a")
	body := fmt.Sprintf(`{"uploadSessionId":%q,"uploads":[{"uploadId":%q,"size":1,"mimeType":"application/pdf"},{"uploadId":%q,"size":1,"mimeType":"application/pdf"}]}`, init.SessionID, init.Uploads[0].UploadID, init.Uploads[1].UploadID)
	evidenceStatus(t, completionBody(f, f.principal, "/v1/evidence/commit", body), 409)
	var count int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM evidence_uploads WHERE status='RECEIVED'`).Scan(&count); err != nil || count != 0 {
		t.Fatal("partial batch acceptance", count, err)
	}
	m.objects[init.Uploads[1].StorageKey] = []byte("b")
	evidenceStatus(t, completionBody(f, f.principal, "/v1/evidence/commit", body), 200)
}
