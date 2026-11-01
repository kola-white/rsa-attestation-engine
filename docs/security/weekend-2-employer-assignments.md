# Proposal A implementation evidence and blocked database integration

Proposal A was explicitly approved. Application enforcement and operator tooling are implemented and tested against **explicit disposable fixtures** on the existing Weekend 2 branch. **Migration and integration against the actual legacy database are BLOCKED.** No production database, schema export, credential, provisioning action or deployment was accessed or executed. There is no new migration file.

Proposal B is not implemented. Arbitrary-case evidence-upload authorization remains an **unresolved high-severity release blocker**. Draft PR #130 must not be merged/deployed or treated as release-ready on the basis of these tests.

## Implemented enforcement

[`employeraccess.Authorize`](../../apps/evt-api-go/internal/employeraccess/assignments.go) requires a canonical UUID principal resolving to `domain_users.id`, current `status=active`, current `role=hr_reviewer`, and an explicit unrevoked `employer_hr_assignments` row for that principal and exact employer. Unknown/malformed principals, absent/revoked grants, disabled accounts and removed roles deny. A missing assignment table or lookup failure returns an error; no role-only fallback exists.

The check runs **inside each existing repository transaction**, before data access or signing:

| Endpoint | Enforcement path |
| --- | --- |
| `GET /v1/employer/requests?employer_id=...` | `EmployerHandlers.List` passes authenticated `claims.Sub` to `Repo.EmployerList`. |
| `GET /v1/employer/requests/{request_id}?employer_id=...` | `EmployerHandlers.Get` passes `claims.Sub` to `Repo.EmployerGet`; request must belong to the authorized employer. |
| `POST /v1/employer/requests/{request_id}/attest` | `Repo.EmployerAttestServerSigned` checks the authenticated HR principal before locking/processing the request and invoking the signer. Its existing `HREmployerAttest` compatibility alias delegates to the guarded method. |

Existing JWT/HR role gates and request workflow-state checks remain. Unauthorized/unknown resources return the same generic 404. Existing JWT/role failures remain 401/403; failed database checks return generic 500, never data or a successful empty list. Allowed response formats remain unchanged. Neither a `cvera` role nor an employer ID establishes access.

Authorization holds a shared lock on the current principal row through the existing read-committed transaction. Grant/revoke operations lock that same row exclusively before assignment changes. Tests verify both lock orders: revocation waits for an in-flight authorized transaction, and authorization waiting behind revocation reads the revoked state after commit. User status/role updates also conflict with this row lock. No authorization cache, new token claim, or resource-policy bypass was introduced.

## Operator-controlled provisioning and revocation

[`cmd/hr-assignment`](../../apps/evt-api-go/cmd/hr-assignment/main.go) is a separate operator CLI, with no HTTP route. It requires an explicit administrative DSN in `CVERA_HR_ADMIN_DATABASE_URL`; it does not fall back to application credentials. Its write transaction is bounded to 15 seconds and commits before reporting success. Error output omits raw driver errors and DSNs.

Grant requires explicit principal UUID, exact employer ID, attributable operator, and reviewed evidence/basis reference. It requires an existing active HR user and existing employer configuration row; it creates neither. The operator must separately verify organizational authorization—the configuration row alone is not evidence of it. An operator must verify the real employer identity and approval before executing the tool.

Revoke requires the assignment instance UUID, attributable operator and reason. It records revocation without deleting history, works after account disablement/role removal, and refuses to silently rewrite an already revoked instance. Regrant creates a fresh UUID; the fixture's partial unique index rejects duplicate active assignments. No grant is inferred from roles, registration, email, candidate requests, demo IDs or previous access.

**The CLI must not be run against a real database until schema integration and administrative/runtime privilege separation are approved and verified.** Intended invocation shapes, for a reviewed non-production environment only after that work:

```text
go run ./cmd/hr-assignment -action grant -principal <domain-user-uuid> -employer <exact-employer-id> -actor <approving-operator> -basis <reviewed-approval-reference>
go run ./cmd/hr-assignment -action revoke -assignment <assignment-uuid> -actor <revoking-operator> -reason <revocation-reason>
```

The operator must use a separate, attributable database administrative identity and approved schema search path. Application runtime credentials must have no ownership, INSERT, UPDATE, DELETE or administrative role inheritance that could alter assignments. The CLI's exported helper is not itself a permission boundary: database privileges enforce this separation. Fixture tests prove restricted-role behavior; actual privilege configuration remains BLOCKED with the migration. Raw ad-hoc assignment updates must not bypass the common principal-lock protocol.

## What schema verification established—and did not

The repository search covered every tracked SQL file/migration, schema-related documentation, `scripts/deploy-api.sh`, CI workflow deployment steps, runtime SQL queries, and the prior deployment security contract.

- `migrations/000001` tracks schema versions; `000002`–`000013` define/harden credential-authority persistence. They do **not** define the legacy user/employer/request tables or their relationships.
- `docs/auth/auth-contract.md` contains illustrative refresh-token DDL, not authoritative legacy user/employer/request definitions.
- `docs/api/v1-evidence-upload.md` contains proposed case/upload tables, unrelated to employer assignment FKs.
- `docs/architecture/subject-identifier-proposal.md` documents the implemented UUID application principal `domain_users.id`, but is not a database schema export. The unused `ResolveUserID` helper's `user_id` assumption is not adopted.
- The deployment script builds/restarts the API and checks health/revision; it does not establish these legacy tables or a migration/provisioning mechanism.
- The existing [authority deployment security contract](../operations/authority-persistence-v1-deployment-security.md) already records that the legacy DDL, ownership, grants and provisioning mechanism were not established.
- A read-only catalog query in the existing **disposable local** PostgreSQL 16 database found no `domain_users`, `evt_employer_config` or `evt_requests` relations. Only the disposable test database and default `postgres` database were listed. No production connection was attempted.

