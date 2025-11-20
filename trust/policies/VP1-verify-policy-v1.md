# Verifier Policy v1 — VP-1 Profile

**ID:** `VP1-verify-policy-v1`  
**Version:** `1.0.0`  
**Purpose.** Define verification behavior for relying parties (ATS/HR systems) for `employment.role` attestations, aligned with **POL-1 / VP-1: Verification Policy Layer**.

This policy is a **concrete profile** of VP-1. It assumes that all protocol-level checks are implemented by:

- **AP-1** (schema & claim semantics),
- **RP-1** (revocation),
- **KP-1** (keys),
- **TP-1** (trust path),
- **DP-1** (local disambiguation).

VP-1 `vp-1.verify-v1` sits *above* these and defines how to interpret DP-1 results for relying parties.

---

## 1. Protocol Dependencies (Informative)

`vp-1.verify-v1` relies on the following protocol behavior (defined in their own specs):

- **JWKS & Signature** — KP-1 / TP-1:
  - Fetch JWKS, match `kid`, validate JWS signature.
  - Validate trust chain (issuing CA → root CA).

- **Validity Window & Schema** — AP-1:
  - Enforce `not_before` / `not_after` with ±5 min clock skew.
  - Validate `schema_uri` against canonical schemas.

- **Revocation** — RP-1:
  - Evaluate revocation using `statuslist.json`.

The present policy does **not** redefine these rules; it only defines **policy behavior** once these checks have produced a DP-1 result.

---

## 2. Policy Profile (`vp-1.verify-v1`)

### 2.1 Risk Tier

- `risk_tier = "MEDIUM"`  
  Intended for general ATS / HR verification where credentials materially matter but are not life-critical.

### 2.2 Scope

- `applies_to.claim_types = ["employment.role"]` for v1.

---

## 3. Scoring & Ambiguity

- This legacy policy **does not introduce soft ambiguity**.  
- It accepts DP-1’s `OK` winner as long as recency and trust constraints are satisfied.

**Configuration:**

- `min_score_accept = 0`  
- `min_score_review = 0`  
- `soft_ambiguity.enable = false`

Implication:  
Under this policy, DP-1’s composite score is **recorded** but **not used** to gate decisions. If DP-1 says `OK`, the policy either ACCEPTs or REJECTs purely based on recency/trust.

- `disposition.no_valid_attestations = "REJECT"`  
- `disposition.ambiguous_result = "REJECT"`  

Implication:  
- If DP-1 returns `NO_VALID_ATTESTATIONS` or `AMBIGUOUS_RESULT`, the relying party gets a **hard INVALID** result (fail closed).

---

## 4. Recency / Revocation

The original policy stated:

- *“Check `statuslist.json` (fail closed on stale cache).”*  
- *“Status list cache ≤ 15 minutes (configurable).”*

In VP-1 terms:

- `recency.max_revocation_age_seconds = 900` (15 minutes)

Policy behavior:

- If `revocation.checked_at` is older than 900 seconds relative to `context.now`, VP-1 MUST return:
  - `decision = "REJECT"`, `reason = "REVOCATION_STALE"`.

This encodes the **“fail closed on stale cache”** rule.

---

## 5. Trust Constraints

For v1:

- `trust_constraints.min_trust_path_strength = 1`  
  (Require at least a minimally validated path.)

- `trust_constraints.allowed_key_states = ["active", "grace", "deprecated"]`  
  (Compromised keys are excluded at protocol level by KP-1.)

Behavior:

- If `issuer.trust_path_strength < 1` → `decision = "REJECT"`, `reason = "TRUST_PATH_TOO_WEAK"`.
- If `key.state` ∉ allowed set → `decision = "REJECT"`, `reason = "KEY_STATE_NOT_ALLOWED"`.

---

## 6. Caching (Operational, Informative)

Operational guidance (non-normative in VP-1 JSON, but recommended defaults):

- **JWKS cache TTL:** ≤ 1 hour.
- **Status list cache TTL:** ≤ 15 minutes (aligned with `max_revocation_age_seconds`).

Implementations SHOULD:

- Treat cache entries older than these TTLs as expired and refresh them.
- For status list, if a refresh fails and the existing entry is older than 15 min, VP-1 recency rules force a **REJECT** (`REVOCATION_STALE`).

---

## 7. Results & Receipts

Original:

- *“Emit `VerificationReceipt` with `VALID` or `INVALID` (include reasons).”*
- *“Sign receipts when stored server-side.”*

In VP-1 terms:

- Map `VerificationDecision` to receipt status:

  - If `decision = "ACCEPT"` → receipt `status = "VALID"`.
  - If `decision = "REJECT"` → receipt `status = "INVALID"`.
  - If `decision = "REVIEW"` (not used in v1, but reserved) → receipt `status = "REVIEW_REQUIRED"`.

- `audit.emit_verification_receipt = true`
- `audit.include_scores = true` (include DP-1 score in the receipt)
- `audit.include_candidate_set_hash = true` (to bind decision to input set)
- Receipts SHOULD be signed server-side per TP-1 / KP-1 guidance.

---

## 8. Versioning

- Policy URI: `https://<issuer>/policies/verify-v1`.  
- `policy.id = "vp-1.verify-v1"`, `policy.version = "1.0.0"`.

Any change to:

- recency thresholds,
- disposition for DP-1 `NO_VALID_ATTESTATIONS` / `AMBIGUOUS_RESULT`,
- trust constraints,

MUST result in at least a **minor** version bump of this policy.
