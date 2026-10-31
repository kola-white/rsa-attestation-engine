-- Bind each recording allocation to its transaction.
-- Legacy evidence remains nullable; only new inserts are validated.

ALTER TABLE authority_recording_boundary
    ADD COLUMN allocating_xid xid8;

CREATE OR REPLACE FUNCTION next_authority_recording_position()
RETURNS BIGINT
LANGUAGE plpgsql
AS $$
DECLARE
    next_position BIGINT;
BEGIN
    UPDATE authority_recording_boundary
    SET committed_position = committed_position + 1,
        allocating_xid = pg_current_xact_id()
    WHERE singleton = TRUE
    RETURNING committed_position INTO next_position;

    IF next_position IS NULL THEN
        RAISE EXCEPTION 'authority recording boundary is missing';
    END IF;

    RETURN next_position;
END;
$$;

CREATE FUNCTION authority_evidence_position_count(p_position BIGINT)
RETURNS BIGINT
LANGUAGE sql
AS $$
    SELECT
        (SELECT COUNT(*) FROM authority_authorizations
         WHERE recording_position = p_position)
        +
        (SELECT COUNT(*) FROM authority_adjudications
         WHERE recording_position = p_position)
        +
        (SELECT COUNT(*) FROM authority_journal_events
         WHERE recording_position = p_position);
$$;

CREATE FUNCTION validate_authority_evidence_recording()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    boundary_position BIGINT;
    owner_xid xid8;
BEGIN
    IF NEW.recording_position IS NULL THEN
        RAISE EXCEPTION 'new authority evidence requires a recording position';
    END IF;

    SELECT committed_position, allocating_xid
    INTO boundary_position, owner_xid
    FROM authority_recording_boundary
    WHERE singleton = TRUE;

    IF owner_xid IS DISTINCT FROM pg_current_xact_id()
       OR boundary_position IS DISTINCT FROM NEW.recording_position THEN
        RAISE EXCEPTION 'authority evidence position was not allocated by this transaction';
    END IF;

    IF authority_evidence_position_count(NEW.recording_position) <> 1 THEN
        RAISE EXCEPTION 'authority recording position must identify exactly one evidence row';
    END IF;

    RETURN NEW;
END;
$$;

CREATE CONSTRAINT TRIGGER authority_authorizations_recording_guard
AFTER INSERT ON authority_authorizations
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION validate_authority_evidence_recording();

CREATE CONSTRAINT TRIGGER authority_adjudications_recording_guard
AFTER INSERT ON authority_adjudications
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION validate_authority_evidence_recording();

CREATE CONSTRAINT TRIGGER authority_journal_recording_guard
AFTER INSERT ON authority_journal_events
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION validate_authority_evidence_recording();

CREATE FUNCTION validate_authority_recording_allocation()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    IF authority_evidence_position_count(NEW.committed_position) <> 1 THEN
        RAISE EXCEPTION 'authority recording allocation must commit exactly one evidence row';
    END IF;

    RETURN NEW;
END;
$$;

CREATE CONSTRAINT TRIGGER authority_recording_allocation_guard
AFTER UPDATE OF committed_position ON authority_recording_boundary
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION validate_authority_recording_allocation();
