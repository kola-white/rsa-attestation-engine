# Attested Identity – v0.1 (MVP)

A minimal, production-minded **attestation layer** that lets issuers (employers/institutions) **sign verifiable credentials** about a subject (candidate), and verifiers (ATS/recruiters) **validate** them with PKI-style trust. This is the resume-replacement core.

**Spec status:** AP-1 (Attestation Processing) · DP-1 (Disambiguation Protocol) (optional for MVP). We have these in Google Docs (my Drive)
**Crypto baseline:** JWS (RS256), JWKS for issuer keys, JSON Status List for revocation
**Privacy:** PII-minimal; selective disclosure reserved (SD-JWT/Merkle) for later versions

---

## Contents

* [Goals](#goals)
* [Architecture (MVP)](#architecture-mvp)
* [Data Schemas](#data-schemas)
* [Services & Endpoints](#services--endpoints)
* [Quickstart (dev)](#quickstart-dev)
* [Golden Test Vectors](#golden-test-vectors)
* [Verifier SDK Shape](#verifier-sdk-shape)
* [Security Notes](#security-notes)
* [Roadmap](#roadmap)
* [License](#license)

---

## Goals

* **Issue** signed attestations (employment role, skills, outcomes) bound to a subject.
* **Verify** integrity, chain of trust, validity window, and revocation status.
* **Revoke/Rotate** with short-TTL cache and fail-closed verification.
* **Audit** every issue/verify/revoke with signed logs.
* Keep friction low; no résumés required.

---

## Architecture (MVP)

```
Issuer API ──signs──► Attestation JSON
    │                       │
    │ publishes keys        ▼
    ├──► Trust Directory (JWKS, statuslist.json, policies, schemas)
    │                       │
Verifier API ◄─fetches───┘   │
    │                       │
    └──► VerificationReceipt (VALID/REVOKED/EXPIRED + reasons)
```

* **Trust Directory** is public: serves **JWKS** (issuer public keys), **statuslist.json** (revocations), **policy** URIs, and **schema** files.
* **Issuer API** holds private keys (dev: software; prod: KMS/HSM later).
* **Verifier API/SDK** validates JWS, chain, times, and revocation.

---

## Data Schemas

Place schemas under `./schemas/` (update paths if different):

* `schemas/attestation.schema.json`
* `schemas/verification-receipt.schema.json`
* `schemas/revocation-event.schema.json`

Each schema is versioned by `schema_uri` in the payload (e.g., `https://…/employment.role/v1`).

---

## Services & Endpoints

### Issuer Service

**POST `/issue`** → `Attestation`

* Validates claim against `schema_uri`
* Binds subject (`pubkey` and/or identifier)
* Signs JWS with `kid` + `alg` (RS256 default)
* Emits signed audit log

**POST `/revoke`** → `RevocationEvent`

* Reason codes: `key_compromise | cessation | superseded | administrative`
* Updates `statuslist.json` in Trust Directory
* Emits signed audit log

**POST `/rotate`** → `{ ok: true, new_kid }`

* Publishes updated JWKS to Trust Directory (grace window supported)

### Trust Directory (public)

**GET `/.well-known/jwks.json`** → issuer public keys
**GET `/statuslist.json`** → revocation/status entries
**GET `/policies/:name`** → human-readable/JSON policy
**GET `/schemas/:name`** → JSON Schema files

### Verifier Service

**POST `/verify`** → `VerificationReceipt`

* Inputs: attestation JSON (or URL/pointer)
* Validates sig, chain, schema, liveness, revocation
* Returns structured result + audit signature

---

## Quickstart (dev)

1. **Generate a dev keypair & JWKS**

   ```bash
   # example script path — replace with your own
   ./scripts/dev-keygen.sh  # outputs jwks.json and kid
   ```

2. **Seed Trust Directory**

   ```
   public/.well-known/jwks.json
   public/statuslist.json        # start with empty list: []
   public/policies/attest-v1.json
   public/schemas/attestation.schema.json
   public/schemas/verification-receipt.schema.json
   public/schemas/revocation-event.schema.json
   ```

3. **Issue an attestation (example)**

   ```bash
   curl -s -X POST http://localhost:8080/issue \
     -H "Content-Type: application/json" \
     -d @examples/attestation_input.json > out/attestation.json
   ```

4. **Verify**

   ```bash
   curl -s -X POST http://localhost:8081/verify \
     -H "Content-Type: application/json" \
     -d @out/attestation.json | jq
   ```

5. **Revoke**

   ```bash
   curl -s -X POST http://localhost:8080/revoke \
     -H "Content-Type: application/json" \
     -d '{"attestation_id":"att-ULID-01HXYZ...","reason_code":"administrative"}'
   # statuslist.json should update; subsequent verifies return REVOKED
   ```

---

## 'Golden Rules' to test against

### **Valid**

* `not_before` ≤ now ≤ `not_after`
* Signature **valid**, issuer `kid` present in JWKS
* `statuslist.json` shows **good**
  **Expected:** `VerificationReceipt.result = "VALID"`

### **Expired**

* now > `not_after`
  **Expected:** `result = "INVALID"` with `liveness_check.state = "EXPIRED"`

### **Revoked**

* `statuslist.json` contains `{ attestation_serial, status:"revoked" }`
  **Expected:** `result = "INVALID"` with `revocation_check.status = "revoked"`

Add these under `./examples/` and reference them in a quick CI test.

---

## Verifier SDK Shape

TypeScript sketch:

```ts
type VerifyInput = { attestation: object } | { url: string };

type VerificationReceipt = {
  id: string;
  attestation_id: string;
  result: "VALID" | "INVALID";
  signature_check: { valid: boolean; alg: string; kid: string };
  chain_check: { trusted: boolean; path: string[] };
  liveness_check: { state: "ACTIVE" | "EXPIRED" | "NOT_YET_VALID" };
  revocation_check: { status: "good" | "revoked" | "unknown"; source: string; cache_ttl_s: number };
  schema_check: { valid: boolean; schema_uri: string };
  binding_check: { bound: boolean; method: string };
  audit: { resolver_signature: string; policy_version: string; request_hash: string; time: string };
};
```

---

## Security Notes

* **Fail closed** after revocation TTL expires (stale cache → INVALID).
* Enforce **clock-skew tolerance** (e.g., ±5 minutes).
* Keep issuer private keys **in KMS/HSM** in prod; rotate regularly (publish new `kid`).
* Always emit **signed audit logs** for `issue/verify/revoke`.
* Minimize PII in payloads; prefer hashed/opaque identifiers.

---

## Roadmap

* **Ed25519/ECDSA** support (crypto agility)
* **Selective disclosure** (SD-JWT or Merkle proofs)
* **Candidate wallet** ("purple badge" to import/export, shareable across digital touchpoints. Physical format could be a credential "business card" for mobility)
* **Badge/viewer** (embeddable trust path UI)
* **DP-1 integration** (Disambiguation Protocol: tie-breakers)
* **ATS plugins** (LinkedIn/Greenhouse/Lever/Workday)
* **Governance console** (Issuer Trust Score / policy management)

---

## License

MIT (proposed) 

---

### Maintainers

* Spec/architecture: Whitehouse
* PKI review: Coe

---
