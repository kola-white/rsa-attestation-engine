-- Reviewed operator-only grants, not an application migration or automatic runner.
-- Required transaction settings: cvera.service_runtime_role; explicit trusted schema.
DO $service_grants$
DECLARE
    target_schema text := current_schema();
    runtime_name text := current_setting('cvera.service_runtime_role',true);
    runtime_oid oid;
    candidate record;
    object record;
    table_name text;
    tables text[] := ARRAY['continuity_containers','courier_message_dispatches','courier_messages','identities','identity_credential_identifiers','identity_credential_types','identity_credentials','identity_login_codes','identity_recovery_addresses','identity_recovery_codes','identity_recovery_tokens','identity_registration_codes','identity_verifiable_addresses','identity_verification_codes','identity_verification_tokens','networks','schema_migration','selfservice_errors','selfservice_login_flows','selfservice_recovery_flows','selfservice_registration_flows','selfservice_settings_flows','selfservice_verification_flows','session_devices','session_token_exchanges','sessions'];
BEGIN
    SELECT * INTO candidate FROM pg_catalog.pg_roles WHERE rolname=runtime_name;
    IF NOT FOUND THEN RAISE EXCEPTION 'explicit existing service role required'; END IF;
    runtime_oid := candidate.oid;
    IF candidate.rolsuper OR candidate.rolcreatedb OR candidate.rolcreaterole
       OR candidate.rolreplication OR candidate.rolbypassrls
       OR EXISTS(SELECT 1 FROM pg_catalog.pg_database WHERE datdba=runtime_oid)
       OR EXISTS(SELECT 1 FROM pg_catalog.pg_auth_members WHERE member=runtime_oid OR roleid=runtime_oid)
       OR pg_catalog.has_database_privilege(runtime_oid,current_database(),'CREATE')
       OR pg_catalog.has_schema_privilege(runtime_oid,target_schema,'CREATE') THEN
        RAISE EXCEPTION 'service requires a dedicated non-owner role without memberships or creation rights';
    END IF;
    FOREACH table_name IN ARRAY tables LOOP
        SELECT c.* INTO object FROM pg_catalog.pg_class c
        WHERE c.oid=pg_catalog.to_regclass(pg_catalog.format('%I.%I',target_schema,table_name))
          AND c.relkind IN ('r','p');
        IF NOT FOUND THEN RAISE EXCEPTION 'required service table missing: %',table_name; END IF;
        IF object.relrowsecurity OR object.relforcerowsecurity
           OR pg_catalog.pg_has_role(runtime_oid,object.relowner,'MEMBER') THEN
            RAISE EXCEPTION 'service table ownership/RLS requires review: %',table_name;
        END IF;
        -- Fresh dedicated roles are expected. Do not remove privileges from
        -- legacy owners or silently reconcile a previously overprivileged role.
        IF pg_catalog.has_table_privilege(runtime_oid,object.oid,'TRUNCATE,REFERENCES,TRIGGER') THEN
            RAISE EXCEPTION 'unexpected effective service privileges require review: %',table_name;
        END IF;
    END LOOP;
    EXECUTE pg_catalog.format('GRANT CONNECT ON DATABASE %I TO %I',current_database(),runtime_name);
    EXECUTE pg_catalog.format('GRANT USAGE ON SCHEMA %I TO %I',target_schema,runtime_name);
    FOREACH table_name IN ARRAY tables LOOP
        EXECUTE pg_catalog.format('GRANT SELECT ON %I.%I TO %I',target_schema,table_name,runtime_name);
        -- Runtime can inspect the existing vendor version ledger, not rewrite
        -- it or run migrations. The 25 other tables belong to the identity store.
        IF table_name <> 'schema_migration' THEN
            EXECUTE pg_catalog.format('GRANT INSERT,UPDATE,DELETE ON %I.%I TO %I',target_schema,table_name,runtime_name);
        END IF;
    END LOOP;
    IF pg_catalog.has_any_column_privilege(runtime_oid,pg_catalog.to_regclass(pg_catalog.format('%I.schema_migration',target_schema)),'INSERT,UPDATE')
       OR pg_catalog.has_table_privilege(runtime_oid,pg_catalog.to_regclass(pg_catalog.format('%I.schema_migration',target_schema)),'DELETE') THEN
        RAISE EXCEPTION 'Kratos runtime may not rewrite its migration ledger';
    END IF;
    -- The authoritative current identity schema has no sequences. No CREATE,
    -- ownership, function administration or future-table default grants are added.
END;
$service_grants$;
