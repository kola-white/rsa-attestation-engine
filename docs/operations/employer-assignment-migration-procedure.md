# Proposal A: proposed operator procedure (not execution approval)

Applies only to draft PR #130, migration 000014 and its separate privilege binding. No production migration, rehearsal, grant, revocation or deployment is authorized by this document. Proposal B remains untouched and a high-severity release blocker.

## Required human decisions

- Approve the exact reviewed commit and file checksums, change window, backup/rollback and version-recording responsibility. No production migration runner is identified in this repository; the SQL loaders are tests only. Do not bootstrap 000001–000014 or fabricate a `schema_migrations` entry. If an external process exists, its transaction and recording behavior must be reviewed first.
- Approve a DBA executor able to act as the verified existing owner `postgres` in database `evt`. That session creates and retains ownership of the new table and routines. Do not execute DDL as `evt-app` or the assignment operator; do not transfer SECURITY DEFINER ownership to them. This specifies the required capability, **not an approved human identity**.
- Designate and review an existing, dedicated assignment operator role and the humans authorized to use it. No identity is selected/created here. It must not own/inherit the objects, assume the runtime/owner role, create schema/database objects, bypass RLS, or possess unrelated profile/request privileges rejected by the binding. Approve which organizational authority establishes/revokes each HR-to-employer relationship and where its approval reference is retained.
- Confirm other public-schema creators are trusted. The supplied metadata already establishes that `evt-app` lacks CREATE and memberships; TEMP is allowed and is handled by qualified references/fixed function paths.
- Verify the actual API pool and separate operator CLI connection schema lookup through an approved operational method. Admin-session defaults alone are insufficient. Do not export DSNs or add a public diagnostic endpoint.

## Reviewable SQL sequence

The following is a **future rollback rehearsal**, only after separate approval. Use a fresh trusted psql connection to `evt`, authenticated privately as the approved DBA executor acting as `postgres`. Start psql in the reviewed repository root, with startup files disabled (`psql -X`), without putting credentials in commands or logs. Confirm the commit/checksums out of band. Do not use an application connection pool.

```sql
\set ON_ERROR_STOP on
BEGIN;
SET LOCAL search_path = public, pg_temp;

-- Metadata only: require evt / postgres / reviewed PostgreSQL 16 build.
SELECT current_database(), current_user, current_setting('server_version');
-- Require all NULL. Stop on any existing/partial objects; do not drop or adopt.
SELECT to_regclass('public.employer_hr_assignments'),
       to_regprocedure('public.check_employer_hr_assignment(uuid,text)'),
       to_regprocedure('public.lock_employer_hr_principal(uuid)'),
       to_regprocedure('public.enforce_employer_hr_assignment()');

SET LOCAL cvera.hr_runtime_role = 'evt-app';
-- Replace only after role/authority approval; do not execute the placeholder.
SET LOCAL cvera.hr_operator_role = '<approved-existing-operator-role>';
\i apps/evt-api-go/migrations/000014_employer_hr_assignments.sql
\i apps/evt-api-go/operations/hr-assignment-privileges.sql

-- Metadata only. Require table owner postgres, no RLS, runtime CRUD all false.
SELECT pg_get_userbyid(relowner) AS owner, relrowsecurity, relforcerowsecurity,
       has_any_column_privilege('evt-app',oid,'SELECT') AS runtime_select,
       has_any_column_privilege('evt-app',oid,'INSERT') AS runtime_insert,
       has_any_column_privilege('evt-app',oid,'UPDATE') AS runtime_update,
       has_table_privilege('evt-app',oid,'DELETE') AS runtime_delete
FROM pg_catalog.pg_class
WHERE oid='public.employer_hr_assignments'::regclass;

-- Require postgres ownership throughout; check+lock definer=true; guard=false.
-- Require fixed pg_catalog, public, pg_temp path; runtime_execute true ONLY
-- for check_employer_hr_assignment. Inspect ACLs: no PUBLIC EXECUTE entries.
SELECT p.oid::regprocedure, pg_get_userbyid(p.proowner) AS owner,
       p.prosecdef, p.provolatile, p.proconfig, p.proacl,
       has_function_privilege('evt-app',p.oid,'EXECUTE') AS runtime_execute
FROM pg_catalog.pg_proc p
WHERE p.oid IN ('public.check_employer_hr_assignment(uuid,text)'::regprocedure,
               'public.lock_employer_hr_principal(uuid)'::regprocedure,
               'public.enforce_employer_hr_assignment()'::regprocedure);
ROLLBACK;
```

