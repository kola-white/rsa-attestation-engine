package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/auth"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/authority"
	evtdb "github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/db"
	ec "github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/evidencecredential"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/evt"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/requestbinding"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/verification"
)

type issuedFixture struct {
	f                evidenceFixture
	request, binding string
	engine           *evtdb.IssuanceEngine
	authority        *evtdb.AuthorityRepository
	authorityID      string
}

func issuanceSetup(t *testing.T, activate ...bool) issuedFixture {
	t.Helper()
	f, request, binding := bindingSetup(t, `{"primary_employment":{"title":"Engineer","issuer_name":"Fixture Employer","start_date":"2020-01-01"}}`)
	boundReceipt(t, f, binding)
	// Existing api-runtime-privileges.sql grants this legacy SELECT in production.
	evidenceExec(t, f.pool, `GRANT SELECT ON domain_users TO `+pgx.Identifier{f.runtime}.Sanitize())
	evidenceExec(t, f.pool, `UPDATE domain_users SET role='requestor' WHERE id=$1`, f.principal)
	evidenceExec(t, f.pool, employerSQL(t, "../../migrations/000019_evidence_backed_issuance.sql"))
	tx, err := f.pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(context.Background(), `SELECT set_config('cvera.evidence_runtime_role',$1,true)`, f.runtime); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(context.Background(), employerSQL(t, "../../operations/evidence-issuance-privileges.sql")); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	repo, id := seedBindingAuthority(t, f, ec.Issuer, "binding-employer")
	// Explicit local-only fixture: reproduce restricted production roles without
	// inheriting the hostile CRUD defaults exercised by the migration fixtures.
	evidenceExec(t, f.pool, `REVOKE ALL ON authority_authorizations,authority_adjudications,authority_journal_events,authority_recording_boundary FROM `+pgx.Identifier{f.runtime}.Sanitize())
	key, err := rsa.GenerateKey(rand.Reader, 3072)
	if err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(t.TempDir(), "issuance-local-fixture.pem")
	if err = os.WriteFile(filename, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}), 0600); err != nil {
		t.Fatal(err)
	}
	signer, err := evt.NewAttestationSignerFromPEMPath(filename)
	if err != nil {
		t.Fatal(err)
	}
	pub, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}

	if len(activate) == 0 || activate[0] {
		registration, err := f.pool.Begin(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		defer registration.Rollback(context.Background())
		now := time.Now().UTC()
		settings := map[string]string{"kid": signer.KeyID(), "public_pem": string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pub})), "spki_sha256": ec.Fingerprint(&key.PublicKey), "valid_from": now.Add(-time.Hour).Format(time.RFC3339Nano), "valid_until": now.Add(24 * time.Hour).Format(time.RFC3339Nano), "claim_lifetime_seconds": "3600", "established_by": "local-test-owner", "basis_ref": "local-test-only"}
		for name, value := range settings {
			if _, err = registration.Exec(context.Background(), `SELECT set_config($1,$2,true)`, "cvera.credential_"+name, value); err != nil {
				t.Fatal(err)
			}
		}
		if _, err = registration.Exec(context.Background(), employerSQL(t, "../../operations/register-evidence-issuer-credential.sql")); err != nil {
			t.Fatal(err)
		}
		if err = registration.Commit(context.Background()); err != nil {
			t.Fatal(err)
		}

	}
	return issuedFixture{f: f, request: request, binding: binding, engine: &evtdb.IssuanceEngine{DB: &evtdb.DB{Pool: bindingRuntimePool(t, f)}, Signer: signer}, authority: repo, authorityID: id}
}
func issueAndVerify(t *testing.T, x issuedFixture) (evtdb.IssuanceResult, ec.Outcome) {
	t.Helper()
	issued, err := x.engine.Issue(context.Background(), x.f.other, x.request, "binding-employer")
	if err != nil {
		t.Fatal(err)
	}
	verifier := &evtdb.IssuanceEngine{DB: x.engine.DB} // deliberately no signer
	outcome := ec.Verify(context.Background(), issued.Credential, x.f.principal, ec.Policy, time.Now().UTC(), verifier)
	return issued, outcome
}
func TestEvidenceIssuanceTrustedIntegratedPath(t *testing.T) {
	x := issuanceSetup(t)
	issued, verified := issueAndVerify(t, x)
	if verified.Decision != verification.VerificationVerified || verified.Claim == nil || verified.Commitment == nil || verified.ScanPolicy != ec.ManualPolicy {
		t.Fatalf("%+v", verified)
	}
	if _, err := x.engine.Issue(context.Background(), x.f.other, x.request, "binding-employer"); err == nil {
		t.Fatal("second credential issued")
	}
	x.f.s.issuanceVerifier = &evtdb.IssuanceEngine{DB: x.engine.DB}
	body, _ := json.Marshal(map[string]string{"jwsCompact": issued.Credential, "expectedSubject": x.f.principal, "policy": ec.Policy})
	w := httptest.NewRecorder()
	x.f.s.HandleVerificationOutcome(w, httptest.NewRequest(http.MethodPost, "/v1/verification/outcome", strings.NewReader(string(body))))
	var result VerificationOutcome
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || result.Trust != "trusted" || result.Verification == nil || result.Verification.Decision != verification.VerificationVerified {
		t.Fatalf("HTTP outcome: status %d", w.Code)
	}
	// Owner-only claim revocation does not pretend that a signature stopped being
	// mathematically valid. The same public artifact must become untrusted.
	evidenceExec(t, x.f.pool, `SELECT revoke_bound_credential($1,'local-owner','withdrawn')`, issued.CredentialID)
	after := ec.Verify(context.Background(), issued.Credential, x.f.principal, ec.Policy, time.Now().UTC(), &evtdb.IssuanceEngine{DB: x.engine.DB})
	if after.Decision != verification.VerificationRejected || after.Signature != "valid" {
		t.Fatalf("revocation: %+v", after)
	}
}
func TestEvidenceIssuanceCurrentAuthorityAndEvidence(t *testing.T) {
	for _, condition := range []string{"authority-revoked", "authority-unavailable", "binding-revoked", "snapshot-changed", "scan-infected", "key-revoked", "cross-employer", "cross-user"} {
		t.Run(condition, func(t *testing.T) {
			x := issuanceSetup(t)
			principal, employer := x.f.other, "binding-employer"
			switch condition {
			case "authority-revoked":
				now := time.Now().UTC().Truncate(time.Microsecond)
				if err := x.authority.AppendJournalEvent(context.Background(), authority.AuthorizationJournalEvent{EventID: "local-revoke", AuthorizationID: x.authorityID, EventType: authority.EventRevoke, EffectiveAt: now, RecordedAt: now, ActorID: "local-owner", EvidenceRef: "withdrawn", ConditionsSatisfied: true}); err != nil {
					t.Fatal(err)
				}
			case "authority-unavailable":
				evidenceExec(t, x.f.pool, `ALTER TABLE authority_recording_boundary RENAME TO local_unavailable_boundary`)
			case "binding-revoked":
				x.f.admin(t, `SELECT revoke_request_evidence_binding($1,'local-owner','withdrawn')`, x.binding)
			case "snapshot-changed":
				evidenceExec(t, x.f.pool, `UPDATE evt_requests SET claim_snapshot='{}' WHERE request_id=$1`, x.request)
			case "scan-infected":
				evidenceExec(t, x.f.pool, `UPDATE evidence_scan_jobs SET scan_status='INFECTED'`)
			case "key-revoked":
				evidenceExec(t, x.f.pool, `UPDATE evidence_issuer_credentials SET revoked_at=clock_timestamp(),revoked_by='local-owner',revocation_reason='withdrawn'`)
			case "cross-employer":
				employer = "unrelated-employer"
			case "cross-user":
				principal = x.f.principal
			}
			got, err := x.engine.Issue(context.Background(), principal, x.request, employer)
			if err == nil || got.Credential != "" {
				t.Fatal("ineligible credential issued")
			}
			var count int
			if err = x.f.pool.QueryRow(context.Background(), `SELECT count(*) FROM evidence_issued_credentials`).Scan(&count); err != nil || count != 0 {
				t.Fatal("failed issuance persisted")
			}
		})
	}
}

