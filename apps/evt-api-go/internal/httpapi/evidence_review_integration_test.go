package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/auth"
)

func reviewSetup(t *testing.T) (evidenceFixture, *memoryEvidence, EvidenceInitResponse) {
	t.Helper()
	f := evidenceSetup(t)
	evidenceExec(t, f.pool, employerSQL(t, "../../migrations/000017_evidence_review.sql"))
	tx, err := f.pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(context.Background(), `SELECT set_config('cvera.evidence_runtime_role',$1,true),set_config('cvera.evidence_operator_role',$2,true)`, f.runtime, f.operator); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(context.Background(), employerSQL(t, "../../operations/evidence-review-privileges.sql")); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	m := &memoryEvidence{objects: map[string][]byte{}, mime: "application/pdf"}
	init := completionInit(t, f, m)
	evidenceStatus(t, completionRequest(f, f.principal, init, "/v1/evidence/commit"), 200)
	return f, m, init
}
func reviewRequest(f evidenceFixture, principal, method, path, body string) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	f.s.registerEvidenceRoutes(mux)
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if principal != "" {
		token, err := auth.MintAccessTokenHS256(f.s.cfg.Auth.JWTSecret, f.s.cfg.Auth.Issuer, f.s.cfg.Auth.Audience, principal, "", []auth.Role{auth.RoleHRReviewer}, time.Minute)
		if err != nil {
			panic(err)
		}
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	return w
}
func reviewPaths(init EvidenceInitResponse) (string, string, string) {
	base := "/v1/cases/case-a/checks/employment.company_and_dates/evidence"
	return base, base + "/" + init.Uploads[0].UploadID + "/download", base + "/" + init.Uploads[0].UploadID + "/review"
}
func TestEvidenceReviewExplicitGrantAndPrivateDownload(t *testing.T) {
	f, _, init := reviewSetup(t)
	list, download, review := reviewPaths(init)
	for _, principal := range []string{"", f.principal, f.other} {
		want := 404
		if principal == "" {
			want = 401
		}
		for _, path := range []string{list, download} {
			evidenceStatus(t, reviewRequest(f, principal, "GET", path, ""), want)
		}
		evidenceStatus(t, reviewRequest(f, principal, "POST", review, `{"decision":"APPROVED","reason":"checked"}`), want)
	}
	// Reviewer role alone and uploader ownership/permission cannot read evidence.
	f.adminString(t, `SELECT grant_evidence_review($1,'case-a','fixture','review approval')`, f.other)
	w := reviewRequest(f, f.other, "GET", list, "")
	evidenceStatus(t, w, 200)
	if strings.Contains(w.Body.String(), "sealed-evidence/") || strings.Contains(w.Body.String(), "storageKey") {
		t.Fatal("metadata leaked storage capability")
	}
	var body struct {
		Evidence []struct {
			ReviewStatus, ScanStatus string
			CanReview                bool
		}
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || len(body.Evidence) != 1 || body.Evidence[0].ReviewStatus != "PENDING" || body.Evidence[0].ScanStatus != "PENDING" || !body.Evidence[0].CanReview {
		t.Fatal("incorrect pending metadata", err, w.Body.String())
	}
	w = reviewRequest(f, f.other, "GET", download, "")
	evidenceStatus(t, w, 200)
	if w.Body.String() != "a" || w.Header().Get("Content-Type") != "application/octet-stream" || !strings.HasPrefix(w.Header().Get("Content-Disposition"), "attachment;") || w.Header().Get("Cache-Control") != "private, no-store" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("unsafe or incorrect attachment response")
	}
	evidenceStatus(t, f.request(f.other, "case-a", "employment.company_and_dates", false, evidenceFile), 404) // Read grant never grants upload.
	evidenceStatus(t, reviewRequest(f, f.other, "GET", strings.Replace(download, "case-a", "case-b", 1), ""), 404)
	evidenceStatus(t, reviewRequest(f, f.other, "GET", strings.Replace(download, init.Uploads[0].UploadID, uuid.NewString(), 1), ""), 404)
}
func TestEvidenceManualReviewAttributionAndNoSelfReview(t *testing.T) {
	f, _, init := reviewSetup(t)
	_, _, review := reviewPaths(init)
	f.adminString(t, `SELECT grant_evidence_review($1,'case-a','fixture','review approval')`, f.principal)
	evidenceStatus(t, reviewRequest(f, f.principal, "POST", review, `{"decision":"APPROVED","reason":"own upload"}`), 403)
	evidenceStatus(t, reviewRequest(f, f.principal, "POST", review, `{"decision":"REJECTED","reason":"own upload"}`), 403)
	f.adminString(t, `SELECT grant_evidence_review($1,'case-a','fixture','independent reviewer')`, f.other)
	body := fmt.Sprintf(`{"decision":"APPROVED","reason":"Reviewed source material","reviewerId":%q,"scanStatus":"CLEAN"}`, f.principal)
	first := reviewRequest(f, f.other, "POST", review, body)
	evidenceStatus(t, first, 200)
	again := reviewRequest(f, f.other, "POST", review, body)
	evidenceStatus(t, again, 200)
	if again.Body.String() != first.Body.String() {
		t.Fatal("retry created another review")
	}
	var principal, scan, decision string
	var count int
	err := f.pool.QueryRow(context.Background(), `SELECT reviewer_id,decision,(SELECT scan_status FROM evidence_scan_jobs WHERE upload_id=$1),(SELECT count(*) FROM evidence_reviews) FROM evidence_reviews WHERE upload_id=$1`, init.Uploads[0].UploadID).Scan(&principal, &decision, &scan, &count)
	if err != nil || principal != f.other || scan != "PENDING" || decision != "APPROVED" || count != 1 {
		t.Fatal("review attribution/trust boundary", err, principal, scan, decision, count)
	}
	evidenceStatus(t, reviewRequest(f, f.other, "POST", review, `{"decision":"REJECTED","reason":"silently overwrite"}`), 409)
	for _, body := range []string{`{"decision":"CLEAN","reason":"wrong status"}`, `{"decision":"APPROVED","reason":" "}`, `{"decision":"APPROVED","reason":"` + strings.Repeat("x", 1001) + `"}`} {
		evidenceStatus(t, reviewRequest(f, f.other, "POST", review, body), 400)
	}
}
func TestEvidenceReviewRevocationStatesAndIntegrity(t *testing.T) {
	for _, condition := range []string{"revoked", "inactive", "case-closed", "check-passed", "unsafe-scan", "tampered-sealed", "missing-object"} {
		t.Run(condition, func(t *testing.T) {
			f, m, init := reviewSetup(t)
			_, download, review := reviewPaths(init)
			grant := f.adminString(t, `SELECT grant_evidence_review($1,'case-a','fixture','review approval')`, f.other)
			want := 404
			switch condition {
			case "revoked":
				f.admin(t, `SELECT revoke_evidence_review($1,'fixture','withdrawn')`, grant)
			case "inactive":
				evidenceExec(t, f.pool, `UPDATE domain_users SET status='disabled' WHERE id=$1`, f.other)
			case "case-closed":
				f.admin(t, `SELECT set_evidence_resource_state('case-a',NULL,'CLOSED','fixture','closed')`)
			case "check-passed":
				f.admin(t, `SELECT set_evidence_resource_state('case-a','employment.company_and_dates','PASSED','fixture','finished')`)
			case "unsafe-scan":
				evidenceExec(t, f.pool, `UPDATE evidence_scan_jobs SET scan_status='INFECTED'`)
			case "tampered-sealed", "missing-object":
				var key string
				if err := f.pool.QueryRow(context.Background(), `SELECT sealed_key FROM evidence_uploads WHERE upload_id=$1`, init.Uploads[0].UploadID).Scan(&key); err != nil {
					t.Fatal(err)
				}
				if condition == "tampered-sealed" {
					m.objects[key] = []byte("b")
				} else {
					delete(m.objects, key)
				}
				want = 503
			}
			w := reviewRequest(f, f.other, "GET", download, "")
			evidenceStatus(t, w, want)
			if strings.Contains(w.Body.String(), "sealed-evidence/") {
				t.Fatal("storage error leaked key")
			}
			if condition == "unsafe-scan" {
				evidenceStatus(t, reviewRequest(f, f.other, "POST", review, `{"decision":"APPROVED","reason":"ignore infection"}`), 409)
				evidenceStatus(t, reviewRequest(f, f.other, "POST", review, `{"decision":"REJECTED","reason":"unsafe evidence"}`), 200)
			} else if want == 404 {
				evidenceStatus(t, reviewRequest(f, f.other, "POST", review, `{"decision":"APPROVED","reason":"test"}`), 404)
			}
		})
	}
}
func TestEvidenceScopeDiscoveryUsesExistingAssignments(t *testing.T) {
	f, _, _ := reviewSetup(t)
	parse := func(principal string) []struct {
		CaseID, CheckID      string
		CanUpload, CanReview bool
	} {
		w := reviewRequest(f, principal, "GET", "/v1/evidence/checks", "")
		evidenceStatus(t, w, 200)
		var body struct {
			Items []struct {
				CaseID, CheckID      string
				CanUpload, CanReview bool
			}
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return body.Items
	}
	a := parse(f.principal)
	if len(a) != 1 || a[0].CaseID != "case-a" || !a[0].CanUpload || a[0].CanReview {
		t.Fatal("upload capabilities broadened", a)
	}
	if got := parse(f.other); len(got) != 0 {
		t.Fatal("HR role gave implicit cases", got)
	}
	grant := f.adminString(t, `SELECT grant_evidence_review($1,'case-b','fixture','review')`, f.other)
	b := parse(f.other)
	if len(b) != 1 || b[0].CaseID != "case-b" || b[0].CanUpload || !b[0].CanReview {
		t.Fatal("review scope broadened", b)
	}
	f.admin(t, `SELECT revoke_evidence_review($1,'fixture','withdrawn')`, grant)
	if got := parse(f.other); len(got) != 0 {
		t.Fatal("revoked scope visible", got)
	}
}
func TestEvidenceReviewRuntimeAndOperatorPrivileges(t *testing.T) {
	f, _, _ := reviewSetup(t)
	for _, table := range []string{"case_evidence_review_grants", "evidence_reviews"} {
		for _, role := range []string{f.runtime, f.operator} {
			var has bool
			if err := f.pool.QueryRow(context.Background(), `SELECT has_any_column_privilege($1,$2,'SELECT') OR has_any_column_privilege($1,$2,'INSERT') OR has_any_column_privilege($1,$2,'UPDATE') OR has_table_privilege($1,$2,'DELETE,TRUNCATE')`, role, table).Scan(&has); err != nil || has {
				t.Fatal("unexpected direct access", role, table, err)
			}
		}
	}
	_, err := f.s.db.Exec(`SELECT grant_evidence_review($1,'case-a','forged','forged')`, f.principal)
	expectEmployerSQLState(t, err, "42501")
	_, err = f.s.db.Exec(`SELECT revoke_evidence_review($1,'forged','forged')`, uuid.NewString())
	expectEmployerSQLState(t, err, "42501")
	_, err = f.s.db.Exec(`UPDATE evidence_scan_jobs SET scan_status='CLEAN'`)
	expectEmployerSQLState(t, err, "42501")
	var count int
	if err = f.pool.QueryRow(context.Background(), `SELECT count(*) FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace,LATERAL aclexplode(coalesce(p.proacl,acldefault('f',p.proowner))) a WHERE n.nspname=current_schema() AND p.proname LIKE '%evidence%review%' AND a.grantee=0 AND a.privilege_type='EXECUTE'`).Scan(&count); err != nil || count != 0 {
		t.Fatal("PUBLIC definer access", count, err)
	}
}

// Check both linearization orders against the actual PostgreSQL lock graph.
func TestEvidenceReviewGrantConcurrency(t *testing.T) {
	for _, mutationFirst := range []bool{false, true} {
		t.Run(fmt.Sprint(mutationFirst), func(t *testing.T) {
			f, _, _ := reviewSetup(t)
			grant := f.adminString(t, `SELECT grant_evidence_review($1,'case-a','fixture','review')`, f.other)
			ctx := context.Background()
			read, err := f.pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer read.Rollback(ctx)
			change, err := f.pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer change.Rollback(ctx)
			if _, err = read.Exec(ctx, `SET LOCAL ROLE `+pgx.Identifier{f.runtime}.Sanitize()); err != nil {
				t.Fatal(err)
			}
			if _, err = change.Exec(ctx, `SET LOCAL ROLE `+pgx.Identifier{f.operator}.Sanitize()); err != nil {
				t.Fatal(err)
			}
			query := `SELECT revoke_evidence_review($1,'fixture','withdrawn')`
			type result struct{ err error }
			done := make(chan result, 1)
			if mutationFirst {
				if _, err = change.Exec(ctx, query, grant); err != nil {
					t.Fatal(err)
				}
				go func() {
					var data []byte
					e := read.QueryRow(ctx, `SELECT list_evidence_for_review($1,'case-a','employment.company_and_dates')`, f.other).Scan(&data)
					done <- result{e}
				}()
				waitEmployerBlocked(t, f.pool, change.Conn().PgConn().PID(), read.Conn().PgConn().PID())
				if err = change.Commit(ctx); err != nil {
					t.Fatal(err)
				}
				expectEmployerSQLState(t, (<-done).err, "P0002")
			} else {
				var data []byte
				if err = read.QueryRow(ctx, `SELECT list_evidence_for_review($1,'case-a','employment.company_and_dates')`, f.other).Scan(&data); err != nil {
					t.Fatal(err)
				}
				go func() { _, e := change.Exec(ctx, query, grant); done <- result{e} }()
				waitEmployerBlocked(t, f.pool, read.Conn().PgConn().PID(), change.Conn().PgConn().PID())
				if err = read.Commit(ctx); err != nil {
					t.Fatal(err)
				}
				if r := <-done; r.err != nil {
					t.Fatal(r.err)
				}
				if err = change.Commit(ctx); err != nil {
					t.Fatal(err)
				}
			}
			evidenceStatus(t, reviewRequest(f, f.other, "GET", "/v1/cases/case-a/checks/employment.company_and_dates/evidence", ""), 404)
		})
	}
}

func TestEvidenceReviewFailClosedConfigurationAndHeaders(t *testing.T) {
	for _, condition := range []string{"missing-execute", "rls", "unsafe-filename"} {
		t.Run(condition, func(t *testing.T) {
			f, _, init := reviewSetup(t)
			list, download, _ := reviewPaths(init)
			f.adminString(t, `SELECT grant_evidence_review($1,'case-a','fixture','review')`, f.other)
			switch condition {
			case "missing-execute":
				evidenceExec(t, f.pool, `REVOKE EXECUTE ON FUNCTION list_evidence_for_review(uuid,text,text) FROM `+pgx.Identifier{f.runtime}.Sanitize())
				evidenceStatus(t, reviewRequest(f, f.other, "GET", list, ""), 503)
			case "rls":
				evidenceExec(t, f.pool, `ALTER TABLE evidence_reviews ENABLE ROW LEVEL SECURITY`)
				evidenceStatus(t, reviewRequest(f, f.other, "GET", list, ""), 503)
				evidenceStatus(t, reviewRequest(f, f.other, "GET", download, ""), 503)
			case "unsafe-filename":
				evidenceExec(t, f.pool, `UPDATE evidence_uploads SET file_name=$1 WHERE upload_id=$2`, "../folder\\unsafe\r\nX-Test: value.pdf", init.Uploads[0].UploadID)
				w := reviewRequest(f, f.other, "GET", download, "")
				evidenceStatus(t, w, 200)
				header := w.Header().Get("Content-Disposition")
				if strings.ContainsAny(header, "\r\n") || strings.Contains(header, "../") || w.Header().Get("X-Test") != "" || !strings.HasPrefix(header, "attachment;") {
					t.Fatal("unsafe attachment header")
				}
			}
		})
	}
}
func TestEvidenceReviewConcurrentFinalDecisions(t *testing.T) {
	f, _, init := reviewSetup(t)
	third := uuid.NewString()
	evidenceExec(t, f.pool, `INSERT INTO domain_users(id,kratos_identity_id,role,status) VALUES($1::uuid,$1::uuid::text,'candidate','active')`, third)
	for _, p := range []string{f.other, third} {
		f.adminString(t, `SELECT grant_evidence_review($1,'case-a','fixture','review')`, p)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	first, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Rollback(context.Background())
	second, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Rollback(context.Background())
	for _, tx := range []pgx.Tx{first, second} {
		if _, err = tx.Exec(ctx, `SET LOCAL ROLE `+pgx.Identifier{f.runtime}.Sanitize()); err != nil {
			t.Fatal(err)
		}
	}
	query := `SELECT record_evidence_review($1,'case-a','employment.company_and_dates',$2,$3,'reviewed')`
	var data []byte
	if err = first.QueryRow(ctx, query, f.other, init.Uploads[0].UploadID, "APPROVED").Scan(&data); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		var data []byte
		done <- second.QueryRow(ctx, query, third, init.Uploads[0].UploadID, "REJECTED").Scan(&data)
	}()
	waitEmployerBlocked(t, f.pool, first.Conn().PgConn().PID(), second.Conn().PgConn().PID())
	if err = first.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	expectEmployerSQLState(t, <-done, "23505")
	var count int
	if err = f.pool.QueryRow(ctx, `SELECT count(*) FROM evidence_reviews`).Scan(&count); err != nil || count != 1 {
		t.Fatal("concurrent decision overwrite", count, err)
	}
}

func TestEvidenceReviewMigrationRollbackAndExistingAPICompatibility(t *testing.T) {
	f := evidenceSetup(t)
	ctx := context.Background()
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, employerSQL(t, "../../migrations/000017_evidence_review.sql")); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `SELECT set_config('cvera.evidence_runtime_role',$1,true),set_config('cvera.evidence_operator_role',$2,true)`, f.runtime, "missing_operator_"+uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, employerSQL(t, "../../operations/evidence-review-privileges.sql")); err == nil {
		t.Fatal("missing operator accepted")
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	var absent bool
	if err = f.pool.QueryRow(ctx, `SELECT to_regclass(current_schema()||'.evidence_reviews') IS NULL AND to_regprocedure(current_schema()||'.list_evidence_checks(uuid)') IS NULL`).Scan(&absent); err != nil || !absent {
		t.Fatal("failed binding left partial schema", absent, err)
	}
	evidenceStatus(t, reviewRequest(f, f.principal, "GET", "/v1/evidence/checks", ""), 503)
	evidenceStatus(t, f.request(f.principal, "case-a", "employment.company_and_dates", false, evidenceFile), 200)
}

