# NorStar production readiness and first genuine issuance

As of **2026-11-10**. This is documentation and operational preparation, **not
execution authorization**. No production connection or operation was performed
for this reconciliation. Owner-confirmed observations below are distinguished
from repository/GitHub delivery evidence and from still-missing acceptance.
Keep completed inputs, identities, documents, credentials and receipts private.

## Current baseline and evidence

| Layer | Confirmed state | Evidence and limits |
|---|---|---|
| Employer configuration | `norstar-systems-llc`, `MANUAL`; registration complete, transaction `3007` | Owner-confirmed. **Never repeat registration** or infer a new mapping from an old candidate search. |
| Organizational issuer authority | `ACTIVE` for `https://cvera.app / norstar-systems-llc / employment.role`; activation transaction `3019` | Owner-confirmed. Authorization `1ace0869-272c-4187-8928-1d7613b67269`. **Never repeat activation.** Fresh current authority is still evaluated at issuance/verification. Activation timestamp/event ID and full receipt are not supplied in this reconciliation: **UNVERIFIED** here. |
| Evidence review delivery and synthetic acceptance | API deployed; independent two-account synthetic acceptance **PASS** at `6e7293e` | [Recorded release evidence](evidence-review-release.md). This does not establish genuine NorStar document review or Tara's participation. |
| Integrated issuance/independent verifier | Implemented; PR #141 merged; implementation deployment owner-confirmed | [PR #141](https://github.com/kola-white/rsa-attestation-engine/pull/141) merged as `bce7b9833885b69b94a2f7d6cf85d415cbc6ebac`; [main CI/deployment run 42534540050](https://github.com/kola-white/rsa-attestation-engine/actions/runs/42534540050) succeeded for that SHA. GitHub metadata was checked for this reconciliation. The **current November 10 live SHA is UNVERIFIED**; do not substitute local `main` or a documentation commit for `/version`. |
| Public issuer credential binding | `registered=false`, `active=false` for `https://cvera.app`, `issuer-prod-2026-10` | Owner-confirmed November 10 read-only check. A deployed signer/public JWKS is not a registered issuer credential. |
| NorStar HR authorization | Eligible active `hr_reviewer` principals with unrevoked NorStar assignments: **0** | Owner-confirmed November 10 read-only check. Role alone, authority activation and case grants confer no employer assignment. |
| Genuine case evidence and issuance | Tara's independent review **PENDING**; first genuine evidence-backed NorStar issuance/verification **PENDING** | No genuine acceptance PASS, signed credential or completed Tara review is claimed. |
| Other acceptance | PR #140 two-account/two-request binding smoke **UNCONFIRMED**; native picker/device acceptance remains separate | Preserve earlier records; neither is silently marked PASS or rerun by this document. |

Historical authority recording stopped at APPROVED_NOT_ACTIVE before the separately
completed activation. Preserve its authorization/adjudication/SUBMIT/APPROVE
receipts and append-only history. First-party organizational adjudication is
permitted; it never waives the separate case-evidence no-self-review requirement.
The [old preactivation sequence](norstar-preactivation-sequence.md) is historical,
not a work queue to replay. Schema/bootstrap and release preparation in
[evidence-backed issuance release](evidence-backed-issuance-release.md) are also
historical; inspect existing deployed objects instead of replaying once-only DDL.

## Ordered STOP/GO checklist — four separately approved stages

Each stage needs its own explicit owner scope/approval reference. This document
supplies none. Use the existing owner-controlled Droplet console, reviewed pinned
artifacts and protected inputs; do not create new runtime privileges, accounts,
roles, schema or signing keys to satisfy a check. No secret belongs in command
arguments, logs, Git or a public receipt. Reconfirm current state when an approved
operation is actually scheduled; November 10 observations are not durable grants.

### 1. Register the existing public issuer credential

**Prerequisites / owner inputs.** Review the deployed implementation revision,
existing schema/bindings, current protected recovery material and the absence of
this issuer/kid registry row. Fix actual `approval_ref`, `established_by`,
`basis_ref`, `valid_from`, `valid_until` and integer `claim_lifetime_seconds` in a
root-owned, non-symlink **0600** private JSON file; keep its directory protected.
No example dates or lifetime defaults are approvals. `valid_from` must already
be effective, `valid_until` future and later than `valid_from`; lifetime must be
positive and at most 31,536,000 seconds. Authority's effective start is not an
instruction to backdate credential registration.

**Authorization gate.** Owner approves this exact canonical public credential,
validity, lifetime, attribution and evidence basis. Registration makes the existing
issuance path usable when all other gates pass; it is not an inert registry edit.
It neither issues a claim nor authorizes later signing by itself. Do not repeat
NorStar organizational activation or operate on a private signing key.