func TestEvidenceIssuancePrivilegesAndAtomicMigration(t *testing.T) {
	x := issuanceSetup(t)
	ctx := context.Background()
	for _, role := range []string{x.f.runtime, x.f.operator} {
		for _, table := range []string{"evidence_issuer_credentials", "evidence_issued_credentials"} {
			var allowed bool
			if err := x.f.pool.QueryRow(ctx, `SELECT has_any_column_privilege($1,$2,'SELECT') OR has_any_column_privilege($1,$2,'INSERT') OR has_any_column_privilege($1,$2,'UPDATE') OR has_table_privilege($1,$2,'DELETE,TRUNCATE,TRIGGER,REFERENCES')`, role, table).Scan(&allowed); err != nil || allowed {
				t.Fatal("direct credential access", role, table, err)
			}
		}
	}
	for _, query := range []string{`SELECT * FROM evidence_issuer_credentials`, `UPDATE evidence_issuer_credentials SET revoked_at=clock_timestamp()`, `DELETE FROM evidence_issued_credentials`, `SELECT revoke_bound_credential('00000000-0000-4000-8000-000000000001','x','x')`, `SELECT guard_evidence_issuance()`} {
		_, err := x.f.s.db.Exec(query)
		expectEmployerSQLState(t, err, "42501")
	}
	var publicCount int
	if err := x.f.pool.QueryRow(ctx, `SELECT count(*) FROM pg_proc p,LATERAL aclexplode(coalesce(p.proacl,acldefault('f',p.proowner))) a WHERE p.pronamespace=current_schema()::regnamespace AND p.proname IN ('read_evidence_authority','prepare_bound_issuance','commit_bound_issuance','resolve_bound_credential','revoke_bound_credential','guard_evidence_issuance','protect_evidence_issuance') AND a.grantee=0`).Scan(&publicCount); err != nil || publicCount != 0 {
		t.Fatal("PUBLIC execute", err)
	}
	// Immutable key binding cannot be replaced by an UPDATE, even by its owner.
	_, err := x.f.pool.Exec(ctx, `UPDATE evidence_issuer_credentials SET spki_sha256=repeat('0',64)`)
	expectEmployerSQLState(t, err, "42501")
	f, _, _ := bindingSetup(t)
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, employerSQL(t, "../../migrations/000019_evidence_backed_issuance.sql")); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `SET LOCAL cvera.evidence_runtime_role='missing_runtime_role'`); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, employerSQL(t, "../../operations/evidence-issuance-privileges.sql")); err == nil {
		t.Fatal("unsafe runtime binding accepted")
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	var absent bool
	if err = f.pool.QueryRow(ctx, `SELECT to_regclass('evidence_issued_credentials') IS NULL AND to_regprocedure('read_evidence_authority()') IS NULL AND to_regprocedure('record_request_evidence_review(uuid,text,text,uuid,text,text,uuid,text)') IS NOT NULL`).Scan(&absent); err != nil || !absent {
		t.Fatal("partial migration escaped", err)
	}
}

