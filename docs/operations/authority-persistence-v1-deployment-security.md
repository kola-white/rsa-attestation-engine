# Authority Persistence V1: Deployment Security Contract

Status: Local implementation validated; production deployment not authorized.

## 1. Security boundary

Authority evidence is append-only. The authoritative grant state is reconstructed from authorization records, adjudications, and lifecycle journal events.

Recording positions establish transactional evidence ordering. The recording boundary must not be directly writable by the application runtime role.

The application must fail closed when authoritative evidence is incomplete, inconsistent, or cannot be reconstructed.

## 2. Locally validated implementation

The following controls have been exercised against disposable PostgreSQL 16 schemas:

- Authority migrations 000001 through 000013 apply successfully.
- Recording allocation and evidence insertion participate in one transaction.
- Failed transactions do not advance the committed recording boundary.
- Deferred recording constraints enforce allocation-to-evidence correspondence.
- Existing authority evidence rejects UPDATE and DELETE operations through immutability triggers.
- Authoritative snapshots use a consistent repeatable-read transaction.
- Snapshot completeness rejects missing, invalid, or future recording positions.
- The recording allocator runs as SECURITY DEFINER with a fixed trusted-schema search_path and pg_temp last.
- PUBLIC execution permission is revoked from the allocator.
- A restricted role requires an explicit EXECUTE grant to call the allocator.
- A restricted role without direct boundary UPDATE permission can allocate through the function.
- Caller-controlled search_path redirection does not redirect the allocator to a decoy boundary table.
- A restricted non-owner role can insert and commit a journal event while deferred recording-validation triggers remain enforced.
- PostgreSQL 16 generates the journal's GENERATED ALWAYS AS IDENTITY value without granting the restricted role sequence USAGE.
- The restricted-role journal integration test passes across repeated executions with probe-role cleanup.
- The full Go test suite passed with the local authority test database before the latest restricted-role test was added; a final regression run remains required.

These tests do not constitute production deployment approval.

## 3. Required database role separation

Production must use distinct database responsibilities:

1. A trusted migration role owns the authority schema objects and applies migrations.
2. A non-owner application runtime role accesses only the objects and operations required by application code.
3. The runtime role must not be a superuser, own authority evidence tables or allocator functions, possess schema CREATE permission, or have permission to disable evidence triggers.
4. Migration execution must not use application runtime credentials.

The authority schema must not allow untrusted roles to create objects that could shadow objects referenced by privileged functions.

## 4. Authority runtime permissions

Subject to validation against the actual deployed schema, the authority runtime role requires:

- USAGE on the authority schema.
- SELECT and INSERT on authority_authorizations, authority_adjudications, and authority_journal_events.
- SELECT on authority_recording_boundary.
- EXECUTE on next_authority_recording_position(), granted explicitly to the runtime role.
- UPDATE(authorization_id) on authority_authorizations for SELECT FOR UPDATE locking. The immutable-evidence trigger must remain active.
- No direct sequence USAGE grant for authority_journal_events.journal_sequence in the PostgreSQL 16 insertion path validated locally. Revalidate if the deployed schema or PostgreSQL behavior differs.

The runtime role must not receive direct UPDATE or DELETE permission on authority_recording_boundary or broad UPDATE/DELETE privileges on authority evidence tables.

Do not grant ALL PRIVILEGES on the authority schema or its tables.

## 5. Existing application dependencies

The Go application also accesses legacy authentication and EVT tables, including domain_users, auth_sessions, refresh_tokens, evt_requests, evt_request_events, evt_employer_config, evt_token_consumptions, and recruiter_trusted_issuers.

Their DDL, ownership, sequence dependencies, and provisioning mechanism were not established from the repository inspection.

Do not replace the application database credentials or introduce an application-wide GRANT migration until those dependencies are inventoried and tested with a restricted runtime role.

## 6. Deployment prerequisites

Before any production migration or credential change:

1. Identify the current PostgreSQL runtime identity, schema ownership, and existing grants without exposing credentials.
2. Identify the production migration mechanism and trusted migration identity.
3. Confirm the authority schema destination and existing data compatibility.
4. Inventory legacy application tables, sequences, functions, and required privileges.
5. Create and test a dedicated non-owner runtime role in a disposable or staging environment.
6. Verify authority write, snapshot read, authentication, refresh, logout, and EVT workflows under that role.
7. Verify restricted-role attempts to modify or delete authority evidence are rejected.
8. Confirm backup, rollback, and operational recovery procedures.
9. Obtain explicit production deployment authorization.

## 7. Legacy evidence

Pre-existing authority evidence without recording positions must not be assigned fabricated positions or silently accepted as complete.

Historical reconstruction remains an internal V1 capability. No public historical authority API is authorized by this implementation.

## 8. Deployment status

No production database migrations, role changes, credential rotations, or infrastructure changes have been performed as part of this validation.

Production readiness remains blocked on verified runtime-role provisioning and legacy application permission coverage.
