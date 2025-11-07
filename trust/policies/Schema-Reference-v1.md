# Schema Reference v1 (Frozen for MVP)

**Status:** Frozen for MVP. Structural changes require a new major version.

## 1. Canonical Schemas
- `schemas/attestation.schema.json` → `schema_uri: https://<issuer>/schemas/attestation/v1`
- `schemas/verification-receipt.schema.json` → `.../verification-receipt/v1`
- `schemas/revocation-event.schema.json` → `.../revocation-event/v1`

## 2. Stability Rules
- Field names are stable.
- New optional fields allowed (minor).
- Changing existing field semantics → major version.
- Removal of fields → major version.

## 3. Identifiers
- `id` fields SHOULD be ULIDs/UUIDs.
- `kid` MUST be unique per key rotation.
- `serial` is issuer-scoped attestation serial.

## 4. Timestamps
- RFC 3339, UTC (`Z`).
- Clock-skew tolerance: ±5 minutes (verifier policy).

## 5. Revocation
- `revocation.pointer` points to issuer `statuslist.json`.
- `revocation.serial` MUST map to entry in status list.

## 6. Hashing & Signatures
- `hash.payload_alg`: `SHA-256`.
- `signature.sig_alg`: `RS256` (MVP).

## 7. Examples
See `/examples/` for golden vectors:
- valid.json
- expired.json
- revoked.json
- invalid-signature.json
- invalid-schema.json
- invalid-liveness.json
