package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/auth"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/config"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/storage"
)

type evidenceFixture struct {
	s                                          *Server
	pool                                       *pgxpool.Pool
	runtime, operator, principal, other, grant string
}

func evidenceExec(t *testing.T, pool *pgxpool.Pool, query string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), query, args...); err != nil {
		t.Fatal(err)
	}
}
func evidenceSetup(t *testing.T) evidenceFixture {
	t.Helper()
	s, pool := securityTestServer(t)
	runtime, operator := createEmployerProbeRole(t, pool), createEmployerProbeRole(t, pool)
	// Reproduce hostile default ACLs, including normal PUBLIC function EXECUTE.
	var schema string
	if err := pool.QueryRow(context.Background(), `SELECT current_schema()`).Scan(&schema); err != nil {
		t.Fatal(err)
	}
	evidenceExec(t, pool, `ALTER DEFAULT PRIVILEGES IN SCHEMA `+pgx.Identifier{schema}.Sanitize()+` GRANT SELECT,INSERT,UPDATE,DELETE ON TABLES TO `+pgx.Identifier{runtime}.Sanitize())
	evidenceExec(t, pool, employerSQL(t, "../../migrations/000015_case_evidence_authorization.sql"))
	tx, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(context.Background(), `SELECT set_config('cvera.evidence_runtime_role',$1,true),set_config('cvera.evidence_operator_role',$2,true)`, runtime, operator); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(context.Background(), employerSQL(t, "../../operations/evidence-authorization-privileges.sql")); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}

	evidenceExec(t, pool, employerSQL(t, "../../migrations/000016_evidence_completion.sql"))
	completionTx, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer completionTx.Rollback(context.Background())
	if _, err = completionTx.Exec(context.Background(), `SELECT set_config('cvera.evidence_runtime_role',$1,true)`, runtime); err != nil {
		t.Fatal(err)
	}
	if _, err = completionTx.Exec(context.Background(), employerSQL(t, "../../operations/evidence-completion-privileges.sql")); err != nil {
		t.Fatal(err)
	}
	if err = completionTx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	a, b := uuid.NewString(), uuid.NewString()
	evidenceExec(t, pool, `INSERT INTO domain_users(id,kratos_identity_id,role,status) VALUES($1::uuid,$1::uuid::text,'candidate','active'),($2::uuid,$2::uuid::text,'hr_reviewer','active')`, a, b)
	f := evidenceFixture{s: s, pool: pool, runtime: runtime, operator: operator, principal: a, other: b}
	for _, c := range []string{"case-a", "case-b"} {
		f.admin(t, `SELECT provision_evidence_case($1,'fixture-operator','reviewed fixture')`, c)
		f.admin(t, `SELECT provision_evidence_check($1,'employment.company_and_dates','fixture-operator','reviewed fixture')`, c)
	}
	f.grant = f.adminString(t, `SELECT grant_evidence_upload($1,'case-a','fixture-operator','reviewed fixture')`, a)
	// Exercise the HTTP path under the restricted runtime, not its owner.
	runtimeDB, err := sql.Open("pgx", pool.Config().ConnString())
	if err != nil {
		t.Fatal(err)
	}
	runtimeDB.SetMaxOpenConns(1)
	t.Cleanup(func() { runtimeDB.Close() })
	if _, err = runtimeDB.Exec(`SET ROLE ` + pgx.Identifier{runtime}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	s.db = runtimeDB
	s.s3, err = storage.NewS3Store(context.Background(), config.SpacesConfig{AccessKey: "synthetic-test-key", SecretKey: "synthetic-test-secret", Region: "sfo3", Endpoint: "https://storage.example.invalid", Bucket: "synthetic-test-bucket"})
	if err != nil {
		t.Fatal(err)
	}
	s.policy = storage.DefaultEvidencePolicy()
	return f
}
func (f evidenceFixture) adminString(t *testing.T, query string, args ...any) string {
	t.Helper()
	ctx := context.Background()
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SET LOCAL ROLE `+pgx.Identifier{f.operator}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	var result string
	if err = tx.QueryRow(ctx, query, args...).Scan(&result); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return result
}
func (f evidenceFixture) admin(t *testing.T, query string, args ...any) {
	t.Helper()
	ctx := context.Background()
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SET LOCAL ROLE `+pgx.Identifier{f.operator}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, query, args...); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}
func (f evidenceFixture) request(principal, c, check string, legacy bool, files string) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	f.s.registerEvidenceRoutes(mux)
	path := "/v1/cases/" + c + "/checks/" + check + "/evidence:init"
	if legacy {
		path = "/v1/evidence/init"
	}
	body := fmt.Sprintf(`{"caseId":%q,"checkId":%q,"principalId":%q,"files":%s}`, c, check, f.principal, files)
	token, err := auth.MintAccessTokenHS256(f.s.cfg.Auth.JWTSecret, f.s.cfg.Auth.Issuer, f.s.cfg.Auth.Audience, principal, "", []auth.Role{auth.RoleHRReviewer}, time.Minute)
	if err != nil {
		panic(err)
	}
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	return w
}

