# Revocation Policy v1

**Methods.** JSON Status List at `https://<issuer>/statuslist.json`.

## 1. Reasons
- `key_compromise` | `cessation` | `superseded` | `administrative`.

## 2. Timeliness
- Publish within 15 minutes of decision (target).
- Verifiers must treat status unknown/stale as `INVALID`.

## 3. Evidence
- Record `RevocationEvent` (signed), retain ≥ 365 days.
