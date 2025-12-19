# V1 API Contract

## Conventions

### Auth

All endpoints require an **access token** (minutes–hours lifetime).

```
Authorization: Bearer <access_token>
```

### IDs

* `caseId`: external case identifier (string, e.g. `EVT-10324`)
* `checkId`: machine id for the check (string, e.g. `employment.company_and_dates`)
* `uploadSessionId`, `uploadId`: server-generated IDs (UUID)

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
      "sha256": "hex-or-base64",
      "size": 384112,
      "mimeType": "application/pdf"
    }
  ]
}
```

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
      "sha256": "hex-or-base64",
      "status": "AVAILABLE",
      "scanStatus": "CLEAN",
      "createdAt": "2025-12-16T21:11:00Z",
      "evidenceExpiresAt": "2026-01-15T21:10:00Z"
    }
  ]
}
```

> V1 does **not** return raw download URLs by default. If needed, add `GET /download-url` to return a short-lived presigned GET.

---

## 6) Issue Derived Employment Verification Token

### `POST /v1/cases/{caseId}/checks/{checkId}/tokens:issue`

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

**Server behavior**

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