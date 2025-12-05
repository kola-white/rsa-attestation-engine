# **DP-1: Disambiguation Protocol (v1.0.0)**

**Attested Identity — Deterministic Resolution of Human-Claim Attestations**

**Status:** Final (MVP)
**Editors:** Whitehouse 
**Depends on:** AP-1 (Attestation Profile), RP-1 (Revocation Profile), KP-1 (Key Profile), TP-1 (Trust Path Specification)
**Audience:** Verifiers, ATS platforms, Security Engineers
**Purpose:** Ensure deterministic, unambiguous selection of *one authoritative attestation* in environments where multiple valid candidates may exist.

---

# **1. Purpose**

DP-1 defines the canonical **disambiguation layer** of the Attested Identity system.
It ensures that any verifier—human or automated—can deterministically choose **one correct attestation** when confronted with ambiguous, overlapping, or partially valid inputs.

DP-1 is required when:

* A subject presents **multiple attestations** for the same claim type (`employment.role`).
* An issuer publishes **multiple active signing keys** due to rotation.
* A verifier encounters **ambiguous status**, incomplete temporal windows, or conflicting policy boundaries.
* Multiple trust paths exist (e.g., parent-org CA vs subsidiary CA).

DP-1 solves ambiguity through:

1. **Eligibility Filtering** (hard gates)
2. **Confidence Scoring** (0–100 composite score)
3. **Deterministic Tie-Breaking**
4. **AmbiguousResult** semantics (canonical error output)

---

# **2. Position Within the Operational Architecture**

DP-1 is the fourth element of the **Operational Quartet**:

| Component                           | Purpose                                                       | Citation          |
| ----------------------------------- | ------------------------------------------------------------- | ----------------- |
| **AP-1 — Attestation Profile**      | Defines payload shape, schema_uri, claim semantics            |                   |
| **RP-1 — Revocation Profile**       | Defines the status-list method, serial lookup, vocabulary     |                   |
| **KP-1 — Key Profile**              | Defines key material, JWKS construction, rotation             |                   |
| **TP-1 — Trust Path Specification** | Defines trust chain, CA hierarchy, JWKS fetch, liveness       |                   |
| **DP-1 — Disambiguation Protocol**  | *Defines deterministic selection logic when ambiguity exists* | *(this document)* |

DP-1 does **not** redefine trust, revocation, or schema correctness.
Those are enforced by AP-1, RP-1, KP-1, and TP-1.
DP-1 *only* resolves ambiguity **after** those layers have validated and normalized the candidate set.

---

# **3. Problem Definition**

Real-world verification systems frequently encounter ambiguity:

1. **Multiple attestations** for the same employment role (different dates, different keys).
2. **Multiple issuers** representing corporate lineage (e.g., parent company vs subsidiary).
3. **Key rotation overlaps**, where both old and new keys produce valid signatures.
4. **Partial metadata** in older attestations.
5. **Fail-open revocation state**, where “unknown/not listed” is acceptable.
6. **Schema version drift** (e.g., AP-1 line evolving over time).
7. **Temporal windows** that overlap but do not match exactly.

Without DP-1, the verifier may produce non-deterministic results depending on ordering, input structure, or transient environmental states.

DP-1 removes that uncertainty.

---

# **4. Disambiguation Model**

DP-1 uses a **three-layer model**:

## **4.1 Layer 1 — Eligibility Filter (Hard Gates)**

Candidates are **discarded** if they fail *any* mandatory requirement:

1. **Signature Invalid** (via KP-1 + TP-1)
2. **CA Chain Invalid**
3. **Schema Invalid** (via AP-1)
4. **Expired** (`validity.not_after < now`)
5. **Not Yet Valid** (`now < not_before`)
6. **Revoked** (via RP-1)
7. **Invalid Status Value** (RP-1 vocabulary error)
8. **Key Invalid** (compromised/retired per KP-1)
9. **Policy Assurance Failure**

