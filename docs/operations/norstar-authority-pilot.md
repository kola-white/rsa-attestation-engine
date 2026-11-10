# NorStar first-party authority pilot: preparation, not activation

> **Historical preparation record — current status reconciled 2026-11-10.**
> NorStar registration completed as `norstar-systems-llc`, MANUAL, transaction
> `3007`; exact canonical Cvera/employment.role authority is ACTIVE, activation
> transaction `3019` (owner-confirmed). **Never repeat registration or activation.**
> Earlier pending/unapproved statements below describe their original stages;
> preserve them as history, not instructions to replay. PR #141 is merged and
> implementation deployment is owner-confirmed. Public credential registration,
> eligible NorStar HR assignment, Tara's genuine independent review and genuine
> issuance acceptance remain pending. Use the [current baseline and ordered
> checklist](norstar-production-readiness.md); this document grants no new authority.


## Current facts and boundaries

Owner-provided candidate: **NorStar Systems LLC**, California,
**B20250427754**; issuer **https://cvera.app**; claim **employment.role**;
proposed factual title **Managing Member — owner**. The owner reports a California
Statement of Information listing the manager/member; the separate delegation is
being prepared. These supplied facts are not a claim that Cvera has independently
reviewed the filing, identified the internal employer ID or accepted delegation.

PR #141 c7e5239 passed engineering CI, security checks, local full PostgreSQL/Go
suite, race checks and vet. It is not deployed or live-accepted. Baseline #140 is
deployed at e84b9a6; its synthetic binding smoke remains unconfirmed and must not
be rerun as part of this preparation. No production operation is authorized here.

## 1. Reconcile the legal entity, without guessing an employer ID

`public.evt_employer_config.employer_id` is the canonical internal TEXT identifier;
the verified schema has no legal-name, jurisdiction or government-registration
columns. California's entity number is an external identity reference, not an
existing internal ID. The repository's old NorStar Tech examples are not mappings.

The prepared `operations/reconcile-norstar-employer.sql` runs one bounded read-only
transaction. It returns only candidate employer IDs, AUTO-dispatch flags and a
registry count; it never outputs request/identity fields, documents or credentials.
Candidate discovery uses ID fragments and the established employer-name path in
existing request snapshots. The owner-reported production read-only result was
**12 registry entries, zero candidate employers, legal_mapping_verified=false,
records_or_configuration_changed=false**. No existing employer ID is therefore
confirmed. Those matches, if any, would be leads rather than authoritative proof.
A zero-match result cannot prove absence when existing IDs are opaque.

Owner/adjudicator must reconcile a candidate with existing provisioning records
and the verified legal entity. If an existing mapping is confirmed, reuse that ID
exactly. If no mapping exists, later explicit production approval must cover one
stable internal ID and its private mapping to California B20250427754. Do not
create a duplicate, repurpose a demo employer, rename an ID or assume the proposed
slug `norstar-systems-llc` exists. Keep the mapping in the private evidence packet
and eventual authority scope; no new registry architecture is needed.

## 2. Minimum private packet and representative requirements

Retain the following privately; do not put filing addresses, signatures, evidence,
account identifiers or document contents in GitHub, chat or application logs:

1. Current official California entity/filing copy, including the Statement of
   Information and filing date, corroborating legal name/number and reported
   manager/member. Check current status and later amendments. The statement alone
   does not prove every ownership or signing-power assertion.
2. Relevant authentic governance evidence confirming the signer is the actual
   representative and has power to delegate: operating-agreement extract,
   membership/management record or valid resolution/consent as appropriate. Verify
   both membership and management if claiming Managing Member — owner; do not
   infer ownership merely from a manager listing. No full tax return, SSN, EIN or
   unrelated personal information is required for this packet.
3. Evidence of the claimed actual role and any dates asserted. A passive ownership
   interest alone does not establish a working management role. Keep the exact
   owner-qualified title; do not assert employee/payroll status. Optional dates
   may be omitted rather than invented from the incorporation date.
