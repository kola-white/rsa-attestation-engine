# **CP / CPS — Combined Certificate Policy & Certification Practice Statement**

### **Attested Identity Trust Framework**

**Version:** 1.0-MVP
**Status:** Final
**Editors:** Whitehouse 

---

# **1. Introduction**

## **1.1 Purpose**

This document defines the **Certificate Policy (CP)** and **Certification Practice Statement (CPS)** for the Attested Identity ecosystem. It governs:

* Root CA operations
* Issuing CA operations
* Issuer key usage
* The publication of trust artifacts (JWKS, status lists, policies)
* Revocation and incident response procedures
* Verifier obligations

This CP/CPS ensures **interoperable, deterministic, high-assurance attestation verification** across the Attested Identity stack (AP-1, KP-1, RP-1, TP-1, DP-1).

## **1.2 Roles**

* **Root CA**
  Offline trust anchor that signs Issuing CA certificates.

* **Issuing CA**
  Online or HSM-protected CA that signs issuer keys and policy-bound attestation profiles.

* **Issuer**
  Authorized entity that signs **attestation credentials** using JWS keys approved by the Issuing CA.

* **Verifier**
  Any relying party performing AP-1, RP-1, KP-1, TP-1, DP-1 processing to evaluate attestations.

## **1.3 Applicability**

This CP/CPS applies to all trust artifacts used in the Attested Identity system:

* CA certificates
* Issuer signing keys
* `/.well-known/jwks.json`
* `statuslist.json`
* Policy URIs and schema URIs

---

# **2. Identity & Authentication**

## **2.1 Administrative Authentication**

All administrative actions affecting CA configuration, issuer authorization, or trust-path content MUST require:

* WebAuthn (hardware-backed) authentication **or**
* Client certificate authentication **or**
* Equivalent hardware MFA

## **2.2 Separation of Duties**

At minimum:

* CA administrators MUST NOT be able to authorize new issuers alone.
* Policy editors MUST NOT control key generation.
* Revocation actions MUST require dual-authorization.

## **2.3 Change Control**

Changes to:

* JWKS
* status lists
* issuer registry
* CA configurations
* allowed algorithms

MUST be approved by at least two authorized administrators and logged in immutable audit logs.

---

# **3. Key Management**

## **3.1 Key Generation**

* Root CA keys MUST be generated offline using a FIPS-validated library or equivalent.
* Issuing CA keys MAY be generated offline or inside a cloud KMS/HSM (AWS KMS, GCP KMS, Azure Key Vault).
* Issuer JWS keys MUST be generated using secure library primitives (WebCrypto, libsodium, OpenSSL).

## **3.2 Key Storage**

* Root CA key MUST remain offline except during signing ceremonies.
* Issuing CA keys MUST reside in HSM/KMS with role-based access control.
* Issuer keys SHOULD be stored in secure enclaves or KMS when possible.

## **3.3 Backup & Recovery**

* Only Issuing CA keys MAY be backed up.
* Root CA keys SHOULD NOT be backed up except in controlled offline media.
* Backups MUST be encrypted and access-logged.

## **3.4 Key Destruction**

When keys are retired:

* CA keys MUST be destroyed securely (HSM purge or equivalent).
* Issuer keys MUST be revoked and removed from JWKS publication.

---

# **4. Certificate / Key Usage**

## **4.1 Root CA**

* Key usage: **certSign** only
* NOT permitted: digital signatures of attestations

## **4.2 Issuing CA**

* Key usage: **keyCertSign**, **digitalSignature**
* Permitted to sign:

  * Issuer JWS key metadata
  * KP-1 policy bindings
  * Attestation policy roots

## **4.3 Issuer Keys**

* JWS signing only

* MUST conform to KP-1 rules:

  * allowed algorithms (`ES256`, `EdDSA`, `RS256` if allowed)
  * required headers
  * KID rules

* MAY only sign attestation credentials conforming to AP-1.

---

# **5. Key & Certificate Lifecycle**

## **5.1 Rotation**

* Issuer keys SHOULD rotate every 90–180 days.
* Issuing CA keys SHOULD rotate every 1–2 years.
* Root CA rotates only during major policy changes (5–10 year cadence).

## **5.2 Deprecation Windows**

