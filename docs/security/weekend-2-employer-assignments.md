# Proposal A: locking, privileges, and local verification

**Proposal A is implemented and locally verified; production execution remains unauthorized.** Draft PR #130 must remain draft. Proposal B is untouched and remains an unresolved **high-severity release blocker**. No production SQL, migrations, grants, deployment, infrastructure changes or merge were executed by this work.

## Authoritative evidence supplied by the operator

The separately completed, rolled-back production inspections establish database `evt`, PostgreSQL **16.11**; database/existing table owner `postgres`; `public` schema owner `pg_database_owner`; and application role **`evt-app`**, LOGIN, non-superuser, without CREATEDB/CREATEROLE/BYPASSRLS or memberships. `postgres` directly grants it SELECT/INSERT/UPDATE/DELETE on `domain_users`, `evt_employer_config`, and `evt_requests`, without grant options. RLS is disabled on those tables. `public.employer_hr_assignments` and `public.schema_migrations` do not exist. These are operator-supplied observations, not queries independently executed by this assistant.

The earlier supplied parent definitions establish UUID application principal `domain_users.id`, unique non-null text Kratos identity linkage, text role/status and supplied defaults/timestamps; text employer primary key; UUID request primary key/candidate reference; employer FK; nullable UUID `hr_assigned_personid`; and request-status enum/default. The nullable HR field is not substituted for the explicitly approved employer grant model.

[`testdata/proposal_a_legacy_schema.sql`](../../apps/evt-api-go/internal/httpapi/testdata/proposal_a_legacy_schema.sql) reproduces the supplied parent subset in isolated local schemas. UUID request IDs, nullability/defaults and the employer FK are exercised. Remaining workflow columns/enum labels are explicitly labelled test scaffolding. Complete index/trigger definitions were not supplied and are not fabricated. This is local integration with the supplied subset, not a complete production clone, connection/authentication test of `evt-app`, or production verification.

## Justified locking change: no profile UPDATE privilege needed

The previous direct `FOR SHARE`/`FOR UPDATE` calls required an UPDATE privilege on some principal column; the initial binding used `UPDATE(email)`. That added an unnecessary profile-write capability to a minimal authorization/operator identity. The binding also rejected runtime role/status writes, conflicting with the now-confirmed legacy `evt-app` grants.

Migration [`000014_employer_hr_assignments.sql`](../../apps/evt-api-go/migrations/000014_employer_hr_assignments.sql) now provides two **bounded SECURITY DEFINER** routines owned by the trusted migration owner:

| Routine | Capability and intended caller |
| --- | --- |
| `check_employer_hr_assignment(UUID,TEXT) → BOOLEAN` | Runtime-only authorization check. Takes the shared principal-row lock, requires current active HR status, and checks the exact unrevoked employer assignment. Returns only a boolean; cannot modify profiles or grants. |
| `lock_employer_hr_principal(UUID) → (id,status,role)` | Operator-only exclusive principal-row lock. Returns only the selected principal's ID/status/role, permitting reviewed grant/revoke operations and the invoker trigger to coordinate without direct principal UPDATE/SELECT grants. It does not modify records. |

These routines use typed parameters, fixed SQL, fixed `pg_catalog, <trusted schema>, pg_temp` search paths, and no caller-supplied SQL or dynamic execution in their bodies. EXECUTE is owner-only until reviewed binding; PUBLIC and other default execution grants are removed. They require READ COMMITTED and reject unreviewed RLS on principals/assignments rather than silently applying owner-bypass semantics. PostgreSQL's [SECURITY DEFINER guidance](https://www.postgresql.org/docs/16/sql-createfunction.html) explains the fixed trusted search path and selective EXECUTE requirements. The trusted schema's CREATE ACL still requires the review below.

[`employeraccess.Authorize/Grant/Revoke`](../../apps/evt-api-go/internal/employeraccess/assignments.go) now use these functions. All employer list/get/attest repository guards, JWT/role gates, exact request/employer binding, current-user/workflow checks and generic denials remain. The shared lock is held through the protected transaction; operator grant/revoke takes its exclusive counterpart. Revocation waiting for an authorized transaction, authorization waiting behind revocation, and direct operator-SQL coordination remain tested. No profile UPDATE privilege is used to acquire those locks.

## Migration 000014: owner-only creation and explicit grants

The unmerged/unapplied migration retains the verified UUID/text FK types, restrictive parent referential actions, one-active-grant partial index, complete attribution/revocation constraints, immutable grant provenance and immutable revoked records. The invoker trigger uses the operator lock routine and validates active HR status for new grants. It creates **no assignments**, roles, memberships, authority records or case permissions.

