# EVT Evidence & Claims API

API Version: v1
Spec Version: EVT-API v0.2.0

## Conventions

### Auth

All endpoints require an **access token** (minutes–hours lifetime).

```
Authorization: Bearer <access_token>
```
**Token identity**
- The token MUST include a stable subject identifier (`sub`) representing the authenticated caller.
- The server treats `sub` as the caller identity for authorization checks and audit logging.

**Authorization rules**
- On `evidence:init`, the server sets `upload_sessions.uploaded_by = sub`.
- On `evidence:complete`, the server MUST verify the `uploadSessionId` belongs to the same caller (`upload_sessions.uploaded_by == sub`) and is not expired.
- All `/v1/cases/...` reads/writes MUST be authorized based on the caller identity and server policy (e.g., membership in the case, reviewer role, or service account).

**Scopes (optional in V1)**
If tokens include scopes, the API SHOULD enforce them:
- `cases:write` for creating cases/checks
- `evidence:write` for init/complete uploads
- `evidence:read` for listing evidence
- `tokens:issue` / `tokens:revoke` for token actions

### IDs

* `caseId`: external case identifier (string, e.g. `EVT-10324`)
* `checkId`: machine id for the check (string, e.g. `employment.company_and_dates`)
* `uploadSessionId`, `uploadId`: server-generated IDs (UUID)

---

## Evidence Integrity Representation

### 1. Scope and Purpose

This section defines the **canonical representation**, **validation rules**, and **boundary constraints** for evidence integrity hashes used within the Evidence Upload and Review subsystem.

The intent of this section is to:

* Ensure deterministic integrity verification of uploaded evidence
* Enable reliable debugging, auditability, and cross-system comparison
* Preserve strict separation between **evidence material** and **issued human-claims tokens**

This section applies to all fields named `sha256` within the Evidence API surface and storage model.

---

### 2. Canonical Digest Definition

* Evidence integrity is represented by a **SHA-256 digest** computed over the raw bytes of the uploaded evidence object.
* The **canonical value** of an evidence hash is the **32-byte binary output** of the SHA-256 function.
* All textual representations defined in this specification MUST decode to exactly these 32 bytes.

Encodings are representations only and MUST NOT alter the underlying digest semantics.

---

### 3. Canonical API Representation (HEX)

#### 3.1 Required Encoding

* Within all Evidence APIs, databases, logs, and audit records, the `sha256` field MUST be represented as **lowercase hexadecimal** encoding of the canonical digest bytes.

#### 3.2 Format Requirements

* Length MUST be exactly **64 characters**
* Characters MUST match the regular expression:

```
^[0-9a-f]{64}$
```

* Uppercase hex characters MUST NOT be accepted or emitted.

#### 3.3 Definition

Let `D` be the canonical 32-byte SHA-256 digest.

```
sha256 = HEX_LOWER_ENCODE(D)
```

This representation is referred to as **`sha256Hex`** for descriptive purposes.

---

### 4. JOSE Boundary Representation (Base64URL)

#### 4.1 Applicability

When, and only when, a SHA-256 digest must be represented **inside a JOSE object** (e.g., JWS payloads, protected headers, or related cryptographic structures), it MUST be encoded using **base64url without padding**, as required by JOSE specifications.

This representation is referred to as **`sha256B64u`**.

#### 4.2 Definition

```
sha256B64u = BASE64URL_NOPAD_ENCODE(D)
```

* Padding characters (`=`) MUST NOT be included.
* For a 32-byte digest, the resulting string length MUST be **43 characters**.

#### 4.3 Boundary Rule

Base64url encoding is **confined to the JOSE boundary**.
It MUST NOT be used as the canonical representation in Evidence APIs, storage, or audit logs.

---

### 5. Conversion Rules (Normative)

#### 5.1 HEX → Base64URL (No Padding)

Input: `sha256` (lowercase hex)

1. Decode hex string into bytes → `D`
2. Assert `len(D) == 32`
3. Encode `D` using base64url without padding

