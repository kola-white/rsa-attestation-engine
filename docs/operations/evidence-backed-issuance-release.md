# Integrated issuance release and genuine NorStar acceptance

Engineering is implemented; production release and live acceptance are not yet
executed. NorStar registration is complete: `norstar-systems-llc`, MANUAL,
transaction 3007. **Never repeat registration.** Signed delegation, attributable
first-party adjudication and the separate human case reviewer remain owner inputs.
They do not block preparation or a separately authorized non-signing deployment.

## Executable non-signing release

`operations/release-evidence-issuance-sql.py --prepare` checks the deployed e84b9a6
baseline and actual 000018 parents, runtime role/path and absence of authority and
issuance objects. It makes a fresh root-private evt archive, retains API recovery
image/configuration, restores the same PostgreSQL image in a network-none 64 MiB
clone without shared live volumes, and rehearses rollback, injected failure (local
regression), commit, exact privileges, normal request defaults/enums/events,
contextual review, manifest preparation and issuance-receipt persistence. All
synthetic rows/public placeholders are clone-only and rolled back. No production
SQL writes, credentials, authority records, grants or service changes occur.
This is compatibility evidence, not proof of a genuine signature or delegation.

The pinned driver stages SQL from its explicit SOURCE/hash manifest, independently
of the running checkout. The required transaction is **existing 000002–000013,
owner-only ACL cleanup including the identity sequence, 000019, five runtime
EXECUTE bindings and verification**. No 000001, presumed migration history, runner,
role or version-table entries are invented. Existing/partial authority or issuance
objects cause STOP for compatibility/history inspection; they are never replaced.
Runtime gets no new table/sequence privileges or authority-writing permission.
Normal PUBLIC function EXECUTE and inherited non-owner default ACLs are removed
from the new objects in that same transaction. Parents/roles/ACLs remain unchanged.
Empty authority and credential registries cannot enable evidence-backed signing.

After production preparation PASS, obtain explicit authorization for that exact
schema transaction and non-signing PR #141 merge/deployment. Then execute:

```sh
python3 release-evidence-issuance-sql.py \
  --apply-approved-schema /opt/cvera/issuance-release-<actual-preparation-directory> \
  --approval-ref '<actual owner authorization reference>'
```

Use only reviewed, pinned artifacts in the existing Droplet root console. The
executor checks the recorded backup/rehearsal/source hashes and unchanged source
state before applying, records actual transaction/source/hashes privately and
verifies after COMMIT. A prior apply intent/receipt or ambiguous COMMIT is STOP,
not permission to retry. It never records/activates authority, registers a public
credential, deploys or signs. The separately authorized squash merge uses existing
main CI/exact-main API deployment. Verify health/version/JWKS and the unavailable
credential failure before any subsequent activation. No extra infrastructure.

## Actual authority and credential inputs; separately authorized execution

Keep completed private documents/settings outside Git. First-party adjudication
is permitted with truthful representative/subject/operator/conflict disclosure;
independent third-party authority adjudication is not a requirement. The different
human CASE reviewer remains mandatory. Use the existing signed templates and
`record-norstar-authority-approved.sql`: authorization, actual adjudication,
SUBMIT, APPROVE in **four separately allocated transactions**. Each must commit
before the next; errors preserve partial history for reconciliation. No activation
is inferred from approval, a filing number, registration, HR or case grants.

After owner approval of the real packet and exact scope
`https://cvera.app / norstar-systems-llc / employment.role`, the private input for
`activate-norstar-authority.sql` adds these session settings to the actual NorStar
recording settings:

- `cvera.norstar_activation_approval_ref`: actual explicit production approval.
- `cvera.norstar_activation_event_id`: newly reserved event ID, never reused.
- `cvera.norstar_activation_conditions_satisfied`: `true` only after actual signed
  organizational evidence, representative power, delegation and decision checks.
