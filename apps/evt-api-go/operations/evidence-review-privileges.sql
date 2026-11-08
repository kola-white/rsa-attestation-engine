-- Trusted owner only, same transaction as 000017. No role creation or table grants.
-- LOCAL cvera.evidence_runtime_role and cvera.evidence_operator_role are explicit.
DO $binding$
DECLARE s text:=current_schema(); runtime_name text:=current_setting('cvera.evidence_runtime_role',true);
 operator_name text:=current_setting('cvera.evidence_operator_role',true); runtime_oid oid; operator_oid oid;
 owner_oid oid; item record; signature text; name text;
 runtime_functions text[]:=ARRAY['list_evidence_checks(uuid)','list_evidence_for_review(uuid,text,text)',
  'get_evidence_for_review(uuid,text,text,uuid)','record_evidence_review(uuid,text,text,uuid,text,text)'];
 operator_functions text[]:=ARRAY['grant_evidence_review(uuid,text,text,text)','revoke_evidence_review(uuid,text,text)'];
BEGIN
 SELECT oid INTO runtime_oid FROM pg_catalog.pg_roles WHERE rolname=runtime_name;
 SELECT oid INTO operator_oid FROM pg_catalog.pg_roles WHERE rolname=operator_name AND NOT rolcanlogin;
 SELECT relowner INTO owner_oid FROM pg_catalog.pg_class WHERE oid=pg_catalog.to_regclass(pg_catalog.format('%I.cases',s));
 IF runtime_oid IS NULL OR operator_oid IS NULL OR owner_oid IS NULL OR runtime_oid=operator_oid THEN
  RAISE EXCEPTION 'existing restricted runtime and NOLOGIN operator required';
 END IF;
 FOR item IN SELECT * FROM pg_catalog.pg_roles WHERE oid IN (runtime_oid,operator_oid) LOOP
  IF item.rolsuper OR item.rolcreatedb OR item.rolcreaterole OR item.rolbypassrls
   OR EXISTS(SELECT 1 FROM pg_catalog.pg_auth_members WHERE member=item.oid)
   OR pg_catalog.pg_has_role(item.oid,owner_oid,'MEMBER')
   OR pg_catalog.has_schema_privilege(item.oid,s,'CREATE')
   OR pg_catalog.has_database_privilege(item.oid,current_database(),'CREATE') THEN
   RAISE EXCEPTION 'restricted non-owner role required';
  END IF;
 END LOOP;
 FOREACH name IN ARRAY ARRAY['case_evidence_review_grants','evidence_reviews'] LOOP
  SELECT * INTO item FROM pg_catalog.pg_class WHERE oid=pg_catalog.to_regclass(pg_catalog.format('%I.%I',s,name));
  IF NOT FOUND OR item.relowner<>owner_oid OR item.relrowsecurity OR item.relforcerowsecurity
   OR EXISTS(SELECT 1 FROM pg_catalog.aclexplode(coalesce(item.relacl,pg_catalog.acldefault('r',item.relowner))) WHERE grantee<>owner_oid)
   OR EXISTS(SELECT 1 FROM pg_catalog.pg_attribute WHERE attrelid=item.oid AND attacl IS NOT NULL) THEN
   RAISE EXCEPTION 'owner-only review tables without RLS required';
  END IF;
 END LOOP;
 FOREACH signature IN ARRAY runtime_functions||operator_functions||ARRAY['guard_evidence_review_catalog()','check_evidence_review(uuid,text,text)'] LOOP
  SELECT * INTO item FROM pg_catalog.pg_proc WHERE oid=pg_catalog.to_regprocedure(pg_catalog.format('%I.%s',s,signature));
  IF NOT FOUND OR item.proowner<>owner_oid OR NOT item.prosecdef OR item.provolatile<>'v'
   OR NOT coalesce(pg_catalog.format('search_path=pg_catalog, %I, pg_temp',s)=ANY(item.proconfig),false)
   OR EXISTS(SELECT 1 FROM pg_catalog.aclexplode(coalesce(item.proacl,pg_catalog.acldefault('f',item.proowner)))
    WHERE grantee<>owner_oid) THEN RAISE EXCEPTION 'trusted fixed-path owner-only functions required'; END IF;
 END LOOP;
 FOREACH signature IN ARRAY runtime_functions LOOP
  EXECUTE pg_catalog.format('GRANT EXECUTE ON FUNCTION %I.%s TO %I',s,signature,runtime_name);
 END LOOP;
 FOREACH signature IN ARRAY operator_functions LOOP
  EXECUTE pg_catalog.format('GRANT EXECUTE ON FUNCTION %I.%s TO %I',s,signature,operator_name);
 END LOOP;
END
$binding$;
