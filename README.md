# Cvera | Verifiable Claim Tokens

**Claims. Proven.**

A prototype system for **user-held, cryptographically verifiable claims** that enables trusted issuers to create signed records about individuals, which can be independently verified without relying on direct access to the issuing authority.

Cvera defines a generalized framework for **portable, verifiable human claims** across domains.

## What this demo shows

- Issuance of a verifiable claim token  
- User-held credential (portable JSON)  
- Independent verification:
  - signature validation  
  - trust chain evaluation  
  - revocation checking  

## Claim types (illustrative)

This model supports a wide range of attestable claims, including:

- Professional employment roles and organizational relationships  
- Academic qualifications and certifications  
- Licenses and regulatory status  
- Legal identity changes  
- Achievements, affiliations, and records  

## Why this matters

As AI accelerates the creation of synthetic and unverifiable information, trust in claims about individuals is eroding. Yet, verification still relies on manual checks across fragmented systems.
Cvera explores a model where claims become:

- **Portable** — held by the individual  
- **Verifiable** — cryptographically provable  
- **Independent** — validated without contacting the issuer  
- **Auditable** — with clear trust and revocation semantics  

This shifts systems from **trusting claims** to **verifying proof**.

## Vision

A world where individuals hold their own verifiable records, and institutions rely on **independent verification rather than intermediaries** to establish truth.


# Attested Identity – PH-1 EVT (v0.1.0-evt)

A minimal, production-minded **attestation layer** that allows issuers (employers, institutions) to **sign verifiable employment records**, and allows verifiers (ATS systems, recruiters, HR platforms, background check tools) to **validate** those records via a modern PKI-backed trust fabric.

Phase 1 focuses on a single wedge:

> **Employment Verification Tokens (EVT)** – cryptographically signed confirmation that:
> **“This person worked here, in this role, during this time window, in this capacity.”**

Later phases (attested job descriptions, résumé tokens, student portfolios, admissions) build on the same protocol, but PH-1 EVT is the **current MVP**.

For a Phase-1 deep-dive, see:

📄 `docs/pre-wedge-README.md`

---

## Design Principles

* **Web-native**: JSON, JWS, JWKS, HTTPS primitives
* **PKI-anchored**: X.509 trust foundations, issuer keys in HSM/KMS
* **Low friction**: minimal integration for issuers and ATS/HR systems
* **Long-lived**: credentials survive devices, accounts, and job changes
* **Privacy-aware**: minimal PII, with future support for selective disclosure

---

## Specification Status (PH-1)

**Core protocol specs (Phase 1 – EVT):**

* **AP-1 (Attestation Processing)** – EVT shape + validation rules
  `specs/PH-1-EVT/`
* **KP-1 (Key Profile)** – key lifecycle and usage constraints
  `specs/PH-1-EVT/`
* **TP-1 (Trust Path Spec)** – trust chain evaluation
  `specs/PH-1-EVT/TP-1-Trust-Path-Spec.md`
* **RP-1 / POL-1 (Revocation & Verification Policy Layer)** – liveness, status list, policies
  `specs/PH-1-EVT/POL-1-Verification-Policy-Layer.md`, `trust/policies/`

**Cross-cutting / future:**

* **DP-1 (Disambiguation Protocol)** – draft, mainly for PH-3 résumé tokens
  `specs/DP-1/`
* **Generic employment.role / skills / outcomes schemas** – parked under
  `schemas/future/`

---

## Contents

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

## Goals

PH-1 EVT has a very constrained goal set:

* **Issue** signed employment verification tokens (EVT) for a subject:
  employer, role, start/end dates, employment type.
* **Verify**:

  * signature and trust chain
  * validity window
  * revocation state
* **Revoke** via JSON status lists (fail-closed behavior).
* **Rotate** issuer keys without breaking existing EVT records.
* **Audit** issuance / verification / revocation.
* Keep friction low – so EVT can realistically be piloted in real orgs.

