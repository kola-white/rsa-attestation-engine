# **RP-1: Revocation Profile — Status-List Method (v1.0.0)**

**Scope:** Human employment verification claims (**PH-1 EVT / `employment.verification.v1`**)
**Status:** Final (PH-1 EVT)
**Audience:** Issuers, Verifiers, Security Engineers
**Depends on:** PH-1-EVT (Employment Verification Token), KP-1 (Key Profile), Deployment Guide
**Version:** 1.0.0

---

## **1. Purpose**

RP-1 defines the **canonical revocation mechanism** for attestations issued under **PH-1 EVT** using the **status-list** method.

This profile standardizes:

* The **status-list document format** (`trust/statuslist.json`)
* The **revocation event** object (issuer audit logs)
* The **verification algorithm** used to determine if an EVT is revoked
* The **allowed revocation statuses**
* Error semantics and canonical verifier return codes

The goal is revocation that is predictable, minimal, privacy-preserving, and compatible with high-throughput verification pipelines.

---

## **2. Applicability**

An EVT attestation conforms to RP-1 when its `revocation` stanza contains:

```jsonc
"revocation": {
  "method": "status-list",
  "pointer": "trust/statuslist.json",
  "serial": "att-SN-000389"
}
```

* `method` MUST be `"status-list"`
* `pointer` MUST identify a valid RP-1 status list
* `serial` MUST uniquely identify the attestation within that list

> **Note:**
> In production, `pointer` will typically be a full HTTPS URL (e.g. CDN path).
> The literal `"trust/statuslist.json"` value is a **dev / repo-local** convention for golden vectors.

The **golden revoked EVT** example adheres to this pattern.

---

## **3. Status-List Document**

The canonical status list is a JSON document located at:

```text
trust/statuslist.json
```

(Repo-local canonical; published via Digital Ocean CDN for verifiers.)

### **3.1 Structure**

```jsonc
{
  "version": 1,
  "issuer": "did:org:acme-corp",
  "generated_at": "2025-11-07T20:00:00Z",
  "ttl_s": 900,
  "entries": [
    {
      "serial": "att-SN-000389",
      "status": "revoked",
      "reason_code": "administrative",
      "time": "2025-11-01T09:00:00Z"
    },
    {
      "serial": "att-SN-000777",
      "status": "good"
    }
  ]
}
```

### **3.2 Field Requirements**

| Field          | Requirement                        | Notes                                |
| -------------- | ---------------------------------- | ------------------------------------ |
| `version`      | MUST be `1`                        | Versioning for forward compatibility |
| `issuer`       | MUST match `attestation.issuer.id` | Prevents cross-issuer poisoning      |
| `generated_at` | MUST be valid UTC timestamp        | Used for caching heuristics          |
| `ttl_s`        | SHOULD be honored by verifiers     | Soft guidance, not strict            |
| `entries`      | MAY be empty                       | Each entry maps serial → status      |

### **3.3 Entry Object**

| Field         | Requirement                           |
| ------------- | ------------------------------------- |
| `serial`      | MUST uniquely identify an attestation |
| `status`      | MUST be `"good"` or `"revoked"`       |
| `reason_code` | SHOULD be present for `"revoked"`     |
| `time`        | SHOULD indicate event timestamp       |

### **3.4 Allowed Status Values (Normative)**

RP-1 **ONLY** allows:

* `"good"`
* `"revoked"`

Any other value MUST cause the verifier to reject with:

```text
code: "REVOCATION_STATUS_INVALID"
```

---

## **4. Revocation Events (Issuer Audit)**

Issuers SHOULD record revocation events using the following structure:

```jsonc
{
  "request_id": "rev-ULID-01HDEF...",
  "attestation_id": "att-ULID-01HXYZ...",
  "issuer": { "request_id": "did:org:acme-corp" },
  "reason_code": "key_compromise",
  "reason_text": "Role misattributed; corrected record issued",
  "time": "2026-04-03T09:22:00Z",
  "supersedes": "att-ULID-OLD123",
  "signature": {
    "kid": "issuer-key-2026q2",
    "alg": "RS256",
    "sig": "BASE64URL..."
  }
}
```

Revocation events are **not** required for online revocation checks.
They exist for operational, compliance, and audit purposes.

---

## **5. Verification Algorithm (Normative)**

### **5.1 Precondition**

The EVT’s signature MUST already be verified according to **KP-1** (and the PH-1 EVT schema).
Revocation checks do **not** bypass signature requirements.

---

### **5.2 Algorithm**

Verifiers MUST follow these steps in order:

