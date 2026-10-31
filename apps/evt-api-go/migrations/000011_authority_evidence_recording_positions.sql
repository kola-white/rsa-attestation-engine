-- Preserve legacy evidence without inventing historical recording order.
-- New evidence will require a positive database-validated position once
-- the recording protocol is enforced by a subsequent migration.

ALTER TABLE authority_authorizations
    ADD COLUMN recording_position BIGINT
    CHECK (recording_position > 0);

ALTER TABLE authority_adjudications
    ADD COLUMN recording_position BIGINT
    CHECK (recording_position > 0);

ALTER TABLE authority_journal_events
    ADD COLUMN recording_position BIGINT
    CHECK (recording_position > 0);

CREATE UNIQUE INDEX authority_authorizations_recording_position_unique
    ON authority_authorizations (recording_position)
    WHERE recording_position IS NOT NULL;

CREATE UNIQUE INDEX authority_adjudications_recording_position_unique
    ON authority_adjudications (recording_position)
    WHERE recording_position IS NOT NULL;

CREATE UNIQUE INDEX authority_journal_events_recording_position_unique
    ON authority_journal_events (recording_position)
    WHERE recording_position IS NOT NULL;