The operator must stop on unexpected metadata even if SQL itself succeeds. The binding refuses unsafe effective permissions. Creation strips all non-owner defaults on the **new objects** atomically; `postgres`'s global default TABLE CRUD and ordinary PUBLIC function defaults are not altered. Legacy `evt-app` parent CRUD remains unchanged.

After a successful approved rehearsal, a **separate approved execution plan** may use the same transaction with COMMIT only after all checks and the agreed version record are satisfied. Nothing here chooses or installs a migration runner. 000014 is intentionally once-only: re-execution fails atomically; it does not reconcile partial/manual objects. The binding can be repeated after review. If any step fails, roll back; do not resume at a later statement. A failed psql session must not leave an open transaction. After real grants exist, rollback is not a casual table drop: preserve history and coordinate application rollback to avoid restoring the old authorization defect.

## Runtime lookup and assignment administration

Before rollout, inspect metadata on the **actual** API and separate CLI connection paths using the separately approved operational method:

```sql
SELECT current_setting('search_path'), current_schemas(true),
       to_regprocedure('check_employer_hr_assignment(uuid,text)') AS resolved_check,
       to_regprocedure('check_employer_hr_assignment(uuid,text)') =
         to_regprocedure('public.check_employer_hr_assignment(uuid,text)') AS expected_check,
       to_regprocedure('lock_employer_hr_principal(uuid)') =
         to_regprocedure('public.lock_employer_hr_principal(uuid)') AS expected_lock;
```

Expect resolution to the reviewed public routines and no untrusted preceding persistent schema. After rollback rehearsal these functions will be absent; this check belongs after an approved committed migration or in an equivalent non-production environment. A psql session's SET is not inherited by the API or CLI's new connection. No production lookup is executed by this work.

Use the reviewed `cmd/hr-assignment` utility only with a privately supplied `CVERA_HR_ADMIN_DATABASE_URL` for the approved dedicated operator, never runtime credentials. From `apps/evt-api-go`, the command forms are:

```text
go run ./cmd/hr-assignment -action grant -principal <domain_users.id UUID> -employer <exact employer_id> -actor <approved attributable operator> -basis <organizational approval reference>
go run ./cmd/hr-assignment -action revoke -assignment <exact assignment UUID> -actor <approved attributable operator> -reason <reviewed revocation reason>
```

These are forms for review, not executable sample assignments. Retain the returned assignment UUID in the approved change record. The operator must validate the approval's canonical principal/employer independently; a role or configuration-row existence is not authority. No bulk/implicit grants are created. Existing reviewers have no employer access until explicitly assigned; existing response contracts remain generic denial. Current active HR status and existing request/workflow checks still apply. Revocation retains history and waits for already-authorized transactions; new authorization waits behind revocation and then denies. Revocation can proceed after account disablement. Regrant creates a fresh assignment UUID, never reactivates old history.

The operator role can submit approved provenance, so organizational attribution is a trusted administrative responsibility, not a cryptographic assertion about the `-actor` string. The DBA remains a trusted owner capable of overriding ACLs. Local tests do not establish production human identity, live connection configuration, a production version ledger, or complete omitted workflow DDL parity. Release acceptance also remains blocked by Proposal B and outstanding release-build verification of GO-2026-6611 / GO-2026-6809.

