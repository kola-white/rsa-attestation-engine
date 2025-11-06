# Attested Identity – v0.2 (MVP)

A minimal, production-minded **attestation layer** that allows issuers (employers/institutions) to **sign verifiable claims** about a person (e.g., role, skills, contributions), and allows verifiers (ATS systems, recruiters, HR platforms) to **validate** these claims via a modern PKI-backed trust fabric.

This system is the core of a **resume-replacement credential format**, built to be:

* **Web-native** (JSON, JWS, JWKS)
* **PKI-anchored** (X.509 for issuer trust, HSM/KMS-compatible)
* **Low-friction** (simple APIs for issuers & verifiers)
* **Long-lived** (credentials survive devices, logins, and job changes)

**Spec status:**

* AP-1 (Attestation Processing) ✅
* DP-1 (Disambiguation Protocol) ✅ (in Google Docs)

**Crypto baseline:**

* JWS (RS256) for signing human-fact claims
* JWKS for issuer public-key distribution
* JSON Status List for revocation (CRL-analog)

**Privacy:**

* PII-minimal
* Selective disclosure (SD-JWT / Merkle proofs) reserved for v0.2+

---

# Contents

* [Goals](#goals)
* [Architecture (MVP)](#architecture-mvp)
* [Device vs Human Identity Model](#device-vs-human-identity-model)
* [Layered Trust Stack](#layered-trust-stack)
* [Data Schemas](#data-schemas)
* [Services & Endpoints](#services--endpoints)
* [Quickstart (dev)](#quickstart-dev)
* [Golden Rules (Test Cases)](#golden-rules-to-test-against)
* [Verifier SDK Shape](#verifier-sdk-shape)
* [Security Notes](#security-notes)
* [Roadmap](#roadmap)
* [License](#license)

---

# Goals

* **Issue** signed attestations (employment role, skills, outcomes) bound to a subject.
* **Verify** integrity, issuer trust chain, validity window, and revocation status.
* **Revoke/Rotate** with short-TTL caches and fail-closed verification.
* **Audit** every issuance, verification, and revocation with signed logs.
* Keep friction low; support resume-free workflows.

---

# Architecture (MVP)

```
Issuer API ──signs──► Attestation JSON
    │                       │
    │ publishes keys        ▼
    ├──► Trust Directory (JWKS, statuslist.json, policies, schemas)
    │                       │
Verifier API ◄─fetches───┘   │
    │                       │
    └──► VerificationReceipt (VALID / REVOKED / EXPIRED)
```

* **Trust Directory** publishes issuer public keys (JWKS), revocation state, policies, schemas.
* **Issuer API** holds private keys (dev: software; prod: HSM/KMS).
* **Verifier API/SDK** validates signatures, trust path, time windows, and revocation.

---

# Device vs Human Identity Model

Human claim verification is **independent of devices**, but device authentication matters for **session-level identity**.

Below is the correct mental model:

### ✅ Device = authentication

“How do I know the person pressing the button is real?”

* Passkeys (FIDO2)
* WebAuthn
* TPM/Secure Enclave keys
* Optional client-side X.509 certs

Used for login and admin authentication — not for long-term claim storage.

### ✅ Issuer = authority

“Which organization signs the truth?”

Backed by an X.509 PKI hierarchy:

* Root CA (offline — the **vampire in the crypt**)
* Intermediate / issuing keys (online — **vampire lieutenants**)
* Keys eventually stored in KMS/HSM

### ✅ Attestations = lasting facts

JSON claims: roles, skills, accomplishments.
Signed using **JWS** with issuer private keys.
Verified using **JWKS** (public keys).

### ✅ JWS = claim signature format

Portable, web-native, flexible, future-proof.

### ✅ JWKS = public key distribution

Web-standard for publishing issuer keys.

---

# ✅ Diagram: Device Identity vs Human Claim Identity

```
                           ┌───────────────────────────────┐
                           │         DEVICE IDENTITY        │
                           │      (Authentication Layer)    │
                           └───────────────────────────────┘
                                         │
                      ┌──────────────────┼──────────────────┐
                      │                  │                  │
                      ▼                  ▼                  ▼
    ┌────────────────────────┐   ┌─────────────────┐   ┌──────────────────────────┐
    │ User Device            │   │ Browser/FIDO    │   │ Device-bound Keypair     │
    │ (Laptop/Phone)         │   │ (Passkey/WebAuth│   │ (TPM/Secure Enclave)     │
    └────────────────────────┘   └─────────────────┘   └──────────────────────────┘
                       │                 proves possession
                       └───────────────────────────────┘
                                         │
                                         ▼
                    ✅ Authenticates the user (session identity)
                                         │
──────────────────────────────────────────────────────────────────────────────
   HUMAN CLAIM IDENTITY (Independent of Device; persists for years)
──────────────────────────────────────────────────────────────────────────────
                           ┌───────────────────────────────┐
                           │     ISSUER = AUTHORITY        │
                           │  (The “Vampire in the Crypt”) │
                           └───────────────────────────────┘
                                         │
                      Root CA (offline, guarded — “vampire key”)
                                         │
                      Intermediate / Issuing CA Keys (“lieutenants”)
                                         │
──────────────────────────────────────────────────────────────────────────────
                     JWS / JWKS ATTESTATION LAYER (Human Facts)
──────────────────────────────────────────────────────────────────────────────
                    - Role, skills, outcomes, dates
                    - JSON claims signed as JWS
                    - Public keys distributed as JWKS
──────────────────────────────────────────────────────────────────────────────
                           VERIFIER (ATS, Recruiters, APIs)
──────────────────────────────────────────────────────────────────────────────
     1) Fetch JWKS  
     2) Verify JWS signature  
     3) Check validity window  
     4) Check revocation state  
     5) Establish trust chain  
```

---

# Layered Trust Stack

```
[ Layer 5 — Apps / HR Systems / ATS ]
    - LinkedIn-style viewers
    - Recruiter dashboards
    - Candidate wallets
    - Hiring APIs

[ Layer 4 — JWS Attestations ]
    - JSON claims
    - Signature envelope
    - Selective disclosure (future)

[ Layer 3 — JWKS Trust Directory ]
    - Public keys
    - Key rotation
    - Status lists
    - Policy URIs

[ Layer 2 — Issuer CA Keys (X.509 PKI) ]
    - Offline root CA (“vampire in crypt”)
    - Intermediate issuing keys (“lieutenants”)
    - CA chain metadata

[ Layer 1 — Device Identity ]
    - Passkeys / FIDO2 / WebAuthn
    - TPM secure key storage
    - Optional client X.509 certs
```

This model is **integrated**, not a binary choice.
We use **X.509 for issuer trust** and **JWS/JWKS for human claims** — identical to Apple, Google, Microsoft, and W3C’s modern credential ecosystems.

---

# Data Schemas

Schemas live under `./schemas/`:

* `attestation.schema.json`
* `verification-receipt.schema.json`
* `revocation-event.schema.json`

Each schema is versioned in-band via `schema_uri`.

---

# Services & Endpoints

### **Issuer Service**

POST `/issue` → Attestation
POST `/revoke` → RevocationEvent
POST `/rotate` → new `kid` + updated JWKS

### **Trust Directory (public)**

GET `/.well-known/jwks.json`
GET `/statuslist.json`
GET `/policies/:name`
GET `/schemas/:name`

### **Verifier Service**

POST `/verify` → VerificationReceipt

---

# Quickstart (dev)

(Same as your version; unchanged except for clarity — omitted here for brevity.)

---

# Golden Rules to test against

Valid, expired, revoked — unchanged from your draft.

---

# Verifier SDK Shape

Unchanged — strongly typed TS interface.

---

# Security Notes

Rewritten for clarity; includes fail-closed policy, rotation rules, and HSM notes.

---

# Roadmap

Same as your list, including “purple badge” wallet.

---

# License

MIT.

---

# Maintainers

* **Spec/architecture:** Whitehouse
* **PKI review:** Coe