**Existing procedure / entry point** (for later approved execution only):

```sh
python3 register-approved-issuer-credential.py \
  --approved-public-registration /path/to/actual-approved-private-input.json
```

Stage this [existing driver](../../apps/evt-api-go/operations/register-approved-issuer-credential.py)
beside [issuance-acceptance-support.py](../../apps/evt-api-go/operations/issuance-acceptance-support.py)
from the reviewed revision; retain file hashes in the private approval record.
The driver pins [register-evidence-issuer-credential.sql](../../apps/evt-api-go/operations/register-evidence-issuer-credential.sql)
to source `973fb3bb2ab9c80dc965b27458f1df85413f4dee` and SHA-256
`975f5621056d542f397b9501fb8b0f6e010d4b2f6a6a664db5fc52e7901312e5`.
That commit/object must be available locally; do not replace the SQL or its pin.
It executes one owner READ COMMITTED transaction through container-local psql,
with bounded timeouts, fixed schema and transaction-local settings. Runtime and
evidence/HR operators cannot register a credential.

**Public consistency checks.** The driver fetches only the stable public JWKS
`https://hapis.sfo3.cdn.digitaloceanspaces.com/trust/jwks.json` and reads committed
`keys/public/issuer-prod-2026-10.pub.pem`. The support routine requires exactly one
matching kid, RSA/RS256/signing metadata, at least 3072 bits and exponent 65537;
JWKS-derived DER SPKI and the committed public PEM must agree. Pinned SPKI SHA-256:
`35fabf72cfd7dfa88f41aa7a40d7a506966b6ddd797a54c98bd1cd3cf9ec007c`.
The SQL independently checks the public PEM digest. No private key is read,
exported, generated or rotated. Actual in-memory signer matching is separately
required by issuance; these public checks alone do not prove a live signature.

**Read-only verification / success.** The driver checks before writing that the
row does not exist, creates an exclusive `.credential-registration` private intent
directory, and after COMMIT reads issuer, kid, SPKI, unrevoked status and current
validity. Success is `public_credential_registration:PASS` with the actual
transaction and private `result.json`. The existing post-registration query is:

```sql
BEGIN READ ONLY;
SELECT jsonb_build_object(
  'issuer',issuer_id,'kid',kid,'fingerprint',spki_sha256,
  'unrevoked',revoked_at IS NULL,
  'current',clock_timestamp()>=valid_from AND clock_timestamp()<valid_until)
FROM public.evidence_issuer_credentials
WHERE issuer_id='https://cvera.app' AND kid='issuer-prod-2026-10';
ROLLBACK;
```

Run only through a separately approved owner connection; never grant runtime raw
registry SELECT. The schema has no `active` boolean column: the November 10
`active=false` observation means no currently usable registration, not a field to
UPDATE. Current usability requires registration, validity and no revocation.

**STOP:** missing approval/metadata, existing row or intent directory, public-key/
JWKS/SPKI mismatch or unavailable JWKS, wrong owner/schema, invalid bounds,
transaction error, ambiguous COMMIT or failed postcheck. The driver emits sanitized
`STOP` with phase; preserve intent/receipt and reconcile read-only. Do not retry,
reactivate, overwrite a row, rotate keys or sign to diagnose it.

### 2. Establish an eligible, employer-authorized HR principal

**Prerequisites / owner inputs.** Privately identify a real authorized HR actor
through normal login and API exchange and reconcile the unique Kratos link to
canonical `domain_users.id`. Reuse an existing principal only if its stored status
is `active` and its stored role is exactly `hr_reviewer`; inspect the exact
NorStar assignment and existing revocation history. Do not select by an untrusted
caller-supplied email/ID assertion or infer HR power from a case grant.

[Auth exchange](../../apps/evt-api-go/internal/httpapi/auth_handlers.go)
(`issueAuthExchangeResponseForKratosIdentity`) creates a new domain principal as
`requestor`, preserves an existing role/status and rejects inactive accounts.
Normal registration cannot select HR. The schema's legacy default or the unused
helper default is not the actual exchange policy. If no suitable HR principal
exists, the supported start is normal account registration/login/exchange and an
explicit owner decision selecting a legitimate HR actor. The repository exposes
**no self-service role-promotion endpoint or dedicated role-provisioning utility**.
The precise production account-role provisioning procedure is **UNVERIFIED**;
stop and obtain its separately approved operator procedure rather than inventing
a promotion SQL command or changing the subject/requestor account to pass a test.
The acceptance client requires subject and HR principals to differ. Tara may also
act as HR only if genuinely eligible and separately authorized; no third human is
required solely because the review and HR permissions are distinct.

