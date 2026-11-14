# Claim-bound HR review integrity

Status: local implementation and isolated validation only. Not deployed; migration
000020 is not applied to production. Employer resolution and existing employer
identifiers are unchanged. This supersedes positive legacy approval instructions
only after the separately approved release is complete.

## Scope and behavior

The HR queue now opens the existing Evidence Review screen filtered to the exact
selected request. It never submits a legacy approve/reject decision. Discovery
uses only explicitly granted cases and checks; the server still authorizes every
read and decision independently. An employer assignment does not confer a case
review grant, issuer delegation, or signing-key authority.

For an eligible bound simulated case the reviewer must open the canonical file,
inspect the claim snapshot, acknowledge the binding and document SHA-256, select
**Approve evidence (simulated UAT)**, supply a rationale, and confirm. The response
is an `UNSIGNED_EVIDENCE_REVIEW_RECEIPT`, not a signed credential. The request stays
ATTESTATION_PENDING with its frozen version; no HR_ATTESTED event, signing,
credential issuance, or VERIFIED transition is produced by evidence review.
Existing standalone evidence reviews remain separate and cannot be adopted into
an employment binding retroactively.

The new API rejects FULL_MATCH/PARTIAL_MATCH on `/attest` with HTTP 409 and
`positive_decision_requires_evidence_review` after ordinary authentication,
employer authorization and exact request/employer lookup. There is no legacy
signer call on this path. Negative legacy responses retain existing authorization
and status requirements and now also exclude the claimant; the HR screen no
longer exposes this shortcut. Historical signing/parsing helpers and read paths
are retained for existing artifacts and isolated compatibility fixtures.

## Trusted context: one immutable classification per existing binding

Migration `000020_claim_bound_review_integrity.sql` adds
`request_review_classifications` with a primary/FK binding_id, review_context,
classified_by, basis_ref, database timestamp and session-user attribution.

- Values are SIMULATED_UAT or AUTHORITATIVE. No row means UNCLASSIFIED. New bound
  decisions and issuance fail closed for unclassified bindings.
- The existing immutable binding already identifies one exact request, subject,
  employer, case, version and JSONB snapshot digest. Classification references
  that binding; it does not copy or replace its identity or permission model.
- `classify_request_review` is owner-only until the separately reviewed
  `operations/claim-review-integrity-privileges.sql` grants EXECUTE to the existing
  restricted evidence operator. Runtime has no classification function or table
  access. No roles, grants to Tara, authority records or classifications are seeded.
- Classification locks the case and request, requires unchanged eligible context,
  and refuses any prior case review, manifest or issuance. Existing uploads in an
  already bound case are not adopted or rebound; the original no-retroactive-
  evidence-binding rule remains intact.
- The primary key and UPDATE/DELETE trigger prohibit replacement, upgrade,
  downgrade or deletion, even through ordinary owner DML. The trusted DBA can
  alter schema/disable triggers; this is not a claim of protection from a malicious
  database administrator. Concurrent classification attempts yield one success.
- A wrong classification is not repaired by overwriting history. A new request/
  case requires separate approval; existing binding uniqueness is preserved.

AUTHORITATIVE classification is only a workflow context. It never establishes
organizational delegation, key trust or issuance eligibility by itself.

## Decision receipt and separation

The existing `request_evidence_review_contexts` gains nullable evidence_hash.
Historical rows remain unchanged; new bound decisions must acknowledge it.
The bound review SQL now requires the exact binding ID, claim digest and actual
received upload hash under the existing locks. It rejects the canonical claimant
as reviewer even when a different person uploaded the document, and retains the
existing uploader/reviewer exclusion, resource state, current case grant,
rationale, exact retry and conflict rules. The obsolete eight-argument SQL review
entry point rejects decisions; private pre-integrity helpers lose runtime/operator
EXECUTE. Standalone review cannot bypass a binding's acknowledgement requirement.

New receipt fields include kind, reviewContext, requestId, bindingId, caseId,
checkId, frozen requestVersion, claimDigest and evidenceHash, plus the original
attributable review ID, actor, decision, rationale and time. They are durably
recoverable from the review/context/classification and immutable parent binding.
Review decisions now also have a database UPDATE/DELETE guard. Exact retries
return the same receipt; a different decision or rationale conflicts. A context
write failure rolls back the underlying review insert.

