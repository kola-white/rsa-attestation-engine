# **Phase 2 — Employment Verification Request Lifecycle**

**System:** Cvera / Employment Verification Tokens (EVT)
**Audience:** Platform, security, HR, legal, and enterprise customers
**Scope:** US-only, employee-initiated, employer-attested verification

---

## 1. Purpose

Phase-2 introduces the **Request Lifecycle**:
the process by which a candidate converts self-declared employment claims into a **cryptographically signed, employer-attested, machine-verifiable token** that can be trusted by recruiters, lenders, and HR systems.

This phase operationalizes the product’s core wedge:

> **“Did this person actually work here in this role, during this time window?”**

No scoring.
No hiring decisions.
No résumé reconstruction.
Only signed employment facts.

---

## 2. Legal & Policy Boundary

Cvera is a **verification protocol**, not a decision system.

This product operates as:

> **Employee-initiated, employer-signed attestation**

It is legally equivalent to:

* ADP employment letters
* DocuSign-signed HR statements
* Union verification letters
* The Work Number (when employee-initiated)

It is **not**:

* A consumer reporting agency
* A background investigator
* A hiring recommender

| Cvera does                    | Cvera does not         |
| ----------------------------- | ---------------------- |
| Sign employer facts           | Decide hiring outcomes |
| Verify cryptographic validity | Score candidates       |
| Enforce protocol              | Enforce policy         |
| Provide trust signals         | Provide judgments      |

All hiring, screening, or employment decisions remain the sole responsibility of the consuming employer.

---

## 3. Actors

```
+------------+      +------------------+      +------------------+
|  Candidate | ---> |  Cvera Platform   | ---> | Employer / HR     |
+------------+      +------------------+      +------------------+
                            |
                            v
                     Cryptographic Trust
                   (YubiHSM2 → CA → JWS)
```

---

## 4. The Request Lifecycle

Every employment verification request moves through the same **six legally-valid states**.

```
Draft → Submitted → Attested → Verified → Consumed → Closed
```

These states map cleanly to HR, legal, and cryptographic reality.

---

## 5. Lifecycle States

### 5.1 DRAFT

**Candidate composing a claim**

The candidate selects:

* employer
* job title
* start and end dates

No employer contact occurs.
No legal or cryptographic meaning exists yet.

```
[Candidate UI]
  └─ Edit Employment Claim
        └─ Save as Draft
```

State:

```
DRAFT
```

---

### 5.2 SUBMITTED

**Candidate formally requests verification**

Standing consent is invoked.
An immutable snapshot of the claim is recorded.

This creates:

* a `request_id`
* an audit record
* a legal basis to request employer verification

```
Candidate
   |
   v
+------------------+
|  SUBMITTED CLAIM |
+------------------+
```

State:

```
SUBMITTED
```

---

### 5.3 ATTESTED

**Employer signs or rejects the claim**

The employer (or its HR system) verifies the claim and responds.

Three machine outcomes are allowed:

* Full match
* Partial match
* Rejected (no record)

The response is cryptographically signed using:

```
YubiHSM2 → Issuing CA → JWS
```

```
Employer HR
     |
     v
+-------------------------+
| Cryptographic Attestation|
+-------------------------+
```

State:

```
ATTESTED   or   REJECTED
```

---

### 5.4 VERIFIED

**Cvera validates the attestation**

The platform verifies:

* CA chain
* key validity
* revocation
* timestamps
* schema
* status lists

This produces a **trust outcome**, not a hiring outcome.

Possible results:

* VERIFIED
* VERIFIED_WITH_FLAGS
* UNVERIFIED

```
Attestation JWS
       |
       v
+--------------------+
| Trust Evaluation   |
+--------------------+
       |
       v
   Trust Badge
```

State:

```
VERIFIED  or  UNVERIFIED
```

---

### 5.5 CONSUMED

**A recruiter or HR team uses the token**

The token is presented.
The trust badge is visible.
The employer decides what it means.

Cvera does not participate in the decision.

```
Recruiter UI
   └─ View EVT
       └─ Accept / Reject / Ignore
```

State:

```
CONSUMED
```

---

### 5.6 CLOSED

**The workflow ends**

The request lifecycle is complete.
The token remains:

* portable
* verifiable
* revocable
* reusable

But the transaction is finished.

State:

```
CLOSED
```

---

## 6. Why a Single Trust Badge

Each candidate receives **one trust badge**, not one per job.

