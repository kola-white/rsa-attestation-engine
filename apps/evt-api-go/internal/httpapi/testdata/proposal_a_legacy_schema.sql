-- Operator-supplied PostgreSQL 16 metadata, reproduced in an isolated schema.
-- In production these three tables and evt_request_status are in public.
CREATE TABLE domain_users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kratos_identity_id TEXT NOT NULL UNIQUE,
    email TEXT,
    name TEXT,
    role TEXT NOT NULL DEFAULT 'hr_reviewer',
    status TEXT NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE evt_employer_config (
    employer_id TEXT PRIMARY KEY,
    dispatch_mode TEXT NOT NULL DEFAULT 'AUTO',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- Only the name/default of this enum was exported. These labels are runtime
-- test values, not a claim to reproduce the complete production enum.
CREATE TYPE evt_request_status AS ENUM ('DRAFT','SUBMITTED','ATTESTATION_PENDING','ATTESTED','REJECTED');
-- The response enum and remaining workflow columns below are test scaffolding:
-- their complete authoritative DDL was not supplied. No migration alters them.
CREATE TYPE employer_response_type AS ENUM ('FULL_MATCH','PARTIAL_MATCH','REJECTED_NO_RECORD','REJECTED_POLICY');
CREATE TABLE evt_requests (
    request_id UUID PRIMARY KEY,
    candidate_personid UUID NOT NULL,
    employer_id TEXT NOT NULL REFERENCES evt_employer_config(employer_id),
    hr_assigned_personid UUID,
    status evt_request_status NOT NULL DEFAULT 'DRAFT',
    claim_snapshot JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now(),
    version INT DEFAULT 1,
    employer_response_type employer_response_type,
    employer_response JSONB,
    attestation_jws TEXT,
    attested_at TIMESTAMPTZ
);
-- Index names alone and two trigger names were supplied; their definitions
-- were not. They are not fabricated here or required by the new migration.
