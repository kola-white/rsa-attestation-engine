# PH-1 EVT Cheatsheet (Ultra-Short)

> Phase 1 wedge: **Employment Verification Tokens (EVT)**  
> “Did this person work here, in this role, during this time window, in this capacity?”

---

## 1. What PH-1 Covers (and *doesn’t*)

**Covers:**
- Employer
- Start / end dates (minimum month/year mm/yyyy)
- Role / capacity (optional)
- Employment type (full_time / part_time / contractor / etc.) (optional)

**Explicitly NOT PH-1:**
- Skills / levels
- Outcomes / metrics
- Résumé fields or disambiguation (DP-1)
- Student / admissions flows

Just **clean, signed employment facts**.

---

## 2. Where Things Live

**Specs & docs**

- Phase overview: `docs/PH-1-EVT.md`
- Key profile: `docs/KP1-key-profile.md` (KP-1)
- Revocation profile: `docs/RP1-revocation.md` (RP-1)
- Deployment: `docs/deployment.md`
- Runbook: `docs/runbook.md`
- Phase specs: `specs/PH-1-EVT/`

**Schemas**

- EVT schema (canonical):  
  `schemas/PH-1-EVT/employment.verification.v1.json`

**Trust directory**

- JWKS: `trust/jwks.json`
- Status list: `trust/statuslist.json`
- CA material (dev-only): `trust/root-ca/`, `trust/issuing-ca/`
- Audit logs: `trust/audit-logs/`

---

## 3. Minimal EVT Shape (Mental Model)

```jsonc
{
  "schema_uri": "schema/PH-1-EVT/employment.verification.v1",
  "issuer": { "id": "did:org:employer" },
  "key": { "kid": "issuer-dev-key-1", "alg": "RS256" },

  "subject": {
    "identifier": "mailto:user@example.com"
    // or HR ID, etc. (no heavy résumé fields here)
  },

  "employment": {
    "title": "Senior Project Manager",
    "start_date": "2023-01-01",
    "end_date": "2025-01-01",
    "employment_type": "full_time"
  },

  "validity": {
    "issued_at": "2025-01-01T00:00:00Z",
    "not_before": "2025-01-01T00:00:00Z",
    "not_after": "2028-01-01T00:00:00Z"
  },

  "revocation": {
    "method": "status-list",
    "pointer": "trust/statuslist.json",
    "serial": "evt-ULID-01HXYZ..."
  },

  "signature": {
    "alg": "RS256",
    "kid": "issuer-dev-key-1",
    "sig": "BASE64URL..."
  }
}
