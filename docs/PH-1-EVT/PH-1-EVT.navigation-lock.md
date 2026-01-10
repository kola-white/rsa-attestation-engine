# PH-1 EVT – Recruiter UX Navigation Lockdown (Expo iOS)

**Status:** LOCKED (Phase 1)
**Scope:** Recruiter candidate search → filter → candidate detail
**Guarantees:** Signature badge, Trust badge (mandatory), optional “Why?” disclosure
**Non-goals:** Root CA display, key material display, advanced policy editing UX

---

## 1) Screens and Navigation Topology (LOCKED)

### 1.1 Routes (React Navigation)

* `RecruiterCandidates` — root screen
* `CandidateDetail` — pushed detail screen
* `RecruiterFilters` — modal sheet

### 1.2 Allowed Transitions (only these)

```
RecruiterCandidates
     ├─ push ──► CandidateDetail
     └─ present ──► RecruiterFilters

RecruiterFilters
     └─ dismiss ──► RecruiterCandidates

CandidateDetail
     └─ back ──► RecruiterCandidates
```

No other navigation paths are permitted in Phase 1.

---

## 2) State Preservation Contract (LOCKED)

### 2.1 Back Navigation (Detail → List)

When returning from `CandidateDetail` to `RecruiterCandidates`, the app **must preserve**:

* Search text
* Active filter chips
* Sort selection (if present)
* Scroll position (anchored by stable `candidate_id`, never index)

### 2.2 Filter Sheet Dismissal

* **Done** → apply selections and update list
* **Reset** → clears selections without dismissing
* **Swipe-down** → behaves exactly like Done

### 2.3 Refresh Behavior

* `RecruiterCandidates`: pull-to-refresh refetches list using current query state
* `CandidateDetail`: pull-to-refresh refetches candidate + EVT verification outcomes

Errors render **inline** with Retry; no error-driven navigation.

---

## 3) Route Param Contract (LOCKED)

### 3.1 Route Param Definitions

| Route                 | Params                   |
| --------------------- | ------------------------ |
| `RecruiterCandidates` | `undefined`              |
| `RecruiterFilters`    | `RecruiterFiltersParams` |
| `CandidateDetail`     | `CandidateDetailParams`  |

---

### 3.2 `CandidateDetailParams`

CandidateDetail receives **only stable identifiers plus a render-safe snapshot**.

#### Required

* `candidate_id: string`
  Stable internal ID (never PII).
* `subject_ref: { full_name: string; employee_id?: string }`
  Header identity (render-only, may be stale).
* `primary_evt_ref: { evt_id: string }`
  EVT record selected from the list.

#### Optional (UX optimization)

* `list_context?: {`

  * `search?: string`
  * `filters_hash?: string`
  * `sort?: string`
  * `anchor_key?: string`
  * `}`
* `prefetch_snapshot?: CandidateRowSnapshot`

---

### 3.3 `RecruiterFiltersParams`

* `initial: RecruiterQueryState`
* `on_apply_id?: string` (optional internal coordination hook)

---

### 3.4 `RecruiterQueryState`

Canonical list query state:

```ts
{
  search: string
  trust_mode: "any" | "trusted_only" | "include_untrusted"
  signature_status: Array<"verified" | "invalid" | "unknown">
  company_ids: string[]
  title_query?: string
  dates?: {
    start_after?: string
    end_before?: string
    include_current?: boolean
  }
  sort?: "most_recent" | "name_az" | "trust_first"
  page?: { cursor?: string; limit?: number }
}
```

**Notes**

* `trust_mode` is a **platform/protocol choice**, enabling future per-customer policy monetization.
* Root CA concepts are **never** surfaced in UI state.

---

## 4) Minimum Server Response Fields (NO GUESSING)

The UI renders:

* **Signature badge** (always)
* **Trust badge** (always)
* **Why? disclosure** (conditionally)

These fields are the minimum required.

---

## 5) API: Candidate List (Minimum Contract)

### 5.1 Request

```
GET /v1/recruiter/candidates
```

Query params derived directly from `RecruiterQueryState`.

### 5.2 Response

```json
{
  "results": [CandidateListItem],
  "next_cursor": null
}
```

#### `CandidateListItem`

```ts
{
  candidate_id: string
  subject: { full_name: string; employee_id?: string }
  primary_employment: {
    issuer_name: string
    title: string
    start_date: string
    end_date: string | null
  }
  primary_evt: { evt_id: string }
  badges: {
    signature: "verified" | "invalid" | "unknown"
    trust: "trusted" | "untrusted" | "unknown"
  }
  updated_at: string
}
```

**Invariant**

* `signature = verified` **does not imply** `trust = trusted`

---

## 6) API: Candidate Detail (Minimum Contract)

### 6.1 Request

```
GET /v1/recruiter/candidates/:candidate_id
```

### 6.2 Response

