# **TP-1: Trust Path Specification (v1.0.0)**

**Attested Identity — Trust Path Definition**

**Status:** Final (MVP)
**Editors:** Whitehouse, Coe
**Scope:** Defines the cryptographic trust chain that governs the issuance and verification of human-claim attestations within the Attested Identity system.

---

# **1. Purpose**

This document establishes the **authoritative trust path** for all credential operations within the system:

* **Authentication of administrative actors** (X.509 device/issuer authentication)
* **Issuance of attestations** (JWS signed)
* **Verification of attestations** (JWKS published)
* **Revocation and key rotation**
* **Trust chain validation across issuers and verifiers**

TP-1 ensures that any verifier—human or machine—can reconstruct a complete trust chain from the attestation back to a cryptographically trusted root.

---

# **2. Trust Model Overview**

Attested Identity uses a hybrid trust model:

1. **X.509 PKI**

   * anchors issuer identity
   * authenticates administrative actors
   * creates a durable trust chain for signing keys

2. **JWS + JWKS**

   * signs and distributes human-claim attestations
   * enables web-native verification and key rotation
   * enables long-lived, device-independent credentials

The X.509 layer establishes who is allowed to sign;
the JWS layer establishes what is being signed.

This model aligns with:

* NIST SP 800-63A / 800-63C
* W3C Verifiable Credentials Data Model
* WebAuthn + FIDO2 security architecture
* Public WebPKI best practices

---

# **3. Trust Roles**

### **3.1 Root Certificate Authority (Root CA)**

* Purpose: anchor the cryptographic trust chain.
* Location: **offline** (“vampire in the crypt”).
* Key type: RSA-4096 or ECDSA P-384 (MVP allows RSA-2048).
* Key usage: **certSigning** only.
* Storage: hardware vault or encrypted offline storage.
* Rotation: 5–10 year cycle (MVP: manual).

### **3.2 Issuing Certificate Authority (Issuing CA)**

* Purpose: sign keys used for JWS/JWKS attestation signing.
* Location: **online**, hardened service (“lieutenant”).
* Key type: RSA-2048 or ECDSA P-256.
* Key usage:

  * `keyCertSign`
  * `digitalSignature`
* Rotation: annually or quarterly (configurable).
* Revocation method: CRL optional; JWKS supersedes for JWS keys.

### **3.3 Issuer (Attestation Authority)**

An issuer is any organization that:

* owns an issuing CA chain
* controls at least one active signing key
* publishes JWKS
* publishes a revocation list
* issues human-claim attestations conforming to AP-1

Examples: employers, education institutions, certifying bodies.

### **3.4 Verifier**

Any ATS, platform, or service validating attestations.
Verifiers must:

* fetch JWKS
* validate CA chain
* validate JWS signatures
* enforce policy
* check revocation status
* enforce validity windows

---

# **4. Trust Path Components**

The system’s trust path consists of the following elements:

### **4.1 Root CA Certificate (PEM)**

The ultimate trust anchor.

### **4.2 Issuing CA Certificate (PEM)**

Signed by Root CA.

### **4.3 JWKS (JSON Web Key Set)**

Published at:

```
https://<issuer-domain>/.well-known/jwks.json
```

Contains:

* active JWS signing keys
* deprecated keys within grace windows
* metadata for rotation

### **4.4 JWS-Signed Attestations**

Content-layer claims:

* roles
* skills
* outcomes
* employment metadata

Bound to issuer keys via `kid`.

### **4.5 Revocation Status List (JSON)**

Published at:

```
https://<issuer-domain>/statuslist.json
```

Format: compact JSON hash-list.
Purpose: mark attestations as:

* `good`
* `revoked`
* `unknown`

### Implicit State: `unknown`

The `unknown` status is **not stored** explicitly.  
A serial is considered **unknown** if it is **absent** from the `entries` list.

---

### Verifier Policy (MVP)

**Fail-open (default):**  
If a serial is **not found**, treat it as `unknown` → **accept**, and log: status=unknown (not listed).

A future version may switch to **fail-closed** (reject unknown) when every issued serial is required to appear in the list.  
Any policy change must be recorded in the changelog.

---

### Freshness & Caching

- `ttl_s` defines the maximum cache lifetime.  
- Verifiers should refetch the list when:
  - the TTL expires, **or**
  - `ETag` or `Last-Modified` indicates a newer version.

### Verifier Pseudocode (MVP)