4. A separately signed and dated NorStar delegation identifying the legal entity,
   canonical issuer https://cvera.app, employment.role scope, effective start,
   optional end, authorized withdrawal contact and the signer's capacity/basis.
   Match the internal employer ID through the adjudicated mapping. Explicitly
   disclose overlapping employer-representative, claim-subject and Cvera-operator
   capacities. Neither Cvera domain/key ownership nor issuer self-declaration is
   sufficient evidence of NorStar authorization.

Keep original files and a SHA-256 inventory in existing protected private storage.
Use an opaque reference such as `urn:cvera:authority-evidence:<random-uuid>`;
record its exact private locator/hash mapping and any redacted review-copy hash.
An opaque reference without retrievable evidence and integrity mapping is not proof.

## 3. Attributable adjudication; independent review is optional here

The accepted authority contract requires sufficient organizational evidence and an
attributable Cvera decision. It does **not** require a third-party adjudicator or
a different human for the employer representative, subject and Cvera operator.
Authority evidence §2.7 permits attributable independently verifiable evidence;
§3 requires provenance, representative power and a reasoned decision. The state
machine §13 deliberately leaves the evidence-establishment mechanism open. Its
APPROVE/ACTIVATE requirements in §§6–7 still apply without exception.

The earlier wording made an additional pilot safeguard mandatory. That was an
operating recommendation, not a frozen architectural constraint. Independent
review may be selected to strengthen conflict management; it is not a requirement
that can be inferred from the word adjudication or from the projector's comments.

Paul Coe is only a potential reviewer and is currently unavailable. His prior
Cvera engineering participation is owner-disclosed; neither his independence nor
his appointment/approval is established. If later selected, assess current interests,
authority/control, compensation and involvement in the specific assertion before
labelling his review independent. Prior engineering participation alone proves
neither eligibility nor disqualification. A second account is not another person.

A first-party procedure is permissible in principle, subject to all of the following:

1. Verify NorStar's legal entity/internal ID mapping, actual representative's
   capacity and power, authentic organizational consent and the precise issuer/
   employer/employment.role scope. Public filings and authentic private governance
   records must corroborate the signed delegation; issuer self-declaration, domain
   control or an RSA key alone never establishes organizational authority.
2. Keep NorStar's organizational authorization, Cvera's evidence adjudication and
   later operational activation as separate attributable records/acts, even if
   performed by the same person in disclosed capacities. Approval is not ACTIVE.
3. Explicitly record that the representative, claim subject, adjudicator and
   recording operator overlap, identify the conflict, describe what was checked,
   and label the decision first-party/operator-adjudicated. Never describe it as
   independent third-party validation or invent a reviewer identity.
4. Preserve decision/authorization IDs, actual decision-maker identity and
   capacity/mandate, APPROVED or REJECTED outcome, rationale, authority basis,
   opaque retrievable evidence reference plus hashes, actual decision/recording
   timestamps and the conflict disclosure. Detailed disclosures/signature can be
   retained in the referenced private packet; no new database column is needed.
   Missing/conflicting evidence cannot produce approval or ACTIVE.
5. Use the accepted append-only recording protocol and real actor identity.
   APPROVE's actor and evidence reference must match its adjudication; ACTIVATE
   requires separately verified prerequisites and explicit owner authorization.
   Paul must not be named as decision-maker unless he actually made the decision.

The owner must explicitly select the first-party operating procedure before any
production adjudication/activation; this clarification is not execution approval.
A different person remains an optional authority-review safeguard, not a new role,
vendor, endpoint or database privilege requirement.

**Separate mandatory evidence boundary:** the owner-approved case evidence policy
still forbids reviewing one's own uploads. First-party authority adjudication does
not authorize self-review, create a case-review grant or establish evidence-backed
issuance eligibility. Another eligible active reviewer with an explicit case grant
must approve the subject/uploader's evidence through the accepted API. Offline
authority review and case evidence approval are distinct decisions.

The implemented machine authority scope is issuer + employer + claim type, not a
per-title or per-subject grant. The single-request pilot is an operational release
bound: inspect other pending/issuable requests and keep operator-created bindings
limited to the approved request. Do not describe narrower letter restrictions as
machine-enforced unless represented by existing effective intervals/access checks.

## 4. Bounded sequence and explicit approval gates