All DDL and ACL normalization are enclosed in **one outer DO statement**. New objects are atomic even with an autocommit client; this does not invent a migration runner or version ledger. Creation clears non-owner privileges on the new assignment table and all three new routines, including custom ALTER DEFAULT PRIVILEGES grants, before they become visible. No legacy parent ACL/default-privilege policy is modified. Production remains unchanged; editing 000014 here does not rewrite an already-applied migration.

## Least-privilege binding and verified legacy DML

[`operations/hr-assignment-privileges.sql`](../../apps/evt-api-go/operations/hr-assignment-privileges.sql) remains a separately reviewed, atomic binding template. It requires explicit existing runtime/operator names through `cvera.hr_runtime_role` / `cvera.hr_operator_role`, with the verified schema selected. The known runtime identity is `evt-app`; **no production operator identity is assumed or created**.

| Identity | New assignment-boundary access |
| --- | --- |
| Runtime | Schema USAGE and EXECUTE on the boolean check only. **No raw assignment SELECT, INSERT, UPDATE, DELETE, TRUNCATE, REFERENCES, TRIGGER, operator-lock execution or ownership.** No principal/profile privileges are added. This is a read/check capability using transaction locks, not a database READ ONLY transaction. |
| Operator | Schema USAGE; EXECUTE on the exclusive lock; employer-ID SELECT; assignment SELECT; INSERT of grant/provenance fields; UPDATE of revocation fields only. **No principal/profile writes or email/name reads**, request-record reads, grant reassignment/deletion/reactivation, or object ownership. |
| Trusted migration owner / DBA | Owns the new table/routines and applies separately approved DDL. It remains a trusted administrative authority; runtime/operator cannot own or assume its roles. |

The binding no longer rejects `evt-app` solely because its **existing** parent tables permit role/status or other legacy DML. It neither adds nor removes those grants. A dedicated operator still must not inherit unrelated profile/data privileges. Effective new-table/function permissions, ownership, memberships, schema/database CREATE, unexpected ACLs/delegation and RLS are checked. No PUBLIC permission or blanket application grant is introduced.

**Known residual privilege exposure:** `evt-app` is not an application-wide read-only/least-privilege role: its reported broad DML on the three legacy tables remains. This work does not claim that arbitrary SQL executed with that identity cannot mutate legacy records. Narrowing those existing grants requires a separately reviewed inventory of other application workflows. The new assignment boundary is independently non-writable by runtime credentials. Tests prove both minimal zero-profile-write authorization and compatibility with the supplied broad legacy DML profile.

[`cmd/hr-assignment`](../../apps/evt-api-go/cmd/hr-assignment/main.go) remains operator-only: separate administrative DSN, explicit actor/approval basis/revocation reason, no new principals/employers, no inferred assignments. Organizational approval is required independently of configuration-row existence. Revocation after account disablement and fresh-instance regrant remain supported. There is no public grant-management endpoint.

## Exact repository migration mechanism: test-only, no production runner identified

The targeted search covered tracked SQL files, Go startup/DB code, commands/dependencies, Docker/Compose startup, package scripts, deployment scripts and GitHub workflows. Evidence:

| Repository location | Actual behavior |
| --- | --- |
| [`openIsolatedAuthoritySnapshotPool`](../../apps/evt-api-go/internal/db/authority_repository_integration_test.go) | **Test-only**: `filepath.Glob`, lexicographic sort/sequence validation for authority migrations 000001–000013, isolated schema, `pgx.Begin`, `SET LOCAL search_path`, `os.ReadFile`, `tx.Exec` per file, then `Commit`. No production ledger is maintained by this helper. |
| [`securityTestServer`](../../apps/evt-api-go/internal/httpapi/security_integration_test.go) | **Test-only**: read local parent-fixture SQL and 000014, `database.Begin` / `Exec` / `Commit` in disposable schemas. New migration tests also execute the actual binding template. |
| [`000001_schema_migrations.sql`](../../apps/evt-api-go/migrations/000001_schema_migrations.sql) | Defines a version table; it is not an executable runner. No runtime version-recording mechanism was found. |
| [`cmd/server/main.go`](../../apps/evt-api-go/cmd/server/main.go), [`NewRouter`](../../apps/evt-api-go/internal/httpapi/router.go) | Configuration, DB initialization and HTTP startup; no migration-file execution. |
| [`Dockerfile`](../../apps/evt-api-go/Dockerfile), [`docker-compose.yml`](../../apps/evt-api-go/docker-compose.yml) | Final API image contains the server binary and starts it directly. No application migration/init-SQL service or hook is configured. |
| [`scripts/deploy-api.sh`](../../scripts/deploy-api.sh), [CI/deploy workflow](../../.github/workflows/main.yml), [`package.json`](../../package.json) | Build/recreate/health/revision checks, tests and artifact work; no production SQL migration command or ledger update. |