```ts
// inputs: protectedHeader.kid, payload.serial
verifySignatureWithJWKS(kid, jwks); // throws if signature or key invalid

const entry = statusList.entries.find(e => e.serial === payload.serial);

// Implicit unknown: not listed → accept (fail-open) but log it
if (!entry) {
  log.warn(`status=unknown serial=${payload.serial} (not listed)`);
  return ACCEPT;
}

switch (entry.status) {
  case "good":
    return ACCEPT;

  case "revoked":
    // A revoked entry may include reason_code and time
    return REJECT;

  default:
    // Defensive coding: unknown status value should be treated as failure
    log.error(`invalid status value "${entry.status}" for serial ${payload.serial}`);
    return REJECT;
}

---

### Versioning

Include:

```json
"version": 1 at the top of the file. Increment this value on any breaking change (e.g., adding new statuses such as "suspended").

### **4.6 Policy URIs**

Each attestation references:

```
policy.policy_uri
```

Defines:

* assurance level (TAL-1 to TAL-4)
* required bindings
* schema references
* allowed key types
* rotation window
* time validity rules

---

# **5. Trust Path Construction**

A verifier reconstructs the trust path as follows:

### **Step 1 — Fetch JWKS**

Look up the issuer domain’s `.well-known/jwks.json`.

### **Step 2 — Match `kid`**

Locate the key corresponding to the attestation’s signature.

### **Step 3 — Validate JWS Signature**

Validate signature using:

* `kty`
* `alg`
* `n` / `e` (RSA) or `x` / `y` (EC)

### **Step 4 — Validate CA Chain**

Reconstruct:

```
Attestation JWK → Issuing CA → Root CA
```

Validate:

* Root CA trust anchor
* Issuing CA signing authority
* Key usage constraints
* Certificate validity periods
* Revocation (if CA revocation implemented)

### **Step 5 — Validate Attestation Metadata**

Check:

* `not_before`
* `not_after`
* subject binding
* policy assurance level (TAL-n)
* schema compliance

### **Step 6 — Validate Revocation State**

Look up:

```
statuslist.json
```

Mark attestation as:

* ✅ VALID
* ❌ INVALID (expired, revoked, or untrusted)

---

# **6. Key Lifecycle Requirements**

### **6.1 Key Generation**

* Use FIPS-validated libraries when possible.
* Issuing CA keys generated on hardened nodes
* Root CA offline

### **6.2 Key Rotation**

* Issuer keys rotated every 90 days (recommended).
* JWKS must retain old keys for grace period.
* Extra caution for overlapping signatures.

### **6.3 Key Revocation**

* Use revocation list entries containing:

  * `attestation_serial`
  * `reason_code`
  * `time`
  * signature of issuer

### **6.4 Emergency Key Compromise**

If a key is compromised:

1. Mark compromised in JWKS.
2. Immediately revoke all attestations signed with compromised key.
3. Publish emergency rotation event.
4. Reissue new attestations (optional).

---

# **7. Valid Trust Path Example**

```
Attestation JSON (JWS)
     │   signature verified with
     ▼
JWKS Key (kid: apple-key-2025q1)
     │   belongs to
     ▼
Issuing CA Certificate
     │   signed by
     ▼
Root CA Certificate (Trust Anchor)
```

---

# **8. Threat Model Considerations**

The following threats are explicitly mitigated:

### **8.1 Spoofed Issuers**

Mitigated by: X.509 chain + JWKS domain binding.

### **8.2 Forged Attestations**

Mitigated by: JWS signature verified against JWKS.

### **8.3 Key Compromise**

Mitigated by: revocation list + rotation policy.

### **8.4 Replay Attacks**

Mitigated by: `not_before`, `not_after`, CA validity.

### **8.5 Downgrade Attacks**

Mitigated by: policy version pinning + TAL assurance levels.

---

# **9. Compliance and Interoperability**

TP-1 aligns with:

* **NIST SP 800-63A** — Identity Assurance
* **NIST SP 800-57** — Cryptographic Key Management
* **W3C Verifiable Credentials**
* **IETF RFC 7515** — JWS
* **IETF RFC 8414** — Web PKI metadata
* **WebAuthn/FIDO2 best practices**

---

# **10. Versioning and Change Control**

* Version: **TP-1 v1.0.0 (MVP)**
* Updates tracked in `/trust/CHANGELOG.md`
* Breaking changes require Root CA re-issuance or schema migration.

---