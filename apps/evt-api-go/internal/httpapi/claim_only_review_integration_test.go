package httpapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	evtdb "github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/db"
	ec "github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/evidencecredential"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/verification"
	"time"
)

const claimReviewPath = "/v1/cases/case-a/checks/employment.company_and_dates/evidence/review"

func claimReviewBody(t *testing.T, f evidenceFixture, binding string) string {
	t.Helper()
	var digest string
	var version int64
	if err := f.pool.QueryRow(context.Background(), `SELECT claim_digest,request_version FROM request_case_bindings WHERE binding_id=$1`, binding).Scan(&digest, &version); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{"reviewBasis": "CLAIM_ONLY", "decision": "APPROVED", "reason": "Reviewed the immutable submitted claim.", "bindingId": binding, "claimDigest": digest, "requestVersion": version})
	return string(body)
}
func grantClaimReviewer(t *testing.T, f evidenceFixture) {
	f.adminString(t, `SELECT grant_evidence_review($1,'case-a','fixture','independent claim review')`, f.other)
}
func TestClaimOnlyReviewReceiptAndRetry(t *testing.T) {
	f, request, binding := bindingSetupContext(t, true, "SIMULATED_UAT")
	grantClaimReviewer(t, f)
	body := claimReviewBody(t, f, binding)
	w := reviewRequest(f, f.other, "POST", claimReviewPath, body)

	evidenceStatus(t, w, 200)
	var r map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &r); err != nil {
		t.Fatal(err)
	}
	for k, want := range map[string]any{"kind": "UNSIGNED_EVIDENCE_REVIEW_RECEIPT", "reviewBasis": "CLAIM_ONLY", "reviewContext": "SIMULATED_UAT", "bindingId": binding, "requestId": request, "uploadId": nil, "evidenceHash": nil, "reviewerId": f.other, "decision": "APPROVED"} {
		if r[k] != want {
			t.Fatalf("%s=%v want %v", k, r[k], want)
		}
	}
	retry := reviewRequest(f, f.other, "POST", claimReviewPath, body)
	evidenceStatus(t, retry, 200)
	if retry.Body.String() != w.Body.String() {
		t.Fatal("retry changed receipt")
	}
	evidenceStatus(t, reviewRequest(f, f.other, "POST", claimReviewPath, strings.Replace(body, "APPROVED", "REJECTED", 1)), 409)
	listing := reviewRequest(f, f.other, "GET", strings.TrimSuffix(claimReviewPath, "/review"), "")
	evidenceStatus(t, listing, 200)
	if !strings.Contains(listing.Body.String(), r["reviewId"].(string)) {
		t.Fatal("receipt missing from refreshed listing")
	}
	var uploads, reviews int
	if err := f.pool.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM evidence_uploads),(SELECT count(*) FROM evidence_reviews)`).Scan(&uploads, &reviews); err != nil || uploads != 0 || reviews != 1 {
		t.Fatal("unexpected review/upload count", uploads, reviews, err)
	}
	for _, stmt := range []string{`UPDATE evidence_reviews SET reason='changed'`, `DELETE FROM evidence_reviews`, `UPDATE request_review_classifications SET review_context='AUTHORITATIVE'`} {
		_, err := f.pool.Exec(context.Background(), stmt)
		expectEmployerSQLState(t, err, "23514")
	}
	out := ec.Verify(context.Background(), w.Body.String(), f.principal, ec.Policy, time.Now().UTC(), nil)
	if out.Decision == verification.VerificationVerified {
		t.Fatal("unsigned claim receipt verified")
	}
	assertNoIntegrityIssuance(t, f, request)
	var grant string
	if err := f.pool.QueryRow(context.Background(), `SELECT grant_id FROM case_evidence_review_grants WHERE principal_id=$1 AND revoked_at IS NULL`, f.other).Scan(&grant); err != nil {
		t.Fatal(err)
	}
	f.adminString(t, `SELECT revoke_evidence_review($1,'fixture','revoke after receipt')`, grant)
	evidenceStatus(t, reviewRequest(f, f.other, "POST", claimReviewPath, body), 404)
}
func TestClaimOnlyReviewDenials(t *testing.T) {
	cases := []struct{ kind, decision string }{}
	for _, kind := range []string{"unauthenticated", "unauthorized", "revoked", "claimant", "authoritative", "unclassified", "binding", "digest", "version", "stale", "rationale", "basis", "artifact-injection", "check", "inactive"} {
		cases = append(cases, struct{ kind, decision string }{kind, "APPROVED"})
	}
	// Deny a first REJECTED submission before any immutable decision exists.
	for _, kind := range []string{"unauthorized", "revoked", "inactive"} {
		cases = append(cases, struct{ kind, decision string }{kind, "REJECTED"})
	}
	for _, tc := range cases {
		kind := tc.kind
		t.Run(kind+"/"+tc.decision, func(t *testing.T) {
			mode := "SIMULATED_UAT"
			if kind == "authoritative" {
				mode = "AUTHORITATIVE"
			}
			if kind == "unclassified" {
				mode = ""
			}
			f, request, binding := bindingSetupContext(t, true, mode)
			grantClaimReviewer(t, f)
			actor := f.other
			body := strings.Replace(claimReviewBody(t, f, binding), "APPROVED", tc.decision, 1)
			path := claimReviewPath
			want := 400
			var b map[string]any
			_ = json.Unmarshal([]byte(body), &b)
			switch kind {
			case "unauthenticated":
				actor = ""
				want = 401
			case "unauthorized":
				actor = f.principal
				want = 404
			case "claimant":
				actor = f.principal
				f.adminString(t, `SELECT grant_evidence_review($1,'case-a','fixture','deny self review')`, actor)
				want = 403
			case "revoked":
				var grant string
				if err := f.pool.QueryRow(context.Background(), `SELECT grant_id FROM case_evidence_review_grants WHERE principal_id=$1 AND revoked_at IS NULL`, actor).Scan(&grant); err != nil {
					t.Fatal(err)
				}
				f.adminString(t, `SELECT revoke_evidence_review($1,'fixture','revoked fixture')`, grant)
				want = 404
			case "authoritative", "unclassified":
				want = 409
			case "binding":
				b["bindingId"] = "11111111-1111-4111-8111-111111111111"
			case "digest":
				b["claimDigest"] = strings.Repeat("0", 64)
			case "version":
				b["requestVersion"] = 99
			case "stale":
				evidenceExec(t, f.pool, `UPDATE evt_requests SET version=version+1 WHERE request_id=$1`, request)
				want = 409
			case "rationale":
				b["reason"] = " "
			case "basis":
				b["reviewBasis"] = "UPLOADED_EVIDENCE"
			case "artifact-injection":
				b["uploadId"] = "11111111-1111-4111-8111-111111111111"
			case "check":
				path = strings.Replace(path, "employment.company_and_dates", "missing.check", 1)
				want = 404
			case "inactive":
				evidenceExec(t, f.pool, `UPDATE domain_users SET status='disabled' WHERE id=$1`, actor)
				want = 404
			}
			encoded, _ := json.Marshal(b)
			evidenceStatus(t, reviewRequest(f, actor, "POST", path, string(encoded)), want)
			var n int
			_ = f.pool.QueryRow(context.Background(), `SELECT count(*) FROM evidence_reviews`).Scan(&n)
			if n != 0 {
				t.Fatal("denied review persisted")
			}
			assertNoIntegrityIssuance(t, f, request)
		})
	}
}
func TestClaimOnlyConcurrentAndRollback(t *testing.T) {
	f, request, binding := bindingSetupContext(t, true, "SIMULATED_UAT")
	grantClaimReviewer(t, f)
	body := claimReviewBody(t, f, binding)
	var wg sync.WaitGroup
	codes := make(chan int, 2)
	for _, decision := range []string{"APPROVED", "REJECTED"} {
		wg.Add(1)
		go func(d string) {
			defer wg.Done()
			codes <- reviewRequest(f, f.other, "POST", claimReviewPath, strings.Replace(body, "APPROVED", d, 1)).Code
		}(decision)
	}
	wg.Wait()
	close(codes)
	seen := map[int]int{}
	for c := range codes {
		seen[c]++
	}
	if seen[200] != 1 || seen[409] != 1 {
		t.Fatal("concurrent decisions", seen)
	}
	assertNoIntegrityIssuance(t, f, request)
	g, _, b := bindingSetupContext(t, true, "SIMULATED_UAT")
	grantClaimReviewer(t, g)
	evidenceExec(t, g.pool, `CREATE FUNCTION refuse_claim_context() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'fixture rollback'; END $$; CREATE TRIGGER refuse_claim_context BEFORE INSERT ON request_evidence_review_contexts FOR EACH ROW EXECUTE FUNCTION refuse_claim_context()`)
	evidenceStatus(t, reviewRequest(g, g.other, "POST", claimReviewPath, claimReviewBody(t, g, b)), 503)
	var n int
	_ = g.pool.QueryRow(context.Background(), `SELECT count(*) FROM evidence_reviews`).Scan(&n)
	if n != 0 {
		t.Fatal("failed context left review")
	}
	evidenceExec(t, g.pool, `DROP TRIGGER refuse_claim_context ON request_evidence_review_contexts`)
	evidenceStatus(t, reviewRequest(g, g.other, "POST", claimReviewPath, claimReviewBody(t, g, b)), 200)
}
func TestClaimOnlyIssuanceDenial(t *testing.T) {
	for _, decision := range []string{"APPROVED", "REJECTED"} {
		t.Run(decision, func(t *testing.T) {
			x := issuanceSetupContext(t, "SIMULATED_UAT") // Includes independently reviewed documentary evidence and valid fixture issuer authority.
			// The legacy fixture intentionally has a partial status enum. Include
			// VERIFIED here so its denial must come from the real promotion guard,
			// rather than an invalid fixture enum label.
			evidenceExec(t, x.f.pool, `ALTER TYPE evt_request_status ADD VALUE 'VERIFIED'`)
			receipt := reviewRequest(x.f, x.f.other, "POST", claimReviewPath, strings.Replace(claimReviewBody(t, x.f, x.binding), "APPROVED", decision, 1))
			evidenceStatus(t, receipt, 200)
			spy := &countedIntegritySigner{Signer: x.engine.Signer}
			x.engine.Signer = spy
			if _, err := x.engine.Issue(context.Background(), x.f.other, x.request, "binding-employer"); err == nil || spy.count.Load() != 0 {
				t.Fatal("claim-only simulated review reached signer", err)
			}
			for _, stmt := range []string{`SELECT prepare_bound_issuance($1,$2,'binding-employer','fixture')`, `SELECT commit_bound_issuance($1,$2,'binding-employer','fixture','fake','{}'::jsonb)`} {
				if _, err := x.f.s.db.Exec(stmt, x.f.other, x.request); err == nil {
					t.Fatal("issuance bypass")
				}
			}
			for _, stmt := range []string{
				`UPDATE evt_requests SET status='VERIFIED' WHERE request_id=$1`,
				`UPDATE evt_requests SET status='ATTESTED' WHERE request_id=$1`,
				`UPDATE evt_requests SET attestation_jws='fake' WHERE request_id=$1`,
			} {
				_, err := x.f.pool.Exec(context.Background(), stmt, x.request)
				expectEmployerSQLState(t, err, "23514")
			}
			// The persisted unsigned receipt must not become trusted through either
			// the credential verifier or the legacy verification-outcome route.
			outcome := ec.Verify(context.Background(), receipt.Body.String(), x.f.principal, ec.Policy, time.Now().UTC(), &evtdb.IssuanceEngine{DB: x.engine.DB})
			if outcome.Decision == verification.VerificationVerified || outcome.Claim != nil {
				t.Fatal("unsigned claim receipt verified", outcome)
			}
			w := httptest.NewRecorder()
			body, _ := json.Marshal(map[string]string{"jwsCompact": receipt.Body.String()})
			x.f.s.HandleVerificationOutcome(w, httptest.NewRequest("POST", "/v1/verification/outcome", strings.NewReader(string(body))))
			if w.Code != 200 || strings.Contains(w.Body.String(), `"trust":"trusted"`) {
				t.Fatal("claim receipt promoted to trusted outcome", w.Body.String())
			}
			assertNoIntegrityIssuance(t, x.f, x.request)
		})
	}
}
func TestClaimOnlyMigrationRollbackACLAndReplay(t *testing.T) {
	f, _, _ := bindingSetupVersion(t, false)
	evidenceExec(t, f.pool, employerSQL(t, "../../migrations/000019_evidence_backed_issuance.sql"))
	evidenceExec(t, f.pool, employerSQL(t, "../../migrations/000020_claim_bound_review_integrity.sql"))
	ctx := context.Background()
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, employerSQL(t, "../../migrations/000021_claim_only_simulated_review.sql")); err != nil {
		t.Fatal(err)
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	var absent bool
	if err = f.pool.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM pg_attribute WHERE attrelid='evidence_reviews'::regclass AND attname='review_basis' AND NOT attisdropped)`).Scan(&absent); err != nil || !absent {
		t.Fatal("migration escaped rollback", err)
	}
	evidenceExec(t, f.pool, employerSQL(t, "../../migrations/000021_claim_only_simulated_review.sql"))
	for _, role := range []string{f.runtime, f.operator, "public"} {
		var allowed bool
		if err = f.pool.QueryRow(ctx, `SELECT has_function_privilege($1,'protect_claim_only_receipt()','EXECUTE')`, role).Scan(&allowed); err != nil || allowed {
			t.Fatal("helper leaked", err)
		}
	}
	if _, err = f.pool.Exec(ctx, employerSQL(t, "../../migrations/000021_claim_only_simulated_review.sql")); err == nil {
		t.Fatal("migration replay accepted")
	}
}

