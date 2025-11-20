# **POL-1 / VP-1: Verification Policy Layer (v1.0.0 — Draft)**

**Attested Identity — Verification Policy Layer for Human-Claim Attestations**

**Status:** Draft (Discover Phase)  
**Editors:** Whitehouse, Coe  
**Depends on:**  
- AP-1 (Attestation Profile)  
- RP-1 (Revocation Profile)  
- KP-1 (Key Profile)  
- TP-1 (Trust Path Specification)  
- DP-1 (Disambiguation Protocol)  

**Audience:** Product Owners, Verifier Integrators, Security / Risk Teams

---

## 1. Purpose & Positioning

**POL-1 / VP-1** defines the **Verification Policy Layer** that sits *above* the protocol stack:

- AP-1 / RP-1 / KP-1 / TP-1 → core validity & trust path
- DP-1 → deterministic disambiguation (local winner / ambiguity)
- **VP-1 (this doc)** → **policy knobs** that reflect business risk appetite and use case

VP-1:

- **Does**:
  - Interpret DP-1 outputs in light of configurable **risk profiles**.
  - Introduce **soft ambiguity** thresholds (score margins, caps).
  - Enforce **recency** requirements (claim age, revocation freshness).
  - Emit a **final decision** suitable for product/business use:
    - `ACCEPT`, `REVIEW`, `REJECT`, plus optional risk labels.

- **Does NOT**:
  - Change DP-1’s scoring or tie-break semantics.
  - Alter AP-1 / RP-1 / KP-1 / TP-1 behavior.
  - Encode domain-specific hiring / compliance law by itself.

Those remain strictly protocol-level and/or tenant-specific policy content.

VP-1 is the **monetization surface**: tenants buy customized verification policies (risk knobs, soft ambiguity profiles, recency guarantees) mapped to their use cases.

---

## 2. Evaluation Pipeline (High-Level)

A complete verification run with policy looks like:

```text
Raw JWS/attestations
   ⬇  AP-1 / KP-1 / TP-1 / RP-1
Normalized candidates (subject+claim_type)
   ⬇  DP-1.disambiguate(...)
Local disambiguation result (winner / ambiguity / none)
   ⬇  VP-1.apply_policy(policy, result, context, extras)
Final decision: ACCEPT / REVIEW / REJECT (+ risk label, receipt)
````

VP-1 **never** bypasses DP-1. It wraps the DP-1 result and applies additional, policy-driven constraints.

---

## 3. Inputs and Outputs (Normative)

### 3.1 Function Signature

VP-1 is defined as:

```ts
verify_with_policy(
  policy: VerificationPolicy,
  subject_id: string,
  claim_type: string,
  candidates: NormalizedCandidate[],
  context: VerificationContext
) -> VerificationDecision
```

Where:

* `policy`: a `VerificationPolicy` document (see §4).
* `subject_id`, `claim_type`: same as DP-1.
* `candidates[]`: **DP-1 NormalizedCandidate** set (already AP-1 / RP-1 / KP-1 / TP-1 processed).
* `context`:

  ```ts
  type VerificationContext = {
    now: string; // ISO 8601 UTC
    revocation_mode: "stable" | "pinned";
    scoring_version: string; // e.g. "v1"
    tenant_id?: string;
    use_case?: "screening" | "hiring" | "compliance" | "other";
  };
  ```

The VP-1 implementation MUST:

1. Invoke `DP-1.disambiguate(...)` with the `candidates` & `context`.
2. Then apply the configured `policy` to that result and surrounding metadata.

### 3.2 Policy-Aware Candidate (Informative)

For recency knobs, VP-1 **may** consume extra, optional fields that are not required by DP-1:

```ts
type PolicyCandidateExtras = {
  issued_at?: string;     // when the attestation was first issued
  last_seen_at?: string;  // when this attestation was last observed / refreshed
};
```

If present, these fields MAY be used in recency calculations. If absent, VP-1 falls back to DP-1’s temporal and revocation data.

### 3.3 VerificationDecision (Normative)

VP-1 outputs a **decision envelope**:

```ts
type VerificationDecision =
  | {
      ok: true;
      decision: "ACCEPT" | "REVIEW";
      risk_label: "LOW" | "MEDIUM" | "HIGH" | "UNKNOWN";
      reason: string; // short machine-readable code
      attestation?: NormalizedCandidate; // present for ACCEPT/REVIEW with winner
      dp1: DisambiguationResult;         // passthrough of DP-1 result
      policy_id: string;
      policy_version: string;
      context: VerificationContext;
    }
  | {
      ok: false;
      decision: "REJECT";
      risk_label: "HIGH" | "UNKNOWN";
      reason: string; // e.g. "NO_VALID_ATTESTATIONS", "EXPIRED", "SOFT_AMBIGUITY"
      attestation?: NormalizedCandidate; // optional (e.g. best-effort candidate)
      dp1: DisambiguationResult;
      policy_id: string;
      policy_version: string;
      context: VerificationContext;
    };
