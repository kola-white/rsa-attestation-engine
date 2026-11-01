package httpapi

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/employeraccess"
)

func employerSQL(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func createEmployerProbeRole(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	ctx := context.Background()
	role := "hr_probe_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quoted := pgx.Identifier{role}.Sanitize()
	if _, err := pool.Exec(ctx, `CREATE ROLE `+quoted+` NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOBYPASSRLS`); err != nil {
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
	return role
}

func applyEmployerBindings(t *testing.T, pool *pgxpool.Pool, runtime, operator string) {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT set_config('cvera.hr_runtime_role',$1,true),set_config('cvera.hr_operator_role',$2,true)`, runtime, operator); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, employerSQL(t, "../../operations/hr-assignment-privileges.sql")); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

func expectEmployerSQLState(t *testing.T, err error, want string) {
	t.Helper()
	var state interface{ SQLState() string }
	if !errors.As(err, &state) || state.SQLState() != want {
		t.Fatalf("expected SQLSTATE %s, got %v", want, err)
	}
}

func TestEmployerMigrationConfirmedParentMetadata(t *testing.T) {
	s, pool := securityTestServer(t)
	ctx := context.Background()
	for _, tc := range []struct {
		table, column, typ string
		notNull            bool
	}{
		{"domain_users", "id", "uuid", true}, {"domain_users", "kratos_identity_id", "text", true},
		{"domain_users", "role", "text", true}, {"domain_users", "status", "text", true},
		{"evt_employer_config", "employer_id", "text", true}, {"evt_employer_config", "dispatch_mode", "text", true},
		{"evt_requests", "request_id", "uuid", true}, {"evt_requests", "candidate_personid", "uuid", true},
		{"evt_requests", "employer_id", "text", true}, {"evt_requests", "hr_assigned_personid", "uuid", false},
		{"employer_hr_assignments", "principal_id", "uuid", true}, {"employer_hr_assignments", "employer_id", "text", true},
	} {
		var typ string
		var notNull bool
		err := pool.QueryRow(ctx, `SELECT format_type(a.atttypid,a.atttypmod),a.attnotnull FROM pg_attribute a WHERE a.attrelid=to_regclass($1) AND a.attname=$2 AND NOT a.attisdropped`, tc.table, tc.column).Scan(&typ, &notNull)
		if err != nil || typ != tc.typ || notNull != tc.notNull {
			t.Fatalf("%s.%s: %s notnull=%t err=%v", tc.table, tc.column, typ, notNull, err)
		}
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM employer_hr_assignments`).Scan(&count); err != nil || count != 0 {
		t.Fatal("migration created implicit assignments", err)
	}
	var role, status string
	if err := s.db.QueryRow(`INSERT INTO domain_users(kratos_identity_id) VALUES('confirmed-defaults') RETURNING role,status`).Scan(&role, &status); err != nil || role != "hr_reviewer" || status != "active" {
		t.Fatal("confirmed defaults changed", err)
	}
	var fkCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_constraint WHERE conrelid='employer_hr_assignments'::regclass AND contype='f' AND confdeltype='r' AND confupdtype='r'`).Scan(&fkCount); err != nil || fkCount != 2 {
		t.Fatal("assignment FK restrict contracts missing", err)
	}
	var invoker, fixedPath bool
	if err := pool.QueryRow(ctx, `SELECT NOT p.prosecdef, EXISTS(SELECT 1 FROM unnest(p.proconfig) c WHERE c LIKE 'search_path=pg_catalog,%pg_temp') FROM pg_proc p WHERE p.oid='enforce_employer_hr_assignment()'::regprocedure`).Scan(&invoker, &fixedPath); err != nil || !invoker || !fixedPath {
		t.Fatal("trigger security contract", err)
	}
	for _, fn := range []string{"check_employer_hr_assignment(uuid,text)", "lock_employer_hr_principal(uuid)"} {
		var bounded, private bool
		err := pool.QueryRow(ctx, `SELECT p.prosecdef AND p.provolatile='v' AND EXISTS(SELECT 1 FROM unnest(p.proconfig) c WHERE c LIKE 'search_path=pg_catalog,%pg_temp'), NOT EXISTS(SELECT 1 FROM aclexplode(coalesce(p.proacl,acldefault('f',p.proowner))) a WHERE a.grantee<>p.proowner AND a.privilege_type='EXECUTE') FROM pg_proc p WHERE p.oid=$1::regprocedure`, fn).Scan(&bounded, &private)
		if err != nil || !bounded || !private {
			t.Fatalf("bounded private locking function %s: %v", fn, err)
		}
	}

}

func TestEmployerMigrationTransactionAndPreflight(t *testing.T) {
	_, pool := securityTestServer(t)
	ctx := context.Background()
	for _, mismatch := range []bool{false, true} {
		schema := "hr_migration_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if _, err = tx.Exec(ctx, `CREATE SCHEMA `+pgx.Identifier{schema}.Sanitize()); err != nil {
			t.Fatal(err)
		}
		if _, err = tx.Exec(ctx, `SELECT set_config('search_path',$1,true)`, schema); err != nil {
			t.Fatal(err)
		}
		if _, err = tx.Exec(ctx, employerSQL(t, "testdata/proposal_a_legacy_schema.sql")); err != nil {
			t.Fatal(err)
		}
		if mismatch {
			if _, err = tx.Exec(ctx, `ALTER TABLE evt_requests ALTER COLUMN request_id TYPE text USING request_id::text`); err != nil {
				t.Fatal(err)
			}
		}
		_, err = tx.Exec(ctx, employerSQL(t, "../../migrations/000014_employer_hr_assignments.sql"))
		if mismatch && (err == nil || !strings.Contains(err.Error(), "prerequisite mismatch")) {
			t.Fatalf("expected mismatch rejection: %v", err)
		}
		if !mismatch && err != nil {
			t.Fatal(err)
		}
		if err = tx.Rollback(ctx); err != nil {
			t.Fatal(err)
		}
		var remains bool
		if err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_namespace WHERE nspname=$1)`, schema).Scan(&remains); err != nil || remains {
			t.Fatal("migration transaction did not roll back", err)
		}
	}
}

