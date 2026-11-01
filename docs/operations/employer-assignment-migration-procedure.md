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