func TestClaimOnlyRequiresImmutableContextAtCommit(t *testing.T) {
	f, request, binding := bindingSetupContext(t, true, "SIMULATED_UAT")
	grantClaimReviewer(t, f)
	var grant string
	if err := f.pool.QueryRow(context.Background(), `SELECT grant_id FROM case_evidence_review_grants WHERE principal_id=$1 AND revoked_at IS NULL`, f.other).Scan(&grant); err != nil {
		t.Fatal(err)
	}
	_, err := f.pool.Exec(context.Background(), `INSERT INTO evidence_reviews(review_basis,claim_binding_id,claim_check_id,reviewer_id,grant_id,decision,reason) VALUES('CLAIM_ONLY',$1,'employment.company_and_dates',$2,$3,'APPROVED','missing context must fail')`, binding, f.other, grant)
	expectEmployerSQLState(t, err, "23514")
	_, err = f.s.db.Exec(`INSERT INTO evidence_reviews(review_basis,claim_binding_id,claim_check_id,reviewer_id,grant_id,decision,reason) VALUES('CLAIM_ONLY',$1,'employment.company_and_dates',$2,$3,'APPROVED','runtime bypass')`, binding, f.other, grant)
	expectEmployerSQLState(t, err, "42501")
	var n int
	if err = f.pool.QueryRow(context.Background(), `SELECT count(*) FROM evidence_reviews`).Scan(&n); err != nil || n != 0 {
		t.Fatal("invalid review persisted", err)
	}
	assertNoIntegrityIssuance(t, f, request)
}
