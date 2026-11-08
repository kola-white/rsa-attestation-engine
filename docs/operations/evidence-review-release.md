# Evidence pilot release: one controlled operator phase

This is preparation, not authorization or production execution. It covers PR #136
(review/discovery), #137 (Expo completion) and #138 (retention tooling). The owner
must separately authorize privileged production SQL and deployment. Production
case deletion, credentials, new identities, employer assignments and signing are
excluded. Keep scan jobs PENDING after manual review.

## Before any merge to main

Use the existing DigitalOcean root console and exact reviewed Git commits. Confirm
current `/version`, health and clean running checkout; do not assume the checkout
SHA equals the API binary after documentation-only merges. Verify the live API role
is still `cvera_api_runtime`, operator `cvera_evidence_operator` is restricted
NOLOGIN, and the schema is through 000016 with postgres ownership and no RLS.
No workstation SSH account or production secret retrieval is needed.

Create a fresh root-private evt custom-format dump, verify its checksum and restore
it into a separate network-none PostgreSQL container without sharing live volumes.
Use the previously verified 64 MiB hard-capped rehearsal configuration, retain host
headroom, stop the clone afterward, and keep existing recovery material intact.
Retain/tag the exact current API image and protected configuration for recovery.
Use the same current image/settings for recovery; do not restore old superuser
connections or re-enable the exposed legacy evt-app credential.

On the restored clone, rehearse the exact migration-plus-binding transaction below
with ROLLBACK, confirm both new tables/functions absent, then COMMIT and verify
ACLs/denials. No cases or grants are backfilled. The existing local integration
suite already covers these operations and revoked/status/concurrent behavior;
the restore rehearsal establishes actual current production compatibility.

Stage the reviewed migration/binding with their relative directories preserved,
root-only permissions, and verify SHA-256 against the selected source commit.
This is the complete production schema transaction, executed by postgres through
the existing `docker exec -i evt-api-go-db-1 psql -X -U postgres -d evt` channel:

```sql
\set ON_ERROR_STOP on
BEGIN ISOLATION LEVEL READ COMMITTED;
SET LOCAL search_path=public,pg_temp;
SET LOCAL lock_timeout='5s';
SET LOCAL statement_timeout='60s';
SET LOCAL cvera.evidence_runtime_role='cvera_api_runtime';
SET LOCAL cvera.evidence_operator_role='cvera_evidence_operator';
\ir /EXACT_REVIEWED_STAGE/migrations/000017_evidence_review.sql
\ir /EXACT_REVIEWED_STAGE/operations/evidence-review-privileges.sql
-- Verify the new objects/ACLs below in this same transaction before COMMIT.
```

The binding independently rejects missing/non-restricted/owner/member/create-capable
roles, wrong owners, RLS, unexpected table or column privileges, and unsafe function
ownership/path/default EXECUTE. ON_ERROR_STOP plus connection exit rolls back an
uncommitted failure. Do not add grants to work around a failure. Do not rerun a
committed once-only migration or fabricate 000001–000013 history.

Before COMMIT, verify empty review-grant/receipt tables; postgres ownership and
valid indexes; no nonowner table/column grants; PUBLIC and legacy evt-app have no
new function EXECUTE; runtime has only list/discovery/get/record EXECUTE and
operator only grant/revoke EXECUTE. Internal checker/guard remain owner-only.
Verify missing principal/case/check under SET LOCAL ROLE cvera_api_runtime returns
empty discovery and no successful resource access. Reset role and record
`txid_current()` plus source SQL hashes privately before COMMIT. Repeat catalog
verification in a fresh read-only transaction afterward. Keep that actual-change
record with the backup; do not invent a repository migration ledger.

## Merge, deploy and focused acceptance

Final head CI must pass. Keep main unchanged until successful production SQL
verification; main pushes automatically deploy. Integrate the focused slices on
the feature branch first if needed so the approved API goes through one normal
exact-main deployment rather than several unnecessary intermediate deployments.
Use descriptive squash commits and existing tests/build-vulnerability/deployment
workflow. No credential/configuration change or Kratos restart is needed.

Verify container/health/version, database errors and public JWKS continuity using
sanitized results. For HTTP acceptance, use two independently authenticated normal
accounts (uploader and reviewer) and one explicitly authorized synthetic case/check
and tiny synthetic file. Operator provisioning is limited to that exact fixture:

1. Existing upload grant allows init/complete but no list/download/review.
2. Separate review grant permits exact-case listing and hash-verified private
   attachment download. Another user/case and revoked/inactive access deny.