**Authorization gate.** Owner approves the exact existing eligible principal,
`norstar-systems-llc`, actual approving actor, organizational mandate and opaque
`grant_basis_ref`. Grant administration remains restricted to the existing
`cvera_hr_operator`; no LOGIN/membership/runtime privileges are added.

**Existing procedure.** [employeraccess.Grant](../../apps/evt-api-go/internal/employeraccess/assignments.go)
and [cmd/hr-assignment](../../apps/evt-api-go/cmd/hr-assignment/main.go) require a
separate administrative connection, READ COMMITTED, exclusive principal locking,
active HR role/status, existing employer, immutable provenance and a fresh grant
instance. The CLI shape is `-action grant -principal <canonical UUID> -employer
norstar-systems-llc -actor <actual actor> -basis <approved reference>` with private
`CVERA_HR_ADMIN_DATABASE_URL`. Do not assume that DSN or a CLI binary is installed
on production, or try to log in as the NOLOGIN operator role.

The existing root-console equivalent, also used by the acceptance client, uses
`SET LOCAL ROLE cvera_hr_operator` in an owner-opened transaction. Completed psql
variables below must come from the approved private input; this is a documented
procedure, not an instruction to execute now:

```sql
BEGIN ISOLATION LEVEL READ COMMITTED;
SET LOCAL search_path=pg_catalog,public,pg_temp;
SET LOCAL lock_timeout='5s';
SET LOCAL statement_timeout='30s';
SET LOCAL ROLE cvera_hr_operator;
SELECT public.lock_employer_hr_principal(:'hr_principal'::uuid);
INSERT INTO public.employer_hr_assignments
  (assignment_id,principal_id,employer_id,granted_by,grant_basis_ref)
VALUES (gen_random_uuid(),:'hr_principal'::uuid,'norstar-systems-llc',
        :'actor',:'basis')
RETURNING assignment_id;
COMMIT;
```

The existing trigger locks/checks the principal and prevents grant reassignment
or reactivation; the partial unique index forbids duplicate active assignments.
Recheck/reuse an already approved unrevoked assignment instead of repeating INSERT.
Capture the assignment UUID, actual transaction, actor and basis privately.

**Read-only verification / success.** Using the owner connection and a privately
resolved `hr_principal`, inspect only eligibility flags:

```sql
BEGIN READ ONLY;
SET LOCAL search_path=pg_catalog;
SELECT jsonb_build_object(
  'active_hr',EXISTS(SELECT 1 FROM public.domain_users
    WHERE id=:'hr_principal'::uuid AND status='active' AND role='hr_reviewer'),
  'eligible',EXISTS(SELECT 1 FROM public.domain_users u
    JOIN public.employer_hr_assignments a ON a.principal_id=u.id
    WHERE u.id=:'hr_principal'::uuid AND u.status='active'
      AND u.role='hr_reviewer' AND a.employer_id='norstar-systems-llc'
      AND a.revoked_at IS NULL));
ROLLBACK;
```

Both must be true. This is a snapshot, not an authorization lease: the API rechecks
under transaction locks on every protected action. The bounded authorization
function itself takes row locks and is not a READ ONLY diagnostic query.

**STOP:** unresolved role provisioning, inactive/wrong-role principal, missing
mandate, scope mismatch, duplicate/conflicting grant, privilege/locking failure,
ambiguous COMMIT or failed verification. Do not broaden privileges or substitute
organizational issuer authority for HR access. Revocation uses the existing
operator lock and attributable revocation fields, never DELETE/reactivation.

### 3. Establish Tara's independent review participation and access

**Prerequisites / owner inputs.** Tara personally registers or recovers her own
account through the existing Cvera application, completes the offered mailbox
verification flow, then performs normal password login/exchange. Supported browser
entry points are `/register`, `/verify`, `/recovery` and `/login` on
`https://mvp.cvera.app`; [Kratos configuration](../../infra/kratos/kratos.yml),
[RegisterScreen](../../apps/evt-ui-expo/screens/RegisterScreen.tsx) and
[VerifyScreen](../../apps/evt-ui-expo/screens/VerifyScreen.tsx) define the flows.
Do not share credentials or insert/restore identities in PostgreSQL. Verify the
active identity/session and unique canonical domain principal privately, and
confirm that Tara is another real human authorized to inspect these documents.
A successful login alone is not proof of verified-email metadata. If verification
is unavailable/not returned, label that result **UNVERIFIED**, resolve through the
supported flow and do not fabricate verified status or edit identity rows. This
is not a new verified-email requirement imposed on all legacy accounts.