The established SHA-256 convention remains exact UTF-8 PostgreSQL JSONB text.
No caller JSON serialization or later status-version increment redefines the
claim version or digest. The browser verifies downloaded canonical bytes before
allowing inspection acknowledgement, and validates every receipt context field.
An unsigned receipt is never treated as a credential by the verifier or UI.

## Issuance, positive-state and verifier boundaries

Existing preparation/commit routines are wrapped with restricted, fixed-schema
SECURITY DEFINER functions. The original implementations become owner-only
helpers; existing reviewed EXECUTE ACLs are preserved on the public wrappers.
The new review overload inherits the existing review entry-point recipients.
New-table and new/helper-function PUBLIC/hostile default ACLs are stripped.

- New issuance requires AUTHORITATIVE classification, a current assigned HR
  principal other than the canonical subject, and no claimant/uploader approvals
  or evidence-hash mismatches among the reviews being consumed.
- Preparation and commit both recheck this guard. An issuance INSERT trigger
  provides the same defense at persistence. The Go issuer refuses new signing
  before migration 000020 exists. Simulated eligibility reports review-only;
  unclassified eligibility reports unavailable context and neither prepares a
  production manifest nor enables signing.
- Existing evidence readiness, exact snapshot/manifest commitments, ACTIVE
  issuer/employer/claim-type authority, public key registration/fingerprint,
  validity, revocation, transaction and post-signing clock checks remain required.
  No UAT key or fake employer delegation is needed or created.
- A request trigger prevents internal trust-recording or legacy request-update
  paths from installing attestations/tokens or promoting ATTESTED/VERIFIED without
  a classified authoritative committed issuance matching the subject, employer,
  snapshot, version and token digest. Simulated/unclassified review alone cannot
  be promoted. This is not a replacement for independent cryptographic verification.
- Trusted credential resolution never treats a simulated classification or
  claimant-issued/reviewed artifact as eligible. Existing cryptographically valid
  historical evidence-backed credentials without a classification retain their
  original resolution, subject to current trust and the claimant-separation checks.
  No metadata is backfilled to assert that historical reviews were authoritative.

Existing signatures, signed claims, manifests, snapshot bytes, request bindings,
authority journals and audit events are not rewritten or re-signed. A historical
artifact with newly disqualifying claimant involvement may remain readable while
failing current trust; preserving bytes does not promise perpetually positive trust.

## Local verification

Tests run only against a newly created PostgreSQL 16 container with a RAM-backed
data directory, no persistent mounts and a random loopback-only port. Production
and the pre-existing persistent local database are excluded. All signing fixtures
use existing isolated test helpers; no production keys or credentials are accessed.

Coverage includes legacy positive denial with an unusable API signer; historical
legacy static-JWKS compatibility; claimant reviewer with a different uploader;
claimant issuer; forged stored claimant/uploader approvals; simulated issuance and
internal positive-state denial; missing grants/classification; binding/claim/hash
mismatch; stale versions; concurrent decisions/classification; exact/conflicting
retries; rollback after review insertion; migration rollback/replay; private helper
ACLs; and actual pre-000020 evidence-backed credential retrieval/verification.
Frontend coverage includes request filtering, mode-aware receipt validation,
claimant/unclassified denial, legacy-positive suppression, and existing stale-
response and submission ambiguity protections. Browser/device UAT is separate.

The old self-signing-success test now expects denial and tests historical artifact
compatibility separately. The old unbound-legacy-positive test expects denial.
Immutable review-state tests create rejected decisions through the real review
endpoint rather than rewriting an accepted review.

## Changed files and validation record

| Area | Files |
| --- | --- |
| Backend | `internal/evt/repo.go`, `internal/evt/handlers_employer.go`, `internal/db/evidence_issuance.go`, `internal/httpapi/handlers_evidence_review.go` under `apps/evt-api-go` |
| Schema / existing operator | `apps/evt-api-go/migrations/000020_claim_bound_review_integrity.sql`, `apps/evt-api-go/operations/claim-review-integrity-privileges.sql` |
| Frontend | `screens/HRReviewScreen.SettingsStyle.tsx`, `screens/EvidenceReviewScreen.tsx`, `src/api/employerReview.ts`, `src/api/evidenceReview.ts`, `src/navigation/MainAppNavigator.tsx`, `components/EvidenceReviewLink.tsx` under `apps/evt-ui-expo` |
| PostgreSQL regressions | `claim_review_integrity_integration_test.go` (new), `request_binding_integration_test.go`, `evidence_issuance_integration_test.go`, `security_integration_test.go` under `apps/evt-api-go/internal/httpapi` |
| Node regressions | `tests/employer-review.test.ts`, `tests/evidence-review-client.test.ts` |
| Documentation | This document and `docs/api/evidence-backed-issuance.md` |