**Conclusion:** no exact **production** migration runner can be identified from repository evidence. The only executable SQL-file mechanisms found are the tests above. The missing production `public.schema_migrations` is consistent with this, but does not prove that no external/manual process or other-schema ledger exists. Do not label the test helper, numbered filenames or deployment script a production runner. Do not bootstrap migrations 000001–000014 or create a new production ledger based on that assumption. No runner/CI infrastructure was added in this task.

## Remaining production facts and decisions

Already resolved by the supplied inspection: database/parent owners, `evt-app` identity/flags/memberships, its explicit parent DML, disabled parent RLS, and absent public assignment/version tables. Do not repeat those checks as though unknown.

Still required before binding or any separately authorized execution:

1. The dedicated operator identity and approved organizational-assignment provisioning/revocation responsibility. Confirm its effective rights/memberships and that runtime cannot assume it.
2. Trusted `public` schema CREATE ACLs, database CREATE rights, relevant default ACLs and confirmed application schema/search_path configuration (without exporting DSNs). New-object defaults are neutralized, but the privileged routines must live in a schema not writable by untrusted identities.
3. The actual external/manual migration process, if any: tool/entrypoint/version, approved migration identity, intended new-object owner, target schema, transactional/version-recording convention, rollback/backup and rollout responsibility. If no process exists, that requires an explicit operator decision; this task did not invent one.
4. Full omitted workflow/enum/index/trigger definitions only where needed to claim complete production-schema parity; current tests deliberately claim the supplied subset. Parent hard-deletion behavior must account for the new restrictive grant FKs.

The existing [read-only metadata script](../../apps/evt-api-go/operations/hr-assignment-metadata.sql) contains database/schema/default ACL outputs and membership metadata useful for the remaining review. Do not re-run completed checks unnecessarily. It does not read application records, passwords, role configuration values, connection strings, private keys or active SQL; RLS expressions are excluded. It cannot establish an external migration command. No production queries are executed by this work.

## Verification evidence

All commands run from `apps/evt-api-go` against the existing guarded disposable PostgreSQL 16 database, with isolated schemas/probe roles and cleanup. This does not claim exact production 16.11 image/platform parity.

Added/preserved regressions cover:

- All three employer endpoint denials, current status/role enforcement, allowed temporary-key signing and request-state checks.
- Revocation ordering in both directions, including direct operator UPDATE through the trigger.
- Successful runtime authorization with **no profile SELECT/UPDATE or raw assignment access**; denied runtime grant/revoke/table writes/exclusive-lock execution.
- Successful binding with the verified legacy SELECT/INSERT/UPDATE/DELETE profile, unchanged parent grants, and denied new assignment writes.
- Operator grant/revoke without principal update privileges; immutable provenance/history and restrictive FKs.
- Fixed function ownership/security/search paths, temporary-table shadowing denial, unsupported isolation/RLS failure, and removal of PUBLIC/custom creation-default execution/write privileges.
- Migration rollback/preflight, atomic failure without an external transaction wrapper, and owner-only object creation, plus preserved authority/auth/SSRF/body-limit/signing regressions. Proposal B's intentionally passing gap reproduction still denotes an open blocker.

| Command | Final result |
| --- | --- |
| `go test ./... -count=1 -timeout=90s` | PASS with local database enabled. |
| `go test -race ./internal/httpapi ./internal/auth ./internal/employeraccess -count=1 -timeout=90s` | PASS. |
| `go vet ./...` | PASS. |
| `git diff --check` | PASS. |

See [migration/security tests](../../apps/evt-api-go/internal/httpapi/employer_migration_integration_test.go) and [endpoint/concurrency tests](../../apps/evt-api-go/internal/httpapi/employer_authorization_integration_test.go). No new dependency, fresh security audit, deployment hook or unrelated refactoring was introduced. **GO-2026-6611 and GO-2026-6809 remain OUTSTANDING until release-build verification.** Prior credential exposure, legacy normalization/status and live signing-verification limitations remain.