**Authorization gate / procedure.** Owner approves only Tara's canonical principal,
actual case, attributable granting actor/basis and genuine review scope. Existing
`cvera_evidence_operator` invokes
`public.grant_evidence_review(principal_uuid, case_id, actor, basis)`; no HR role
or employer assignment follows. Tara's registration, verified control and grant
are **not assumed complete**. Until the stage-4 case exists and is exclusively
bound, stage 3 can resolve the person and approve the intended scope only; do not
create a placeholder case or blanket grant. The acceptance client provisions the
exact approved grant after case/binding creation, before evidence review.

**Read-only verification / success.** Normal authenticated
`GET /v1/evidence/checks` must show Tara's exact authorized case/check with
`canReview`; subsequent list/download must resolve the intended immutable request,
subject, employer, issuer, binding ID and claim digest. The authenticated API
performs locks/checks; these calls make no review decision. Preserve sanitized
verification flags and private canonical IDs. Upload grant alone is insufficient.

Tara personally inspects the canonical downloaded bytes and bound claim context,
then records her actual APPROVED or REJECTED decision and nonempty rationale via
`POST /v1/cases/{caseId}/checks/{checkId}/evidence/{uploadId}/review`, including
`bindingId` and `claimDigest` for the bound request. Every required check needs
independent contextual approval. Exact retry is idempotent; conflicting final
review is denied. No self-review, automated affirmative answer, synthetic document
or second account controlled by the uploader substitutes for Tara's decision.

**Trust / STOP.** Keep scan status **PENDING** visibly distinct from manual
APPROVED under `MANUAL_REVIEW_PILOT_V1`; it is not malware clearance. INFECTED/ERROR,
missing scan work, unreviewed/disqualifying evidence, inactive identity, revoked
grant, closed case, wrong context, missing verification result or lack of another
human is STOP for the applicable step. Genuine rejection stops signing. No review
or genuine NorStar evidence acceptance has yet been reported.

### 4. One genuine request, issuance and independent verification

**Prerequisites / owner inputs.** Stages 1–3 must be resolved to the extent needed
before each action. Verify current canonical authority is ACTIVE, the approved
public credential current/unrevoked, exact HR eligibility, genuine evidence hashes,
Tara's availability and real deployed `/version`. Preserve approved backup/recovery
references; their current availability/freshness is **UNVERIFIED** in this
reconciliation. Do not rerun old schema/bootstrap or activation to refresh them.

**Authorization gate.** Owner separately authorizes the one genuine request and
truthful snapshot, exact people, new case/check IDs, upload/review grants, bounded
NorStar AUTO dispatch window with MANUAL restoration, one signing attempt and the
explicit retain-versus-revoke choice. Credential registration or ACTIVE authority
alone is not this approval. Keep any stage-2 assignment outside this operation if
already granted; do not authorize a second assignment implicitly.

**Existing entry point**, not executed here:

```sh
python3 smoke-evidence-issuance.py \
  --approved-live-acceptance /path/to/actual-approved-private-input.json
```