## Owner-authorized first-change design (2026-11-01 closeout)

The owner confirmed there is **no existing production migration runner or version-recording process** and authorized designing/validating one controlled procedure. This supersedes the earlier uncertainty about an external runner; it does not authorize the actual privileged production SQL. No prior authority migration is assumed applied. The responsible production operator is the owner; the restricted grant-administration database identity still requires explicit selection/creation approval.

**Compatibility gate:** the candidate API cannot deploy compatibly without 000014 and the runtime binding. All three employer operations call `check_employer_hr_assignment`; missing objects produce generic 500, as the regression demonstrates. `/healthz` plus `/version` alone cannot establish this prerequisite. Merging main automatically runs `scripts/deploy-api.sh`, so hold merge until the migration verification is recorded. Do not remove authorization or return implicit permission to make the old schema work. Do not run 000001–000013 or create their records.

### Controlled sequence for separate production approval

1. Freeze the reviewed PR SHA and SHA-256 digests of 000014 and the binding SQL in a private operator-controlled change record. Record database `evt`, PostgreSQL version, current production application SHA and image ID, responsible owner, intended role binding, recovery target, and the pre-change absence of the four new objects. Checkums and metadata are safe to report; no DSN, tokens or key contents belong in the record.
2. Through an approved authenticated DBA session as `postgres`, take a consistent full `evt` custom-format backup with the PostgreSQL 16 `pg_dump` client, preserving schema, data, ownership and ACLs. Use peer/authentication already configured, never a password in arguments. Use `umask 077`, a private operator-controlled directory on protected storage, no public artifact bucket, and record backup SHA-256, size and timestamp. Do not export private PEM files. The database backup itself contains sensitive records and must not be printed or copied into this repository/CI artifacts. Separately preserve role definitions **without passwords** (`pg_dumpall --roles-only --no-role-passwords`) and document recovery of authentication through the existing secure mechanism.
3. Prove backup restoration into an approved isolated PostgreSQL 16 recovery target with the necessary role definitions, using `pg_restore --exit-on-error`; verify metadata/ACLs there. A backup listing or successful dump alone is not a restore test. Do not overwrite production to test recovery. Retain the pre-change API image by immutable ID and record its exact Compose configuration privately; rebuilding a floating tag is not an exact rollback.
4. Approve an existing dedicated unprivileged assignment operator, or separately approve its creation as a non-owner role. Keep it distinct from `evt-app`; do not grant memberships or choose an identity implicitly. The existing binding validates the effective permission boundary. `postgres` remains DDL/function owner. Role creation, if needed, belongs explicitly in the approved change plan, not hidden in migration 000014.
5. Rehearse the SQL sequence above with ROLLBACK in a fresh `postgres` session. Add `SET LOCAL lock_timeout = '5s'; SET LOCAL statement_timeout = '60s'; SET LOCAL idle_in_transaction_session_timeout = '60s';` and an approved change-window serialization lock (`SELECT pg_advisory_xact_lock(hashtext('cvera:employer-assignments:000014'));`). Stop on timeout or unexpected objects, rollback, and diagnose; do not kill application transactions or bypass checks. The old API can continue to run during this additive change, but no claim is made that old code enforces the new assignment policy.
6. After explicit approval for the actual change, repeat from the same reviewed files in one transaction, verify owners/fixed function configuration/ACLs and parent prerequisites, then COMMIT instead of ROLLBACK. No grants are seeded automatically. Verify afterward using fresh runtime and operator connection paths, including expected public function resolution, runtime direct-table/admin denial and operator restrictions. If no assignments have been organizationally approved, all employer access intentionally denies; approval is needed before accepting availability for real HR workflows.
7. Record **only the executed 000014** in the private change record: database, trusted executor, SQL digests, commit SHA, start/end UTC, transaction ID (`pg_current_xact_id()` captured inside the transaction), result, post-change catalog verification and backup/restore evidence. Confirm the committed objects from a new connection before recording success. Pre-record intent before execution so a disconnected COMMIT can be reconciled by catalog/provenance inspection; never automatically retry an ambiguous result. Explicitly record earlier migration history as unknown, not fabricated. This initial process uses an external operator-controlled record; `public.schema_migrations` remains absent. Introducing a future database ledger requires its own owner/default-ACL design and does not backfill invented versions.
8. Mark the PR ready for review after code verification and final CI pass; merge only after compatibility and an executable recovery plan are established. Then merge using the repository's squash convention, and let the existing exact-SHA workflow deploy. Preserve existing public ports/routes; this PR adds authentication to already public evidence paths and does not broaden their exposure. API-only recreation does **not** apply the Kratos service port change to an existing container; any Kratos restart must be included explicitly in the separately verified deployment plan.
9. Post-deployment verify container status/image revision, health/version, DB/authentication/session behavior with approved synthetic identities, denial of foreign/unassigned employer access and current assignment behavior, public JWKS continuity, and logs without copying credentials/PII. Do not issue production attestations or operate on real requests as a smoke test. Public key continuity and local temporary-key signing do not prove fresh production issuance.

