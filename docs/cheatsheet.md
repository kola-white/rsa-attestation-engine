# **Attested Identity – Protocol Stack Cheat Sheet (v1)**

## **THE SIX CORE SPECIFICATIONS**

### **1. AP-1 — Attestation Profile**

**What it defines:**

* The **shape** and **semantics** of an attestation.
* Required fields (`subject`, `claim`, `validity`, `policy`, `signature`).
* Claim namespaces like `employment.role`.
* Schema URIs and versioning.

**Input to:** DP-1
**Depends on:** CP/CPS (schemas published), KP-1 (signing rules)

---

### **2. KP-1 — Key Profile**

**What it defines:**

* Allowed algorithms (`ES256`, `EdDSA`, etc.)
* JWS header rules (`kid`, `alg`, protected fields)
* Key lifecycle states: `active`, `grace`, `deprecated`, `compromised`
* JWKS structure and metadata.

**Used by:**

* TP-1 (trust path)
* RP-1 (revocation state uses KP-1 key lifecycle)
* DP-1 (scoring, eligibility)

**Governed by:** CP/CPS (generation, storage, rotation)

---

### **3. RP-1 — Revocation Profile**

**What it defines:**

* JSON Status List format
* Status vocabulary: `good`, `revoked`, `unknown`, `invalid`
* How verifiers interpret each state
* How status list freshness applies (`checked_at`, `ttl_s`)

**Used by:**

* DP-1 eligibility
* KP-1 (key state)
* Verifier-side policy

**Governed by:** CP/CPS (timelines, publication, reasons)

---

### **4. TP-1 — Trust Path Specification**

**What it defines:**

* Root CA → Issuing CA → Issuer key chain
* How verifiers validate chains
* Trust path strength (`trust_path_strength`, `ca_depth`)
* How JWKS integrates with CAs.

**Used by:**

* DP-1 scoring
* KP-1 binding
* Policy layers

**Governed by:**

* CP/CPS (CA roles, lifecycle, repository rules)

---

### **5. DP-1 — Disambiguation Protocol**

**What it defines:**

* How to choose **one authoritative attestation**
* Eligibility filters
* Composite scoring (A–E, 0–100)
* Deterministic tie-breaking (tieKey)
* `AMBIGUOUS_RESULT` semantics
* No optionality, no policy, no risk knobs.

**Depends on:** AP-1, KP-1, RP-1, TP-1
**Governed by:** CP/CPS (revocation, validity, key lifecycles)

---

### **6. CP/CPS — Certificate Policy & Certification Practice Statement**

**What it defines:**

* Rules governing **who** can operate which part of the trust architecture
* How keys are generated, rotated, revoked
* Publication rules for JWKS and status lists
* Incident response
* Audit & compliance
* Legal boundaries
* Operational responsibilities for CA, Issuer, Verifier.

**This is the governance layer for the whole stack.**

---

# **THE STACK IN 1 SENTENCE**

> **CP/CPS defines the rules; AP-1/KP-1/RP-1/TP-1 define the wire formats and validation logic; DP-1 deterministically selects the correct attestation.**

---

# **END-TO-END DATA FLOW**

**1. Issuer**
→ Uses KP-1-compliant key
→ Signs AP-1-compliant attestation
→ Publishes via TP-1 trust path rules

**2. Repository**
→ JWKS + StatusList served per CP/CPS rules

**3. Verifier**
→ Performs AP-1 validation
→ Performs KP-1 key checks
→ Performs TP-1 trust path checks
→ Performs RP-1 revocation checks
→ Inputs normalized candidates into DP-1
→ DP-1 selects 1 or returns AMBIGUOUS_RESULT

---

# **SPEC INTER-DEPENDENCY GRAPH (TEXT FORM)**

```
           CP/CPS
        (governance)
             |
    -----------------------
    |          |          |
   KP-1       RP-1      TP-1
    |           \        /
    |            \      /
    |             \    /
    ------->  AP-1   <------
           (attestation)
                |
               DP-1
          (deterministic)
```

---

# **WHAT LIVES WHERE?**

| Topic                                 | Spec                                            |
| ------------------------------------- | ----------------------------------------------- |
| Schema definition                     | AP-1                                            |
| Key algorithms                        | KP-1                                            |
| Revocation                            | RP-1                                            |
| Trust chain                           | TP-1                                            |
| Disambiguation                        | DP-1                                            |
| Governance / lifecycle                | CP/CPS                                          |
| Soft ambiguity / recency / risk knobs | **Not in any protocol — lives in policy layer** |
| Monetizable features                  | **Policy layer (see POL-1/VP-1)**            |

---

# **BULLETED SUMMARY**

1. **AP-1** defines what an attestation *is*.
2. **KP-1** defines how keys work and how they behave.
3. **RP-1** defines how revocation is expressed.
4. **TP-1** defines how trust paths are built and validated.
5. **DP-1** deterministically selects the authoritative attestation.
6. **CP/CPS** governs the whole ecosystem.

---
