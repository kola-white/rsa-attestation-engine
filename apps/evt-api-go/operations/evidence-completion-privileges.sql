-- Owner-only binding in the same transaction as 000016.
-- SET LOCAL cvera.evidence_runtime_role to the verified API runtime identity.
DO $binding$
DECLARE s text := current_schema(); runtime_name text := current_setting('cvera.evidence_runtime_role',true); runtime_oid oid; owner_oid oid; item record;
BEGIN
 SELECT oid INTO runtime_oid FROM pg_catalog.pg_roles WHERE rolname=runtime_name;
 IF runtime_oid IS NULL THEN RAISE EXCEPTION 'explicit runtime role required'; END IF;
 SELECT relowner INTO owner_oid FROM pg_catalog.pg_class WHERE oid=pg_catalog.to_regclass(pg_catalog.format('%I.cases',s));
 IF owner_oid IS NULL OR NOT pg_catalog.has_function_privilege(runtime_oid,pg_catalog.to_regprocedure(pg_catalog.format('%I.check_evidence_upload(uuid,text,text)',s)),'EXECUTE') THEN RAISE EXCEPTION 'verified registry checker binding required'; END IF;
 IF EXISTS(SELECT 1 FROM pg_catalog.pg_roles WHERE oid=runtime_oid AND (rolsuper OR rolcreatedb OR rolcreaterole OR rolbypassrls))
 OR EXISTS(SELECT 1 FROM pg_catalog.pg_auth_members WHERE member=runtime_oid)
 OR pg_catalog.pg_has_role(runtime_oid,owner_oid,'MEMBER')
 OR pg_catalog.has_schema_privilege(runtime_oid,s,'CREATE')
 OR pg_catalog.has_database_privilege(runtime_oid,current_database(),'CREATE') THEN RAISE EXCEPTION 'restricted runtime required'; END IF;
 FOR item IN SELECT c.* FROM pg_catalog.pg_class c WHERE c.oid IN
 (pg_catalog.to_regclass(pg_catalog.format('%I.upload_sessions',s)),pg_catalog.to_regclass(pg_catalog.format('%I.evidence_uploads',s)),pg_catalog.to_regclass(pg_catalog.format('%I.evidence_scan_jobs',s))) LOOP
  IF item.relowner<>owner_oid OR item.relrowsecurity OR item.relforcerowsecurity THEN RAISE EXCEPTION 'trusted owner without RLS required'; END IF;
 END LOOP;
 EXECUTE pg_catalog.format('GRANT SELECT ON %I.upload_sessions,%I.evidence_uploads TO %I',s,s,runtime_name);
 EXECUTE pg_catalog.format('GRANT INSERT(upload_session_id,principal_id,case_id,check_id,expires_at) ON %I.upload_sessions TO %I',s,runtime_name);
 EXECUTE pg_catalog.format('GRANT INSERT(upload_id,upload_session_id,case_id,check_id,staging_key,file_name,expected_mime_type,expected_size) ON %I.evidence_uploads TO %I',s,runtime_name);
 EXECUTE pg_catalog.format('GRANT UPDATE(closed_at) ON %I.upload_sessions TO %I',s,runtime_name);
 EXECUTE pg_catalog.format('GRANT UPDATE(status,sealed_key,evidence_hash,completed_at) ON %I.evidence_uploads TO %I',s,runtime_name);
 EXECUTE pg_catalog.format('GRANT INSERT(upload_id) ON %I.evidence_scan_jobs TO %I',s,runtime_name);
 -- No registry administration or permission to mark a scan CLEAN.
END
$binding$;