func TestEvidenceReviewDownloadHoldsAuthorizationThroughStorageRead(t *testing.T) {
	f, m, init := reviewSetup(t)
	_, download, _ := reviewPaths(init)
	grant := f.adminString(t, `SELECT grant_evidence_review($1,'case-a','fixture','review')`, f.other)
	var pid uint32
	if err := f.s.db.QueryRow(`SELECT pg_backend_pid()`).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	m.readStarted = make(chan struct{})
	m.readContinue = make(chan struct{})
	var once sync.Once
	release := func() { once.Do(func() { close(m.readContinue) }) }
	defer release()
	response := make(chan *httptest.ResponseRecorder, 1)
	go func() { response <- reviewRequest(f, f.other, "GET", download, "") }()
	select {
	case <-m.readStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("download did not read sealed storage")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	change, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer change.Rollback(context.Background())
	if _, err = change.Exec(ctx, `SET LOCAL ROLE `+pgx.Identifier{f.operator}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	changed := make(chan error, 1)
	go func() {
		_, err := change.Exec(ctx, `SELECT revoke_evidence_review($1,'fixture','withdrawn')`, grant)
		changed <- err
	}()
	waitEmployerBlocked(t, f.pool, pid, change.Conn().PgConn().PID())
	release()
	select {
	case w := <-response:
		evidenceStatus(t, w, 200)
	case <-ctx.Done():
		t.Fatal("download stalled")
	}
	if err = <-changed; err != nil {
		t.Fatal(err)
	}
	if err = change.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	evidenceStatus(t, reviewRequest(f, f.other, "GET", download, ""), 404)
}
