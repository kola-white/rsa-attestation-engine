package httpapi

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/auth"
	evtdb "github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/db"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/employeraccess"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/evt"
)

func grantEmployer(t *testing.T, pool *pgxpool.Pool, principal, employer string) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	id, err := employeraccess.Grant(ctx, tx, principal, employer, "fixture-operator", "fixture-reviewed-approval")
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return id
}

func revokeEmployer(t *testing.T, pool *pgxpool.Pool, id uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if err = employeraccess.Revoke(ctx, tx, id, "fixture-revoker", "approval withdrawn"); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

func employerEngine(s *Server, pool *pgxpool.Pool) *gin.Engine {
	d := &evtdb.DB{Pool: pool}
	m := &evt.Module{Employer: &evt.EmployerHandlers{DB: d, Repo: &evt.Repo{DB: d}}}
	engine := gin.New()
	engine.Use(auth.GinRequireAuth(s.cfg.Auth.JWTSecret, s.cfg.Auth.Issuer, s.cfg.Auth.Audience))
	m.Register(engine.Group("/v1"))
	return engine
}

func employerToken(t *testing.T, s *Server, principal string, role auth.Role) string {
	t.Helper()
	raw, err := auth.MintAccessTokenHS256(s.cfg.Auth.JWTSecret, s.cfg.Auth.Issuer, s.cfg.Auth.Audience, principal, "", []auth.Role{role}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func employerCall(engine *gin.Engine, token, method, path, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(w, r)
	return w
}

func seedEmployerFixture(t *testing.T, s *Server) (string, string) {
	t.Helper()
	a, b := uuid.NewString(), uuid.NewString()
	if _, err := s.db.Exec(`INSERT INTO domain_users(id,kratos_identity_id,role,status) VALUES($1::uuid,$1::uuid::text,'hr_reviewer','active'),($2::uuid,$2::uuid::text,'hr_reviewer','active')`, a, b); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO evt_employer_config(employer_id) VALUES('employer-a'),('employer-b'); INSERT INTO evt_requests(request_id,candidate_personid,employer_id,status) VALUES('00000000-0000-4000-8000-00000000000a','00000000-0000-4000-8000-000000000001','employer-a','ATTESTATION_PENDING'),('00000000-0000-4000-8000-00000000000b','00000000-0000-4000-8000-000000000002','employer-b','ATTESTATION_PENDING')`); err != nil {
		t.Fatal(err)
	}
	return a, b
}

func TestEmployerAuthorizationBoundaries(t *testing.T) {
	s, pool := securityTestServer(t)
	a, b := seedEmployerFixture(t, s)
	grantEmployer(t, pool, a, "employer-a")
	grantEmployer(t, pool, b, "employer-b")
	engine := employerEngine(s, pool)
	token := employerToken(t, s, a, auth.RoleHRReviewer)
	for _, tc := range []struct {
		method, path, body string
		want               int
	}{
		{"GET", "/v1/employer/requests?employer_id=employer-a", "", 200},
		{"GET", "/v1/employer/requests/00000000-0000-4000-8000-00000000000a?employer_id=employer-a", "", 200},
		{"GET", "/v1/employer/requests?employer_id=employer-b", "", 404},
		{"GET", "/v1/employer/requests/00000000-0000-4000-8000-00000000000b?employer_id=employer-b", "", 404},
		{"GET", "/v1/employer/requests/00000000-0000-4000-8000-00000000000b?employer_id=employer-a", "", 404},
		{"GET", "/v1/employer/requests/00000000-0000-4000-8000-00000000000a?employer_id=employer-b", "", 404},
		{"POST", "/v1/employer/requests/00000000-0000-4000-8000-00000000000b/attest", `{"employer_id":"employer-b","response_type":"FULL_MATCH"}`, 404},
		{"POST", "/v1/employer/requests/00000000-0000-4000-8000-00000000000b/attest", `{"employer_id":"employer-a","response_type":"FULL_MATCH"}`, 404},
		{"GET", "/v1/employer/requests?employer_id=unknown", "", 404},
	} {
		w := employerCall(engine, token, tc.method, tc.path, tc.body)
		if w.Code != tc.want {
			t.Fatalf("%s %s returned %d: %s", tc.method, tc.path, w.Code, w.Body.String())
		}
		if tc.want == 200 && strings.Contains(w.Body.String(), "00000000-0000-4000-8000-00000000000b") {
			t.Fatal("foreign request disclosed")
		}
	}
	var status string
	var events int
	var compact *string
	if err := s.db.QueryRow(`SELECT status,attestation_jws FROM evt_requests WHERE request_id='00000000-0000-4000-8000-00000000000b'`).Scan(&status, &compact); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow(`SELECT count(*) FROM evt_request_events`).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if status != "ATTESTATION_PENDING" || compact != nil || events != 0 {
		t.Fatal("denied action modified request, signed, or audited success")
	}
	// A revoked/absent signer would return 500 if authorization were bypassed.
	grantEmployer(t, pool, a, "employer-b")
	if w := employerCall(engine, token, "GET", "/v1/employer/requests?employer_id=employer-b", ""); w.Code != 200 {
		t.Fatalf("explicit second employer grant: %d", w.Code)
	}
	if _, err := s.db.Exec(`UPDATE evt_requests SET status='SUBMITTED' WHERE request_id='00000000-0000-4000-8000-00000000000a'`); err != nil {
		t.Fatal(err)
	}
	if w := employerCall(engine, token, "POST", "/v1/employer/requests/00000000-0000-4000-8000-00000000000a/attest", `{"employer_id":"employer-a","response_type":"FULL_MATCH"}`); w.Code != 409 {
		t.Fatalf("workflow state gate: %d", w.Code)
	}
}

func TestEmployerAuthorizationCurrentIdentityAndFailClosed(t *testing.T) {
	s, pool := securityTestServer(t)
	a, b := seedEmployerFixture(t, s)
	grantEmployer(t, pool, a, "employer-a")
	engine := employerEngine(s, pool)
	for _, tc := range []struct {
		principal string
		role      auth.Role
		want      int
	}{
		{b, auth.RoleHRReviewer, 404}, {uuid.NewString(), auth.RoleHRReviewer, 404}, {"not-a-uuid", auth.RoleHRReviewer, 404}, {a, auth.RoleRequestor, 403}, {a, auth.RoleCvera, 403},
	} {
		token := employerToken(t, s, tc.principal, tc.role)
		assertEmployerEndpoints(t, engine, token, tc.want)
	}
	token := employerToken(t, s, a, auth.RoleHRReviewer)
	if _, err := s.db.Exec(`UPDATE domain_users SET status='disabled' WHERE id=$1`, a); err != nil {
		t.Fatal(err)
	}
	assertEmployerEndpoints(t, engine, token, 404)
	if _, err := s.db.Exec(`UPDATE domain_users SET status='active',role='requestor' WHERE id=$1`, a); err != nil {
		t.Fatal(err)
	}
	assertEmployerEndpoints(t, engine, token, 404)
	if _, err := s.db.Exec(`UPDATE domain_users SET role='hr_reviewer' WHERE id=$1`, a); err != nil {
		t.Fatal(err)
	}
	// Missing schema must never produce implicit permission or a successful list.
	if _, err := s.db.Exec(`ALTER TABLE employer_hr_assignments RENAME TO unavailable_assignments`); err != nil {
		t.Fatal(err)
	}
	assertEmployerEndpoints(t, engine, token, 500)
}

func assertEmployerEndpoints(t *testing.T, engine *gin.Engine, token string, want int) {
	t.Helper()
	for _, tc := range []struct{ method, path, body string }{
		{"GET", "/v1/employer/requests?employer_id=employer-a", ""},
		{"GET", "/v1/employer/requests/00000000-0000-4000-8000-00000000000a?employer_id=employer-a", ""},
		{"POST", "/v1/employer/requests/00000000-0000-4000-8000-00000000000a/attest", `{"employer_id":"employer-a","response_type":"FULL_MATCH"}`},
	} {
		w := employerCall(engine, token, tc.method, tc.path, tc.body)
		if w.Code != want {
			t.Fatalf("%s %s: %d want %d (%s)", tc.method, tc.path, w.Code, want, w.Body.String())
		}
	}
}

func TestEmployerOperatorGrantRevokeAndRegrant(t *testing.T) {
	s, pool := securityTestServer(t)
	a, _ := seedEmployerFixture(t, s)
	ctx := context.Background()
	id := grantEmployer(t, pool, a, "employer-a")
	engine := employerEngine(s, pool)
	token := employerToken(t, s, a, auth.RoleHRReviewer)
	for _, tc := range []struct{ principal, employer, actor, basis string }{
		{a, "employer-a", "operator", "duplicate"}, {a, "unknown", "operator", "approval"},
		{uuid.NewString(), "employer-a", "operator", "approval"}, {a, "employer-b", "", "approval"}, {a, "employer-b", "operator", ""},
	} {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		_, err = employeraccess.Grant(ctx, tx, tc.principal, tc.employer, tc.actor, tc.basis)
		tx.Rollback(ctx)
		if err == nil {
			t.Fatalf("invalid grant succeeded: %+v", tc)
		}
	}
	revokeEmployer(t, pool, id)
	assertEmployerEndpoints(t, engine, token, 404)
	newID := grantEmployer(t, pool, a, "employer-a")
	if newID == id {
		t.Fatal("reactivated old grant")
	}
	var count int
	if err := s.db.QueryRow(`SELECT count(*) FROM employer_hr_assignments WHERE principal_id=$1`, a).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatal("revoked history lost")
	}
	var actor, basis, revoker, reason string
	if err := s.db.QueryRow(`SELECT granted_by,grant_basis_ref,revoked_by,revocation_reason FROM employer_hr_assignments WHERE assignment_id=$1`, id).Scan(&actor, &basis, &revoker, &reason); err != nil {
		t.Fatal(err)
	}
	if actor == "" || basis == "" || revoker == "" || reason == "" {
		t.Fatal("missing operator attribution")
	}
	// Revocation is possible after a role change/account disable.
	if _, err := s.db.Exec(`UPDATE domain_users SET role='requestor',status='disabled' WHERE id=$1`, a); err != nil {
		t.Fatal(err)
	}
	revokeEmployer(t, pool, newID)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = employeraccess.Grant(ctx, tx, a, "employer-b", "operator", "approval"); !errors.Is(err, evtdb.ErrNotFound) {
		t.Fatalf("disabled user grant: %v", err)
	}
}

func waitEmployerBlocked(t *testing.T, pool *pgxpool.Pool, blocker, waiter uint32) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var blocked bool
		if err := pool.QueryRow(context.Background(), `SELECT $1::int = ANY(pg_blocking_pids($2::int))`, blocker, waiter).Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		if blocked {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("expected principal lock contention was not observed")
}

func TestEmployerRevocationSerializesWithAuthorization(t *testing.T) {
	s, pool := securityTestServer(t)
	a, _ := seedEmployerFixture(t, s)
	id := grantEmployer(t, pool, a, "employer-a")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	authorized, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer authorized.Rollback(context.Background())
	if err := employeraccess.Authorize(ctx, authorized, a, "employer-a"); err != nil {
		t.Fatal(err)
	}
	var authorizedPID uint32
	if err := authorized.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&authorizedPID); err != nil {
		t.Fatal(err)
	}
	pid := make(chan uint32, 1)
	done := make(chan error, 1)
	go func() {
		tx, err := pool.Begin(ctx)
		if err != nil {
			done <- err
			return
		}
		defer tx.Rollback(context.Background())
		var p uint32
		if err = tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&p); err != nil {
			done <- err
			return
		}
		pid <- p
		// Deliberately bypass the CLI helper: the migration trigger must enforce
		// the principal lock even for a direct reviewed operator UPDATE.
		if _, err = tx.Exec(ctx, `UPDATE employer_hr_assignments SET revoked_at=clock_timestamp(),revoked_by='operator',revocation_reason='concurrent revocation' WHERE assignment_id=$1`, id); err == nil {
			err = tx.Commit(ctx)
		}
		done <- err
	}()
	var waiter uint32
	select {
	case waiter = <-pid:
	case err := <-done:
		t.Fatalf("revocation start: %v", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	waitEmployerBlocked(t, pool, authorizedPID, waiter)
	if err := authorized.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	assertEmployerEndpoints(t, employerEngine(s, pool), employerToken(t, s, a, auth.RoleHRReviewer), 404)
	// Start authorization while revocation is uncommitted. It must wait and
	// then read the newly revoked grant, rather than an old snapshot.
	id = grantEmployer(t, pool, a, "employer-a")
	revoking, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer revoking.Rollback(context.Background())
	if err = employeraccess.Revoke(ctx, revoking, id, "operator", "second revocation"); err != nil {
		t.Fatal(err)
	}
	var revokingPID uint32
	if err = revoking.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&revokingPID); err != nil {
		t.Fatal(err)
	}
	go func() {
		tx, err := pool.Begin(ctx)
		if err != nil {
			done <- err
			return
		}
		defer tx.Rollback(context.Background())
		var p uint32
		if err = tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&p); err != nil {
			done <- err
			return
		}
		pid <- p
		done <- employeraccess.Authorize(ctx, tx, a, "employer-a")
	}()
	select {
	case waiter = <-pid:
	case err := <-done:
		t.Fatalf("authorization start: %v", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	waitEmployerBlocked(t, pool, revokingPID, waiter)
	if err = revoking.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, evtdb.ErrNotFound) {
			t.Fatalf("revocation waiter authorized: %v", err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

func TestEmployerAssignmentsRestrictedRuntimeCannotAdminister(t *testing.T) {
	s, pool := securityTestServer(t)
	a, _ := seedEmployerFixture(t, s)
	id := grantEmployer(t, pool, a, "employer-a")
	ctx := context.Background()
	role := "hr_fixture_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quoted := pgx.Identifier{role}.Sanitize()
	if _, err := pool.Exec(ctx, `CREATE ROLE `+quoted+` NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(ctx, `DROP OWNED BY `+quoted); err != nil {
			t.Error(err)
		}
		if _, err := pool.Exec(ctx, `DROP ROLE `+quoted); err != nil {
			t.Error(err)
		}
	})

	operator := createEmployerProbeRole(t, pool)
	applyEmployerBindings(t, pool, role, operator)
	asRuntime := func(operation func(pgx.Tx) error) error {
		tx, err := pool.Begin(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx)
		if _, err = tx.Exec(ctx, `SET LOCAL ROLE `+quoted); err != nil {
			return err
		}
		return operation(tx)
	}
	if err := asRuntime(func(tx pgx.Tx) error { return employeraccess.Authorize(ctx, tx, a, "employer-a") }); err != nil {
		t.Fatalf("restricted authorization: %v", err)
	}
	operations := []func(pgx.Tx) error{
		func(tx pgx.Tx) error {
			_, err := employeraccess.Grant(ctx, tx, a, "employer-b", "runtime", "unreviewed")
			return err
		},
		func(tx pgx.Tx) error { return employeraccess.Revoke(ctx, tx, id, "runtime", "unreviewed") },
		func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `UPDATE employer_hr_assignments SET revoked_at=NULL,revoked_by=NULL,revocation_reason=NULL WHERE assignment_id=$1`, id)
			return err
		},
		func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `DELETE FROM employer_hr_assignments WHERE assignment_id=$1`, id)
			return err
		},
		func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `UPDATE domain_users SET role='hr_reviewer' WHERE id=$1`, a)
			return err
		},
	}
	for i, operation := range operations {
		err := asRuntime(operation)
		var state interface{ SQLState() string }
		if !errors.As(err, &state) || state.SQLState() != "42501" {
			t.Fatalf("runtime admin operation %d: expected permission denial, got %v", i, err)
		}
	}
}
