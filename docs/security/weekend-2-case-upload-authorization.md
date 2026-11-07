# Weekend 2 Proposal B: case-upload authorization

Status: owner-approved architecture implemented on `security/weekend-2-case-upload-authorization`; local verification recorded below. **Not applied or deployed to production. The deployed arbitrary-case upload vulnerability remains HIGH until the controlled migration, bindings and application rollout are authorized and verified.** Documentation-only PR #131 remains separate and unmerged. Production login/exchange/logout and live attestation checks remain separate outstanding acceptance checks.

## Evidence and approved scope

`internal/httpapi/handlers_evidence.go:HandleEvidenceInit` previously validated caller-provided identifiers and files and immediately generated storage capabilities. Both init aliases authenticated but performed no case-level authorization. The API document's case/check schema was never evidence of deployed tables or ownership. The supplied production inventory contained neither table. Candidate request ownership and employer HR assignments are unrelated to this registry and are not used as upload permissions.

Migration `000015_case_evidence_authorization.sql` introduces the approved **new** minimal registry; it does not reproduce an assumed production schema or implement the entire evidence API document. The verified existing dependency is `domain_users.id UUID PRIMARY KEY` and `status TEXT NOT NULL`. No application records, cases, checks or grants are seeded.

| New relation | Identity and policy |
| --- | --- |
| `cases` | String `case_id` primary key, at most 64 documented identifier characters; `OPEN`, `CLOSED`, `DELETION_SCHEDULED`, `DELETED`; creation and latest state-change actor/basis/timestamps. |
| `verification_checks` | Composite primary key `(case_id, check_id)`, restricting FK to `cases`; check string at most 96 identifier characters; `PENDING`, `IN_REVIEW`, `PASSED`, `FAILED`, `DELETED`; creation/latest state attribution. |
| `case_evidence_upload_grants` | UUID grant ID; restricting FKs to canonical principal UUID and case string; grant actor, approval reference, timestamp and actual database session user; revocation actor/reason/time/session user. Partial unique index permits one unrevoked grant per principal/case. |

Single-dot and double-dot case/check identifiers are rejected in the new registry because the existing object-key builder normalizes path segments. Other documented identifier characters remain supported. These are new registry constraints, not conversions of legacy IDs. No case/request, employer/case or user/case ownership mapping is inferred.

## Authorization and concurrency

The shared handler gates **both** `POST /v1/cases/{caseId}/checks/{checkId}/evidence:init` and `POST /v1/evidence/init`. The subject comes exclusively from authenticated claims (`sub = domain_users.id`), never from JSON, token roles, email or a Kratos identifier. Existing path/body consistency and file policy remain in force.

A READ COMMITTED transaction invokes `check_evidence_upload(uuid,text,text)`. It locks, in order, the principal, case, exact composite check and unrevoked grant with `FOR SHARE`. It permits only a currently `active` principal, an `OPEN` case, a `PENDING` or `IN_REVIEW` check, and that principal's explicit unrevoked grant. Missing or unauthorized resources return the same generic 404. Invalid authentication remains 401. Database/checker failures return generic 503 with no URL. Unreviewed RLS or a different transaction isolation level is rejected.

Locks remain held through offline presigning. The handler commits before exposing any URL; failure paths roll back. Revocation updates the grant row and state changes update the actual case/check row, so their row locks conflict with authorization; principal deactivation also conflicts with the principal shared lock. If a mutation commits first, the waiting authorization reads current data and denies. If authorization wins, the mutation waits until that transaction ends; later requests deny after the mutation commits. SQL tests observe actual PostgreSQL blocking in both orderings for all four mutations, rather than relying on sleeps alone.

A grant-revocation transaction may commit between an authorized request's commit and delivery of its response. This is an already-issued-capability limitation, not a promise that response delivery is synchronized with revocation. No positive permission cache is introduced.

## Runtime and operator boundary

All new tables start owner-only, including removal of privileges inherited from the production owner's broad default table ACLs. Every new function loses PUBLIC and other non-owner default EXECUTE privileges. Definer functions use typed inputs, static schema-qualified statements and `search_path = pg_catalog, <trusted schema>, pg_temp`; no caller-controlled SQL is executed. Temporary tables cannot replace registry or principal lookups.

The separate reviewed `operations/evidence-authorization-privileges.sql` takes explicit runtime/operator role settings. It rejects owner-capable/privileged/member roles, schema/database creation rights, RLS, owner/path mismatches, unexpected table/column ACLs and unexpected function grants. The operator must be NOLOGIN. It creates no role or membership and changes no legacy table privileges.

- Runtime receives only schema USAGE and EXECUTE on `check_evidence_upload`. It receives no direct registry SELECT/INSERT/UPDATE/DELETE and no administrative function EXECUTE.
- Operator receives only schema USAGE and EXECUTE on the five bounded administrative functions below, with no direct registry DML. No runtime-to-operator membership is allowed.
- The trusted migration owner retains ownership. Owner/superuser access remains the explicit administrative trust boundary.

| Operator function | Authorized action |
| --- | --- |
| `provision_evidence_case(case, actor, basis)` | Explicitly create an empty case; duplicate IDs fail. No grant is created. |
| `provision_evidence_check(case, check, actor, basis)` | Explicitly create a check under an existing case; missing parent or duplicate identity fails. |
| `grant_evidence_upload(principal, case, actor, basis)` | Require an existing active principal and OPEN case; return a new grant UUID for the operator's controlled record. |
| `revoke_evidence_upload(grant UUID, actor, reason)` | Revoke one unrevoked instance; preserve its identity/provenance. Regrant is a new record. |
| `set_evidence_resource_state(case, check-or-NULL, state, actor, reason)` | Update the exact persisted resource; NULL check selects the case. Constraints restrict each resource's state vocabulary. No implicit resource creation. |

