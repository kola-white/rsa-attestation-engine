# Request-bound evidence preparation

Status reconciled **2026-11-10**: #140 deployed at e84b9a6; its separate synthetic
acceptance remains UNCONFIRMED. PR #141 subsequently shipped integrated issuance/
verification (owner-confirmed deployment). Genuine NorStar acceptance is pending.
The original non-signing #140 behavior and remaining-work list below are historical
milestone context; see [current readiness](mvp-readiness.md) and the
[operational checklist](../operations/norstar-production-readiness.md).
The owner approved the policy and exact issuer `https://cvera.app` (no trailing
slash) on 2026-11-08. That identifier is distinct from authentication JWT `iss`,
the API hostname, HR principal IDs, employer IDs and signing-key `kid`. Choosing
an issuer identifier establishes no employer authorization.

## Binding and permissions

Migration 000018 adds an operator-only, attributable, revocable relationship:

- Exactly one `evt_requests.request_id` UUID per `cases.case_id` text and vice
  versa, permanently unique even after revocation. No reassignment/backfill.
- Explicit nonempty required check IDs, each persisted under the exact composite
  `(case_id, check_id)` relationship. IDs/labels do not imply claim semantics.
- Subject copied from that request's canonical `candidate_personid`, employer from
  `employer_id`, version and SHA-256 digest of its persisted JSONB claim snapshot.
  The uploader/reviewer is not inferred to be the subject. This is an internal
  request-local subject reference, not a new global/public identifier model.
- Issuer `https://cvera.app`, existing claim type `employment.role`, actor/basis,
  server timestamp/session attribution and immutable revocation audit.

Binding requires an OPEN case, eligible persisted checks and a submitted request
in ATTESTATION_PENDING. No upload session or file may already exist anywhere in
that case, including expired/failed reservations. No cases, checks, principals,
employers, HR assignments or evidence grants are created implicitly. Old standalone
pilot evidence cannot be adopted retroactively. A changed request version,
subject, employer, claim digest or workflow state cannot silently replace context.

The existing restricted evidence operator gets only bind/revoke EXECUTE. Runtime
gets bounded context/review/preparation/signing-exclusion functions, no new direct
table access or authority-schema privileges. Existing HR/employer and case
upload/review grants remain separate; binding confers none of them. New tables
strip inherited legacy CRUD defaults and functions strip PUBLIC/custom EXECUTE.
The canonical issuer/manual policy row is owner-only and immutable; it creates
no ACTIVE authority or organizational delegation.

## Contextual review

The existing received-evidence listing gains an optional `binding` object for a
bound case. Current case reviewers can see request/subject/employer/issuer refs,
claim type/digest and the matching claim snapshot. If the request changes or the
binding is revoked, context is unusable, review flags are false and changed claim
contents are not disclosed through the old binding. Existing file access remains
controlled by the accepted case-review grants and resource-state policy.

For a required bound check, POST to the existing file `/review` route must include
`bindingId` and `claimDigest` returned by the server, alongside decision/reason.
These are acknowledgements, not caller-established identity or authority. The
server matches current persisted context and records it atomically with the
immutable review receipt. Wrong/missing context denies; an upload/review receipt
from another request cannot satisfy this binding. No self-review, exact retry,
conflicting decision and manual-versus-scan rules remain unchanged. Standalone
cases retain their existing listing and review contract.

## Eligibility endpoint

`POST /v1/employer/requests/{request_id}/evidence-eligibility` takes only the
existing `employer_id` scope. It requires a current active HRReviewer with an
explicit employer assignment and that employer's exact request. Body assertions
about issuer, authority, subject, case or review outcome confer no permission.
Unauthorized resources return 404; missing authentication 401, wrong API role
403, invalid body 400 and unavailable persistence/authority operations 503.

All required checks must remain PENDING or IN_REVIEW in an OPEN case. Each needs
at least one RECEIVED file; every received file in those checks must have an
independent APPROVED review acknowledging this binding/context. Pending/rejected
reviews, missing scan work, INFECTED/ERROR scans and live incomplete uploads block
preparation. Expired, uncompletable reservations are not accepted evidence and do
not block otherwise approved files. Deleted storage tombstones cannot qualify.
PENDING malware status is permitted **only** by MANUAL_REVIEW_PILOT_V1, is retained
in the manifest/response and never becomes CLEAN through review. No check becomes
PASSED, no trust result becomes VERIFIED, and no attestation is created.

The server creates an immutable evidence-only manifest containing the binding and
request/subject/employer/issuer/claim-digest context, exact required checks, upload
IDs/hashes, review IDs/reviewer/grant attribution and distinct scan status. No file
bytes, storage keys, URLs or duplicate raw claim snapshot are stored in it.
The internal digest is SHA-256 over PostgreSQL JSONB text encoded as UTF-8; it is
not a new portable credential canonicalization profile. Exact retries return the
same receipt; changed evidence produces another immutable preparation snapshot.