Local validation: full Go/PostgreSQL suite **452 tests/subtests, 236 top-level
cases, zero failures or skips**; Node CI **42 passed**; race-enabled integrity and
issuance/verification concurrency suite **PASS**; `go vet ./...`, Expo typecheck,
web export, offline issuance-smoke tooling and isolated archive-restore release-
tool regression **PASS**. The historical release-tool regression covers its
existing 000019 contract; 000020 is tested by the new PostgreSQL suite, not claimed
as production-restored acceptance. Local compiler is Go 1.27.1; release CI remains
pinned to 1.26.10 and has not run for these uncommitted changes.

Evidence is retained in `/private/tmp/cvera-integrity-go-final.jsonl`,
`/private/tmp/cvera-integrity-race.log`, `/private/tmp/cvera-integrity-node-final.log`,
`/private/tmp/cvera-integrity-export-final.log`, and
`/private/tmp/cvera-integrity-release-tools.log`. Export output is
`/private/tmp/cvera-claim-integrity-web-final`. Browser/native device acceptance
and production schema/permission compatibility are not inferred from these tests.
No dependencies, earlier migration files, authentication logging, employer
identifiers or infrastructure configurations were changed.

## Exact operational prerequisites for Tara — not performed here

1. Obtain separate release/migration approval and an authorized read-only report
   of the existing request ID, status/version, canonical subject, exact employer,
   binding/case/check IDs and state, existing evidence/reviews/manifests/issuance,
   and Tara's current principal and relevant grants. Do not infer this from a
   queue or call a preparation POST as a read-only inspection.
2. Keep Tara paused. Deploy the API that disables positive legacy signing and
   fails closed before 000020. Do not restore an older signing-enabled API during
   simulated UAT: the database cannot prevent an old binary from invoking its
   signer before a rejected commit. No new deployment orchestration is required.
3. Before an approved production apply, inspect the actual catalog/ACLs and test
   a fresh private backup restore on the same PostgreSQL image. Rehearse 000020
   and its narrow operator binding in that isolated clone. The migration is one
   atomic DO; it seeds no business records and rejects replay. After apply, verify
   retained wrappers/helper ACLs and no direct classification-table privileges.
   These local fixture tests do not prove full production-schema compatibility.
   Record only the actual approved 000020 apply in the established migration
   ledger; do not fabricate earlier history or replay already completed releases.
4. Release the matching frontend and verify the deployed code, schema and
   simulation-denial behavior. Old bound-review clients without evidenceHash
   fail closed; they are not silently supported for new decisions.
5. For an existing unreviewed binding, obtain explicit authority for its one-time
   SIMULATED_UAT classification. If unbound, separately approve provisioning an
   OPEN empty case with explicit required checks and binding through the existing
   operator functions. The request must already be ATTESTATION_PENDING via its
   normal workflow; do not patch status, rename employers, overwrite a binding,
   adopt prior standalone evidence or invent a manual-dispatch capability.
6. If reviews, manifests or issuance already exist, or classification is already
   AUTHORITATIVE, stop: this implementation deliberately refuses retrospective
   classification/reclassification. Resolve through a separately approved new
   UAT request/case if needed, preserving existing records.
7. Independently verify Tara is neither the claimant nor uploader, with an active
   account, the existing employer assignment for queue access, and a separate
   unrevoked case-review grant. Only add a missing case grant or upload synthetic
   evidence under explicit new operational authority. Never broaden access merely
   to make acceptance pass. Issuer activation and organizational delegation are
   not prerequisites for simulated unsigned review.
8. Tara opens the selected claim's evidence workflow, sees the simulation banner,
   inspects harmless authorized evidence and frozen facts, acknowledges binding/
   file hash, gives rationale and records the simulated approval. Confirm the
   durable unsigned receipt and exact retry; verify unchanged request status/
   version, zero issuance/HR_ATTESTED records and no authoritative VERIFIED outcome.
   Genuine signing acceptance remains separately scoped and unauthorized here.

A pre-commit migration error rolls back the atomic apply. After classified
reviews exist, keep the safety schema and history; do not drop it to roll back UI
or restore a signing-enabled API. Any rollback/recovery requires its own reviewed
plan and authority. No deletion, revocation, key changes or replay is implied.
