-- Reviewed existing evidence operator only. No new roles, runtime access or data.
DO $binding$
DECLARE s text:=current_schema(); op text:=current_setting('cvera.evidence_operator_role',true); r record; owner_oid oid;
BEGIN
 SELECT * INTO r FROM pg_roles WHERE rolname=op;
 SELECT relowner INTO owner_oid FROM pg_class WHERE oid=to_regclass(format('%I.request_review_classifications',s));
 IF NOT FOUND OR r.oid IS NULL OR owner_oid IS NULL OR owner_oid<>current_user::regrole OR r.oid=owner_oid
  OR r.rolsuper OR r.rolcreatedb OR r.rolcreaterole OR r.rolbypassrls OR r.rolreplication
  OR EXISTS(SELECT 1 FROM pg_auth_members WHERE member=r.oid)
  OR has_schema_privilege(r.oid,s,'CREATE') OR has_database_privilege(r.oid,current_database(),'CREATE')
  OR NOT has_function_privilege(r.oid,format('%I.bind_request_evidence(uuid,text,text[],text,text)',s),'EXECUTE') THEN RAISE EXCEPTION 'existing restricted evidence operator required'; END IF;
 EXECUTE format('GRANT EXECUTE ON FUNCTION %I.classify_request_review(uuid,text,text,text) TO %I',s,op);
END $binding$;