```

Notes:

* `decision = "ACCEPT"` → issuer & protocol stack are trusted *enough* under this policy.
* `decision = "REVIEW"` → protocol stack OK, but policy wants human-in-the-loop.
* `decision = "REJECT"` → policy-level hard stop (no auto-use of this attestation).

---

## 4. VerificationPolicy Model

A **VerificationPolicy** is a JSON document (e.g. `trust/policies/policy.json`) with this logical shape:

```ts
type VerificationPolicy = {
  id: string;        // e.g. "vp-1.default.screening"
  version: string;   // semantic version, e.g. "1.0.0"
  description?: string;

  risk_tier: "LOW" | "MEDIUM" | "HIGH" | "CUSTOM";

  // Which claims this policy applies to
  applies_to: {
    claim_types: string[]; // e.g. ["employment.role"]
  };

  // Score + ambiguity knobs
  scoring: {
    min_score_accept: number;       // e.g. 70
    min_score_review: number;       // e.g. 50
    // everything below min_score_review => REJECT

    soft_ambiguity: {
      enable: boolean;              // if false -> strictly follow DP-1
      margin_soft: number;          // e.g. 5
      margin_hard: number;          // e.g. 15
      max_candidates_considered: number; // e.g. 5
    };
  };

  // Recency / freshness knobs
  recency: {
    // attestation “age” constraints
    max_claim_age_days?: number;      // e.g. 365
    max_issued_age_days?: number;     // if issued_at exists
    max_last_seen_age_days?: number;  // if last_seen_at exists

    // revocation freshness expectations
    max_revocation_age_seconds: number; // e.g. 86400 (24h)
  };

  // Issuer / key trust constraints
  trust_constraints: {
    min_trust_path_strength: number;  // 0–3, from DP-1
    allowed_key_states: ("active" | "grace" | "deprecated")[];
  };

  // Mapping DP-1 result + context to final decision
  disposition: {
    no_valid_attestations: "REJECT" | "REVIEW";
    ambiguous_result: "REJECT" | "REVIEW";
  };

  // Audit behavior and receipt policy
  audit: {
    emit_verification_receipt: boolean; // e.g. controls writing to verification-receipt.example.json
    include_scores: boolean;
    include_candidate_set_hash: boolean;
  };
};
```

**Normative rules:**

* `min_score_accept` MUST be ≥ `min_score_review`.
* `soft_ambiguity.margin_hard` MUST be ≥ `soft_ambiguity.margin_soft`.
* `max_revocation_age_seconds` MUST be > 0.
* A policy MUST declare exactly one `risk_tier`.

---

## 5. Soft Ambiguity Model

Soft ambiguity is where you make money: **tunable risk** vs **false positive/negative comfort**.

### 5.1 Margin Definitions

Let:

* `dp1_result` = output of `DP-1.disambiguate(...)`.
* `S1` = score of top candidate (if any).
* `S2` = score of the second-best candidate (if present, else `S2 = -∞`).
* `margin = S1 - S2`.

The policy defines:

* `margin_hard`: if `margin >= margin_hard`, policy considers the winner **decisive**.
* `margin_soft`: if `margin <= margin_soft`, policy considers the winner **too close** to be auto-accepted.
* If `margin_soft < margin < margin_hard`, the winner exists but is **borderline**.

### 5.2 Behavior by Zone (Normative)

Given `dp1_result.ok === true` and `policy.scoring.soft_ambiguity.enable === true`:

1. **Decisive Zone** (`margin >= margin_hard`):

   * If `S1 >= min_score_accept` and all other policy constraints pass:

     * `decision = "ACCEPT"`, `risk_label = "LOW" | "MEDIUM"` depending on `risk_tier`.
   * Else if `S1 >= min_score_review`:

     * `decision = "REVIEW"`.
   * Else:

     * `decision = "REJECT"`.

2. **Borderline Zone** (`margin_soft < margin < margin_hard`):

   * If `S1 >= min_score_accept`:

     * `decision = "REVIEW"` (policy forces human review despite high score).
   * Else if `S1 >= min_score_review`:

     * `decision = "REVIEW"`.
   * Else:

     * `decision = "REJECT"`.

3. **Soft Ambiguity Zone** (`margin <= margin_soft`):

   * Policy MUST NOT auto-accept.
   * If `policy.disposition.ambiguous_result === "REVIEW"`:

     * `decision = "REVIEW"`, `reason = "SOFT_AMBIGUITY"`.
   * Else:

     * `decision = "REJECT"`, `reason = "SOFT_AMBIGUITY"`.

If `soft_ambiguity.enable === false`, then VP-1 behaves as if **margin is not considered at all**, and only `min_score_*` and recency/trust constraints are applied.

---

## 6. Recency and Freshness Rules

VP-1 introduces explicit **recency guarantees**:

Given `context.now = T`, candidate fields:

* `not_before`, `not_after` (from DP-1).
* `revocation.checked_at`.
* Optional `issued_at` and `last_seen_at`.

### 6.1 Claim Age

If `recency.max_claim_age_days` is set:

```text
claim_age_days = floor((T - not_before) / 1 day)

