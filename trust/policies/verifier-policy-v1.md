# Verifier Policy v1

**Purpose.** Define verification behavior for relying parties (ATS/HR systems).

## 1. Required Checks
- Fetch JWKS; match `kid`; validate JWS signature.
- Validate trust chain (issuing CA → root CA).
- Enforce `not_before` / `not_after` with ±5 min skew.
- Check `statuslist.json` (fail closed on stale cache).
- Validate `schema_uri` against canonical schemas.

## 2. Caching
- JWKS cache ≤ 1 hour.
- Status list cache ≤ 15 minutes (configurable).

## 3. Results
- Emit `VerificationReceipt` with `VALID` or `INVALID` (include reasons).
- Sign receipts when stored server-side.

## 4. Versioning
Policy URI: `https://<issuer>/policies/verify-v1`.