```ts
{
  candidate: {
    candidate_id: string
    subject: { full_name: string; employee_id?: string }
  }

  summary: {
    primary_employment: {
      issuer_name: string
      title: string
      start_date: string
      end_date: string | null
    }
    badges: {
      signature: "verified" | "invalid" | "unknown"
      trust: "trusted" | "untrusted" | "unknown"
    }
  }

  why?: {
    trust_decision: "trusted" | "untrusted" | "unknown"
    summary: string
    code: string
  }

  records: CandidateEmploymentRecord[]
}
```

#### `CandidateEmploymentRecord`

```ts
{
  evt_id: string
  issuer: { id: string; name: string }
  employment: {
    title: string
    start_date: string
    end_date: string | null
    employment_type?: string
  }
  validity: { issued_at: string }
  badges: {
    signature: "verified" | "invalid" | "unknown"
    trust: "trusted" | "untrusted" | "unknown"
  }
  why?: { summary: string; code: string }
  checks?: {
    validity_window: "valid_now" | "not_valid_now" | "unknown"
    revocation: "not_revoked" | "revoked" | "unknown"
  }
}
```

**Hard rule**

* Root CA, JWKS, key IDs, chain details are never returned.

---

## 7) Badge + Disclosure Rendering Rules (LOCKED)

### 7.1 Badge Vocabulary

| Badge     | Values                        |
| --------- | ----------------------------- |
| Signature | Verified · Invalid · Unknown  |
| Trust     | Trusted · Untrusted · Unknown |

### 7.2 “Why?” Disclosure Rules

Render the **Why?** block **only if**:

* `trust ∈ {untrusted, unknown}`
* AND both `summary` and `code` are present

This applies at:

* Candidate summary level
* Individual employment record level

---

## 8) ASCII UX Diagrams (LOCKED)

### 8.1 RecruiterCandidates

```
┌──────────────────────────────────────────────────────────────┐
│ Candidates                                    [ Filter ]     │
├──────────────────────────────────────────────────────────────┤
│ 🔍 Search name, company, title, employee_id                  │
├──────────────────────────────────────────────────────────────┤
│ [Trust: Any ✕] [Signature: Verified ✕] [Company: Acme ✕]    │
│                                              Clear all      │
├──────────────────────────────────────────────────────────────┤
│ Jane Doe                                                     │
│ Acme Corp • Senior Electrician                               │
│ Jan 2019 – Mar 2023                                          │
│ Signature: Verified ✔      Trust: Trusted 🔒                │
│ evt_id: evt_01HX…                                            │
│                                                              │
│ Michael Smith                                                │
│ NorStar Tech • Project Manager                               │
│ Apr 2020 – Present                                           │
│ Signature: Verified ✔      Trust: Untrusted ⚠︎              │
│ evt_id: evt_01HY…                                            │
└──────────────────────────────────────────────────────────────┘
```

---

### 8.2 RecruiterFilters (Modal)

```
┌──────────────────────────────────────────────────────────────┐
│ Reset            Filters                           Done      │
├──────────────────────────────────────────────────────────────┤
│ Trust (platform policy)                                     │
│  ( ) Trusted issuers only                                   │
│  (●) Allow untrusted issuers                                │
│                                                              │
│ Signature Verification                                      │
│  [✔] Verified   [ ] Invalid   [ ] Unknown                    │
│                                                              │
│ Company                                                      │
│  [✔] Acme Corp   [ ] NorStar Tech                            │
│                                                              │
│ Employment Dates                                             │
│  Start after: [ Jan 2018 ]                                   │
│  End before:  [ Present ]                                    │
├──────────────────────────────────────────────────────────────┤
│                Show Results (23)                             │
└──────────────────────────────────────────────────────────────┘
```

---

### 8.3 CandidateDetail

```
┌──────────────────────────────────────────────────────────────┐
│ ‹ Candidates                              Michael Smith      │
├──────────────────────────────────────────────────────────────┤
│ Signature: Verified ✔     Trust: Untrusted ⚠︎               │
│ NorStar Tech • Project Manager                               │
│ Apr 2020 – Present                                           │
│                                                              │
│ Why?                                                         │
│ Issuer not approved by your company policy                   │
│ Code: POLICY_DENY                                            │
│                                                              │
│ Employment Records                                           │
│ ──────────────────────────────────────────────────────────  │
│ NorStar Tech – Project Manager                               │
│ Signature: Verified ✔   Trust: Untrusted ⚠︎                 │
│                                                              │
│ Acme Corp – Electrician                                      │
│ Signature: Verified ✔   Trust: Trusted 🔒                   │
└──────────────────────────────────────────────────────────────┘
```

---

## 9) Final Lock Statement

This document fully locks:

* Navigation topology
* Route params
* Badge semantics
* Trust vs signature separation
* Optional disclosure placement
* No-root-CA UX invariant

---