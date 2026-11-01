-- Proposal A only. Apply as a trusted migration owner in the verified legacy
-- schema (public in the operator-supplied production metadata), in a transaction.
-- Runtime/operator role bindings are deliberately separate and require review.
-- The single outer DO keeps all new objects and ACL cleanup atomic even for an
-- autocommit client; it does not invent a runner or version-recording mechanism.
DO $atomic_migration$
BEGIN
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
        WHERE a.attrelid = pg_catalog.to_regclass(pg_catalog.format('%I.%I', target_schema, expected.table_name))
          AND a.attname = expected.column_name AND a.attnum > 0 AND NOT a.attisdropped;
        IF actual_type IS DISTINCT FROM expected.type_oid THEN
            RAISE EXCEPTION 'legacy schema prerequisite mismatch: %.%.%',
                target_schema, expected.table_name, expected.column_name;
        END IF;
    END LOOP;
END;
$preflight$;

-- Pin every DDL reference to the schema validated above, even if this
-- administrator session has colliding temporary tables in its search path.
DO $objects$
DECLARE target_schema TEXT := current_schema();
BEGIN
    EXECUTE pg_catalog.format($table$
CREATE TABLE %1$I.employer_hr_assignments (
    assignment_id UUID PRIMARY KEY,
    principal_id UUID NOT NULL REFERENCES %1$I.domain_users(id) ON UPDATE RESTRICT ON DELETE RESTRICT,
    employer_id TEXT NOT NULL REFERENCES %1$I.evt_employer_config(employer_id) ON UPDATE RESTRICT ON DELETE RESTRICT,
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
    $table$,target_schema);
    EXECUTE pg_catalog.format('CREATE UNIQUE INDEX employer_hr_one_active ON %I.employer_hr_assignments(principal_id,employer_id) WHERE revoked_at IS NULL',target_schema);
    EXECUTE pg_catalog.format('REVOKE ALL ON TABLE %I.employer_hr_assignments FROM PUBLIC',target_schema);
END;
$objects$;

-- Enforce the same lock protocol for reviewed operator SQL as for the CLI.
-- The invoker trigger calls the separately permissioned operator lock helper.
DO $install$
DECLARE
    target_schema TEXT := current_schema();
BEGIN
    -- These bounded SECURITY DEFINER functions delegate row locking, never
    -- profile mutation. Typed parameters, fixed SQL/search_path, no PUBLIC ACL.
    -- Refuse isolation/RLS changes rather than silently bypass their semantics.
    EXECUTE pg_catalog.format($definition$
        CREATE FUNCTION %1$I.check_employer_hr_assignment(p_principal UUID, p_employer TEXT)
        RETURNS BOOLEAN LANGUAGE plpgsql VOLATILE SECURITY DEFINER
        SET search_path = pg_catalog, %1$I, pg_temp
        AS $body$
        DECLARE principal_status TEXT; principal_role TEXT;
        BEGIN
            IF current_setting('transaction_isolation') <> 'read committed' THEN
                RAISE EXCEPTION 'employer authorization requires READ COMMITTED' USING ERRCODE = '55000';
            END IF;
            IF EXISTS(SELECT 1 FROM pg_catalog.pg_class WHERE oid IN
                (%2$L::regclass,%3$L::regclass)
                AND (relrowsecurity OR relforcerowsecurity)) THEN
                RAISE EXCEPTION 'employer authorization RLS requires review' USING ERRCODE = '55000';
            END IF;
            SELECT d.status,d.role INTO principal_status,principal_role
            FROM %1$I.domain_users d WHERE d.id=p_principal FOR SHARE;
            IF NOT FOUND OR principal_status <> 'active' OR principal_role <> 'hr_reviewer' THEN
                RETURN FALSE;
            END IF;
            RETURN EXISTS(SELECT 1 FROM %1$I.employer_hr_assignments a
                WHERE a.principal_id=p_principal AND a.employer_id=p_employer AND a.revoked_at IS NULL);
        END;
        $body$;
    $definition$,target_schema,pg_catalog.format('%I.domain_users',target_schema),pg_catalog.format('%I.employer_hr_assignments',target_schema));
    EXECUTE pg_catalog.format($definition$
        CREATE FUNCTION %1$I.lock_employer_hr_principal(p_principal UUID)
        RETURNS TABLE(id UUID,status TEXT,role TEXT)
        LANGUAGE plpgsql VOLATILE SECURITY DEFINER
        SET search_path = pg_catalog, %1$I, pg_temp
        AS $body$
        BEGIN
            IF current_setting('transaction_isolation') <> 'read committed' THEN
                RAISE EXCEPTION 'employer administration requires READ COMMITTED' USING ERRCODE = '55000';
            END IF;
            IF EXISTS(SELECT 1 FROM pg_catalog.pg_class WHERE oid IN
                (%2$L::regclass,%3$L::regclass)
                AND (relrowsecurity OR relforcerowsecurity)) THEN
                RAISE EXCEPTION 'employer administration RLS requires review' USING ERRCODE = '55000';
            END IF;
            RETURN QUERY SELECT d.id,d.status,d.role FROM %1$I.domain_users d
                WHERE d.id=p_principal FOR UPDATE;
        END;
        $body$;
    $definition$,target_schema,pg_catalog.format('%I.domain_users',target_schema),pg_catalog.format('%I.employer_hr_assignments',target_schema));
    EXECUTE pg_catalog.format($definition$
        CREATE FUNCTION %1$I.enforce_employer_hr_assignment() RETURNS TRIGGER
        LANGUAGE plpgsql SECURITY INVOKER
        SET search_path = pg_catalog, %1$I, pg_temp
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
            FROM %1$I.lock_employer_hr_principal(NEW.principal_id);
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
    $definition$, target_schema);
    EXECUTE pg_catalog.format('REVOKE ALL ON FUNCTION %I.enforce_employer_hr_assignment() FROM PUBLIC', target_schema);
    EXECUTE pg_catalog.format(
        'CREATE TRIGGER trg_employer_hr_assignment_guard BEFORE INSERT OR UPDATE ON %I.employer_hr_assignments '
        'FOR EACH ROW EXECUTE FUNCTION %I.enforce_employer_hr_assignment()',
        target_schema, target_schema
    );
END;
$install$;

-- New objects start owner-only, including when ALTER DEFAULT PRIVILEGES would
-- otherwise grant this runtime writes or other identities routine execution.
-- This affects only objects created above; no legacy parent ACL is changed.
DO $acl$
DECLARE target_schema TEXT := current_schema(); recipient RECORD; routine RECORD;
BEGIN
    FOR recipient IN
        SELECT DISTINCT a.grantee FROM pg_catalog.pg_class c,
        LATERAL pg_catalog.aclexplode(coalesce(c.relacl,pg_catalog.acldefault('r',c.relowner))) a
        WHERE c.oid=pg_catalog.to_regclass(pg_catalog.format('%I.employer_hr_assignments',target_schema))
          AND a.grantee <> c.relowner
    LOOP
        EXECUTE pg_catalog.format('REVOKE ALL ON TABLE %I.employer_hr_assignments FROM %s',target_schema,
            CASE WHEN recipient.grantee=0 THEN 'PUBLIC' ELSE pg_catalog.quote_ident(pg_catalog.pg_get_userbyid(recipient.grantee)) END);
    END LOOP;
    FOR routine IN SELECT * FROM (VALUES
        ('check_employer_hr_assignment','uuid,text'),
        ('lock_employer_hr_principal','uuid'),('enforce_employer_hr_assignment','')
    ) r(name,args) LOOP
        FOR recipient IN
            SELECT DISTINCT a.grantee FROM pg_catalog.pg_proc p,
            LATERAL pg_catalog.aclexplode(coalesce(p.proacl,pg_catalog.acldefault('f',p.proowner))) a
            WHERE p.oid=pg_catalog.to_regprocedure(pg_catalog.format('%I.%I(%s)',target_schema,routine.name,routine.args))
              AND a.grantee <> p.proowner
        LOOP
            EXECUTE pg_catalog.format('REVOKE ALL ON FUNCTION %I.%I(%s) FROM %s',target_schema,routine.name,routine.args,
                CASE WHEN recipient.grantee=0 THEN 'PUBLIC' ELSE pg_catalog.quote_ident(pg_catalog.pg_get_userbyid(recipient.grantee)) END);
        END LOOP;
    END LOOP;
END;
$acl$;

END;
$atomic_migration$;