const evidenceFile = `[{"name":"synthetic.pdf","mimeType":"application/pdf","size":1}]`

func evidenceStatus(t *testing.T, w *httptest.ResponseRecorder, want int) {
	t.Helper()
	if w.Code != want {
		t.Fatalf("want %d, got %d: %s", want, w.Code, w.Body.String())
	}
	if want != 200 && (strings.Contains(w.Body.String(), "uploads") || strings.Contains(w.Body.String(), "X-Amz")) {
		t.Fatal("denial leaked a capability")
	}
}
func TestEvidenceAuthorizationBothRoutes(t *testing.T) {
	f := evidenceSetup(t)
	for _, legacy := range []bool{false, true} {
		w := f.request(f.principal, "case-a", "employment.company_and_dates", legacy, evidenceFile)
		evidenceStatus(t, w, 200)
		var body EvidenceInitResponse
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if len(body.Uploads) != 1 || !strings.Contains(body.Uploads[0].URL, "X-Amz-Expires=600") || !strings.HasPrefix(body.Uploads[0].StorageKey, "cases/case-a/checks/employment.company_and_dates/evidence/") {
			t.Fatal("upload contract changed")
		}
		for _, tc := range []struct{ principal, c, check string }{
			{f.principal, "case-b", "employment.company_and_dates"}, // existing foreign case
			{f.other, "case-a", "employment.company_and_dates"},     // HR role and body principal assertion give no access
			{uuid.NewString(), "case-a", "employment.company_and_dates"},
			{f.principal, "missing", "employment.company_and_dates"},
			{f.principal, "case-a", "foreign-check"},
		} {
			evidenceStatus(t, f.request(tc.principal, tc.c, tc.check, legacy, evidenceFile), 404)
		}
	}
	// A check existing only under B does not become a member of granted case A.
	f.admin(t, `SELECT provision_evidence_check('case-b','only-b','fixture','approval')`)
	evidenceStatus(t, f.request(f.principal, "case-a", "only-b", false, evidenceFile), 404)
	// No implicit grant creation for the other principal.
	var count int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM case_evidence_upload_grants`).Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
}
func TestEvidenceRevocationStatesAndFilePolicy(t *testing.T) {
	f := evidenceSetup(t)
	for _, state := range []string{"CLOSED", "DELETION_SCHEDULED", "DELETED", "OPEN"} {
		f.admin(t, `SELECT set_evidence_resource_state('case-a',NULL,$1,'fixture','state approval')`, state)
		want := 404
		if state == "OPEN" {
			want = 200
		}
		evidenceStatus(t, f.request(f.principal, "case-a", "employment.company_and_dates", false, evidenceFile), want)
	}
	for _, state := range []string{"PASSED", "FAILED", "DELETED", "IN_REVIEW", "PENDING"} {
		f.admin(t, `SELECT set_evidence_resource_state('case-a','employment.company_and_dates',$1,'fixture','state approval')`, state)
		want := 404
		if state == "IN_REVIEW" || state == "PENDING" {
			want = 200
		}
		evidenceStatus(t, f.request(f.principal, "case-a", "employment.company_and_dates", true, evidenceFile), want)
	}
	evidenceExec(t, f.pool, `UPDATE domain_users SET status='disabled' WHERE id=$1`, f.principal)
	evidenceStatus(t, f.request(f.principal, "case-a", "employment.company_and_dates", false, evidenceFile), 404)
	evidenceExec(t, f.pool, `UPDATE domain_users SET status='active' WHERE id=$1`, f.principal)
	for _, files := range []string{`[]`, `[{"name":"x.exe","mimeType":"application/x-msdownload","size":1}]`, `[{"name":"x.pdf","mimeType":"application/pdf","size":0}]`, `[{"name":"x.pdf","mimeType":"application/pdf","size":5242881}]`, "[" + strings.Trim(evidenceFile, "[]") + "," + strings.Trim(evidenceFile, "[]") + "," + strings.Trim(evidenceFile, "[]") + "," + strings.Trim(evidenceFile, "[]") + "]"} {
		evidenceStatus(t, f.request(f.principal, "case-a", "employment.company_and_dates", false, files), 400)
	}
	f.admin(t, `SELECT revoke_evidence_upload($1,'fixture-revoker','approval withdrawn')`, f.grant)
	for _, legacy := range []bool{false, true} {
		evidenceStatus(t, f.request(f.principal, "case-a", "employment.company_and_dates", legacy, evidenceFile), 404)
	}
	evidenceExec(t, f.pool, `UPDATE upload_sessions SET created_at=clock_timestamp()-interval '1 hour',expires_at=clock_timestamp()-interval '1 minute'`)
	// Regrant is a new attributable record, never reactivation of the revoked row.
	newGrant := f.adminString(t, `SELECT grant_evidence_upload($1,'case-a','fixture','new approval')`, f.principal)
	if newGrant == f.grant {
		t.Fatal("reused revoked grant")
	}
	evidenceStatus(t, f.request(f.principal, "case-a", "employment.company_and_dates", false, evidenceFile), 200)
}

func TestEvidenceRuntimeAndPublicPrivileges(t *testing.T) {
	f := evidenceSetup(t)
	ctx := context.Background()
	for _, role := range []string{f.runtime, f.operator} {
		for _, table := range []string{"cases", "verification_checks", "case_evidence_upload_grants"} {
			var allowed bool
			if err := f.pool.QueryRow(ctx, `SELECT has_any_column_privilege($1,$2,'SELECT') OR has_any_column_privilege($1,$2,'INSERT') OR has_any_column_privilege($1,$2,'UPDATE') OR has_table_privilege($1,$2,'DELETE,TRUNCATE,TRIGGER,REFERENCES')`, role, table).Scan(&allowed); err != nil || allowed {
				t.Fatal("direct registry access", role, table, allowed, err)
			}
		}
	}
	// Actual SQL denials, not ACL metadata alone.
	for _, query := range []string{`SELECT * FROM cases`, `INSERT INTO cases(case_id) VALUES('unauthorized')`, `UPDATE verification_checks SET status='PASSED'`, `DELETE FROM case_evidence_upload_grants`, `SELECT provision_evidence_case('x','x','x')`, `SELECT revoke_evidence_upload('00000000-0000-4000-8000-000000000001','x','x')`, `SELECT grant_evidence_upload('00000000-0000-4000-8000-000000000001','x','x','x')`} {
		_, err := f.s.db.Exec(query)
		expectEmployerSQLState(t, err, "42501")
	}
	var publicCount int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM pg_proc p,LATERAL aclexplode(coalesce(p.proacl,acldefault('f',p.proowner))) a WHERE p.pronamespace=current_schema()::regnamespace AND p.proname IN ('check_evidence_upload','provision_evidence_case','provision_evidence_check','grant_evidence_upload','revoke_evidence_upload','set_evidence_resource_state') AND a.grantee=0`).Scan(&publicCount); err != nil || publicCount != 0 {
		t.Fatal(publicCount, err)
	}
	// Missing/unavailable checker must fail closed, without a storage capability.
	evidenceExec(t, f.pool, `REVOKE EXECUTE ON FUNCTION check_evidence_upload(uuid,text,text) FROM `+pgx.Identifier{f.runtime}.Sanitize())
	evidenceStatus(t, f.request(f.principal, "case-a", "employment.company_and_dates", false, evidenceFile), 503)
}

