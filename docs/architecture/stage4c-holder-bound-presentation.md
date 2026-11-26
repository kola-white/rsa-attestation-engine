# Stage 4C — Holder-bound presentation and reciprocal authentication

Status: locally implemented synthetic interoperability boundary; not deployed.
Baseline: `fd1bbfcd7150a9c39a0a334817f153e4f0e73fc4` (Stage 4B, PR #152).

## Scope and architectural decision

One synthetic issuer-signed credential can be presented to two separately
trusted verifier identities without reissuing or reattesting it. The holder and
each verifier evaluate trust locally, using public inputs. There are no Cvera
accounts, API calls or database dependencies in verification.

The additive Go `internal/holderpresentation` package reuses
`internal/portableverify` only for the unchanged credential layer. The independent
JavaScript `interoperability/stage4c/verify.mjs` uses JOSE and reuses the Stage 4B
JavaScript credential verifier. Neither invokes the other implementation.

The bounded implementation uses RS256, RSA public keys of at least 3072 bits with
exponent 65537, public SPKI roots and strict public RSA JWKs. It adds no production
HTTP route, profile registration, migration, issuer authorization or key loading.
The local CLI is a test/integration runner, not a remotely exposed verification
service. Synthetic authority/status assertions remain explicitly synthetic.

See [ADR-010](adr/010-bounded-reciprocal-presentation.md).

## Compatibility boundary

The existing pinned SD-JWT VC draft-18 employment-role profile, `dc+sd-jwt` type,
VCT, claim schema and Stage 4B verification semantics are preserved. The new
synthetic issuance seam adds an issuer-signed `cnf.jwk` to a **new** credential;
it does not rewrite or add assurance to any existing credential. The original
production issuance implementation is unchanged and does not call this seam.

This is a fully disclosed credential profile. Presentation supports the compact
`issuerJWT~kbJWT` form with zero disclosures. It does not implement selective
claim release, arbitrary SD-JWT profiles, alternate algorithms or profile upgrade.
Requests asking for unsupported claim filtering, extra credentials or excess
scope are denied. Full credential release requires explicit policy and consent;
this profile cannot minimize disclosure by removing signed employment fields.

Stage 4B still reports holder assurance UNKNOWN for the bound issuer credential
and never emits VERIFIED. A complete presentation is evaluated only by Stage 4C.
The historical legacy CLI diagnostic and its limitations remain unchanged;
these findings are not attributed to the database-backed SD-JWT verifier.

## Roles, inputs and flow

1. An authorized issuer caller supplies canonical claims, the intended holder
   public key, and its own fresh challenge context. The holder signs a possession
   proof. `IssueBound`/`issueBound` verifies it before invoking the issuer signer.
   The signed credential protects the holder key. This seam **does not establish
   issuance authority**: the caller must already have it. Tests supply synthetic
   claims only. Key possession does not establish a person's real-world identity.
2. A verifier produces a signed OpenID4VP Request Object. The holder looks up
   pre-registered public keys and exact HTTPS response URI by client ID. A key
   supplied in an untrusted request is never accepted as a trust anchor. There
   is no remote key retrieval, metadata override or request URI fetch.
3. The holder independently checks authentication, the verifier's disclosure
   permission, local full-disclosure policy, current credential eligibility and
   its own trusted local consent decision. Consent is bound to the exact request
   hash, credential hash and expiration. Caller-supplied network assertions must
   never be mapped directly into this local consent or trust state.
4. Only after these gates pass does `Present`/`present` verify the private key
   matches issuer-protected `cnf.jwk` and produce the Key Binding JWT. Any denial
   or error returns no protected presentation. This library returns presentation
   bytes; delivery transport and a real holder consent UI are not implemented.
5. The intended verifier evaluates the credential independently, verifies the
   KB-JWT using only issuer-protected `cnf.jwk`, and compares its own original
   request and transaction. It atomically consumes that transaction only after
   every applicable check passes. An invalid submission does not burn a challenge.

### Trusted local contract inputs

| Input                 | Meaning and provenance                                                                                                                           |
| --------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------ |
| `base`                | Unchanged Stage 4B credential, public issuer/authority/status roots and verifier policy; see [contract](../api/portable-verification-stage4b.md) |
| `request`             | Untrusted signed Request Object; never a trust source                                                                                            |
| `verifiers`           | Out-of-band local registry: unique `id`, pinned `kid`/SPKI `key`, exact `responseURI`, separate boolean `allow` permission                       |
| `allowFullDisclosure` | Local governing policy, never read from the signed request                                                                                       |
| `consent`             | Holder-only trusted local decision: `granted`, `requestHash`, `credentialHash`, `expires`                                                        |
| `transaction`         | Verifier-owned original `verifier`, `nonce`, `requestHash`, `expires`; never registered from an incoming presentation                            |
| `presentation`        | Untrusted complete compact SD-JWT+KB; bounded by the CLI's 16 MiB session input limit                                                            |

Hashes in the new local context contract are base64url SHA-256. Existing Stage 4B
hex digests remain unchanged. Verification time is explicitly supplied, allowing
deterministic evaluation. Production callers would need an accurate trusted clock.
The registry is static local policy, not a production verifier registrar.

## Protocol and policy enforcement

Requests require RS256, `typ=oauth-authz-req+jwt`, the pinned key ID, matching
`iss`/`client_id`, `aud=https://self-issued.me/v2`, `response_type=vp_token`,
`response_mode=direct_post`, exact registered response URI, `nonce`, `state`,
`iat`, `exp` and one supported `dcql_query`. IDs have no colon (pre-registered
fallback). Unknown request parameters and security-critical headers are denied.
The permitted query requests one `employment` credential, `format=dc+sd-jwt`,
the pinned VCT, and `require_cryptographic_holder_binding=true`.

Cvera's local policy requires requests to be no more than 300 seconds old, with
an original lifetime no longer than 300 seconds and exclusive expiry. Challenges
and state strings must be at least 22 characters. Synthetic fixtures deliberately
use deterministic strings; **a real caller must generate fresh unpredictable
challenges with at least 128 bits of entropy**, associate them with the original
transaction, and supply trustworthy transaction/registry state. Length alone
does not establish entropy.

Issuer possession proofs use the bounded OpenID4VCI JWT proof form:
`typ=openid4vci-proof+jwt`, embedded public `jwk` matching the intended key,
issuer audience, issuer nonce and recent `iat`. The issuer-owned context expires
within 300 seconds. Missing, incorrect, expired or replayed proof never reaches
the signing callback. A signer error returns no artifact and consumes the proof;
no automatic retry is attempted after uncertain signing.

KB-JWTs require `typ=kb+jwt`, RS256, intended verifier audience, exact nonce,
recent `iat`, and `sd_hash` over the ASCII issuer credential **including the final
tilde**. The supported hash is SHA-256. Optional `exp` and `nbf`, when present,
are also checked. Unsupported headers, private/ambiguous holder JWKs, wrong keys,
malformed inputs, hash mismatch, future proofs and expired proofs fail closed.

The Stage 4B status policy is untouched: authenticated, applicable evidence aged
0–900 seconds inclusive is eligible; older evidence is STALE. The trusted policy
may configure another maximum. Missing/unavailable or absent entry is UNKNOWN;
invalid binding/signature/future date is INVALID; fresh explicit revocation is
REVOKED. Untrusted status artifacts cannot relax freshness policy.

## Independent outcome semantics

The result embeds the complete Stage 4B result, retaining distinct cryptographic,
claim-binding, time, authority, status and legacy holder-assurance dimensions.
Stage 4C adds:

| Dimension                 | Values                                                                                   |
| ------------------------- | ---------------------------------------------------------------------------------------- |
| `verifierAuthentication`  | AUTHENTICATED, UNKNOWN, INVALID                                                          |
| `disclosureAuthorization` | AUTHORIZED, DENIED, UNKNOWN                                                              |
| `holderAuthorization`     | AUTHORIZED, DENIED, UNKNOWN on holder; NOT_OBSERVABLE on verifier                        |
| `holderPossession`        | VALID, INVALID, UNKNOWN                                                                  |
| `transactionBinding`      | VALID, INVALID, REPLAYED, UNKNOWN                                                        |
| `overall`                 | DENIED; holder-only AUTHORIZED_TO_PRESENT; receiver-only ACCEPTED_SYNTHETIC_PRESENTATION |

AUTHENTICATED does not imply AUTHORIZED. Holder authorization is evaluated locally
before release. A receiver **cannot prove human consent** from the KB-JWT and does
not accept forwarded unsigned consent as evidence. Its NOT_OBSERVABLE result is
explicit, rather than a false claim that every human authorization is established.
A holder decision is independently testable from receiver acceptance. The
receiver acceptance label is synthetic and bounded; it is not an unqualified
VERIFIED claim, real employer authority or assurance of legal subject identity.

## Replay execution model and threat boundaries

Go's mutex-protected store and JavaScript's synchronous post-verification
check-and-mark provide one atomic consumption point. Tests cover sequential
replay, concurrent duplicates (32 Go goroutines), cross-verifier/transaction
substitution and invalid submissions followed by valid ones. Only one duplicate
may succeed. Each verifier owns its original, locally registered transaction.
A fixture CLI session seeds those **trusted** contexts externally; it must never
be exposed with attacker-controlled transaction/registry inputs.

Stores are single-process, in-memory, and intended for bounded local experiments.
Restart discards outstanding transactions; callers must not restore/reseed
consumed challenges. There is no distributed or crash-persistent replay claim.
Production deployment requires an atomic persistent store, secure transaction
creation/lookup, bounded retention, fresh challenges, and a real consent boundary.
These are integration requirements, not implemented production infrastructure.

Threats tested include copied credentials, wrong holder keys, signature tampering,
request impersonation, untrusted metadata/keys, permission denial, absent consent,
excess disclosure, stale status, transaction substitution and repeated submissions.
Malicious trusted roots, compromised holder/issuer keys, coercion, endpoint malware,
colluding authorized recipients and a dishonest authorized issuer are not solved
by this cryptography. Full disclosure permits recipient correlation. This
increment does not establish holder key custody, recovery, rotation, or lifetime
validity. The verifier cannot prevent a recipient retaining already disclosed data.

## Normative traceability and bounded conformance

These are specific requirements implemented in a bounded subset, not claims of
full protocol, wallet, JSON-LD VC or certification conformance.

| Source                                                                                                                                                                                                        | Applied requirement or guidance                                                                                                           | Implementation / validation                                                                                                                                                        |
| ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| [RFC 9901](https://www.rfc-editor.org/rfc/rfc9901.html) §§4.1.2, 4.3, 4.3.1, 7.3                                                                                                                              | Issuer-protected confirmation key; signed KB-JWT; exact presentation hash; audience, nonce and temporal checks                            | Both presentation libraries; issuer possession tests; positive A/B, copied credential, incorrect key, hash/audience/nonce/time cases                                               |
| [OpenID4VP 1.0 Final](https://openid.net/specs/openid-4-verifiable-presentations-1_0-final.html) §§5, 5.8, 5.9.2, 6–8, 14–15                                                                                  | Authenticated Request Object; pre-registered client metadata; request/response transaction binding; cryptographic holder binding; privacy | Pinned registry, exact URI, strict DCQL; holder denial, unknown/impersonated verifier, cross-transaction and replay cases. No HTTP authorization endpoint or transport implemented |
| [OpenID4VCI 1.0 Final](https://openid.net/specs/openid-4-verifiable-credential-issuance-1_0-final.html) Appendix F.1                                                                                          | JWT key proof, explicit type, asymmetric key, issuer audience and nonce                                                                   | `IssueBound`/`issueBound` local proof seam; no OAuth issuance server or claim authorization implementation                                                                         |
| [W3C VC Data Model 2.0](https://www.w3.org/TR/vc-data-model/)                                                                                                                                                 | Distinct issuer/holder/verifier roles; trust and data minimization principles                                                             | Explicit independent determinations and disclosure denial; SD-JWT profile remains unchanged, no JSON-LD conformance claim                                                          |
| [Digital Credentials API](https://www.w3.org/TR/digital-credentials/)                                                                                                                                         | Holder-mediated release and privacy architectural guidance                                                                                | Trusted holder consent gate; Working Draft consulted 2026-11-26, not a mandatory browser dependency or implemented browser integration                                             |
| [Google online verification](https://developers.google.com/wallet/identity/verify/accepting-ids-from-wallet-online), [registrar](https://developers.google.com/wallet/identity/verify/verifier-registrar)     | Supporting ecosystem guidance on identified verifiers and requests                                                                        | Confirms distinction between verifier identity and entitlement; no registrar/Wallet integration claimed                                                                            |
| [Apple IdentityDocumentServices](https://developer.apple.com/documentation/identitydocumentservices), [OpenID4VPRequest](https://developer.apple.com/documentation/identitydocumentservices/openid4vprequest) | Supporting holder-mediated request/presentation guidance                                                                                  | Architectural compatibility only; no Apple API or product integration                                                                                                              |

Cvera-specific choices are the static synthetic registry, full-disclosure policy,
300-second request/proof limits, 22-character minimum challenge representation,
explicit synthetic result label and single-process store. The 900-second status
policy was owner-approved in Stage 4B. These are not universal standards mandates.

## Reproduce and inspect evidence

Install existing locked root dependencies (`npm ci`), use Go 1.26.10 and cached
Node 20 Docker image, then run from the repository root:

```sh
python3 interoperability/stage4c/run.py --isolated
python3 interoperability/stage4b/run.py --isolated
cd apps/evt-api-go
GOTOOLCHAIN=go1.26.10 go test -race ./internal/holderpresentation -count=1
GOTOOLCHAIN=go1.26.10 go vet ./...
```

The new runner first generates a single intended credential with ephemeral keys
in a separate Node 20 process. Only public artifacts survive that process. It
then runs independent JS and static Go verifiers in a network-disabled,
read-only, unprivileged container with one public input mount and no repository,
private keys, sockets, accounts or databases. Cases use independent stores except
explicit replay sequences. Concurrent results are compared as sets because the
winner is intentionally nondeterministic. Synthetic signatures vary between runs;
verification time, case definitions, expected outcomes and summary are reproducible.

No newly generated signed JWT or private key is committed. Generated public
fixtures exist only in a temporary directory deleted by the runner. The generator
uses the established Stage 4B **public synthetic claim template**, not a genuine
employment claim. Negative tokens are adversarial test artifacts, not new
successful issuances. Summary evidence contains case names and results only.
The previous GitGuardian exception is not extended to Stage 4C.

Durable evidence and the 21-criterion scorecard are in
[`docs/validation/stage4c`](../validation/stage4c). Local validation artifacts precede final integration authorization. Remote CI and
release verification are recorded in the integration pull request closure record.
The workflow addition runs the same isolated Node 20 test and archives its result
when subsequently authorized for integration.