Future phases (résumés, job descriptions, student records) build on top.

---

## Architecture (MVP)

```
Issuer API ──signs──► Employment Verification Token (EVT JSON)
│                      │
│ publishes keys       ▼
├──► Trust Directory (JWKS, statuslist.json, policies, schemas)
│                      │
Verifier API ◄─fetches─┘
│
└──► VerificationReceipt (VALID / INVALID / REVOKED / EXPIRED)
```

### Components

**Issuer API**

* Holds private keys (dev: local; prod: HSM/KMS)
* Issues and revokes EVT payloads
* Publishes JWKS and revocation data

**Verifier API**

* Performs signature verification
* Evaluates trust path and validity
* Checks revocation via JSON status list
* Returns a structured **VerificationReceipt**

---

## Trust Directory (Source of Truth)

Trust artifacts live under `./trust/`.

Canonical, versioned, and intended for publishing to CDN endpoints:

* `/.well-known/jwks.json`
* `/statuslist.json`
* `/policies/...`

### Repository Structure (simplified)

```
trust/
  jwks.json
  statuslist.json
  policy.json
  policies/
  root-ca/           # dev only
  issuing-ca/        # dev only
  audit-logs/
  changelog/
```

> Specs now live under `specs/PH-1-EVT/`, not `trust/specs/`.

Example published endpoints:

```
https://issuer.example.com/.well-known/jwks.json
https://issuer.example.com/statuslist.json
```

---

## Device vs Human Identity Model

Human-fact credentials persist for **years** and must be independent of devices.

### Device Identity = Authentication Layer

“How do I know the admin pressing ‘issue’ actually belongs at the issuer?”

* WebAuthn / FIDO2 passkeys
* TPM / Secure Enclave keypairs
* Optional client X.509 certs

Used for **admin login**, not attestation signing.

### Issuer Identity = PKI Authority

Backed by an **X.509 CA hierarchy**:

* **Root CA** (offline)
* **Intermediate / Issuing CAs**
* Private keys stored in HSM/KMS (prod)

### Human Claim Identity = Long-Lived Facts

Phase-1: **employment records**.

* JWS-signed JSON
* Verified via JWKS, schema, revocation
* Bound to subject identifiers (subject keybinding later)

---

## Diagram: Device Identity vs Human Claim Identity

```
┌───────────────────────────────┐
│ DEVICE IDENTITY               │
│ (Authentication Layer)        │
└───────────────────────────────┘
            │ proves possession
            ▼
┌────────────────────────┐  ┌─────────────────┐  ┌──────────────────────────┐
│ User Device            │  │ Browser/FIDO2   │  │ Device-bound Keypair     │
│ (Laptop/Phone)         │  │ WebAuthn/Passkey│  │ (Secure Enclave / TPM)   │
└────────────────────────┘  └─────────────────┘  └──────────────────────────┘
            │
            ▼ Authenticates admin users
──────────────────────────────────────────────────────────────────────────────
HUMAN CLAIM IDENTITY (Independent of Device; persists for years)
──────────────────────────────────────────────────────────────────────────────
┌───────────────────────────────┐
│ ISSUER = AUTHORITY            │
└───────────────────────────────┘
       │
Offline Root CA
       │
Intermediate / Issuing CA Keys
──────────────────────────────────────────────────────────────────────────────
JWS / JWKS ATTESTATION LAYER (Human Facts)
──────────────────────────────────────────────────────────────────────────────
- JSON claims
- JWS signatures
- Verified via JWKS
──────────────────────────────────────────────────────────────────────────────
VERIFIER
──────────────────────────────────────────────────────────────────────────────
1. Fetch JWKS
2. Verify JWS
3. Check validity window
4. Check revocation
5. Evaluate trust chain
```

---

## Diagram: Attested Identity — Protocol + Policy Stack

