# Proposal A: migration and local integration evidence

**Proposal A's application enforcement, migration and local integration verification are implemented. Production role binding and deployment authorization remain OUTSTANDING/BLOCKED on the metadata review below.** Proposal B remains untouched and an unresolved **high-severity release blocker**. PR #130 stays draft. No production query, migration, grant, infrastructure change, data access, deployment or merge was executed by this work.

## Confirmed schema and verification limits

The user supplied the results of a separately performed PostgreSQL 16 schema-only inspection. These establish:

- `public.domain_users.id UUID PRIMARY KEY DEFAULT gen_random_uuid()`; `kratos_identity_id TEXT NOT NULL UNIQUE`; nullable `email`/`name`; non-null text role/status defaulting to `hr_reviewer`/`active`; non-null `created_at`/`updated_at` defaulting to `now()`.
- `public.evt_employer_config.employer_id TEXT PRIMARY KEY`; non-null text `dispatch_mode DEFAULT 'AUTO'`; non-null creation/update timestamps defaulting to `now()`.
- `public.evt_requests.request_id UUID PRIMARY KEY`; `candidate_personid UUID NOT NULL`; `employer_id TEXT NOT NULL REFERENCES public.evt_employer_config(employer_id)`; nullable `hr_assigned_personid UUID`; non-null `public.evt_request_status DEFAULT 'DRAFT'`. No additional FK relationships involving those three tables were reported.

[`testdata/proposal_a_legacy_schema.sql`](../../apps/evt-api-go/internal/httpapi/testdata/proposal_a_legacy_schema.sql) reproduces these supplied columns, defaults, nullability and key relationships locally. The production `public` namespace is mapped into a unique disposable schema for each test; this avoids touching shared public objects. Tests now use UUID request IDs and the confirmed employer FK, not the earlier text-ID fixtures. `hr_assigned_personid` is not treated as an authoritative employer grant: it is nullable, has no reported FK and no established access-control semantics.

**Scope of the evidence:** local compatibility with the supplied parent-schema subset, actual migration and protected runtime operations. This is not a complete production-schema clone or production verification. Complete workflow-column types, enum labels, index definitions and existing trigger bodies were not supplied. Test-only workflow columns/enum labels are explicitly marked scaffolding. The reported index names (`idx_domain_users_kratos_identity_id`, `idx_evt_requests_employer_status`, `idx_evt_requests_candidate`, `idx_evt_requests_status`) and update-trigger names are not fabricated into definitions. Migration 000014 does not alter those legacy columns, indexes or triggers.

## Migration 000014

[`000014_employer_hr_assignments.sql`](../../apps/evt-api-go/migrations/000014_employer_hr_assignments.sql) follows the repository's numbered, one-way SQL convention. It requires a trusted migration owner, the explicitly selected legacy schema, and a transaction. For the supplied production schema that target is `public`; isolated tests select their corresponding schema.

The migration:

- Preflights the confirmed UUID/text parent-column types and existence, failing rather than substituting a different key/model.
- Creates `employer_hr_assignments` with UUID assignment/principal identifiers, a text employer identifier, attributable grant/revocation fields and complete revocation metadata checks.
- Adds FKs to `domain_users(id)` and `evt_employer_config(employer_id)` with `ON UPDATE/DELETE RESTRICT`, preserving grant history when parents are referenced.
- Enforces one unrevoked grant per `(principal_id, employer_id)` through a partial unique index.
- Creates **no assignments** and revokes PUBLIC table privileges.
- Installs an invoker-security trigger with fixed `pg_catalog, <trusted schema>, pg_temp` search path and no PUBLIC function execution. New grants require a current active HR principal; grant identity/provenance cannot be edited, and revoked records cannot be reactivated or rewritten through ordinary UPDATE.
- Makes INSERT/revocation acquire the same exclusive principal-row lock used by operator tooling. This protects the protocol even for direct reviewed operator SQL. The trigger grants no extra privileges and does not disable/bypass legacy RLS.

