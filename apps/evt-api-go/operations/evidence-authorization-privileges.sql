-- Reviewed owner-only binding, separate from migration; no role creation.
-- Explicit LOCAL cvera.evidence_runtime_role and cvera.evidence_operator_role.
-- Only the boolean checker goes to runtime; operator gets bounded functions.
DO $binding$
DECLARE
    s text := current_schema();
    runtime_name text := current_setting('cvera.evidence_runtime_role',true);
    operator_name text := current_setting('cvera.evidence_operator_role',true);
    runtime_oid oid;
    operator_oid oid;
    trusted_owner oid;
    item record;
    name text;
    signature text;
    signatures text[] := ARRAY[
        'check_evidence_upload(uuid,text,text)',
        'provision_evidence_case(text,text,text)',
        'provision_evidence_check(text,text,text,text)',
        'grant_evidence_upload(uuid,text,text,text)',
        'revoke_evidence_upload(uuid,text,text)',
        'set_evidence_resource_state(text,text,text,text,text)'];
BEGIN
    SELECT oid INTO runtime_oid FROM pg_catalog.pg_roles WHERE rolname=runtime_name;
    SELECT oid INTO operator_oid FROM pg_catalog.pg_roles WHERE rolname=operator_name AND NOT rolcanlogin;
    IF runtime_oid IS NULL OR operator_oid IS NULL OR runtime_oid=operator_oid THEN
        RAISE EXCEPTION 'distinct existing runtime and NOLOGIN operator roles required';
    END IF;
    FOR item IN SELECT * FROM pg_catalog.pg_roles WHERE oid IN (runtime_oid,operator_oid) LOOP
        IF item.rolsuper OR item.rolcreatedb OR item.rolcreaterole OR item.rolbypassrls
           OR EXISTS(SELECT 1 FROM pg_catalog.pg_auth_members WHERE member=item.oid)
           OR pg_catalog.has_schema_privilege(item.oid,s,'CREATE')
           OR pg_catalog.has_database_privilege(item.oid,current_database(),'CREATE') THEN
            RAISE EXCEPTION 'restricted non-owner roles without memberships/creation rights required';
        END IF;
    END LOOP;
    SELECT relowner INTO trusted_owner FROM pg_catalog.pg_class
        WHERE oid=pg_catalog.to_regclass(pg_catalog.format('%I.cases',s));
    IF trusted_owner IS NULL THEN RAISE EXCEPTION 'migration 000015 required'; END IF;
    FOREACH name IN ARRAY ARRAY['domain_users','cases','verification_checks','case_evidence_upload_grants'] LOOP
        SELECT * INTO item FROM pg_catalog.pg_class WHERE oid=pg_catalog.to_regclass(pg_catalog.format('%I.%I',s,name));
        IF NOT FOUND OR item.relrowsecurity OR item.relforcerowsecurity
           OR pg_catalog.pg_has_role(runtime_oid,item.relowner,'MEMBER')
           OR pg_catalog.pg_has_role(operator_oid,item.relowner,'MEMBER') THEN
            RAISE EXCEPTION 'missing table, RLS or owner-role access requires review';
        END IF;
        IF name<>'domain_users' THEN
            IF item.relowner<>trusted_owner THEN RAISE EXCEPTION 'registry owner mismatch'; END IF;
            IF EXISTS(SELECT 1 FROM pg_catalog.aclexplode(coalesce(item.relacl,pg_catalog.acldefault('r',item.relowner))) a
                WHERE a.grantee<>trusted_owner) THEN
                RAISE EXCEPTION 'registry must remain owner-only';
            END IF;
            -- Also reject column ACLs; table-level ACL checks alone are insufficient.
            IF EXISTS(SELECT 1 FROM pg_catalog.pg_attribute a WHERE a.attrelid=item.oid AND a.attacl IS NOT NULL) THEN
                RAISE EXCEPTION 'unexpected registry column ACL';
            END IF;
        END IF;
    END LOOP;
    FOREACH signature IN ARRAY signatures LOOP
        SELECT * INTO item FROM pg_catalog.pg_proc WHERE oid=pg_catalog.to_regprocedure(pg_catalog.format('%I.%s',s,signature));
        IF NOT FOUND OR item.proowner<>trusted_owner OR NOT item.prosecdef
           OR NOT (pg_catalog.format('search_path=pg_catalog, %I, pg_temp',s)=ANY(item.proconfig)) THEN
            RAISE EXCEPTION 'trusted fixed-path definer required: %',signature;
        END IF;
        IF EXISTS(SELECT 1 FROM pg_catalog.aclexplode(coalesce(item.proacl,pg_catalog.acldefault('f',item.proowner))) a
            WHERE a.grantee NOT IN (trusted_owner,runtime_oid,operator_oid) OR (a.grantee<>trusted_owner AND a.is_grantable)) THEN
            RAISE EXCEPTION 'unexpected function ACL';
        END IF;
        EXECUTE pg_catalog.format('REVOKE ALL ON FUNCTION %I.%s FROM PUBLIC,%I,%I',s,signature,runtime_name,operator_name);
        EXECUTE pg_catalog.format('GRANT EXECUTE ON FUNCTION %I.%s TO %I',s,signature,
            CASE WHEN signature=signatures[1] THEN runtime_name ELSE operator_name END);
    END LOOP;
    EXECUTE pg_catalog.format('GRANT USAGE ON SCHEMA %I TO %I,%I',s,runtime_name,operator_name);
END;
$binding$;
