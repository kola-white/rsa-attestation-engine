-- Harden recording allocation without changing its transactional semantics.
-- This migration must be executed by a trusted schema-owning migration role.
-- Runtime application roles must not own this function or its underlying table.

DO $migration$
DECLARE
    target_schema TEXT := current_schema();
BEGIN
    IF target_schema IS NULL THEN
        RAISE EXCEPTION 'authority migration requires a current schema';
    END IF;

    -- Resolve the recording boundary in the migration-owned schema, never
    -- through a caller-controlled search_path. pg_temp must come last.
    EXECUTE format(
        'ALTER FUNCTION %I.next_authority_recording_position() '
        'SECURITY DEFINER SET search_path = %I, pg_temp',
        target_schema,
        target_schema
    );

    EXECUTE format(
        'REVOKE ALL ON FUNCTION %I.next_authority_recording_position() FROM PUBLIC',
        target_schema
    );
END;
$migration$;