**Preparation gate — current authorization:** read-only reconciliation; private
packet/delegation preparation; explicit choice of first-party or independent authority adjudication; attributable
decision; local tests/CI and pinned release artifacts. No production writes.

**Production schema gate — separate owner authorization:** use the existing root
console, confirm current revision/schema/runtime privileges, restore-test a fresh
private backup, retain recovery image/configuration and rehearse exact 000019 plus
its five EXECUTEs. Inspect existing authority objects; only when wholly absent and
explicitly approved may the existing 000002–000013 be restored/rehearsed/installed
without fabricated migration history. Unexpected partial state is STOP. If needed,
include the one reconciled employer row explicitly; do not silently insert or change
its dispatch mode. This gate creates no ACTIVE authorization or credential binding.

**Authority/credential gate — separate explicit scope:** after sufficient, attributable adjudication, owner
approves the exact employer ID, authorization/decision IDs, evidence hashes/ref,
actual actor, start/end and withdrawal procedure. Persist the accepted authorization,
adjudication and SUBMIT/APPROVE/ACTIVATE sequence using one evidence-row recording
transaction per step. Confirm the coherent evaluator's exact ACTIVE scope. If an
end is promised, verify the projection actually bounds ValidUntil; a date written
only in a letter is not enforcement. No arbitrary platform lifetime is imposed.
Separately approve binding the existing public signing credential to canonical
Cvera with verified SPKI/JWKS/kid, explicit credential validity and claim lifetime.
No private-key export, rotation, new role or password change is part of this gate.
These approvals may be one consolidated owner decision if every input is fixed.

**Live acceptance gate — explicit one-request signing authorization:** after schema,
credential and authority verification and approved deployment, use the normal
requestor API to create/submit exactly the approved owner-role request. Bind one
new empty case with explicit required checks; provision only approved scoped
HR/upload/review access. Use authentic reviewed role evidence, not a synthetic
PNG as proof of real managerial status. Keep malware status PENDING and select the
explicit manual pilot policy. Stop if normal dispatch, account roles, evidence,
review independence or any eligibility/authority check fails; do not patch records.

Issue once through the new path, retrieve the subject-owned credential, independently
verify with expected subject and policy, and test tamper/wrong-context/revocation
negatives. Record only sanitized outcomes. Credential contents and private evidence
stay off logs/chat. Native picker/device validation remains separate.

**Containment/closure gate — explicitly included in the live approval:** specify
whether the real NorStar delegation remains in effect or is pilot-only. Do not
revoke a standing real delegation by treating it as a disposable test fixture.
For an authorized pilot-only grant, append attributable SUSPEND/REVOKE as agreed;
revoke the exact acceptance claim and prove the unchanged artifact no longer
verifies as trusted. Close/revoke fixture case/binding/grants and log out sessions;
preserve the signed request and append-only audit. REVOKED/EXPIRED authorizations
require a new authorization ID, not historical edits or resurrection. Do not revoke
the signing key merely to clean up a claim. Never fake rejection of an issued
request or delete evidence/audit without separate destructive-operation authority.

Use the recovery constraints in `evidence-backed-issuance-release.md`: retain the
new verifier for issued artifacts; no downgrade past the bound-request signing
guard, stale restore, fabricated history or re-enabled legacy credentials. A failed
backup restore, ambiguous COMMIT, scope mismatch or unexpected production condition
is STOP, not permission for ad hoc grants or a second signing attempt.

Official reference sources for the adjudicator: California Secretary of State
[business records and document copies](https://www.sos.ca.gov/business-programs/business-entities/information-requests)
and [LLC entity/governance records](https://www.sos.ca.gov/business-programs/business-entities/starting-business/types).
The company's private operating agreement is not itself a filed Secretary of State
record. These general sources are not verification of the supplied specific filing.

Executable preparation now lives in [minimum preactivation sequence](norstar-preactivation-sequence.md)
and the [blank private paperwork templates](norstar-delegation-and-decision-templates.md).
The guarded registration and four-transaction recording scripts were tested only
in isolated local PostgreSQL; they stop at APPROVED and are not production execution
authorization. The independent case-evidence reviewer remains a separate requirement.
