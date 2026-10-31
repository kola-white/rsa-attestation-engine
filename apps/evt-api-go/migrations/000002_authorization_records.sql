CREATE TABLE authority_authorizations (
    authorization_id TEXT PRIMARY KEY,
    issuer_id TEXT NOT NULL,
    authority_subject_id TEXT NOT NULL,
    claim_type TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT authority_authorization_id_nonempty
        CHECK (authorization_id <> ''),
    CONSTRAINT authority_issuer_id_nonempty
        CHECK (issuer_id <> ''),
    CONSTRAINT authority_subject_id_nonempty
        CHECK (authority_subject_id <> ''),
    CONSTRAINT authority_claim_type_nonempty
        CHECK (claim_type <> '')
);

CREATE INDEX authority_authorizations_lookup_idx
    ON authority_authorizations (issuer_id, claim_type);
