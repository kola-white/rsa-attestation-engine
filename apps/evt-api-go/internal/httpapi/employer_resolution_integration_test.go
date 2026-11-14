package httpapi

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/auth"
)

// Uses the existing disposable-database guard and existing migration fixture.
// Do not run under a no-migrations execution boundary; CI supplies its own DB.
func TestEmployerDiscoveryAssignmentsAndRevocation(t *testing.T) {
	s, pool := securityTestServer(t)
	a, b := seedEmployerFixture(t, s)
	if _, err := s.db.Exec(`INSERT INTO evt_employer_config(employer_id) VALUES('cei-uat-sim-001'),('019a5ace-3b00-7000-8000-000000000001')`); err != nil {
		t.Fatal(err)
	}
	// Exercise the actual runtime grants; no SELECT on assignments is needed.
	servicePrivilegeStubs(t, pool)
	runtime, operator := createEmployerProbeRole(t, pool), createEmployerProbeRole(t, pool)
	applyServicePrivileges(t, pool, runtime, "api")
	applyEmployerBindings(t, pool, runtime, operator)
	restricted, err := pgxpool.New(context.Background(), serviceRoleURL(t, pool, runtime))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(restricted.Close)
	engine := employerEngine(s, restricted)
	token := employerToken(t, s, a, auth.RoleHRReviewer)
	assertIDs := func(want []string) {
		t.Helper()
		w := employerCall(engine, token, "GET", "/v1/employer/employers?principal_id="+b+"&employer_id=employer-b", "")
		if w.Code != 200 || w.Header().Get("Cache-Control") != "private, no-store" {
			t.Fatalf("discovery: %d %s", w.Code, w.Body.String())
		}
		var data struct {
			Items []struct {
				ID string `json:"employer_id"`
			} `json:"items"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil {
			t.Fatal(err)
		}
		if data.Items == nil {
			t.Fatal("null empty list")
		}
		ids := make([]string, 0)
		for _, item := range data.Items {
			ids = append(ids, item.ID)
		}
		if !reflect.DeepEqual(ids, want) {
			t.Fatalf("got %v want %v", ids, want)
		}
	}
	grantEmployer(t, pool, b, "employer-b")
	assertIDs([]string{})
	grant := grantEmployer(t, pool, a, "cei-uat-sim-001")
	assertIDs([]string{"cei-uat-sim-001"})
	grantEmployer(t, pool, a, "019a5ace-3b00-7000-8000-000000000001")
	assertIDs([]string{"019a5ace-3b00-7000-8000-000000000001", "cei-uat-sim-001"})
	revokeEmployer(t, pool, grant)
	assertIDs([]string{"019a5ace-3b00-7000-8000-000000000001"})
	if w := employerCall(engine, token, "GET", "/v1/employer/requests?employer_id=cei-uat-sim-001", ""); w.Code != 404 {
		t.Fatal("revoked employer queue was accessible", w.Code)
	}
	if w := employerCall(engine, token, "GET", "/v1/employer/requests?employer_id=employer-b", ""); w.Code != 404 {
		t.Fatal("foreign employer queue was accessible", w.Code)
	}
	for _, change := range []string{"status='disabled'", "status='active',role='requestor'"} {
		if _, err := s.db.Exec(`UPDATE domain_users SET `+change+` WHERE id=$1`, a); err != nil {
			t.Fatal(err)
		}
		assertIDs([]string{}) // Old JWT cannot restore the current principal's rights.
	}
	// Discovery and denied operations cannot rewrite identities/history.
	var employer string
	var events int
	if err := s.db.QueryRow(`SELECT employer_id FROM evt_requests WHERE request_id='00000000-0000-4000-8000-00000000000a'`).Scan(&employer); err != nil || employer != "employer-a" {
		t.Fatal("historical request changed", err)
	}
	if err := s.db.QueryRow(`SELECT count(*) FROM evt_request_events`).Scan(&events); err != nil || events != 0 {
		t.Fatal("discovery mutated audit", err)
	}
}