Actors and approval/reason fields must be nonblank; actual grant/revocation database session users are independently recorded. Operator functions do not let their caller overwrite grant provenance, delete grants, reactivate a revoked grant or change another principal's identity. State attribution records creation and the latest change, not a complete append-only state history. Resource reopening requires an explicit operator state change; no background or upload-triggered reopening exists.

## Migration and compatibility design — not execution authority

Production database changes, role binding, rollout and credential changes require separate authorization. The verified runtime role is `cvera_api_runtime`; **the evidence operator identity and any new role/membership must be explicitly reviewed**, not inferred from the HR operator role. No additional password or LOGIN role is needed by this design.

1. Use the established owner console and recovery procedure. Before any approved SQL execution, take and verify current private backups, stage exact reviewed artifact hashes and rehearse against an isolated current restore. Confirm parent metadata, trusted schema, owners/default ACLs/RLS, absence of conflicting registry/function names, and the selected operator/runtime identities. Stop on unexpected existing objects instead of reconciling or overwriting them.
2. As the trusted owner, in one explicit transaction at READ COMMITTED with bounded lock/statement timeouts and explicit trusted search path, apply **only 000015**, set the reviewed `cvera.evidence_runtime_role` and `cvera.evidence_operator_role`, then apply the binding. Verify new tables are empty, owner/path/ACL contracts, PUBLIC denial, runtime boolean check access and administrative denial. Commit only after all checks pass.
3. Record only this actual migration's reviewed hashes, database, committed transaction and verification in the controlled operator record, using the established 000014 recording convention. Do not invent earlier versions, insert fabricated history or introduce an automatic migration runner. The migration's outer DO is atomic even without an external transaction; reapplication intentionally fails on existing objects. The binding may be repeated for the same reviewed roles, not used to silently transfer privileges to new roles.
4. Before application rollout, explicitly provision real approved case/check records and individually approved grants for intended existing clients. Obtain identifiers and approval references from their responsible operator; do not seed demo IDs, register users, backfill implicit ownership or assume all authenticated users are entitled. Unknown resources and existing unassigned users will be denied by design. The registry is intentionally narrower than the full evidence-lifecycle schema in the API document.
5. Deploy the reviewed application only under separate rollout authority, then verify allowed/denied upload initiation using approved records without making uploads or accepting evidence unless separately authorized. No migration or provisioning occurs at application startup. With code but no function/binding, init fails closed with 503; with an empty registry it returns 404. Existing authentication, employment workflows and completion behavior are preserved.

Before commit, errors roll back the entire schema/binding transaction. After commit, do not drop populated registry/grant records as an improvised rollback. The prior application ignores these new tables and can run with them present, but restoring it reintroduces the HIGH arbitrary-case issue; such recovery needs an explicit risk decision and the established captured-image procedure. This work adds no automatic deployment or recovery action.

## Verification evidence

Local PostgreSQL **16.15** (production metadata previously reported **16.11**) uses the guarded existing disposable `cvera_authority_test` fixture and isolated per-test schemas. Tests apply the **actual new migration and binding**, with confirmed existing parent metadata. The HTTP path runs under a restricted runtime role; administrative actions run under a separate NOLOGIN operator. Synthetic storage credentials presign offline; no real upload, production connection, account credential or signing key is used.

| Command (Go app directory) | Result |
| --- | --- |
| `go test ./internal/httpapi -run TestEvidence -count=1 -v -timeout=120s` with guarded local fixture | PASS: both init aliases, allowed uploads/expiry, cross-case/user/check denial, no implicit grants, all denied states, deactivation, revocation/regrant, unchanged file policy and completion 501. |
| Same focused suite | PASS: effective ACLs and actual SQL runtime denials, PUBLIC ACL removal, TEMP shadowing denial, RLS/isolation fail-closed, migration rollback/late failure/wrong parent type/reapplication, grant attribution and invalid operator actions. |
| Same focused suite | PASS: eight observed blocking/order tests, both directions for revocation, case closure, check state and principal deactivation; subsequent HTTP requests deny. |
| `go test ./... -count=1 -timeout=120s` with guarded local fixture | PASS. |
| `go test -race ./internal/httpapi ./internal/auth ./internal/employeraccess -count=1 -timeout=120s` with guarded local fixture | PASS. |
| `go vet ./...` | PASS. |
| `git diff --check` | PASS. |

The local wrapper supplies the existing disposable database password in memory; no credential-bearing DSN is written into this report. Initial test attempts exposed a missing local fixture password and a fixture parameter-type mismatch; both were corrected before the passing runs. Production was not contacted. GitHub verification is recorded below once available.

## Residual limitations and acceptance

Previously issued presigned URLs remain bearer capabilities for their unchanged ten-minute validity. Database revocation prevents later init authorization but cannot withdraw a URL already issued; transfers begun before expiry may finish later. URLs must not be copied into logs, Git or review comments.

Both completion aliases remain authenticated **501**. This remediation does not add upload-session persistence, server-side hashing/deduplication, object-size/content validation at completion, evidence acceptance or a claim that arbitrary uploaded objects are accepted evidence. Existing MIME/declared-size/file-count restrictions are preserved; they are not a certification of uploaded bytes. No complete SHA-256 acceptance lifecycle existed in the inspected handler to preserve or claim verified.

Proposal B can be called locally remediated only for the reviewed migration/binding/application combination. Its production HIGH finding remains open until separately authorized installation and verification. Positive production login/exchange/logout and controlled live signing/verification remain independent acceptance items; neither is inferred from local tests or Checkpoint 24.
