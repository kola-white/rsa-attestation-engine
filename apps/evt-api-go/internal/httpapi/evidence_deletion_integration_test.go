package httpapi

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func deletionSettings(t *testing.T, tx pgx.Tx, operation, basis, fingerprint string) {
	t.Helper()
	_, err := tx.Exec(context.Background(), `SELECT set_config('cvera.purge_case','case-a',true),set_config('cvera.purge_actor','fixture-owner',true),set_config('cvera.purge_basis',$1,true),set_config('cvera.purge_operation',$2,true),set_config('cvera.purge_fingerprint',$3,true)`, basis, operation, fingerprint)
	if err != nil {
		t.Fatal(err)
	}
}
func TestEvidenceDeletionOwnerOnlyFreezeAndRedaction(t *testing.T) {
	f, _, init := reviewSetup(t)
	_, download, review := reviewPaths(init)
	f.adminString(t, `SELECT grant_evidence_review($1,'case-a','fixture','review')`, f.other)
	evidenceStatus(t, reviewRequest(f, f.other, "POST", review, `{"decision":"APPROVED","reason":"Private source details"}`), 200)
	ctx := context.Background()
	operation := uuid.NewString()
	basis := "approved synthetic deletion"
	// Neither application runtime nor provisioning operator receives redaction power.
	for _, role := range []string{f.runtime, f.operator} {
		tx, err := f.pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = tx.Exec(ctx, `SET LOCAL ROLE `+pgx.Identifier{role}.Sanitize()); err != nil {
			t.Fatal(err)
		}
		deletionSettings(t, tx, operation, basis, "")
		_, err = tx.Exec(ctx, employerSQL(t, "../../operations/evidence-case-delete-freeze.sql"))
		expectEmployerSQLState(t, err, "42501")
		tx.Rollback(ctx)
	}
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	deletionSettings(t, tx, operation, basis, "")
	if _, err = tx.Exec(ctx, employerSQL(t, "../../operations/evidence-case-delete-freeze.sql")); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	evidenceStatus(t, f.request(f.principal, "case-a", "employment.company_and_dates", false, evidenceFile), 404)
	evidenceStatus(t, completionRequest(f, f.principal, init, "/v1/evidence/commit"), 404)
	evidenceStatus(t, reviewRequest(f, f.other, "GET", download, ""), 404)
	var state string
	var active int
	if err = f.pool.QueryRow(ctx, `SELECT status,(SELECT count(*) FROM case_evidence_upload_grants WHERE case_id='case-a' AND revoked_at IS NULL)+(SELECT count(*) FROM case_evidence_review_grants WHERE case_id='case-a' AND revoked_at IS NULL) FROM cases WHERE case_id='case-a'`).Scan(&state, &active); err != nil || state != "DELETION_SCHEDULED" || active != 0 {
		t.Fatal("freeze did not revoke access", state, active, err)
	}
	var fingerprint string
	if err = f.pool.QueryRow(ctx, `SELECT md5(coalesce(jsonb_agg(to_jsonb(u) ORDER BY upload_id)::text,'[]')) FROM evidence_uploads u WHERE case_id='case-a'`).Scan(&fingerprint); err != nil {
		t.Fatal(err)
	}
	// Physical deletion is outside SQL. SQL independently refuses early redaction.
	early, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	deletionSettings(t, early, operation, basis, fingerprint)
	if _, err = early.Exec(ctx, employerSQL(t, "../../operations/evidence-case-delete-redact.sql")); err == nil {
		t.Fatal("valid PUT capability ignored")
	}
	early.Rollback(ctx)
	evidenceExec(t, f.pool, `UPDATE upload_sessions SET created_at=$1,expires_at=$2 WHERE case_id='case-a'`, time.Now().Add(-time.Hour), time.Now().Add(-2*time.Minute))
	// A wrong manifest fingerprint never mutates metadata.
	wrong, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	deletionSettings(t, wrong, operation, basis, "wrong")
	if _, err = wrong.Exec(ctx, employerSQL(t, "../../operations/evidence-case-delete-redact.sql")); err == nil {
		t.Fatal("changed manifest accepted")
	}
	wrong.Rollback(ctx)
	final, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer final.Rollback(ctx)
	deletionSettings(t, final, operation, basis, fingerprint)
	if _, err = final.Exec(ctx, employerSQL(t, "../../operations/evidence-case-delete-redact.sql")); err != nil {
		t.Fatal(err)
	}
	if err = final.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var filename, staging, sealed, reason, decision, scan string
	if err = f.pool.QueryRow(ctx, `SELECT u.file_name,u.staging_key,u.sealed_key,r.reason,r.decision,j.scan_status FROM evidence_uploads u JOIN evidence_reviews r USING(upload_id) JOIN evidence_scan_jobs j USING(upload_id) WHERE u.upload_id=$1`, init.Uploads[0].UploadID).Scan(&filename, &staging, &sealed, &reason, &decision, &scan); err != nil {
		t.Fatal(err)
	}
	if filename != "[deleted]" || staging != "deleted-staging/"+init.Uploads[0].UploadID || sealed != "sealed-evidence/deleted/"+init.Uploads[0].UploadID || reason != "[redacted following authorized evidence deletion]" || decision != "APPROVED" || scan != "PENDING" {
		t.Fatal("redaction/audit trust boundary failed")
	}
	if err = f.pool.QueryRow(ctx, `SELECT status FROM cases WHERE case_id='case-a'`).Scan(&state); err != nil || state != "DELETED" {
		t.Fatal(state, err)
	}
}
func TestEvidenceSealedKeysAreCaseAttributable(t *testing.T) {
	f, m, init := reviewSetup(t)
	var key string
	if err := f.pool.QueryRow(context.Background(), `SELECT sealed_key FROM evidence_uploads WHERE upload_id=$1`, init.Uploads[0].UploadID).Scan(&key); err != nil {
		t.Fatal(err)
	}
	prefix := "sealed-evidence/case-a/employment.company_and_dates/" + init.Uploads[0].UploadID + "/"
	if len(key) <= len(prefix) || key[:len(prefix)] != prefix {
		t.Fatal("sealed object lacks case/check/upload attribution")
	}
	if _, err := uuid.Parse(key[len(prefix):]); err != nil {
		t.Fatal("missing independent server nonce")
	}
	if string(m.objects[key]) != "a" {
		t.Fatal("namespacing changed sealed bytes")
	}
}
