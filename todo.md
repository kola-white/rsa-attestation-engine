# TODO – Engineering Work Plan (MVP)

This file tracks pending work, future tasks, and engineering actions that are **not yet in the repo**.

---

## ✅ Near-Term (Blockers for MVP)

### 1. Generate dev JWKS (3 fake RSA keys)
- Create `jwks.json` under `/trust/`.
- Use naming scheme (finalize):
  - `issuer-dev-key-1`
  - `issuer-dev-key-2`
  - `issuer-dev-key-3`
- Ensure each key has:
  - `kid`
  - modulus/exponent
  - `alg`: RS256
  - `use`: sig

### 2. Create `statuslist.json`
- Include:
  - revoked: `att-SN-000389`
  - good: `att-SN-000777`, `att-SN-000045`
- `ttl_s`: 900
- Version header

### 3. Update Golden Vectors to reference real keys
- Replace placeholder `kid` with real JWKS entries.
- Ensure revocation serials match `statuslist.json`.
- Add three additional vectors:
  - `golden-invalid-signature.attestation.json`
  - `golden-invalid-schema.attestation.json`
  - `golden-notyetvalid.attestation.json`
  - `golden-invalid-liveness.attestation.json`

---

## ✅ Medium-Term (MVP Issuer + Verifier)

### 4. Implement Issuer API (cmd/issuer)
- `/issue`
- `/revoke`
- `/rotate`
- Signed audit logs
- Schema validation

### 5. Implement Verifier API (cmd/verifier)
- JWKS caching (fail-closed)
- Signature verification
- Revocation checking
- Liveness window enforcement
- Structured VerificationReceipt output

### 6. Dev Key Management
- Script `scripts/dev-keygen.sh`:
  - generate RSA keys
  - output JWKS entries
  - output PEM archive for local testing

---

## ✅ Longer-Term (MVP+)

### 7. Storage Layer
- Lightweight trust directory hosting (local filesystem first)
- Later: S3/GCS + CloudFront/CDN

### 8. Selective Disclosure (optional)
- SD-JWT or Merkle-based selective claim exposure

### 9. Optional: DP-1 (Tie-Breaker / Disambiguation Protocol)
- Only after MVP attestation pipeline is stable

---

## ✅ Notes
- TODO items are intentionally not reflected in the main `CHANGELOG.md`.
- This file tracks engineering work, discussions, and execution sequencing.
