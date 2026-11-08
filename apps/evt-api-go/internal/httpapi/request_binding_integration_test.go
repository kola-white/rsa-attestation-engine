package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/auth"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/authority"
	evtdb "github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/db"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/evt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/requestbinding"
)

func bindingSetup(t *testing.T) (evidenceFixture, string, string) {
	t.Helper()
	f := evidenceSetup(t)
	ctx := context.Background()
	evidenceExec(t, f.pool, employerSQL(t, "../../migrations/000017_evidence_review.sql"))
	apply := func(file string) {
		tx, err := f.pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if _, err = tx.Exec(ctx, `SELECT set_config('cvera.evidence_runtime_role',$1,true),set_config('cvera.evidence_operator_role',$2,true)`, f.runtime, f.operator); err != nil {
			t.Fatal(err)
		}
		if _, err = tx.Exec(ctx, employerSQL(t, file)); err != nil {
			t.Fatal(err)
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	apply("../../operations/evidence-review-privileges.sql")
	var schema string
	if err := f.pool.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema); err != nil {
		t.Fatal(err)
	}
	evidenceExec(t, f.pool, `ALTER DEFAULT PRIVILEGES IN SCHEMA `+pgx.Identifier{schema}.Sanitize()+` GRANT EXECUTE ON FUNCTIONS TO `+pgx.Identifier{f.operator}.Sanitize())
	evidenceExec(t, f.pool, employerSQL(t, "../../migrations/000018_request_evidence_binding.sql"))
	apply("../../operations/request-evidence-binding-privileges.sql")
	applyEmployerBindings(t, f.pool, f.runtime, f.operator)
	evidenceExec(t, f.pool, `INSERT INTO evt_employer_config(employer_id) VALUES('binding-employer')`)
	request := uuid.NewString()
	evidenceExec(t, f.pool, `INSERT INTO evt_requests(request_id,candidate_personid,employer_id,status,claim_snapshot) VALUES($1,$2,'binding-employer','ATTESTATION_PENDING','{"title":"Engineer"}')`, request, f.principal)
	grantEmployer(t, f.pool, f.other, "binding-employer")
	evidenceExec(t, f.pool, `GRANT SELECT,UPDATE ON evt_requests TO `+pgx.Identifier{f.runtime}.Sanitize())
	binding := f.adminString(t, `SELECT bind_request_evidence($1,'case-a',ARRAY['employment.company_and_dates'],'fixture','approved bound request')`, request)
	return f, request, binding
}
func prepareBinding(t *testing.T, f evidenceFixture, request string, source requestbinding.LockedAuthoritySource) requestbinding.Result {
	t.Helper()
	ctx := context.Background()
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SET LOCAL ROLE `+pgx.Identifier{f.runtime}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	result, err := requestbinding.Prepare(ctx, tx, f.other, request, "binding-employer", source)
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return result
}
func boundReceipt(t *testing.T, f evidenceFixture, binding string) EvidenceInitResponse {
	t.Helper()
	m := &memoryEvidence{objects: map[string][]byte{}, mime: "application/pdf"}
	init := completionInit(t, f, m)
	evidenceStatus(t, completionRequest(f, f.principal, init, "/v1/evidence/commit"), 200)
	f.adminString(t, `SELECT grant_evidence_review($1,'case-a','fixture','independent bound review')`, f.other)
	list, _, review := reviewPaths(init)
	evidenceStatus(t, reviewRequest(f, f.other, "POST", review, `{"decision":"APPROVED","reason":"missing context"}`), 400)
	var listing struct {
		Binding struct {
			BindingID string `json:"bindingId"`
			Digest    string `json:"claimDigest"`
			Subject   string `json:"subjectId"`
		}
	}
	response := reviewRequest(f, f.other, "GET", list, "")
	evidenceStatus(t, response, 200)
	if err := json.Unmarshal(response.Body.Bytes(), &listing); err != nil {
		t.Fatal(err)
	}
	if listing.Binding.BindingID != binding || listing.Binding.Subject != f.principal {
		t.Fatal("wrong review context", response.Body.String())
	}
	body, _ := json.Marshal(map[string]string{"decision": "APPROVED", "reason": "Reviewed bound subject and claim", "bindingId": binding, "claimDigest": listing.Binding.Digest})
	evidenceStatus(t, reviewRequest(f, f.other, "POST", review, string(body)), 200)
	return init
}
func TestRequestBindingManifestAndUnavailableAuthority(t *testing.T) {
	f, request, binding := bindingSetup(t)
	if result := prepareBinding(t, f, request, nil); result.EvidenceReady || result.Eligible || result.Reason != "REQUIRED_EVIDENCE_MISSING" {
		t.Fatal(result)
	}
	boundReceipt(t, f, binding)
	result := prepareBinding(t, f, request, nil)
	if !result.EvidenceReady || result.Eligible || result.SigningEnabled || result.Reason != "AUTHORITY_SOURCE_UNAVAILABLE" || result.ManifestID == "" || result.PendingScanCount != 1 || len(result.ScanStatuses) != 1 || result.ScanStatuses[0] != "PENDING" {
		t.Fatal(result)
	}
	again := prepareBinding(t, f, request, nil)
	if again.ManifestID != result.ManifestID {
		t.Fatal("manifest not idempotent")
	}
	var scan string
	if err := f.pool.QueryRow(context.Background(), `SELECT scan_status FROM evidence_scan_jobs`).Scan(&scan); err != nil || scan != "PENDING" {
		t.Fatal(scan, err)
	}
	var historicalGrant string
	if err := f.pool.QueryRow(context.Background(), `SELECT grant_id FROM case_evidence_review_grants WHERE principal_id=$1 AND case_id='case-a' AND revoked_at IS NULL`, f.other).Scan(&historicalGrant); err != nil {
		t.Fatal(err)
	}
	f.admin(t, `SELECT revoke_evidence_review($1,'fixture','access ended; history retained')`, historicalGrant)
	historical := prepareBinding(t, f, request, nil)
	if !historical.EvidenceReady || historical.ManifestID != result.ManifestID {
		t.Fatal("access revocation erased historical approval", historical)
	}
	f.admin(t, `SELECT revoke_request_evidence_binding($1,'fixture','withdrawn')`, binding)
	if r := prepareBinding(t, f, request, nil); r.EvidenceReady || r.Eligible || r.Reason != "BINDING_REVOKED" {
		t.Fatal(r)
	}
}

func bindingRuntimePool(t *testing.T, f evidenceFixture) *pgxpool.Pool {
	t.Helper()
	cfg := f.pool.Config().Copy()
	cfg.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
		_, err := c.Exec(ctx, `SET ROLE `+pgx.Identifier{f.runtime}.Sanitize())
		return err
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}
func TestRequestBindingHTTPAndNoSigning(t *testing.T) {
	f, request, binding := bindingSetup(t)
	boundReceipt(t, f, binding)
	engine := employerEngine(f.s, bindingRuntimePool(t, f))
	path := "/v1/employer/requests/" + request + "/evidence-eligibility"
	body := `{"employer_id":"binding-employer","authorityDecision":"AUTHORIZED","issuerId":"https://other.invalid","subjectId":"spoof"}`
	for _, tc := range []struct {
		token  string
		status int
	}{
		{"", 401}, {employerToken(t, f.s, f.principal, auth.RoleHRReviewer), 404},
		{employerToken(t, f.s, f.other, auth.RoleRequestor), 403},
		{employerToken(t, f.s, f.other, auth.RoleHRReviewer), 200},
	} {
		response := employerCall(engine, tc.token, "POST", path, body)
		evidenceStatus(t, response, tc.status)
		if tc.status == 200 {
			var result requestbinding.Result
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || result.Eligible || !result.EvidenceReady || result.SigningEnabled || result.Reason != "AUTHORITY_SOURCE_UNAVAILABLE" {
				t.Fatal(result, err)
			}
		}
	}
	var count int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM evt_requests WHERE attestation_jws IS NOT NULL`).Scan(&count); err != nil || count != 0 {
		t.Fatal("preparation signed", count, err)
	}
	evidenceExec(t, f.pool, `UPDATE domain_users SET status='disabled' WHERE id=$1`, f.other)
	evidenceStatus(t, employerCall(engine, employerToken(t, f.s, f.other, auth.RoleHRReviewer), "POST", path, body), 404)
}
func TestRequestBindingContextStateAndCrossReuse(t *testing.T) {
	for _, change := range []string{"claim", "version", "null-version", "subject", "case", "check", "rejected", "missing-scan", "infected", "error", "pending-review", "incomplete"} {
		t.Run(change, func(t *testing.T) {
			f, request, binding := bindingSetup(t)
			init := boundReceipt(t, f, binding)
			want := "EVIDENCE_NOT_APPROVED"
			switch change {
			case "claim":
				evidenceExec(t, f.pool, `UPDATE evt_requests SET claim_snapshot='{"title":"Different person claim"}' WHERE request_id=$1`, request)
				want = "REQUEST_CONTEXT_CHANGED"
			case "version":
				evidenceExec(t, f.pool, `UPDATE evt_requests SET version=version+1 WHERE request_id=$1`, request)
				want = "REQUEST_CONTEXT_CHANGED"
			case "null-version":
				evidenceExec(t, f.pool, `UPDATE evt_requests SET version=NULL WHERE request_id=$1`, request)
				want = "REQUEST_CONTEXT_CHANGED"
			case "subject":
				evidenceExec(t, f.pool, `UPDATE evt_requests SET candidate_personid=$1 WHERE request_id=$2`, f.other, request)
				want = "REQUEST_CONTEXT_CHANGED"
			case "case":
				f.admin(t, `SELECT set_evidence_resource_state('case-a',NULL,'CLOSED','fixture','closed')`)
				want = "CASE_NOT_OPEN"
			case "check":
				f.admin(t, `SELECT set_evidence_resource_state('case-a','employment.company_and_dates','FAILED','fixture','failed')`)
				want = "CHECK_NOT_ELIGIBLE"
			case "missing-scan":
				evidenceExec(t, f.pool, `DELETE FROM evidence_scan_jobs`)
			case "infected", "error":
				evidenceExec(t, f.pool, `UPDATE evidence_scan_jobs SET scan_status=$1`, strings.ToUpper(change))
			case "pending-review":
				evidenceExec(t, f.pool, `TRUNCATE evidence_reviews,request_evidence_review_contexts`)
			case "rejected":
				evidenceExec(t, f.pool, `UPDATE evidence_reviews SET decision='REJECTED'`)
			case "incomplete":
				evidenceStatus(t, f.request(f.principal, "case-a", "employment.company_and_dates", false, evidenceFile), 200)
				want = "UPLOAD_INCOMPLETE"
			}
			result := prepareBinding(t, f, request, nil)
			if result.Eligible || result.EvidenceReady || result.Reason != want {
				t.Fatal(change, result)
			}
			_ = init
		})
	}
	f, request, binding := bindingSetup(t)
	init := boundReceipt(t, f, binding)
	second := uuid.NewString()
	evidenceExec(t, f.pool, `INSERT INTO evt_requests(request_id,candidate_personid,employer_id,status) VALUES($1,$2,'binding-employer','ATTESTATION_PENDING')`, second, f.principal)
	// An existing case cannot be reassigned even after binding revocation.
	tx, err := f.pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(context.Background(), `SELECT bind_request_evidence($1,'case-a',ARRAY['employment.company_and_dates'],'fixture','reuse')`, second)
	expectEmployerSQLState(t, err, "23514")
	tx.Rollback(context.Background())
	if r := prepareBinding(t, f, second, nil); r.EvidenceReady || r.Eligible || r.Reason != "REQUEST_NOT_BOUND" {
		t.Fatal(r)
	}
	secondBinding := f.adminString(t, `SELECT bind_request_evidence($1,'case-b',ARRAY['employment.company_and_dates'],'fixture','independent request')`, second)
	f.adminString(t, `SELECT grant_evidence_review($1,'case-b','fixture','review')`, f.other)
	var digest string
	f.pool.QueryRow(context.Background(), `SELECT claim_digest FROM request_case_bindings WHERE binding_id=$1`, secondBinding).Scan(&digest)
	body := fmt.Sprintf(`{"decision":"APPROVED","reason":"reuse","bindingId":%q,"claimDigest":%q}`, secondBinding, digest)
	path := "/v1/cases/case-b/checks/employment.company_and_dates/evidence/" + init.Uploads[0].UploadID + "/review"
	evidenceStatus(t, reviewRequest(f, f.other, "POST", path, body), 404)
	if r := prepareBinding(t, f, second, nil); r.EvidenceReady || r.Reason != "REQUIRED_EVIDENCE_MISSING" {
		t.Fatal(r)
	}
	_ = request
}
func TestRequestBindingPrivilegesAndImmutability(t *testing.T) {
	f, request, binding := bindingSetup(t)
	boundReceipt(t, f, binding)
	prepareBinding(t, f, request, nil)
	for _, table := range []string{"request_binding_policy", "request_case_bindings", "request_binding_checks", "request_evidence_review_contexts", "request_evidence_manifests"} {
		for _, role := range []string{f.runtime, f.operator} {
			var allowed bool
			if err := f.pool.QueryRow(context.Background(), `SELECT has_any_column_privilege($1,$2,'SELECT') OR has_any_column_privilege($1,$2,'INSERT') OR has_any_column_privilege($1,$2,'UPDATE') OR has_table_privilege($1,$2,'DELETE,TRUNCATE')`, role, table).Scan(&allowed); err != nil || allowed {
				t.Fatal(table, role, allowed, err)
			}
		}
		_, err := f.pool.Exec(context.Background(), `DELETE FROM `+pgx.Identifier{table}.Sanitize())
		expectEmployerSQLState(t, err, "23514")
	}
	_, err := f.s.db.Exec(`SELECT bind_request_evidence($1,'case-b',ARRAY['employment.company_and_dates'],'runtime','not allowed')`, request)
	expectEmployerSQLState(t, err, "42501")
	_, err = f.s.db.Exec(`SELECT revoke_request_evidence_binding($1,'runtime','not allowed')`, binding)
	expectEmployerSQLState(t, err, "42501")
	_, err = f.s.db.Exec(`SELECT record_unbound_evidence_review($1,'case-a','employment.company_and_dates',$2,'APPROVED','bypass')`, f.other, uuid.NewString())
	expectEmployerSQLState(t, err, "42501")
	f.admin(t, `SELECT revoke_request_evidence_binding($1,'fixture','revoked')`, binding)
	_, err = f.pool.Exec(context.Background(), `UPDATE request_case_bindings SET revoked_at=NULL,revoked_by=NULL,revocation_reason=NULL,revoked_session_user=NULL WHERE binding_id=$1`, binding)
	expectEmployerSQLState(t, err, "23514")
}
func seedBindingAuthority(t *testing.T, f evidenceFixture, issuer, employer string, activate ...bool) (*evtdb.AuthorityRepository, string) {
	t.Helper()
	files, err := filepath.Glob("../../migrations/*.sql")
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	for _, file := range files {
		if filepath.Base(file) < "000014_" {
			evidenceExec(t, f.pool, employerSQL(t, file))
		}
	}
	repo := evtdb.NewAuthorityRepository(f.pool)
	id := uuid.NewString()
	ctx := context.Background()
	record := authority.AuthorizationRecord{AuthorizationID: id, IssuerID: issuer, AuthoritySubjectID: employer, ClaimType: "employment.role"}
	if err = repo.CreateAuthorization(ctx, record); err != nil {
		t.Fatal(err)
	}
	start := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	decision := authority.AuthorizationAdjudication{DecisionID: uuid.NewString(), AuthorizationID: id, IssuerID: issuer, AuthoritySubjectID: employer, ClaimType: "employment.role", Outcome: authority.AdjudicationApproved, DecisionMakerID: "synthetic-adjudicator", AuthorityBasis: "synthetic employer delegation", EvidenceRef: "synthetic-proof", DecidedAt: start.Add(time.Minute), RecordedAt: start.Add(time.Minute)}
	if err = repo.CreateAdjudication(ctx, decision); err != nil {
		t.Fatal(err)
	}
	events := []authority.AuthorizationEvent{authority.EventSubmit, authority.EventApprove, authority.EventActivate}
	if len(activate) > 0 && !activate[0] {
		events = events[:2]
	}
	for i, event := range events {
		at := start.Add(time.Duration(i) * time.Minute)
		if err = repo.AppendJournalEvent(ctx, authority.AuthorizationJournalEvent{EventID: uuid.NewString(), AuthorizationID: id, EventType: event, EffectiveAt: at, RecordedAt: at, ActorID: "synthetic-adjudicator", EvidenceRef: "synthetic-proof", ConditionsSatisfied: true}); err != nil {
			t.Fatal(err)
		}
	}
	return repo, id
}
func TestRequestBindingExactActiveAuthority(t *testing.T) {
	for _, condition := range []string{"active", "wrong-issuer", "wrong-employer", "suspended", "revoked", "expired", "approved-only", "incomplete-snapshot"} {
		t.Run(condition, func(t *testing.T) {
			f, request, binding := bindingSetup(t)
			boundReceipt(t, f, binding)
			issuer, employer := requestbinding.IssuerID, "binding-employer"
			if condition == "wrong-issuer" {
				issuer = "https://other.invalid"
			}
			if condition == "wrong-employer" {
				employer = "different-employer"
			}
			repo, id := seedBindingAuthority(t, f, issuer, employer, condition != "approved-only")
			if condition == "suspended" || condition == "revoked" || condition == "expired" {
				eventType := authority.EventSuspend
				if condition == "revoked" {
					eventType = authority.EventRevoke
				}
				if condition == "expired" {
					eventType = authority.EventExpire
				}
				now := time.Now().UTC().Add(-time.Second).Truncate(time.Microsecond)
				if err := repo.AppendJournalEvent(context.Background(), authority.AuthorizationJournalEvent{EventID: uuid.NewString(), AuthorizationID: id, EventType: eventType, EffectiveAt: now, RecordedAt: now, ActorID: "synthetic-operator", EvidenceRef: "suspended", ConditionsSatisfied: true}); err != nil {
					t.Fatal(err)
				}
			}
			// Positive authority exercises the actual trusted repository/engine under
			// a test owner transaction. Production runtime receives NO authority grants.
			tx, err := f.pool.Begin(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(context.Background())
			source := requestbinding.LockedAuthoritySource(evtdb.LoadLockedAuthoritySnapshot)
			if condition == "incomplete-snapshot" {
				source = func(context.Context, pgx.Tx) (authority.AuthoritySnapshot, error) {
					return authority.AuthoritySnapshot{}, nil
				}
			}
			result, err := requestbinding.Prepare(context.Background(), tx, f.other, request, "binding-employer", source)
			if condition == "incomplete-snapshot" {
				if err == nil {
					t.Fatal("incomplete authority accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if result.Eligible != (condition == "active") || result.SigningEnabled {
				t.Fatal(condition, result)
			}
			if condition == "active" && result.AuthorityGrantID == "" {
				t.Fatal("unattributed authority")
			}
			if err = tx.Commit(context.Background()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRequestBindingLegacySigningCannotBypass(t *testing.T) {
	f, request, binding := bindingSetup(t)
	boundReceipt(t, f, binding)
	engine := employerEngine(f.s, bindingRuntimePool(t, f))
	token := employerToken(t, f.s, f.other, auth.RoleHRReviewer)
	for _, revoked := range []bool{false, true} {
		if revoked {
			f.admin(t, `SELECT revoke_request_evidence_binding($1,'fixture','withdrawn')`, binding)
		}
		for _, kind := range []string{"FULL_MATCH", "PARTIAL_MATCH"} {
			response := employerCall(engine, token, "POST", "/v1/employer/requests/"+request+"/attest", `{"employer_id":"binding-employer","response_type":"`+kind+`"}`)
			evidenceStatus(t, response, 409)
			if !strings.Contains(response.Body.String(), "evidence_backed_signing_not_enabled") {
				t.Fatal(response.Body.String())
			}
		}
	}
	var state string
	if err := f.pool.QueryRow(context.Background(), `SELECT status::text FROM evt_requests WHERE request_id=$1`, request).Scan(&state); err != nil || state != "ATTESTATION_PENDING" {
		t.Fatal("legacy route mutated bound request", state, err)
	}
}
func TestRequestBindingRequiredChecksAndStandaloneReview(t *testing.T) {
	f, _, _ := bindingSetup(t)
	// Existing standalone case review still works with no request context.
	f.adminString(t, `SELECT grant_evidence_upload($1,'case-b','fixture','standalone')`, f.principal)
	f.adminString(t, `SELECT grant_evidence_review($1,'case-b','fixture','standalone review')`, f.other)
	response := f.request(f.principal, "case-b", "employment.company_and_dates", false, evidenceFile)
	evidenceStatus(t, response, 200)
	var init EvidenceInitResponse
	if err := json.Unmarshal(response.Body.Bytes(), &init); err != nil {
		t.Fatal(err)
	}
	m := &memoryEvidence{objects: map[string][]byte{init.Uploads[0].StorageKey: []byte("b")}, mime: "application/pdf"}
	f.s.s3 = m
	evidenceStatus(t, completionRequest(f, f.principal, init, "/v1/evidence/commit"), 200)
	base := "/v1/cases/case-b/checks/employment.company_and_dates/evidence"
	evidenceStatus(t, reviewRequest(f, f.other, "GET", base, ""), 200)
	evidenceStatus(t, reviewRequest(f, f.other, "POST", base+"/"+init.Uploads[0].UploadID+"/review", `{"decision":"APPROVED","reason":"standalone"}`), 200)
	// A started standalone case cannot later be adopted into a request.
	request := uuid.NewString()
	evidenceExec(t, f.pool, `INSERT INTO evt_requests(request_id,candidate_personid,employer_id,status) VALUES($1,$2,'binding-employer','ATTESTATION_PENDING')`, request, f.principal)
	_, err := f.pool.Exec(context.Background(), `SELECT bind_request_evidence($1,'case-b',ARRAY['employment.company_and_dates'],'fixture','retroactive')`, request)
	expectEmployerSQLState(t, err, "23514")
}
func TestRequestBindingConcurrencyAndDeletion(t *testing.T) {
	for _, first := range []string{"prepare", "revoke", "state", "delete"} {
		t.Run(first, func(t *testing.T) {
			f, request, binding := bindingSetup(t)
			boundReceipt(t, f, binding)
			if first == "delete" {
				tx, err := f.pool.Begin(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				defer tx.Rollback(context.Background())
				deletionSettings(t, tx, uuid.NewString(), "approved fixture deletion", "")
				if _, err = tx.Exec(context.Background(), employerSQL(t, "../../operations/evidence-case-delete-freeze.sql")); err != nil {
					t.Fatal(err)
				}
				if err = tx.Commit(context.Background()); err != nil {
					t.Fatal(err)
				}
				if r := prepareBinding(t, f, request, nil); r.Reason != "BINDING_REVOKED" || r.EvidenceReady {
					t.Fatal(r)
				}
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			a, err := f.pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer a.Rollback(context.Background())
			b, err := f.pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer b.Rollback(context.Background())
			change := `SELECT revoke_request_evidence_binding($1,'fixture','revoked')`
			arg := any(binding)
			if first == "state" {
				change = `SELECT set_evidence_resource_state('case-a',NULL,'CLOSED','fixture','closed')`
				arg = nil
			}
			changeTx := func(tx pgx.Tx) error {
				if arg == nil {
					_, err := tx.Exec(ctx, change)
					return err
				}
				_, err := tx.Exec(ctx, change, arg)
				return err
			}
			done := make(chan error, 1)
			var result requestbinding.Result
			if first == "prepare" {
				if result, err = requestbinding.Prepare(ctx, a, f.other, request, "binding-employer", nil); err != nil || !result.EvidenceReady {
					t.Fatal(result, err)
				}
				go func() { done <- changeTx(b) }()
			} else {
				if err = changeTx(a); err != nil {
					t.Fatal(err)
				}
				go func() {
					var err error
					result, err = requestbinding.Prepare(ctx, b, f.other, request, "binding-employer", nil)
					done <- err
				}()
			}
			waitEmployerBlocked(t, f.pool, a.Conn().PgConn().PID(), b.Conn().PgConn().PID())
			if err = a.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			if err = <-done; err != nil {
				t.Fatal(err)
			}
			if first != "prepare" && result.EvidenceReady {
				t.Fatal("stale state accepted", result)
			}
			if err = b.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			if r := prepareBinding(t, f, request, nil); r.EvidenceReady || r.Eligible {
				t.Fatal("revocation/state ignored", r)
			}
		})
	}
}
func TestRequestBindingAuthorityRecordingLock(t *testing.T) {
	for _, first := range []string{"prepare", "revoke"} {
		t.Run(first, func(t *testing.T) {
			f, request, binding := bindingSetup(t)
			boundReceipt(t, f, binding)
			_, id := seedBindingAuthority(t, f, requestbinding.IssuerID, "binding-employer")
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			a, err := f.pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer a.Rollback(context.Background())
			b, err := f.pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer b.Rollback(context.Background())
			appendRevoke := func(tx pgx.Tx) error {
				var position int64
				if err := tx.QueryRow(ctx, `SELECT next_authority_recording_position()`).Scan(&position); err != nil {
					return err
				}
				_, err := tx.Exec(ctx, `INSERT INTO authority_journal_events(event_id,authorization_id,event_type,effective_at,recorded_at,actor_id,evidence_ref,conditions_satisfied,recording_position) VALUES($1,$2,'REVOKE',clock_timestamp(),clock_timestamp(),'synthetic-operator','withdrawn',true,$3)`, uuid.NewString(), id, position)
				return err
			}
			var result requestbinding.Result
			done := make(chan error, 1)
			if first == "prepare" {
				if result, err = requestbinding.Prepare(ctx, a, f.other, request, "binding-employer", evtdb.LoadLockedAuthoritySnapshot); err != nil || !result.Eligible {
					t.Fatal(result, err)
				}
				go func() { done <- appendRevoke(b) }()
			} else {
				if err = appendRevoke(a); err != nil {
					t.Fatal(err)
				}
				go func() {
					var err error
					result, err = requestbinding.Prepare(ctx, b, f.other, request, "binding-employer", evtdb.LoadLockedAuthoritySnapshot)
					done <- err
				}()
			}
			waitEmployerBlocked(t, f.pool, a.Conn().PgConn().PID(), b.Conn().PgConn().PID())
			if err = a.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			if err = <-done; err != nil {
				t.Fatal(err)
			}
			if first == "revoke" && (result.Eligible || result.AuthorityDecision != authority.AuthorityUnauthorized) {
				t.Fatal("stale authority", result)
			}
			if err = b.Commit(ctx); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestRequestBindingRejectsStaleReviewContextAndFailureRollback(t *testing.T) {
	f, request, binding := bindingSetup(t)
	m := &memoryEvidence{objects: map[string][]byte{}, mime: "application/pdf"}
	init := completionInit(t, f, m)
	evidenceStatus(t, completionRequest(f, f.principal, init, "/v1/evidence/commit"), 200)
	f.adminString(t, `SELECT grant_evidence_review($1,'case-a','fixture','review')`, f.other)
	_, _, path := reviewPaths(init)
	body := fmt.Sprintf(`{"decision":"APPROVED","reason":"wrong context","bindingId":%q,"claimDigest":%q}`, binding, strings.Repeat("a", 64))
	evidenceStatus(t, reviewRequest(f, f.other, "POST", path, body), 400)
	var count int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM evidence_reviews`).Scan(&count); err != nil || count != 0 {
		t.Fatal("bad acknowledgement wrote receipt", count, err)
	}
	// A binding/claim mutation is ineligible, never a silent context replacement.
	evidenceExec(t, f.pool, `UPDATE evt_requests SET claim_snapshot='{"title":"private changed claim"}' WHERE request_id=$1`, request)
	response := reviewRequest(f, f.other, "GET", strings.TrimSuffix(path, "/"+init.Uploads[0].UploadID+"/review"), "")
	evidenceStatus(t, response, 200)
	if strings.Contains(response.Body.String(), "private changed claim") {
		t.Fatal("changed claim leaked through old binding")
	}
}

func TestRequestBindingMigrationAtomicFailure(t *testing.T) {
	f := evidenceSetup(t)
	ctx := context.Background()
	evidenceExec(t, f.pool, employerSQL(t, "../../migrations/000017_evidence_review.sql"))
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `SELECT set_config('cvera.evidence_runtime_role',$1,true),set_config('cvera.evidence_operator_role',$2,true)`, f.runtime, f.operator); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, employerSQL(t, "../../operations/evidence-review-privileges.sql")); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	// Hostile inherited EXECUTE must not survive the actual migration.
	evidenceExec(t, f.pool, `ALTER DEFAULT PRIVILEGES IN SCHEMA `+pgx.Identifier{f.pool.Config().ConnConfig.RuntimeParams["search_path"]}.Sanitize()+` GRANT EXECUTE ON FUNCTIONS TO `+pgx.Identifier{f.runtime}.Sanitize())
	tx, err = f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, employerSQL(t, "../../migrations/000018_request_evidence_binding.sql")); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `SELECT set_config('cvera.evidence_runtime_role',$1,true),set_config('cvera.evidence_operator_role','nonexistent_binding_operator',true)`, f.runtime); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, employerSQL(t, "../../operations/request-evidence-binding-privileges.sql")); err == nil {
		t.Fatal("bad binding committed")
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	var absent bool
	if err = f.pool.QueryRow(ctx, `SELECT to_regclass('request_case_bindings') IS NULL AND to_regprocedure('record_unbound_evidence_review(uuid,text,text,uuid,text,text)') IS NULL`).Scan(&absent); err != nil || !absent {
		t.Fatal("partial DDL escaped transaction", err)
	}
	m := &memoryEvidence{objects: map[string][]byte{}, mime: "application/pdf"}
	init := completionInit(t, f, m)
	evidenceStatus(t, completionRequest(f, f.principal, init, "/v1/evidence/commit"), 200)
	f.adminString(t, `SELECT grant_evidence_review($1,'case-a','fixture','standalone')`, f.other)
	_, _, path := reviewPaths(init)
	evidenceStatus(t, reviewRequest(f, f.other, "POST", path, `{"decision":"APPROVED","reason":"still standalone"}`), 200)
}

func TestRequestBindingUnboundLegacyRemainsFunctional(t *testing.T) {
	f, _, _ := bindingSetup(t)
	ctx := context.Background()
	key, err := rsa.GenerateKey(rand.Reader, 3072)
	if err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(t.TempDir(), "binding-unbound-fixture.pem")
	if err = os.WriteFile(filename, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}), 0600); err != nil {
		t.Fatal(err)
	}
	signer, err := evt.NewAttestationSignerFromPEMPath(filename)
	if err != nil {
		t.Fatal(err)
	}
	request := uuid.NewString()
	evidenceExec(t, f.pool, `INSERT INTO evt_requests(request_id,candidate_personid,employer_id,status) VALUES($1,$2,'binding-employer','ATTESTATION_PENDING')`, request, f.principal)
	evidenceExec(t, f.pool, `GRANT INSERT ON evt_request_events TO `+pgx.Identifier{f.runtime}.Sanitize())
	pool := bindingRuntimePool(t, f)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	repo := &evt.Repo{Signer: signer}
	result, err := repo.EmployerAttestServerSigned(ctx, tx, request, "binding-employer", f.other, "FULL_MATCH", json.RawMessage(`{"synthetic":true}`))
	if err != nil || !result.AttestationPresent || result.Status != "ATTESTED" {
		t.Fatal(result, err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var bound bool
	if err = f.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM request_case_bindings WHERE request_id=$1)`, request).Scan(&bound); err != nil || bound {
		t.Fatal("legacy path invented a binding", err)
	}
}