func TestEmployerMigrationConstraintsAndOperatorBoundary(t *testing.T) {
	s, pool := securityTestServer(t)
	a, _ := seedEmployerFixture(t, s)
	ctx := context.Background()
	runtime := createEmployerProbeRole(t, pool)
	operator := createEmployerProbeRole(t, pool)
	applyEmployerBindings(t, pool, runtime, operator)
	asOperator := func(fn func(pgx.Tx) error) error {
		tx, err := pool.Begin(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx)
		if _, err = tx.Exec(ctx, `SET LOCAL ROLE `+pgx.Identifier{operator}.Sanitize()); err != nil {
			return err
		}
		if err = fn(tx); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	id := uuid.Nil
	if err := asOperator(func(tx pgx.Tx) error {
		var err error
		id, err = employeraccess.Grant(ctx, tx, a, "employer-a", "reviewed-human", "evidence-ref")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	// A narrowly bound operator can revoke but cannot rewrite grant provenance,
	// reassign a principal/employer, delete history, or revive a revoked grant.
	for _, query := range []string{
		`UPDATE employer_hr_assignments SET principal_id=gen_random_uuid() WHERE assignment_id=$1`,
		`UPDATE employer_hr_assignments SET granted_by='replacement' WHERE assignment_id=$1`,
		`DELETE FROM employer_hr_assignments WHERE assignment_id=$1`,
	} {
		err := asOperator(func(tx pgx.Tx) error { _, err := tx.Exec(ctx, query, id); return err })
		expectEmployerSQLState(t, err, "42501")
	}
	// Foreign keys are checked by the actual migration, independently of the CLI.
	_, err := s.db.Exec(`INSERT INTO employer_hr_assignments(assignment_id,principal_id,employer_id,granted_by,grant_basis_ref) VALUES($1,$2,'missing','operator','ref')`, uuid.New(), a)
	expectEmployerSQLState(t, err, "23503")
	_, err = s.db.Exec(`INSERT INTO employer_hr_assignments(assignment_id,principal_id,employer_id,granted_by,grant_basis_ref) VALUES($1,$2,'employer-b',' ','ref')`, uuid.New(), a)
	expectEmployerSQLState(t, err, "23514")
	if err = asOperator(func(tx pgx.Tx) error { return employeraccess.Revoke(ctx, tx, id, "operator", "withdrawn") }); err != nil {
		t.Fatal(err)
	}
	err = asOperator(func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE employer_hr_assignments SET revoked_at=NULL,revoked_by=NULL,revocation_reason=NULL WHERE assignment_id=$1`, id)
		return err
	})
	expectEmployerSQLState(t, err, "23514")
	// Even a trusted owner cannot accidentally rewrite immutable provenance via
	// ordinary UPDATE; changing/dropping the trigger remains a trusted DDL power.
	_, err = s.db.Exec(`UPDATE employer_hr_assignments SET granted_by='replacement' WHERE assignment_id=$1`, id)
	expectEmployerSQLState(t, err, "23514")
	_, err = s.db.Exec(`DELETE FROM domain_users WHERE id=$1`, a)
	expectEmployerSQLState(t, err, "23503")
	_, err = s.db.Exec(`DELETE FROM evt_employer_config WHERE employer_id='employer-a'`)
	expectEmployerSQLState(t, err, "23503")
}

func TestEmployerPrivilegeBindingRejectsUnsafeRoles(t *testing.T) {
	_, pool := securityTestServer(t)
	ctx := context.Background()
	runtime := createEmployerProbeRole(t, pool)
	operator := createEmployerProbeRole(t, pool)
	for _, mode := range []string{"same-role", "owner", "membership", "rls", "default-acl", "operator-user-write", "operator-record-read", "operator-provenance-write"} {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx) // Also release locks if an assertion fails.
		r, o := runtime, operator
		switch mode {
		case "same-role":
			o = r
		case "owner":
			if err = tx.QueryRow(ctx, `SELECT current_user`).Scan(&r); err != nil {
				t.Fatal(err)
			}
		case "membership":
			_, err = tx.Exec(ctx, `GRANT `+pgx.Identifier{operator}.Sanitize()+` TO `+pgx.Identifier{runtime}.Sanitize())
		case "rls":
			_, err = tx.Exec(ctx, `ALTER TABLE domain_users ENABLE ROW LEVEL SECURITY`)
		case "default-acl":
			_, err = tx.Exec(ctx, `GRANT SELECT ON employer_hr_assignments TO PUBLIC`)
		case "operator-user-write":
			_, err = tx.Exec(ctx, `GRANT UPDATE(role) ON domain_users TO `+pgx.Identifier{operator}.Sanitize())
		case "operator-record-read":
			_, err = tx.Exec(ctx, `GRANT SELECT ON evt_requests TO `+pgx.Identifier{operator}.Sanitize())
		case "operator-provenance-write":
			_, err = tx.Exec(ctx, `GRANT UPDATE(granted_by) ON employer_hr_assignments TO `+pgx.Identifier{operator}.Sanitize())
		}
		if err != nil {
			t.Fatal(err)
		}
		if _, err = tx.Exec(ctx, `SELECT set_config('cvera.hr_runtime_role',$1,true),set_config('cvera.hr_operator_role',$2,true)`, r, o); err != nil {
			t.Fatal(err)
		}
		_, err = tx.Exec(ctx, employerSQL(t, "../../operations/hr-assignment-privileges.sql"))
		if mode == "operator-provenance-write" && err == nil {
			// PostgreSQL REVOKE ALL also removes this direct column grant.
			// Verify the resulting privilege boundary, rather than requiring
			// rejection of a configuration that was safely narrowed.
			var canRewrite bool
			if err = tx.QueryRow(ctx, `SELECT has_column_privilege($1,'employer_hr_assignments','granted_by','UPDATE')`, operator).Scan(&canRewrite); err != nil || canRewrite {
				t.Fatalf("provenance write survived binding: %v", err)
			}
		} else if err == nil {
			t.Fatalf("unsafe binding %s accepted", mode)
		}
		if err = tx.Rollback(ctx); err != nil {
			t.Fatal(err)
		}
	}
}

func TestEmployerReadOnlyMetadataQuerySyntax(t *testing.T) {
	_, pool := securityTestServer(t)
	// Syntax verification against the disposable local instance only. The public
	// namespace is deliberately not populated with synthetic application tables.
	conn, err := pool.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	defer conn.Exec(context.Background(), `ROLLBACK`)
	if _, err = conn.Exec(context.Background(), employerSQL(t, "../../operations/hr-assignment-metadata.sql")); err != nil {
		t.Fatal(err)
	}
}

func TestEmployerCheckWithoutProfileWritesAndLegacyGrantCompatibility(t *testing.T) {
	s, pool := securityTestServer(t)
	a, _ := seedEmployerFixture(t, s)
	ctx := context.Background()
	runtime := createEmployerProbeRole(t, pool)
	operator := createEmployerProbeRole(t, pool)
	// Match the verified evt-app legacy privileges, without changing their scope.
	for _, table := range []string{"domain_users", "evt_employer_config", "evt_requests"} {
		if _, err := pool.Exec(ctx, `GRANT SELECT,INSERT,UPDATE,DELETE ON `+pgx.Identifier{table}.Sanitize()+` TO `+pgx.Identifier{runtime}.Sanitize()); err != nil {
			t.Fatal(err)
		}
	}
	grantEmployer(t, pool, a, "employer-a")
	applyEmployerBindings(t, pool, runtime, operator)
	for _, table := range []string{"domain_users", "evt_employer_config", "evt_requests"} {
		for _, priv := range []string{"SELECT", "INSERT", "UPDATE", "DELETE"} {
			var allowed bool
			if err := pool.QueryRow(ctx, `SELECT has_table_privilege($1,$2,$3)`, runtime, table, priv).Scan(&allowed); err != nil || !allowed {
				t.Fatalf("legacy %s on %s changed: %v", priv, table, err)
			}
		}
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SET LOCAL ROLE `+pgx.Identifier{runtime}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	if err = employeraccess.Authorize(ctx, tx, a, "employer-a"); err != nil {
		t.Fatal(err)
	}
	if err = employeraccess.Authorize(ctx, tx, a, "employer-b"); err == nil {
		t.Fatal("legacy DML conferred another employer grant")
	}
	_, err = tx.Exec(ctx, `INSERT INTO employer_hr_assignments(assignment_id,principal_id,employer_id,granted_by,grant_basis_ref) VALUES($1,$2,'employer-b','runtime','unreviewed')`, uuid.New(), a)
	expectEmployerSQLState(t, err, "42501")
}

func TestEmployerBoundedFunctionSearchPathIsolationAndRLS(t *testing.T) {
	s, pool := securityTestServer(t)
	a, _ := seedEmployerFixture(t, s)
	ctx := context.Background()
	runtime := createEmployerProbeRole(t, pool)
	operator := createEmployerProbeRole(t, pool)
	grantEmployer(t, pool, a, "employer-a")
	applyEmployerBindings(t, pool, runtime, operator)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SET LOCAL ROLE `+pgx.Identifier{runtime}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `CREATE TEMP TABLE domain_users(id uuid,status text,role text); CREATE TEMP TABLE employer_hr_assignments(principal_id uuid,employer_id text,revoked_at timestamptz)`); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO pg_temp.domain_users VALUES($1,'active','hr_reviewer');`, a); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO pg_temp.employer_hr_assignments VALUES($1,'employer-b',NULL)`, a); err != nil {
		t.Fatal(err)
	}
	if err = employeraccess.Authorize(ctx, tx, a, "employer-a"); err != nil {
		t.Fatalf("trusted grant masked: %v", err)
	}
	if err = employeraccess.Authorize(ctx, tx, a, "employer-b"); err == nil {
		t.Fatal("temporary table bypassed authorization")
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	for _, isolation := range []pgx.TxIsoLevel{pgx.RepeatableRead, pgx.Serializable} {
		tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: isolation})
		if err != nil {
			t.Fatal(err)
		}
		err = employeraccess.Authorize(ctx, tx, a, "employer-a")
		tx.Rollback(ctx)
		expectEmployerSQLState(t, err, "55000")
	}
	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `ALTER TABLE domain_users ENABLE ROW LEVEL SECURITY`); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `SET LOCAL ROLE `+pgx.Identifier{runtime}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	err = employeraccess.Authorize(ctx, tx, a, "employer-a")
	expectEmployerSQLState(t, err, "55000")
}

func TestEmployerMigrationClearsInheritedCreationDefaults(t *testing.T) {
	_, pool := securityTestServer(t)
	ctx := context.Background()
	runtime := createEmployerProbeRole(t, pool)
	operator := createEmployerProbeRole(t, pool)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	schema := "hr_default_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = tx.Exec(ctx, `CREATE SCHEMA `+pgx.Identifier{schema}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `SELECT set_config('search_path',$1,true)`, schema); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, employerSQL(t, "testdata/proposal_a_legacy_schema.sql")); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `ALTER DEFAULT PRIVILEGES IN SCHEMA `+pgx.Identifier{schema}.Sanitize()+` GRANT SELECT,INSERT,UPDATE,DELETE ON TABLES TO `+pgx.Identifier{runtime}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `ALTER DEFAULT PRIVILEGES IN SCHEMA `+pgx.Identifier{schema}.Sanitize()+` GRANT EXECUTE ON FUNCTIONS TO `+pgx.Identifier{runtime}.Sanitize()+`,`+pgx.Identifier{operator}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, employerSQL(t, "../../migrations/000014_employer_hr_assignments.sql")); err != nil {
		t.Fatal(err)
	}
	var exposed bool
	if err = tx.QueryRow(ctx, `SELECT has_any_column_privilege($1,'employer_hr_assignments','SELECT') OR has_any_column_privilege($1,'employer_hr_assignments','INSERT') OR has_any_column_privilege($1,'employer_hr_assignments','UPDATE') OR has_table_privilege($1,'employer_hr_assignments','DELETE')`, runtime).Scan(&exposed); err != nil || exposed {
		t.Fatal("creation defaults exposed assignment access", err)
	}
	for _, r := range []string{runtime, operator, "public"} {
		if r == "public" {
			var publicExec bool
			if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_proc p,LATERAL aclexplode(coalesce(p.proacl,acldefault('f',p.proowner))) a WHERE p.oid IN ('check_employer_hr_assignment(uuid,text)'::regprocedure,'lock_employer_hr_principal(uuid)'::regprocedure) AND a.grantee=0 AND a.privilege_type='EXECUTE')`).Scan(&publicExec); err != nil || publicExec {
				t.Fatal("PUBLIC function access remained", err)
			}
			continue
		}
		if err = tx.QueryRow(ctx, `SELECT has_function_privilege($1,'check_employer_hr_assignment(uuid,text)','EXECUTE') OR has_function_privilege($1,'lock_employer_hr_principal(uuid)','EXECUTE')`, r).Scan(&exposed); err != nil || exposed {
			t.Fatal("creation defaults exposed definer execution", err)
		}
	}
}

