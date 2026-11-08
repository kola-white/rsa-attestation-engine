# Minimum NorStar sequence: prepared, not authorized for execution

This is an executable operator plan, not production permission. The scripts do not
create roles, change privileges, activate issuer authority, register a signing
credential or sign an attestation. They require a separately authorized, verified
owner connection and actual completed private inputs. No production steps ran.

## 1. Fix the entity mapping and approve one registration

Owner confirms whether an existing opaque employer ID already maps to NorStar
Systems LLC, California B20250427754. The read-only result was 12 registry entries,
zero matching candidates; that is not proof of absence. If a mapping exists, reuse
its ID and SKIP the new-registration script. Otherwise approve the new proposed
`norstar-systems-llc` with initial **MANUAL** dispatch. AUTO is not used for
registration: normal CandidateSubmit would advance an AUTO request to
ATTESTATION_PENDING; MANUAL leaves it SUBMITTED. Neither mode grants HR, case
review/upload, issuer authority or credential permissions. Creating a registry row
is not organizational verification. Actual INSERT triggers/rules must be checked;
the prepared registration refuses unreviewed user INSERT triggers/rewrite rules.
Retain the legal-entity/ID mapping and the production approval reference privately.

After that separate write approval, the existing root console runs
`operations/register-norstar-employer.sql` as the verified table owner. Inputs:

- `cvera.norstar_employer_id`: confirmed ID, proposed norstar-systems-llc.
- `cvera.norstar_mapping_ref`: actual approved private identity mapping reference.
- `cvera.norstar_approval_ref`: actual owner production-registration authorization.

It performs one INSERT transaction and returns only registry/transaction metadata.
It refuses missing references, a non-owner or an existing ID; it never overwrites
or silently adopts an existing row. No legal-name/registration columns are invented.
A unique-ID race/error is STOP, not permission to choose an alias or retry blindly.
The later live scenario requires a separately approved dispatch plan: existing
code has no manual-dispatch route, so a narrowly reviewed AUTO window would need
explicit owner authorization before normal create/submit, followed by MANUAL
restoration as approved. Do not patch request status or silently change the mode.

## 2. Complete the two private signed acts

Use the blank [delegation and decision templates](norstar-delegation-and-decision-templates.md).
First sign NorStar's organizational delegation in the actual representative
capacity, corroborated by the official filing and authentic governance/role evidence.
Then separately make/sign Cvera's first-party evidence decision with actual findings,
rationale, capacity, dates and overlap/conflicts. No independent reviewer is claimed.

The exact scope is **https://cvera.app / confirmed employer ID / employment.role**.
The truthful owner-qualified title is **Managing Member — owner**. Keep private
originals, SHA-256 inventory, any redacted-copy mapping and an opaque retrievable
packet reference. Choose the delegation start, optional end and authorized withdrawal
mechanism; a paper end alone does not enforce a machine expiry. A rejected or
undecided packet must not be passed to the positive recording script.

## 3. Record only through APPROVED, after separate production authorization

First verify actual authority-schema state and restore/rehearse required existing
000002–000013 on a fresh backup if absent. Those migrations are not assumed applied,
and no 000001/version history is invented. Bootstrap/schema/privilege operations
are separately approved; the recording script does not perform them. Unexpected
partial schema or existing authority scope is STOP for compatibility/history review.
Migration 000019 and runtime bindings are also separately approved release work,
not implicit prerequisites installed by this script.

Using `psql -X -v ON_ERROR_STOP=1`, read completed session-scoped settings from a
protected private input file (not shell arguments, logs or Git). In addition to
employer/mapping/approval references, provide:

| Session setting suffix after `cvera.norstar_` | Actual input |
|---|---|
| authorization_id / decision_id | New stable IDs reserved for this approved recording |
| actor_id | Actual first-party decision-maker's stable identity, privately mapped to the person |
| evidence_ref | Opaque reference to the complete signed and hashed packet |
| authority_basis | Actual capacity, corroborating basis and substantive decision rationale |
| decided_at | Actual UTC adjudication time, not a fabricated historical timestamp |
| outcome | Explicit APPROVED matching the signed decision; never a default |
| conflict_disclosure_ref | Signed disclosure of representative/subject/adjudicator/operator overlap |

