# Controlled evidence-backed issuance release: preparation only

**BLOCKED: real employer delegation is not yet accepted.** The owner has supplied
NorStar Systems LLC, California B20250427754, as the candidate; internal employer
reconciliation, representative evidence, signed delegation and independent
adjudication remain outstanding. See [NorStar pilot preparation](norstar-authority-pilot.md).
Do not create a demonstration authorization, activate a production credential,
perform live signing, or claim production acceptance. No production operation is
authorized by this document. Continue code/tests/CI independently.

## Exact proposed operations for separate owner authorization

After an authorized employer representative attests to delegation, present the
pinned final commit/hashes, real employer identifier, canonical issuer
`https://cvera.app`, claim `employment.role`, independent adjudicator/actor and
opaque evidence/basis reference to the owner. Keep private documents local.
The following scope needs one explicit bounded production authorization:

1. Established Droplet root console only: verify deployed main, current 000018
   catalog, restricted runtime role/search path, fresh private evt backup and
   isolated 64 MiB restore rehearsal, retained current API image/configuration.
   No SSH credential creation, private-key export or role/credential changes.
2. Rehearse actual-schema 000019 plus `evidence-issuance-privileges.sql` rollback
   and commit on that restore. Runtime receives **five EXECUTEs only**; table
   DML/SELECT and authority mutations remain owner-only. Rehearse a fully local
   issuance/verification with explicit local key/authority fixtures; those are
   never applied to the live database or used as production acceptance proof.
3. Apply the same additive 000019 and bindings in one bounded READ COMMITTED
   owner transaction; verify exact owners, immutable triggers, stripped default
   ACLs/PUBLIC, no active credentials/issuance rows and unchanged parent/role ACLs.
   Record actual source/hash/transaction privately. Do not fabricate migration
   history, assume 000001–000013 were applied, or add an automatic migration runner.
4. Inspect the authority catalog. If all four authority tables/functions are
   absent, separately restore/rehearse the existing **000002–000013** SQL in
   order, without 000001 or fake version rows. Strip inherited non-owner ACLs
   from these tables/functions; do not grant runtime direct access or allocator
   EXECUTE. The only runtime read is the new coherent definer function. Partial
   or unexpected existing authority schema is a STOP requiring compatibility
   review, not permission to overwrite/backfill history. Empty tables establish
   no authority. Normal API remains fail-closed with empty authority.
5. Record the real employer's authorization, independently attributable
   adjudication and lifecycle using the accepted repository/recording protocol:
   authorization, adjudication, SUBMIT, APPROVE, ACTIVATE. Each evidence row uses
   its own transaction/allocated recording position; **do not combine those five
   inserts into one transaction**. Intermediate incomplete views fail closed.
   Evidence, approved actor and actual timestamps must be real and attributable;
   never mark `conditions_satisfied` from test convenience. Confirm the existing
   complete projector/evaluator reports ACTIVE for only the approved exact scope.
6. Bind the existing production public credential explicitly to canonical Cvera
   using `register-evidence-issuer-credential.sql` in an owner transaction.
   Compare the committed PUBLIC key with the stable published JWKS and the
   service's configured kid without reading the private key. Existing expected
   public SPKI SHA-256 is
   `35fabf72cfd7dfa88f41aa7a40d7a506966b6ddd797a54c98bd1cd3cf9ec007c`
   for `issuer-prod-2026-10`; recheck against deployed public material first.
   Owner must explicitly supply credential valid_from/valid_until, positive
   claim lifetime, attribution and basis reference. The SQL has no defaults and
   validates the exact PEM DER fingerprint. The runtime additionally verifies
   RSA size/exponent, kid, issuer and exact in-memory signer public-key match.
   No replacement key, password, file mount or runtime role is needed.
7. Use reviewed PR squash merge, main CI and existing exact-main deployment.
   Verify health/version/JWKS continuity and fail-closed negatives before the
   separately approved controlled live acceptance. No silent fallback to legacy
   attestation is permitted for bound requests.