#### 5.2 Base64URL (No Padding) → HEX

Input: `sha256B64u`

1. Decode base64url-no-pad string into bytes → `D`
2. Assert `len(D) == 32`
3. Encode `D` using lowercase hexadecimal

Conversions MUST be deterministic and lossless.

---

### 6. Validation and Enforcement

Servers MUST enforce the following when receiving evidence integrity data:

* Reject values that do not conform to the required hex format
* Reject values that fail decoding
* Reject values whose decoded byte length is not exactly 32 bytes
* Emit evidence hashes only in the canonical lowercase hex representation

---

### 7. Evidence vs Claims Boundary (Normative Constraint)

**Evidence hashes MUST NOT be embedded directly in issued human-claims tokens.**

Specifically:

* Evidence integrity values (including `sha256`) are confined to:

  * Evidence upload lifecycle
  * Evidence validation and scanning
  * Internal audit and compliance records
* Issued human-claims tokens (e.g., employment verification tokens) MUST contain only:

  * Derived, minimized claims explicitly permitted by policy
  * Cryptographic metadata required for verification, revocation, and trust chaining

Evidence material—including raw files, file metadata, or evidence hashes—MUST remain **out of scope** for issued claim tokens.

This separation enforces:

* Purpose limitation
* Data minimization
* Privacy boundaries
* Long-term token portability independent of evidence retention policies

---

### 8. Rationale (Non-Normative)

* Hexadecimal encoding is selected for Evidence APIs due to superior human readability, tooling compatibility, and audit ergonomics.
* Base64url encoding is restricted to JOSE objects where mandated by specification.
* Encodings are treated as **boundary-specific representations**, not system-wide requirements.

**Note:** All fields named `sha256` in the following endpoints conform to
**Evidence Integrity Representation** (lowercase hex, 64 characters).

---

### Policy constants (server-enforced)

* `MAX_FILES_PER_CHECK = 3`
* `MAX_FILE_BYTES = 5_000_000`
* `PRESIGN_TTL_SECONDS = 600` (10 min)
* `EVIDENCE_RETENTION_DAYS = 30` (or lower)

---

## 1) Create / Get Case

### `POST /v1/cases`

Creates a case for a subject (subject identity can be handled separately; V1 keeps this minimal).

**Request**

```json
{
  "caseId": "EVT-10324",
  "purpose": "employment_verification",
  "evidencePolicyVersion": "EP-1"
}
```

**Response**

```json
{
  "caseId": "EVT-10324",
  "status": "OPEN",
  "createdAt": "2025-12-16T21:10:00Z"
}
```

---

## 2) Create Check (optional if you predefine checks)

### `POST /v1/cases/{caseId}/checks`

**Request**

```json
{
  "checkId": "employment.company_and_dates",
  "allowedDerivedFields": ["company", "startMonth", "endMonth"],
  "maxFiles": 3,
  "maxFileBytes": 5000000
}
```

**Response**

```json
{
  "caseId": "EVT-10324",
  "checkId": "employment.company_and_dates",
  "status": "PENDING"
}
```

---

## 3) Init Evidence Upload (presign)

### `POST /v1/cases/{caseId}/checks/{checkId}/evidence:init`

Creates an upload session and returns presigned PUT URLs for each file.
Integrity hashes are not supplied during `evidence:init`.  
Hashes are computed client-side after upload and submitted during
`evidence:complete` per **Evidence Integrity Representation**.

**Request**

```json
{
  "files": [
    { "name": "paystub.pdf", "mimeType": "application/pdf", "size": 384112 },
    { "name": "offer_letter.jpg", "mimeType": "image/jpeg", "size": 812331 }
  ]
}
```

**Response**