Only **eligible** candidates advance to scoring.

If *zero* eligible candidates remain:
→ return `{ ok: false, code: "NO_VALID_ATTESTATIONS" }`.

---

## **4.2 Layer 2 — Confidence Scoring (0–100)**

Each eligible attestation receives a composite score across five dimensions.

### **A. Issuer Confidence (0–30 pts)**

Derived from TP-1 + KP-1:

* CA chain depth (stronger > weaker)
* Domain binding strength
* JWK freshness (active > grace > deprecated)
* Rotation recency (newer signing key penalizes stale keys)
* Issuer assurance (TAL levels, if published)

---

### **B. Schema Fidelity (0–25 pts)**

Derived from AP-1:

* All required AP-1 fields present
* Context richness
* Completeness of claim.value block (`title`, `level`, `skill`)
* Internal consistency
* No shape errors
* Correct schema_uri & version pinning

---

### **C. Temporal Alignment (0–25 pts)**

Measured against:

* Duration of validity window
* Specificity (tight windows > broad windows)
* Claim period alignment (if context contains role start/end)
* Ordering correctness

---

### **D. Revocation Freshness (0–10 pts)**

Via RP-1:

* Status list freshness (`generated_at`, `ttl_s`)
* ETag / Last-Modified recency
* No conflicting entries
* Fail-open unknown → neutral rather than penalizing

---

### **E. Uniqueness Signals (0–10 pts)**

Entropy signals that the attestation is precise:

* High-entropy `serial` (ULID/UUID)
* Unique context entries
* Lack of duplication across issuers
* Presence of higher-value metadata

---

Composite score:

```
score = A + B + C + D + E
```

---

## **4.3 Layer 3 — Deterministic Tie-Breaking**

If two or more candidates have the **same composite score**, apply:

### **1. Prefer Stronger Trust Path**

* Longer CA chain depth
* Explicit CA lineage in AP-1 (`issuer.ca_chain`)
* Valid cert lifetimes aligned with current time

---

### **2. Prefer Newer Signing Key**

Via KP-1:

* The key with the most recent creation time wins.
* If creation time unavailable, prefer lexicographically higher KID (stable heuristic).

---

### **3. Prefer Narrower Temporal Window**

The attestation with the **tightest range** `(not_after - not_before)` wins.
Higher specificity indicates higher confidence.

---

### **4. Prefer Higher Schema Version**

e.g.,
`schema/employment.role/v1` (version 1.0.1)
beats
`schema/employment.role/v1` (version 1.0.0)

---

### **5. Prefer Richer Context**

Attestations with more explicit, structured `claim.context` metadata win.

---

### **6. Last Resort: Declare AmbiguousResult**

If all tie-breakers fail:

```
{
  "ok": false,
  "code": "AMBIGUOUS_RESULT",
  "reason": "score_tie_after_tiebreak",
  "candidates": ["serial_A", "serial_B"]
}
```

AmbiguousResult is normative and signals to higher-level systems (e.g., ATS) that human adjudication or policy override is required.

---

# **5. Normative Algorithm (Pseudocode)**

```ts
function disambiguateAttestations(candidates) {
  // 1. Eligibility Filter
  const eligible = candidates.filter(isEligible);
  if (eligible.length === 0) {
    return { ok: false, code: "NO_VALID_ATTESTATIONS" };
  }

  // 2. Scoring
  const scored = eligible.map(a => ({
    attestation: a,
    score: computeCompositeScore(a)
  }));

  scored.sort((a, b) => b.score - a.score);

  // 3. Compare Top Two
  const top = scored[0];
  const second = scored[1];

  if (second && top.score === second.score) {
    const resolved = tieBreak(top.attestation, second.attestation);
    if (!resolved) {
      return {
        ok: false,
        code: "AMBIGUOUS_RESULT",
        candidates: [top.attestation.revocation.serial, second.attestation.revocation.serial]
      };
    }
    return resolved;
  }

  return top.attestation;
}
```

