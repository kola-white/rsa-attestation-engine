-- Same transaction as 000019, existing restricted runtime only. No operator
-- credential activation, authority establishment or revocation permission.
DO $binding$
DECLARE s text:=current_schema(); runtime_name text:=current_setting('cvera.evidence_runtime_role',true);
 runtime_oid oid;owner_oid oid;item record;signature text; name text;
 signatures text[]:=ARRAY['active_evidence_signing_credential(text)','read_evidence_authority()','prepare_bound_issuance(uuid,uuid,text,text)','commit_bound_issuance(uuid,uuid,text,text,text,jsonb)','resolve_bound_credential(text)'];
BEGIN
 SELECT oid INTO runtime_oid FROM pg_catalog.pg_roles WHERE rolname=runtime_name;
 SELECT relowner INTO owner_oid FROM pg_catalog.pg_class WHERE oid=pg_catalog.to_regclass(pg_catalog.format('%I.cases',s));
 IF runtime_oid IS NULL OR owner_oid IS NULL OR runtime_oid=owner_oid THEN RAISE EXCEPTION 'existing restricted runtime required'; END IF;
 SELECT * INTO item FROM pg_catalog.pg_roles WHERE oid=runtime_oid;
 IF item.rolsuper OR item.rolcreatedb OR item.rolcreaterole OR item.rolbypassrls OR item.rolreplication OR EXISTS(SELECT 1 FROM pg_catalog.pg_auth_members WHERE member=runtime_oid)
  OR pg_catalog.pg_has_role(runtime_oid,owner_oid,'MEMBER') OR pg_catalog.has_schema_privilege(runtime_oid,s,'CREATE') OR pg_catalog.has_database_privilege(runtime_oid,current_database(),'CREATE') THEN RAISE EXCEPTION 'restricted non-owner runtime required'; END IF;
 FOREACH name IN ARRAY ARRAY['evidence_issuer_credentials','evidence_issued_credentials'] LOOP
  SELECT * INTO STRICT item FROM pg_catalog.pg_class WHERE oid=pg_catalog.to_regclass(pg_catalog.format('%I.%I',s,name));
  IF item.relowner<>owner_oid OR item.relrowsecurity OR item.relforcerowsecurity OR EXISTS(SELECT 1 FROM pg_catalog.aclexplode(coalesce(item.relacl,pg_catalog.acldefault('r',item.relowner))) WHERE grantee<>owner_oid) OR EXISTS(SELECT 1 FROM pg_catalog.pg_attribute WHERE attrelid=item.oid AND attacl IS NOT NULL) THEN RAISE EXCEPTION 'owner-only issuance tables required'; END IF;
 END LOOP;
 FOREACH signature IN ARRAY signatures||ARRAY['guard_evidence_issuance()','revoke_bound_credential(uuid,text,text)'] LOOP
  SELECT * INTO STRICT item FROM pg_catalog.pg_proc WHERE oid=pg_catalog.to_regprocedure(pg_catalog.format('%I.%s',s,signature));
  IF item.proowner<>owner_oid OR NOT item.prosecdef OR item.provolatile<>'v' OR NOT coalesce(pg_catalog.format('search_path=pg_catalog, %I, pg_temp',s)=ANY(item.proconfig),false) OR EXISTS(SELECT 1 FROM pg_catalog.aclexplode(coalesce(item.proacl,pg_catalog.acldefault('f',item.proowner))) WHERE grantee<>owner_oid) THEN RAISE EXCEPTION 'private fixed-path function required'; END IF;
 END LOOP;
 FOREACH signature IN ARRAY signatures LOOP EXECUTE pg_catalog.format('GRANT EXECUTE ON FUNCTION %I.%s TO %I',s,signature,runtime_name);END LOOP;
END $binding$;