- `cvera.norstar_delegation_valid_from`: actual signed start, already effective.

The script locks the global recording boundary before validating exact APPROVED
history and appending one ACTIVATE in its own transaction. It rejects runtime,
missing/false conditions, wrong scope and repeats. The real evaluator still must
report current ACTIVE authority; SQL success alone is not an authority verdict.
Preserve the signed end/withdrawal policy: a date only in a letter does not install
an automatic expiry. Record lifecycle expiry/withdrawal through the accepted
mechanism at the required time; do not claim a scheduler exists.

`contain-norstar-authority.sql` is prepared owner-only recovery for separately
approved attributable SUSPEND or REVOKE, with the same actual authorization/actor/
evidence settings plus `containment_event`, `containment_event_id` and
`containment_approval_ref`. It serializes history checking and append, preserves
positive intervals and refuses invalid/terminal repeats. REVOKE is irreversible;
no reset, deletion, fabricated timestamp or pretend independent decision occurs.
Stage and hash the exact approved SQL, then execute completed private settings
and that SQL through the existing channel:

```sh
cat "$CVERA_PRIVATE_SETTINGS" "$CVERA_REVIEWED_SQL" | docker exec -i evt-api-go-db-1 \
  psql -X -U postgres -d evt -Atq -v ON_ERROR_STOP=1 -v VERBOSITY=sqlstate
```

Capture results only in a private receipt. Do not send settings, documents or raw
errors here. Recording approval does not authorize ACTIVATE or live signing.

`register-approved-issuer-credential.py --approved-public-registration <private.json>`
is separately authorized credential activation, not a key operation. Root-private
0600 JSON supplies `approval_ref`, `valid_from`, `valid_until`, positive
`claim_lifetime_seconds`, `established_by`, `basis_ref`; none has a default. It
compares the committed PUBLIC key and stable JWKS against canonical kid
`issuer-prod-2026-10`, RSA >=3072/E65537 and public SPKI SHA-256
`35fabf72cfd7dfa88f41aa7a40d7a506966b6ddd797a54c98bd1cd3cf9ec007c`,
then runs the pinned existing credential-registration SQL once. It neither opens a
private key nor changes a role/password/key mount. Existing records or ambiguous
commits STOP rather than reactivate/overwrite. Lifetimes are actual owner choices,
not invented pilot defaults. A key binding is not employer authority.

## One genuine normal-API acceptance, not another synthetic claim

`smoke-evidence-issuance.py --approved-live-acceptance <private.json>` implements the
complete workflow. It must be separately authorized at the reviewed deployed
revision, with actual active authority and credential binding already established.
It has no schema, authority or credential activation operation. Stage it beside
`issuance-acceptance-support.py`, `smoke-evidence-review.py` and
`smoke-evidence-completion.py` from the same approved revision.

Root-private 0600 JSON supplies the actual values:

| Fields | Meaning |
|---|---|
| `approval_ref`, `actor_id`, `basis_ref`, `expected_main` | Approved one-request live operation and exact deployed SHA |
| `subject_principal`, `reviewer_principal`, `hr_principal` | Canonical UUIDs from normal authentication, privately reconciled to actual people; reviewer differs from subject |
| `authorization_id`, `authority_evidence_ref` | Real recorded NorStar authority and signed packet |
| `case_id`, `claim_snapshot` | New explicit case; truthful `primary_employment.title` = `Managing Member — owner`, with only corroborated optional facts |
| `evidence` | One genuine file per explicitly required check: `check_id`, private `path`, `mime_type`, approved `sha256`; existing 5 MiB/PDF/PNG/JPEG limits |
| `revoke_claim_after_verification` | Explicit boolean owner choice: revoke this issued claim and prove trust denial, or retain it and verify current non-revoked status |

No personal values or completed examples belong in Git. The reviewer may also be
an already eligible HR actor; otherwise an existing separate HR account acts as
issuer. This is reuse of current single-role authorization, not a requirement to
make the reviewer HR or to change account roles. Hidden local prompts perform
normal login/session/exchange; credentials never leave the console for Codex.