---

# **6. Eligibility Filter (Normative Definitions)**

Eligibility uses the output semantics defined in your AP-1, RP-1, and KP-1 specs.

A candidate is disqualified if:

* `BAD_SIGNATURE` (AP-1 + KP-1 + TP-1)
* `HEADER_INVALID`
* `SCHEMA_INVALID`
* `SCHEMA_URI_INVALID`
* `CLAIM_TYPE_INVALID`
* `DISCLOSURE_MODE_INVALID`
* `NOT_YET_VALID`
* `EXPIRED`
* `REVOKED`
* `REVOCATION_STATUS_INVALID`
* `POLICY_ASSURANCE_INVALID`
* KP-1 marks key as compromised, retired, or non-authoritative
* TP-1 trust path cannot be reconstructed

This normalizes all inputs before scoring.

---

# **7. AmbiguousResult Specification**

Ambiguous results are not failures—they are **explicitly recognized states** that require policy or human examination.

An AmbiguousResult MUST include:

```json
{
  "ok": false,
  "code": "AMBIGUOUS_RESULT",
  "reason": "score_tie_after_tiebreak",
  "candidates": ["serial_A", "serial_B"],
  "metadata": {
    "context": "employment.role",
    "issuer_candidates": [...],
    "scoring_version": "v1"
  }
}
```

Verifiers MAY choose to:

* Request additional attestations
* Present candidates to a human workflow
* Apply proprietary weighting layers (above DP-1)

But MUST NOT silently select a winner.

---

# **8. Threat Model**

DP-1 protects against:

### **8.1 Ambiguity Injection**

Attackers fabricate multiple borderline-valid attestations to confuse verifiers.

### **8.2 Key Rotation Exploits**

Overlapping key windows used to create dual-valid claims.

### **8.3 Multi-Issuer Collisions**

Subsidiaries and parent companies issue conflicting claims.

### **8.4 Temporal Smearing**

Manipulating validity windows to bias results.

### **8.5 Schema Drift Confusion**

Older attestations vs newer profile variants.

DP-1 ensures deterministic resolution in all cases.

---

# **9. Compliance and Interoperability**

DP-1 is compatible with:

* **AP-1**: Ensures schema fidelity and employment-role semantics.
* **RP-1**: Interprets status-list semantics for revocation and freshness.
* **KP-1**: Uses key state, freshness, and rotation properties for scoring.
* **TP-1**: Ensures trust path construction and issuer hierarchy are honored.
* **Deployment Guide**: Governs stable vs pinned artifact lookup (impacts revocation freshness). 

DP-1 does **not** introduce new trust or security requirements.
It only defines unambiguous resolution behavior.

---

# **10. Versioning and Change Control**

* DP-1 v1.0.0 is aligned with AP-1 v1.*, RP-1 v1.*, KP-1 v1.*, TP-1 v1.0.0.
* Any change affecting:

  * scoring weights,
  * tie-breaking logic,
  * eligibility semantics,
  * ambiguity definition

requires a **minor version bump**.

* Any breaking change requires a **major version bump** and MUST be recorded in `/trust/CHANGELOG.md`.

---

# **11. Summary**

DP-1 provides the Attested Identity ecosystem with:

* Deterministic selection
* Predictable ambiguity handling
* Strong defensive posture
* A consistent scoring model
* Tie-breaking logic aligned with trust-fabric principles

With AP-1, RP-1, KP-1, and TP-1 forming the trust and validation foundation, DP-1 delivers the final layer required to make the system **usable at scale**, especially in ATS, HR, and high-volume verification environments.

---

# **📎 Citations**

*KP-1 — Key Profile* — 
*RP-1 — Revocation Profile* — 
*AP-1 — Employment-Role Attestation Profile* — 
*Deployment Guide — Trust Artifacts Publishing* — 
*TP-1 — Trust Path Specification* — 

---
