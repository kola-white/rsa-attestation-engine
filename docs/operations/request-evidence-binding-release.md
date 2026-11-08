# Request/evidence binding release

Preparation only. Production privileged SQL needs the owner's explicit approval
of the reviewed migration and exact bindings. The application authority role,
credentials, signing enablement and native-device acceptance are excluded.

## Approved product boundary

Owner-approved exclusive request/case/check/context binding and manual evidence
eligibility, with exact delegated issuer `https://cvera.app`. The identity/pilot
policy row is not an authority grant. Production authority is intentionally
unconfigured and returns ineligible; no authority migrations or privileges are
silently added. Current accepted API baseline is 6e7293e, main documentation c4d7f9c.

## One controlled schema phase

Use only the established DigitalOcean root console and `evt-api-go-db-1`/evt as
postgres. The guarded `release-request-evidence-binding-sql.py` pins the reviewed
000018 SQL and binding bytes, verifies the accepted live API and protected runtime
configuration, validates actual parent ownership/RLS and request version/JSONB
claim types, makes a fresh private backup and restore-tests it in the established
64 MiB network-none PostgreSQL clone. No live volume is shared or service stopped.

The clone rehearses transaction ROLLBACK and COMMIT, owner-only tables/triggers,
fixed function paths/default ACL stripping, exact runtime/operator EXECUTE and
unauthorized denial, then confirms unchanged parent catalog/roles. Only after
those checks does one READ COMMITTED production transaction apply **000018 only**
and its bindings with 5-second lock/60-second statement limits. Private intent,
actual transaction/source hashes and fresh verification record the real change;
no earlier migration history or new migration runner is invented.

Runtime receives only four new bounded context/review/preparation/signing-exclusion
entry points and the replaced standalone-review wrapper's existing EXECUTE. The
existing NOLOGIN evidence operator receives bind/revoke only. No direct new-table
SELECT/INSERT/UPDATE/DELETE/TRUNCATE, grant options, memberships, new roles, authority
schema permissions or credentials are granted. Existing legacy table ACLs remain
unchanged. Five owner-only tables include the single approved identity/pilot policy
row; no binding, subject, employer, request, case, check, grant or manifest is seeded.

Stop on any failure or unexpected state; never rerun committed once-only DDL or add
ad hoc privileges. The staged driver passed an actual local synthetic PostgreSQL
archive restore, rollback/commit, exact ACL/denial, contextual review/manifest,
unsigned HR-rejection event/sequence compatibility, catalog/data-preservation and
rerun-rejection rehearsal. That same rollback-only synthetic SQL probe runs only
in the restored clone, never the production database. That is not production schema verification.

## Merge and acceptance

Keep main unchanged until the production schema phase passes. Then require final
PR CI and use the existing descriptive squash merge/exact-main deployment. Verify
health/version/JWKS, restricted startup and the authenticated eligibility boundary.
Use two existing normal accounts and an explicitly approved two-request/
case/check/grant fixture under one synthetic employer for context acknowledgement, reviewed manifest, scan
PENDING visibility, unavailable-authority ineligibility, cross-request denial and
409 on positive legacy attestation. No signed credential may be created. Use only
synthetic claims/file bytes, never print credentials/URLs/contents or access real
user evidence. Request fixture provisioning/cleanup must be explicitly authorized;
it is not evidence that the normal candidate create/submit flow was accepted.

Revoke fixture binding/grants, close its case, log out sessions and terminalize the
synthetic request using the supported **unsigned** HR rejection path. Do not delete
audit, restore records, change roles or create identities to make acceptance pass.
Native picker/device acceptance and real live signing remain separate.

## Recovery: preserve the new signing exclusion

Before any binding exists, leaving additive 000018 and restoring the retained
6e7293e API image/configuration is compatible with standalone review. Once bound
requests exist, old code lacks the new signing guard. **Do not roll back to an
older image while any bound request remains ATTESTATION_PENDING, including revoked
bindings.** Case closure or binding revocation alone does not protect an older
signer that never reads them. This is a material recovery precondition.

The existing owner console can check the precondition read-only, without records:

```sql
BEGIN READ ONLY;
SELECT NOT EXISTS (
 SELECT 1 FROM public.request_case_bindings b
 JOIN public.evt_requests r ON r.request_id=b.request_id
 WHERE r.status::text='ATTESTATION_PENDING'
) AS older_api_rollback_safe;
ROLLBACK;
```

If false, keep the guarded image and use a compatible correction, or stop for an
owner-controlled recovery decision. Never unbind/truncate evidence, fabricate a
rejection of a real request, restore older superuser connections or re-enable the
legacy exposed credential to force rollback. Database restore, destructive case
deletion, real request transitions and credential operations require separate
explicit authority. During release, provision only the approved synthetic fixture;
its normal unsigned terminal cleanup restores the safe older-image precondition.

## Guarded acceptance tooling

`smoke-request-evidence-binding.py` uses the existing reviewed login/HTTP helpers
and requires the exact deployed SHA plus two different existing normal accounts.
It verifies an existing active HR role without changing it. The separately
approved fixture creates one synthetic employer and two request/case/check pairs,
explicit scoped HR/upload/review grants and bindings, then uploads one public
synthetic PNG. Matching claim/subject/employer context across the two requests
must not permit upload, review or manifest reuse. Reviewed evidence stays PENDING;
production authority remains unavailable/ineligible; positive legacy signing is
explicitly denied. Cleanup revokes bindings/grants, closes cases and uses only
unsigned HR rejection to terminalize those exact synthetic requests. It verifies
no credential exists and that the older-image recovery precondition is restored.

No normal candidate create/submit acceptance is claimed by SQL fixture creation.
Existing registered identities, credentials, real requests and real evidence
remain untouched. The actual normal candidate/HR issuance flow is a separate,
explicitly authorized next milestone. STOP retains an exact private fixture plan;
never blindly rerun provisioning or use an older signer to bypass the guard.
Offline client-contract tests reject wrong request context and fabricated positive
authority, and verify that private receipts retain no tokens or signed URLs.
