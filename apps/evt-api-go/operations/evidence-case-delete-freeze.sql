-- Privileged, explicitly approved per-case data operation. Not a migration.
-- The operator program sets LOCAL cvera.purge_case/actor/basis/operation.
DO $freeze$
DECLARE s text:=current_schema(); p_case text:=current_setting('cvera.purge_case',true);
 p_actor text:=current_setting('cvera.purge_actor',true); p_basis text:=current_setting('cvera.purge_basis',true);
 p_operation uuid:=current_setting('cvera.purge_operation',true)::uuid;
 item record; name text; value text; marker text;
BEGIN
 IF s IS NULL OR s IN ('pg_catalog','information_schema') OR s LIKE 'pg_temp%'
 OR coalesce(btrim(p_case),'')='' OR coalesce(btrim(p_actor),'')='' OR coalesce(btrim(p_basis),'')='' OR p_operation IS NULL
 THEN RAISE EXCEPTION 'explicit trusted case/actor/basis/operation required'; END IF;
 FOREACH name IN ARRAY ARRAY['cases','case_evidence_upload_grants','case_evidence_review_grants','upload_sessions','evidence_uploads','evidence_scan_jobs','evidence_reviews'] LOOP
  SELECT * INTO item FROM pg_catalog.pg_class WHERE oid=pg_catalog.to_regclass(pg_catalog.format('%I.%I',s,name));
  IF NOT FOUND OR item.relowner<>current_user::regrole OR item.relrowsecurity OR item.relforcerowsecurity THEN
   RAISE EXCEPTION 'verified owner and schema through 000017 required' USING ERRCODE='42501';
  END IF;
 END LOOP;
 EXECUTE pg_catalog.format('SELECT status FROM %I.cases WHERE case_id=$1 FOR UPDATE',s) INTO value USING p_case;
 IF value IS NULL OR value NOT IN ('OPEN','CLOSED') THEN RAISE EXCEPTION 'open/closed case required; inspect prior deletion before retry'; END IF;
 marker:=p_basis||' [purge:'||p_operation::text||']';
 -- The case lock waits for authorized in-flight requests and prevents new grants.
 FOR item IN EXECUTE pg_catalog.format('SELECT grant_id FROM %I.case_evidence_upload_grants WHERE case_id=$1 AND revoked_at IS NULL FOR UPDATE',s) USING p_case LOOP
  EXECUTE pg_catalog.format('SELECT %I.revoke_evidence_upload($1,$2,$3)',s) USING item.grant_id,p_actor,marker;
 END LOOP;
 FOR item IN EXECUTE pg_catalog.format('SELECT grant_id FROM %I.case_evidence_review_grants WHERE case_id=$1 AND revoked_at IS NULL FOR UPDATE',s) USING p_case LOOP
  EXECUTE pg_catalog.format('SELECT %I.revoke_evidence_review($1,$2,$3)',s) USING item.grant_id,p_actor,marker;
 END LOOP;
 EXECUTE pg_catalog.format('SELECT %I.set_evidence_resource_state($1,NULL,$2,$3,$4)',s) USING p_case,'DELETION_SCHEDULED',p_actor,marker;
END
$freeze$;