func TestEvidenceAuthorizationMutationConcurrency(t *testing.T) {
	for _, mutation := range []string{"revoke", "case", "check", "principal"} {
		for _, mutationFirst := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/mutation_first_%t", mutation, mutationFirst), func(t *testing.T) {
				f := evidenceSetup(t)
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				authorize, err := f.pool.Begin(ctx)
				if err != nil {
					t.Fatal(err)
				}
				defer authorize.Rollback(context.Background())
				change, err := f.pool.Begin(ctx)
				if err != nil {
					t.Fatal(err)
				}
				defer change.Rollback(context.Background())
				if _, err = authorize.Exec(ctx, `SET LOCAL ROLE `+pgx.Identifier{f.runtime}.Sanitize()); err != nil {
					t.Fatal(err)
				}
				var query string
				var args []any
				switch mutation {
				case "revoke":
					query = `SELECT revoke_evidence_upload($1,'fixture','withdrawn')`
					args = []any{f.grant}
				case "case":
					query = `SELECT set_evidence_resource_state('case-a',NULL,'CLOSED','fixture','closed')`
				case "check":
					query = `SELECT set_evidence_resource_state('case-a','employment.company_and_dates','PASSED','fixture','passed')`
				case "principal":
					query = `UPDATE domain_users SET status='disabled' WHERE id=$1`
					args = []any{f.principal}
				}
				if mutation != "principal" {
					if _, err = change.Exec(ctx, `SET LOCAL ROLE `+pgx.Identifier{f.operator}.Sanitize()); err != nil {
						t.Fatal(err)
					}
				}
				allow := func() (bool, error) {
					var ok bool
					err := authorize.QueryRow(ctx, `SELECT check_evidence_upload($1,'case-a','employment.company_and_dates')`, f.principal).Scan(&ok)
					return ok, err
				}
				changed := make(chan error, 1)
				type result struct {
					ok  bool
					err error
				}
				checked := make(chan result, 1)
				if mutationFirst {
					if _, err = change.Exec(ctx, query, args...); err != nil {
						t.Fatal(err)
					}
					go func() { ok, e := allow(); checked <- result{ok, e} }()
					waitEmployerBlocked(t, f.pool, change.Conn().PgConn().PID(), authorize.Conn().PgConn().PID())
					if err = change.Commit(ctx); err != nil {
						t.Fatal(err)
					}
					r := <-checked
					if r.err != nil || r.ok {
						t.Fatal("mutation-first authorized", r)
					}
				} else {
					ok, e := allow()
					if e != nil || !ok {
						t.Fatal(ok, e)
					}
					go func() { _, e := change.Exec(ctx, query, args...); changed <- e }()
					waitEmployerBlocked(t, f.pool, authorize.Conn().PgConn().PID(), change.Conn().PgConn().PID())
					if err = authorize.Commit(ctx); err != nil {
						t.Fatal(err)
					}
					if err = <-changed; err != nil {
						t.Fatal(err)
					}
					if err = change.Commit(ctx); err != nil {
						t.Fatal(err)
					}
				}
				// Every subsequent real HTTP init is denied after the mutation commits.
				evidenceStatus(t, f.request(f.principal, "case-a", "employment.company_and_dates", false, evidenceFile), 404)
			})
		}
	}
}

