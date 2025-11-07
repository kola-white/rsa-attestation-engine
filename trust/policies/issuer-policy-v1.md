# Issuer Policy v1

**Purpose.** Define requirements for organizations that issue human-claim attestations.

## 1. Scope
Applies to all issuer-operated keys, services, and processes used to sign JWS attestations and publish JWKS + status lists.

## 2. Normative Requirements
- **Authorization:** Only authenticated admins (WebAuthn/passkeys or client cert) may invoke `/issue`, `/revoke`, `/rotate`.
- **Key Storage:** Private keys must be HSM/KMS in prod; software-only allowed in dev.
- **Algorithms:** `RS256` (MVP); roadmap: `ES256`, `EdDSA`.
- **Validity Windows:** Attestations MUST include `issued_at`, `not_before`, `not_after`.
- **Revocation Pointer:** Every attestation MUST include `revocation.pointer` and `serial`.

## 3. Operational Controls
- Change management tickets for key ops.
- Dual control for root/issuing CA material.
- Daily integrity checks of `/.well-known/jwks.json` and `statuslist.json`.

## 4. Logging & Audit
- Sign audit logs for `/issue`, `/revoke`, `/rotate` (include request hash).
- Retention ≥ 365 days.

## 5. Exceptions
Documented, time-limited, with risk sign-off.

## 6. Versioning
Policy URI: `https://<issuer>/policies/issuer-v1`. Breaking changes bump major.