The production release driver and exact console command must be pinned and
restore-validated for the final approved schema/authority state before execution.
This document is a scope/prerequisite record, not an executable migration runner.
Do not execute snippets from feature development against production.

## One bounded normal-API acceptance

Use two distinct existing normal accounts entered only at local hidden prompts:
requestor subject/uploader and independent authorized HR/reviewer. No identity
creation, role alteration or direct request-status updates. Confirm the approved
real employer exists with supported AUTO dispatch; do not change dispatch mode.

The subject creates/submits exactly one explicitly approved synthetic request
through normal `/v1/requests` routes. Its subject is the authenticated canonical
principal; its title/claims are visibly controlled acceptance data, never a claim
about an unrelated real employee. Normal dispatch must reach ATTESTATION_PENDING.
The operator binds a new empty case and explicit required check to that exact
request, with attributable upload/review/HR assignments only as separately approved.
Before credential activation/signing, verify there is no other eligible bound
pending request within the newly established authority scope and no other ACTIVE
scope; the runtime cannot create bindings or authority. Stop if that bounded
precondition is not met rather than activating general production issuance.

Upload a tiny public synthetic file, complete/hash it, and independently approve
its exact binding/claim context. Keep scan PENDING. Confirm eligibility is false
for unrelated principal/employer/request or unavailable authority, and true only
for the approved authoritative context. One authorized HR call issues the exact
credential; no privileged key use outside the service. The subject retrieves its
own committed bytes through `/v1/requests/{id}/credential`; cross-subject access
denies. Keep tokens, URLs, evidence and artifact contents out of logs/reports.

Independently verify with public-key material and trusted current resolver,
expected canonical subject and the explicit manual pilot policy. Require all
signature, issuer/key binding, employer authority, snapshot/manifest/context,
status and policy dimensions; verify modified bytes, wrong subject/policy,
manifest mismatch and cross-request negatives. Report cryptographic manifest
integrity separately from trusted evidence-review resolution. Capture only
sanitized PASS/STOP flags, revision and artifact ID/reference privately.

As explicitly authorized cleanup, revoke the exact synthetic issued claim and
append real REVOKE/SUSPEND to the test-only authority lifecycle as authorized,
then require the unchanged artifact to fail trust despite valid signature. Revoke
fixture grants/binding and close its case; log out both accounts. Preserve signed
request/credential/audit receipts. Do not fabricate an unsigned rejection of an
already issued request, delete application records, reset identities or erase
history. Retain PENDING synthetic evidence until separately approved deletion.

## Recovery and containment

- Before credential registration/real ACTIVE authority, new issuance fails closed;
  leaving additive tables and reverting only the API to e84b9a6 is compatible.
- With an issued credential, stop further issuance by an attributable authority
  suspension/revocation under owner approval; revoke the exact claim if necessary.
  Revoke a signing credential only for a credential-level decision, not as a
  pretend reversal of bytes already signed. One-way revocation is irreversible;
  reactivation/new delegation is a new explicitly approved authorization.
- Keep the new guarded verifier available to report revocation. An older API
  cannot independently verify these artifacts and must not be described as a
  successful functional rollback. It still blocks legacy signing for bound
  requests. Do not roll back further than e84b9a6 while any bound pending requests
  exist. Do not drop new audit tables, restore stale database data or re-enable
  legacy credentials to make recovery appear successful.
- Any ambiguous COMMIT, failed backup restore, unexpected catalog/privilege/data
  state or failed acceptance is STOP. Preserve private intent/results and use a
  compatible correction or owner-directed recovery; never blindly rerun DDL,
  issuance or fixture provisioning.

## Current disposition

Engineering implementation/tests are independent of real employer approval.
Production authority setup, credential activation and live signing remain blocked
until that prerequisite exists and the owner approves the exact final procedure.
Baseline #140 synthetic acceptance is pending/unconfirmed; it is neither rerun
nor claimed PASS here. Native picker/device acceptance remains separate.
