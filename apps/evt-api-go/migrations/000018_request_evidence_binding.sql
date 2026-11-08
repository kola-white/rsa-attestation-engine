-- Explicit request/evidence context. Requires 000014-000017, not authority schema.
-- Owner approved canonical issuer https://cvera.app on 2026-11-08. No signing or role creation.
DO $migration$
DECLARE s text:=current_schema(); item record; recipient record; name text;
 tables text[]:=ARRAY['request_binding_policy','request_case_bindings','request_binding_checks','request_evidence_review_contexts','request_evidence_manifests'];
 functions text[]:=ARRAY['guard_request_binding','protect_request_binding_records','bind_request_evidence','revoke_request_evidence_binding',
 'request_evidence_context','record_unbound_evidence_review','record_evidence_review','record_request_evidence_review','prepare_request_evidence','request_requires_evidence_issuance'];
BEGIN
 IF s IS NULL OR s IN ('pg_catalog','information_schema') OR s LIKE 'pg_temp%' THEN RAISE EXCEPTION 'trusted schema required'; END IF;
 IF EXISTS(SELECT 1 FROM pg_catalog.pg_class WHERE relnamespace=pg_catalog.to_regnamespace(s) AND relname=ANY(tables))
 OR EXISTS(SELECT 1 FROM pg_catalog.pg_proc WHERE pronamespace=pg_catalog.to_regnamespace(s)
  AND proname=ANY(functions) AND proname<>'record_evidence_review') THEN RAISE EXCEPTION 'binding objects already exist; do not rerun'; END IF;
 IF pg_catalog.to_regprocedure(pg_catalog.format('%I.record_evidence_review(uuid,text,text,uuid,text,text)',s)) IS NULL
 OR pg_catalog.to_regprocedure(pg_catalog.format('%I.check_employer_hr_assignment(uuid,text)',s)) IS NULL
 THEN RAISE EXCEPTION 'existing evidence review/employer authorization required'; END IF;
 FOREACH name IN ARRAY ARRAY['domain_users','evt_requests','evt_employer_config','cases','verification_checks','evidence_reviews'] LOOP
  SELECT * INTO item FROM pg_catalog.pg_class WHERE oid=pg_catalog.to_regclass(pg_catalog.format('%I.%I',s,name));
  IF NOT FOUND OR item.relowner<>current_user::regrole OR item.relrowsecurity OR item.relforcerowsecurity THEN RAISE EXCEPTION 'trusted parent owner/schema required'; END IF;
 END LOOP;
 -- Existing version/claim fields must be established from the real schema, never guessed.
 IF NOT EXISTS(SELECT 1 FROM pg_catalog.pg_attribute WHERE attrelid=pg_catalog.to_regclass(pg_catalog.format('%I.evt_requests',s))
  AND attname='version' AND atttypid IN ('integer'::regtype,'bigint'::regtype) AND NOT attisdropped)
 OR NOT EXISTS(SELECT 1 FROM pg_catalog.pg_attribute WHERE attrelid=pg_catalog.to_regclass(pg_catalog.format('%I.evt_requests',s))
  AND attname='claim_snapshot' AND atttypid='jsonb'::regtype AND NOT attisdropped) THEN RAISE EXCEPTION 'request context DDL must be verified'; END IF;
 EXECUTE pg_catalog.format($definitions$
 CREATE TABLE %1$I.request_binding_policy (
  singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton), issuer_id text NOT NULL CHECK(issuer_id='https://cvera.app'),
  claim_type text NOT NULL DEFAULT 'employment.role' CHECK(claim_type='employment.role'),
  manual_policy text NOT NULL DEFAULT 'MANUAL_REVIEW_PILOT_V1' CHECK(manual_policy='MANUAL_REVIEW_PILOT_V1')
 );
 -- Identity/pilot policy only. This row establishes NO issuer authority.
 INSERT INTO %1$I.request_binding_policy(singleton,issuer_id) VALUES(true,'https://cvera.app');
 CREATE TABLE %1$I.request_case_bindings (
  binding_id uuid PRIMARY KEY DEFAULT pg_catalog.gen_random_uuid(),
  request_id uuid NOT NULL UNIQUE REFERENCES %1$I.evt_requests(request_id) ON DELETE RESTRICT,
  case_id text NOT NULL UNIQUE REFERENCES %1$I.cases(case_id) ON DELETE RESTRICT,
  subject_id uuid NOT NULL REFERENCES %1$I.domain_users(id) ON DELETE RESTRICT,
  employer_id text NOT NULL REFERENCES %1$I.evt_employer_config(employer_id) ON DELETE RESTRICT,
  issuer_id text NOT NULL CHECK(issuer_id='https://cvera.app'), claim_type text NOT NULL CHECK(claim_type='employment.role'),
  request_version bigint NOT NULL, claim_digest text NOT NULL CHECK(claim_digest ~ '^[0-9a-f]{64}$'),
  created_at timestamptz NOT NULL DEFAULT pg_catalog.clock_timestamp(), created_by text NOT NULL CHECK(btrim(created_by)<>''),
  basis_ref text NOT NULL CHECK(btrim(basis_ref)<>''), created_session_user text NOT NULL DEFAULT session_user,
  revoked_at timestamptz,revoked_by text,revocation_reason text,revoked_session_user text,
  UNIQUE(binding_id,case_id),
  CHECK((revoked_at IS NULL AND revoked_by IS NULL AND revocation_reason IS NULL AND revoked_session_user IS NULL)
   OR (revoked_at IS NOT NULL AND revoked_at>=created_at AND revoked_by IS NOT NULL AND btrim(revoked_by)<>'' AND revocation_reason IS NOT NULL
    AND btrim(revocation_reason)<>'' AND revoked_session_user IS NOT NULL))
 );
 CREATE TABLE %1$I.request_binding_checks (
  binding_id uuid NOT NULL, case_id text NOT NULL, check_id text NOT NULL,
  PRIMARY KEY(binding_id,check_id),
  FOREIGN KEY(binding_id,case_id) REFERENCES %1$I.request_case_bindings(binding_id,case_id) ON DELETE RESTRICT,
  FOREIGN KEY(case_id,check_id) REFERENCES %1$I.verification_checks(case_id,check_id) ON DELETE RESTRICT
 );
 CREATE TABLE %1$I.request_evidence_review_contexts (
  review_id uuid PRIMARY KEY REFERENCES %1$I.evidence_reviews(review_id) ON DELETE RESTRICT,
  binding_id uuid NOT NULL REFERENCES %1$I.request_case_bindings(binding_id) ON DELETE RESTRICT,
  claim_digest text NOT NULL CHECK(claim_digest ~ '^[0-9a-f]{64}$')
 );
 CREATE TABLE %1$I.request_evidence_manifests (
  manifest_id uuid PRIMARY KEY DEFAULT pg_catalog.gen_random_uuid(),
  binding_id uuid NOT NULL REFERENCES %1$I.request_case_bindings(binding_id) ON DELETE RESTRICT,
  manifest jsonb NOT NULL, manifest_digest text NOT NULL CHECK(manifest_digest ~ '^[0-9a-f]{64}$'),
  prepared_by uuid NOT NULL REFERENCES %1$I.domain_users(id) ON DELETE RESTRICT,
  prepared_at timestamptz NOT NULL DEFAULT pg_catalog.clock_timestamp(),
  UNIQUE(binding_id,manifest_digest)
 );
 CREATE FUNCTION %1$I.guard_request_binding() RETURNS void LANGUAGE plpgsql VOLATILE SECURITY DEFINER
 SET search_path=pg_catalog,%1$I,pg_temp AS $body$
 BEGIN
  PERFORM %1$I.guard_evidence_review_catalog();
  IF EXISTS(SELECT 1 FROM pg_catalog.pg_class WHERE relnamespace=%2$L::regnamespace
   AND relname IN ('evt_requests','evt_employer_config','employer_hr_assignments','request_binding_policy','request_case_bindings',
    'request_binding_checks','request_evidence_review_contexts','request_evidence_manifests') AND (relrowsecurity OR relforcerowsecurity))
  THEN RAISE EXCEPTION 'binding RLS requires review' USING ERRCODE='55000'; END IF;
 END $body$;
 CREATE FUNCTION %1$I.protect_request_binding_records() RETURNS trigger LANGUAGE plpgsql
 SET search_path=pg_catalog,%1$I,pg_temp AS $body$
 BEGIN
  IF TG_TABLE_NAME='request_case_bindings' THEN
   IF TG_OP='UPDATE' THEN
    IF OLD.revoked_at IS NULL AND NEW.revoked_at IS NOT NULL
     AND (to_jsonb(OLD)-ARRAY['revoked_at','revoked_by','revocation_reason','revoked_session_user'])=
         (to_jsonb(NEW)-ARRAY['revoked_at','revoked_by','revocation_reason','revoked_session_user']) THEN RETURN NEW; END IF;
   END IF;
  END IF;
  RAISE EXCEPTION 'immutable binding evidence' USING ERRCODE='23514';
 END $body$;
 CREATE FUNCTION %1$I.bind_request_evidence(p_request uuid,p_case text,p_checks text[],p_actor text,p_basis text) RETURNS uuid
 LANGUAGE plpgsql VOLATILE SECURITY DEFINER SET search_path=pg_catalog,%1$I,pg_temp AS $body$
 DECLARE r record; policy record; b uuid; value text; count_checks integer;
 BEGIN
  PERFORM %1$I.guard_request_binding();
  IF coalesce(btrim(p_actor),'')='' OR coalesce(btrim(p_basis),'')='' OR coalesce(cardinality(p_checks),0)=0
   OR cardinality(p_checks)>100 OR array_position(p_checks,NULL) IS NOT NULL
   OR (SELECT count(DISTINCT x) FROM unnest(p_checks) x)<>cardinality(p_checks)
  THEN RAISE EXCEPTION 'explicit unique checks and operator attribution required' USING ERRCODE='22023'; END IF;
  SELECT * INTO policy FROM %1$I.request_binding_policy WHERE singleton FOR SHARE;
  IF NOT FOUND THEN RAISE EXCEPTION 'approved issuer policy missing' USING ERRCODE='55000'; END IF;
  SELECT status INTO value FROM %1$I.cases WHERE case_id=p_case FOR UPDATE;
  IF NOT FOUND OR value<>'OPEN' THEN RAISE EXCEPTION 'open case required' USING ERRCODE='23514'; END IF;
  SELECT * INTO r FROM %1$I.evt_requests WHERE request_id=p_request FOR SHARE;
  IF NOT FOUND OR r.status::text<>'ATTESTATION_PENDING' THEN RAISE EXCEPTION 'eligible submitted request required' USING ERRCODE='23514'; END IF;
  IF EXISTS(SELECT 1 FROM %1$I.upload_sessions WHERE case_id=p_case)
   OR EXISTS(SELECT 1 FROM %1$I.evidence_uploads WHERE case_id=p_case)
  THEN RAISE EXCEPTION 'no retroactive evidence adoption' USING ERRCODE='23514'; END IF;
  PERFORM 1 FROM %1$I.verification_checks WHERE case_id=p_case AND check_id=ANY(p_checks) AND status IN ('PENDING','IN_REVIEW') ORDER BY check_id FOR SHARE;
  GET DIAGNOSTICS count_checks=ROW_COUNT;
  IF count_checks<>cardinality(p_checks) THEN RAISE EXCEPTION 'persisted eligible checks required' USING ERRCODE='23514'; END IF;
  INSERT INTO %1$I.request_case_bindings(request_id,case_id,subject_id,employer_id,issuer_id,claim_type,request_version,claim_digest,created_by,basis_ref)
   VALUES(p_request,p_case,r.candidate_personid,r.employer_id,policy.issuer_id,policy.claim_type,r.version::bigint,
    encode(sha256(convert_to(coalesce(r.claim_snapshot,'{}')::jsonb::text,'UTF8')),'hex'),p_actor,p_basis) RETURNING binding_id INTO b;
  INSERT INTO %1$I.request_binding_checks SELECT b,p_case,x FROM unnest(p_checks) x;
  RETURN b;
 END $body$;
 CREATE FUNCTION %1$I.revoke_request_evidence_binding(p_binding uuid,p_actor text,p_reason text) RETURNS void
 LANGUAGE plpgsql VOLATILE SECURITY DEFINER SET search_path=pg_catalog,%1$I,pg_temp AS $body$
 DECLARE c text;
 BEGIN
  PERFORM %1$I.guard_request_binding();
  SELECT case_id INTO c FROM %1$I.request_case_bindings WHERE binding_id=p_binding;
  PERFORM 1 FROM %1$I.cases WHERE case_id=c FOR UPDATE;
  UPDATE %1$I.request_case_bindings SET revoked_at=clock_timestamp(),revoked_by=p_actor,revocation_reason=p_reason,
   revoked_session_user=session_user WHERE binding_id=p_binding AND revoked_at IS NULL;
  IF NOT FOUND THEN RAISE EXCEPTION 'unrevoked binding required' USING ERRCODE='23514'; END IF;
 END $body$;
 CREATE FUNCTION %1$I.request_evidence_context(p_principal uuid,p_case text,p_check text) RETURNS jsonb
 LANGUAGE plpgsql VOLATILE SECURITY DEFINER SET search_path=pg_catalog,%1$I,pg_temp AS $body$
 DECLARE b record; r record;
 BEGIN
  IF %1$I.check_evidence_review(p_principal,p_case,p_check) IS NULL THEN RAISE EXCEPTION 'not found' USING ERRCODE='P0002'; END IF;
  PERFORM %1$I.guard_request_binding();
  SELECT * INTO b FROM %1$I.request_case_bindings WHERE case_id=p_case FOR SHARE;
  IF NOT FOUND THEN RETURN NULL; END IF;
  SELECT * INTO r FROM %1$I.evt_requests WHERE request_id=b.request_id FOR SHARE;
  RETURN jsonb_build_object('bindingId',b.binding_id,'requestId',b.request_id,'subjectId',b.subject_id,'employerId',b.employer_id,
   'issuerId',b.issuer_id,'claimType',b.claim_type,'claimDigest',b.claim_digest,
   'requiredCheck',EXISTS(SELECT 1 FROM %1$I.request_binding_checks WHERE binding_id=b.binding_id AND check_id=p_check),
   'contextValid',b.revoked_at IS NULL AND r.status::text='ATTESTATION_PENDING' AND r.candidate_personid=b.subject_id
    AND r.employer_id=b.employer_id AND r.version::bigint=b.request_version
    AND encode(sha256(convert_to(coalesce(r.claim_snapshot,'{}')::jsonb::text,'UTF8')),'hex')=b.claim_digest,
   'claimSnapshot',CASE WHEN b.revoked_at IS NULL AND r.candidate_personid=b.subject_id AND r.employer_id=b.employer_id
    AND r.version::bigint=b.request_version AND encode(sha256(convert_to(coalesce(r.claim_snapshot,'{}')::jsonb::text,'UTF8')),'hex')=b.claim_digest
    THEN r.claim_snapshot ELSE NULL END);
 END $body$;
 -- Retain the accepted decision implementation as a private owner routine.
 ALTER FUNCTION %1$I.record_evidence_review(uuid,text,text,uuid,text,text) RENAME TO record_unbound_evidence_review;
 CREATE FUNCTION %1$I.record_evidence_review(p_principal uuid,p_case text,p_check text,p_upload uuid,p_decision text,p_reason text) RETURNS jsonb
 LANGUAGE plpgsql VOLATILE SECURITY DEFINER SET search_path=pg_catalog,%1$I,pg_temp AS $body$
 BEGIN
  IF %1$I.check_evidence_review(p_principal,p_case,p_check) IS NULL THEN RAISE EXCEPTION 'not found' USING ERRCODE='P0002'; END IF;
  PERFORM %1$I.guard_request_binding();
  IF EXISTS(SELECT 1 FROM %1$I.request_case_bindings WHERE case_id=p_case) THEN
   RAISE EXCEPTION 'bound context acknowledgement required' USING ERRCODE='22023'; END IF;
  RETURN %1$I.record_unbound_evidence_review(p_principal,p_case,p_check,p_upload,p_decision,p_reason);
 END $body$;
 CREATE FUNCTION %1$I.record_request_evidence_review(p_principal uuid,p_case text,p_check text,p_upload uuid,p_decision text,p_reason text,p_binding uuid,p_digest text) RETURNS jsonb
 LANGUAGE plpgsql VOLATILE SECURITY DEFINER SET search_path=pg_catalog,%1$I,pg_temp AS $body$
 DECLARE context jsonb; result jsonb;
 BEGIN
  context:=%1$I.request_evidence_context(p_principal,p_case,p_check);
  IF context IS NOT NULL AND (context->>'contextValid')::boolean IS NOT TRUE THEN RAISE EXCEPTION 'bound context unavailable' USING ERRCODE='23514'; END IF;
  IF context IS NULL OR p_binding IS NULL OR p_digest IS NULL OR context->>'bindingId'<>p_binding::text OR context->>'claimDigest'<>p_digest THEN
   RAISE EXCEPTION 'bound context mismatch' USING ERRCODE='22023'; END IF;
  IF NOT EXISTS(SELECT 1 FROM %1$I.request_binding_checks WHERE binding_id=p_binding AND check_id=p_check) THEN
   RAISE EXCEPTION 'required check mismatch' USING ERRCODE='22023'; END IF;
  result:=%1$I.record_unbound_evidence_review(p_principal,p_case,p_check,p_upload,p_decision,p_reason);
  INSERT INTO %1$I.request_evidence_review_contexts(review_id,binding_id,claim_digest)
   VALUES((result->>'reviewId')::uuid,p_binding,p_digest) ON CONFLICT(review_id) DO NOTHING;
  IF NOT EXISTS(SELECT 1 FROM %1$I.request_evidence_review_contexts WHERE review_id=(result->>'reviewId')::uuid AND binding_id=p_binding AND claim_digest=p_digest) THEN
   RAISE EXCEPTION 'immutable review context mismatch' USING ERRCODE='23514'; END IF;
  RETURN result;
 END $body$;
 CREATE FUNCTION %1$I.request_requires_evidence_issuance(p_principal uuid,p_request uuid,p_employer text) RETURNS boolean
 LANGUAGE plpgsql VOLATILE SECURITY DEFINER SET search_path=pg_catalog,%1$I,pg_temp AS $body$
 BEGIN
  PERFORM %1$I.guard_request_binding();
  IF %1$I.check_employer_hr_assignment(p_principal,p_employer) IS NOT TRUE THEN RAISE EXCEPTION 'not found' USING ERRCODE='P0002'; END IF;
  PERFORM 1 FROM %1$I.evt_requests WHERE request_id=p_request AND employer_id=p_employer FOR SHARE;
  IF NOT FOUND THEN RAISE EXCEPTION 'not found' USING ERRCODE='P0002'; END IF;
  RETURN EXISTS(SELECT 1 FROM %1$I.request_case_bindings WHERE request_id=p_request);
 END $body$;
 CREATE FUNCTION %1$I.prepare_request_evidence(p_principal uuid,p_request uuid,p_employer text) RETURNS jsonb
 LANGUAGE plpgsql VOLATILE SECURITY DEFINER SET search_path=pg_catalog,%1$I,pg_temp AS $body$
 DECLARE b record; r record; state text; checks jsonb; files jsonb; manifest jsonb; digest text; receipt uuid;
 BEGIN
  PERFORM %1$I.guard_request_binding();
  IF %1$I.check_employer_hr_assignment(p_principal,p_employer) IS NOT TRUE THEN RAISE EXCEPTION 'not found' USING ERRCODE='P0002'; END IF;
  SELECT case_id INTO state FROM %1$I.request_case_bindings WHERE request_id=p_request;
  IF NOT FOUND THEN
   IF NOT EXISTS(SELECT 1 FROM %1$I.evt_requests WHERE request_id=p_request AND employer_id=p_employer) THEN RAISE EXCEPTION 'not found' USING ERRCODE='P0002'; END IF;
   RETURN jsonb_build_object('evidenceReady',false,'reason','REQUEST_NOT_BOUND');
  END IF;
  PERFORM 1 FROM %1$I.cases WHERE case_id=state FOR UPDATE;
  SELECT * INTO b FROM %1$I.request_case_bindings WHERE request_id=p_request FOR SHARE;
  SELECT * INTO r FROM %1$I.evt_requests WHERE request_id=p_request AND employer_id=p_employer FOR SHARE;
  IF NOT FOUND THEN RAISE EXCEPTION 'not found' USING ERRCODE='P0002'; END IF;
  IF b.revoked_at IS NOT NULL THEN RETURN jsonb_build_object('evidenceReady',false,'reason','BINDING_REVOKED'); END IF;
  IF r.status::text IS DISTINCT FROM 'ATTESTATION_PENDING' OR r.candidate_personid IS DISTINCT FROM b.subject_id OR r.employer_id IS DISTINCT FROM b.employer_id
   OR r.version::bigint IS DISTINCT FROM b.request_version OR encode(sha256(convert_to(coalesce(r.claim_snapshot,'{}')::jsonb::text,'UTF8')),'hex') IS DISTINCT FROM b.claim_digest
  THEN RETURN jsonb_build_object('evidenceReady',false,'reason','REQUEST_CONTEXT_CHANGED'); END IF;
  IF NOT EXISTS(SELECT 1 FROM %1$I.request_binding_policy WHERE singleton AND issuer_id=b.issuer_id AND claim_type=b.claim_type)
  THEN RETURN jsonb_build_object('evidenceReady',false,'reason','ISSUER_POLICY_UNAVAILABLE'); END IF;
  IF NOT EXISTS(SELECT 1 FROM %1$I.cases WHERE case_id=b.case_id AND status='OPEN')
  THEN RETURN jsonb_build_object('evidenceReady',false,'reason','CASE_NOT_OPEN'); END IF;
  -- Case lock coordinates normal init/complete/review; parent locks also prevent
  -- concurrent FK insertions. Consistent check/session/upload ordering is explicit.
  PERFORM 1 FROM %1$I.verification_checks v JOIN %1$I.request_binding_checks c USING(case_id,check_id)
   WHERE c.binding_id=b.binding_id ORDER BY v.check_id FOR UPDATE OF v;
  IF NOT EXISTS(SELECT 1 FROM %1$I.request_binding_checks WHERE binding_id=b.binding_id)
   OR EXISTS(SELECT 1 FROM %1$I.request_binding_checks c JOIN %1$I.verification_checks v USING(case_id,check_id)
    WHERE c.binding_id=b.binding_id AND v.status NOT IN ('PENDING','IN_REVIEW'))
  THEN RETURN jsonb_build_object('evidenceReady',false,'reason','CHECK_NOT_ELIGIBLE'); END IF;
  PERFORM 1 FROM %1$I.upload_sessions u JOIN %1$I.request_binding_checks c USING(case_id,check_id)
   WHERE c.binding_id=b.binding_id ORDER BY u.upload_session_id FOR UPDATE OF u;
  PERFORM 1 FROM %1$I.evidence_uploads u JOIN %1$I.request_binding_checks c USING(case_id,check_id)
   WHERE c.binding_id=b.binding_id ORDER BY u.upload_id FOR UPDATE OF u;
  PERFORM 1 FROM %1$I.evidence_scan_jobs j JOIN %1$I.evidence_uploads u USING(upload_id)
   JOIN %1$I.request_binding_checks c USING(case_id,check_id) WHERE c.binding_id=b.binding_id ORDER BY j.upload_id FOR SHARE OF j;
  PERFORM 1 FROM %1$I.evidence_reviews er JOIN %1$I.evidence_uploads u USING(upload_id)
   JOIN %1$I.request_binding_checks c USING(case_id,check_id) WHERE c.binding_id=b.binding_id ORDER BY er.upload_id FOR SHARE OF er;
  IF EXISTS(SELECT 1 FROM %1$I.evidence_uploads u JOIN %1$I.upload_sessions s USING(upload_session_id)
   JOIN %1$I.request_binding_checks c ON c.case_id=u.case_id AND c.check_id=u.check_id
   WHERE c.binding_id=b.binding_id AND u.status='INITIATED' AND s.expires_at>clock_timestamp())
  THEN RETURN jsonb_build_object('evidenceReady',false,'reason','UPLOAD_INCOMPLETE'); END IF;
  IF EXISTS(SELECT 1 FROM %1$I.request_binding_checks c WHERE c.binding_id=b.binding_id AND NOT EXISTS(
   SELECT 1 FROM %1$I.evidence_uploads u WHERE u.case_id=c.case_id AND u.check_id=c.check_id AND u.status='RECEIVED'))
  THEN RETURN jsonb_build_object('evidenceReady',false,'reason','REQUIRED_EVIDENCE_MISSING'); END IF;
  IF EXISTS(SELECT 1 FROM %1$I.evidence_uploads u JOIN %1$I.upload_sessions s USING(upload_session_id)
   JOIN %1$I.request_binding_checks c ON c.case_id=u.case_id AND c.check_id=u.check_id
   LEFT JOIN %1$I.evidence_reviews er USING(upload_id) LEFT JOIN %1$I.request_evidence_review_contexts rc USING(review_id)
   LEFT JOIN %1$I.evidence_scan_jobs j USING(upload_id)
   WHERE c.binding_id=b.binding_id AND u.status='RECEIVED' AND (er.review_id IS NULL OR er.decision<>'APPROVED'
    OR er.reviewer_id=s.principal_id OR rc.binding_id IS DISTINCT FROM b.binding_id OR rc.claim_digest IS DISTINCT FROM b.claim_digest
    OR j.scan_status IS NULL OR j.scan_status NOT IN ('PENDING','CLEAN')
    OR u.staging_key LIKE 'deleted-staging/%%' OR u.sealed_key LIKE 'sealed-evidence/deleted/%%'))
  THEN RETURN jsonb_build_object('evidenceReady',false,'reason','EVIDENCE_NOT_APPROVED'); END IF;
  SELECT jsonb_agg(check_id ORDER BY check_id) INTO checks FROM %1$I.request_binding_checks WHERE binding_id=b.binding_id;
  SELECT jsonb_agg(jsonb_build_object('uploadId',u.upload_id,'checkId',u.check_id,'evidenceHash',u.evidence_hash,
   'reviewId',er.review_id,'reviewerId',er.reviewer_id,'reviewGrantId',er.grant_id,'scanStatus',j.scan_status) ORDER BY u.check_id,u.upload_id)
   INTO files FROM %1$I.evidence_uploads u JOIN %1$I.request_binding_checks c USING(case_id,check_id)
   JOIN %1$I.evidence_reviews er USING(upload_id) JOIN %1$I.evidence_scan_jobs j USING(upload_id)
   WHERE c.binding_id=b.binding_id AND u.status='RECEIVED';
  manifest:=jsonb_build_object('bindingId',b.binding_id,'requestId',b.request_id,'caseId',b.case_id,'subjectId',b.subject_id,
   'employerId',b.employer_id,'issuerId',b.issuer_id,'claimType',b.claim_type,'claimDigest',b.claim_digest,
   'requiredChecks',checks,'evidence',files,'policy','MANUAL_REVIEW_PILOT_V1','purpose','EVIDENCE_PREPARATION_ONLY');
  digest:=encode(sha256(convert_to(manifest::text,'UTF8')),'hex');
  INSERT INTO %1$I.request_evidence_manifests(binding_id,manifest,manifest_digest,prepared_by) VALUES(b.binding_id,manifest,digest,p_principal)
   ON CONFLICT(binding_id,manifest_digest) DO NOTHING;
  SELECT manifest_id INTO receipt FROM %1$I.request_evidence_manifests WHERE binding_id=b.binding_id AND manifest_digest=digest;
  RETURN jsonb_build_object('evidenceReady',true,'reason','REVIEWED_EVIDENCE_PREPARED','manifestId',receipt,'manifestDigest',digest,
   'bindingId',b.binding_id,'issuerId',b.issuer_id,'employerId',b.employer_id,'claimType',b.claim_type,'manualPolicy','MANUAL_REVIEW_PILOT_V1',
   'scanStatuses',(SELECT jsonb_agg(DISTINCT f->>'scanStatus' ORDER BY f->>'scanStatus') FROM jsonb_array_elements(files) f),
   'pendingScanCount',(SELECT count(*) FROM jsonb_array_elements(files) f WHERE f->>'scanStatus'='PENDING'));
 END $body$;