type pausedEvidenceSigner struct {
	ec.Signer
	started chan struct{}
	release chan struct{}
}

func (s pausedEvidenceSigner) SignEvidenceClaims(ctx context.Context, c ec.Claims) (string, error) {
	close(s.started)
	select {
	case <-s.release:
		return s.Signer.SignEvidenceClaims(ctx, c)
	case <-ctx.Done():
		return "", ctx.Err()
	}
}
func TestEvidenceIssuanceRevocationConcurrency(t *testing.T) {
	for _, kind := range []string{"authority", "binding", "credential"} {
		for _, first := range []string{"issue", "revoke"} {
			t.Run(kind+"/"+first, func(t *testing.T) {
				x := issuanceSetup(t)
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				// Discover the single reused runtime connection's backend without reading
				// credentials. No concurrent pool operation is performed until issuance.
				conn, err := x.engine.DB.Pool.Acquire(ctx)
				if err != nil {
					t.Fatal(err)
				}
				issuePID := conn.Conn().PgConn().PID()
				conn.Release()
				revoker, err := x.f.pool.Begin(ctx)
				if err != nil {
					t.Fatal(err)
				}
				defer revoker.Rollback(context.Background())
				revokePID := revoker.Conn().PgConn().PID()
				revoke := func() error {
					switch kind {
					case "binding":
						_, err := revoker.Exec(ctx, `SELECT revoke_request_evidence_binding($1,'local-owner','withdrawn')`, x.binding)
						return err
					case "credential":
						_, err := revoker.Exec(ctx, `UPDATE evidence_issuer_credentials SET revoked_at=clock_timestamp(),revoked_by='local-owner',revocation_reason='withdrawn'`)
						return err
					default:
						var position int64
						if err := revoker.QueryRow(ctx, `SELECT next_authority_recording_position()`).Scan(&position); err != nil {
							return err
						}
						_, err := revoker.Exec(ctx, `INSERT INTO authority_journal_events(event_id,authorization_id,event_type,effective_at,recorded_at,actor_id,evidence_ref,conditions_satisfied,recording_position) VALUES($1,$2,'REVOKE',clock_timestamp(),clock_timestamp(),'local-adjudicator','withdrawn',true,$3)`, uuid.NewString(), x.authorityID, position)
						return err
					}
				}
				issueDone := make(chan error, 1)
				var issued evtdb.IssuanceResult
				if first == "issue" {
					pause := pausedEvidenceSigner{Signer: x.engine.Signer, started: make(chan struct{}), release: make(chan struct{})}
					x.engine.Signer = pause
					go func() {
						var err error
						issued, err = x.engine.Issue(ctx, x.f.other, x.request, "binding-employer")
						issueDone <- err
					}()
					select {
					case <-pause.started:
					case <-ctx.Done():
						t.Fatal("issuance never reached locked signer")
					}
					revokeDone := make(chan error, 1)
					go func() { revokeDone <- revoke() }()
					waitEmployerBlocked(t, x.f.pool, issuePID, revokePID)
					close(pause.release)
					if err = <-issueDone; err != nil {
						t.Fatal(err)
					}
					if err = <-revokeDone; err != nil {
						t.Fatal(err)
					}
					if err = revoker.Commit(ctx); err != nil {
						t.Fatal(err)
					}
					out := ec.Verify(ctx, issued.Credential, x.f.principal, ec.Policy, time.Now().UTC(), &evtdb.IssuanceEngine{DB: x.engine.DB})
					if out.Decision == verification.VerificationVerified {
						t.Fatal("revoked authority/binding/credential trusted")
					}
				} else {
					if err = revoke(); err != nil {
						t.Fatal(err)
					}
					go func() {
						var err error
						issued, err = x.engine.Issue(ctx, x.f.other, x.request, "binding-employer")
						issueDone <- err
					}()
					waitEmployerBlocked(t, x.f.pool, revokePID, issuePID)
					if err = revoker.Commit(ctx); err != nil {
						t.Fatal(err)
					}
					if err = <-issueDone; err == nil || issued.Credential != "" {
						t.Fatal("revocation-first signed credential")
					}
				}
			})
		}
	}
}

