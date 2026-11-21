# Requestor review outcomes

Local implementation; production acceptance is pending separately approved database,
API and Expo releases. No production execution or Stage 2B PASS is claimed here.

`GET /v1/requests/{request_id}/review-outcomes` requires a signed requestor access
token. The persisted principal must also be active, have role `requestor`, and own
the exact request. Unknown, foreign and inactive-account requests receive the same
404 response. Invalid/missing token is 401; other token roles are 403. Responses use
`Cache-Control: private, no-store`. Database/configuration failure is a sanitized 503,
never an invented empty outcome.

Example contract (synthetic identifiers):

```json
{
  "requestId": "11111111-1111-4111-8111-111111111111",
  "requestStatus": "ATTESTATION_PENDING",
  "requestVersion": 3,
  "state": "CURRENT",
  "reviewContext": "SIMULATED_UAT",
  "outcomes": [{
    "checkId": "employment.company_and_dates",
    "reviewId": "22222222-2222-4222-8222-222222222222",
    "reviewBasis": "CLAIM_ONLY",
    "decision": "APPROVED",
    "reason": "Claim is correct.",
    "reviewedAt": "2026-11-18T01:28:01Z"
  }]
}
```

Each required, nondeleted check returns its bound decisions, including uploaded
evidence decisions. A check with no recorded bound decision returns one `PENDING`
item with null review ID, basis, rationale and date. `APPROVED` and `REJECTED` share
the same disclosure and rendering rules; neither changes request status.

`NOT_BOUND` returns no outcomes. `STALE` withholds outcomes when request version,
snapshot digest, subject or employer differs from the immutable binding.
`UNAVAILABLE` withholds outcomes for revoked bindings, unclassified contexts and
cases outside OPEN/CLOSED. Noncurrent states return null `reviewContext` and an
empty array. Version increments after issuance also cause STALE; this endpoint
does not present historical-version decisions as decisions on the current version.
Unbound legacy reviews are outside this disclosure contract.

Only reviews attached to the required check and exact binding/digest are exposed.
Claim-only decisions require SIMULATED_UAT; uploaded-evidence decisions must also
match their stored evidence digest and case/check. Claimant-authored reviews are
excluded. Rationale is disclosed to the owning requestor; reviewer identity,
reviewer grants, assignment metadata, claim contents, document metadata/storage
keys and signed artifacts are not returned.

The handler uses an enforced read-only repeatable-read transaction. Migration
000022 adds only a fixed-search-path, STABLE SECURITY DEFINER reader, strips
inherited/PUBLIC EXECUTE, and grants EXECUTE to the existing bounded context API
recipients. No new table privileges, reviewer grants, roles or records are created.
The migration is necessary because the runtime cannot SELECT the review ledgers.
It deliberately does not call reviewer authorization/context functions, which
require reviewer grants and take row locks.

The existing requestor claim detail screen shows a purple-accented Review outcome
section between lifecycle status and claim fields. Decisions use text labels as
well as color, with rationale and a local-time review date. SIMULATED_UAT is
explicitly unsigned/non-authoritative and not employer verification or a signed
credential. AUTHORITATIVE identifies the review context and states that the
decision alone does not verify the claim or issue a credential. Empty, pending,
loading, stale, unavailable and sanitized retryable error states are separate.

Outcomes reload when detail gains focus and on retry. Late responses are discarded
after blur, request/account change or effect cleanup. Outcome data never updates
the lifecycle status; a version mismatch with loaded detail requires reopening
the claim. Review reading does not require reviewer access or provide actions.

## Validation and release order

Run `npm ci` at repository root and `npm ci --prefix apps/evt-ui-expo` using the
existing lockfiles, then `npm run test:ci`, `npm run test:requestor-review-ui`, and
`npm run typecheck:expo`. Main CI installs existing Expo dependencies for the actual
React component rendering tests; no new package is required. Native host primitives
are adapted to HTML hosts in those tests; device/browser visual acceptance remains
separate. Go integration tests use only the existing guarded synthetic PostgreSQL
fixture via `CVERA_AUTHORITY_TEST_DATABASE_URL`; run `go test ./... -count=1
-timeout=120s` and `go vet ./...` from `apps/evt-api-go`.

After explicit deployment approval:

1. Follow the existing backup/restore and production schema compatibility gates.
   Review and apply only `migrations/000022_requestor_review_outcomes.sql` as the
   trusted schema owner in one bounded transaction after 000021. Verify function
   ownership, fixed search path and runtime-only EXECUTE; do not seed data or add
   direct table/reviewer privileges. This migration is once-only.
2. Release the reviewed API and Expo revisions through their existing deployment
   paths. Before the reader exists, the new endpoint fails closed with 503; an old
   API returns 404, which the new frontend presents as unavailable/retryable.
3. Sign in through the normal production application as the original requestor.
   Select Refresh, reopen the existing submitted claim, and use Request information
   to confirm its request ID. Do not create, edit, submit or approve anything.
4. Confirm the Review outcome section shows the existing APPROVED decision,
   original rationale and review date, with the unsigned/non-authoritative
   simulation label. Confirm lifecycle status is still ATTESTATION_PENDING.
5. Leave and reopen the claim to confirm independent fresh retrieval. In the
   browser network panel, the requestor's normal GET should return CURRENT,
   SIMULATED_UAT, the existing review ID and version 3. Do not copy access tokens.
6. Preserve the case-scoped response/UI observation for Stage 2B acceptance and
   confirm the Stage 2 issuance boundary remains unchanged using the previously
   approved read-only case query. No repeat approval or production rejection test.

Rollback retains 000022 and all records; revert application changes through the
normal approved release path. The additive reader need not be dropped.
