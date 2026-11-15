package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/credential"
	evtdb "github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/db"
	ec "github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/evidencecredential"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/verification"
)

type countedIntegritySigner struct {
	ec.Signer
	count atomic.Int32
}

func (s *countedIntegritySigner) SignEvidenceClaims(c context.Context, claims ec.Claims) (string, error) {
	s.count.Add(1)
	return s.Signer.SignEvidenceClaims(c, claims)
}
func pendingIntegrityEvidence(t *testing.T, f evidenceFixture, binding, uploader string) (EvidenceInitResponse, string, string) {
	t.Helper()
	f.adminString(t, `SELECT grant_evidence_review($1,'case-a','fixture','explicit contextual review')`, f.other)
	m := &memoryEvidence{objects: map[string][]byte{}, mime: "application/pdf"}
	f.s.s3 = m
	if uploader != f.principal {
		f.adminString(t, `SELECT grant_evidence_upload($1,'case-a','fixture','alternate uploader')`, uploader)
	}
	initResponse := f.request(uploader, "case-a", "employment.company_and_dates", false, evidenceFile)
	evidenceStatus(t, initResponse, 200)
	var init EvidenceInitResponse
	if err := json.Unmarshal(initResponse.Body.Bytes(), &init); err != nil {
		t.Fatal(err)
	}
	m.objects[init.Uploads[0].StorageKey] = []byte("a")
	evidenceStatus(t, completionRequest(f, uploader, init, "/v1/evidence/commit"), 200)
	var digest string
	if err := f.pool.QueryRow(context.Background(), `SELECT claim_digest FROM request_case_bindings WHERE binding_id=$1`, binding).Scan(&digest); err != nil {
		t.Fatal(err)
	}
	_, _, path := reviewPaths(init)
	return init, path, fmt.Sprintf(`{"decision":"APPROVED","reason":"Inspected claim and evidence","bindingId":%q,"claimDigest":%q,"evidenceHash":%q}`, binding, digest, evidenceDigest([]byte("a")))
}
func assertNoIntegrityIssuance(t *testing.T, f evidenceFixture, request string) {
	t.Helper()
	var status string
	var artifact *string
	var receipts, events int
	if err := f.pool.QueryRow(context.Background(), `SELECT status,attestation_jws FROM evt_requests WHERE request_id=$1`, request).Scan(&status, &artifact); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM evidence_issued_credentials WHERE request_id=$1`, request).Scan(&receipts); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM evt_request_events WHERE request_id=$1`, request).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if status != "ATTESTATION_PENDING" || artifact != nil || receipts != 0 || events != 0 {
		t.Fatalf("unexpected mutation %s artifact=%v issued=%d events=%d", status, artifact, receipts, events)
	}
}
func TestIntegritySimulatedReviewCannotIssueOrVerify(t *testing.T) {
	// Even active fixture authority and an active fixture credential cannot override UAT mode.
	x := issuanceSetupContext(t, "SIMULATED_UAT")
	spy := &countedIntegritySigner{Signer: x.engine.Signer}
	x.engine.Signer = spy
	_, err := x.engine.Issue(context.Background(), x.f.other, x.request, "binding-employer")
	if err == nil || spy.count.Load() != 0 {
		t.Fatal("simulated context reached signer", err)
	}
	assertNoIntegrityIssuance(t, x.f, x.request)
	for _, sql := range []string{
		`SELECT prepare_bound_issuance($1,$2,'binding-employer','fixture')`,
		`SELECT commit_bound_issuance($1,$2,'binding-employer','fixture','fake~','{}'::jsonb)`,
	} {
		_, err = x.f.s.db.Exec(sql, x.f.other, x.request)
		if err == nil {
			t.Fatal("direct issuance bypass", sql)
		}
	}
	for _, sql := range []string{`UPDATE evt_requests SET status='ATTESTED' WHERE request_id=$1`, `UPDATE evt_requests SET attestation_jws='fake' WHERE request_id=$1`} {
		_, err = x.f.pool.Exec(context.Background(), sql, x.request)
		expectEmployerSQLState(t, err, "23514")
	}
	// An unsigned review receipt is not accepted by either verification boundary.
	var review string
	if err = x.f.pool.QueryRow(context.Background(), `SELECT jsonb_build_object('kind','UNSIGNED_EVIDENCE_REVIEW_RECEIPT','decision',decision,'reviewerId',reviewer_id)::text FROM evidence_reviews LIMIT 1`).Scan(&review); err != nil {
		t.Fatal(err)
	}
	outcome := ec.Verify(context.Background(), review, x.f.principal, ec.Policy, time.Now().UTC(), &evtdb.IssuanceEngine{DB: x.engine.DB})
	if outcome.Decision == verification.VerificationVerified || outcome.Claim != nil {
		t.Fatal("unsigned receipt verified", outcome)
	}
	w := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]string{"jwsCompact": review})
	x.f.s.HandleVerificationOutcome(w, httptest.NewRequest("POST", "/v1/verification/outcome", strings.NewReader(string(body))))
	if w.Code != 200 || strings.Contains(w.Body.String(), `"trust":"trusted"`) {
		t.Fatal("receipt promoted to trusted outcome", w.Body.String())
	}
	assertNoIntegrityIssuance(t, x.f, x.request)
}
func TestIntegrityContextClassificationAndReceipt(t *testing.T) {
	f, request, binding := bindingSetupContext(t, true, "SIMULATED_UAT")
	init, path, body := pendingIntegrityEvidence(t, f, binding, f.principal)
	body = strings.TrimSuffix(body, "}") + `,"reviewContext":"AUTHORITATIVE"}`
	receipt := reviewRequest(f, f.other, "POST", path, body)
	evidenceStatus(t, receipt, 200)
	var r map[string]any
	if err := json.Unmarshal(receipt.Body.Bytes(), &r); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{"kind": "UNSIGNED_EVIDENCE_REVIEW_RECEIPT", "reviewContext": "SIMULATED_UAT", "bindingId": binding, "requestId": request, "caseId": "case-a", "checkId": "employment.company_and_dates", "evidenceHash": evidenceDigest([]byte("a")), "reviewerId": f.other, "uploadId": init.Uploads[0].UploadID} {
		if r[key] != want {
			t.Fatalf("receipt %s=%v want %s", key, r[key], want)
		}
	}
	retry := reviewRequest(f, f.other, "POST", path, body)
	evidenceStatus(t, retry, 200)
	if retry.Body.String() != receipt.Body.String() {
		t.Fatal("exact retry changed receipt")
	}
	evidenceStatus(t, reviewRequest(f, f.other, "POST", path, strings.Replace(body, "Inspected claim and evidence", "different rationale", 1)), 409)
	for _, sql := range []string{`UPDATE request_review_classifications SET review_context='AUTHORITATIVE' WHERE binding_id=$1`, `DELETE FROM request_review_classifications WHERE binding_id=$1`, `UPDATE request_case_bindings SET request_version=request_version+1 WHERE binding_id=$1`} {
		_, err := f.pool.Exec(context.Background(), sql, binding)
		expectEmployerSQLState(t, err, "23514")
	}
	for _, sql := range []string{`UPDATE evidence_reviews SET reason='changed'`, `DELETE FROM evidence_reviews`} {
		_, err := f.pool.Exec(context.Background(), sql)
		expectEmployerSQLState(t, err, "23514")
	}
	_, err := f.s.db.Exec(`SELECT classify_request_review($1,'AUTHORITATIVE','client','upgrade')`, binding)
	expectEmployerSQLState(t, err, "42501")
	assertNoIntegrityIssuance(t, f, request)
}
func TestIntegrityClaimantReviewWithDifferentUploader(t *testing.T) {
	f, request, binding := bindingSetupContext(t, true, "SIMULATED_UAT")
	_, path, body := pendingIntegrityEvidence(t, f, binding, f.other)
	f.adminString(t, `SELECT grant_evidence_review($1,'case-a','fixture','claimant must still deny')`, f.principal)
	evidenceStatus(t, reviewRequest(f, f.principal, "POST", path, body), 403)
	evidenceStatus(t, reviewRequest(f, f.other, "POST", path, body), 403) // uploader, not claimant
	var n int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM evidence_reviews`).Scan(&n); err != nil || n != 0 {
		t.Fatal("denied decision persisted", err)
	}
	assertNoIntegrityIssuance(t, f, request)
}
func TestIntegrityClaimantIssuerDeniesBeforeSigner(t *testing.T) {
	x := issuanceSetup(t)
	evidenceExec(t, x.f.pool, `UPDATE domain_users SET role='hr_reviewer' WHERE id=$1`, x.f.principal)
	grantEmployer(t, x.f.pool, x.f.principal, "binding-employer")
	spy := &countedIntegritySigner{Signer: x.engine.Signer}
	x.engine.Signer = spy
	_, err := x.engine.Issue(context.Background(), x.f.principal, x.request, "binding-employer")
	if err == nil || spy.count.Load() != 0 {
		t.Fatal("claimant issuance reached signer", err)
	}
	_, err = x.f.s.db.Exec(`SELECT commit_bound_issuance($1,$2,'binding-employer','fixture','fake~','{}'::jsonb)`, x.f.principal, x.request)
	if err == nil {
		t.Fatal("claimant commit allowed")
	}
	assertNoIntegrityIssuance(t, x.f, x.request)
}
func TestIntegrityUnclassifiedAndAcknowledgementDenials(t *testing.T) {
	for _, change := range []string{"unclassified", "claim-digest", "evidence-digest", "missing-evidence-digest", "binding", "stale-version", "unauthorized", "old-signature"} {
		t.Run(change, func(t *testing.T) {
			mode := "SIMULATED_UAT"
			if change == "unclassified" {
				mode = ""
			}
			f, request, binding := bindingSetupContext(t, true, mode)
			init, path, body := pendingIntegrityEvidence(t, f, binding, f.principal)
			want := 400
			actor := f.other
			switch change {
			case "unclassified":
				want = 409
			case "claim-digest":
				var original string
				_ = f.pool.QueryRow(context.Background(), `SELECT claim_digest FROM request_case_bindings WHERE binding_id=$1`, binding).Scan(&original)
				body = strings.Replace(body, original, strings.Repeat("0", 64), 1)
			case "evidence-digest":
				body = strings.Replace(body, evidenceDigest([]byte("a")), strings.Repeat("0", 64), 1)
			case "missing-evidence-digest":
				var m map[string]any
				_ = json.Unmarshal([]byte(body), &m)
				delete(m, "evidenceHash")
				b, _ := json.Marshal(m)
				body = string(b)
			case "binding":
				body = strings.Replace(body, binding, "11111111-1111-4111-8111-111111111111", 1)
			case "stale-version":
				evidenceExec(t, f.pool, `UPDATE evt_requests SET version=version+1 WHERE request_id=$1`, request)
				want = 409
			case "unauthorized":
				actor = f.principal
				want = 404
			case "old-signature":
				_, err := f.s.db.Exec(`SELECT record_request_evidence_review($1,'case-a','employment.company_and_dates',$2,'APPROVED','rationale',$3,repeat('0',64))`, f.other, init.Uploads[0].UploadID, binding)
				expectEmployerSQLState(t, err, "22023")
				return
			}
			evidenceStatus(t, reviewRequest(f, actor, "POST", path, body), want)
			var n int
			if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM evidence_reviews`).Scan(&n); err != nil || n != 0 {
				t.Fatal("denied decision persisted", err)
			}
			assertNoIntegrityIssuance(t, f, request)
		})
	}
}
func TestIntegrityConcurrentDecisionsAndRollback(t *testing.T) {
	f, request, binding := bindingSetupContext(t, true, "SIMULATED_UAT")
	init, path, body := pendingIntegrityEvidence(t, f, binding, f.principal)
	var wg sync.WaitGroup
	codes := make(chan int, 2)
	for _, rationale := range []string{"first rationale", "second rationale"} {
		wg.Add(1)
		go func(reason string) {
			defer wg.Done()
			codes <- reviewRequest(f, f.other, "POST", path, strings.Replace(body, "Inspected claim and evidence", reason, 1)).Code
		}(rationale)
	}
	wg.Wait()
	close(codes)
	seen := map[int]int{}
	for code := range codes {
		seen[code]++
	}
	if seen[200] != 1 || seen[409] != 1 {
		t.Fatal("concurrent decisions", seen)
	}
	assertNoIntegrityIssuance(t, f, request)
	// A failure after the underlying review INSERT must roll back that INSERT too.
	g, otherRequest, otherBinding := bindingSetupContext(t, true, "SIMULATED_UAT")
	otherInit, otherPath, otherBody := pendingIntegrityEvidence(t, g, otherBinding, g.principal)
	evidenceExec(t, g.pool, `CREATE FUNCTION refuse_context() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'fixture rollback'; END $$; CREATE TRIGGER refuse_context BEFORE INSERT ON request_evidence_review_contexts FOR EACH ROW EXECUTE FUNCTION refuse_context()`)
	evidenceStatus(t, reviewRequest(g, g.other, "POST", otherPath, otherBody), 503)
	var n int
	if err := g.pool.QueryRow(context.Background(), `SELECT count(*) FROM evidence_reviews WHERE upload_id=$1`, otherInit.Uploads[0].UploadID).Scan(&n); err != nil || n != 0 {
		t.Fatal("failed context left review", err)
	}
	evidenceExec(t, g.pool, `DROP TRIGGER refuse_context ON request_evidence_review_contexts`)
	evidenceStatus(t, reviewRequest(g, g.other, "POST", otherPath, otherBody), 200)
	assertNoIntegrityIssuance(t, g, otherRequest)
	_ = init
}
func TestIntegrityRejectsStoredClaimantOrUploaderApproval(t *testing.T) {
	for _, who := range []string{"claimant", "uploader"} {
		t.Run(who, func(t *testing.T) {
			x := issuanceSetup(t)
			third := "33333333-3333-4333-8333-333333333333"
			evidenceExec(t, x.f.pool, `INSERT INTO domain_users(id,kratos_identity_id,role,status) VALUES($1::uuid,$1::uuid::text,'hr_reviewer','active')`, third)
			x.f.adminString(t, `SELECT grant_evidence_upload($1,'case-a','fixture','third party upload')`, third)
			m := &memoryEvidence{objects: map[string][]byte{}, mime: "application/pdf"}
			x.f.s.s3 = m
			w := x.f.request(third, "case-a", "employment.company_and_dates", false, evidenceFile)
			evidenceStatus(t, w, 200)
			var init EvidenceInitResponse
			if err := json.Unmarshal(w.Body.Bytes(), &init); err != nil {
				t.Fatal(err)
			}
			m.objects[init.Uploads[0].StorageKey] = []byte("b")
			evidenceStatus(t, completionRequest(x.f, third, init, "/v1/evidence/commit"), 200)
			reviewer := third
			if who == "claimant" {
				reviewer = x.f.principal
			}
			grant := x.f.adminString(t, `SELECT grant_evidence_review($1,'case-a','fixture','malformed-history fixture only')`, reviewer)
			// Trusted fixture owner inserts inconsistent history to verify consumption
			// rejects it even if an old/external writer bypassed the review endpoint.
			var review string
			if err := x.f.pool.QueryRow(context.Background(), `INSERT INTO evidence_reviews(upload_id,reviewer_id,grant_id,decision,reason) VALUES($1,$2,$3,'APPROVED','fixture corruption') RETURNING review_id`, init.Uploads[0].UploadID, reviewer, grant).Scan(&review); err != nil {
				t.Fatal(err)
			}
			evidenceExec(t, x.f.pool, `INSERT INTO request_evidence_review_contexts(review_id,binding_id,claim_digest,evidence_hash) SELECT $1,binding_id,claim_digest,$3 FROM request_case_bindings WHERE binding_id=$2`, review, x.binding, evidenceDigest([]byte("b")))
			spy := &countedIntegritySigner{Signer: x.engine.Signer}
			x.engine.Signer = spy
			if _, err := x.engine.Issue(context.Background(), x.f.other, x.request, "binding-employer"); err == nil || spy.count.Load() != 0 {
				t.Fatal("bad stored approval reached signer", err)
			}
			assertNoIntegrityIssuance(t, x.f, x.request)
		})
	}
}
func TestIntegrityMigrationPreservesHistoricalCredential(t *testing.T) {
	x := issuanceSetupVersion(t, false, "")
	ctx := context.Background()
	tx, err := x.engine.DB.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	var raw []byte
	if err = tx.QueryRow(ctx, `SELECT prepare_bound_issuance($1,$2,'binding-employer',$3)`, x.f.other, x.request, x.engine.Signer.KeyID()).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var wire struct {
		Context            ec.Commitment
		Snapshot, Manifest string
		Now, ExpiresAt     time.Time
		Key                ec.KeyBinding
	}
	if err = json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	wire.Key.PublicKey = x.engine.Signer.PublicKey()
	snapshot, err := evtdb.LoadBoundedAuthoritySnapshot(ctx, tx)
	if err != nil {
		t.Fatal(err)
	}
	startDate := "2020-01-01"
	claims, artifact, err := ec.IssueLocked(ctx, ec.Prepared{Context: wire.Context, Snapshot: wire.Snapshot, Manifest: wire.Manifest, Now: wire.Now, ExpiresAt: wire.ExpiresAt, Key: wire.Key, Authority: snapshot, Employment: credential.EmploymentRelationship{EmployerID: "binding-employer", EmployerName: "Fixture Employer", Title: "Engineer", StartDate: &startDate}}, x.engine.Signer)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(claims)
	if _, err = tx.Exec(ctx, `SELECT commit_bound_issuance($1,$2,'binding-employer',$3,$4,$5::jsonb)`, x.f.other, x.request, x.engine.Signer.KeyID(), artifact, string(encoded)); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	// The migration preserves the original bytes and does not manufacture a context.
	evidenceExec(t, x.f.pool, employerSQL(t, "../../migrations/000020_claim_bound_review_integrity.sql"))
	evidenceExec(t, x.f.pool, employerSQL(t, "../../migrations/000021_claim_only_simulated_review.sql"))
	actual, err := x.engine.SubjectCredential(ctx, x.f.principal, x.request)
	if err != nil || actual != artifact {
		t.Fatal("historical artifact changed", err)
	}
	out := ec.Verify(ctx, artifact, x.f.principal, ec.Policy, time.Now().UTC(), &evtdb.IssuanceEngine{DB: x.engine.DB})
	if out.Decision != verification.VerificationVerified {
		t.Fatalf("historical verification changed: %+v", out)
	}
	var n int
	if err = x.f.pool.QueryRow(ctx, `SELECT count(*) FROM request_review_classifications`).Scan(&n); err != nil || n != 0 {
		t.Fatal("migration classified history", err)
	}
	_, err = x.f.pool.Exec(ctx, `SELECT classify_request_review($1,'AUTHORITATIVE','fixture','retroactive upgrade')`, x.binding)
	if err == nil {
		t.Fatal("retrospective classification accepted")
	}
	spy := &countedIntegritySigner{Signer: x.engine.Signer}
	x.engine.Signer = spy
	if _, err = x.engine.Issue(ctx, x.f.other, x.request, "binding-employer"); err == nil || spy.count.Load() != 0 {
		t.Fatal("historical unclassified context authorized new signing", err)
	}
}
func TestIntegrityMigrationRollbackAndPrivateHelpers(t *testing.T) {
	f, _, _ := bindingSetupVersion(t, false)
	evidenceExec(t, f.pool, employerSQL(t, "../../migrations/000019_evidence_backed_issuance.sql"))
	ctx := context.Background()
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, employerSQL(t, "../../migrations/000020_claim_bound_review_integrity.sql")); err != nil {
		t.Fatal(err)
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	var absent bool
	if err = f.pool.QueryRow(ctx, `SELECT to_regclass('request_review_classifications') IS NULL AND to_regprocedure('request_evidence_context_before_integrity(uuid,text,text)') IS NULL AND NOT EXISTS(SELECT 1 FROM pg_attribute WHERE attrelid='request_evidence_review_contexts'::regclass AND attname='evidence_hash' AND NOT attisdropped)`).Scan(&absent); err != nil || !absent {
		t.Fatal("partial migration escaped rollback", err)
	}
	evidenceExec(t, f.pool, employerSQL(t, "../../migrations/000020_claim_bound_review_integrity.sql"))
	for _, role := range []string{f.runtime, f.operator} {
		for _, routine := range []string{"classify_request_review(uuid,text,text,text)", "prepare_bound_issuance_before_integrity(uuid,uuid,text,text)", "commit_bound_issuance_before_integrity(uuid,uuid,text,text,text,jsonb)", "record_request_evidence_review_before_integrity(uuid,text,text,uuid,text,text,uuid,text)", "guard_claim_review_issuance(uuid,uuid,text)"} {
			var allowed bool
			if err = f.pool.QueryRow(ctx, `SELECT has_function_privilege($1,$2,'EXECUTE')`, role, routine).Scan(&allowed); err != nil || allowed {
				t.Fatal("private helper leaked", role, routine, err)
			}
		}
		var allowed bool
		if err = f.pool.QueryRow(ctx, `SELECT has_table_privilege($1,'request_review_classifications','SELECT,INSERT,UPDATE,DELETE,TRUNCATE')`, role).Scan(&allowed); err != nil || allowed {
			t.Fatal("classification table exposed", err)
		}
	}
	_, err = f.pool.Exec(ctx, employerSQL(t, "../../migrations/000020_claim_bound_review_integrity.sql"))
	if err == nil {
		t.Fatal("migration replay accepted")
	}
}
func TestIntegrityClassificationIsOneTimeUnderConcurrency(t *testing.T) {
	f, request, binding := bindingSetupContext(t, true, "")
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, mode := range []string{"SIMULATED_UAT", "AUTHORITATIVE"} {
		wg.Add(1)
		go func(value string) {
			defer wg.Done()
			_, err := f.pool.Exec(context.Background(), `SELECT classify_request_review($1,$2,'fixture','concurrent trusted classification')`, binding, value)
			results <- err
		}(mode)
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else {
			expectEmployerSQLState(t, err, "23505")
		}
	}
	if success != 1 {
		t.Fatal("classification race", success)
	}
	var mode string
	if err := f.pool.QueryRow(context.Background(), `SELECT review_context FROM request_review_classifications WHERE binding_id=$1`, binding).Scan(&mode); err != nil {
		t.Fatal(err)
	}
	_, path, body := pendingIntegrityEvidence(t, f, binding, f.principal)
	// Ordinary client context assertions cannot change the trusted classification.
	body = strings.TrimSuffix(body, "}") + `,"reviewContext":"AUTHORITATIVE"}`
	w := reviewRequest(f, f.other, "POST", path, body)
	evidenceStatus(t, w, 200)
	var receipt map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &receipt); err != nil || receipt["reviewContext"] != mode {
		t.Fatal("client replaced trusted mode", err)
	}
	var current, frozen int64
	if err := f.pool.QueryRow(context.Background(), `SELECT r.version,b.request_version FROM evt_requests r JOIN request_case_bindings b USING(request_id) WHERE r.request_id=$1`, request).Scan(&current, &frozen); err != nil || current != frozen {
		t.Fatal("review changed claim version", err)
	}
	assertNoIntegrityIssuance(t, f, request)
}
