# Weekend 2 authorization proposals — approval required

Status: **Proposal A APPROVED; application/operator code implemented against explicit fixtures, migration and actual-schema integration BLOCKED. Proposal B remains PROPOSED, NOT APPROVED OR IMPLEMENTED.** See [Proposal A implementation evidence and missing schema requirements](weekend-2-employer-assignments.md). No production provisioning, deployment or merge is authorized. The sections below preserve the designs reviewed for approval.

## Observed identifiers, relationships, and provisioning

| Object | Repository evidence | What may be relied upon |
| --- | --- | --- |
| Application principal | `domain_users.id` (UUID); `kratos_identity_id`, `email`, `name`, `role`, `status`. `issueAuthExchangeResponseForKratosIdentity` upserts by provider ID and mints JWT `sub=id`. `HandleAuthRefresh` reads role/status by `id`. | Use the authenticated application's UUID principal. Email and Kratos IDs are not resource grants. Existing table DDL is unavailable. |
| Identity helper | `httpapi.ResolveUserID` instead assumes `domain_users.user_id` and references `hr_assigned_user_id` only in a comment. No call sites were found. | Do not adopt this unused helper or assume those columns exist; use the implemented `id` contract. No identity-model refactoring is proposed. |
| User provisioning | Exchange creates new users with `role=requestor`, `status=active`, retaining existing role/status on conflict. `db.EnsureDomainUser` has a legacy default `hr_reviewer` and can update role/status, but no call sites were found. | Registration/exchange must create **no resource assignments**. No in-repository supported role-administration or employer-assignment mechanism was found. |
| Employer/request | `evt_requests.request_id`, `employer_id`, `candidate_personid`, status and response/signature fields. Candidate handler passes `claims.Sub` as `candidate_personid`. `CandidateGet/List/Update/Submit/Cancel` scope by that value. | These existing predicates protect candidate workflows. They do not map a reviewer to an employer or a request to a case. Actual legacy column types/FKs need catalog confirmation. |
| Employer configuration | `evt_employer_config.employer_id`, `dispatch_mode`; `ensureEmployerConfig` inserts a config row during `CandidateCreateDraft` using the requested employer ID. | A config row is a workflow/configuration reference, **not evidence that the organization or reviewer is authorized**. No canonical employer registry/onboarding relationship was found. |
| HR handlers | `EmployerHandlers.List/Get/Attest`; `Repo.EmployerList`, `EmployerGet`, `EmployerAttestServerSigned`. Existing global `GinRequireRole(RoleHRReviewer)` and request status/row-lock checks. | Keep those controls; add an explicit principal/employer relationship. HR subject currently enters audit/signed payload only. |
| Credential authority | Migrated `authority_authorizations(authorization_id, issuer_id, authority_subject_id, claim_type)`, adjudications, lifecycle journal, resolver/evaluator. | This represents issuer assertion authority. No principal-to-issuer mapping makes it an application access table. Do not equate user UUIDs, issuer IDs and employer IDs or repurpose the frozen lifecycle as API membership. |
| Cases/checks | Documentation proposes `cases(case_id TEXT PK, status, deleted_at, ...)` and `verification_checks(case_id, check_id, status, deleted_at, ...)`, with `(case_id,check_id)` PK and FK to case. | These are **documented DDL, not repository migrations or verified deployed tables**. No owner/membership column exists in the documented case schema. No case creation/provisioning implementation was found. |
| Upload sessions/evidence | Documentation proposes `upload_sessions(upload_session_id UUID, case_id, check_id, uploaded_by TEXT, expires_at, closed_at, ...)` and `evidence_uploads(upload_id UUID, upload_session_id, case_id, check_id, storage_key, expected_size, expected_mime_type, ...)`. | Ownership of an upload session is necessary for completion, but cannot establish permission to the case before session creation. These tables are likewise not migrated by this repository. |
| Actual evidence implementation | `HandleEvidenceInit` validates IDs and presigns directly; it does not query cases, create upload sessions or persist evidence. `HandleEvidenceComplete` returns 501. `registerEvidenceRoutes` now authenticates all four mounted paths. | No case-access relationship currently exists to reuse. `caseId` is a string identifier; it is not demonstrated to equal `request_id`. Check IDs are local to a case. |
| Client compatibility | `HRReviewScreen.SettingsStyle.tsx` uses demo employer `emp_demo_001`, case `EVT-10324`, and check `employment.company_and_dates`. Its upload helper omits bearer authorization; employer calls use an access token. | Demo constants are not provisioning. Existing upload authentication already requires the client to send a bearer token. No demo ID or existing session will be grandfathered into a grant. |