func TestEvidenceIssuanceHTTPAndEligibility(t *testing.T) {
	x := issuanceSetup(t)
	engine := gin.New()
	engine.Use(auth.GinRequireAuth(x.f.s.cfg.Auth.JWTSecret, x.f.s.cfg.Auth.Issuer, x.f.s.cfg.Auth.Audience))
	module := &evt.Module{Candidate: &evt.CandidateHandlers{DB: x.engine.DB, Repo: &evt.Repo{DB: x.engine.DB}}, Employer: &evt.EmployerHandlers{DB: x.engine.DB, Repo: &evt.Repo{DB: x.engine.DB, Signer: x.engine.Signer.(*evt.AttestationSigner)}}}
	module.Register(engine.Group("/v1"))
	hr := employerToken(t, x.f.s, x.f.other, auth.RoleHRReviewer)
	path := "/v1/employer/requests/" + x.request
	var eligibility requestbinding.Result
	response := employerCall(engine, hr, "POST", path+"/evidence-eligibility", `{"employer_id":"binding-employer"}`)
	evidenceStatus(t, response, 200)
	if err := json.Unmarshal(response.Body.Bytes(), &eligibility); err != nil || !eligibility.Eligible || !eligibility.SigningEnabled || eligibility.PendingScanCount != 1 {
		t.Fatal("configured eligibility did not resolve exact authority", err, eligibility)
	}
	for _, tc := range []struct {
		token, body string
		status      int
	}{{"", `{"employer_id":"binding-employer"}`, 401}, {employerToken(t, x.f.s, x.f.principal, auth.RoleHRReviewer), `{"employer_id":"binding-employer"}`, 404}, {hr, `{"employer_id":"binding-employer","eligible":true}`, 400}, {hr, `{"employer_id":"unrelated"}`, 404}} {
		evidenceStatus(t, employerCall(engine, tc.token, "POST", path+"/issue-evidence", tc.body), tc.status)
	}
	response = employerCall(engine, hr, "POST", path+"/issue-evidence", `{"employer_id":"binding-employer"}`)
	evidenceStatus(t, response, 200)
	var issued evtdb.IssuanceResult
	if err := json.Unmarshal(response.Body.Bytes(), &issued); err != nil || issued.Credential == "" {
		t.Fatal("missing issued credential", err)
	}
	outcome := ec.Verify(context.Background(), issued.Credential, x.f.principal, ec.Policy, time.Now().UTC(), &evtdb.IssuanceEngine{DB: x.engine.DB})
	if outcome.Decision != verification.VerificationVerified {
		t.Fatalf("HTTP artifact not independently verified: %+v", outcome)
	}
	subjectToken := employerToken(t, x.f.s, x.f.principal, auth.RoleRequestor)
	downloadPath := "/v1/requests/" + x.request + "/credential"
	downloaded := employerCall(engine, subjectToken, "GET", downloadPath, "")
	evidenceStatus(t, downloaded, 200)
	var portable struct {
		Credential string `json:"credential"`
	}
	if json.Unmarshal(downloaded.Body.Bytes(), &portable) != nil || portable.Credential != issued.Credential {
		t.Fatal("subject did not receive exact committed credential")
	}
	evidenceStatus(t, employerCall(engine, employerToken(t, x.f.s, x.f.other, auth.RoleRequestor), "GET", downloadPath, ""), 404)
	evidenceExec(t, x.f.pool, `UPDATE domain_users SET status='inactive' WHERE id=$1`, x.f.principal)
	evidenceStatus(t, employerCall(engine, subjectToken, "GET", downloadPath, ""), 404)

}

