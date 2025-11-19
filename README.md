# ✅ **README.md — Attested Identity (v0.3, MVP)**

# Attested Identity – v0.3 (MVP)

A minimal, production-minded **attestation layer** that allows issuers (employers, institutions) to **sign verifiable claims** about a person (roles, skills, contributions), and allows verifiers (ATS systems, recruiters, HR platforms) to **validate** those claims via a modern PKI-backed trust fabric.

This is the core of a **resume-optional credential format** designed for the web era.

### ✅ Design Principles

* **Web-native**: JSON, JWS, JWKS, HTTPS primitives
* **PKI-anchored**: X.509 trust foundations, issuer keys in HSM/KMS
* **Low friction**: minimal integration requirements for issuers and ATSes
* **Long-lived**: credentials survive devices, accounts, and job changes
* **Privacy-aware**: minimal PII, selective disclosure planned

### ✅ Specification Status

* **AP-1 (Attestation Processing)** — complete
* **DP-1 (Disambiguation Protocol)** — draft (Google Docs)
* **Canonical JSON Objects** — complete (below)

### ✅ Cryptographic Baseline

* **JWS (RS256)** — human claim signing
* **JWKS** — issuer public-key distribution
* **JSON Status List** — revocation (CRL-like, W3C-aligned)

---

# Contents