If claim_age_days > recency.max_claim_age_days:
  -> policy-level violation
```

Policy behavior:

* If violation occurs and `risk_tier = "HIGH"`:

  * `decision = "REJECT"`, `reason = "CLAIM_TOO_OLD"`.
* If violation occurs and `risk_tier ∈ {"LOW","MEDIUM","CUSTOM"}`:

  * Implementation MAY map this to `REVIEW` via policy extension; default: `REJECT`.

### 6.2 Issued Age (Optional)

If `issued_at` exists and `max_issued_age_days` is set:

```text
issued_age_days = floor((T - issued_at) / 1 day)

If issued_age_days > max_issued_age_days:
  -> treat as policy violation
```

Same disposition pattern as above.

### 6.3 Last-Seen Age (Optional)

If `last_seen_at` exists and `max_last_seen_age_days` is set:

```text
last_seen_age_days = floor((T - last_seen_at) / 1 day)

If last_seen_age_days > max_last_seen_age_days:
  -> policy violation
```

Used to capture “we haven’t seen a fresh version in a long time, even if validity window is large”.

### 6.4 Revocation Freshness

Compute:

```text
revocation_age_seconds = max(0, (T - revocation.checked_at) in seconds)

If revocation_age_seconds > recency.max_revocation_age_seconds:
  -> policy violation