func TestEmployerMigrationAtomicWithoutExternalRunner(t *testing.T) {
	_, pool := securityTestServer(t)
	ctx := context.Background()
	schema := "hr_atomic_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quoted := pgx.Identifier{schema}.Sanitize()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `CREATE SCHEMA `+quoted); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `SELECT set_config('search_path',$1,true)`, schema); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, employerSQL(t, "testdata/proposal_a_legacy_schema.sql")); err != nil {
		t.Fatal(err)
	}
	// Force a failure after the migration creates its assignment table/index.
	if _, err = tx.Exec(ctx, `CREATE FUNCTION check_employer_hr_assignment(uuid,text) RETURNS boolean LANGUAGE sql AS 'SELECT false'`); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), `DROP SCHEMA `+quoted+` CASCADE`); err != nil {
			t.Error(err)
		}
	})
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	var original string
	if err = conn.QueryRow(ctx, `SELECT current_setting('search_path')`).Scan(&original); err != nil {
		t.Fatal(err)
	}
	defer conn.Exec(context.Background(), `SELECT set_config('search_path',$1,false)`, original)
	if _, err = conn.Exec(ctx, `SELECT set_config('search_path',$1,false)`, schema); err != nil {
		t.Fatal(err)
	}
	// Intentionally no explicit transaction: the single DO must be atomic.
	_, err = conn.Exec(ctx, employerSQL(t, "../../migrations/000014_employer_hr_assignments.sql"))
	expectEmployerSQLState(t, err, "42723")
	var absent bool
	if err = conn.QueryRow(ctx, `SELECT to_regclass($1) IS NULL`, schema+".employer_hr_assignments").Scan(&absent); err != nil || !absent {
		t.Fatal("autocommit migration left partial assignment objects", err)
	}
}
