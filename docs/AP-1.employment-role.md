# **AP-1: Employment Role Attestation Profile (v1.0.0)**

**Profile ID:** `AP-1/employment.role/v1`
**Schema URI:** `schema/employment.role/v1`
**Status:** Draft (MVP)
**Scope:** Human employment-role claims

AP-1 defines the canonical payload shape and validation semantics for **employment role** attestations in the Attested Identity system. It is used by issuers to encode **human claims about employment roles**, and by verifiers to validate those claims against the trust path, temporal validity, revocation state, and policy.

---

## 1. Scope and Intent

AP-1 covers:

* A **single claim type**: `employment.role`
* A **single schema URI**: `schema/employment.role/v1`
* A **fixed JSON payload shape** (see §2) validated against:

  * A **JSON Schema** at `schema/employment.role/v1` (structural correctness)

The **JSON Schema** identified by `schema_uri = "schema/employment.role/v1"` is
published at:

  **${TRUST_BASE_URL}/schema/employment.role.v1.json**

The publication location is an implementation detail and may change; the
logical identifier `schema/employment.role/v1` remains stable.

  * A verifier implementation (`verifyAttestationJws`) for:

    * Signature & header checks
    * Temporal liveness
    * Revocation via status list
    * Policy assurance
    * Minimal profile-specific semantics (disclosure + hash)

AP-1 is intentionally **simple** for the MVP:

* Only **full disclosure** is allowed (`disclosure.mode = "full"`).
* **Selective / partial disclosure** of fields is **out of scope** for this profile and this MVP.
* It assumes a single revocation method (`status-list`) and a single algorithm family (`RS256` + `SHA-256`).

Other profiles (AP-N) can define different claim types or more complex semantics.

---

## 2. Normative Structural Definition

### 2.1 JSON Schema as structural authority

The **authoritative structural definition** of AP-1 is the JSON Schema at:

> `schema/employment.role/v1`

That schema:

* Defines the top-level object as **closed** (`additionalProperties: false`).

* Requires the following top-level keys:

  ```json
  [
    "id",
    "schema_uri",
    "version",
    "issuer",
    "key",
    "subject",
    "claim",
    "validity",
    "revocation",
    "policy",
    "disclosure",
    "hash",
    "signature"
  ]
  ```

* Constrains field types, enums, and constants (e.g., `schema_uri`, `claim.type`, `key.alg`, `signature.sig_alg`, `hash.payload_alg`, etc.).

**Schema vs semantics**

* JSON Schema enforces **structural correctness only**—field presence, types, and enumerations.
* The verifier (`verifyAttestationJws`) is responsible for **semantic checks**, including:

  * Temporal consistency (how times are interpreted, ordering of `issued_at`, `not_before`, `not_after`)
  * Revocation lookup and interpretation of status list entries
  * Policy assurance evaluation (mapping `TAL-*` to allowed values)
  * Hash semantics (what content is hashed and when it’s checked)

These semantics are defined in AP-1 and related specs (TP-1, RP-1, KP-1), not in the JSON Schema.

---

### 2.2 Logical Type (TypeScript view)

For implementers, the schema corresponds to this shape:

```ts
type EmploymentRoleAttestationV1 = {
  id: string;
  schema_uri: "schema/employment.role/v1";
  version: string; // semver, e.g., "1.0.0"

  issuer: {
    id: string;
    name?: string;
    ca_chain?: string[];
  };

  key: {
    kid: string;
    alg: "RS256";
  };

  subject: {
    binding?: {
      type: "pubkey" | "identifier" | "both";
      pubkey_thumbprint?: string;
      identifier?: string;
    };
  };

  claim: {
    type: "employment.role";
    context?: Record<string, unknown>;
    value: {
      title: string;
      skill?: string;
      level?: string;
    };
  };

  validity: {
    issued_at: string;   // ISO-8601 (RFC 3339)
    not_before: string;  // ISO-8601
    not_after: string;   // ISO-8601
  };

  revocation: {
    method: "status-list";
    pointer: string;
    serial: string;
  };

  policy: {
    policy_uri: string;
    assurance: "TAL-1" | "TAL-2" | "TAL-3" | "TAL-4";
  };

  disclosure: {
    mode: "full";
    disclosed_fields?: string[];
  };

  hash: {
    payload_alg: "SHA-256";
    payload_hash: string;
  };

  signature: {
    mode: "attached";
    sig_alg: "RS256";
    sig: string;
  };
};
```

---

## 3. Field-Level Requirements

This section describes how AP-1 interprets the structurally-valid payload.

### 3.1 Identification

* `id`

  * Type: `string`
  * SHOULD be stable and unique per attestation (ULID or UUID recommended).
* `schema_uri`

  * MUST be `schema/employment.role/v1`.
  * Used to route to AP-1 logic and JSON Schema.
