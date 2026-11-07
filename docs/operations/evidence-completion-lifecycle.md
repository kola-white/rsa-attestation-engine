# Evidence completion lifecycle

This feature continues MVP API development after the accepted Weekend 2 baseline.
It does not change the historical production smoke evidence or the explicit live
attestation deferral. Production changes have not been executed for this feature.

## Runtime contract

Both evidence-init routes persist a ten-minute session owned by `domain_users.id`
and its exact `(case_id, check_id)` in the operator-provisioned registry. Both
completion routes resolve that persisted scope and require the same active
principal, current unrevoked grant, OPEN case and PENDING/IN_REVIEW check. No cases,
checks or grants are provisioned automatically. The runtime still cannot administer
the registry. Existing init URL/header/key fields remain compatible; session/file
UUIDs are additive. Existing callers must start a new persisted session to complete
an upload issued before this feature.

Completion requires the exact file set and expected size/MIME. The server reads
at most 5 MiB per file, hashes the exact bytes, and writes a new private
`sealed-evidence/<random UUID>` object, never a presigned staging key. It records
RECEIVED evidence and a PENDING scan job atomically. A unique case/check/hash index
and transaction-scoped per-check advisory lock enforce deduplication and the
three-file reservation limit. Expired uncompleted reservations no longer count.
Retries within session expiry recheck authorization and return the accepted hashes
without rereading staging objects. Check scope locking precedes the existing
principal/case/check/grant shared locks; those locks remain held through storage
work and commit. Revocation/state changes either win first and deny completion or
wait for that acceptance transaction. Requests have a 45-second context deadline.
Temporary relations are ordered after the trusted application schema.

## Schema and privilege changes

`000016_evidence_completion.sql` creates only new `upload_sessions`,
`evidence_uploads` and `evidence_scan_jobs` tables. UUID principals and text composite
case/check keys reuse migration 000015. The migration is atomic and once-only;
reruns fail rather than silently accept a mismatched schema. It removes inherited
non-owner table privileges, including historical default grants to `evt-app`.

`operations/evidence-completion-privileges.sql` requires an explicit verified
restricted runtime role. It grants SELECT and the necessary INSERT/UPDATE columns
on lifecycle tables, plus INSERT(upload_id) on scan jobs. It grants no DELETE,
registry mutation, session-owner reassignment or scan-status UPDATE rights.
Registry operator privileges remain unchanged. No new roles or credentials are
required. No migration history for 000001–000013 is assumed.

## Deployment gate and recovery

Do not merge this application change into main before production compatibility is
established: the existing main workflow deploys automatically. Migration 000016
and its bindings must precede new API startup; init fails closed with 503 when
persistence is absent. The old API remains compatible with the additive schema.

For an authorized production release, use the existing operator console procedure:

1. Confirm current backup and restore/recovery material, current API revision,
   verified trusted `public` schema resolution, and the absence of the three new
   tables. Review the exact release-commit SQL and checksums.
2. As the verified table owner `postgres`, apply migration and binding together
   with ON_ERROR_STOP, READ COMMITTED, explicit trusted search_path, bounded
   lock/statement timeouts, and LOCAL `cvera.evidence_runtime_role` set to the
   verified `cvera_api_runtime`. Roll back on any error. Verify new table owners,
   ACLs/column ACLs, constraints, and unchanged registry/admin permissions before
   committing. Record this migration only with its actual commit/checksum; do not
   invent historical migration state.
3. Merge/deploy through the existing workflow after release authorization and CI.
   Smoke-test a specifically authorized fixture via init → PUT → complete, both
   route aliases, unauthorized denial, revocation and exact retry. No synthetic
   identity or grant is implicit in this document.
4. Roll application back to its retained prior image if necessary. Keep the
   additive schema and received evidence. Schema rollback is safe only before
   any lifecycle data exists; otherwise preserve data and investigate. A database
   restore is not an automatic rollback because it can discard later writes.

These are preparation instructions, not authorization to execute production SQL,
create evidence/grants, merge an incompatible build, or deploy.

## Remaining MVP work and storage limitations

- Previously issued staging PUT URLs remain usable until their ten-minute expiry.
  Revocation immediately prevents later completion, and accepted objects use keys
  outside the presigned staging namespace.
- No scanner worker is implemented. Pending scan jobs must not be interpreted as
  clean evidence or eligible for a future CLEAN-only issuance policy. No new claims
  or attestation behavior is introduced here.
- Retention deletion, evidence listing/download, staging cleanup and orphan cleanup
  remain future work. Failed storage/SQL operations can leave private, unreferenced
  sealed objects. An ambiguous commit must not trigger object deletion because
  acceptance may have committed. No automatic cleanup or retention SLA is claimed.
- MIME metadata is checked but is not proof of file content safety. Pending objects
  stay private and are not served by a new download endpoint.
- The storage credential must permit private reads/writes in the existing bucket,
  including the new sealed namespace. Real Spaces compatibility and migration
  privileges require release verification; local protocol tests are not production
  proof.

## Local verification

Verified on 2026-11-07 with Go 1.26.10 and the guarded local PostgreSQL fixture.
The local wrapper reads only the disposable container connection and supplies
`CVERA_AUTHORITY_TEST_DATABASE_URL`; no production connection is used.

From `apps/evt-api-go`:

```sh
python3 /private/tmp/cvera-proposal-b-test.py test ./... -count=1 -timeout=120s
python3 /private/tmp/cvera-proposal-b-test.py test -race ./internal/httpapi ./internal/storage -count=1 -timeout=120s
go vet ./...
```

All passed. Equivalent CI invocation is `go test ./... -count=1 -timeout=120s`
with the existing CI PostgreSQL service and explicit disposable test DSN.
`git diff --check` also passed. Logs are local verification artifacts, not committed
credentials or production evidence.

Regression coverage includes both route aliases, server-only sealed bytes despite
staging overwrite, idempotent completion, batch atomicity, canonical hashing,
size/MIME/hash mismatches, unavailable/oversized objects, expired sessions,
cross-user/session/path denial, inactive principals, revoked grants/closed cases,
revocation blocking until in-flight acceptance completes, concurrent quota
reservations, expired-reservation reuse, pending scan insertion, runtime column
privileges, unchanged operator restrictions, temporary-table shadowing, absent
persistence failing closed, migration rollback and rejection of migration reruns.
Storage tests exercise the real AWS client against a local HTTP fixture, including
both content-length and chunked responses and private sealed PUT metadata.

An initial full run caught an unescaped `%` in the migration's formatted SQL LIKE
pattern. It was corrected before the passing full/race runs. No production SQL
was executed. GitHub CI is tracked on the feature PR separately.