### Recovery boundaries

Before COMMIT, rollback leaves no new objects/ACLs; a rehearsal failure never authorizes continuation at a later statement. After COMMIT, prefer leaving the additive schema/history intact while reverting only the application to the retained compatible image through an explicitly approved recovery action. The current deployment script deliberately accepts only origin/main and has **no automatic rollback**; do not pretend passing an older SHA to it works. An application revert must account for restoring the old cross-employer authorization defect. If the migration itself is faulty, stop rollout and use the prevalidated recovery plan rather than dropping grants/history under live traffic. Database restore into production requires a separate destructive-recovery decision, a maintenance window and reconciliation of writes after the backup. The isolated restore rehearsal is not authorization for that operation.

### Local rehearsal evidence and limits

Reproduce with `python3 apps/evt-api-go/operations/test-employer-recovery.py` from the repository root (requires only the existing local Docker test container). The closeout rehearsal used only the existing guarded local PostgreSQL container and fresh disposable schemas/roles/database. It created the supplied parent-schema subset and the verified legacy/default CRUD profile, took a custom-format dump (mode 0600), restored it into a fresh database, verified restored synthetic request data, parent/ACL state and absence of the new table, applied actual 000014 plus actual binding with ROLLBACK (absence verified), then applied with COMMIT and verified runtime check EXECUTE with direct assignment SELECT/UPDATE denied. Disposable database/schema/roles were removed. Backup SHA-256: `40201cf5c19ba4d7e1c3a28fe0bfc219e09cfca40d8de09d394a9d0daf813f8a`. This proves the mechanics for synthetic fixtures, **not** restoration of a production backup, complete omitted workflow schema parity, production role provisioning or an executed production migration. Full authorization/concurrency/default-ACL regressions separately exercise the actual SQL files.

The actual production migration/binding, dedicated operator selection or creation, production backup/recovery target and production access are the remaining approval/access gate. The available local SSH key failed the non-mutating access probe; no database connection, production SQL or privileged operation was attempted. Do not grant this workstation arbitrary superuser access merely to bypass the gate; use the owner's approved execution channel.

## Live console prerequisites discovered during closeout

The established administration channel is the owner's existing root session in the DigitalOcean browser console on `cvera-api`. Repository: `/opt/cvera/rsa-attestation-engine`; DB container: `evt-api-go-db-1`; API container: `evt-api-go-api-1`; database: `evt`. No Codex SSH account is authorized and no new SSH access or credentials are to be created. Browser execution is unavailable in this agent session, so the owner is running one metadata checkpoint at a time. All database commands so far were READ ONLY and rolled back; no migration/grant/password/configuration modification has occurred.