Internally, a token may contain many attestations.
Externally, recruiters need one answer:

> “Can I trust this profile?”

Multiple badges would undermine usability and legal clarity.

---

## 7. Why This Model Is Legally Safe

Because:

* The candidate initiates
* Consent exists
* Employers sign their own data
* Cvera never evaluates people

The platform avoids:

* FCRA obligations
* adverse-action notices
* dispute arbitration
* liability for hiring outcomes

Cvera verifies facts.
Employers decide policy.

That boundary is absolute.

---

## 8. Phase-2 Contract

Phase-2 is complete when the platform can:

* Accept candidate claims
* Obtain employer attestations
* Produce cryptographically verified trust signals
* Allow employers to consume them
* Close the request lifecycle

All without becoming a decision engine.

---

**This document defines the canonical request lifecycle for EVT Phase-2.**

Any UI, API, database, or policy implementation must conform to these states.

---

# **9. Canonical System States (Operational Layer)**

The UI lifecycle maps to the following **system-level persistence states**:

| State                 | Meaning                                   |
| --------------------- | ----------------------------------------- |
| `DRAFT`               | Candidate is editing claims               |
| `SUBMITTED`           | Candidate formally requested verification |
| `ATTESTATION_PENDING` | Employer request dispatched               |
| `ATTESTED`            | Employer cryptographically responded      |
| `REJECTED`            | Employer declined or no record            |
| `VERIFIED`            | Cvera validated the attestation           |
| `UNVERIFIED`          | Attestation invalid or unverifiable       |
| `CONSUMED`            | Recruiter viewed token                    |
| `CLOSED`              | Lifecycle complete                        |

---

# **10. Role-Based Visibility & Actions**

## Roles

| Role         | Description                          |
| ------------ | ------------------------------------ |
| `Candidate`  | Employee requesting verification     |
| `EmployerHR` | Employer or HR system signing claims |
| `Recruiter`  | Third-party consumer of EVT          |
| `Cvera`      | Protocol and trust engine            |

---

## Candidate

| State                 | Visible | Allowed Actions         |
| --------------------- | ------- | ----------------------- |
| `DRAFT`               | Yes     | Edit, delete, submit    |
| `SUBMITTED`           | Yes     | Cancel (until attested) |
| `ATTESTATION_PENDING` | Yes     | Wait                    |
| `ATTESTED`            | Yes     | View employer response  |
| `REJECTED`            | Yes     | View                    |
| `VERIFIED`            | Yes     | Share token             |
| `UNVERIFIED`          | Yes     | Share with warning      |
| `CONSUMED`            | Yes     | View audit              |
| `CLOSED`              | Yes     | Reuse token             |

---

## Employer / HR

| State                 | Visible | Allowed Actions |
| --------------------- | ------- | --------------- |
| `SUBMITTED`           | Yes     | Review claim    |
| `ATTESTATION_PENDING` | Yes     | Sign or reject  |
| `ATTESTED`            | Yes     | Read-only       |
| `REJECTED`            | Yes     | Final           |
| `VERIFIED`            | No      | N/A             |
| `UNVERIFIED`          | No      | N/A             |

Employers never see trust evaluation — they only see what they signed.

---

## Recruiter

| State        | Visible | Allowed Actions |
| ------------ | ------- | --------------- |
| `VERIFIED`   | Yes     | View EVT        |
| `UNVERIFIED` | Yes     | View EVT        |
| `CONSUMED`   | Yes     | Acknowledge     |
| `CLOSED`     | Yes     | Re-verify       |

Recruiters never see drafts, submissions, or employer workflows.

---

## Cvera (Protocol Engine)

| State        | Responsibility               |
| ------------ | ---------------------------- |
| `ATTESTED`   | Validate JWS, CA, revocation |
| `VERIFIED`   | Publish trust result         |
| `UNVERIFIED` | Publish failure              |
| `CONSUMED`   | Record audit                 |
| `CLOSED`     | Archive lifecycle            |

Cvera never participates in hiring decisions.

---

# **11. State Transition Diagram**

```
[DRAFT]
   |
   v
[SUBMITTED]
   |
   v
[ATTESTATION_PENDING]
   |           \
   |            \
   v             v
[ATTESTED]    [REJECTED]
   |
   v
[VERIFIED] or [UNVERIFIED]
   |
   v
[CONSUMED]
   |
   v
[CLOSED]
```

---