Primary references: [exchange](../../apps/evt-api-go/internal/httpapi/auth_handlers.go), [claims](../../apps/evt-api-go/internal/auth/claims.go), [identity helper](../../apps/evt-api-go/internal/httpapi/identity_resolver.go), [legacy provisioning helper](../../apps/evt-api-go/internal/db/store.go), [EVT repository](../../apps/evt-api-go/internal/evt/repo.go), [employer handlers](../../apps/evt-api-go/internal/evt/handlers_employer.go), [evidence handlers](../../apps/evt-api-go/internal/httpapi/handlers_evidence.go), [documented evidence schema/contract](../api/v1-evidence-upload.md), [principal/subject draft](../architecture/subject-identifier-proposal.md), [frozen issuer authority specification](../architecture/authority-evidence-and-grant-resolution.md).

## Proposal A: explicit employer HR assignment

**Decision requested:** approve one authoritative server-side assignment per principal/employer pair, administered explicitly by a trusted operator. An active assignment permits the existing employer read and attest operations only while the user is currently an active HRReviewer. It confers no access to other employers or evidence cases.

**Minimum schema addition, if no equivalent real table is subsequently verified:** `employer_hr_assignments` with:

- `assignment_id UUID PRIMARY KEY` identifying one grant instance.
- `principal_id UUID NOT NULL REFERENCES domain_users(id)`.
- `employer_id NOT NULL`, using the actual existing employer-ID type and exact stored value. Reference `evt_employer_config(employer_id)` only after its type/uniqueness are confirmed. That FK supplies referential integrity, not organizational authorization.
- `granted_at TIMESTAMPTZ NOT NULL`, `granted_by TEXT NOT NULL`, `grant_basis_ref TEXT NOT NULL` (attributable operator identity and approval/evidence reference).
- `revoked_at TIMESTAMPTZ NULL`, `revoked_by TEXT NULL`, `revocation_reason TEXT NULL`; constraints require complete revocation metadata and consistent time order.
- A unique partial index on `(principal_id, employer_id)` for unrevoked rows. No wildcard employer value, default principal, default grant, or global bypass. A revoked row is never reactivated; regrant uses a new assignment ID and new approval, retaining history.

No new employer directory, authentication provider, JWT claims, permission engine, or authority-registry mutation is needed. A trusted operator must verify which real organization an employer ID represents before assigning it; the auto-created config row cannot do that verification. If an authoritative employer table/assignment already exists outside the repository, reuse its confirmed keys and semantics instead of creating a duplicate.

**Enforcement:** after existing JWT and role validation, resolve the exact UUID `sub` to `domain_users.id` and require current DB `status=active`, `role=hr_reviewer`, and a matching unrevoked assignment. Unknown/invalid principal, missing/revoked relationship, unavailable lookup or ambiguous data never authorizes. Preserve role checks even if a grant exists. No grant is inferred from a candidate's request, the employee/claim subject, audit events, email, issuer authority, signing keys or caller input.

Pass principal identity into all three repository operations; enforce at that transaction boundary, not just in one route handler. For list, authorize the requested employer before querying and return only its records. For get/attest, additionally require the selected request's stored employer to equal the authorized employer. Client `employer_id` remains a selector for compatibility; it never establishes permission. Attestation authorization and request-state checks must succeed before the signer is called or any response/event is written.

**Affected endpoints, all required:**

| Route | Existing functions requiring the check |
| --- | --- |
| `GET /v1/employer/requests?employer_id=...` | `EmployerHandlers.List` → `Repo.EmployerList` |
| `GET /v1/employer/requests/{request_id}?employer_id=...` | `EmployerHandlers.Get` → `Repo.EmployerGet` |
| `POST /v1/employer/requests/{request_id}/attest` | `EmployerHandlers.Attest` → `Repo.EmployerAttestServerSigned` |

No other currently registered route calls these repository operations. Existing requestor/recruiter/internal routes keep their separate policies; this assignment does not grant or redefine those permissions.

**Create/revoke/administer:** initially use reviewed, parameterized operator transactions through the established trusted database administration process; no public assignment-management endpoint or dashboard. Record the approving operator and basis. Operator credentials must be separate from the application runtime. The runtime can read assignments but cannot insert/delete grants, change their principals/employers or clear revocation. A global `cvera` application role does not bypass this boundary. Revocation sets the metadata atomically; deleting a user/resource must not silently cascade-delete the assignment audit history. Reassigning an employer requires revoking the old grant and approving a new one. Setting a local user inactive or removing the HR role must also deny on the next operation, even with an unexpired JWT.

**Concurrency:** authorization and protected work use one read-committed transaction and a common principal-row coordination lock. The request holds a shared lock on the current `domain_users` row before reading assignments; the operator obtains an exclusive lock on that row before grant/revoke changes. Attestation retains that lock through signing and commit, alongside its existing request-row lock. This orders in-flight actions with revocation; after a revocation commits, subsequent checks cannot read an old grant. No grant cache/replica is introduced. Use the same lock order for all administrators and requests. Verify PostgreSQL locking privileges against actual grants first; do not solve a lock-permission error by granting runtime write access to assignment tables. The existing exchange already needs limited updates on `domain_users`; any additional coordination permission must be explicitly reviewed.

