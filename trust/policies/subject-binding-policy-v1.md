# Subject Binding Policy v1

**Binding Modes.** `pubkey`, `identifier`, or `both`.

## 1. Requirements
- PII-minimize identifiers (e.g., email, HR ID, DID).
- If `pubkey` binding is used, include `pubkey_thumbprint` (base64url).

## 2. Matching Rules
- Verifiers MUST confirm claimed subject matches the relying-party context (e.g., same email domain or wallet pubkey).
