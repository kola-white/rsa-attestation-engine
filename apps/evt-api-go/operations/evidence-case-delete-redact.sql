-- Run only after confirmed scoped object deletion and upload URL expiry.
-- Owner-only per-case privacy redaction; runtime/provisioning role rights unchanged.
DO $redact$
DECLARE s text:=current_schema(); p_case text:=current_setting('cvera.purge_case',true);
 p_actor text:=current_setting('cvera.purge_actor',true); p_basis text:=current_setting('cvera.purge_basis',true);
 p_operation uuid:=current_setting('cvera.purge_operation',true)::uuid;
 p_fingerprint text:=current_setting('cvera.purge_fingerprint',true);
 value text; actual_basis text; due timestamptz; fingerprint text; item record; name text;
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
 EXECUTE pg_catalog.format('SELECT status,update_basis_ref FROM %I.cases WHERE case_id=$1 FOR UPDATE',s) INTO value,actual_basis USING p_case;
 IF value IS DISTINCT FROM 'DELETION_SCHEDULED' OR actual_basis IS DISTINCT FROM p_basis||' [purge:'||p_operation::text||']' THEN
  RAISE EXCEPTION 'frozen case/operation marker mismatch';
 END IF;
 EXECUTE pg_catalog.format('SELECT max(expires_at)+interval ''60 seconds'' FROM %I.upload_sessions WHERE case_id=$1',s) INTO due USING p_case;
 IF due IS NOT NULL AND due>clock_timestamp() THEN RAISE EXCEPTION 'upload URL expiry window still open'; END IF;
 EXECUTE pg_catalog.format('SELECT md5(coalesce(jsonb_agg(to_jsonb(u) ORDER BY u.upload_id)::text,''[]'')) FROM %I.evidence_uploads u WHERE case_id=$1',s) INTO fingerprint USING p_case;
 IF p_fingerprint IS NULL OR fingerprint<>p_fingerprint THEN RAISE EXCEPTION 'metadata changed after frozen deletion plan'; END IF;
 EXECUTE pg_catalog.format('SELECT 1 FROM %I.case_evidence_upload_grants WHERE case_id=$1 AND revoked_at IS NULL UNION ALL SELECT 1 FROM %I.case_evidence_review_grants WHERE case_id=$1 AND revoked_at IS NULL',s,s) INTO value USING p_case;
 IF value IS NOT NULL THEN RAISE EXCEPTION 'active grants remain'; END IF;
 EXECUTE pg_catalog.format('UPDATE %I.evidence_reviews SET reason=''[redacted following authorized evidence deletion]'' WHERE upload_id IN (SELECT upload_id FROM %I.evidence_uploads WHERE case_id=$1)',s,s) USING p_case;
 EXECUTE pg_catalog.format('UPDATE %I.evidence_uploads SET file_name=''[deleted]'',staging_key=''deleted-staging/''||upload_id::text,sealed_key=CASE WHEN status=''RECEIVED'' THEN ''sealed-evidence/deleted/''||upload_id::text ELSE NULL END WHERE case_id=$1',s) USING p_case;
 EXECUTE pg_catalog.format('SELECT %I.set_evidence_resource_state($1,NULL,$2,$3,$4)',s) USING p_case,'DELETED',p_actor,p_basis||' [purge:'||p_operation::text||']';
 -- Keep minimal receipt IDs/hash/size/MIME and review decision/actor/time audit.
 -- No authentication, identity, employment request or attestation data changes.
END
$redact$;