```json
{
  "uploadSessionId": "b5b6c2a8-6c5c-4b7a-a1c3-9fe4d6af5c2a",
  "expiresAt": "2025-12-16T21:20:00Z",
  "constraints": {
    "maxFiles": 3,
    "maxFileBytes": 5000000
  },
  "uploads": [
    {
      "uploadId": "3f2f7d2c-7e58-42b2-8e76-05b7dc0d4d7e",
      "storageKey": "cases/EVT-10324/checks/employment.company_and_dates/evidence/3f2f7d2c-7e58-42b2-8e76-05b7dc0d4d7e/paystub.pdf",
      "putUrl": "https://<space>.<region>.digitaloceanspaces.com/<bucket>/...presigned...",
      "requiredHeaders": {
        "Content-Type": "application/pdf"
      }
    },
    {
      "uploadId": "81c3d5f1-4b86-4c2a-bf20-7b3bcbf8c1c2",
      "storageKey": "cases/EVT-10324/checks/employment.company_and_dates/evidence/81c3d5f1-4b86-4c2a-bf20-7b3bcbf8c1c2/offer_letter.jpg",
      "putUrl": "https://...presigned...",
      "requiredHeaders": {
        "Content-Type": "image/jpeg"
      }
    }
  ]
}
```
| Field              | Requirement                          | Enforcement           |
| ------------------ | ------------------------------------ | --------------------- |
| `files`            | MUST be an array                     | Reject otherwise      |
| `files.length`     | MUST be `≤ MAX_FILES_PER_CHECK`      | Reject                |
| `files[].name`     | MUST be non-empty string             | Reject                |
| `files[].mimeType` | MUST be allow-listed                 | Reject                |
| `files[].size`     | MUST be `> 0` and `≤ MAX_FILE_BYTES` | Reject                |
| `storageKey`       | MUST NOT be client-supplied          | Server-generated only |
| `sha256`           | MUST NOT appear                      | Reject if present     |


> **Note:** Evidence integrity hashes are intentionally excluded from
> `evidence:init`. Hashes are submitted only during `evidence:complete`
> per **Evidence Integrity Representation**.


**Server enforcement**

* Reject if `files.length > 3`
* Reject any `size > 5MB`
* Reject disallowed `mimeType`
* Generate `storageKey` (never accept from client)
* Record `expected_size`, `expected_mimeType`
* Presign with TTL (e.g., 10 minutes)

---

## 4) Complete Evidence Upload (finalize + scan queue)

### `POST /v1/cases/{caseId}/checks/{checkId}/evidence:complete`

**Request**

```json
{
  "uploadSessionId": "b5b6c2a8-6c5c-4b7a-a1c3-9fe4d6af5c2a",
  "uploads": [
    {
      "uploadId": "3f2f7d2c-7e58-42b2-8e76-05b7dc0d4d7e",
      "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
      "size": 384112,
      "mimeType": "application/pdf"
    }
  ]
}
```
| Field                | Requirement                                           | Enforcement |
| -------------------- | ----------------------------------------------------- | ----------- |
| `uploadSessionId`    | MUST exist and not be expired                         | Reject      |
| `uploadSessionId`    | MUST belong to caller                                 | Reject      |
| `uploads`            | MUST be non-empty array                               | Reject      |
| `uploads[].uploadId` | MUST belong to session + case + check                 | Reject      |
| `uploads[].size`     | MUST equal observed object size                       | Reject      |
| `uploads[].mimeType` | MUST equal expected mime type                         | Reject      |
| `uploads[].sha256`   | MUST conform to **Evidence Integrity Representation** | Reject      |
| `uploads[].sha256`   | MUST decode to 32 bytes                               | Reject      |
| `uploads[].sha256`   | MUST match object bytes                               | Reject      |

#### `sha256`

- Lowercase hexadecimal SHA-256 digest of the uploaded object
- MUST conform to **Evidence Integrity Representation**
- MUST be computed over the exact bytes uploaded via the presigned PUT


**Response**

```json
{
  "caseId": "EVT-10324",
  "checkId": "employment.company_and_dates",
  "evidence": [
    {
      "uploadId": "3f2f7d2c-7e58-42b2-8e76-05b7dc0d4d7e",
      "status": "RECEIVED",
      "scanStatus": "PENDING",
      "evidenceExpiresAt": "2026-01-15T21:10:00Z"
    }
  ]
}
```