**Proposed tests:**

1. HR A assigned only employer A can list/get/attest A; each B endpoint denies, including when B's real request ID is supplied with A's employer selector and vice versa.
2. HR role alone, no grant, revoked grant, disabled account, stale JWT role, wrong role with a grant, malformed/unknown principal, unknown employer and lookup failure all deny.
3. An explicitly approved multi-employer user can access exactly those two employers; duplicate active assignments are rejected; regrant retains the revoked instance.
4. Denied attestations never call the signer, change workflow state, save a JWS or create a success event. Existing allowed workflows still sign with the temporary key and verify as before.
5. Concurrent revoke versus attest follows the documented lock ordering; revoke committed before authorization prevents signing. Database failure rolls back all state.
6. Runtime role cannot create/modify/re-enable assignments. Operator transactions are attributable, cannot grant unknown principals and do not fabricate employer confirmation.

**Migration/compatibility:** add the assignment table/indexes and least-privilege access through a reviewed migration after inspecting actual legacy schemas. It starts empty. Populate only individually verified principal/employer pairs; no cross-product/backfill from roles, request history or demo IDs. Missing reviewed assignments intentionally produce access denial. Stage schema before code and stage reviewed grants before legitimate workflows need them. Preserve URL/query/body and success-response formats and existing JWTs; new failures are intentional security enforcement. Existing JWT role revocation latency is removed for these guarded operations by the current DB role/status check.

## Proposal B: explicit case evidence-upload grant

**Decision requested:** approve a separate, narrow `evidence_upload` grant linking a principal to an existing case. An employer HR assignment alone gives no case access. Since no authoritative owner relationship was found, do not invent one by treating `case_id` as `request_id`, interpreting the subject as the uploader, or using the last uploader as an owner.

**Minimum schema addition:** `case_evidence_upload_grants` with `grant_id UUID PRIMARY KEY`, `principal_id UUID NOT NULL REFERENCES domain_users(id)`, `case_id TEXT NOT NULL REFERENCES cases(case_id)`, and the same attributable grant/revocation metadata and constraints as proposal A. A partial unique index prevents duplicate unrevoked `(principal_id, case_id)` grants. The table expresses only this fixed capability; it grants no case deletion, evidence reading, token issuance/revocation or assignment administration. A separate generic permissions table/role hierarchy is unnecessary.

If the documented `cases` and `verification_checks` already exist, reuse their verified relationships and status fields. Otherwise their minimal parent records, composite `(case_id, check_id)` key and FK must first be introduced in approved migrations/provisioning. Do not create a case/check as a side effect of uploading or accepting an arbitrary ID. This is a deployment prerequisite, not an assumption that the documentation has already been deployed. No mapping to `evt_requests` is introduced in the minimal design.

**Enforcement:** retain JWT authentication, current local `status=active`, an exact unrevoked case-upload grant, and existence of the requested check under that same case. Permit upload initiation only while case is `OPEN`, neither case nor check is deleted, and check is in `PENDING` or `IN_REVIEW` (proposed writable states; approval is requested for this choice). A new or existing global role has no implicit case rights. Grant existence does not override closed/deleted workflow state. No positive permission is inferred on missing data or DB error.

Read `sub` from authenticated context, not JSON. The canonical route's path identifies the resource; preserve current path/body consistency checks. Legacy init resolves the same resource from its body and invokes the identical authorization path. Authorization, state validation and offline presigning occur within one transaction, using the same principal-lock protocol as proposal A; return URLs only after success. Failures return no usable capability. Grants are read afresh, without positive caching.

**Mounted endpoints and aliases:**

| Route | Enforcement |
| --- | --- |
| `POST /v1/cases/{caseId}/checks/{checkId}/evidence:init` | `HandleEvidenceInit`: case grant and exact check relationship before any presigning. |
| `POST /v1/evidence/init` | Same handler and policy; the alias must not bypass the canonical check. |
| `POST /v1/cases/{caseId}/checks/{checkId}/evidence:complete` | Currently authenticated 501. Keep disabled. Before enabling completion, require current case permission **and** caller-owned session, matching case/check/upload IDs, expiry and closure checks. |
| `POST /v1/evidence/commit` | Currently authenticated 501; retain the same disabled behavior and identical future completion requirements. |

Case/check creation, evidence listing, token issuance/revocation and case deletion appear in the API document but are not mounted implementations. This proposal grants none of those actions or implements those endpoints. Any future implementation must establish its own explicit operation-specific authorization first.