func TestEvidenceVerificationCurrentFacts(t *testing.T) {
	for _, condition := range []string{"binding-revoked", "case-closed", "scan-infected", "key-revoked", "authority-revoked", "authority-unavailable", "request-changed", "required-check-added"} {
		t.Run(condition, func(t *testing.T) {
			x := issuanceSetup(t)
			issued, initial := issueAndVerify(t, x)
			if initial.Decision != verification.VerificationVerified {
				t.Fatal(initial)
			}
			switch condition {
			case "binding-revoked":
				x.f.admin(t, `SELECT revoke_request_evidence_binding($1,'local-owner','withdrawn')`, x.binding)
			case "case-closed":
				x.f.admin(t, `SELECT set_evidence_resource_state('case-a',NULL,'CLOSED','local-owner','closed')`)
			case "scan-infected":
				evidenceExec(t, x.f.pool, `UPDATE evidence_scan_jobs SET scan_status='INFECTED'`)
			case "key-revoked":
				evidenceExec(t, x.f.pool, `UPDATE evidence_issuer_credentials SET revoked_at=clock_timestamp(),revoked_by='local-owner',revocation_reason='withdrawn'`)
			case "authority-revoked":
				at := time.Now().UTC().Truncate(time.Microsecond)
				if err := x.authority.AppendJournalEvent(context.Background(), authority.AuthorizationJournalEvent{EventID: uuid.NewString(), AuthorizationID: x.authorityID, EventType: authority.EventRevoke, EffectiveAt: at, RecordedAt: at, ActorID: "local-adjudicator", EvidenceRef: "withdrawn", ConditionsSatisfied: true}); err != nil {
					t.Fatal(err)
				}
			case "authority-unavailable":
				evidenceExec(t, x.f.pool, `ALTER TABLE authority_recording_boundary RENAME TO local_missing_boundary`)
			case "request-changed":
				evidenceExec(t, x.f.pool, `UPDATE evt_requests SET candidate_personid=$1 WHERE request_id=$2`, x.f.other, x.request)
			case "required-check-added":
				x.f.admin(t, `SELECT provision_evidence_check('case-a','extra','local-owner','changed context')`)
				evidenceExec(t, x.f.pool, `INSERT INTO request_binding_checks(binding_id,case_id,check_id) VALUES($1,'case-a','extra')`, x.binding)
			}
			out := ec.Verify(context.Background(), issued.Credential, x.f.principal, ec.Policy, time.Now().UTC(), &evtdb.IssuanceEngine{DB: x.engine.DB})
			if out.Decision == verification.VerificationVerified || out.Claim != nil {
				t.Fatalf("stale trust for %s: %+v", condition, out)
			}
		})
	}
}