* Deprecated keys MUST remain in JWKS for a **grace window** (default: 30 days).
* Deprecated keys MUST be marked as `"deprecated"` in KP-1 metadata.

## **5.3 Emergency Rotation**

If a key is suspected compromised:

* Immediate revocation in `statuslist.json`
* Immediate removal from JWKS
* Issuers MUST invalidate any affected attestations
* Emergency re-issuance MAY occur

---

# **6. Publication & Repository Management**

## **6.1 Mandatory Artifacts**

The following MUST be published and accessible:

* `/.well-known/jwks.json` (active + deprecated keys)
* `/.well-known/statuslist.json`
* Policy URIs for AP-1, RP-1, KP-1, TP-1, DP-1
* Schema URIs
* ETags for freshness detection

## **6.2 Repository Requirements**

* MUST be served over HTTPS
* MUST provide correct `Cache-Control` headers
* JWKS and statuslist MUST be atomic (replace as a whole document)

## **6.3 Versioning & Cache TTL**

* JWKS: default TTL = 5 minutes
* Statuslist: default TTL = 60 minutes
* Verifiers MUST use ETags or `Last-Modified` for delta checks

---

# **7. Revocation**

## **7.1 Reasons**

Keys or attestations may be revoked for:

* compromise
* unauthorized issuance
* cessation of operation
* issuer misconduct
* policy violation
* administrative request

## **7.2 Status List Format**

Revocation information is published via:

* **RP-1 JSON Status List**
* MUST include:

  * `"serial"`
  * `"status"`
  * `"checked_at"`
  * `"evidence"` (optional)
  * `"ttl_s"`

## **7.3 Timelines**

* Routine revocation: same-day publishing
* Compromise: < 4 hours
* High-severity: < 60 minutes

## **7.4 Verifier Obligations**

Verifiers MUST:

* Retrieve status list using TTL and ETags
* Treat `"revoked"` as fatal
* Treat `"unknown"` as neutral
* Fail closed when status list is unavailable (unless explicitly configured otherwise)

---

# **8. Security Controls**

## **8.1 Physical & Logical Controls**

* CA systems MUST use hardened environments
* MFA for all administrative actions
* Firewall restrictions on CA interfaces
* Root CA MUST remain offline

## **8.2 Least Privilege**

Admin roles MUST be scoped:

* policy editor
* CA cryptographic operator
* issuer onboarding operator
* revocation approver
* auditor (read-only)

## **8.3 Logging & Retention**

Logs MUST include:

* key events
* issuer onboarding
* revocation actions
* publication updates
* CA ceremonies
* failures and anomalies

Retention: minimum 2 years (MVP).

---

# **9. Audit & Compliance**

* CA events MUST generate verifiable, signed audit logs.
* Internal audit SHOULD occur every 6 months.
* External audit MAY be performed annually or at customer request.
* Log tampering MUST be detectable (hash-chains or Merkle proofs recommended).

---

# **10. Incident Response**

## **10.1 Indicators of Compromise**

* Suspicious JWS signatures
* Unexpected issuer key rotation
* Misaligned status list TTL
* Detectable tampering of trust artifacts
* Unapproved policy change

## **10.2 Required Actions**

* Revoke compromised keys immediately
* Publish updated status list
* Notify affected issuers/verifiers
* Issue replacement keys
* Document evidence in audit logs

---

# **11. Legal**

## **11.1 Intellectual Property**

Policy documents and schema URIs are licensed under a permissive license (e.g., MIT, Apache 2.0).

## **11.2 Warranties & Disclaimers**

* No warranty of correctness of claim *content* — issuers are responsible.
* CA/Root CA liability limited to proper operation of KP-1, RP-1, and TP-1.
* Verifiers assume responsibility for interpreting scores and risk.
* Force majeure exclusions apply.

---

# **12. Versioning & Change Control**

## **12.1 Semantic Versioning**

Policy changes follow:

* `MAJOR` — breaking
* `MINOR` — new fields or clarifications
* `PATCH` — typo fixes and editorial changes

## **12.2 Deprecation Policy**

* Deprecated URIs remain valid for at least 90 days
* CA certificates MUST include validity periods aligned with policy lifecycle

## **12.3 Migration Expectations**

* Verifiers MUST support older schema URIs during the deprecation window
* Issuers MUST reissue attestations if policy updates invalidate older profiles

---