$definitions$,s,s);
 FOREACH name IN ARRAY tables LOOP
  EXECUTE pg_catalog.format('CREATE TRIGGER binding_immutable BEFORE UPDATE OR DELETE ON %I.%I FOR EACH ROW EXECUTE FUNCTION %I.protect_request_binding_records()',s,name,s);
 END LOOP;
 FOR item IN SELECT * FROM pg_catalog.pg_class WHERE relnamespace=pg_catalog.to_regnamespace(s) AND relname=ANY(tables) LOOP
  FOR recipient IN SELECT DISTINCT grantee FROM pg_catalog.aclexplode(coalesce(item.relacl,pg_catalog.acldefault('r',item.relowner))) WHERE grantee<>item.relowner LOOP
   EXECUTE pg_catalog.format('REVOKE ALL ON TABLE %I.%I FROM %s',s,item.relname,CASE WHEN recipient.grantee=0 THEN 'PUBLIC' ELSE pg_catalog.quote_ident(pg_catalog.pg_get_userbyid(recipient.grantee)) END);
  END LOOP;
 END LOOP;
 FOR item IN SELECT * FROM pg_catalog.pg_proc WHERE pronamespace=pg_catalog.to_regnamespace(s) AND proname=ANY(functions) LOOP
  FOR recipient IN SELECT DISTINCT grantee FROM pg_catalog.aclexplode(coalesce(item.proacl,pg_catalog.acldefault('f',item.proowner))) WHERE grantee<>item.proowner LOOP
   EXECUTE pg_catalog.format('REVOKE ALL ON FUNCTION %s FROM %s',item.oid::regprocedure,CASE WHEN recipient.grantee=0 THEN 'PUBLIC' ELSE pg_catalog.quote_ident(pg_catalog.pg_get_userbyid(recipient.grantee)) END);
  END LOOP;
 END LOOP;
END $migration$;
