-- Trusted owner only, same transaction as 000018 plus approved owner policy seed. No role creation or table grants.
-- LOCAL cvera.evidence_runtime_role and cvera.evidence_operator_role are explicit.
DO $binding$
DECLARE s text:=current_schema(); runtime_name text:=current_setting('cvera.evidence_runtime_role',true);
 operator_name text:=current_setting('cvera.evidence_operator_role',true); runtime_oid oid; operator_oid oid;
 owner_oid oid; item record; signature text; name text;
 runtime_functions text[]:=ARRAY['record_evidence_review(uuid,text,text,uuid,text,text)',
  'request_evidence_context(uuid,text,text)','record_request_evidence_review(uuid,text,text,uuid,text,text,uuid,text)',
  'prepare_request_evidence(uuid,uuid,text)','request_requires_evidence_issuance(uuid,uuid,text)'];
 operator_functions text[]:=ARRAY['bind_request_evidence(uuid,text,text[],text,text)','revoke_request_evidence_binding(uuid,text,text)'];
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
 FOREACH name IN ARRAY ARRAY['request_binding_policy','request_case_bindings','request_binding_checks','request_evidence_review_contexts','request_evidence_manifests'] LOOP
  SELECT * INTO item FROM pg_catalog.pg_class WHERE oid=pg_catalog.to_regclass(pg_catalog.format('%I.%I',s,name));
  IF NOT FOUND OR item.relowner<>owner_oid OR item.relrowsecurity OR item.relforcerowsecurity
   OR EXISTS(SELECT 1 FROM pg_catalog.aclexplode(coalesce(item.relacl,pg_catalog.acldefault('r',item.relowner))) WHERE grantee<>owner_oid)
   OR EXISTS(SELECT 1 FROM pg_catalog.pg_attribute WHERE attrelid=item.oid AND attacl IS NOT NULL) THEN
   RAISE EXCEPTION 'owner-only binding tables without RLS required';
  END IF;
 END LOOP;
 FOREACH signature IN ARRAY runtime_functions||operator_functions||ARRAY['guard_request_binding()','record_unbound_evidence_review(uuid,text,text,uuid,text,text)'] LOOP
  SELECT * INTO item FROM pg_catalog.pg_proc WHERE oid=pg_catalog.to_regprocedure(pg_catalog.format('%I.%s',s,signature));
  IF NOT FOUND OR item.proowner<>owner_oid OR NOT item.prosecdef OR item.provolatile<>'v'
   OR NOT coalesce(pg_catalog.format('search_path=pg_catalog, %I, pg_temp',s)=ANY(item.proconfig),false)
   OR EXISTS(SELECT 1 FROM pg_catalog.aclexplode(coalesce(item.proacl,pg_catalog.acldefault('f',item.proowner)))
    WHERE grantee<>owner_oid) THEN RAISE EXCEPTION 'trusted fixed-path owner-only functions required'; END IF;
 END LOOP;
 SELECT * INTO item FROM pg_catalog.pg_proc WHERE oid=pg_catalog.to_regprocedure(pg_catalog.format('%I.protect_request_binding_records()',s));
 IF NOT FOUND OR item.proowner<>owner_oid OR item.prosecdef OR EXISTS(SELECT 1 FROM pg_catalog.aclexplode(coalesce(item.proacl,pg_catalog.acldefault('f',item.proowner))) WHERE grantee<>owner_oid)
 THEN RAISE EXCEPTION 'private invoker immutability trigger required'; END IF;
 FOREACH signature IN ARRAY runtime_functions LOOP
  EXECUTE pg_catalog.format('GRANT EXECUTE ON FUNCTION %I.%s TO %I',s,signature,runtime_name);
 END LOOP;
 FOREACH signature IN ARRAY operator_functions LOOP
  EXECUTE pg_catalog.format('GRANT EXECUTE ON FUNCTION %I.%s TO %I',s,signature,operator_name);
 END LOOP;
END
$binding$;