* [Goals](#goals)
* [Architecture (MVP)](#architecture-mvp)
* [Device vs Human Identity Model](#device-vs-human-identity-model)
* [Layered Trust Stack](#layered-trust-stack)
* [Trust Contract Overview](#trust-contract-overview)
* [Data Schemas](#data-schemas)
* [Canonical JSON Objects](#canonical-json-objects)
* [Services & Endpoints](#services--endpoints)
* [Quickstart (dev)](#quickstart-dev)
* [Golden Rules (Test Cases)](#golden-rules-test-cases)
* [Verifier SDK Shape](#verifier-sdk-shape)
* [Security Notes](#security-notes)
* [Roadmap](#roadmap)
* [License](#license)
* [Maintainers](#maintainers)

---

# Goals

* **Issue** signed attestations about employment, skills, or outcomes
* **Verify** integrity, trust chain, validity window, and revocation
* **Revoke** via status lists, with fail-closed behavior
* **Rotate** issuer keys safely, without breaking existing attestations
* **Audit** issuance/verification/revocation events
* Keep friction low — **resume-free workflows** supported

---

# Architecture (MVP)

```
Issuer API ──signs──► Attestation JSON
    │                       │
    │ publishes keys        ▼
    ├──► Trust Directory (JWKS, statuslist.json, policies, schemas)
    │                       │
Verifier API ◄─fetches───┘  │
    │                       │
    └──► VerificationReceipt (VALID / INVALID / REVOKED / EXPIRED)
```

### Components

* **Issuer API**

  * Holds private keys (dev: local; prod: HSM/KMS)
  * Issues and revokes attestations
  * Publishes JWKS and revocation data

---

# 📁 Trust Directory (Source of Truth)

All trust artifacts are maintained in-repo under `./trust/`.
These files are **canonical**, versioned, and form the basis for what is later **published** to our production CDN (DigitalOcean Spaces / S3) under the public paths:

* `/.well-known/jwks.json`
* `/statuslist.json`
* `/policies/...`
* `/schemas/...`

**Repository structure:**

```
trust/
  jwks.json              # canonical issuer keyset (stable)
  statuslist.json        # canonical revocation list
  policy.json            # issuer policy metadata
  policies/              # versioned policy docs
  specs/                 # spec artifacts (AP-1, RP-1, KP-1, DP-1, etc.)
  TP-1-Trust-Path-Spec.md
  root-ca/               # offline root CA material (dev only)
  issuing-ca/            # intermediate & issuing CA keys/certs (dev only)
  audit-logs/            # signed audit events (non-prod)

```

**Note:**
At deploy time, a publish step syncs these artifacts to CDN endpoints so that verifiers can access:

```
https://issuer.example.com/.well-known/jwks.json
https://issuer.example.com/statuslist.json

```

The Git repo maintains the *authoritative sources*, not the final `.well-known` directory.

---

* **Verifier API**

  * Signature verification
  * Trust path evaluation
  * Validity and revocation checking
  * Outputs a structured **VerificationReceipt**

---

# Device vs Human Identity Model

Human-fact credentials persist for **years** and must be independent of devices.
Device identity is used only for **authentication** (e.g., issuer admin actions).

### ✅ Device Identity = Authentication Layer

“How do I know the admin pressing ‘issue’ actually belongs at the issuer?”

* WebAuthn
* FIDO2 passkeys
* TPM / Secure Enclave keypairs
* Optional client X.509 certs

Used for **admin login**, not attestation signing.

### ✅ Issuer Identity = PKI Authority

Backed by an **X.509 CA hierarchy**:

* **Root CA** (offline — “vampire in the crypt”)
* **Intermediate / Issuing CAs** (“vampire lieutenants”)
* Private keys stored in KMS/HSM in production

### ✅ Human Claim Identity = Long-Lived Facts

* JWS-signed JSON describing role, skills, outcomes
* Verified via JWKS, schema, and revocation
* Bound to subject public key and/or stable identifier

---

# Diagram: Device Identity vs Human Claim Identity

```
                           ┌───────────────────────────────┐
                           │         DEVICE IDENTITY       │
                           │      (Authentication Layer)   │
                           └───────────────────────────────┘
                                         │
                      proves possession ➜│
                      ▼                  ▼
    ┌────────────────────────┐   ┌─────────────────┐   ┌──────────────────────────┐
    │ User Device            │   │ Browser/FIDO2   │   │ Device-bound Keypair     │
    │ (Laptop/Phone)         │   │ WebAuthn/Passkey│   │ (Secure Enclave/TPM)     │
    └────────────────────────┘   └─────────────────┘   └──────────────────────────┘
                              │
                     ✅ Authenticates admin users
──────────────────────────────────────────────────────────────────────────────
   HUMAN CLAIM IDENTITY (Independent of Device; persists for years)
──────────────────────────────────────────────────────────────────────────────
                           ┌───────────────────────────────┐
                           │       ISSUER = AUTHORITY      │
                           │  (“Vampire in the Crypt”)     │
                           └───────────────────────────────┘
                                         │
                                 Offline Root CA
                                         │
                       Intermediate / Issuing CA Keys
                                         │
──────────────────────────────────────────────────────────────────────────────
                     JWS / JWKS ATTESTATION LAYER (Human Facts)
──────────────────────────────────────────────────────────────────────────────
                   - JSON claims: role, skill, outcomes
                   - JWS-signed using issuer private key
                   - Verified with issuer JWKS
──────────────────────────────────────────────────────────────────────────────
                           VERIFIER (ATS / Recruiters)
──────────────────────────────────────────────────────────────────────────────
       1. Fetch JWKS  
       2. Verify JWS  
       3. Check validity window  
       4. Check revocation  
       5. Evaluate trust chain  
```

---

# Layered Trust Stack

```
[ Layer 5 — Apps / HR Systems / ATS ]
    - Candidate wallets
    - Recruiter dashboards
    - Resume-optional workflows

[ Layer 4 — JWS Attestations ]
    - JSON claims
    - Signature envelopes (JWS)
    - Selective disclosure (future)

[ Layer 3 — JWKS Trust Directory ]
    - Public key distribution
    - Key rotation metadata
    - Revocation status list (JSON)

[ Layer 2 — Issuer CA Keys (X.509 PKI) ]
    - Offline root CA (“vampire in crypt”)
    - Intermediate / issuing CA keys
    - Encoded in `issuer.ca_chain`

[ Layer 1 — Device Identity ]
    - Passkeys / WebAuthn
    - TPM keypairs
    - Optional client certificates
```

---

# Trust Contract Overview

The Attestation Trust Contract governs:

✅ **What issuers may sign** (role, time, skills, outcomes).  
✅ **Required metadata** (issuer ID, CA chain, key metadata).  
✅ **Subject binding rules** (pubkey, identifier, or both).  
✅ **Validity windows** (`issued_at`, `not_before`, `not_after`).  
✅ **Revocation semantics** (status list entries, reason codes).  
✅ **Verifier responsibilities** (fail-closed behavior, TTL, clock skew).  

DP-1 (Disambiguation Protocol) builds the tie-break logic for large candidate sets but is **non-MVP**.

---

# Data Schemas

Schemas live in `./schemas/`:

```
schemas/
  attestation.schema.json
  verification-receipt.schema.json
  revocation-event.schema.json
```

Each schema is referenced by its `schema_uri`.

Schemas are responsible for validating:

* required fields
* enumerations
* timestamp formats
* nested objects
* key metadata
* revocation pointers

---

# Canonical JSON Objects

✅ These are the **authoritative** JSON structures for all endpoints.

## **1. Attestation (Signed Object)**

```jsonc
{
  "id": "string",                         // stable attestation ID (UUID/ULID)
  "schema_uri": "https://.../skill/v1",   // versioned claim schema
  "version": "1.0.0",

  "issuer": {
    "id": "did:org:apple",                // issuer identifier (DID/URI)
    "name": "Apple Inc.",                 // optional display value
    "ca_chain": ["urn:ca:root:...", "urn:ca:int:..."] // references to trust anchors
  },

  "key": {
    "kid": "apple-key-2025q1",            // key identifier (rotatable)
    "alg": "RS256"                        // cryptographic algorithm
  },

  "subject": {
    "binding": {
      "type": "pubkey|identifier|both",
      "pubkey_thumbprint": "BASE64URL...",      // if bound to subject's public key
      "identifier": "mailto:nick@example.com"   // HR ID, email, DID (PII-minimized)
    }
  },

  "claim": {
    "type": "employment.role",            // claim kind (role, skill.level, outcome.metric)
    "context": {
      "org_unit": "Vision Pro",
      "project": "CV pipeline",
      "location": "US-CA"
    },
    "value": {
      "title": "Lead Engineer",
      "skill": "Computer Vision",
      "level": "L4"
    }
  },

  "validity": {
    "issued_at": "2025-11-06T20:05:00Z",
    "not_before": "2025-11-06T20:05:00Z",
    "not_after": "2027-11-06T00:00:00Z"
  },

  "revocation": {
    "method": "status-list|endpoint",     // CRL/OCSP-style mechanisms
    "pointer": "https://issuer.tld/revocations/abcd.json",
    "serial": "att-3f2c..."
  },

  "policy": {
    "policy_uri": "internal/issuer/issuer-v1.md",
    "assurance": "TAL-3"                  // Trust Attestation Level
  },

  "disclosure": {
    "mode": "full|sd-jwt|merkle",
    "merkle_root": "BASE64URL...",        // only if selective disclosure used
    "disclosed_fields": [
      "claim.value.title",
      "claim.value.skill"
    ]
  },

  "hash": {
    "payload_alg": "SHA-256",
    "payload_hash": "BASE64URL..."        // hash of signed payload for detached signatures
  },

  "signature": {
    "mode": "attached|detached",
    "sig_alg": "RS256",
    "sig": "BASE64URL..."                 // JWS-style signature blob
  }
}
```

---

## **2. Verification Receipt**

```jsonc
{
  "id": "vr-ULID-01HABC...",
  "attestation_id": "att-ULID-01HXYZ...",
  "verifier": {
    "id": "did:org:acme-ats",
    "name": "Acme ATS"
  },
  "time": "2025-11-06T20:06:11Z",

  "signature_check": {
    "valid": true,
    "alg": "RS256",
    "kid": "apple-key-2025q1"
  },

  "chain_check": {
    "trusted": true,
    "path": [
      "urn:ca:int:apple-issuing-2025",
      "urn:ca:root:digicert-G3"
    ]
  },

  "schema_check": {
    "valid": true,
    "schema_uri": "https://.../employment.role/v1"
  },

  "liveness_check": {
    "state": "ACTIVE",
    "now": "2025-11-06T20:06:11Z"
  },

  "revocation_check": {
    "status": "good",
    "source": "https://apple.com/.../statuslist.json",
    "cache_ttl_s": 3600
  },

  "binding_check": {
    "bound": true,
    "method": "pubkey+identifier"
  },

  "result": "VALID",
  "reasons": [],

  "audit": {
    "resolver_signature": "BASE64URL...",
    "request_hash": "BASE64URL...",
    "policy_version": "verify-1.0.0"
  }
}
```

---

## **3. Revocation Event (Issuer Signed CA)**

```jsonc
{
  "id": "rev-ULID-01HDEF...",
  "attestation_id": "att-ULID-01HXYZ...",
  "issuer": {
    "id": "did:org:apple"
  },

  "reason_code": "key_compromise|cessation|superseded|administrative",
  "reason_text": "Role misattributed; corrected record issued",
  "time": "2026-04-03T09:22:00Z",

  "supersedes": "att-ULID-OLD123",        // optional: link to replacement attestation

  "signature": {
    "kid": "apple-key-2026q2",
    "alg": "RS256",
    "sig": "BASE64URL..."
  }
}
```

---

# Services & Endpoints

### Issuer Service

#### **POST `/issue`** → `201 Created`

* Validates schema
* Binds subject
* Signs JWS
* Emits signed audit log

#### **POST `/revoke`** → `201 Created`

* Updates `statuslist.json`
* Reason codes follow the trust contract
* Emits signed audit log

#### **POST `/rotate`** → `200 OK`

* Publishes new JWKS
* Supports grace period

Issuer authentication must use **device identity** (WebAuthn/passkeys/client certs).

---

### Trust Directory (public)

```
GET /.well-known/jwks.json
GET /statuslist.json
GET /policies/:name
GET /schemas/:name
```

All are cache-friendly with strong ETags.

---

### Verifier API

#### **POST `/verify`** → `200 OK`

Returns a structured **VerificationReceipt**:

* signature validity
* trust chain validity
* revocation state
* schema conformance
* binding check
* audit signature

---

# Quickstart (dev)

### Prerequisites

* Node 18+
* `curl`
* `jq`
* Bash / zsh
* `./scripts/dev-keygen.sh` (included)

---

### 1. Generate dev keypair + JWKS

```bash
./scripts/dev-keygen.sh
```

Produces:

```
public/.well-known/jwks.json
```

---

### 2. Seed Trust Directory

```bash
mkdir -p public/{.well-known,schemas,policies}
cp schemas/*.json public/schemas/
echo "[]" > public/statuslist.json
```

---

### 3. Issue an attestation

```bash
curl -s -X POST http://localhost:8080/issue \
  -H "Content-Type: application/json" \
  -d @examples/attestation_input.json \
  > out/attestation.json
```

---

### 4. Verify it

```bash
curl -s -X POST http://localhost:8081/verify \
  -H "Content-Type: application/json" \
  -d @out/attestation.json | jq
```

---

### 5. Revoke it

```bash
curl -s -X POST http://localhost:8080/revoke \
  -H "Content-Type: application/json" \
  -d '{"attestation_id":"att-ULID-01HXYZ...","reason_code":"administrative"}'
```

---

# Golden Rules (Test Cases)

### ✅ Valid

* Signature OK
* Now ∈ `[not_before, not_after]`
* Revocation state = good

### ✅ Expired

* Now > `not_after`

### ✅ Revoked

* `statuslist.json` marks serial as revoked

### ✅ Signature Invalid

* Wrong key, wrong algorithm, or tampering
* MUST result in `INVALID`

---

# Verifier SDK Shape (TypeScript)

```ts
type VerificationReceipt = {
  id: string;
  attestation_id: string;
  verifier: { id: string; name?: string };
  time: string;

  signature_check: { valid: boolean; alg: string; kid: string };
  chain_check: { trusted: boolean; path: string[]; depth?: number; root?: string };
  schema_check: { valid: boolean; schema_uri: string };
  liveness_check: { state: "ACTIVE" | "EXPIRED" | "NOT_YET_VALID"; now: string };
  revocation_check: { status: "good" | "revoked" | "unknown"; cache_ttl_s: number };
  binding_check: { bound: boolean; method: string };

  result: "VALID" | "INVALID";
  reasons: string[];

  audit: {
    resolver_signature: string;
    request_hash: string;
    policy_version: string;
  };
}
```

---

# Security Notes

* **Fail closed** on expired revocation TTL
* Enforce **clock-skew ±5 minutes**
* Private keys MUST be in **HSM/KMS** in production
* Audit logs must include **request body hash**
* Issuers must rotate keys **at least annually**

---

# Roadmap

* Ed25519 / ECDSA support
* Selective disclosure (SD-JWT / Merkle) - out of scope
* Candidate wallet (“purple badge”)
* LinkedIn-style trust viewer
* DP-1 disambiguation scoring
* ATS plugins (Greenhouse, Lever, Workday)
* Governance console for issuers

---

# License

MIT

---

# Maintainers

* **Spec/architecture:** Whitehouse
* **PKI review:** Coe
