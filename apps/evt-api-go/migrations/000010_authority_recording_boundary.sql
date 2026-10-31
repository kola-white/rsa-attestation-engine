-- Transactional serialization point for authority evidence recording.
-- Authority writers must update this row within the same transaction
-- as their evidence insert. The row lock remains held until commit
-- or rollback, preventing later recording positions from committing first.

CREATE TABLE authority_recording_boundary (
    singleton BOOLEAN PRIMARY KEY DEFAULT TRUE
        CHECK (singleton),
    committed_position BIGINT NOT NULL DEFAULT 0
        CHECK (committed_position >= 0)
);

INSERT INTO authority_recording_boundary (
    singleton,
    committed_position
) VALUES (TRUE, 0);

CREATE OR REPLACE FUNCTION next_authority_recording_position()
RETURNS BIGINT
LANGUAGE plpgsql
AS $$
DECLARE
    next_position BIGINT;
BEGIN
    UPDATE authority_recording_boundary
    SET committed_position = committed_position + 1
    WHERE singleton = TRUE
    RETURNING committed_position INTO next_position;

    IF next_position IS NULL THEN
        RAISE EXCEPTION 'authority recording boundary is missing';
    END IF;

    RETURN next_position;
END;
$$;
