# Stage 4B portable verification contract

Status: local synthetic interoperability capability. Not a production API, new
production credential profile, authority mandate, wallet or status publisher.
It preserves production v1 behavior and ADR-009's separation of cryptographic
validity, authority, subject assurance and verifier reliance.

## Inputs and trust boundary

Both runners consume one UTF-8 JSON document on stdin (maximum 4 MiB). The input
has `credential`, `trust`, `policy`, optional `authorityJws`, `status`, and `holder`.
The public vectors are complete examples. Invalid JSON, non-synthetic trust,
invalid policy or oversized input exits nonzero (2); a completed evaluation emits
one result and exits 0 even when the outcome rejects the credential. Automation
must inspect the result, not equate process success with verification success.

**The verifier controls `trust` and `policy`.** Never accept a sender-supplied
whole JSON envelope as a trusted verification request. Credential senders cannot
select trust roots, expected subject, clock or maximum status age. All keys here
are public SPKI PEM RSA keys (at least 3072 bits, exponent 65537), pinned to an
explicit issuer and key ID. No key is discovered from an untrusted token. There
are no network fetches, credentials, Cvera API calls or database connections.

`policy.now` is a required positive integer Unix second, supplied by the verifier.
`policy.expectedSubject` is the expected canonical subject identifier. Its equality
is a consistency check, not proof of possession. `policy.maxStatusAgeSeconds` is
an optional nonnegative integer, default **900**; zero means current-second only.
Integers must be at most 9,007,199,254,740,991 for Go/JavaScript interoperability.
Credential or status fields resembling policy settings are ignored.

`trust.synthetic` must be true. These public fixtures use ephemeral local keys,
synthetic authority and fictional identifiers. The existing credential profile
fixes `iss=https://cvera.app`; that string does not make these test signatures or
test authority genuine. No fixture key is a production trusted issuer key.

## Existing credential format

The supported format is the existing fully disclosed evidence-backed v1 RS256
`dc+sd-jwt`, serialized as `<issuer JWT>~`. The existing `vct`, claim type and
manual-evidence policy remain unchanged. Selective presentations, KB-JWT,
unsupported algorithms and other serializations are outside this increment.

Go calls the existing `CredentialProfile.VerifyAndNormalize` boundary using a
pinned public-key resolver; JavaScript uses JOSE directly and independently
checks headers, signature and claims. Neither invokes the other implementation.

`claimBinding=VALID` establishes internal signed-reference consistency, canonical
identifier/digest shapes, expected subject, employer consistency and issuer-key
SPKI fingerprint. It does **not** establish that underlying private evidence is
true or reviewed. It does not rehash arbitrarily reserialized JSON as though that
were the production PostgreSQL snapshot. Authority assertions below authenticate
only the explicitly synthetic fixture mandate.

## Fixture authority and status assertions

These are signed **test envelopes**, not new credential profiles or production
status infrastructure. Actual publication/distribution remains Stage 4D.

Each assertion is an RS256 compact JWT. Its issuer, kid, key and protected typ must
match the appropriate locally pinned trust input. Both require `synthetic=true`
and `credential_sha256` of the exact entire portable credential bytes, including
the final `~`. Signatures, issuer, binding and field types are validated before
trusting timestamps or decisions. Keys embedded in a token cannot replace roots.

Authority typ is `cvera-stage4b-fixture-authority+jwt`. Payload: `iss`, `synthetic`,
`iat`, `exp`, `credential_sha256`, credential `issuer`, `employer_id`, `claim_type`,
and `decision` (`AUTHORIZED`, `UNAUTHORIZED`, `UNKNOWN`). Time validity is
`iat <= now < exp`. Missing authority is UNKNOWN; bad signature/scope/time is
INVALID. This fixture mechanism does not settle historical-authority semantics.

Status typ is `cvera-stage4b-fixture-status+jwt`. Payload: `iss`, `synthetic`, `iat`,
`credential_sha256`, and `entries`, each with `credential_id` and `state` (`GOOD`
or `REVOKED`). The fixture status signer is explicitly trusted for these test
credentials. A matching digest plus matching credential ID establishes applicability.
Duplicate matching entries and invalid state/type are INVALID, not last-entry wins.

Status processing precedence:

1. Missing status or `availability=UNAVAILABLE`: UNKNOWN. No cached fallback.
2. Invalid availability, signature, issuer/kid/typ, binding, timestamp or entries:
   INVALID. Future `iat` is INVALID; there is no implicit clock-skew allowance.
3. No applicable credential entry: UNKNOWN.
4. Applicable evidence with `now - iat > maxStatusAgeSeconds`: STALE, including
   stale revoked evidence. It cannot establish current status or positive trust.