**Server enforcement**

* Verify session not expired and belongs to caller
* Verify `uploadId` belongs to this session+case+check
* (Recommended) `HEAD` object in Spaces; validate size + content-type
* Set `scan_status = PENDING`, enqueue scan job
* Set `evidence_expires_at = now + retention`

---

## 5) List Evidence for a Check

### `GET /v1/cases/{caseId}/checks/{checkId}/evidence`

**Response**

```json
{
  "caseId": "EVT-10324",
  "checkId": "employment.company_and_dates",
  "items": [
    {
      "uploadId": "3f2f7d2c-7e58-42b2-8e76-05b7dc0d4d7e",
      "fileName": "paystub.pdf",
      "mimeType": "application/pdf",
      "size": 384112,
      "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
      "status": "AVAILABLE",
      "scanStatus": "CLEAN",
      "createdAt": "2025-12-16T21:11:00Z",
      "evidenceExpiresAt": "2026-01-15T21:10:00Z"
    }
  ]
}
```
| Field                | Requirement                                           | Guarantee      |
| -------------------- | ----------------------------------------------------- | -------------- |
| `items[].sha256`     | MUST be lowercase hex                                 | Always emitted |
| `items[].sha256`     | MUST conform to **Evidence Integrity Representation** | Guaranteed     |
| `items[].sha256`     | MUST correspond to stored digest                      | Guaranteed     |
| `items[].status`     | MUST reflect evidence lifecycle                       | Guaranteed     |
| `items[].scanStatus` | MUST reflect latest scan state                        | Guaranteed     |

> Returned integrity values are canonical representations and MUST NOT
> require client-side normalization.

- `sha256` is emitted in canonical lowercase hex form
- Encoding and validation rules are defined in **Evidence Integrity Representation**


> V1 does **not** return raw download URLs by default. If needed, add `GET /download-url` to return a short-lived presigned GET.

---

## 6) Issue Derived Employment Verification Token

### `POST /v1/cases/{caseId}/checks/{checkId}/tokens:issue`

> **Boundary Constraint:**  
> Evidence hashes, file metadata, or storage identifiers are **never**
> embedded in issued human-claims tokens.
>
> Only derived, policy-allowed claims may appear in the token payload.
> See **Evidence Integrity Representation – Evidence vs Claims Boundary**.


**Request**

```json
{
  "method": "human_document_review",
  "derivedClaims": {
    "company": "Acme Corp",
    "startMonth": "2021-03",
    "endMonth": "2023-08",
    "jobTitle": "Electrician" 
  }
}
```

**Response**

```json
{
  "tokenId": "f7e4f998-3f86-4df0-a6a3-11d2c1c30f33",
  "status": "ISSUED",
  "issuedAt": "2025-12-16T22:00:00Z",
  "expiresAt": "2028-12-16T22:00:00Z",
  "jwsCompact": "<JWS>",
  "jwksKid": "<kid>",
  "revocation": {
    "status": "good",
    "statusListUrl": "https://.../status-lists/rp-1.json",
    "statusListIndex": 18273
  }
}
```
| Constraint           | Requirement                              | Enforcement |
| -------------------- | ---------------------------------------- | ----------- |
| Evidence hashes      | MUST NOT be present                      | Reject      |
| File metadata        | MUST NOT be present                      | Reject      |
| Storage identifiers  | MUST NOT be present                      | Reject      |
| Claims               | MUST be subset of `allowedDerivedFields` | Reject      |
| Evidence scan status | MUST be `CLEAN` (unless overridden)      | Reject      |

> **Boundary Constraint:**  
> Evidence integrity values (`sha256`) are strictly confined to the Evidence
> subsystem and MUST NOT be embedded or referenced in issued tokens.

**Server enforcement**