* `version`

  * MUST be a semver-like string (`X.Y.Z`).
  * This document describes the **v1 line**; initial fixtures use `1.0.0`.

### 3.2 Issuer and Key

* `issuer.id`

  * Logical identifier for the issuing organization (e.g. `did:org:acme-corp`).
* `issuer.name`

  * Optional human-friendly label.
* `issuer.ca_chain`

  * Optional array describing CA lineage (root → intermediate → …).
* `key.kid`

  * MUST match the JWS header `kid` used to sign the attestation.
* `key.alg`

  * MUST be `"RS256"`.

### 3.3 Subject Binding

`subject` describes how the **human subject** of the attestation is bound.

* `subject.binding.type`

  * Enum: `"pubkey" | "identifier" | "both"`.
  * `"pubkey"`: Attestation is bound to a public key (thumbprint).
  * `"identifier"`: Attestation is bound to an identifier (e.g., `mailto:`, `did:`).
  * `"both"`: Both are present and **jointly** identify the subject.
* `subject.binding.pubkey_thumbprint`

  * REQUIRED if `type` is `"pubkey"` or `"both"`.
* `subject.binding.identifier`

  * REQUIRED if `type` is `"identifier"` or `"both"`.

(These conditional requirements are enforced semantically; the schema allows them as optional properties.)

### 3.4 Claim

* `claim.type`

  * MUST equal `"employment.role"`.
* `claim.context`

  * Optional object for issuer-defined context (e.g., `org_unit`, `project`, `location`).
  * AP-1 does **not** fix the key set; issuers may add fields as needed.
* `claim.value`

  * `title` (REQUIRED): human-readable job title.
  * `skill` (OPTIONAL): primary skill (e.g., “Backend”, “Data Engineering”).
  * `level` (OPTIONAL): ladder level (“L5”, “Senior”, etc.).
  * AP-1 does not define a canonical ontology for `skill` or `level`; this can be governed by policy.

### 3.5 Validity (Temporal Window)

* `validity.issued_at`

  * Time the attestation was issued.
* `validity.not_before`

  * Earliest instant at which the attestation is considered valid.
* `validity.not_after`

  * Instant after which the attestation is considered expired.

All three are RFC 3339 timestamps (JSON Schema `format: "date-time"`). Temporal semantics (e.g., `now < not_before`) are handled in the verifier (see §4).

### 3.6 Revocation

* `revocation.method`

  * MUST be `"status-list"`.
* `revocation.pointer`

  * A locator for the status list (e.g., `trust/statuslist.json` or URL).
* `revocation.serial`

  * A serial or index used to look up the attestation in the status list.

RP-1 (Revocation Profile) will define the exact status list format.

### 3.7 Policy

* `policy.policy_uri`

  * Location of the issuer’s internal policy document (e.g., HR verification process).
* `policy.assurance`

  * Enum: `"TAL-1" | "TAL-2" | "TAL-3" | "TAL-4"`.
  * The verifier loads `policy.allowed_assurance` from trust artifacts and rejects any value not present.

### 3.8 Disclosure (Human-Claims Semantics)

AP-1 explicitly models **disclosure** as:

> “What is the subject *showing* from this attestation?”

For AP-1 and the current MVP:

* `disclosure.mode`

  * MUST be `"full"`.
  * Indicates the subject is presenting the **entire** attestation payload for this schema.
  * No partial / selective views are permitted.
* `disclosed_fields`

  * Optional array of strings naming fields that are considered “visible” to the verifier.
  * In AP-1, when `mode = "full"`, this list is **informational only**. The verifier does not enforce it.

**Human-claims perspective / value-add**

From the standpoint of human claims:

* Issuers benefit by being able to say:

  > “This attestation is presented in its entirety; nothing from this schema is being hidden.”
* Verifiers benefit by being able to set policies like:

  > “We only trust AP-1 attestations where `disclosure.mode === 'full'`.”

AP-1 **does not** support selective or partial disclosure. Users cannot choose to show only some fields of an AP-1 employment role claim without leaving the profile. Any future selective-disclosure work would live in a **different profile** or version.

### 3.9 Hash

The `hash` section anchors the payload as seen by the issuer:

* `hash.payload_alg`

  * MUST be `"SHA-256"`.
* `hash.payload_hash`

  * MUST be a non-empty string.
  * Semantically: hash over a canonical representation of the payload (exact source-of-truth definition lives in implementation / TP-1), used for logging/audit and possible cross-format verification.

### 3.10 Signature (Wrapper)

The payload also carries signature metadata:

* `signature.mode`

  * MUST be `"attached"`.
* `signature.sig_alg`

  * MUST be `"RS256"`.
* `signature.sig`

  * Opaque signature material; in practice, the verifier relies on the **JWS wrapper** and local JWKS to validate the signature.

