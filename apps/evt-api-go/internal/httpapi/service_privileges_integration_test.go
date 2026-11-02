package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/auth"
)

// These additional objects are explicit privilege-test stubs, not claims about
// omitted production workflow DDL. The actual grant scripts create no tables.
func servicePrivilegeStubs(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
CREATE SEQUENCE evt_request_events_event_id_seq;
ALTER TABLE evt_request_events ADD COLUMN event_id bigint DEFAULT nextval('evt_request_events_event_id_seq');
CREATE SEQUENCE evt_token_consumptions_consumption_id_seq;
CREATE TABLE evt_token_consumptions(consumption_id bigint DEFAULT nextval('evt_token_consumptions_consumption_id_seq'),request_id uuid,recruiter_personid uuid,user_agent text,ip_hash text);
CREATE TABLE recruiter_trusted_issuers(id uuid);
`)
	if err != nil {
		t.Fatal(err)
	}
}

func applyServicePrivileges(t *testing.T, pool *pgxpool.Pool, role, profile string) {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT set_config('cvera.service_runtime_role',$1,true)`, role); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, employerSQL(t, "../../operations/"+profile+"-runtime-privileges.sql")); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

func serviceRoleURL(t *testing.T, owner *pgxpool.Pool, runtime string) string {
	t.Helper()
	u, err := url.Parse(os.Getenv("CVERA_AUTHORITY_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", owner.Config().ConnConfig.RuntimeParams["search_path"])
	q.Set("role", runtime)
	u.RawQuery = q.Encode()
	return u.String()
}

func TestServiceAPIRuntimePrivilegesSupportAuthAndEmployerChecks(t *testing.T) {
	s, owner := securityTestServer(t)
	servicePrivilegeStubs(t, owner)
	a, _ := seedEmployerFixture(t, s)
	grantEmployer(t, owner, a, "employer-a")
	runtime := createEmployerProbeRole(t, owner)
	operator := createEmployerProbeRole(t, owner)
	applyServicePrivileges(t, owner, runtime, "api")
	applyEmployerBindings(t, owner, runtime, operator)

	// Authenticate the test connection as the existing disposable administrator,
	// then select the dedicated role at startup; current_user must be the role.
	// No production password or DSN is accepted by securityTestServer's guard.
	runtimeURL := serviceRoleURL(t, owner, runtime)
	restricted, err := sql.Open("pgx", runtimeURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { restricted.Close() })
	var current string
	if err = restricted.QueryRow(`SELECT current_user`).Scan(&current); err != nil || current != runtime {
		t.Fatal("startup role was not selected", err)
	}
	s.db = restricted
	first := exchangeTestUser(t, s, "service-profile-identity")
	// Exercise the existing user's ON CONFLICT UPDATE(email,name,updated_at).
	exchangeTestUser(t, s, "service-profile-identity")
	refreshed := callTokenHandler(s.HandleAuthRefresh, first.RefreshToken)
	if refreshed.Code != http.StatusOK {
		t.Fatalf("restricted refresh: %d %s", refreshed.Code, refreshed.Body.String())
	}
	var pair AuthRefreshResponse
	if err = json.Unmarshal(refreshed.Body.Bytes(), &pair); err != nil {
		t.Fatal(err)
	}
	if w := callTokenHandler(s.HandleAuthLogout, pair.RefreshToken); w.Code != http.StatusOK {
		t.Fatalf("restricted logout: %d", w.Code)
	}
	if w := callTokenHandler(s.HandleAuthRefresh, pair.RefreshToken); w.Code != http.StatusUnauthorized {
		t.Fatalf("restricted revoked refresh: %d", w.Code)
	}
	if _, err = restricted.Exec(`UPDATE domain_users SET role='cvera' WHERE id=$1`, first.User.ID); err == nil {
		t.Fatal("runtime changed principal role")
	} else {
		expectEmployerSQLState(t, err, "42501")
	}
	if _, err = restricted.Exec(`INSERT INTO evt_request_events(request_id,event_type) VALUES($1,'synthetic')`, uuid.NewString()); err != nil {
		t.Fatal("event INSERT/default nextval failed", err)
	}
	if _, err = restricted.Exec(`INSERT INTO evt_token_consumptions(request_id) VALUES($1)`, uuid.NewString()); err != nil {
		t.Fatal("consumption INSERT/default nextval failed", err)
	}
	pool, err := pgxpool.New(context.Background(), runtimeURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	engine := employerEngine(s, pool)
	token := employerToken(t, s, a, auth.RoleHRReviewer)
	for _, path := range []string{"/v1/employer/requests?employer_id=employer-a", "/v1/employer/requests/00000000-0000-4000-8000-00000000000a?employer_id=employer-a"} {
		if w := employerCall(engine, token, "GET", path, ""); w.Code != http.StatusOK {
			t.Fatalf("restricted employer read: %d %s", w.Code, w.Body.String())
		}
	}
	if w := employerCall(engine, token, "GET", "/v1/employer/requests?employer_id=employer-b", ""); w.Code != http.StatusNotFound {
		t.Fatalf("restricted cross-employer read: %d", w.Code)
	}
	if _, err = restricted.Exec(`SELECT * FROM employer_hr_assignments`); err == nil {
		t.Fatal("runtime read assignment table directly")
	} else {
		expectEmployerSQLState(t, err, "42501")
	}
}

func TestServiceKratosPrivilegesAreBoundedToIdentityStore(t *testing.T) {
	_, pool := securityTestServer(t)
	ctx := context.Background()
	runtime := createEmployerProbeRole(t, pool)
	apiRuntime := createEmployerProbeRole(t, pool)
	servicePrivilegeStubs(t, pool)
	applyServicePrivileges(t, pool, apiRuntime, "api")
	tables := []string{
		"continuity_containers", "courier_message_dispatches", "courier_messages", "identities",
		"identity_credential_identifiers", "identity_credential_types", "identity_credentials",
		"identity_login_codes", "identity_recovery_addresses", "identity_recovery_codes",
		"identity_recovery_tokens", "identity_registration_codes", "identity_verifiable_addresses",
		"identity_verification_codes", "identity_verification_tokens", "networks", "schema_migration",
		"selfservice_errors", "selfservice_login_flows", "selfservice_recovery_flows",
		"selfservice_registration_flows", "selfservice_settings_flows", "selfservice_verification_flows",
		"session_devices", "session_token_exchanges", "sessions",
	}
	schema := "kratos_privilege_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `CREATE SCHEMA `+pgx.Identifier{schema}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `SELECT set_config('search_path',$1,true),set_config('cvera.service_runtime_role',$2,true)`, schema, runtime); err != nil {
		t.Fatal(err)
	}
	for _, name := range tables {
		// Access-control stubs only: no real identity schema/types or records.
		if _, err = tx.Exec(ctx, `CREATE TABLE `+pgx.Identifier{schema, name}.Sanitize()+`(id uuid PRIMARY KEY)`); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = tx.Exec(ctx, employerSQL(t, "../../operations/kratos-runtime-privileges.sql")); err != nil {
		t.Fatal(err)
	}
	var isolated bool
	if err = tx.QueryRow(ctx, `SELECT NOT has_table_privilege($1,$2,'SELECT,INSERT,UPDATE,DELETE') AND NOT has_table_privilege($3,$4,'SELECT,INSERT,UPDATE,DELETE')`, apiRuntime, schema+".identities", runtime, pool.Config().ConnConfig.RuntimeParams["search_path"]+".evt_requests").Scan(&isolated); err != nil || !isolated {
		t.Fatal("service roles cross the identity/application boundary", err)
	}
	var ledgerWrite bool
	if err = tx.QueryRow(ctx, `SELECT has_any_column_privilege($1,'schema_migration','INSERT,UPDATE') OR has_table_privilege($1,'schema_migration','DELETE,TRUNCATE,REFERENCES,TRIGGER')`, runtime).Scan(&ledgerWrite); err != nil || ledgerWrite {
		t.Fatal("runtime has vendor ledger write/administration privileges", err)
	}
	if _, err = tx.Exec(ctx, `SET LOCAL ROLE `+pgx.Identifier{runtime}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO identities(id) VALUES($1)`, uuid.NewString()); err != nil {
		t.Fatal("identity store INSERT denied", err)
	}
	if _, err = tx.Exec(ctx, `SELECT * FROM schema_migration`); err != nil {
		t.Fatal("version ledger read denied", err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO schema_migration(id) VALUES($1)`, uuid.NewString()); err == nil {
		t.Fatal("runtime rewrote vendor migration ledger")
	} else {
		expectEmployerSQLState(t, err, "42501")
	}
}

func TestServiceAPIProfileRefusesOwnerAndUnreviewedRLS(t *testing.T) {
	_, pool := securityTestServer(t)
	servicePrivilegeStubs(t, pool)
	ctx := context.Background()
	var owner string
	if err := pool.QueryRow(ctx, `SELECT current_user`).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	for _, profile := range []string{"api", "kratos"} {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if _, err = tx.Exec(ctx, `SELECT set_config('cvera.service_runtime_role',$1,true)`, owner); err != nil {
			t.Fatal(err)
		}
		_, err = tx.Exec(ctx, employerSQL(t, "../../operations/"+profile+"-runtime-privileges.sql"))
		expectEmployerSQLState(t, err, "P0001")
		tx.Rollback(ctx)
	}
	runtime := createEmployerProbeRole(t, pool)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `ALTER TABLE evt_requests ENABLE ROW LEVEL SECURITY;`); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `SELECT set_config('cvera.service_runtime_role',$1,true)`, runtime); err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, employerSQL(t, "../../operations/api-runtime-privileges.sql"))
	expectEmployerSQLState(t, err, "P0001")
}
