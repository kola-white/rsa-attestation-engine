# Evidence-backed employment issuance and independent verification

## Delivery state

Implementation branch: `feature/evidence-backed-issuance`. Not deployed or
production-accepted. Binding release #140 is deployed at e84b9a6; its separate
synthetic acceptance is **pending**, with no rerun authorized or performed.
Migration 000018 has passed production schema/backup/restore verification.

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

The existing authority schema is not presumed deployed. Existing 000002–000013
must be inspected/restored/rehearsed before a separately authorized bootstrap;
000001 migration-history fabrication is forbidden. All authority mutations must
retain the accepted one-evidence-row-per-recording-transaction protocol.

## Verification and remaining acceptance

Local tests exercise actual RSA, migrations 000014–000019, existing authoritative
journals/projector/evaluator, restricted runtime HTTP issuance and independent
verification. They cover wrong scope/user/employer, missing/revoked authority,
changed context, missing or disqualified evidence, immutable manifests, revoked
keys/claims/bindings, invalid signature/subject/policy, failure rollback, hostile
ACLs and real PostgreSQL blocking in both issuance/revocation orderings.
Local keys and authority are explicit test fixtures, never production substitutes.
Exact commands/results and CI are recorded in the release PR/readiness assessment.

Production gates remain: baseline #140 acceptance; separately authorized schema
and bounded runtime read deployment; real attributable employer delegation;
verified canonical issuer/public credential activation; one explicitly authorized
normal-request, two-account evidence/review/issuance/verification acceptance.
Native device/picker acceptance is separate. No signing-key operation, authority
establishment, credential activation or live signing has been executed here.

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
compatibility must be established by a restored-schema rehearsal before live
normal-request acceptance. No assumption about missing production DDL is a migration.