Stage [the client](../../apps/evt-api-go/operations/smoke-evidence-issuance.py)
with `issuance-acceptance-support.py`, `smoke-evidence-review.py` and
`smoke-evidence-completion.py` from the same reviewed deployed revision. Root-only
0600 JSON and genuine files must supply all fields listed in the
[release runbook](evidence-backed-issuance-release.md#one-genuine-normal-api-acceptance-not-another-synthetic-claim),
including actual `expected_main`, subject/reviewer/HR UUIDs, authorization/evidence
reference, exact case/checks, snapshot and `revoke_claim_after_verification`.
No personal completed input belongs in Git. Keep the title `Managing Member — owner`;
do not infer payroll status or dates. Accepted evidence is genuine PDF/PNG/JPEG,
within 5 MiB and matching its approved SHA-256. Do not relabel Markdown/RTF as PDF;
keep any authentic review-copy provenance/integrity mapping privately.

The existing client performs this sequence:

1. Normal subject/reviewer login/exchange; separate HR login unless Tara already
   has the independently approved HR role/assignment. Match all canonical IDs.
   Verify deployed revision, clean checkout, health, configured signer path and
   public-key/JWKS consistency without reading the private key.
2. Create exactly one normal subject request in MANUAL. Explicitly approved AUTO
   is opened only around normal submit, reaching ATTESTATION_PENDING, and MANUAL
   is restored in `finally`. There is no manual-dispatch API or direct status patch.
   Because AUTO is employer-wide, check for other affected submissions/requests
   before the window; the client also rejects another pending NorStar request
   before signing. An unexpected concurrent submission is STOP, not permission
   to sign it or modify its state.
3. Operator provisions the approved empty case/checks and exclusive request/case
   binding with exact required checks, then subject upload and Tara review grants.
   No old upload/review is adopted and no evidence crosses request bindings.
   Reuse the existing approved HR assignment. The tool can create an absent exact
   assignment, but that requires explicit scope; under this staged plan a missing
   stage-2 assignment is STOP before starting, not implicit permission to use that
   fallback. Do not run the tool without verifying this prerequisite.
4. Upload genuine bytes, complete to sealed private storage, verify canonical hash
   and PENDING scan. Tara personally downloads/inspects each canonical attachment
   and immutable context and records her actual decision/rationale. Rejection stops.
5. Eligibility must report current evidence, `AUTHORIZED` authority for the exact
   authorization, matching immutable manifest/digest, correct manual policy,
   PENDING scans and `signingEnabled=true`. The flag is a current eligibility
   result, not a new environment switch to turn on. The endpoint rechecks locks,
   authority, credential, binding/resources and HR assignment transactionally.
6. Persist durable issuance intent **before the single positive**
   `POST /v1/employer/requests/{request_id}/issue-evidence`. Anonymous/requestor
   denial probes precede it. One request issues once; do not retry on timeout,
   disconnect, duplicate/conflict, STOP or an ambiguous response.
7. Subject retrieves its own exact committed artifact from
   `GET /v1/requests/{request_id}/credential`. Check cross-subject denial and retain
   the artifact privately. Independently verify public RS256 signature and JWKS
   binding, canonical issuer/subject/employer/claim, request/binding/version,
   exact snapshot and evidence-manifest SHA-256 and authority commitment.
8. `POST /v1/verification/outcome` with the exact expected subject and
   `employment.authoritative/manual-evidence-pilot/v1` must return trusted VERIFIED,
   valid signature/validity/status/manifest/review and `AUTHORIZED` current authority.
   Wrong subject/policy and tampered signature must deny trust. If explicitly chosen,
   revoke only this claim and prove trust denial despite unchanged valid signature;
   otherwise verify current non-revoked status and report revocation testing as
   NOT_EXECUTED, never PASS. Trusted registry/review resolution remains necessary;
   a signature or manifest hash alone does not prove review facts or current status.

**Receipts / success.** Preserve the root-private input, `.acceptance/state.json`,
actual request/binding/upload/grant/credential references, expected/deployed SHA,
actual human decision, private artifact and sanitized `issuance_acceptance:PASS`
output. The script reports the chosen revocation outcome and PENDING scan policy.
Only actual returned production results can change this baseline to genuine PASS.

**STOP / recovery boundaries.** Any missing approval, unexpected existing intent,
wrong revision/identity/authority/credential, altered context/bytes, unavailable
trusted source, denied/rejected review, extra pending request or failed cleanup
stops. Preserve committed records and reconcile the existing subject artifact
read-only; never restart with a new config/case to conceal an uncertain attempt.
MANUAL restoration and temporary-access/session cleanup must be verified. Only
new grants/assignments created by the tool are revoked; pre-existing stage-2 HR
assignment is retained. Authentic evidence, case/binding, request, authority and
audit remain. Do not close/revoke a retained credential's binding as fixture cleanup.
Do not restore stale production data, drop tables/history, downgrade past the
binding guard or discard the verifier required for issued claims. Authority
containment or claim/credential revocation needs its own included or separate
explicit scope. No destructive deletion or key operation is implied.

## Gap assessment

| Kind | Actual gap / next action |
|---|---|
| Missing operational configuration | Public issuer/kid registration/current validity; eligible NorStar HR principal plus unrevoked assignment. Separately approve stages 1–2. These observations do not demonstrate code defects. |
| Missing human participation/approval | Tara's account/verification and genuine independent decision; exact HR selection/mandate; each stage's bounded owner approval and live retain/revoke choice. |
| Missing evidence/verification | Genuine request/review/issuance/independent-verifier receipt; current live SHA; current recovery readiness; exact production role-provisioning procedure if no existing HR qualifies. Historical #140 smoke and native picker/device acceptance remain separate UNCONFIRMED/PENDING checks. |
| Implementation defects | **None demonstrated by this documentation reconciliation.** Lack of a role-provisioning utility is an operational procedure gap, not proof of an authorization defect. Any actual incompatibility must be reported without implementing or bypassing it here. |

No new schema, automation, scanner, infrastructure or administration feature is
proposed. This checklist does not reopen completed registration, activation,
evidence-pilot acceptance or the owner-closed Weekend 2 baseline.