**Upload-session boundary:** `uploaded_by` proves who initiated a particular upload, not who is entitled to an arbitrary case. When implementing the documented session lifecycle, store the canonical application principal UUID (not email or Kratos ID) and bind every upload to its persisted session and exact case/check. Another member of the same case must not complete someone else's session. The existing `uploaded_by TEXT` can store the UUID string without a blind type conversion; legacy non-UUID ownership values require verified mapping or denial, never guessing.

For the minimal authorization remediation, no changes to the existing init success schema or implementation of completion are necessary. Add the case/check/grant gate before the current presigner and keep completion 501. Creating `upload_sessions`/`evidence_uploads`, adding session IDs to responses and enabling completion is a separate, presently unfinished contract implementation; those changes are **not bundled as strictly necessary schema changes for this authorization flaw**. Absence of that lifecycle remains an explicit verification limitation, not permission to enable the stub.

**Create/revoke/administer:** a trusted operator verifies the case, principal and upload purpose, then explicitly inserts a grant with its approval reference. Neither case creation nor user registration auto-grants access. A candidate, HRReviewer, recruiter or internal user may upload only when that exact principal has the approved case grant; no account receives blanket rights. Revoke through the common principal-lock transaction; regrant creates a new recorded instance. Runtime has read-only grant access; administration remains outside the public API. New cases/checks must come from an established trusted provisioning source; none was found in the repository, so initial records require reviewed provisioning rather than user-supplied inference.

**Revocation limitation requiring acceptance:** revocation immediately prevents new init/complete authorization after it commits. An already returned presigned URL remains a bearer capability until its existing expiry (currently up to ten minutes); database revocation alone cannot withdraw it. Do not promise immediate storage revocation. The minimal proposal preserves that bounded capability lifetime; requiring immediate invalidation would need a separately approved storage design. No presigned URL may be logged in administrative audit records.

**Proposed tests:**

1. User A with a grant to case A receives URLs only for existing writable checks under A. Case B denies even with a valid access token, an HR role, an employer assignment, a guessed existing ID, or a copied URL/body from B.
2. Same-case/different-check relationship mismatches and path/body case mismatches deny; legacy aliases apply the same checks. Missing case/check/grant, revoked grant, disabled user, closed/deleted case/check and DB failures return no URL and never invoke presigning.
3. Grant lookup denial occurs before calling storage. Use a signing-call spy/synthetic offline credentials; make no storage upload or external request.
4. Revocation and init concurrency follow the approved transaction ordering; no grant is created implicitly and role changes do not fabricate case access.
5. Completion aliases remain 501 while unimplemented. If later enabled: cross-user completion denies even within the same case; cross-case/check/session/upload substitutions, expired/closed sessions and revoked access deny before storage access or persistence.
6. Runtime role cannot create/change grants or bypass their revocation. Case deletion/closure and parent-key constraints are exercised under the actual chosen schema/privileges.
7. Retain existing auth/refresh/SSRF/body-limit regressions. After approved implementation, replace each known-gap reproduction's expected vulnerable success with actual deny/allow assertions, retaining the local temporary-key signing verification as a separate allowed-path test. Until then the existing passing tests remain unchanged.

**Migration/compatibility:** no implicit case-owner backfill, demo-case grant, or equality join to `request_id`. Confirm case/check parent DDL before referencing it; stage empty grant schema and individually approved assignments. Existing unknown cases become inaccessible, intentionally. Preserve current init fields, aliases and response format, and the completion stub. The demo upload helper requires real resource selection/provisioning and authenticated requests; its hard-coded IDs must not be used to seed grants. Any client or lifecycle work beyond the minimum gate needs explicit scope approval.

## Shared failure semantics and approval boundary

Authentication failure remains 401; the existing role guard remains 403. Authenticated requests to nonexistent or unauthorized resources should return the same generic 404, without exposing whether a foreign employer/case exists. Database/authorization-system errors return a generic 5xx and no data, JWS or URL; they must not be mistaken for permission or silently treated as an empty successful list. Approved empty employer queues still return 200 with an empty list. Existing invalid-body/ID/state errors remain where they do not reveal foreign resources.

Both proposals require confirming actual schema names/types, FK targets, grants and migration/provisioning mechanisms before implementation. This inspection did not access production or reinterpret synthetic test DDL as deployed schema. No executable migration or provisioning command is supplied for accidental execution.

Approval recorded for **A** (explicit employer HR assignments, current role/status gates, no global bypass, operator administration) ; separate approval is still required for **B** (explicit per-case upload grants independent of employer assignments, writable states, existing URL lifetime, completion remaining disabled). Approval of this document would authorize a subsequent focused implementation proposal/work step; it would not authorize deployment, production provisioning, merge, a new admin API or broad upload-lifecycle development.
