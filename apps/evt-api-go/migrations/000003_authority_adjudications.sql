CREATE TABLE authority_adjudications (
    decision_id TEXT PRIMARY KEY,
    authorization_id TEXT NOT NULL
        REFERENCES authority_authorizations (authorization_id),
    issuer_id TEXT NOT NULL,
    authority_subject_id TEXT NOT NULL,
    claim_type TEXT NOT NULL,
    outcome TEXT NOT NULL
        CHECK (outcome IN ('APPROVED', 'REJECTED')),
    decision_maker_id TEXT NOT NULL,
    authority_basis TEXT NOT NULL,
    evidence_ref TEXT NOT NULL,
    decided_at TIMESTAMPTZ NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL,

    CONSTRAINT authority_adjudication_identifiers_nonempty
        CHECK (
            decision_id <> ''
            AND authorization_id <> ''
            AND issuer_id <> ''
            AND authority_subject_id <> ''
            AND claim_type <> ''
            AND decision_maker_id <> ''
            AND authority_basis <> ''
            AND evidence_ref <> ''
        ),

    CONSTRAINT authority_adjudication_temporal_order
        CHECK (decided_at <= recorded_at)
);

CREATE INDEX authority_adjudications_history_idx
    ON authority_adjudications (
        authorization_id,
        recorded_at,
        decision_id
    );