The configured API uses **postgres superuser/owner**, not intended restricted `evt-app`. This must be corrected before accepting runtime assignment isolation. Both API connection variables contain an administrator connection; removing the unused DATABASE_URL alias from Compose is required so it cannot retain that credential after switching EVT_DB_DSN. The actual server entrypoint requires and consumes only EVT_DB_DSN. Do not weaken the role-binding checks to accept postgres as runtime.

`evt-app` already has CRUD on the eight legacy table dependencies with RLS off. Its two serial sequence USAGE privileges are absent (evt_request_events_event_id_seq and evt_token_consumptions_consumption_id_seq); verify the default expressions and UPDATE alternative before adding only the required nextval capability. The four authority tables/allocator are absent: preserve that observed state, record incomplete authority persistence separately, and never pretend 000001–000013 ran.

Before any password reset, identify the live Kratos and Courier DSN roles so shared backend credentials can be coordinated. These are service database identities, not registered users. Do not read or alter registered Kratos identity records, drop/recreate authentication databases, or remove Docker volumes. Existing identities and authentication functionality must be preserved. Current API passwords do not match the known exposed fallback in the specific private comparison, but an evt-app credential has not yet been established for the API. Reuse only a verified secure existing credential, or obtain the separate decision for a controlled database password change; never request password values in chat.

The existing app `.env` is UID 501/mode 0644. Stage backup/new connection settings privately with mode 0600, preserve or explicitly verify the deployment identity's read access, and report only metadata/check results. Do not publish backups, environment values or password verifiers. The owner remains the responsible operator; `cvera_hr_operator` is only a proposed non-login grant-administration role, currently absent. Creating it and running 000014 require the actual privileged execution decision after backup/rehearsal readiness.

## Revised service-role plan after complete ownership inspection

Do not use evt-app as the API runtime: it owns evt_kratos and its entire identity schema. Retain existing ownership/grants, use fresh non-owner cvera_api_runtime and cvera_kratos_runtime roles with the reviewed per-service grant files, and bind Proposal A runtime EXECUTE to cvera_api_runtime. The proposed cvera_hr_operator remains a separate non-login administrative role. These names were confirmed absent; no existing role is silently adopted. Actual creation/credential activation still requires the controlled execution decision after compatibility/recovery verification.

No postgres password, Kratos cryptographic secret, identity row, user password or session is reset. Prepare independent service connection credentials privately, remove the API administrator alias, stage EVT_DB_DSN/KRATOS_DB_DSN in protected configuration, then validate both service identities before decommissioning administrator connections. Preserve the old environment and image as recovery evidence. Existing evt-app credentials are historical and potentially exposed; after confirming no consumers/connections, disabling LOGIN while preserving that role's ownership/grants is a separately recorded controlled action, never an identity-account operation.

The owner has created and checksum-verified root-only backups of evt/evt_kratos, password-free role definitions and the original environment, and retained the old API image under the recovery tag recorded in the closeout report. The production dump restore is being verified in a distinct network-none, memory-capped PostgreSQL container. No privileged changes have yet been applied to the live databases. A source-schema restore/rehearsal is not permission to run a destructive restore against production.

### Isolated restored-copy authorization checkpoint

After clone migration/binding, run `apps/evt-api-go/operations/test-restored-employer-authorization.py` on the Droplet using Python 3. Stage it from the exact reviewed Git commit into the private recovery directory without checking out candidate application code. The script refuses any target other than `cvera-weekend2-restore-20261102T004322Z` with network none, the rehearsal label and storage distinct from `evt-api-go-db-1`. It invokes local container psql as the clone owner and selects the NOLOGIN runtime/operator roles inside transactions. No password creation, live database connection, identity-store mutation, migration-history write or product endpoint call is performed.

Successful output must report all authorization/revocation/concurrency checks true and process exit zero (including cleanup). On failure, stop before any production role/migration/configuration action; diagnose the isolated clone only. Owner authority for the eventual privileged production transaction/configuration change remains a separate checkpoint after this evidence and the concrete recovery sequence are reviewed.
