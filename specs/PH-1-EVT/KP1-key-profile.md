# **KP-1 — Issuer Key Profile (Final, PH-1 EVT)**

**Scope:** Defines how issuer signing keys are created, identified, published, and rotated for the Attested Identity system, beginning with PH-1 EVT.
**Status:** Final (PH-1 EVT)
**Audience:** Issuers, Verifiers, Security Engineers
**Depends on:** PH-1-EVT, RP-1, Deployment Guide (`docs/deployment.md`)
**Version:** 1.0.0

---

## **1. Purpose**

KP-1 is the canonical **Key Profile** for PH-1, where *attestations = EVT* (`employment.verification.v1`).

It defines:

* The **key material inventory** for issuer signing keys.
* How keys are **generated** and stored in `keys/*.pem`.
* How the **JWKS** at `trust/jwks.json` is built and structured.
* How keys are **published** using `scripts/publish.ts` and `.github/workflows/publish.yml`.
* How KP-1 relates to:

  * **PH-1 EVT**: how issuer keys sign employment verification claims.
  * **RP-1**: how revocation interacts with key state.
  * **Deployment Guide**: environment-specific behavior (dev/stage/prod).

KP-1 is **normative** for all systems that issue or verify EVT attestations under Phase 1 and remains forward-compatible with future phases.

---

## **2. Key Material Inventory**

Issuer keys live under:

* Private keys: `keys/*.pem`
* Public JWKS: `trust/jwks.json`

### **2.1 Current Dev Issuer Keys**

Each `kid` is derived from the PEM filename:

* `keys/issuer-dev-key-1.pem` → `kid: "issuer-dev-key-1"`
* `keys/issuer-dev-key-2.pem` → `kid: "issuer-dev-key-2"`
* `keys/issuer-dev-key-3.pem` → `kid: "issuer-dev-key-3"`

JWKS entries share:

* `kty: "RSA"`
* `alg: "RS256"`
* `use: "sig"`
* `kid: <pem filename without .pem>`

> **Normative:** Dev keys MUST NOT be used in production.
> Production keys MUST be distinct and must never share private key material with dev/test.

---

## **3. JWKS Construction (`trust/jwks.json`)**

### **3.1 Source of Truth**

JWKS MUST be produced exclusively by:

* `scripts/make-jwks.mjs`

Inputs: all `keys/issuer-dev-key-*.pem`
Output: `trust/jwks.json`

### **3.2 KID Derivation**

`kid = basename(<pemPath>).replace(/\.pem$/i, "")`

Changing filenames changes `kid` → **breaking change**.

> **Normative:** Never rename PEM files for keys still in use.
> Add new keys instead of renaming old ones.

### **3.3 Base64URL Normalization**

`make-jwks.mjs` strips `=` padding to ensure JWKS uses proper base64url encoding.

> **Normative:** Never reintroduce `=` padding.

---

## **4. Key Generation (`scripts/dev-keygen.sh`)**

`scripts/dev-keygen.sh` is the **only supported** path for generating dev keys.

Naming pattern:

```
keys/issuer-dev-key-<N>.pem
```

> **Normative:**
>
> * All dev keys MUST follow this filename pattern.
> * Each filename defines the `kid`; do not recycle filenames.

---

## **5. Key Roles & Usage**

### **5.1 Issuer Signing Keys**

* **Role:** Sign **EVT** (`employment.verification.v1`) JWS payloads.
* **JWKS fields:**

  * `use: "sig"`
  * `alg: "RS256"`

**Key selection workflow:**

* Issuer selects a private key and sets the `kid` in the JWS header.
* Verifier resolves the same `kid` from `trust/jwks.json`.

> **Normative:**
>
> * All EVT attestations MUST specify a valid `kid`.
> * Selected keys MUST NOT be revoked or retired per Deployment Guide.

---

### **5.2 Rotating Keys vs Revoking Claims**

* **Key rotation:** Introduce a new signing key and phase out old ones.
* **Claim revocation:** Revoke individual attestations (RP-1).

> **Normative:**
>
> * Routine rotations SHOULD append new keys without removing old ones until no EVT requires them.
> * Compromised keys MUST be removed and all dependent attestations MUST be marked revoked in `trust/statuslist.json`.

---

## **6. Publishing & Distribution**

### **6.1 GitHub Actions Workflow**

`.github/workflows/publish.yml`:

1. Checkout
2. Setup Node
3. `npm ci`
4. `npx tsx scripts/publish.ts`
5. Optional CDN purge via `doctl`

### **6.2 Responsibilities of `scripts/publish.ts`**

Must:

* Upload `trust/jwks.json` to DigitalOcean Spaces
* Serve publicly via CDN:

```
https://hapis.sfo3.cdn.digitaloceanspaces.com/trust/jwks.json
```

> **Normative:**
> Verifiers MUST treat this CDN URL as authoritative.

### **6.3 CDN Purge**

Workflow may flush:

```
/trust/jwks.json
/trust/statuslist.json
```

> **Normative:** Purge MUST follow any JWKS or revocation update.

---

## **7. Operational Procedures**

### **7.1 Adding a New Dev Issuer Key**

1. `scripts/dev-keygen.sh`
2. Run JWKS rebuild
3. Commit PEM + updated JWKS
4. Publish
5. Confirm CDN availability

### **7.2 Retiring a Key**

1. Stop using key for new EVT.
2. Keep in JWKS until all affected attestations expire.
3. Remove from JWKS only after safe.

### **7.3 Emergency Compromise**

1. Stop using key
2. Remove from JWKS
3. Identify affected attestations
4. Revoke via `trust/statuslist.json` (RP-1)
5. Publish + CDN purge

---

## **8. Security Considerations**

* Private keys MUST remain secret.
* Dev keys MAY exist in repo but MUST NOT be reused for prod.
* Production keys MUST come from secured secret management.
* Audit logs MUST record:

  * Which `kid` signed each EVT
  * JWKS changes

---

## **9. Relationship to PH-1 EVT, RP-1, and Deployment Guide**

### **PH-1 EVT**

* Defines the canonical EVT schema:

  * `schemas/PH-1-EVT/employment.verification.v1.json`
  * `docs/PH-1-EVT/PH-1-EVT.md`
* Specifies required JWS structure and field constraints.
* Depends on KP-1 for:

  * Valid `kid` resolution
  * Signature algorithm (`RS256`)
  * Trust chain rules

### **RP-1 (Revocation Profile)**

* Defines revocation semantics using `trust/statuslist.json`.
* Interacts with KP-1 during key compromise or mass-revocation.

### **Deployment Guide**

* Defines environment-specific lifetimes, secrets, JWKS endpoints.
* KP-1 remains environment-agnostic; Deployment Guide specializes it.

> **Normative:**
> Deployment customization MUST NOT contradict KP-1 key semantics.

---
