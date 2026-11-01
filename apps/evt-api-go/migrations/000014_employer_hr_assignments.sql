-- Proposal A only. Apply as a trusted migration owner in the verified legacy
-- schema (public in the operator-supplied production metadata), in a transaction.
-- Runtime/operator role bindings are deliberately separate and require review.
DO $preflight$
DECLARE
    target_schema TEXT := current_schema();
    expected RECORD;
    actual_type OID;
BEGIN
    IF target_schema IS NULL THEN
        RAISE EXCEPTION 'employer assignment migration requires an explicit legacy schema';
    END IF;
    FOR expected IN
        SELECT * FROM (VALUES
            ('domain_users', 'id', 'uuid'::regtype),
            ('domain_users', 'role', 'text'::regtype),
            ('domain_users', 'status', 'text'::regtype),
            ('evt_employer_config', 'employer_id', 'text'::regtype),
            ('evt_requests', 'request_id', 'uuid'::regtype),
            ('evt_requests', 'candidate_personid', 'uuid'::regtype),
            ('evt_requests', 'employer_id', 'text'::regtype),
            ('evt_requests', 'hr_assigned_personid', 'uuid'::regtype)
        ) AS columns(table_name, column_name, type_oid)
    LOOP
        SELECT a.atttypid INTO actual_type
        FROM pg_catalog.pg_attribute a
        WHERE a.attrelid = pg_catalog.to_regclass(format('%I.%I', target_schema, expected.table_name))
          AND a.attname = expected.column_name AND a.attnum > 0 AND NOT a.attisdropped;
        IF actual_type IS DISTINCT FROM expected.type_oid THEN
            RAISE EXCEPTION 'legacy schema prerequisite mismatch: %.%.%',
                target_schema, expected.table_name, expected.column_name;
        END IF;
    END LOOP;
END;
$preflight$;

CREATE TABLE employer_hr_assignments (
    assignment_id UUID PRIMARY KEY,
    principal_id UUID NOT NULL REFERENCES domain_users(id) ON UPDATE RESTRICT ON DELETE RESTRICT,
    employer_id TEXT NOT NULL REFERENCES evt_employer_config(employer_id) ON UPDATE RESTRICT ON DELETE RESTRICT,
    granted_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    granted_by TEXT NOT NULL CHECK (btrim(granted_by) <> ''),
    grant_basis_ref TEXT NOT NULL CHECK (btrim(grant_basis_ref) <> ''),
    revoked_at TIMESTAMPTZ,
    revoked_by TEXT,
    revocation_reason TEXT,
    CONSTRAINT employer_hr_revocation_metadata CHECK (
        (revoked_at IS NULL AND revoked_by IS NULL AND revocation_reason IS NULL)
        OR
        (revoked_at IS NOT NULL AND revoked_at >= granted_at
         AND revoked_by IS NOT NULL AND btrim(revoked_by) <> ''
         AND revocation_reason IS NOT NULL AND btrim(revocation_reason) <> '')
    )
);
CREATE UNIQUE INDEX employer_hr_one_active
    ON employer_hr_assignments(principal_id, employer_id) WHERE revoked_at IS NULL;
REVOKE ALL ON employer_hr_assignments FROM PUBLIC;

-- Enforce the same lock protocol for reviewed operator SQL as for the CLI.
-- SECURITY INVOKER: this function grants no extra database privileges.
DO $install$
DECLARE
    target_schema TEXT := current_schema();
BEGIN
    EXECUTE format($definition$
        CREATE FUNCTION %I.enforce_employer_hr_assignment() RETURNS TRIGGER
        LANGUAGE plpgsql SECURITY INVOKER
        SET search_path = pg_catalog, %I, pg_temp
        AS $body$
        DECLARE
            principal_status TEXT;
            principal_role TEXT;
        BEGIN
            IF TG_OP = 'UPDATE' THEN
                IF OLD.revoked_at IS NOT NULL THEN
                    RAISE EXCEPTION 'revoked employer assignments are immutable' USING ERRCODE = '23514';
                END IF;
                IF ROW(NEW.assignment_id, NEW.principal_id, NEW.employer_id,
                       NEW.granted_at, NEW.granted_by, NEW.grant_basis_ref)
                   IS DISTINCT FROM
                   ROW(OLD.assignment_id, OLD.principal_id, OLD.employer_id,
                       OLD.granted_at, OLD.granted_by, OLD.grant_basis_ref) THEN
                    RAISE EXCEPTION 'employer grant identity and provenance are immutable' USING ERRCODE = '23514';
                END IF;
                IF NEW.revoked_at IS NULL THEN
                    RAISE EXCEPTION 'assignment updates must revoke the grant' USING ERRCODE = '23514';
                END IF;
            ELSIF NEW.revoked_at IS NOT NULL THEN
                RAISE EXCEPTION 'new assignments must start unrevoked' USING ERRCODE = '23514';
            END IF;

            SELECT status, role INTO principal_status, principal_role
            FROM domain_users WHERE id = NEW.principal_id FOR UPDATE;
            IF NOT FOUND THEN
                RAISE EXCEPTION 'assignment principal does not exist' USING ERRCODE = '23503';
            END IF;
            IF TG_OP = 'INSERT' AND
                (principal_status <> 'active' OR principal_role <> 'hr_reviewer') THEN
                RAISE EXCEPTION 'grant requires a currently active HR reviewer' USING ERRCODE = '23514';
            END IF;
            RETURN NEW;
        END;
        $body$;
    $definition$, target_schema, target_schema);
    EXECUTE format('REVOKE ALL ON FUNCTION %I.enforce_employer_hr_assignment() FROM PUBLIC', target_schema);
    EXECUTE format(
        'CREATE TRIGGER trg_employer_hr_assignment_guard BEFORE INSERT OR UPDATE ON %I.employer_hr_assignments '
        'FOR EACH ROW EXECUTE FUNCTION %I.enforce_employer_hr_assignment()',
        target_schema, target_schema
    );
END;
$install$;
