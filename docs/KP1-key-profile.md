# KP-1 — Issuer Key Profile

**Scope:** Defines how issuer signing keys are created, identified, published, and rotated for the Attested Identity system.
**Status:** Final (MVP)
**Audience:** Issuers, Verifiers, Security Engineers
**Depends on:** AP-1 (Attestation Profile), RP-1 (Revocation Profile)
**Version:** 1.0.0  

---

## **1. Purpose**

KP-1 is the canonical **Key Profile** for the Attested Identity issuer.

It defines:

- The **key material inventory** for issuer signing keys.
- How keys are **generated** and stored in `keys/*.pem`.
- How the **JWKS** at `trust/jwks.json` is built and structured.
- How keys are **published** via `scripts/publish.ts` and `.github/workflows/publish.yml`.
- How KP-1 relates to:
  - **AP-1** (Attestation Profile): how keys sign claims.
  - **RP-1** (Revocation Profile): how revocation interacts with key state.
  - **Deployment**: environment-specific overrides (dev/stage/prod) and publish behavior.

KP-1 is **normative** for any component that issues or verifies human-claim attestations using this issuer.

---

## 2. Key Material Inventory

All issuer keys for the current implementation live under:

- Private keys: `keys/*.pem`
- Public JWKS: `trust/jwks.json`

### 2.1 Current Dev Issuer Keys

These key IDs (KIDs) are derived directly from the PEM filenames:

- `keys/issuer-dev-key-1.pem` → `kid: "issuer-dev-key-1"`
- `keys/issuer-dev-key-2.pem` → `kid: "issuer-dev-key-2"`
- `keys/issuer-dev-key-3.pem` → `kid: "issuer-dev-key-3"`

The corresponding JWKS entries are in `trust/jwks.json` and share the following properties:

- `kty: "RSA"`
- `alg: "RS256"`
- `use: "sig"`
- `kid: <filename-without-.pem>`

> **Normative:**  
> - These keys are **development** issuer keys and **MUST NOT** be used for production issuance.  
> - Production keys MUST have distinct `kid` values and MUST NOT share private key material with dev/test.

---

## 3. JWKS Construction (`trust/jwks.json`)

### 3.1 Source of Truth

The JWKS file **MUST** be produced by:

- `scripts/make-jwks.mjs`

Inputs:

- `keys/issuer-dev-key-1.pem`
- `keys/issuer-dev-key-2.pem`
- `keys/issuer-dev-key-3.pem`

Output:

- `trust/jwks.json`

### 3.2 KID Derivation

`make-jwks.mjs` derives the key ID as:

- `kid = basename(<pemPath>).replace(/\.pem$/i, "")`

Implications:

- Renaming a PEM file changes the `kid` in the JWKS.
- Any change to `kid` is a **breaking change** for verifiers relying on that `kid`.

> **Normative:**  
> - Do **not** rename PEM files for keys that are still in use.  
> - To introduce a new key, generate a new PEM (new filename) rather than renaming an existing one.

### 3.3 Base64URL Normalization

`make-jwks.mjs` strips `=` padding from `n` and `e` values before writing out the JWKS. This ensures:

- `n` and `e` are stored as **base64url-encoded** strings without trailing `=`.

> **Normative:**  
> - Any future tooling that reads/writes `trust/jwks.json` MUST preserve base64url encoding and **MUST NOT** reintroduce `=` padding.

---

## 4. Key Generation (`scripts/dev-keygen.sh`)

### 4.1 Canonical Entry Point

`scripts/dev-keygen.sh` is the **only supported** entry point for generating new dev issuer keys.

> **Normative:**  
> - To create a new dev issuer key, you MUST use `scripts/dev-keygen.sh` and place the resulting PEM in `keys/`.
> - Directly hand-editing PEMs or dropping arbitrary keys into `keys/` is discouraged and should be treated as an exception-only operation.

### 4.2 Naming Convention

- All dev issuer keys MUST follow the pattern:  
  `keys/issuer-dev-key-<N>.pem`  
  where `<N>` is a small integer or other stable suffix.
- The resulting `kid` MUST be stable and should not be recycled across unrelated keys.

> **Example (dev only):**  
> - `keys/issuer-dev-key-4.pem` → `kid: "issuer-dev-key-4"`

---

## 5. Key Roles & Usage

### 5.1 Issuer Signing Keys

- **Role:** Sign JWS attestations according to AP-1.
- **JWKS fields:**
  - `use: "sig"`  
  - `alg: "RS256"`
- **Key selection:**  
  - Issuer selects a private key (e.g., `issuer-dev-key-1.pem`) and sets `kid` in the JWS header.
  - Verifier looks up the same `kid` in `trust/jwks.json`.

> **Normative:**  
> - All attestations MUST specify a `kid` in the JWS header that matches a public key in the JWKS.  
> - The issuer MUST ensure that the selected key is **not revoked** and **currently valid** under Deployment Guide (e.g., not retired).

### 5.2 Rotating Keys vs Revoking Claims

- **Key rotation**: Replacing one signing key with another while old attestations remain valid.
- **Claim revocation**: Marking specific attestations as no longer valid (see RP-1).