5. At ages **0 through 900 inclusive** under the default policy: GOOD or REVOKED
   according to the authenticated applicable entry. Exactly 900 is eligible;
   exactly 901 is STALE. A status envelope cannot override the verifier policy.

A fresh timestamp alone never authenticates status. Freshness applies only after
cryptographic and applicability checks. STALE/UNKNOWN are unresolved, not positive.

## Result contract

Contract ID: `cvera.stage4b.portable-verification.v1`; `synthetic=true`.

| Field                  | Outcomes                                                                 |
| ---------------------- | ------------------------------------------------------------------------ |
| cryptographicIntegrity | VALID, INVALID, UNSUPPORTED, UNKNOWN                                     |
| claimBinding           | VALID, INVALID, UNKNOWN                                                  |
| temporalValidity       | VALID, INVALID, UNKNOWN                                                  |
| issuerAuthority        | AUTHORIZED, UNAUTHORIZED, INVALID, UNKNOWN                               |
| credentialStatus       | GOOD, REVOKED, STALE, INVALID, UNKNOWN                                   |
| holderAssurance        | UNKNOWN, UNSUPPORTED                                                     |
| eligibleWithoutHolder  | true only when crypto/binding/time/authority/status all pass             |
| overall                | REJECTED for explicit negative/invalid evidence; otherwise INDETERMINATE |

Processing stops on unusable cryptography/profile; unevaluated checks stay UNKNOWN.
Credential time validity requires integer `iat=nbf`, `exp>iat`, and `nbf<=now<exp`.
Unsupported or missing holder assurance stays unresolved. Even a caller's `VALID`
holder hint cannot establish possession. **Stage 4B never emits VERIFIED.**
A passing test vector means expected behavior, not complete credential trust.

Offline signature verification establishes only cryptographic integrity. Current
status additionally needs authentic applicable evidence within policy freshness.
Complete policy-governed verification additionally needs real authority and
holder assurance, which synthetic fixtures and identifier equality cannot supply.
Stage 4C supplies holder presentation; Stage 4D supplies portable production trust
and status infrastructure. Lifetime validity is not established.

## Reproduction and independent isolation

From the repository root, install existing lockfile dependencies (`npm ci`) and
use Go 1.26.10, matching CI. Public fixtures are committed and require no key.

```sh
npm run test:portable
# Enforced isolation with the CI Node runtime:
docker pull node:20
python3 interoperability/stage4b/run.py --isolated --output /tmp/stage4b-result.json
```

The isolated harness builds a static Linux Go runner, then independently invokes
it and the JavaScript runner in a Node 20 container. Docker configuration is
inspected before execution: network none, read-only root, unprivileged user,
dropped capabilities, no-new-privileges, one read-only public-input directory.
Only runners, public vectors and the existing JOSE dependency are mounted. No
repository checkout, database, Docker socket, private key or host credential
configuration is mounted. Child processes get a minimal environment.

For exact image reproduction, pass `--node-image node@sha256:<registry digest>`
using the digest recorded in `evidence/isolated-interoperability.json`. The harness
also verifies installed JOSE against the lockfile.

The harness checks every expected result from both processes, tests invalid
policies, and repeats the unchanged base credential three times per verifier.
The manifest records one issuance call and the base credential SHA-256. Verification
does not regenerate fixtures. Negative re-signed JWT mutations are test vectors,
not additional authoritative attestation/issuance workflow calls.

Explicit fixture regeneration, if needed, creates new ephemeral local keys in
memory and overwrites only public fixtures. Never use production keys or data:

```sh
cd apps/evt-api-go
GOTOOLCHAIN=go1.26.10 go run ./cmd/portable-fixtures ../../interoperability/stage4b/fixtures
```

This calls existing `IssueLocked` once using synthetic authority/evidence. The
fixture setup is adapted from the existing issuance unit test. Random test keys
and credential IDs change on regeneration; evaluation of committed vectors is
deterministic. No private key is persisted or supplied to verification.

## Legacy boundary and preserved diagnostic

Neither runner imports `pkg/verification/verify.ts`. The Stage 4A synthetic probe
showed the legacy CLI accepting stale status, absent applicable entries, and an
arbitrary nonempty payload-hash field; explicit revocation was rejected. These
findings are **not** attributed to the database-backed SD-JWT verifier.

The reproducer is `interoperability/stage4b/legacy-diagnostic.mjs`. It uses a fresh
local-only synthetic key and temporary public trust files; audit output contains
no token or private key. Run from root with existing dependencies:

```sh
node --import ./node_modules/tsx/dist/loader.mjs interoperability/stage4b/legacy-diagnostic.mjs
```

Correction/deprecation of that legacy implementation is not included here.
