# Evidence-backed employment issuance and independent verification

## Delivery state

As of **2026-11-10**, PR #141 is merged as
`bce7b9833885b69b94a2f7d6cf85d415cbc6ebac`; its
[main CI/deployment run 42534540050](https://github.com/kola-white/rsa-attestation-engine/actions/runs/42534540050)
succeeded, and the owner confirms the implementation is deployed. Current live
November 10 revision is **UNVERIFIED** in this documentation-only reconciliation.
No genuine evidence-backed NorStar issuance/verification acceptance is claimed.

NorStar registration is complete (`norstar-systems-llc`, MANUAL, transaction 3007)
and its exact organizational authority is owner-confirmed ACTIVE (transaction
3019). Do not repeat registration or activation. November 10 read-only observations
show public issuer credential `issuer-prod-2026-10` **not registered/not active**
and zero eligible active NorStar-assigned HR principals. Tara's genuine independent
case review remains pending. These are separate operational/human gates, not proof
of defects in the deployed contract below.

The independent two-account evidence-review synthetic acceptance is PASS;
it does not prove genuine NorStar review. Binding release #140 is recorded deployed
at e84b9a6 with 000018 verified; its separate synthetic smoke remains UNCONFIRMED,
with no rerun implied. See the [current four-stage operational checklist](../operations/norstar-production-readiness.md)
for exact prerequisites, approvals, verification and STOP conditions.

## Integrated contract

`POST /v1/employer/requests/{request_id}/issue-evidence` accepts only
`{"employer_id":"..."}`. Authentication, current active HR role, explicit current
HR/employer assignment and the exact persisted request are mandatory. It does
not accept eligibility, issuer, subject, authority, manifest or response assertions.
The separate legacy positive attestation route continues to deny every bound
request, including revoked bindings.

One READ COMMITTED transaction pins the trusted schema and re-runs the accepted
000018 preparation. Case/check/upload/scan/review, binding and request locks
remain held through signing and persistence. There is no client receipt to replay.
The exact immutable snapshot and manifest text/digests come from that transaction.
A bounded SECURITY DEFINER read locks the accepted authority recording boundary,
checks all positions are complete and reconstructs the existing journals,
adjudications and authority projector. Only current ACTIVE delegated authority for
**https://cvera.app + employer_id + employment.role** permits signing. Missing,
ambiguous, incomplete, suspended, expired or revoked authority fails closed.

The issuer-credential registry is a separate owner-established association of
canonical issuer, kid, RSA public SPKI fingerprint, validity interval and explicit
claim lifetime. It contains no private key. RSA must be at least 3072 bits with
exponent 65537; the existing in-memory signer must match the exact registered
public key and kid. The empty registry activates nothing. The runtime and evidence
operator cannot establish, replace or revoke credentials or authority. Registration
is **not** inferred from the key filename, public JWKS membership, HR assignment,
case grant or review. This is a configured trusted binding, not a claim of automatic
HTTPS issuer-metadata discovery or organizational delegation.

The existing RS256 implementation signs the bounded, fully disclosed SD-JWT VC
profile (the repository's pinned draft-18 adapter; protected `typ=dc+sd-jwt`,
serialization terminating in `~`, no selective disclosures or holder-key JWT).
`vct=https://cvera.app/credentials/employment-role/v1` selects this exact profile.
Existing generic SD-JWT and legacy verification behavior remains compatible.
The signed payload commits:

- `iss=https://cvera.app`, `sub=domain_users.id`, unique `jti`, `iat`, `nbf`, `exp`;
- `claim_type=employment.role`, canonical employment fields derived from the
  existing `primary_employment` snapshot, with the persisted employer ID;
- exact request, case, binding, subject, employer and original request version;
- immutable request snapshot SHA-256 and manifest ID/SHA-256;
- manual-evidence pilot policy, authority grant/recording position and credential
  SPKI fingerprint.

Descriptive `primary_employment.issuer_name` remains employer name, never the
credential issuer. Explicit conflicting subject/employer/claim-type assertions in
the snapshot are rejected. Domain subject UUID is an identifier binding, **not**
proof of holder-key possession, human identity or employment merely from login.
The digest convention is SHA-256 of the exact UTF-8 PostgreSQL JSONB text retained
by the trusted receipt; arbitrary JSON reserialization must not be substituted.
Neither the signed artifact nor its public outcome contains private object keys,
storage URLs or evidence bytes. No proprietary CA or new claim type is introduced.

Only after exact claims/manifest persistence, normal ATTESTED transition and event
commit succeed does HTTP return the portable credential. An event/transaction
failure returns no artifact and leaves no issuance receipt. Each request issues
once; a repeated issuance attempt conflicts. `GET /v1/requests/{request_id}/credential` returns the exact committed artifact
only to its current active requestor subject; cross-user/inactive access is denied.
Retrieval does not assert that the artifact remains trusted. Expiry is explicitly bounded by the owner-established
credential validity and claim lifetime; no implicit perpetual credential.

## Independent verification

`POST /v1/verification/outcome` preserves `jwsCompact` and accepts `expectedSubject`
and `policy=employment.authoritative/manual-evidence-pilot/v1` for this profile.
Expected subject and policy are mandatory for a positive result. The verifier
has **no signing object or private-key access**. It independently verifies RS256,
exact trusted issuer/key association, registered validity, canonical identities,
full immutable issued claims, token hash, snapshot and manifest byte/context
commitments. It returns the canonical portable claim and commitment only on
VERIFIED. Signature validity remains distinct from trust and status.

A signed manifest hash proves only that the issuer committed to those bytes.
Approval is established separately through the minimum trusted-resolution mechanism:
bounded database resolution of the exact presented token digest/issued receipt,
immutable manifest and independent contextual reviews. It rechecks current request
identity/snapshot, binding, case/check eligibility, missing/new incomplete or
unreviewed evidence, deleted objects and scan disqualification. Original required
checks must match; closing/revoking the binding or discovering infected/error
scans makes the claim unusable. Historical review-grant revocation does not erase
an already attributable approval. Scan status remains visible and PENDING is
permitted **only** under the explicitly selected manual pilot policy.

Credential and claim revocations are separately evaluated. Current authority is
re-resolved under the same global boundary lock and the existing authority and
verifier-policy evaluators determine VERIFIED/REVIEW/REJECTED. Explicit negatives
reject; missing/unavailable trusted resolution yields REVIEW, never VERIFIED.
A stale authority snapshot predating issuance cannot establish current trust.

This verifier is independent of the signer, not independent of every trusted data
source: it relies on the configured Cvera issuer binding, authoritative registry
and receipt/review resolver. Offline signature-only checking cannot prove current
revocation or underlying review facts. Portable bytes are not an offline trust
proof, a generic wallet protocol or permission to rely under an unknown policy.

## Persistence and privileges

000019 creates owner-only `evidence_issuer_credentials` and
`evidence_issued_credentials`; both permit only attributable one-way revocation.
It creates no roles, active credential, authority record, resource or case grant.
Hostile default table/function ACLs and PUBLIC EXECUTE are stripped. Runtime gets
only five bounded EXECUTEs: active public credential lookup, coherent authority
read, prepare issuance, commit issuance and resolve exact issued facts. No direct
new-table or authority-table DML/SELECT/UPDATE locking grants. Owner-only claim
revocation and credential registration are not exposed as application endpoints.

The historical #141 release required verified authority 000002–000013 plus
000019/bindings before deployment. The owner now confirms deployment and ACTIVE
NorStar authority; do not replay that bootstrap. Preserve actual schema/change
receipts rather than inventing 000001 migration history. Any future authority
mutation retains the accepted one-evidence-row-per-recording-transaction protocol.

## Verification and remaining acceptance

Local tests exercise actual RSA, migrations 000014–000019, existing authoritative
journals/projector/evaluator, restricted runtime HTTP issuance and independent
verification. They cover wrong scope/user/employer, missing/revoked authority,
changed context, missing or disqualified evidence, immutable manifests, revoked
keys/claims/bindings, invalid signature/subject/policy, failure rollback, hostile
ACLs and real PostgreSQL blocking in both issuance/revocation orderings.
Local keys and authority are explicit test fixtures, never production substitutes.
Exact commands/results and CI are recorded in the release PR/readiness assessment.

Remaining genuine production gates are separately approved public credential
registration, an eligible NorStar HR principal/assignment, Tara's independent
review and the one-request acceptance. Authority activation is complete, not a
remaining gate. Synthetic acceptance, historical #140 smoke, native-device/picker
validation and genuine live acceptance must remain separately reported. This
feature document authorizes no operation, account/grant change or signing.

Local verification commands (run from `apps/evt-api-go`; the existing guarded
local test driver supplies only the disposable PostgreSQL DSN, and CI supplies
`CVERA_AUTHORITY_TEST_DATABASE_URL`):

```sh
go test ./... -count=1 -timeout=120s
go test -race ./internal/httpapi ./internal/db ./internal/evidencecredential -count=1 -timeout=120s
go vet ./...
```

All passed locally with database integration enabled. Tests without that explicit
local/CI database setting skip PostgreSQL cases and are not equivalent evidence.
The normal create/submit test explicitly labels its legacy request UUID default
and timestamp columns as workflow scaffolding; actual production catalog/default
compatibility required the release's restored-schema rehearsal. That historical
preparation does not prove genuine live normal-request acceptance. No assumption
about missing production DDL is a migration.