```
+------------------------------------------------------------------------------------+
| Relying Party (ATS / HR System)                                                    |
+------------------------------------------------------------------------------------+
       |
       v
+------------------------------------------------------------------------------------+
| Attestation Verification Pipeline                                                   |
+------------------------------------------------------------------------------------+

PROTOCOL STACK
----------------------------------------------------------------------------------------
AP-1 → KP-1 → TP-1 → RP-1 → DP-1
----------------------------------------------------------------------------------------

POLICY LAYER
----------------------------------------------------------------------------------------
POL-1 / VP-1 → final decision (ACCEPT / REVIEW / REJECT)
----------------------------------------------------------------------------------------
```

---

## Layered Trust Stack

```
[ Layer 5 — Apps / HR Systems / ATS ]
[ Layer 4 — JWS Attestations        ]
[ Layer 3 — JWKS Trust Directory    ]
[ Layer 2 — Issuer CA Keys (X.509)  ]
[ Layer 1 — Device Identity         ]
```

---

## Trust Contract Overview

The Attestation Trust Contract governs:

* **What issuers may sign** (PH-1: employment facts)
* **Required metadata** (issuer ID, CA chain, key metadata)
* **Subject binding rules**
* **Validity windows**
* **Revocation semantics**
* **Verifier responsibilities** (fail-closed, TTL, clock skew)

DP-1 matters later (résumé tokens), not PH-1.

---

## Data Schemas

```
schemas/
  common/
  PH-1-EVT/
    employment.verification.v1.json
  PH-2-JD/
  PH-3-RESUME/
    disambiguation.result.v1.json
  PH-4-STUDENT/
  PH-5-ADMISSIONS/
  future/
    employment.role.v1.json
```

Schemas validate required fields, timestamps, nested objects, key metadata, and revocation hooks.

---

## Canonical JSON Objects

### 1. Employment Verification Token (EVT)

```jsonc
{
  "request_id": "evt-ULID-01HXYZ...",
  "issuer": { "request_id": "did:org:acme-electric", "name": "Acme Electric, Inc." },
  "subject": { "full_name": "Jane Doe", "employee_id": "E12345" },
  "employment": {
    "title": "Project Manager",
    "start_date": "2020-01-15",
    "end_date": "2024-10-31",
    "employment_type": "full_time"
  },
  "validity": { "issued_at": "2025-11-06T20:05:00Z" },
  "signature": {
    "alg": "RS256",
    "kid": "acme-key-2025q4",
    "sig": "BASE64URL..."
  }
}
```

---

### 2. Verification Receipt

```jsonc
{
  "request_id": "vr-ULID-01HABC...",
  "attestation_id": "evt-ULID-01HXYZ...",
  "verifier": { "request_id": "did:org:acme-ats", "name": "Acme ATS" },
  "time": "2025-11-06T20:06:11Z",
  "signature_check": {
    "valid": true,
    "alg": "RS256",
    "kid": "acme-key-2025q4"
  },
  "chain_check": {
    "trusted": true,
    "path": [
      "urn:ca:int:acme-issuing-2025",
      "urn:ca:root:digicert-G3"
    ]
  },
  "schema_check": {
    "valid": true,
    "schema_uri": "https://.../PH-1-EVT/employment.verification.v1.json"
  },
  "liveness_check": { "state": "ACTIVE", "now": "2025-11-06T20:06:11Z" },
  "revocation_check": {
    "status": "good",
    "source": "https://acme.com/.well-known/statuslist.json",
    "cache_ttl_s": 3600
  },
  "binding_check": { "bound": true, "method": "identifier" },
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

### 3. Revocation Event

```jsonc
{
  "request_id": "rev-ULID-01HDEF...",
  "attestation_id": "evt-ULID-01HXYZ...",
  "issuer": { "request_id": "did:org:acme-electric" },
  "reason_code": "administrative",
  "reason_text": "Record superseded by corrected employment dates",
  "time": "2026-04-03T09:22:00Z",
  "supersedes": "evt-ULID-OLD123",
  "signature": {
    "kid": "acme-key-2026q2",
    "alg": "RS256",
    "sig": "BASE64URL..."
  }
}
```

---

## Services & Endpoints

### Issuer Service

#### `POST /issue` → `201 Created`

* Validate payload
* Bind subject
* Sign EVT
* Emit audit log

#### `POST /revoke` → `201 Created`

* Update status list
* Store revocation event
* Emit audit log

#### `POST /rotate` → `200 OK`

* Publish JWKS update
* Support key grace period

---

### Trust Directory (public)

```
GET /.well-known/jwks.json
GET /statuslist.json
GET /policies/:name
GET /schemas/:phase/:name
```

---

### Verifier API

#### `POST /verify` → `200 OK`

Returns a **VerificationReceipt** including:

* signature validity
* chain validation
* revocation state
* schema conformity
* binding check
* policy ruling

---

## Quickstart (dev)

### 1. Generate dev keypair + JWKS

```bash
./scripts/dev-keygen.sh
```

Outputs:

```
trust/jwks.json
trust/root-ca/
trust/issuing-ca/
```

---

### 2. Seed a local dev trust directory

```bash
mkdir -p public/{.well-known,schemas,policies}