func TestEvidenceIssuanceCommitFailureDoesNotPublish(t *testing.T) {
	x := issuanceSetup(t)
	evidenceExec(t, x.f.pool, `CREATE FUNCTION refuse_issuance_event() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'local commit failure'; END $$; CREATE TRIGGER local_issuance_event BEFORE INSERT ON evt_request_events FOR EACH ROW EXECUTE FUNCTION refuse_issuance_event()`)
	issued, err := x.engine.Issue(context.Background(), x.f.other, x.request, "binding-employer")
	if err == nil || issued.Credential != "" {
		t.Fatal("failed commit published signed credential")
	}
	var untouched bool
	if err = x.f.pool.QueryRow(context.Background(), `SELECT NOT EXISTS(SELECT 1 FROM evidence_issued_credentials) AND EXISTS(SELECT 1 FROM evt_requests WHERE request_id=$1 AND status::text='ATTESTATION_PENDING' AND coalesce(attestation_jws,'')='')`, x.request).Scan(&untouched); err != nil || !untouched {
		t.Fatal("issuance failure escaped transaction", err)
	}
}

type pausedEvidenceResolver struct {
	source           ec.Resolver
	started, release chan struct{}
}

func (p pausedEvidenceResolver) WithFacts(ctx context.Context, hash string, fn func(ec.Facts) error) error {
	return p.source.WithFacts(ctx, hash, func(f ec.Facts) error {
		close(p.started)
		select {
		case <-p.release:
			return fn(f)
		case <-ctx.Done():
			return ctx.Err()
		}
	})
}
func TestEvidenceVerificationRevocationConcurrency(t *testing.T) {
	for _, kind := range []string{"authority", "claim", "credential"} {
		for _, first := range []string{"verify", "revoke"} {
			t.Run(kind+"/"+first, func(t *testing.T) {
				x := issuanceSetup(t)
				issued, initial := issueAndVerify(t, x)
				if initial.Decision != verification.VerificationVerified {
					t.Fatal(initial)
				}
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				conn, err := x.engine.DB.Pool.Acquire(ctx)
				if err != nil {
					t.Fatal(err)
				}
				verifyPID := conn.Conn().PgConn().PID()
				conn.Release()
				writer, err := x.f.pool.Begin(ctx)
				if err != nil {
					t.Fatal(err)
				}
				defer writer.Rollback(context.Background())
				writerPID := writer.Conn().PgConn().PID()
				revoke := func() error {
					if kind == "claim" {
						_, err := writer.Exec(ctx, `SELECT revoke_bound_credential($1,'local-owner','withdrawn')`, issued.CredentialID)
						return err
					}
					if kind == "credential" {
						_, err := writer.Exec(ctx, `UPDATE evidence_issuer_credentials SET revoked_at=clock_timestamp(),revoked_by='local-owner',revocation_reason='withdrawn'`)
						return err
					}
					var position int64
					if err := writer.QueryRow(ctx, `SELECT next_authority_recording_position()`).Scan(&position); err != nil {
						return err
					}
					_, err := writer.Exec(ctx, `INSERT INTO authority_journal_events(event_id,authorization_id,event_type,effective_at,recorded_at,actor_id,evidence_ref,conditions_satisfied,recording_position) VALUES($1,$2,'REVOKE',clock_timestamp(),clock_timestamp(),'local-adjudicator','withdrawn',true,$3)`, uuid.NewString(), x.authorityID, position)
					return err
				}
				verifier := &evtdb.IssuanceEngine{DB: x.engine.DB}
				done := make(chan ec.Outcome, 1)
				if first == "verify" {
					pause := pausedEvidenceResolver{source: verifier, started: make(chan struct{}), release: make(chan struct{})}
					go func() { done <- ec.Verify(ctx, issued.Credential, x.f.principal, ec.Policy, time.Now().UTC(), pause) }()
					select {
					case <-pause.started:
					case <-ctx.Done():
						t.Fatal("verification never acquired facts")
					}
					revoked := make(chan error, 1)
					go func() { revoked <- revoke() }()
					waitEmployerBlocked(t, x.f.pool, verifyPID, writerPID)
					close(pause.release)
					if out := <-done; out.Decision != verification.VerificationVerified {
						t.Fatal("pre-revocation linearization failed", out)
					}
					if err = <-revoked; err != nil {
						t.Fatal(err)
					}
					if err = writer.Commit(ctx); err != nil {
						t.Fatal(err)
					}
					if out := ec.Verify(ctx, issued.Credential, x.f.principal, ec.Policy, time.Now().UTC(), verifier); out.Decision == verification.VerificationVerified {
						t.Fatal("post-revocation stale trust")
					}
				} else {
					if err = revoke(); err != nil {
						t.Fatal(err)
					}
					go func() {
						done <- ec.Verify(ctx, issued.Credential, x.f.principal, ec.Policy, time.Now().UTC(), verifier)
					}()
					waitEmployerBlocked(t, x.f.pool, writerPID, verifyPID)
					if err = writer.Commit(ctx); err != nil {
						t.Fatal(err)
					}
					if out := <-done; out.Decision == verification.VerificationVerified {
						t.Fatal("revocation-first stale trust")
					}
				}
			})
		}
	}
}