* Require check evidence `scan_status = CLEAN` (or explicit override)
* Ensure derived claims are a subset of allowed fields for `checkId`
* Sign using your signing stack (TS now, Go/YubiHSM2 later)

---

## 7) Revoke Token

### `POST /v1/tokens/{tokenId}:revoke`

**Request**

```json
{
  "reason": "user_request",
  "effectiveAt": "2025-12-16T22:30:00Z"
}
```

**Response**

```json
{
  "tokenId": "f7e4f998-3f86-4df0-a6a3-11d2c1c30f33",
  "revocationStatus": "revoked",
  "revokedAt": "2025-12-16T22:30:00Z"
}
```

| Field         | Requirement            | Enforcement |
| ------------- | ---------------------- | ----------- |
| `tokenId`     | MUST exist             | Reject      |
| Token status  | MUST be revocable      | Reject      |
| Evidence data | MUST NOT be referenced | Reject      |

> Token revocation operates solely within the claims layer and does not
> depend on evidence integrity data.

---

## 8) Delete Case (CPRA/GDPR-style)

### `POST /v1/cases/{caseId}:delete`

**Request**

```json
{ "mode": "hard_delete" }
```

**Response**

```json
{
  "caseId": "EVT-10324",
  "status": "DELETION_SCHEDULED"
}
```

| Aspect           | Requirement                    | Enforcement |
| ---------------- | ------------------------------ | ----------- |
| Evidence objects | MUST be deleted                | Enforced    |
| Evidence hashes  | MUST be deleted with evidence  | Enforced    |
| Tokens           | MUST be revoked or invalidated | Enforced    |
| Audit events     | MAY retain minimal references  | Allowed     |

> Evidence integrity values do not survive beyond evidence retention
> requirements and MUST NOT be retained independently.

**Server behavior**

- Evidence integrity hashes are deleted alongside evidence objects and
  MUST NOT persist beyond retention requirements, except in minimal audit events.

* Immediately revoke tokens (or mark “subject deleted” depending on policy)
* Delete evidence objects from Spaces
* Delete/soft-delete DB rows; retain minimal audit events if needed

---

# Postgres Schema (V1)

Assumes you have a `users` table already; if not, treat `uploaded_by` as an opaque string.