A response separates `evidenceReady`, `eligible`, `authorityDecision`, reason and
manifest reference. `scanStatuses` and `pendingScanCount` explicitly expose the
distinct malware state; `signingEnabled` is always false. Manifests are preparation
receipts, not issuance capabilities or proof of current authority. Revoking a
reviewer's access grant does not erase their historical valid review; revoking
this binding or closing its case prevents new eligibility. Deletion freeze also
revokes a binding, and retained manifests are historical audit, not restored files.

## Authority and signing boundary

Eligibility requires current ACTIVE authority for the exact issuer, employer
(authority subject) and employment.role. The existing accepted journal/projector,
resolver and evaluator are reused; missing authority remains REVIEW, explicit
current inactive authority remains UNAUTHORIZED, and neither permits eligibility.
No positive authority is inferred from the identity policy row, HR assignments,
case grants, manual review, key possession or a client-supplied decision.

A trusted source must hold the existing authority recording-boundary lock within
the same transaction. The new locked-snapshot loader reuses existing completeness
and projection validation; the shared boundary blocks participating authority
writes until preparation commits. Request, case, check, binding, upload, scan and
review locks coordinate resource/review/state changes; no stale snapshot is a
cached permission. Unsupported isolation, RLS or unsafe schema resolution fails
closed. The positive authority bridge is tested with real local journal fixtures.

**Historical #140 deployment state, superseded by #141:** production authority
source was intentionally unconfigured. The runtime is
not granted authority-table access and migrations 000001–000013 are not presumed
deployed or fabricated. When evidence is ready the production endpoint therefore
returns `eligible:false`, `evidenceReady:true`, `reason:AUTHORITY_SOURCE_UNAVAILABLE`.
A source operational/integrity failure is an error, not successful empty authority.

Positive FULL_MATCH/PARTIAL_MATCH on the existing legacy `/attest` route returns
409 `evidence_backed_signing_not_enabled` for any bound request, including revoked
bindings. This prevents an eligibility bypass. Unbound legacy behavior and normal
employer rejections remain unchanged. No private key is read for diagnostics and
no new signing path, issuer metadata publication or production signing is enabled.

## Verification and release

Local PostgreSQL tests cover binding/context/ACL/immutability, no adoption/reuse,
legacy signing exclusion, standalone compatibility, malformed/changed context,
required/disqualifying evidence, manifest idempotency, real authority scope and
ACTIVE/APPROVED/suspended/revoked/expired behavior, failed-binding DDL rollback,
plus actual blocking in both binding/authority revocation orderings. Existing
Go tests, relevant race checks and vet pass; CI outcomes are recorded separately
when complete.
Production migration must verify the real request version and claim_snapshot
DDL, restore-test a fresh backup, and atomically apply 000018 plus exact bounded
bindings before the API is represented as binding-ready. No authority schema,
credentials, memberships or broad table privileges are added by this milestone.

## Historical remaining work at the #140 milestone

The implementation work below shipped in #141; NorStar authority is now
owner-confirmed ACTIVE (transaction 3019). Public credential/HR configuration,
Tara's review and genuine acceptance remain pending as listed in current readiness.

1. Establish real, attributable employer delegation for canonical issuer
   https://cvera.app and employment.role. Verify/deploy the existing authority
   persistence and restricted coherent-read mechanism through a separately
   approved production procedure; wire the trusted source without blanket CRUD.
2. Approve the credential profile binding: cryptographically commit the intended
   issuer, appropriate subject representation, frozen claim/request context and
   exact evidence-manifest reference/digest. Current legacy AttestationClaims lacks
   those commitments. An unsigned preparation receipt is not portable evidence.
3. Add a single atomic issuance path that rechecks current HR/employer authority,
   request/binding/resources/reviews and ACTIVE issuer authority; signs only that
   locked context; and records the resulting artifact/manifest/authority boundary
   without enabling the existing legacy route as a fallback. Define credential
   validity/status independently; post-issuance binding revocation must not be
   falsely described as cryptographic credential revocation.
4. Obtain the separately deferred live-signing authorization for one legitimate
   controlled request. Use normal account/request/assigned-employer workflows,
   then independently verify exact public-key/profile/signature, subject/issuer/
   manifest commitments, authority, validity/status and verifier policy. Include
   tamper, wrong subject/employer/issuer, reuse, expiry/revocation and unavailable
   authority negatives. Do not substitute internal trust recording or recruiter
   consumption for independent verification, or fabricate VERIFIED.

Actual browser/native picker acceptance remains a separate validation task.
Automated scanning, workers, self-service administration and new infrastructure
are excluded.