func TestEvidenceIssuanceNormalRequestToPortableClaim(t *testing.T) {
	x := issuanceSetup(t)
	// Explicit workflow test scaffolding, not a claim about unexported production
	// defaults/columns. The production restore rehearsal must establish these.
	evidenceExec(t, x.f.pool, `ALTER TABLE evt_requests ALTER COLUMN request_id SET DEFAULT gen_random_uuid();ALTER TABLE evt_requests ADD COLUMN submitted_at timestamptz;ALTER TABLE evt_requests ADD COLUMN attestation_dispatched_at timestamptz`)
	// Exact existing legacy privileges from api-runtime-privileges.sql.
	evidenceExec(t, x.f.pool, `GRANT INSERT ON evt_requests,evt_employer_config,evt_request_events TO `+pgx.Identifier{x.f.runtime}.Sanitize()+`;GRANT SELECT ON evt_employer_config TO `+pgx.Identifier{x.f.runtime}.Sanitize())
	repo := &evt.Repo{DB: x.engine.DB, Signer: x.engine.Signer.(*evt.AttestationSigner)}
	module := &evt.Module{Candidate: &evt.CandidateHandlers{DB: x.engine.DB, Repo: repo}, Employer: &evt.EmployerHandlers{DB: x.engine.DB, Repo: repo}}
	engine := gin.New()
	engine.Use(auth.GinRequireAuth(x.f.s.cfg.Auth.JWTSecret, x.f.s.cfg.Auth.Issuer, x.f.s.cfg.Auth.Audience))
	module.Register(engine.Group("/v1"))
	subject := employerToken(t, x.f.s, x.f.principal, auth.RoleRequestor)
	hr := employerToken(t, x.f.s, x.f.other, auth.RoleHRReviewer)
	created := employerCall(engine, subject, "POST", "/v1/requests", `{"employer_id":"binding-employer","claim_snapshot":{"primary_employment":{"title":"Engineer","start_date":"2020-01-01"}}}`)
	evidenceStatus(t, created, 201)
	var request evt.CreateDraftResp
	if json.Unmarshal(created.Body.Bytes(), &request) != nil || request.RequestID == "" {
		t.Fatal("normal request creation failed")
	}
	submitted := employerCall(engine, subject, "POST", "/v1/requests/"+request.RequestID+"/submit", `{}`)
	evidenceStatus(t, submitted, 200)
	var dispatch evt.SubmitResp
	if json.Unmarshal(submitted.Body.Bytes(), &dispatch) != nil || dispatch.Status != "ATTESTATION_PENDING" {
		t.Fatal("normal dispatch did not authorize binding")
	}
	binding := x.f.adminString(t, `SELECT bind_request_evidence($1,'case-b',ARRAY['employment.company_and_dates'],'local-owner','normal-request fixture')`, request.RequestID)
	x.f.adminString(t, `SELECT grant_evidence_upload($1,'case-b','local-owner','normal-request upload')`, x.f.principal)
	x.f.adminString(t, `SELECT grant_evidence_review($1,'case-b','local-owner','normal-request independent review')`, x.f.other)
	m := &memoryEvidence{objects: map[string][]byte{}, mime: "application/pdf"}
	x.f.s.s3 = m
	initialized := x.f.request(x.f.principal, "case-b", "employment.company_and_dates", false, evidenceFile)
	evidenceStatus(t, initialized, 200)
	var init EvidenceInitResponse
	if json.Unmarshal(initialized.Body.Bytes(), &init) != nil {
		t.Fatal("normal evidence initialization failed")
	}
	m.objects[init.Uploads[0].StorageKey] = []byte("a")
	evidenceStatus(t, completionRequest(x.f, x.f.principal, init, "/v1/evidence/commit"), 200)
	base := "/v1/cases/case-b/checks/employment.company_and_dates/evidence"
	list := reviewRequest(x.f, x.f.other, "GET", base, "")
	evidenceStatus(t, list, 200)
	var listing struct {
		Binding struct {
			BindingID string `json:"bindingId"`
			Digest    string `json:"claimDigest"`
		}
	}
	if json.Unmarshal(list.Body.Bytes(), &listing) != nil || listing.Binding.BindingID != binding {
		t.Fatal("normal request context not bound")
	}
	review, _ := json.Marshal(map[string]string{"decision": "APPROVED", "reason": "Independent contextual review", "bindingId": binding, "claimDigest": listing.Binding.Digest})
	evidenceStatus(t, reviewRequest(x.f, x.f.other, "POST", base+"/"+init.Uploads[0].UploadID+"/review", string(review)), 200)
	response := employerCall(engine, hr, "POST", "/v1/employer/requests/"+request.RequestID+"/issue-evidence", `{"employer_id":"binding-employer"}`)
	evidenceStatus(t, response, 200)
	var issued evtdb.IssuanceResult
	if json.Unmarshal(response.Body.Bytes(), &issued) != nil {
		t.Fatal("normal issuance failed")
	}
	exported := employerCall(engine, subject, "GET", "/v1/requests/"+request.RequestID+"/credential", "")
	evidenceStatus(t, exported, 200)
	var portable struct {
		Credential string `json:"credential"`
	}
	if json.Unmarshal(exported.Body.Bytes(), &portable) != nil || portable.Credential != issued.Credential {
		t.Fatal("normal subject export mismatch")
	}
	out := ec.Verify(context.Background(), portable.Credential, x.f.principal, ec.Policy, time.Now().UTC(), &evtdb.IssuanceEngine{DB: x.engine.DB})
	if out.Decision != verification.VerificationVerified || out.Commitment == nil || out.Commitment.RequestID != request.RequestID {
		t.Fatal("normal request not independently verified", out)
	}
}

type watchedEvidenceSigner struct {
	ec.Signer
	calls int
}

func (s *watchedEvidenceSigner) SignEvidenceClaims(ctx context.Context, c ec.Claims) (string, error) {
	s.calls++
	return "", ec.ErrUnavailable
}
func TestEvidenceIssuanceRequiresExplicitCredentialActivation(t *testing.T) {
	x := issuanceSetup(t, false)
	watch := &watchedEvidenceSigner{Signer: x.engine.Signer}
	x.engine.Signer = watch
	got, err := x.engine.Issue(context.Background(), x.f.other, x.request, "binding-employer")
	if err == nil || got.Credential != "" || watch.calls != 0 {
		t.Fatal("unregistered production-like credential reached signer")
	}
	var empty bool
	if err = x.f.pool.QueryRow(context.Background(), `SELECT NOT EXISTS(SELECT 1 FROM evidence_issuer_credentials) AND NOT EXISTS(SELECT 1 FROM evidence_issued_credentials)`).Scan(&empty); err != nil || !empty {
		t.Fatal("implicit credential activation or issuance", err)
	}
}
