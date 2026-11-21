package httpapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/auth"
	evtdb "github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/db"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/evt"
)

func requestorOutcomeEngine(t *testing.T, f evidenceFixture) *gin.Engine {
	t.Helper()
	evidenceExec(t, f.pool, employerSQL(t, "../../migrations/000022_requestor_review_outcomes.sql"))
	evidenceExec(t, f.pool, `UPDATE domain_users SET role='requestor' WHERE id=$1`, f.principal)
	pool, err := pgxpool.New(context.Background(), serviceRoleURL(t, f.pool, f.runtime))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	d := &evtdb.DB{Pool: pool}
	m := &evt.Module{Candidate: &evt.CandidateHandlers{DB: d, Repo: &evt.Repo{DB: d}}}
	engine := gin.New()
	engine.Use(auth.GinRequireAuth(f.s.cfg.Auth.JWTSecret, f.s.cfg.Auth.Issuer, f.s.cfg.Auth.Audience))
	m.Register(engine.Group("/v1"))
	return engine
}

func readRequestorOutcomes(t *testing.T, f evidenceFixture, engine *gin.Engine, request string) map[string]any {
	t.Helper()
	w := employerCall(engine, employerToken(t, f.s, f.principal, auth.RoleRequestor), "GET", "/v1/requests/"+request+"/review-outcomes", "")
	evidenceStatus(t, w, 200)
	if w.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatal("private outcomes must not be cached")
	}
	var result map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func reviewStateFingerprint(t *testing.T, f evidenceFixture) string {
	t.Helper()
	var value string
	err := f.pool.QueryRow(context.Background(), `SELECT jsonb_build_array(
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY id) FROM domain_users x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY request_id) FROM evt_requests x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY binding_id) FROM request_case_bindings x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY review_id) FROM evidence_reviews x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY review_id) FROM request_evidence_review_contexts x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY grant_id) FROM case_evidence_review_grants x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY assignment_id) FROM employer_hr_assignments x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY credential_id) FROM evidence_issued_credentials x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY binding_id) FROM request_review_classifications x)
 )::text`).Scan(&value)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestRequestorOutcomesClaimDecisionsAndReadOnlyBoundary(t *testing.T) {
	for _, decision := range []string{"APPROVED", "REJECTED"} {
		t.Run(decision, func(t *testing.T) {
			f, request, binding := bindingSetupContext(t, true, "SIMULATED_UAT")
			grantClaimReviewer(t, f)
			body := strings.Replace(claimReviewBody(t, f, binding), "APPROVED", decision, 1)
			evidenceStatus(t, reviewRequest(f, f.other, "POST", claimReviewPath, body), 200)
			// Change fixture account role before measuring migration preservation.
			evidenceExec(t, f.pool, `UPDATE domain_users SET role='requestor' WHERE id=$1`, f.principal)
			beforeMigration := reviewStateFingerprint(t, f)
			engine := requestorOutcomeEngine(t, f)
			if reviewStateFingerprint(t, f) != beforeMigration {
				t.Fatal("read migration changed records")
			}
			before := reviewStateFingerprint(t, f)
			for i := 0; i < 2; i++ {
				out := readRequestorOutcomes(t, f, engine, request)
				if out["state"] != "CURRENT" || out["reviewContext"] != "SIMULATED_UAT" || out["requestStatus"] != "ATTESTATION_PENDING" {
					t.Fatal(out)
				}
				items := out["outcomes"].([]any)
				if len(items) != 1 {
					t.Fatal(items)
				}
				row := items[0].(map[string]any)
				if row["decision"] != decision || row["reason"] != "Reviewed the immutable submitted claim." || row["reviewedAt"] == nil || row["reviewId"] == nil {
					t.Fatal(row)
				}
				for _, private := range []string{"reviewerId", "grantId", "subjectId", "claimSnapshot", "credential", "uploadId"} {
					if _, exists := row[private]; exists {
						t.Fatal("private field disclosed", private)
					}
				}
			}
			if reviewStateFingerprint(t, f) != before {
				t.Fatal("GET changed persisted state")
			}
			assertNoIntegrityIssuance(t, f, request)
			var grants int
			if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM case_evidence_review_grants WHERE principal_id=$1`, f.principal).Scan(&grants); err != nil || grants != 0 {
				t.Fatal("requestor acquired reviewer access", err)
			}
			// Runtime can execute only the bounded reader; it still cannot SELECT the ledgers.
			var direct, execute bool
			if err := f.pool.QueryRow(context.Background(), `SELECT has_table_privilege($1,'evidence_reviews','SELECT'),has_function_privilege($1,'requestor_review_outcomes(uuid,uuid)','EXECUTE')`, f.runtime).Scan(&direct, &execute); err != nil || direct || !execute {
				t.Fatal(direct, execute, err)
			}
			for _, role := range []string{f.operator, "public"} {
				if err := f.pool.QueryRow(context.Background(), `SELECT has_function_privilege($1,'requestor_review_outcomes(uuid,uuid)','EXECUTE')`, role).Scan(&execute); err != nil || execute {
					t.Fatal("unexpected EXECUTE recipient", role, err)
				}
			}
		})
	}
}

func TestRequestorOutcomesOwnershipAndAccountAuthorization(t *testing.T) {
	f, request, _ := bindingSetupContext(t, true, "SIMULATED_UAT")
	engine := requestorOutcomeEngine(t, f)
	otherRequestor := uuid.NewString()
	evidenceExec(t, f.pool, `INSERT INTO domain_users(id,kratos_identity_id,role,status) VALUES($1::uuid,$1::uuid::text,'requestor','active')`, otherRequestor)
	path := "/v1/requests/" + request + "/review-outcomes?principal_id=" + f.principal
	for _, tc := range []struct {
		principal string
		role      auth.Role
		want      int
	}{
		{otherRequestor, auth.RoleRequestor, 404}, {f.other, auth.RoleRequestor, 404}, {f.principal, auth.RoleHRReviewer, 403},
		{f.principal, auth.RoleRecruiter, 403}, {f.principal, auth.RoleCvera, 403},
		{"not-a-uuid", auth.RoleRequestor, 404},
	} {
		w := employerCall(engine, employerToken(t, f.s, tc.principal, tc.role), "GET", path, "")
		evidenceStatus(t, w, tc.want)
		if strings.Contains(w.Body.String(), "SIMULATED_UAT") {
			t.Fatal("denial leaked review context")
		}
	}
	for _, suffix := range []string{uuid.NewString(), "not-a-uuid"} {
		evidenceStatus(t, employerCall(engine, employerToken(t, f.s, f.principal, auth.RoleRequestor), "GET", "/v1/requests/"+suffix+"/review-outcomes", ""), 404)
	}
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
	evidenceStatus(t, w, 401)
	for _, change := range []string{"UPDATE domain_users SET status='inactive' WHERE id=$1", "UPDATE domain_users SET status='active',role='hr_reviewer' WHERE id=$1"} {
		evidenceExec(t, f.pool, change, f.principal)
		evidenceStatus(t, employerCall(engine, employerToken(t, f.s, f.principal, auth.RoleRequestor), "GET", path, ""), 404)
	}
}

func TestRequestorOutcomesPendingEmptyStaleAndRevoked(t *testing.T) {
	for _, kind := range []string{"pending", "unbound", "stale-version", "stale-digest", "revoked", "deleted", "unclassified"} {
		t.Run(kind, func(t *testing.T) {
			mode := "SIMULATED_UAT"
			if kind == "unclassified" {
				mode = ""
			}
			f, request, binding := bindingSetupContext(t, true, mode)
			if kind != "pending" && kind != "unbound" && kind != "unclassified" {
				grantClaimReviewer(t, f)
				evidenceStatus(t, reviewRequest(f, f.other, "POST", claimReviewPath, claimReviewBody(t, f, binding)), 200)
			}
			engine := requestorOutcomeEngine(t, f)
			want := "CURRENT"
			switch kind {
			case "unbound":
				request = uuid.NewString()
				evidenceExec(t, f.pool, `INSERT INTO evt_requests(request_id,candidate_personid,employer_id,status) VALUES($1,$2,'binding-employer','DRAFT')`, request, f.principal)
				want = "NOT_BOUND"
			case "stale-version":
				evidenceExec(t, f.pool, `UPDATE evt_requests SET version=version+1 WHERE request_id=$1`, request)
				want = "STALE"
			case "stale-digest":
				evidenceExec(t, f.pool, `UPDATE evt_requests SET claim_snapshot='{"title":"changed"}' WHERE request_id=$1`, request)
				want = "STALE"
			case "revoked":
				f.adminString(t, `SELECT revoke_request_evidence_binding($1,'fixture','withdraw binding')`, binding)
				want = "UNAVAILABLE"
			case "deleted":
				evidenceExec(t, f.pool, `UPDATE cases SET status='DELETION_SCHEDULED' WHERE case_id='case-a'`)
				want = "UNAVAILABLE"
			case "unclassified":
				want = "UNAVAILABLE"
			}
			out := readRequestorOutcomes(t, f, engine, request)
			if out["state"] != want {
				t.Fatal(out)
			}
			items := out["outcomes"].([]any)
			if kind == "pending" {
				if len(items) != 1 || items[0].(map[string]any)["decision"] != "PENDING" {
					t.Fatal(out)
				}
			} else if len(items) != 0 || out["reviewContext"] != nil {
				t.Fatal("unavailable context disclosed outcomes", out)
			}
		})
	}
}

func TestRequestorOutcomesAuthoritativeUploadedEvidence(t *testing.T) {
	f, request, binding := bindingSetupContext(t, true, "AUTHORITATIVE")
	boundReceipt(t, f, binding)
	engine := requestorOutcomeEngine(t, f)
	out := readRequestorOutcomes(t, f, engine, request)
	items := out["outcomes"].([]any)
	if out["reviewContext"] != "AUTHORITATIVE" || out["requestStatus"] != "ATTESTATION_PENDING" || len(items) != 1 || items[0].(map[string]any)["reviewBasis"] != "UPLOADED_EVIDENCE" || items[0].(map[string]any)["decision"] != "APPROVED" {
		t.Fatal(out)
	}
	assertNoIntegrityIssuance(t, f, request)
}

// The new database reader must work in an enforced read-only transaction itself.
func TestRequestorOutcomesSQLReadOnly(t *testing.T) {
	f, request, _ := bindingSetupContext(t, true, "SIMULATED_UAT")
	requestorOutcomeEngine(t, f)
	tx, err := f.pool.BeginTx(context.Background(), pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	var out []byte
	if err = tx.QueryRow(context.Background(), `SELECT requestor_review_outcomes($1,$2)`, f.principal, request).Scan(&out); err != nil {
		t.Fatal(err)
	}
}

func TestRequestorOutcomesMissingReaderFailsClosed(t *testing.T) {
	f, request, _ := bindingSetupContext(t, true, "SIMULATED_UAT")
	engine := requestorOutcomeEngine(t, f)
	evidenceExec(t, f.pool, `DROP FUNCTION requestor_review_outcomes(uuid,uuid)`)
	w := employerCall(engine, employerToken(t, f.s, f.principal, auth.RoleRequestor), "GET", "/v1/requests/"+request+"/review-outcomes", "")
	evidenceStatus(t, w, 503)
	if w.Body.String() != `{"error":"review_outcomes_unavailable"}` {
		t.Fatal("database failure leaked details", w.Body.String())
	}
}
