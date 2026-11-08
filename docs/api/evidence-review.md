# Bounded evidence review API

The owner approved this policy on 2026-11-07: case review grants are separate from
upload grants, operator-provisioned, attributable and revocable; active granted
reviewers can list/download/review received evidence; no self-review; manual decisions
are distinct from scan status. This document describes the implementation. The owner-approved production SQL
phase for 000017/bindings passed on 2026-11-08 UTC; the API is deployed at 6e7293e. Positive two-account HTTP acceptance also passed, including independent
review, revocation and unchanged PENDING scans. See the operations release evidence.

## Authorization and trust

Only a current explicit `case_evidence_review_grants` assignment grants review
access. An HR role, upload grant, knowledge of an upload UUID, and ownership of a
past upload do not establish review access. Every action uses the signed canonical
`domain_users.id`, not a body-supplied reviewer. Case OPEN and check PENDING or
IN_REVIEW are required. Inactive principals, revoked grants and closed/deleting
resources deny access. A reviewer with a separate valid grant can view their own
files but cannot record either decision on them.

Manual decisions are APPROVED or REJECTED, with reviewer UUID, authorizing grant,
server time and a nonempty reason of at most 1,000 UTF-8 bytes. This first bounded
slice records **one final manual decision per upload**. An exact retry by the same
reviewer with the same decision/reason returns the original receipt; conflicting
re-review returns 409. Review records cannot be edited or deleted by application
or provisioning roles. Corrections/re-review are outside this first slice; an
operator can close a case to withdraw further access rather than silently rewrite
its history.

Approval means a named authorized person recorded their review. It does not mean
malware-free, does not change `scan_status` from PENDING to CLEAN, does not change
case/check workflow state, and does not create or authorize a human-claims token.
No relationship to employers or employment requests is inferred. Revocation stops
future access/decisions; it does not erase past decision attribution.

## Routes

All routes require the existing Bearer access token. No token or object URL is
returned by these endpoints.

- `GET /v1/evidence/checks` returns `{items:[{caseId,checkId,canUpload,canReview}]}`,
  ordered by case/check, up to 100 authorized writable pairs. Flags reflect the
  existing explicit upload/review ledgers, not inferred roles. No new assignments
  are made. This provides real identifiers for the client. Discovery is a bounded
  snapshot; every subsequent action independently rechecks current authorization.
- `GET /v1/cases/{caseId}/checks/{checkId}/evidence` requires a review grant. Returns
  received-file metadata, canonical `evidence_hash`, scan status, manual review
  status/attribution/reason and whether a first decision is permitted. No staging
  or sealed storage keys, presigned URLs or uploader identity are returned.
- `GET /v1/cases/{caseId}/checks/{checkId}/evidence/{uploadId}/download` requires the
  same current review grant and exact persisted case/check/upload relationship.
  Only received objects with PENDING or CLEAN scan status can be downloaded.
  The API privately reads the sealed object, checks actual bytes, size, MIME and
  canonical hash, then serves an attachment as application/octet-stream with
  nosniff and private/no-store headers. It never redirects to object storage or
  issues a reusable download capability. The scan status is explicitly identified.
  A download request already authorized/completed cannot recall bytes the reviewer
  has received; grant/case changes prevent subsequent requests.
- `POST /v1/cases/{caseId}/checks/{checkId}/evidence/{uploadId}/review` takes
  `{"decision":"APPROVED","reason":"Reviewed source document"}` or REJECTED.
  Returns reviewId, uploadId, reviewerId, decision, reason and reviewedAt. A scan
  marked INFECTED/ERROR cannot be approved or downloaded; an authorized independent
  reviewer may reject it without downloading. Missing scan work fails closed.

Unauthorized resources return 404, missing authentication 401, self-review 403,
conflicting final decisions or unsafe approval 409, invalid review input 400, and
unavailable authorization/persistence/storage or integrity mismatch 503. Error
responses reveal no object keys, contents or raw database/storage errors.

## Database and concurrency

Migration 000017 adds only the explicit reviewer grant ledger and immutable manual
review receipts. It reuses migration 000015 text case/check identifiers and migration
000016 UUID upload/principal relationships. No existing user/grant/case data is
backfilled. Both new tables have owner-only ACLs, including removal of inherited
legacy default grants. New functions are fixed-path SECURITY DEFINER routines with
PUBLIC/default EXECUTE stripped; the separate binding grants only bounded runtime
list/get/record functions and operator grant/revoke functions. Runtime and operator
receive no direct new-table access. Neither can use these functions to mark scans
CLEAN or provision implicit case/request associations.

Review authorization locks principal, case, check and reviewer grant in that order
under READ COMMITTED. Download holds these and the sealed-file/scan rows through
object verification and database commit. Review decisions also lock the upload row,
serializing independent reviewers and retries. Revocation/status changes either win
first and deny or wait for an authorized in-flight action. RLS changes, unsupported
isolation or missing privileges fail closed instead of bypassing a new policy.
Temporary relations cannot shadow the fixed-path database functions.

## Compatibility and release gate

Existing init/completion and the approved PENDING boundary remain unchanged.
New routes fail closed if migration 000017/bindings are absent; deployment must
not be represented as review-ready until operator SQL and focused acceptance pass.
The migration is atomic/once-only with no invented historical migration state.
Use the existing restricted runtime and NOLOGIN evidence operator; no new service,
role, credential, provider, scanner worker or background job is required.

Production release requires the separately authorized owner-console SQL phase,
a current restore-tested backup/recovery image, final CI, then the existing exact-main
workflow and a synthetic two-principal smoke (uploader and independent reviewer).
No production review assignment, data access or privileged signing is authorized
by this feature document. The previously approved live-signing deferral remains.

## Local verification

Commands run from `apps/evt-api-go`, using the existing guarded disposable PostgreSQL
wrapper (no production connection):

```sh
python3 /private/tmp/cvera-proposal-b-test.py test ./... -count=1 -timeout=120s
python3 /private/tmp/cvera-proposal-b-test.py test -race ./internal/httpapi ./internal/storage -count=1 -timeout=120s
go vet ./...
```

Full tests, race checks and vet passed. Regression coverage exercises the actual
000017 migration/binding, separate upload/review grants, HR-role-only denial,
cross-case/upload denial, active/principal/resource state checks, immutable
attributable decisions, idempotent retry/conflict, forbidden self-review, unchanged
PENDING scans, unsafe-scan rejection, bounded private attachments, tampered/missing
objects, missing function permissions/RLS failing closed, discovery capability
flags, runtime/operator/PUBLIC privilege boundaries, real PostgreSQL blocking in
both revocation orders and competing final decisions, plus atomic rollback on a
failed binding with existing upload initialization remaining operational.

The approved migration and exact-main API deployment passed on 2026-11-08 UTC
at 6e7293e; public health/version/JWKS and anonymous denial probes passed. The separate owner-run two-principal HTTP acceptance also passed at that exact
revision: grant separation, wrong case/check denial, no self-review, private
canonical bytes, attribution/idempotency/conflict, revocation and cleanup. Manual
approval left scans PENDING. No real-user evidence or signing was involved. See
the operations release evidence for current results.
