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
    tables text[] := ARRAY['domain_users','auth_sessions','refresh_tokens','evt_employer_config','evt_requests','evt_request_events','evt_token_consumptions','recruiter_trusted_issuers'];
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
    EXECUTE pg_catalog.format('GRANT SELECT,INSERT ON %1$I.domain_users TO %2$I',target_schema,runtime_name);
    EXECUTE pg_catalog.format('GRANT UPDATE(email,name,updated_at) ON %1$I.domain_users TO %2$I',target_schema,runtime_name);
    EXECUTE pg_catalog.format('GRANT SELECT,INSERT,UPDATE ON %1$I.auth_sessions,%1$I.refresh_tokens,%1$I.evt_requests TO %2$I',target_schema,runtime_name);
    EXECUTE pg_catalog.format('GRANT SELECT,INSERT ON %1$I.evt_employer_config TO %2$I',target_schema,runtime_name);
    EXECUTE pg_catalog.format('GRANT INSERT ON %1$I.evt_request_events,%1$I.evt_token_consumptions TO %2$I',target_schema,runtime_name);
    EXECUTE pg_catalog.format('GRANT SELECT ON %1$I.recruiter_trusted_issuers TO %2$I',target_schema,runtime_name);
    -- These exact serial sequences/defaults were verified in production.
    EXECUTE pg_catalog.format('GRANT USAGE ON SEQUENCE %1$I.evt_request_events_event_id_seq,%1$I.evt_token_consumptions_consumption_id_seq TO %2$I',target_schema,runtime_name);
    IF pg_catalog.has_column_privilege(runtime_oid,pg_catalog.to_regclass(pg_catalog.format('%I.domain_users',target_schema)),'role','UPDATE')
       OR pg_catalog.has_column_privilege(runtime_oid,pg_catalog.to_regclass(pg_catalog.format('%I.domain_users',target_schema)),'status','UPDATE') THEN
        RAISE EXCEPTION 'runtime may not change principal roles/status';
    END IF;
    -- Assignment access is bound separately by hr-assignment-privileges.sql.
    -- No authority-table history or privileges are fabricated here.
END;
$service_grants$;