This section is mostly redundant with JWS header/body but is kept for explicitness and easier introspection by non-cryptographic tooling.

---

## 4. Verification Stages and Result Codes

The reference verifier (`verifyAttestationJws`) processes AP-1 attestations in the following stages:

1. **Signature + Header**
2. **Shape / Schema & Profile Checks**
3. **Temporal Liveness**
4. **Revocation**
5. **Policy Assurance**

### 4.1 Signature + Header

* Uses `jwtVerify` and `createLocalJWKSet(jwks)` loaded from trust artifacts.
* Expects:

  * `alg = "RS256"`
  * A recognized `kid` mapped to a JWK in the JWKS.

Failure outcomes:

* `BAD_SIGNATURE`

  * JWS signature verification fails.
* `HEADER_INVALID`

  * Header `alg` ≠ `"RS256"` or `kid` missing/invalid.

### 4.2 Shape / Schema & Profile Checks

At runtime, the verifier does a lightweight semantic check in addition to JSON Schema:

* `schema_uri` must equal `policy.schema_uri`.
* `claim.type` must equal `"employment.role"`.
* Required fields must be present: `id`, `claim.value.title`, `revocation.serial`, `validity.not_before`, `validity.not_after`, `policy.assurance`.
* `disclosure.mode` must be `"full"`.
* `hash.payload_alg` must be `"SHA-256"` and `hash.payload_hash` non-empty.

Failure outcomes:

* `SCHEMA_URI_INVALID`

  * Payload `schema_uri` does not match `policy.schema_uri`.
* `CLAIM_TYPE_INVALID`

  * `claim.type` not equal to `"employment.role"`.
* `SCHEMA_INVALID`

  * Required fields missing or structurally invalid (AP-1 shape violation).
* `DISCLOSURE_MODE_INVALID`

  * `disclosure.mode` is not `"full"`.
* `HASH_INVALID`

  * `hash` block missing, wrong algorithm, or missing/empty `payload_hash`.

If JSON Schema is also run, any schema failure is mapped to `SCHEMA_INVALID`.

### 4.3 Temporal Liveness

The verifier compares `now = isoNow()` to `validity.not_before` and `validity.not_after`:

* If `now < not_before` → `NOT_YET_VALID`
* If `now > not_after` → `EXPIRED`
* Otherwise → liveness passes.

### 4.4 Revocation

Using `revocation.pointer` and `revocation.serial`, the verifier consults the status list:

* If no entry is found:

  * Treated as “unknown/not listed”; revocation check passes but is logged.
* If `entry.status === "revoked"`:

  * Fail with `REVOKED`.
* If `entry.status` is anything other than `"good"` or `"revoked"`:

  * Fail with `REVOCATION_STATUS_INVALID`.

### 4.5 Policy Assurance

The verifier loads `policy.allowed_assurance` from trust artifacts:

* `policy.assurance` must be a value in that list.
* Otherwise → `POLICY_ASSURANCE_INVALID`.

### 4.6 Success

If all stages pass, the result is:

```ts
{ ok: true, code: "VALID" }
```

---

## 5. Golden Fixtures as Normative Examples

AP-1 uses a set of golden fixtures as **normative examples and tests**:

| File                                        | Scenario              | Expected Result Code               |
| ------------------------------------------- | --------------------- | ---------------------------------- |
| `golden-valid.attestation.json`             | Happy path            | `VALID`                            |
| `golden-revoked.attestation.json`           | Revoked in statuslist | `REVOKED`                          |
| `golden-notyetvalid.attestation.json`       | Not yet valid         | `NOT_YET_VALID`                    |
| `golden-expired.attestation.json`           | Expired               | `EXPIRED`                          |
| `golden-invalid-signature.attestation.json` | Bad signature         | `BAD_SIGNATURE` / `HEADER_INVALID` |
| `golden-invalid-liveness.attestation.json`  | Temporal edge-cases   | A liveness-related failure         |
| `golden-invalid-schema.attestation.json`    | Shape/schema invalid  | `SCHEMA_INVALID`                   |

These files should all conform to the **top-level shape** of AP-1, except `golden-invalid-schema`, which is intentionally violating the schema.

---

## 6. Relationship to Other Profiles

* **TP-1 (Trust Path Specification)**
  Defines how issuers, keys, and status lists are anchored, authenticated, and rotated. AP-1 assumes TP-1 for JWKS and status list trust.

* **RP-1 (Revocation Profile)**
  Defines the status list format that `revocation.pointer` and `serial` refer to.

* **KP-1 (Key Profile)**
  Defines key requirements (sizes, algorithms, rotation cadence) for JWKs referenced by `key.kid`.

AP-1 focuses only on the **employment-role claim payload** and its immediate validation semantics. It depends on TP-1, RP-1, and KP-1 for the surrounding trust fabric.

---