| Step                   | Logic                                                             |
| ---------------------- | ----------------------------------------------------------------- |
| **1. Check method**    | Reject if `revocation.method !== "status-list"`                   |
| **2. Resolve pointer** | Fetch `revocation.pointer` (e.g., CDN or `trust/statuslist.json`) |
| **3. Validate list**   | Reject if parse failure, issuer mismatch, or malformed            |
| **4. Find entry**      | `entry = entries.find(e => e.serial === serial)`                  |
| **5. Evaluate status** | Apply rules in §5.3                                               |

---

### **5.3 Status Evaluation Rules**

#### **Case A — Entry not found**

```text
outcome: ACCEPT
reason: unknown_not_listed
```

Result: **Not revoked**
Missing data is treated as “no revocation information,” not as implicit revocation.

---

#### **Case B — Entry has `status = "revoked"`**

```text
outcome: REJECT
reason: revoked
return code: REVOKED
```

Result: **Revoked**

---

#### **Case C — Entry has invalid status value**

```text
outcome: REJECT
reason: invalid_status_value
return code: REVOCATION_STATUS_INVALID
```

Result: Verifier **MUST** reject.
This prevents issuers from accidentally introducing non-standard states.

---

#### **Case D — Entry has `status = "good"`**

```text
outcome: ACCEPT
reason: good
```

Result: **Not revoked**

---

### **5.4 Additional Verifier Error Code**

Verifiers SHOULD emit:

```text
REVOCATION_SOURCE_UNAVAILABLE
```

when:

* status list cannot be fetched
* JSON is invalid
* issuer mismatch occurs
* structural validation fails

This distinguishes **operational errors** from actual revocation states.

---

## **6. Issuer Responsibilities**

Issuers implementing RP-1 MUST:

### **6.1 Assign a unique `serial`**

* MUST be unique per EVT attestation
* MUST be opaque (ULID/UUID/random)
* MUST NOT encode PII

### **6.2 Maintain the canonical status list**

* MUST be authoritative for all RP-1 EVTs
* MUST update entries atomically and predictably
* MUST avoid PII in list entries

### **6.3 Emit revocation events (SHOULD)**

* Append-only
* Signed under KP-1
* Internal / non-public

### **6.4 Keep status vocabulary strict**

Only `"good"` and `"revoked"` are permitted.

### **6.5 Respect PH-1 EVT and KP-1 invariants**

* Revocation does **not** replace signature validation.
* Signature failure does **not** imply revocation.

---

## **7. Verifier Responsibilities**

Verifiers claiming RP-1 conformance MUST:

* Implement all logic in §5
* Preserve audit logs (`stage: "revocation"`)
* Honor `ttl_s` when caching status lists
* Treat “not listed” as **not revoked**
* Enforce issuer match
* Enforce vocabulary constraints

They MUST map results to the canonical **RP-1 result codes**:

### **RP-1 Result Codes**

| Code                            | Meaning                              |
| ------------------------------- | ------------------------------------ |
| `REVOKED`                       | Attestation explicitly revoked       |
| `REVOCATION_STATUS_INVALID`     | Status value invalid / unsupported   |
| `REVOCATION_SOURCE_UNAVAILABLE` | Status list unreadable / unreachable |
| *none*                          | Attestation considered not revoked   |

---

## **8. Privacy Considerations**

Because RP-1 governs human-centric **employment verification**:

* Status lists MUST NOT contain PII.
* `reason_text` MUST NOT be published in public status lists.
* Revocation events SHOULD be stored privately.
* `serial` MUST NOT be reversible to an individual’s identity.
* “Not listed” MUST NOT be treated as suspicious or negative by itself.

RP-1 prioritizes **privacy-preserving revocation** while retaining strong security guarantees.

---

## **9. Security Considerations**

* Status list MUST be integrity-protected (served over HTTPS; optionally content-hashed if mirrored).
* Issuers MUST ensure availability and freshness of published status lists.
* Verifiers SHOULD re-fetch status lists when `ttl_s` expires.
* Revocation MUST NOT substitute for signature verification.
* Key compromise MUST trigger:

  * KP-1-driven key rotation, and
  * appropriate RP-1 revocation entries.

---

## **10. Compatibility & Extensibility**

Future revocation profiles (e.g., **RP-2**, **RP-3**) MAY add:

* Stapled revocation
* Merkleized / compressed status lists
* Authenticated status-list endpoints
* Signed delta updates

Breaking changes MUST bump both the `version` field in the JSON and the document version.

---

## **11. Summary**

RP-1 establishes the **minimal interoperable revocation mechanism** for **PH-1 EVT** attestations using a shared status list.

It is intentionally:

* Simple
* Privacy-preserving
* Friendly to high-performance, streaming verification

Together with **PH-1-EVT** and **KP-1**, RP-1 forms one leg of the **operational trio** for the Phase 1 Employment Verification Token stack.
