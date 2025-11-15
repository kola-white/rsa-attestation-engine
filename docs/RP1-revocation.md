# **RP-1: Revocation Profile — Status-List Method (v1.0.0)**

**Scope:** Human *employment-role* claims (AP-1.employment-role)
**Status:** Final (MVP)
**Audience:** Issuers, Verifiers, Security Engineers
**Depends on:** AP-1 (Attestation Profile), KP-1 (Key Profile)
**Version:** 1.0.0  

---

## **1. Purpose**

RP-1 defines the **canonical revocation mechanism** for attestations issued under **AP-1.employment-role** using the **status-list** method.
This profile standardizes:

* The **status-list document format** (`trust/statuslist.json`)
* The **revocation event** object (issuer audit logs)
* The **verification algorithm** used to determine if a claim is revoked
* The **allowed revocation statuses**
* Error semantics and canonical verifier return codes

This profile ensures revocation is predictable, minimal, privacy-preserving, and compatible with streaming verification environments.

---

## **2. Applicability**

An attestation conforms to RP-1 when its `revocation` stanza contains:

```jsonc
"revocation": {
  "method": "status-list",
  "pointer": "trust/statuslist.json",
  "serial": "att-SN-000389"
}
```

* `method` MUST be `"status-list"`
* `pointer` MUST reference a valid RP-1 status list
* `serial` MUST uniquely identify the attestation within that list

The **golden revoked attestation** example adheres to this pattern.

---

## **3. Status-List Document**

The canonical status list is a JSON document located at:

```
trust/statuslist.json
```

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
| `entries`      | MAY be empty                       | Each entry maps a serial → status    |

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

Any other value MUST cause the verifier to reject with
`code: "REVOCATION_STATUS_INVALID"`.

---

## **4. Revocation Events (Issuer Audit)**

Issuers SHOULD record revocation events using the following structure:

```jsonc
{
  "id": "rev-ULID-01HDEF...",
  "attestation_id": "att-ULID-01HXYZ...",
  "issuer": { "id": "did:org:acme-corp" },
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

**Revocation events are NOT required for verification.**
They serve operational, compliance, and audit purposes.

---

## **5. Verification Algorithm (Normative)**

The following steps MUST be applied in order.

### **5.1 Precondition**

Attestation signature MUST already be verified under KP-1.
Revocation checks do NOT bypass signature requirements.

---

### **5.2 Algorithm**

| Step                   | Logic                                                   |
| ---------------------- | ------------------------------------------------------- |
| **1. Check method**    | Reject if method ≠ `"status-list"`                      |
| **2. Resolve pointer** | Fetch `pointer` (e.g., `trust/statuslist.json`)         |
| **3. Validate list**   | Reject if: parse failure, issuer mismatch, or malformed |
| **4. Find entry**      | `entry = entries.find(e => e.serial === serial)`        |
| **5. Evaluate status** | Apply below rules                                       |

---

### **5.3 Status Evaluation Rules**

#### **Case A — Entry not found**

```
outcome: ACCEPT
reason: unknown_not_listed
```

Result: **Not revoked**

This avoids punitive interpretation of missing data.

---

#### **Case B — Entry has status = "revoked"**

```
outcome: REJECT
reason: revoked
return code: REVOKED
```

Result: **Revoked**

---

#### **Case C — Entry has invalid status value**

```
outcome: REJECT
reason: invalid_status_value
return code: REVOCATION_STATUS_INVALID
```

Result: **Verifier MUST reject**

This prevents issuers from unintentionally introducing unrecognized states.

---

#### **Case D — Entry has status = "good"**

```
outcome: ACCEPT
reason: good
```

Result: **Not revoked**

---

### **5.4 Additional Verifier Error Code**

Verifiers SHOULD emit:

```
REVOCATION_SOURCE_UNAVAILABLE
```

When:

* status list cannot be fetched
* invalid JSON
* issuer mismatch
* structural validation fails

This distinguishes operational errors from actual revocation.

---

## **6. Issuer Responsibilities**

Issuers implementing RP-1 MUST:

### **6.1 Assign a unique `serial`**

* MUST be unique per attestation
* MUST be opaque (ULID, UUID, random)
* MUST not encode PII

### **6.2 Maintain the canonical status list**

* MUST be authoritative for all RP-1 attestations
* MUST update entries atomically and predictably
* MUST avoid PII in list entries

### **6.3 Emit revocation events (SHOULD)**

* Append-only
* Signed under KP-1
* Internal use only

### **6.4 Keep status vocabulary strict**

Only `"good"` and `"revoked"` are permitted.

### **6.5 Respect AP-1 and KP-1 invariants**

* Revocation does not replace signature failure
* Signature failure does not imply revocation

---

## **7. Verifier Responsibilities**

Verifiers claiming RP-1 conformance MUST:

* Implement all logic in §5
* Preserve audit logs (`stage: "revocation"`)
* Honor `ttl_s` when caching
* Treat “not listed” as “not revoked”
* Enforce issuer match
* Enforce vocabulary constraints
* Map results to the canonical result codes:

### **RP-1 Result Codes**

| Code                            | Meaning                              |
| ------------------------------- | ------------------------------------ |
| `REVOKED`                       | Attestation explicitly revoked       |
| `REVOCATION_STATUS_INVALID`     | Status value invalid / unsupported   |
| `REVOCATION_SOURCE_UNAVAILABLE` | Status list unreadable / unreachable |
| *none*                          | Attestation considered not revoked   |

---

## **8. Privacy Considerations**

Because RP-1 governs human-centric employment claims:

* Status lists MUST NOT contain PII
* `reason_text` MUST NOT appear publicly
* Revocation events SHOULD be stored privately
* Serials MUST NOT be reversible to employee identity
* “Not listed” MUST NOT be treated as suspicious

RP-1 focuses on **minimizing privacy exposure while preserving security guarantees**.

---

## **9. Security Considerations**

* Status list MUST be integrity-protected (served over HTTPS, content hashed if mirrored)
* Issuers MUST ensure availability and freshness
* Verifiers SHOULD revalidate lists when `ttl_s` expires
* Revocation MUST NOT substitute for signature verification
* Key compromise MUST trigger revocation events and KP-1 rotation

---

## **10. Compatibility & Extensibility**

Future profiles (RP-2, RP-3) MAY add:

* “stapled revocation”
* inclusion proofs (Merkleized lists)
* authenticated status-list endpoints
* issuer-signed delta updates

Breaking changes MUST increment both `version` field and document version.

---

## **11. Summary**

RP-1 establishes the **minimal interoperable revocation mechanism** for AP-1 employment-role attestations using a shared status list. It is intentionally simple, privacy-preserving, and compatible with high-performance verification systems.

This profile, together with AP-1 and KP-1, forms one pillar of the **Operational Trio** referenced in DP-1.

---
