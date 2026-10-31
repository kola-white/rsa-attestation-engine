ALTER TABLE authority_authorizations
ADD CONSTRAINT authority_authorizations_scope_unique
UNIQUE (
    authorization_id,
    issuer_id,
    authority_subject_id,
    claim_type
);

ALTER TABLE authority_adjudications
ADD CONSTRAINT authority_adjudications_scope_fk
FOREIGN KEY (
    authorization_id,
    issuer_id,
    authority_subject_id,
    claim_type
)
REFERENCES authority_authorizations (
    authorization_id,
    issuer_id,
    authority_subject_id,
    claim_type
);
