-- Reviewed role binding, NOT a numbered migration and NOT automatic deployment.
-- Execute transactionally as the trusted object owner only after metadata review.
-- Required transaction-local settings (no default role names):
--   cvera.hr_runtime_role, cvera.hr_operator_role
-- Set search_path to the verified legacy schema (public for supplied metadata).
-- No roles, users, credentials, memberships, or legacy RLS policies are created.
DO $binding$
DECLARE
    target_schema TEXT := current_schema();
    runtime_name TEXT := current_setting('cvera.hr_runtime_role', true);
    operator_name TEXT := current_setting('cvera.hr_operator_role', true);
    runtime_oid OID;
    operator_oid OID;
    assignment_oid OID;
    object RECORD;
    candidate RECORD;
BEGIN
    IF coalesce(runtime_name, '') = '' OR coalesce(operator_name, '') = '' THEN
        RAISE EXCEPTION 'reviewed runtime and operator role names are required';
    END IF;
    SELECT oid INTO runtime_oid FROM pg_catalog.pg_roles WHERE rolname = runtime_name;
    SELECT oid INTO operator_oid FROM pg_catalog.pg_roles WHERE rolname = operator_name;
    IF runtime_oid IS NULL OR operator_oid IS NULL OR runtime_oid = operator_oid THEN
        RAISE EXCEPTION 'distinct existing runtime and operator identities are required';
    END IF;
    IF pg_catalog.pg_has_role(runtime_oid, operator_oid, 'MEMBER')
       OR pg_catalog.pg_has_role(operator_oid, runtime_oid, 'MEMBER') THEN
        RAISE EXCEPTION 'runtime and operator identities must not inherit or assume one another';
    END IF;
    FOR candidate IN SELECT oid, rolsuper, rolcreaterole, rolcreatedb, rolbypassrls FROM pg_catalog.pg_roles WHERE oid IN (runtime_oid, operator_oid) LOOP
        IF candidate.rolsuper OR candidate.rolcreaterole OR candidate.rolcreatedb OR candidate.rolbypassrls
           OR pg_catalog.has_schema_privilege(candidate.oid, target_schema, 'CREATE')
           OR pg_catalog.has_database_privilege(candidate.oid, current_database(), 'CREATE') THEN
            RAISE EXCEPTION 'binding requires unprivileged non-owner identities and a trusted schema';
        END IF;
    END LOOP;
    assignment_oid := pg_catalog.to_regclass(format('%I.employer_hr_assignments', target_schema));
    IF assignment_oid IS NULL THEN RAISE EXCEPTION 'assignment migration must be applied first'; END IF;
    FOR object IN SELECT relowner, relrowsecurity, relforcerowsecurity FROM pg_catalog.pg_class
        WHERE oid IN (
            assignment_oid,
            pg_catalog.to_regclass(format('%I.domain_users', target_schema)),
            pg_catalog.to_regclass(format('%I.evt_employer_config', target_schema)),
            pg_catalog.to_regclass(format('%I.evt_requests', target_schema))
        ) LOOP
        IF pg_catalog.pg_has_role(runtime_oid, object.relowner, 'MEMBER')
           OR pg_catalog.pg_has_role(operator_oid, object.relowner, 'MEMBER') THEN
            RAISE EXCEPTION 'runtime/operator ownership or owner-role membership requires resolution';
        END IF;
        IF object.relrowsecurity OR object.relforcerowsecurity THEN
            RAISE EXCEPTION 'existing RLS requires a separately reviewed compatible binding';
        END IF;
    END LOOP;
    IF EXISTS (
        SELECT 1 FROM pg_catalog.pg_class c,
        LATERAL pg_catalog.aclexplode(coalesce(c.relacl, pg_catalog.acldefault('r', c.relowner))) a
        WHERE c.oid = assignment_oid AND
          (a.grantee NOT IN (c.relowner, runtime_oid, operator_oid)
           OR (a.grantee <> c.relowner AND a.is_grantable))
    ) THEN
        RAISE EXCEPTION 'unexpected default/direct assignment ACL requires review';
    END IF;

    EXECUTE format('REVOKE ALL ON TABLE %I.employer_hr_assignments FROM PUBLIC, %I, %I', target_schema, runtime_name, operator_name);
    EXECUTE format('GRANT USAGE ON SCHEMA %I TO %I, %I', target_schema, runtime_name, operator_name);
    EXECUTE format('GRANT SELECT ON TABLE %I.employer_hr_assignments TO %I, %I', target_schema, runtime_name, operator_name);
    EXECUTE format('GRANT SELECT (id, role, status), UPDATE (email) ON TABLE %I.domain_users TO %I, %I', target_schema, runtime_name, operator_name);
    EXECUTE format('GRANT SELECT (employer_id) ON TABLE %I.evt_employer_config TO %I', target_schema, operator_name);
    EXECUTE format('GRANT INSERT (assignment_id, principal_id, employer_id, granted_at, granted_by, grant_basis_ref), UPDATE (revoked_at, revoked_by, revocation_reason) ON TABLE %I.employer_hr_assignments TO %I', target_schema, operator_name);

    -- Detect effective privileges, including inheritance and column ACLs. Do not
    -- silently revoke legacy permissions needed by other application workflows.
    IF pg_catalog.has_any_column_privilege(runtime_oid, assignment_oid, 'INSERT')
       OR pg_catalog.has_any_column_privilege(runtime_oid, assignment_oid, 'UPDATE')
       OR pg_catalog.has_table_privilege(runtime_oid, assignment_oid, 'DELETE,TRUNCATE,REFERENCES,TRIGGER')
       OR pg_catalog.has_table_privilege(operator_oid, assignment_oid, 'DELETE,TRUNCATE,REFERENCES,TRIGGER')
       OR EXISTS(SELECT 1 FROM pg_catalog.pg_attribute a WHERE a.attrelid=assignment_oid
                 AND a.attnum>0 AND NOT a.attisdropped
                 AND a.attname NOT IN ('revoked_at','revoked_by','revocation_reason')
                 AND pg_catalog.has_column_privilege(operator_oid,assignment_oid,a.attname,'UPDATE')) THEN
        RAISE EXCEPTION 'effective assignment privileges exceed the reviewed boundary';
    END IF;
    IF pg_catalog.has_any_column_privilege(operator_oid, pg_catalog.to_regclass(format('%I.evt_requests',target_schema)), 'SELECT')
       OR pg_catalog.has_column_privilege(operator_oid, pg_catalog.to_regclass(format('%I.domain_users',target_schema)), 'email', 'SELECT')
       OR pg_catalog.has_column_privilege(operator_oid, pg_catalog.to_regclass(format('%I.domain_users',target_schema)), 'name', 'SELECT') THEN
        RAISE EXCEPTION 'operator has unrelated application-record read privileges requiring review';
    END IF;
    FOR candidate IN SELECT oid FROM pg_catalog.pg_roles WHERE oid IN (runtime_oid, operator_oid) LOOP
        IF pg_catalog.has_column_privilege(candidate.oid, pg_catalog.to_regclass(format('%I.domain_users', target_schema)), 'role', 'UPDATE')
           OR pg_catalog.has_column_privilege(candidate.oid, pg_catalog.to_regclass(format('%I.domain_users', target_schema)), 'status', 'UPDATE') THEN
            RAISE EXCEPTION 'legacy user role/status write privileges require separate resolution';
        END IF;
    END LOOP;
END;
$binding$;