```

Minimal rule:

* For `risk_tier = "HIGH"`:

  * Violation MUST result in `decision = "REJECT"` with `reason = "REVOCATION_STALE"`.
* For `risk_tier ∈ {"LOW","MEDIUM","CUSTOM"}`:

  * Default mapping: `decision = "REVIEW"` (requires human confirmation or re-check).
  * Tenant-specific overrides MAY upgrade this to `REJECT`.

---

## 7. Trust Constraints & Risk Tiers

VP-1 enforces coarse-grained trust constraints on top of DP-1’s scoring.

### 7.1 Min Trust Path Strength

If:

```text
candidate.issuer.trust_path_strength < trust_constraints.min_trust_path_strength
```

then:

* For `risk_tier = "HIGH"` → `decision = "REJECT"`, `reason = "TRUST_PATH_TOO_WEAK"`.
* For `risk_tier = "MEDIUM"` → `decision = "REVIEW"` by default.
* For `risk_tier = "LOW"` → implementation MAY still accept if score is high; recommended default: `REVIEW`.

### 7.2 Allowed Key States

If `candidate.key.state` is **not** in `trust_constraints.allowed_key_states`:

* MUST result in at least `decision = "REVIEW"`.
* For `risk_tier = "HIGH"`, recommended default is `REJECT`.

This lets you e.g. forbid `deprecated` keys for high-risk hiring, while still allowing them for low-risk screening.

---

## 8. Mapping DP-1 Outcomes to Decisions

VP-1 MUST always start from DP-1’s canonical result:

### 8.1 NO_VALID_ATTESTATIONS

If:

```json
{ "ok": false, "code": "NO_VALID_ATTESTATIONS", ... }
```

Then:

* `decision = policy.disposition.no_valid_attestations` (`REJECT` or `REVIEW`).
* `reason = "NO_VALID_ATTESTATIONS"`.
* `risk_label = "HIGH"` if risk_tier = "HIGH", else `"UNKNOWN"`.

### 8.2 AMBIGUOUS_RESULT (Exact Tie)

If:

```json
{ "ok": false, "code": "AMBIGUOUS_RESULT", ... }
```

Then:

* If `soft_ambiguity.enable === true`:

  * `decision = policy.disposition.ambiguous_result` (`REJECT` or `REVIEW`).
  * `reason = "AMBIGUOUS_RESULT"`.
* If `soft_ambiguity.enable === false`:

  * Policy MUST treat this as **hard ambiguity**:

    * RECOMMENDED default: `decision = "REJECT"`.

### 8.3 OK (Unique Winner)

If:

```json
{ "ok": true, "code": "OK", "attestation": ..., "score": S1 }
```

Then VP-1:

1. Checks recency rules (§6); if violation → `REVIEW` or `REJECT`.
2. Checks trust constraints (§7); if violation → `REVIEW` or `REJECT`.
3. If still not determined, evaluates **score** vs `min_score_*` and **soft ambiguity** (§5).

This preserves protocol semantics but adds policy-level risk gating.

---

## 9. Recommended Default Policy Profiles (Informative)

You can ship pre-baked policies as SKUs in `/trust/policies/`:

### 9.1 `vp-1.default.screening` (LOW risk)

* `risk_tier`: `"LOW"`
* `scoring`:

  * `min_score_accept = 60`
  * `min_score_review = 40`
  * `soft_ambiguity.enable = true`
  * `margin_soft = 3`, `margin_hard = 10`
  * `max_candidates_considered = 5`
* `recency`:

  * `max_claim_age_days = 730` (2 years)
  * `max_revocation_age_seconds = 86400` (24h)
* `trust_constraints`:

  * `min_trust_path_strength = 1`
  * `allowed_key_states = ["active", "grace", "deprecated"]`
* `disposition`:

  * `no_valid_attestations = "REJECT"`
  * `ambiguous_result = "REVIEW"`

### 9.2 `vp-1.default.hiring` (MEDIUM risk)

* `risk_tier`: `"MEDIUM"`
* `scoring`:

  * `min_score_accept = 70`
  * `min_score_review = 55`
  * `soft_ambiguity.enable = true`
  * `margin_soft = 5`, `margin_hard = 15`
* `recency`:

  * `max_claim_age_days = 365`
  * `max_revocation_age_seconds = 43200` (12h)
* `trust_constraints`:

  * `min_trust_path_strength = 2`
  * `allowed_key_states = ["active", "grace"]`

### 9.3 `vp-1.default.clearance` (HIGH risk)

* `risk_tier`: `"HIGH"`
* `scoring`:

  * `min_score_accept = 85`
  * `min_score_review = 70`
  * `soft_ambiguity.enable = true`
  * `margin_soft = 8`, `margin_hard = 20`
* `recency`:

  * `max_claim_age_days = 180`
  * `max_revocation_age_seconds = 3600` (1h)
* `trust_constraints`:

  * `min_trust_path_strength = 3`
  * `allowed_key_states = ["active"]`
* `disposition`:

  * `no_valid_attestations = "REJECT"`
  * `ambiguous_result = "REJECT"`

These are **productizable** tiers: different tenants can pay for higher assurance profiles and/or custom knobs.

---

## 10. Normative Pseudocode

```ts
function verify_with_policy(policy, subject_id, claim_type, candidates, ctx): VerificationDecision {
  const dp1 = disambiguate(subject_id, claim_type, candidates, ctx);

  // 1) Handle non-OK DP-1 results
  if (!dp1.ok) {
    if (dp1.code === "NO_VALID_ATTESTATIONS") {
      return {
        ok: false,
        decision: policy.disposition.no_valid_attestations,
        risk_label: policy.risk_tier === "HIGH" ? "HIGH" : "UNKNOWN",
        reason: "NO_VALID_ATTESTATIONS",
        dp1,
        policy_id: policy.id,
        policy_version: policy.version,
        context: ctx
      };
    }

    if (dp1.code === "AMBIGUOUS_RESULT") {
      const decision = policy.disposition.ambiguous_result;
      return {
        ok: decision === "ACCEPT" ? true : false, // but we recommend REVIEW/REJECT only
        decision,
        risk_label: "UNKNOWN",
        reason: "AMBIGUOUS_RESULT",
        dp1,
        policy_id: policy.id,
        policy_version: policy.version,
        context: ctx
      };
    }
  }

  // 2) OK result: enforce policy
  const winner = dp1.attestation!;
  const score = dp1.score;
  const now = new Date(ctx.now);

  // Helper: policy violation -> decision
  function hardReject(reason: string): VerificationDecision {
    return {
      ok: false,
      decision: "REJECT",
      risk_label: policy.risk_tier === "HIGH" ? "HIGH" : "UNKNOWN",
      reason,
      attestation: winner,
      dp1,
      policy_id: policy.id,
      policy_version: policy.version,
      context: ctx
    };
  }

  // 2.1 Recency checks (simplified, expand per §6)
  // revocation freshness
  const revCheckedAt = new Date(winner.revocation.checked_at);
  const revAgeSec = Math.max(0, (now.getTime() - revCheckedAt.getTime()) / 1000);
  if (revAgeSec > policy.recency.max_revocation_age_seconds) {
    if (policy.risk_tier === "HIGH") return hardReject("REVOCATION_STALE");
    // MEDIUM/LOW: REVIEW
    return {
      ok: true,
      decision: "REVIEW",
      risk_label: policy.risk_tier,
      reason: "REVOCATION_STALE",
      attestation: winner,
      dp1,
      policy_id: policy.id,
      policy_version: policy.version,
      context: ctx
    };
  }

  // 2.2 Trust constraints
  if (winner.issuer.trust_path_strength < policy.trust_constraints.min_trust_path_strength) {
    if (policy.risk_tier === "HIGH") return hardReject("TRUST_PATH_TOO_WEAK");
    return {
      ok: true,
      decision: "REVIEW",
      risk_label: policy.risk_tier,
      reason: "TRUST_PATH_TOO_WEAK",
      attestation: winner,
      dp1,
      policy_id: policy.id,
      policy_version: policy.version,
      context: ctx
    };
  }

  if (!policy.trust_constraints.allowed_key_states.includes(winner.key.state)) {
    if (policy.risk_tier === "HIGH") return hardReject("KEY_STATE_NOT_ALLOWED");
    return {
      ok: true,
      decision: "REVIEW",
      risk_label: policy.risk_tier,
      reason: "KEY_STATE_NOT_ALLOWED",
      attestation: winner,
      dp1,
      policy_id: policy.id,
      policy_version: policy.version,
      context: ctx
    };
  }

  // 2.3 Soft ambiguity (margin-based)
  let margin = Infinity;
  if (policy.scoring.soft_ambiguity.enable) {
    const eligibleScored = /* reconstruct from DP-1 context or re-score */ [];
    // For spec purposes, assume we can obtain S2 when present
    // margin = S1 - S2 or Infinity if S2 absent
  }

  const { min_score_accept, min_score_review, soft_ambiguity } = policy.scoring;

  function accept(reason: string, risk_label: "LOW" | "MEDIUM" | "HIGH" | "UNKNOWN"): VerificationDecision {
    return {
      ok: true,
      decision: "ACCEPT",
      risk_label,
      reason,
      attestation: winner,
      dp1,
      policy_id: policy.id,
      policy_version: policy.version,
      context: ctx
    };
  }

  function review(reason: string): VerificationDecision {
    return {
      ok: true,
      decision: "REVIEW",
      risk_label: policy.risk_tier,
      reason,
      attestation: winner,
      dp1,
      policy_id: policy.id,
      policy_version: policy.version,
      context: ctx
    };
  }

  // No soft ambiguity → purely score-driven
  if (!soft_ambiguity.enable) {
    if (score >= min_score_accept) return accept("SCORE_STRONG", policy.risk_tier);
    if (score >= min_score_review) return review("SCORE_BORDERLINE");
    return hardReject("SCORE_TOO_LOW");
  }

  // Soft ambiguity enabled
  if (!Number.isFinite(margin)) {
    // no S2 → treat as decisive
    if (score >= min_score_accept) return accept("NO_COMPETING_CANDIDATE", policy.risk_tier);
    if (score >= min_score_review) return review("NO_COMPETING_CANDIDATE");
    return hardReject("SCORE_TOO_LOW");
  }

  if (margin >= soft_ambiguity.margin_hard) {
    // decisive zone
    if (score >= min_score_accept) return accept("DECISIVE_MARGIN", policy.risk_tier);
    if (score >= min_score_review) return review("DECISIVE_MARGIN_SCORE_WEAK");
    return hardReject("SCORE_TOO_LOW");
  }

  if (margin <= soft_ambiguity.margin_soft) {
    // soft ambiguity zone
    if (policy.disposition.ambiguous_result === "REVIEW") {
      return review("SOFT_AMBIGUITY");
    }
    return hardReject("SOFT_AMBIGUITY");
  }

  // borderline zone
  if (score >= min_score_accept) return review("BORDERLINE_MARGIN");
  if (score >= min_score_review) return review("BORDERLINE_MARGIN_SCORE_WEAK");
  return hardReject("SCORE_TOO_LOW");
}
```

---

## 11. Versioning and Change Control

* VP-1 v1.0.0 is aligned with DP-1 v1.1.1 and AP-1 / RP-1 / KP-1 / TP-1 v1.*.

* Any change to:

  * policy parameter semantics,
  * default profiles,
  * decision mapping rules,
  * risk tier definitions,

  requires at least a **minor version bump**.

* Breaking changes (e.g., new decision types, removal of fields) require a **major version bump** and MUST be recorded in `trust/changelog/` (or similar).

---

## 12. Protocol vs Policy (Explicit Separation)

* **Protocol (AP-1, RP-1, KP-1, TP-1, DP-1)**:

  * Defines *what is true* about a credential at a technical level.
  * Deterministic, implementation-independent, tenant-agnostic.

* **Policy (VP-1 / POL-1)**:

  * Defines *what is acceptable* for a given risk appetite and use case.
  * Tenant-specific, monetizable, and adjustable over time.

Optionality (soft ambiguity, recency strictness, trust thresholds) belongs here, not in DP-1.

---