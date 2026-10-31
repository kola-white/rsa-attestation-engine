CREATE TABLE authority_journal_events (
    journal_sequence BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    event_id TEXT NOT NULL UNIQUE,
    authorization_id TEXT NOT NULL
        REFERENCES authority_authorizations (authorization_id),
    event_type TEXT NOT NULL,
    effective_at TIMESTAMPTZ NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL,
    actor_id TEXT NOT NULL,
    evidence_ref TEXT NOT NULL,
    conditions_satisfied BOOLEAN NOT NULL,
    corrects_event_id TEXT,
    correction_reason TEXT,
    corrected_field TEXT,

    CONSTRAINT authority_journal_required_fields
        CHECK (
            event_id <> ''
            AND authorization_id <> ''
            AND event_type <> ''
            AND actor_id <> ''
            AND evidence_ref <> ''
        ),

    CONSTRAINT authority_journal_correction_fields
        CHECK (
            (
                corrects_event_id IS NULL
                AND correction_reason IS NULL
                AND corrected_field IS NULL
            )
            OR
            (
                corrects_event_id IS NOT NULL
                AND corrects_event_id <> ''
                AND correction_reason IS NOT NULL
                AND correction_reason <> ''
                AND corrected_field IS NOT NULL
                AND corrected_field <> ''
            )
        ),

    CONSTRAINT authority_journal_no_self_correction
        CHECK (
            corrects_event_id IS NULL
            OR corrects_event_id <> event_id
        )
);

CREATE INDEX authority_journal_history_idx
    ON authority_journal_events (
        authorization_id,
        journal_sequence
    );
