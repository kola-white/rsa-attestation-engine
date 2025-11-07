# Key Rotation Policy v1

**Cadence.** Issuer signing keys rotate ≤ 90 days (MVP can be manual).

## 1. JWKS Publication
- Publish new key with unique `kid`.
- Keep prior key active for grace window (e.g., 14 days) for in-flight issuance.

## 2. Deactivation
- After grace, mark previous `kid` inactive in JWKS; do not reuse `kid`.

## 3. Emergency Rotation
- Immediate JWKS update; broadcast incident notice; consider blanket revocations for at-risk attestations.