```sql
-- Cases
create table cases (
  case_id text primary key,
  purpose text not null check (purpose = 'employment_verification'),
  evidence_policy_version text not null default 'EP-1',
  status text not null default 'OPEN' check (status in ('OPEN','CLOSED','DELETION_SCHEDULED','DELETED')),
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  deleted_at timestamptz null
);

-- Checks within a case
create table verification_checks (
  case_id text not null references cases(case_id) on delete cascade,
  check_id text not null,
  status text not null default 'PENDING' check (status in ('PENDING','IN_REVIEW','PASSED','FAILED','DELETED')),
  allowed_derived_fields jsonb not null, -- e.g. ["company","startMonth","endMonth","jobTitle"]
  max_files int not null default 3 check (max_files between 1 and 3),
  max_file_bytes int not null default 5000000 check (max_file_bytes > 0 and max_file_bytes <= 5000000),
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  deleted_at timestamptz null,
  primary key (case_id, check_id)
);

-- Upload session (caps + expiry)
create table upload_sessions (
  upload_session_id uuid primary key,
  case_id text not null references cases(case_id) on delete cascade,
  check_id text not null,
  uploaded_by text not null, -- user id/email/subject; keep minimal
  max_files int not null default 3 check (max_files between 1 and 3),
  max_file_bytes int not null default 5000000 check (max_file_bytes > 0 and max_file_bytes <= 5000000),
  expires_at timestamptz not null,
  created_at timestamptz not null default now(),
  closed_at timestamptz null,
  constraint fk_session_check
    foreign key (case_id, check_id) references verification_checks(case_id, check_id) on delete cascade
);

-- Evidence uploads (one per file)
create table evidence_uploads (
  upload_id uuid primary key,
  upload_session_id uuid not null references upload_sessions(upload_session_id) on delete cascade,
  case_id text not null,
  check_id text not null,
  storage_key text not null unique,
  file_name text not null,
  expected_mime_type text not null,
  expected_size int not null check (expected_size > 0 and expected_size <= 5000000),

  -- Actual observed (from client + optional HEAD validation)
  mime_type text null,
  size int null,
  sha256 text null,

  status text not null default 'INITIATED'
    check (status in ('INITIATED','UPLOADED','RECEIVED','AVAILABLE','REJECTED','DELETED')),
  scan_status text not null default 'PENDING'
    check (scan_status in ('PENDING','CLEAN','INFECTED','ERROR','SKIPPED')),

  evidence_expires_at timestamptz not null, -- set on complete (e.g., now + 30 days)
  created_at timestamptz not null default now(),
  completed_at timestamptz null,
  deleted_at timestamptz null,

  constraint fk_evidence_check
    foreign key (case_id, check_id) references verification_checks(case_id, check_id) on delete cascade
);

create index idx_evidence_case_check on evidence_uploads(case_id, check_id);
create index idx_evidence_expires on evidence_uploads(evidence_expires_at) where deleted_at is null;

-- Derived tokens (EVT)
create table ev_tokens (
  token_id uuid primary key,
  case_id text not null references cases(case_id) on delete cascade,
  check_id text not null,
  issued_by text not null, -- reviewer/service identity
  method text not null check (method in ('human_document_review')),
  derived_claims jsonb not null, -- minimized claims only
  jws_compact text not null,
  jwks_kid text not null,

  status text not null default 'ISSUED' check (status in ('ISSUED','REVOKED','EXPIRED')),
  issued_at timestamptz not null default now(),
  expires_at timestamptz null,

  -- Revocation/status-list hooks (RP-1 compatible)
  status_list_url text null,
  status_list_index int null,
  revoked_at timestamptz null,
  revoke_reason text null,

  constraint fk_token_check
    foreign key (case_id, check_id) references verification_checks(case_id, check_id) on delete cascade
);

create index idx_tokens_case_check on ev_tokens(case_id, check_id);

-- Minimal audit events (security + compliance proof)
create table audit_events (
  event_id uuid primary key,
  case_id text null,
  check_id text null,
  actor text null,
  event_type text not null,
  event_data jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now()
);

create index idx_audit_case on audit_events(case_id, created_at);

-- Optional: enforce “<=3 evidence files per check” at DB level
-- (you can also enforce purely in API, but this makes it harder to bypass)
create or replace function enforce_max_files_per_check()
returns trigger language plpgsql as $$
declare
  max_files int;
  current_count int;
begin
  select vc.max_files into max_files
  from verification_checks vc
  where vc.case_id = new.case_id and vc.check_id = new.check_id;

  select count(*) into current_count
  from evidence_uploads eu
  where eu.case_id = new.case_id
    and eu.check_id = new.check_id
    and eu.deleted_at is null;

  if current_count >= max_files then
    raise exception 'max files exceeded for case %, check %', new.case_id, new.check_id;
  end if;

  return new;
end $$;

drop trigger if exists trg_enforce_max_files on evidence_uploads;
create trigger trg_enforce_max_files
before insert on evidence_uploads
for each row execute function enforce_max_files_per_check();
```

---

# Notes for V1 correctness (matches your compliance goals)

* **Purpose limitation** is enforced by:

  * `cases.purpose`
  * `verification_checks.allowed_derived_fields`
  * storage key path includes `{caseId}/{checkId}` and server generates it
* **Storage limitation** is enforceable via:

  * `evidence_expires_at` + scheduled deletion job
* **No PII extraction** is enforceable by policy:

  * no OCR pipeline in V1 + token derived_claims must be allowlisted
* **Right to delete** is implementable:

  * `cases:delete` + delete objects by `storage_key` + revoke tokens
* **Revocation** supported:

  * `ev_tokens.status` + status-list pointers

---