The test helper explicitly creates synthetic `domain_users.id UUID`, employer identifiers as `TEXT`, selected parent keys/FKs, and the proposed assignment table. These are **test assumptions only**. They have not been promoted into a migration or represented as actual deployed types/constraints.

## Exact missing information required before migration

Obtain a schema-only export through a separately approved read-only process, including:

1. Exact database schema/relation names and DDL for `domain_users`, `evt_employer_config`, `evt_requests` (including partitioning or views, if applicable).
2. For `domain_users`: exact `id` type/domain, primary/unique keys, nullability/default/generation, `kratos_identity_id` uniqueness, actual role/status types and allowed values/checks. Confirm that deployed JWT subjects resolve to this key rather than a different legacy `user_id`.
3. For employer references: exact types/domains, lengths and collations of both `evt_employer_config.employer_id` and `evt_requests.employer_id`; candidate FK targets, uniqueness, nullability, and any actual authoritative employer table. Confirm which key is stable and FK-eligible. Do not infer `TEXT` from string parameters in Go.
4. For requests: `request_id` key/type and actual FK relationships to employer/principal tables, including the `candidate_personid` relationship; enum/check definitions governing status and employer responses. These are needed to validate the real integration fixtures without changing the workflow schema.
5. Existing indexes, FKs and referential actions, triggers/functions, RLS policies and relevant permissions/ownership on these relations. Include any existing assignment/organization tables or provisioning functions so an authoritative existing relationship can be reused rather than duplicated.
6. The actual migration role/process and destination schema/search path; intended non-owner runtime role and operator roles, their table/column grants, role inheritance and schema CREATE privileges. This is required for assignment read/write separation and principal-row locking; the restricted fixture uses SELECT plus existing-style `UPDATE(email)` on `domain_users` for locks, without role/status writes.
7. Approved employer/principal assignment provisioning source and evidence-review responsibility. Do not include user rows, passwords, connection strings or private keys in the schema export.

Once verified, create the smallest compatible assignment migration with real FK types, grant-instance history, unrevoked-pair uniqueness and attribution constraints; start empty and provision only individually reviewed pairs. No automatic user backfill or blanket grants. Until then **database migration and real-schema integration are BLOCKED**, even though application checks and fixture-based tests pass. Deploying this code before the required assignment schema exists would fail closed on affected endpoints; it is not a supported rollout.

## Regression evidence

Working directory: `apps/evt-api-go`. The existing disposable PostgreSQL 16 test URL is passed privately in `CVERA_AUTHORITY_TEST_DATABASE_URL`; tests reject other hosts/users/database names. Random fixture schemas and probe roles are removed on cleanup.

[`employer_authorization_integration_test.go`](../../apps/evt-api-go/internal/httpapi/employer_authorization_integration_test.go) covers:

- Assigned employer list/get success, cross-employer list/get/attest denial, both request/employer substitution directions, and unknown employers.
- No signed JWS, request transition or successful event after a denied attestation; nil signer would expose authorization-order mistakes.
- Explicit second-employer grants, existing request workflow-state rejection, and no automatic cross-employer access.
- Missing/malformed principals, role-only access, requestor/internal role denial, disabled users, stale JWT roles, and fail-closed behavior with an unavailable assignment table.
- Operator grant validation, duplicate active-grant rejection, nonexistent users/employers, attribution fields, revocation across all endpoints, regrant as a new instance, preserved history, and revocation after account disablement.
- Deterministic PostgreSQL lock contention for authorization/revocation in both orders.
- A restricted NOLOGIN probe role can authorize reads with permitted row locking but cannot grant, revoke, clear revocation, delete assignments or update user roles (SQLSTATE 42501).

`TestEmployerAssignedLocalSigning` in [the existing security integration tests](../../apps/evt-api-go/internal/httpapi/security_integration_test.go) replaces the old employer-gap success reproduction with an explicitly assigned allowed-path test. It preserves temporary RSA-3072 issuance/storage/JWKS checks and rejects wrong keys, tampering and unknown kids. All other completed regressions remain, including the deliberately passing **known case-upload gap reproduction**, which remains evidence of an open blocker.

Commands/results:

- `go test ./internal/httpapi -run 'TestEmployer' -count=1 -v`: PASS with the fixture database; six focused tests.
- `go test ./... -count=1`: PASS with the fixture database, including existing authority PostgreSQL integration tests.
- `go test -race ./internal/httpapi ./internal/auth ./internal/employeraccess -count=1`: PASS with the fixture database.
- `go vet ./...`: PASS.
- `git diff --check`: PASS.

No fresh audit or unnecessary dependency/CI changes were made. Existing `govulncheck` findings are retained with their [individual dispositions](weekend-2-dependency-triage.md). **GO-2026-6611 (outbound HTTP/2 CPU exhaustion) and GO-2026-6809 (x509 certificate distrust) remain OUTSTANDING until release-build verification is complete.** Local compiler results do not certify the release image. Earlier credential-exposure, legacy normalization/status and live production verification limitations also remain.