> **Normative:**  
> - Routine key rotation SHOULD be handled by adding new keys to `trust/jwks.json` and phasing out old keys for **new** attestations, while leaving old keys in JWKS for verification until their attestations expire.  
> - Hard key compromise MAY require both:
>   - Removing the key from `trust/jwks.json`, and
>   - Marking affected serials as revoked in `status/statuslist.json` per RP-1.

---

## 6. Publishing & Distribution

### 6.1 GitHub Actions Workflow

Key publication is handled by:

- Workflow: `.github/workflows/publish.yml`
- Job: `publish`
- Trigger: `push` to `main` or manual `workflow_dispatch`

The workflow:

1. Checks out the repo.
2. Sets up Node (`node 20.19.5`).
3. Runs `npm ci`.
4. Invokes:  
   `npx tsx scripts/publish.ts`
5. Optionally purges CDN cache via `doctl` if `DO_PAT` and `CDN_ENDPOINT_ID` are present.

### 6.2 `scripts/publish.ts` Responsibilities

`publish.ts` is the canonical publishing script. At minimum, it MUST:

- Upload `trust/jwks.json` to the configured DigitalOcean Space:
  - `SPACES_BUCKET` = `${{ vars.DO_SPACE }}`
  - Endpoint: `https://sfo3.digitaloceanspaces.com`
- Ensure `trust/jwks.json` is readable via the CDN base URL:
  - `CDN_BASE_URL` = `https://hapis.sfo3.cdn.digitaloceanspaces.com`
  - Effective JWKS URL (for verifiers):
    - `https://hapis.sfo3.cdn.digitaloceanspaces.com/trust/jwks.json`
- Apply appropriate ACL so verifiers can fetch it without authentication (e.g., public-read or equivalent).

> **Normative:**  
> - Verifiers MUST treat `CDN_BASE_URL + "/trust/jwks.json"` as the **authoritative** JWKS endpoint for this issuer.  
> - Any change to the JWKS distribution path MUST be captured here in KP-1 and in Deployment Guide.

### 6.3 CDN Purge and Consistency

The workflow includes an optional step:

- `doctl compute cdn flush "$CDN_ENDPOINT_ID"` with:
  - `--files "/trust/jwks.json"`
  - `--files "/status/statuslist.json"`

This keeps the JWKS and status list aligned from a caching perspective.

> **Normative:**  
> - Any change to issuer keys (`trust/jwks.json`) or revocation status (`status/statuslist.json`) SHOULD be followed by a CDN purge.  
> - `publish.yml` MUST remain the single source of truth for how cache invalidation is performed.

---

## 7. Operational Procedures

### 7.1 Adding a New Dev Issuer Key

1. Generate a new key:
   - Use `scripts/dev-keygen.sh` to create `keys/issuer-dev-key-<N>.pem`.
2. Rebuild JWKS:
   - Run `node scripts/make-jwks.mjs` (or equivalent npm script).
3. Commit:
   - Commit the new PEM and updated `trust/jwks.json`.
4. Publish:
   - Push to `main` or trigger `workflow_dispatch`.
5. Verify:
   - Confirm JWKS is live at `CDN_BASE_URL/trust/jwks.json`.

### 7.2 Retiring a Key (Soft Retirement)

1. Stop using the key for new attestations.
2. Keep the key in `trust/jwks.json` until all attestations it signed have:
   - Naturally expired, or
   - Been superseded under AP-1.
3. Only then remove the key from `trust/jwks.json`.

### 7.3 Handling Key Compromise (Emergency)

1. Immediately stop using the compromised key.
2. Remove the corresponding entry from `trust/jwks.json`.
3. Identify all attestations signed with the compromised key.
4. Apply RP-1 to mark affected serials as revoked in `status/statuslist.json`.
5. Rebuild and publish:
   - Regenerate `trust/jwks.json` and run `scripts/publish.ts`.
   - Ensure CDN purge for `/trust/jwks.json` and `/status/statuslist.json`.

---

## 8. Security Considerations

- Private keys in `keys/*.pem` MUST be treated as sensitive secret material.
- Dev keys MAY be committed to the repo for local workflows, but:
  - MUST NOT be reused in production.
  - SHOULD be clearly labeled as dev-only.
- Production keys:
  - SHOULD be provisioned via secure secret management (not committed).
  - MUST have their own JWKS, KIDs, and publication path defined in Deployment Guide.
- Audit logging MUST record:
  - Which `kid` was used to sign each attestation.
  - Any changes to the JWKS (add/remove keys).

---

## 9. Relationship to AP-1, RP-1, and Deployment Guide

- **AP-1** (Attestation Profile):
  - Defines how JWS attestations are shaped and which header/body constraints apply.
  - Relies on KP-1 for `kid` resolution and `alg`/`use` expectations.

- **RP-1** (Revocation Profile):
  - Governs per-attestation revocation via `status/statuslist.json`.
  - Coordinates with KP-1 when a key compromise requires mass revocation.

- **Deployment Guide** (Deployment Guide):
  - Specializes KP-1 for specific environments:
    - Dev, Staging, Production.
  - May define separate JWKS endpoints and key lifetimes per environment.

> **Normative:**  
> - KP-1 MUST remain environment-agnostic and focus on **key semantics**.  
> - Environment-specific details (e.g., key lifetimes, vault secrets) belong in Deployment Guide.

---