The separately approved owner session then runs
`operations/record-norstar-authority-approved.sql`. It uses the existing recording
allocator and four independent evidence transactions:

1. Authorization record (REQUESTED), exact canonical scope.
2. Actual APPROVED adjudication record, explicitly first-party/operator-adjudicated.
3. SUBMIT event (PENDING).
4. APPROVE event (APPROVED), using the stored decision-maker and evidence reference.

The script then verifies exactly SUBMIT/APPROVE and no activation event and returns
`APPROVED_NOT_ACTIVE`. It contains no ACTIVATE insert, issuer credential operation
or signing call. Missing inputs, runtime execution, duplicate IDs/scope, rejected
input and blind reruns deny. An error/ambiguous COMMIT after a prior step may leave
legitimate partial history: preserve it and inspect exact IDs before an explicitly
reviewed continuation. Never delete/reset records or allocate new IDs to hide it.

For a later approved execution, stage only the reviewed files from the pinned
approved commit; do not switch the running production checkout to the feature
branch. Run each phase separately with its own completed private settings file.
The console command shape is:

```sh
cat "$CVERA_PRIVATE_INPUT" "$CVERA_REVIEWED_SQL" | docker exec -i evt-api-go-db-1 psql -X -U postgres -d evt -Atq -v ON_ERROR_STOP=1 -v VERBOSITY=sqlstate
```

`CVERA_PRIVATE_INPUT` must be the root-private file containing actual session SET
statements, never example placeholder values; `CVERA_REVIEWED_SQL` selects exactly
the separately approved registration or preactivation script. Capture the result
in the private operation receipt. No command is authorized for production now.

## 4. Stop at the production activation approval gate

Present the actual registration/decision refs, source hashes, transaction IDs,
scope, effective bounds, withdrawal authority, schema/backup/recovery evidence and
APPROVED_NOT_ACTIVE result. Obtain explicit authorization for ACTIVATE. Credential
registration, deployment and the exact one-request live acceptance must also be
explicitly included in a later bounded approval; approval of this preparation or
of preactivation recording does not authorize them. State/authority evaluation
remains fail-closed until the exact ACTIVE scope and every other prerequisite exist.

## Separate case-evidence no-self-review path

Nominate one OTHER real person who is authorized to see/review the role documents.
Use their own existing active Cvera account or normal supported registration/login;
no shared credentials or second account controlled by the uploader. This person
need not be Paul, an authority adjudicator, HR reviewer or database administrator.

Once the approved request is bound to its exact case/check, separately authorized
operator action through existing `cvera_evidence_operator` provisions only
`grant_evidence_review(reviewer_domain_users_id, case_id, actual_actor, mandate_ref)`.
The person logs in and reviews canonical evidence and the binding/claim context
through the existing review API, recording their own APPROVED/REJECTED decision.
Upload/HR grants alone do not confer that access. PENDING malware status remains
visible. First-party authority adjudication cannot replace this case decision.

The HR principal who eventually invokes issuance separately requires the existing
active HR role/employer assignment. Granting case review does not assign an HR role.
If no other eligible reviewer is available, paperwork/authority preparation can
continue but evidence-backed issuance remains blocked. Do not change the policy.

## Local verification

`python3 apps/evt-api-go/operations/test-norstar-authority-preparation.py` passed:
network-none disposable PostgreSQL 16, actual authority migrations, four recording
transactions, APPROVED-not-ACTIVE gate, missing-input/runtime/duplicate/rerun denials.
No production connection, signing key or credential was used. This is not a claim
of real-schema production verification or authentic NorStar delegation.

See [schema, backup and command review](norstar-schema-backup-review.md) for the
read-only collision/side-effect metadata check, fresh actual-backup restore and
future command sequence. MANUAL registration was locally verified; production
trigger/default/constraint and legal-mapping checks remain prerequisites, not
assumptions. No new privileges or signing activity are included in preparation.
