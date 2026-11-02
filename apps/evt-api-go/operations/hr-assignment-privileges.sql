-- Reviewed binding only; no role creation, memberships or automatic deployment.
-- Required LOCAL settings: cvera.hr_runtime_role, cvera.hr_operator_role.
-- Select the verified legacy schema explicitly. Legacy parent ACLs are preserved.
DO $binding$
DECLARE
    target_schema TEXT := current_schema();
    runtime_name TEXT := current_setting('cvera.hr_runtime_role',true);
    operator_name TEXT := current_setting('cvera.hr_operator_role',true);
    runtime_oid OID;
    operator_oid OID;
    assignment_oid OID;
    assignment_owner OID;
    check_oid OID;
    lock_oid OID;
    guard_oid OID;
    object RECORD;
    candidate RECORD;
BEGIN
    IF coalesce(runtime_name,'')='' OR coalesce(operator_name,'')='' THEN
        RAISE EXCEPTION 'reviewed runtime and operator role names are required';
    END IF;
    SELECT oid INTO runtime_oid FROM pg_catalog.pg_roles WHERE rolname=runtime_name;
    SELECT oid INTO operator_oid FROM pg_catalog.pg_roles WHERE rolname=operator_name;
    IF runtime_oid IS NULL OR operator_oid IS NULL OR runtime_oid=operator_oid THEN
        RAISE EXCEPTION 'distinct existing runtime and operator identities are required';
    END IF;
    IF pg_catalog.pg_has_role(runtime_oid,operator_oid,'MEMBER')
       OR pg_catalog.pg_has_role(operator_oid,runtime_oid,'MEMBER') THEN
        RAISE EXCEPTION 'runtime and operator identities must not inherit or assume one another';
    END IF;
    FOR candidate IN SELECT oid,rolsuper,rolcreaterole,rolcreatedb,rolbypassrls
        FROM pg_catalog.pg_roles WHERE oid IN (runtime_oid,operator_oid) LOOP
        IF candidate.rolsuper OR candidate.rolcreaterole OR candidate.rolcreatedb OR candidate.rolbypassrls
           OR pg_catalog.has_schema_privilege(candidate.oid,target_schema,'CREATE')
           OR pg_catalog.has_database_privilege(candidate.oid,current_database(),'CREATE') THEN
            RAISE EXCEPTION 'binding requires unprivileged non-owner identities and a trusted schema';
        END IF;
    END LOOP;
    assignment_oid:=pg_catalog.to_regclass(pg_catalog.format('%I.employer_hr_assignments',target_schema));
    check_oid:=pg_catalog.to_regprocedure(pg_catalog.format('%I.check_employer_hr_assignment(uuid,text)',target_schema));
    lock_oid:=pg_catalog.to_regprocedure(pg_catalog.format('%I.lock_employer_hr_principal(uuid)',target_schema));
    guard_oid:=pg_catalog.to_regprocedure(pg_catalog.format('%I.enforce_employer_hr_assignment()',target_schema));
    IF assignment_oid IS NULL OR check_oid IS NULL OR lock_oid IS NULL OR guard_oid IS NULL THEN
        RAISE EXCEPTION 'assignment migration and bounded locking functions are required';
    END IF;
    SELECT relowner INTO assignment_owner FROM pg_catalog.pg_class WHERE oid=assignment_oid;
    FOR object IN SELECT relowner,relrowsecurity,relforcerowsecurity FROM pg_catalog.pg_class
        WHERE oid IN (assignment_oid,
            pg_catalog.to_regclass(pg_catalog.format('%I.domain_users',target_schema)),
            pg_catalog.to_regclass(pg_catalog.format('%I.evt_employer_config',target_schema)),
            pg_catalog.to_regclass(pg_catalog.format('%I.evt_requests',target_schema))) LOOP
        IF pg_catalog.pg_has_role(runtime_oid,object.relowner,'MEMBER')
           OR pg_catalog.pg_has_role(operator_oid,object.relowner,'MEMBER') THEN
            RAISE EXCEPTION 'runtime/operator ownership or owner-role membership requires resolution';
        END IF;
        IF object.relrowsecurity OR object.relforcerowsecurity THEN
            RAISE EXCEPTION 'existing RLS requires a separately reviewed compatible binding';
        END IF;
    END LOOP;
    IF EXISTS(SELECT 1 FROM pg_catalog.pg_proc WHERE oid IN (check_oid,lock_oid,guard_oid)
              AND proowner<>assignment_owner) THEN
        RAISE EXCEPTION 'locking and guard routines must retain trusted migration ownership';
    END IF;
    IF EXISTS(SELECT 1 FROM pg_catalog.pg_class c,
        LATERAL pg_catalog.aclexplode(coalesce(c.relacl,pg_catalog.acldefault('r',c.relowner))) a
        WHERE c.oid=assignment_oid AND
        (a.grantee NOT IN (c.relowner,runtime_oid,operator_oid)
         OR (a.grantee<>c.relowner AND a.is_grantable))) THEN
        RAISE EXCEPTION 'unexpected assignment ACL requires review';
    END IF;

    EXECUTE pg_catalog.format('REVOKE ALL ON TABLE %I.employer_hr_assignments FROM PUBLIC,%I,%I',target_schema,runtime_name,operator_name);
    EXECUTE pg_catalog.format('REVOKE ALL ON FUNCTION %I.check_employer_hr_assignment(uuid,text), %I.lock_employer_hr_principal(uuid), %I.enforce_employer_hr_assignment() FROM PUBLIC,%I,%I',target_schema,target_schema,target_schema,runtime_name,operator_name);
    EXECUTE pg_catalog.format('GRANT USAGE ON SCHEMA %I TO %I,%I',target_schema,runtime_name,operator_name);
    -- Runtime checks authorization through the bounded function; no raw table
    -- access or principal UPDATE privileges are added for authorization locking.
    EXECUTE pg_catalog.format('GRANT EXECUTE ON FUNCTION %I.check_employer_hr_assignment(uuid,text) TO %I',target_schema,runtime_name);
    EXECUTE pg_catalog.format('GRANT EXECUTE ON FUNCTION %I.lock_employer_hr_principal(uuid) TO %I',target_schema,operator_name);
    EXECUTE pg_catalog.format('GRANT SELECT ON TABLE %I.employer_hr_assignments TO %I',target_schema,operator_name);
    EXECUTE pg_catalog.format('GRANT SELECT (employer_id) ON TABLE %I.evt_employer_config TO %I',target_schema,operator_name);
    EXECUTE pg_catalog.format('GRANT INSERT (assignment_id,principal_id,employer_id,granted_at,granted_by,grant_basis_ref), UPDATE (revoked_at,revoked_by,revocation_reason) ON TABLE %I.employer_hr_assignments TO %I',target_schema,operator_name);

    IF pg_catalog.has_any_column_privilege(runtime_oid,assignment_oid,'SELECT')
       OR pg_catalog.has_any_column_privilege(runtime_oid,assignment_oid,'INSERT')
       OR pg_catalog.has_any_column_privilege(runtime_oid,assignment_oid,'UPDATE')
       OR pg_catalog.has_table_privilege(runtime_oid,assignment_oid,'DELETE,TRUNCATE,REFERENCES,TRIGGER')
       OR pg_catalog.has_function_privilege(runtime_oid,lock_oid,'EXECUTE')
       OR pg_catalog.has_function_privilege(runtime_oid,guard_oid,'EXECUTE')
       OR pg_catalog.has_table_privilege(operator_oid,assignment_oid,'DELETE,TRUNCATE,REFERENCES,TRIGGER')
       OR EXISTS(SELECT 1 FROM pg_catalog.pg_attribute a WHERE a.attrelid=assignment_oid
            AND a.attnum>0 AND NOT a.attisdropped AND a.attname NOT IN ('revoked_at','revoked_by','revocation_reason')
            AND pg_catalog.has_column_privilege(operator_oid,assignment_oid,a.attname,'UPDATE')) THEN
        RAISE EXCEPTION 'effective assignment privileges exceed the reviewed boundary';
    END IF;
    -- The verified evt-app role already has broad legacy DML. Preserve it;
    -- these legacy grants are neither required nor added by the new checker.
    -- A dedicated operator, however, needs no principal/profile writes.
    IF pg_catalog.has_any_column_privilege(operator_oid,pg_catalog.to_regclass(pg_catalog.format('%I.domain_users',target_schema)),'UPDATE')
       OR pg_catalog.has_any_column_privilege(operator_oid,pg_catalog.to_regclass(pg_catalog.format('%I.domain_users',target_schema)),'INSERT')
       OR pg_catalog.has_table_privilege(operator_oid,pg_catalog.to_regclass(pg_catalog.format('%I.domain_users',target_schema)),'DELETE,TRUNCATE,REFERENCES,TRIGGER')
       OR pg_catalog.has_any_column_privilege(operator_oid,pg_catalog.to_regclass(pg_catalog.format('%I.evt_requests',target_schema)),'SELECT')
       OR pg_catalog.has_column_privilege(operator_oid,pg_catalog.to_regclass(pg_catalog.format('%I.domain_users',target_schema)),'email','SELECT')
       OR pg_catalog.has_column_privilege(operator_oid,pg_catalog.to_regclass(pg_catalog.format('%I.domain_users',target_schema)),'name','SELECT') THEN
        RAISE EXCEPTION 'operator has unrelated application-record privileges requiring review';
    END IF;
END;
$binding$;