cp trust/jwks.json public/.well-known/jwks.json
cp trust/statuslist.json public/statuslist.json
cp schemas/common/*.json public/schemas/
cp schemas/PH-1-EVT/*.json public/schemas/
cp trust/policies/*.md public/policies/
```

---

### 3. Issue an EVT

```bash
curl -s -X POST http://localhost:8080/issue \
  -H "Content-Type: application/json" \
  -d @examples/PH-1-EVT/employment.verification.input.json \
  > out/employment.verification.json
```

---

### 4. Verify it

```bash
curl -s -X POST http://localhost:8081/verify \
  -H "Content-Type: application/json" \
  -d @out/employment.verification.json | jq
```

---

### 5. Revoke it

```bash
curl -s -X POST http://localhost:8080/revoke \
  -H "Content-Type: application/json" \
  -d '{"attestation_id":"evt-ULID-01HXYZ...","reason_code":"administrative"}'
```

---

## Golden Rules (Test Cases)

### VALID

* Signature OK
* Validity window OK
* Revocation = `good`

### EXPIRED

* Past `not_after`

### REVOKED

* Status list marks revoked

### SIGNATURE INVALID

* Must return `INVALID`

---

## Verifier SDK Shape (TypeScript)

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
};
```

---

## Security Notes

* **Fail closed** on stale status list TTL
* Enforce **±5 min clock skew**
* Private keys MUST be in **HSM/KMS** in production
* Audit logs must include **request hash**
* Issuers must rotate keys **annually**

---

## Local Postgres (Docker)

* **Note:** 
* See docker-compose.yml for exact configuration (volumes, healthchecks, ports).
* All database access must go through a backend API, which uses `DATABASE_URL`.


### Start
```bash
docker volume create evt_pgdata

docker run --name evt-postgres \
  --restart unless-stopped \
  -e POSTGRES_PASSWORD=devpass \
  -e POSTGRES_DB=evt \
  -p 5432:5432 \
  -v evt_pgdata:/var/lib/postgresql/data \
  --health-cmd="pg_isready -U postgres -d evt" \
  --health-interval=5s \
  --health-timeout=3s \
  --health-retries=20 \
  -d postgres:16

---

## Roadmap (Phases)

* **PH-1 – EVT (current)**
  Employment Verification Tokens (MVP)

* **PH-2 – JD**
  Attested job descriptions

* **PH-3 – RESUME**
  Verified résumé tokens + DP-1 disambiguation

* **PH-4 – STUDENT**
  Student e-portfolios

* **PH-5 – ADMISSIONS**
  Attested records for admissions workflows

---

## License

MIT

---

## Maintainers

* **Spec / architecture:** Whitehouse
* **PKI review:** *(TBD)*

---
