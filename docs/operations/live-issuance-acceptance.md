# Deferred live issuance acceptance: prepared, not authorized

The owner deferred positive live current-key signing proof when closing Weekend 2.
That decision remains unchanged. This plan does **not** authorize production
issuance, role changes, new employer assignments, direct data updates or use of the
private signing key outside the running service. Do not execute it until the owner
specifically selects and authorizes the request and its legitimate claims.

## Existing path and exact prerequisites

`internal/evt/routes.go` exposes candidate, employer, internal and recruiter routes.
`CandidateHandlers.CreateDraft` binds `candidate_personid` to authenticated
`claims.Sub`, the canonical `domain_users.id`. Employer attestation requires the
HRReviewer role plus a current explicit employer assignment; the request must
belong to that same employer and be ATTESTATION_PENDING.

Use supported account login/exchange with credentials entered locally and held in
memory. Select a legitimate employer and an independently authorized reviewer;
never infer permission from a caller-supplied ID or a case-review/upload grant.
Select an eligible existing request if available. Otherwise obtain explicit
permission for one candidate request and use normal `POST /v1/requests`, then
`POST /v1/requests/{request_id}/submit`. AUTO dispatch reaches
ATTESTATION_PENDING; MANUAL remains SUBMITTED. Stop if normal dispatch does not
provide the eligible state. Do not force it with SQL or change employer settings.

## One controlled issuance and public-key proof

After that separate authorization, the assigned reviewer uses the normal
`POST /v1/employer/requests/{request_id}/attest` with the exact approved
`employer_id`, `response_type` (FULL_MATCH or PARTIAL_MATCH) and truthful approved
`response_body`. `Repo.EmployerAttestServerSigned` signs inside the service, stores
the compact JWS and returns status/presence/kid metadata; the API does not return
the compact artifact. Reject responses do not sign. Record no sensitive body,
JWT, session token, document, URL or key contents in logs or issue comments.

The owner may then use an exact-request, read-only private lookup of
`evt_requests.attestation_jws` for that single approved UUID. No broad record
export or private-key access is needed. Verify in memory or a protected temporary
file using only the published JWKS and committed
`keys/public/issuer-prod-2026-10.pub.pem`:

- Protected header has `alg=RS256`, `kid=issuer-prod-2026-10` and `typ=JWT`.
- Published RSA modulus/exponent match the committed public key; verify SHA-256
  PKCS#1 v1.5 over the exact compact protected-header/payload segments.
- Parsed payload matches the specifically approved request, employer, canonical
  HR principal, response type/body and server issuance time. The existing schema
  uses `RequestID`, `EmployerID`, `HRPersonID`, `ResponseType`, `ResponseBody`
  (base64-encoded byte slice) and `IssuedAtUnix`. Do not silently expect lower-case
  JWT claims or add claims in the acceptance test.
- A changed payload and a changed signature both fail. Reject other kids,
  algorithms and historical demonstration keys rather than looking for a
  permissive verifier. Report only PASS/FAIL flags and non-sensitive revision/kid.

Log out through normal API and Kratos flows; delete private temporary artifacts.
Retain the issuance audit according to the owner's policy. Do not delete or
rewrite the issued request as smoke cleanup without separate authority.

## What this proof does not establish

A valid signature is not a trust, revocation, expiry or business-verification
verdict. This legacy signer injects no `iss`, `aud`, `iat` or `exp`; its exact
contract is in `internal/evt/attestation_signer.go`. Document that limitation rather
than asserting expiry validation. `POST /v1/internal/requests/{id}/verify` accepts
an authorized internal trust decision; it does not independently verify a
signature. Recruiter consumption records use; it is not cryptographic proof.
Neither is an appropriate shortcut for this acceptance.

The separate `/v1/verification/outcome` path must be assessed against its configured
profile/trust/status prerequisites and may report unsupported/unconfigured. Record
that result independently; never synthesize VERIFIED to obtain a green test.

Evidence cases/checks and employment requests have no authoritative association
in this MVP slice. A separate reviewer APPROVED decision leaves scan status
PENDING and does not authorize employer issuance. Before advertising
**evidence-backed issued claims**, the product owner must decide the explicit
persisted case/check-to-request relationship and which review outcomes permit
issuance. Until then, private evidence collection/manual review and existing
employment issuance remain separate workflows. No relationship is invented here.
