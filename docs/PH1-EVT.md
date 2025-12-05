# PH-1 – Employment Verification Tokens (EVT)

Phase 1 is the **wedge**: a minimal, production-minded format for employers to **cryptographically sign employment verification** and for verifiers to check those claims.

This phase does **one thing well**:

> “Did this person actually work here in this role, during this time window, in this capacity?”

No skills scoring, no outcomes metrics, no résumé reconstruction — just clean, signed employment facts.

---

## Goals

- Give employers a **low-friction way** to issue signed employment records.
- Give verifiers (ATS, HR systems, background check tools) a **simple API** to:
  - verify signature and trust chain,
  - check validity window,
  - confirm revocation state.
- Keep the shape small and boring so it can realistically be adopted in pilots.

---

## Canonical EVT JSON Shape

The schema lives at:

```text
schemas/PH-1-EVT/employment.verification.v1.json