3. Self-review denies even with a separate reviewer grant. The independent reviewer
   records APPROVED with canonical attribution; exact retry is idempotent and a
   conflicting decision denies. Scan status remains PENDING.
4. Revoke the fixture grants and close the case; subsequent access denies. No live
   issuance or real-user evidence is accessed. Retain the synthetic audit; do not
   invoke the destructive deletion tool as smoke cleanup without explicit authority.
5. Load the Expo client through the normal approved distribution path; select the
   server-listed fixture case/check, exercise PUT/complete/retry and confirm the
   received-awaiting-review display. A local web export alone is not device proof.

Never print credentials, signed URLs, object keys or file contents. If two usable
accounts are unavailable, report that acceptance gate; do not insert identities or
weaken no-self-review. Normal registration remains the supported account path.

## Failure and recovery

A failed pre-commit transaction leaves no new schema. Stop on unexpected state.
After a successful additive migration, if API acceptance fails, recover the exact
retained restricted-runtime API image/configuration through the established
recovery procedure. Leaving the unused additive review tables is compatible with
c5fdb5a completion. Do not drop review data or restore the database to force a
rollback. Review functions can remain unavailable while the earlier API operates.
Any destructive database restore needs a separate decision. Preserve source hashes,
transaction/result evidence and backups privately; do not claim deployment or
acceptance until the actual workflow and smoke results pass.

## Guarded console driver

`apps/evt-api-go/operations/release-evidence-review-sql.py` implements this approved
SQL phase using immutable migration/binding hashes from 7d97e12. It checks the
known c5fdb5a API and accepted clean checkout, restricted runtime and protected
configuration, fresh backup capacity, and current parent types/owners/RLS. It
restore-tests the new dump, rehearses rollback/commit and precise ACL/denials, then
applies only 000017 and bindings. Source hashes and actual transaction/result are
recorded privately. Existing services/configuration/roles are compared afterward;
no fixture resources/grants are created by this schema phase.

The driver passed an actual local network-none 64 MiB PostgreSQL backup/restore,
rollback/commit, exact ACL/fail-closed checks, existing registry preservation and
rerun rejection. This is local preparation, not production execution evidence.
A production STOP must be inspected without blind retry or merge/deployment.

## Production SQL evidence

On 2026-11-08 UTC the owner returned PASS for the checksum-verified driver at
0205d47. Migration **000017 only** and restricted bindings were applied from source
7d97e12 after a fresh actual backup restore and rollback/commit rehearsal.
Parent catalog/roles remained unchanged, API recovery was retained, and no
resources/grants, credentials, configuration or services changed. The actual
transaction/source record stays private on the Droplet. This establishes schema
compatibility, not deployed HTTP or device acceptance.

`smoke-evidence-review.py`, staged beside the reviewed completion smoke helper,
implements the approved two-account acceptance. It requires the exact deployed
main SHA and two different normally authenticated canonical principals before
provisioning a unique synthetic fixture. Its offline contract test exercises
grant separation, self-review denial, attribution/retry/conflict/revocation and
corrupt completion stopping before review. It revokes fixture grants, closes the
case and attempts both API and Kratos logout for each account. It does not create
identities, delete evidence, sign claims or change scan status. STOP retains a
private exact-fixture intent for inspection; do not blindly rerun or print it.

## Deployment evidence — 2026-11-08 UTC

PRs #137 and #138 were squash-integrated into the feature branch first, without
main deployments. PR #136 was then squash-merged as
**6e7293e75bf0577c1ce9105744249c718fe65361** after final head CI passed and the
owner-confirmed 000017 production compatibility gate. The merged main tree was
compared with the reviewed integrated tree and matched exactly.

[Main CI/deployment run 42455187085](https://github.com/kola-white/rsa-attestation-engine/actions/runs/42455187085)
passed, including Go/PostgreSQL/Node tests, release build/vulnerability checks and
exact-revision API deployment. Publish run 42455186874 also passed. Public health
returned 200, `/version` reported 6e7293e, and JWKS retained the single expected
issuer-prod-2026-10 RS256 key. All four new evidence routes returned 401 without
credentials. These probes changed no application records or grants.

The existing Expo Production deployment for this same commit succeeded. Public
mvp.cvera.app assets contain real assignment discovery, completion retry, assigned
case selection and awaiting-review display. This establishes code delivery, not
authenticated picker/native-device acceptance. No new mobile credentials/build or
infrastructure were created. PR #131 remains untouched.

Two-account authenticated review acceptance remains pending the existing console
smoke result. No positive HTTP access/review result is inferred from anonymous
401s, local tests or successful deployment. No signing, credential change or
production destructive case deletion was performed by this release.