Runtime checks in [`employeraccess.Authorize`](../../apps/evt-api-go/internal/employeraccess/assignments.go) retain the shared principal lock through the existing read-committed transaction and require the current active HR role plus an explicit unrevoked assignment. The three employer list/get/attest repository methods remain guarded before disclosure/signing. Existing JWT/role/workflow checks, generic denial responses and all earlier remediations are retained. The attestation compatibility alias delegates to the guarded method.

## Least-privilege role binding: reviewed template, not production configuration

[`operations/hr-assignment-privileges.sql`](../../apps/evt-api-go/operations/hr-assignment-privileges.sql) is a **separate reviewed binding template**, not an automatically applied migration. It requires two distinct existing role names supplied explicitly through transaction-local `cvera.hr_runtime_role` and `cvera.hr_operator_role` settings. There are no default production identities, role creation, memberships, passwords or deployment hooks.

| Identity | Narrow binding added by the template |
| --- | --- |
| Runtime | Trusted-schema USAGE; assignment SELECT; principal `id/role/status` SELECT and `UPDATE(email)` for PostgreSQL row-lock permission. No assignment writes, deletion, truncation, trigger administration or role/status writes. Other application permissions are not invented/reprovisioned here. |
| Operator | Same principal lookup/lock access; employer-ID SELECT; assignment SELECT; INSERT of grant-instance/provenance columns; UPDATE of only the three revocation columns. No request-record reads, principal role/status updates, assignment deletion or provenance reassignment. |
| Migration owner | Trusted owner that creates schema objects and applies reviewed migrations. Neither runtime nor operator may own or assume ownership of these objects. |

PostgreSQL's direct row-locking path requires UPDATE permission on at least one principal column; the fixture binding uses `email`, already updated by exchange. **This permission also permits blind email updates; it is not a zero-profile-write operator capability.** Its suitability must be reviewed before production binding. An alternative narrowly privileged locking routine would require a separate explicit design decision; none was silently introduced. The operator cannot read email/name or update role/status under the tested binding.

The template checks actual effective local permissions and refuses unsafe ownership, superuser/CREATEROLE/CREATEDB/BYPASSRLS capabilities, schema/database CREATE, runtime/operator cross-membership, unreviewed RLS, unexpected assignment ACLs/default grants, privilege delegation, unrelated operator record reads, or user role/status write privileges. It narrows direct assignment ACLs and verifies the resulting boundary; it does not silently revoke/rewrite unrelated legacy application rights. Existing RLS requires separate policy review instead of a bypass switch.

[`cmd/hr-assignment`](../../apps/evt-api-go/cmd/hr-assignment/main.go) remains the operator-only grant/revoke utility. It requires its separately supplied administrative DSN, explicit human actor and approval basis/revocation reason. It creates no principals/employers and infers no permissions from role, email, candidate requests or demo IDs. Granting still requires organizational approval evidence independently of an employer configuration row. Revocation works after user disablement; regrant creates a fresh instance. No public administration API exists.

## Missing production metadata and read-only inspection

The repository's migration files establish numbering and authority-persistence SQL. The authority test helper applies 000001–000013 transactionally in an authority-only schema; the new application migration has separate integration tests because it requires the legacy parents. `schema_migrations` defines bookkeeping, but the repository does **not** establish the production migration runner or version-recording mechanism. `scripts/deploy-api.sh` builds/restarts/checks the API; it does not apply migrations or bind these roles. Existing deployment guidance requires a trusted migration owner and a separate non-owner runtime identity but does not identify actual production identities/grants.

Use the separately reviewed [metadata-only catalog script](../../apps/evt-api-go/operations/hr-assignment-metadata.sql) in an already authenticated PostgreSQL 16 session:

```text
\i apps/evt-api-go/operations/hr-assignment-metadata.sql
```

**Do not treat this as execution authorization. The user will review and run it separately; it was not run against production.** It uses a read-only repeatable-read transaction, a local 15-second statement timeout, catalog-only queries and rollback. Expected output:

1. Database/schema/table owners and RLS enabled/forced flags for the confirmed tables and any assignment/bookkeeping tables present.
2. Custom role names and login/superuser/creation/inheritance/BYPASSRLS flags, without passwords or role configuration values.
3. Role memberships, including PostgreSQL 16 inheritance/SET/admin options and predefined-role inheritance.
4. Direct database/schema/table/column ACLs and global/public table default ACLs, with grantors, grantees and grant-option flags. Interpret them with memberships; direct ACLs alone are not effective privileges.
5. RLS policy names, commands, roles, permissiveness and expression-presence flags. Expressions are intentionally excluded to avoid exporting literal values. If enabled RLS is present, obtain a separately approved policy-definition review before binding.

No application records, `pg_authid`, password fields, connection strings, role configuration values, private keys or active SQL statements are queried. Syntax was verified against the disposable local instance only. Catalogs cannot identify which listed identity the API actually uses or which external migration runner is authoritative. Supply the API/runtime identity, proposed operator/migration identities, runner/tool/version, transactional/version-recording convention and target schema/search_path separately, without credentials. If policy logic or omitted workflow DDL affects integration, review its schema-only definition separately.

**Unresolved:** production object ownership, actual identities/memberships/effective/default privileges, RLS/policies, approved lock-column permission, migration runner and deployment authorization. No production role binding is finalized until those are established. Parent hard-deletion behavior must account for the new restrictive FKs. No grant backfill, data deletion, automatic role provisioning or deployment is authorized by local test success.

## Verification evidence

Working directory: `apps/evt-api-go`. Local PostgreSQL 16 is the existing disposable `cvera_authority_test` database; tests validate the loopback host/database/user and isolate/drop their schemas and probe roles. Core tests load the confirmed schema subset, apply **the actual migration file** and exercise **the actual binding file**, rather than mirroring new objects/permissions in test code.

Tests in [`employer_migration_integration_test.go`](../../apps/evt-api-go/internal/httpapi/employer_migration_integration_test.go) verify parent/assignment types and nullability, supplied defaults, no implicit grants, restricting FKs, trigger security, migration commit/rollback/type-mismatch preflight, approved operator grant/revoke, metadata constraints, provenance/revoked-state immutability, denied reactivation/deletion/parent deletion, and unsafe privilege-binding rejection or safe narrowing. The catalog script is syntax-checked locally.

Existing [authorization tests](../../apps/evt-api-go/internal/httpapi/employer_authorization_integration_test.go) now run on that migrated schema: all three cross-employer denials, role-only/disabled/stale-role denial, request/employer substitution, no signing/state changes after denial, explicit multi-employer grants, revocation/regrant history, and both concurrency orders. One concurrency branch deliberately uses direct SQL so the **database trigger's lock protocol** is tested independently of the CLI helper. Restricted runtime permission tests now apply the real binding template. `TestEmployerAssignedLocalSigning` retains temporary RSA-3072 signing/storage/JWKS verification and all negative cryptographic checks.

Final checks, all with the local database enabled:

| Command | Result |
| --- | --- |
| `go test ./... -count=1 -timeout=90s` | PASS, including existing authority PostgreSQL tests and migrated employer tests. |
| `go test -race ./internal/httpapi ./internal/auth ./internal/employeraccess -count=1 -timeout=90s` | PASS. |
| `go vet ./...` | PASS. |
| `git diff --check` | PASS. |

An intermediate negative test expected rejection of a direct column grant that PostgreSQL's REVOKE safely removed. Its failed assertion exposed missing rollback on that assertion path and blocked local cleanup. The assertion now verifies the resulting permission denial and always rolls back; the two affected local runs were stopped, only their identified disposable schemas cleaned, and the final suites rerun successfully. No production operation or authorization weakening was involved.

Proposal B's known-gap reproduction remains intentionally present and is **not a security acceptance test**. **GO-2026-6611 (outbound HTTP/2 CPU exhaustion) and GO-2026-6809 (x509 certificate distrust) remain OUTSTANDING until release-build verification.** No fresh audit/dependency upgrade or unrelated CI change was performed. Prior credential-exposure, legacy normalization/status and live production-key verification limits also remain.