func TestEvidenceMigrationAtomicityAndPrerequisites(t *testing.T) {
	_, pool := securityTestServer(t)
	ctx := context.Background()
	migration := employerSQL(t, "../../migrations/000015_case_evidence_authorization.sql")
	for _, mode := range []string{"rollback", "late_failure", "wrong_parent_type"} {
		t.Run(mode, func(t *testing.T) {
			schema := "evidence_migration_" + strings.ReplaceAll(uuid.NewString(), "-", "")
			evidenceExec(t, pool, `CREATE SCHEMA `+pgx.Identifier{schema}.Sanitize())
			t.Cleanup(func() { evidenceExec(t, pool, `DROP SCHEMA `+pgx.Identifier{schema}.Sanitize()+` CASCADE`) })
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			if _, err = tx.Exec(ctx, `SELECT set_config('search_path',$1,true)`, schema); err != nil {
				t.Fatal(err)
			}
			typ := "uuid"
			if mode == "wrong_parent_type" {
				typ = "text"
			}
			if _, err = tx.Exec(ctx, `CREATE TABLE domain_users(id `+typ+` PRIMARY KEY,status text NOT NULL)`); err != nil {
				t.Fatal(err)
			}
			if mode == "late_failure" {
				// Fail after CREATE TABLE has run, rather than only at preflight.
				if _, err = tx.Exec(ctx, `CREATE FUNCTION check_evidence_upload(uuid,text,text) RETURNS boolean LANGUAGE sql AS 'SELECT true'`); err != nil {
					t.Fatal(err)
				}
			}
			if err = tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			conn, err := pool.Acquire(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Release()
			if _, err = conn.Exec(ctx, `SELECT set_config('search_path',$1,false)`, schema); err != nil {
				t.Fatal(err)
			}
			defer conn.Exec(ctx, `RESET search_path`)
			if mode == "rollback" {
				inner, err := conn.Begin(ctx)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = inner.Exec(ctx, migration); err != nil {
					t.Fatal(err)
				}
				if err = inner.Rollback(ctx); err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err = conn.Exec(ctx, migration); err == nil {
					t.Fatal("invalid migration succeeded")
				}
			}
			var count int
			if err = conn.QueryRow(ctx, `SELECT count(*) FROM pg_class WHERE relnamespace=$1::regnamespace AND relname IN ('cases','verification_checks','case_evidence_upload_grants')`, schema).Scan(&count); err != nil || count != 0 {
				t.Fatal("partial migration survived", count, err)
			}
		})
	}
}

func TestEvidenceMigrationOwnerPathsAndGrantIntegrity(t *testing.T) {
	f := evidenceSetup(t)
	ctx := context.Background()
	// Once-only migration: existing objects are not silently reused or reset.
	_, err := f.pool.Exec(ctx, employerSQL(t, "../../migrations/000015_case_evidence_authorization.sql"))
	expectEmployerSQLState(t, err, "42P07")
	// Rebinding is safe and does not add raw registry privileges.
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT set_config('cvera.evidence_runtime_role',$1,true),set_config('cvera.evidence_operator_role',$2,true)`, f.runtime, f.operator); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, employerSQL(t, "../../operations/evidence-authorization-privileges.sql")); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		`SELECT provision_evidence_case('..','fixture','approval')`,
		`SELECT provision_evidence_check('case-a','..','fixture','approval')`,
		`SELECT provision_evidence_check('unprovisioned','x','fixture','approval')`,
		`SELECT provision_evidence_case('no-approval','fixture','')`,
		`SELECT grant_evidence_upload('` + f.principal + `','case-a','fixture','duplicate')`,
		`SELECT set_evidence_resource_state('case-a',NULL,'UNKNOWN','fixture','approval')`,
	} {
		tx, err := f.pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = tx.Exec(ctx, `SET LOCAL ROLE `+pgx.Identifier{f.operator}.Sanitize()); err != nil {
			t.Fatal(err)
		}
		_, err = tx.Exec(ctx, query)
		tx.Rollback(ctx)
		if err == nil {
			t.Fatal("operator input unexpectedly accepted", query)
		}
	}
	f.admin(t, `SELECT revoke_evidence_upload($1,'fixture','withdrawn')`, f.grant)
	var principal, c, actor, basis, revoker, reason, session string
	if err := f.pool.QueryRow(ctx, `SELECT principal_id::text,case_id,granted_by,grant_basis_ref,revoked_by,revocation_reason,revoked_session_user FROM case_evidence_upload_grants WHERE grant_id=$1`, f.grant).Scan(&principal, &c, &actor, &basis, &revoker, &reason, &session); err != nil {
		t.Fatal(err)
	}
	if principal != f.principal || c != "case-a" || actor != "fixture-operator" || basis != "reviewed fixture" || revoker != "fixture" || reason != "withdrawn" || session != "cvera_test" {
		t.Fatal("attribution lost")
	}
	// A runtime temporary table cannot shadow definer table lookups.
	if _, err = f.s.db.Exec(`CREATE TEMP TABLE case_evidence_upload_grants(principal_id uuid,case_id text,revoked_at timestamptz); INSERT INTO case_evidence_upload_grants VALUES('` + f.principal + `','case-a',NULL)`); err != nil {
		t.Fatal(err)
	}
	evidenceStatus(t, f.request(f.principal, "case-a", "employment.company_and_dates", false, evidenceFile), 404)
	// Refuse unreviewed RLS and stale-snapshot isolation rather than bypassing them.
	evidenceExec(t, f.pool, `ALTER TABLE cases ENABLE ROW LEVEL SECURITY`)
	evidenceStatus(t, f.request(f.principal, "case-a", "employment.company_and_dates", false, evidenceFile), 503)
	evidenceExec(t, f.pool, `ALTER TABLE cases DISABLE ROW LEVEL SECURITY`)
	isolated, err := f.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		t.Fatal(err)
	}
	defer isolated.Rollback(ctx)
	_, err = isolated.Exec(ctx, `SELECT check_evidence_upload($1,'case-a','employment.company_and_dates')`, f.principal)
	expectEmployerSQLState(t, err, "55000")
}