The client creates one normal subject request while NorStar remains MANUAL, opens
only the explicitly approved brief AUTO window for normal submit, and restores
MANUAL in `finally`. It never patches request status. It provisions only the
approved case/check/binding/upload/review grants and, if absent and explicitly in
scope, that exact HR assignment through existing operator permissions. Genuine
files are uploaded/completed privately. The OTHER human personally inspects the
canonical attachment and immutable context, then supplies their actual decision
and rationale under their authenticated principal. No automated review approval,
synthetic documents or second account controlled by the uploader substitutes.

Before signing, eligibility must show current authority, evidence and key, and no
other NorStar request may be pending within the narrow pilot scope. Durable
issuance intent precedes the **single** positive HTTP call. Retries/restarts using
the same private input refuse duplicate provisioning/signing. An ambiguous result
requires read-only reconciliation of the subject's already committed artifact,
not another signing call. Tokens and URLs never enter the private state receipt.

The subject retrieves the exact portable committed SD-JWT VC. A separate public-
key/OpenSSL check establishes its RS256 signature and trusted public JWKS binding.
Canonical issuer/subject/employer/claim, request/binding/version and exact database
snapshot/manifest digests must match. The independent verification pathway (no
signer) must separately return valid signature, manifest integrity, resolved review,
current authority/status and explicit policy. Wrong subject/policy and modified
signature must deny trust. Cryptographic manifest integrity is not a substitute
for the trusted resolver's evidence-review facts. Malware stays visibly PENDING.

Only temporary grants/HR assignment created by this run are revoked on exit;
existing assignments, genuine evidence, request, binding, case, authority,
credential and audit records are retained. Claim revocation occurs only when
explicitly selected/authorized. Do not close/revoke the binding of a retained
credential: the verifier correctly treats that as disqualification. Both normal
sessions are logged out. The private credential is retained root-only; report
sanitized PASS/STOP and revision only. Native picker/device acceptance remains
separate. Historic PR #140 smoke stays unconfirmed and is not rerun.

## Recovery and evidence limits

Before real activation/issuance, leaving additive empty tables and recovering the
API to e84b9a6 with the retained image/configuration is compatible. Never roll back
past its bound-request legacy-signing guard. After issuance, preserve the new
verifier so current revocation can be evaluated; an old API is not a complete
functional rollback. Owner-authorized authority suspension stops further issuance;
owner-authorized exact claim/credential revocation has its distinct purpose and
is not erasure of a signature. Never restore stale data, drop audit tables, fake an
unsigned rejection or enable legacy credentials to conceal a failed acceptance.

Local operations regressions cover actual PostgreSQL transaction/restore/ACL and
workflow compatibility, runtime/false-condition/duplicate/terminal lifecycle
refusal, independent temporary RSA-3072 signature verification, incomplete trust
verdict rejection and ambiguous-issuance no-retry behavior. Existing Go tests cover
authority/key/binding/claim revocation in both concurrency orders. These are local
proofs; production preparation, deployment and genuine live acceptance need their
actual console/CI results before being reported PASS.

## Actual production preparation result

Owner returned PASS for executor d3ee63c / pinned SQL ab0c0f6. The fresh private
backup, same-image restore, rollback/commit and actual normal-workflow probe all
passed; runtime received five EXECUTEs only in the clone. Production SQL remained
read-only and production schema/authority/credentials/services were unchanged.
Use the already prepared root-private directory
`/opt/cvera/issuance-release-20261108T224912Z-02ce57` for a separately approved apply;
do not repeat NorStar registration or regenerate the preparation without a state
change or failed verification. The next independent release decision can authorize
only schema apply plus non-signing merge/deployment, keeping real authority,
credential activation and live acceptance reserved until their inputs and approvals.
