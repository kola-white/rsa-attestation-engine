-- Claim-bound review integrity. Apply once as the trusted owner after 000019.
-- No context, credential, authority, principal, grant or employer is seeded.
DO $migration$
DECLARE s text:=current_schema(); obj record; acl record; spec record; saved jsonb:='{}'; name text;
BEGIN
 IF s IS NULL OR s IN ('pg_catalog','information_schema') OR s LIKE 'pg_temp%' THEN RAISE EXCEPTION 'trusted schema required'; END IF;
 IF to_regclass(format('%I.request_review_classifications',s)) IS NOT NULL THEN RAISE EXCEPTION '000020 already present'; END IF;
 FOREACH name IN ARRAY ARRAY['request_case_bindings','request_evidence_review_contexts','evidence_reviews','evidence_issued_credentials','evt_requests'] LOOP
  SELECT * INTO obj FROM pg_class WHERE oid=to_regclass(format('%I.%I',s,name));
  IF NOT FOUND OR obj.relowner<>current_user::regrole OR obj.relrowsecurity OR obj.relforcerowsecurity THEN RAISE EXCEPTION 'trusted owner-controlled 000019 schema required'; END IF;
 END LOOP;
 -- Keep original implementations private; wrappers retain only the reviewed
 -- existing EXECUTE recipients. All helper/default/PUBLIC ACLs are stripped.
 FOR spec IN SELECT * FROM (VALUES
  ('request_evidence_context','uuid,text,text'),('prepare_request_evidence','uuid,uuid,text'),
  ('prepare_bound_issuance','uuid,uuid,text,text'),('commit_bound_issuance','uuid,uuid,text,text,text,jsonb'),
  ('resolve_bound_credential','text'),('record_request_evidence_review','uuid,text,text,uuid,text,text,uuid,text')
 ) v(name,args) LOOP
  SELECT * INTO STRICT obj FROM pg_proc WHERE oid=to_regprocedure(format('%I.%I(%s)',s,spec.name,spec.args));
  IF obj.proowner<>current_user::regrole OR NOT obj.prosecdef THEN RAISE EXCEPTION 'trusted existing routine required'; END IF;
  saved:=saved || jsonb_build_object(spec.name,coalesce((SELECT jsonb_agg(jsonb_build_object('grantee',grantee,'grantable',is_grantable))
    FROM aclexplode(coalesce(obj.proacl,acldefault('f',obj.proowner))) WHERE grantee<>obj.proowner),'[]'::jsonb));
  EXECUTE format('ALTER FUNCTION %I.%I(%s) RENAME TO %I',s,spec.name,spec.args,spec.name||'_before_integrity');
 END LOOP;
 EXECUTE format($ddl$
 CREATE TABLE %1$I.request_review_classifications (
  binding_id uuid PRIMARY KEY REFERENCES %1$I.request_case_bindings(binding_id) ON DELETE RESTRICT,
  review_context text NOT NULL CHECK(review_context IN ('SIMULATED_UAT','AUTHORITATIVE')),
  classified_by text NOT NULL CHECK(btrim(classified_by)<>''),basis_ref text NOT NULL CHECK(btrim(basis_ref)<>''),
  classified_at timestamptz NOT NULL DEFAULT clock_timestamp(),classified_session_user text NOT NULL DEFAULT session_user
 );
 CREATE TRIGGER classification_immutable BEFORE UPDATE OR DELETE ON %1$I.request_review_classifications
 FOR EACH ROW EXECUTE FUNCTION %1$I.protect_request_binding_records();
 ALTER TABLE %1$I.request_evidence_review_contexts ADD COLUMN evidence_hash text CHECK(evidence_hash IS NULL OR evidence_hash ~ '^[0-9a-f]{64}$');
 CREATE TRIGGER review_decision_immutable BEFORE UPDATE OR DELETE ON %1$I.evidence_reviews
 FOR EACH ROW EXECUTE FUNCTION %1$I.protect_request_binding_records();
 CREATE FUNCTION %1$I.classify_request_review(p_binding uuid,p_context text,p_actor text,p_basis text) RETURNS void
 LANGUAGE plpgsql VOLATILE SECURITY DEFINER SET search_path=pg_catalog,%1$I,pg_temp AS $fn$
 DECLARE b record; r record; c text;
 BEGIN
  PERFORM %1$I.guard_request_binding();
  IF p_context IS NULL OR p_context NOT IN ('SIMULATED_UAT','AUTHORITATIVE') OR coalesce(btrim(p_actor),'')='' OR coalesce(btrim(p_basis),'')='' THEN RAISE EXCEPTION 'explicit review context and attribution required' USING ERRCODE='22023'; END IF;
  SELECT case_id INTO STRICT c FROM %1$I.request_case_bindings WHERE binding_id=p_binding;
  PERFORM 1 FROM %1$I.cases WHERE case_id=c AND status='OPEN' FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION 'open case required'; END IF;
  SELECT * INTO STRICT b FROM %1$I.request_case_bindings WHERE binding_id=p_binding FOR SHARE;
  SELECT * INTO STRICT r FROM %1$I.evt_requests WHERE request_id=b.request_id FOR SHARE;
  IF b.revoked_at IS NOT NULL OR r.status::text<>'ATTESTATION_PENDING' OR r.candidate_personid<>b.subject_id OR r.employer_id<>b.employer_id
   OR r.version::bigint IS DISTINCT FROM b.request_version OR encode(sha256(convert_to(coalesce(r.claim_snapshot,'{}')::text,'UTF8')),'hex')<>b.claim_digest THEN RAISE EXCEPTION 'stale request context'; END IF;
  IF EXISTS(SELECT 1 FROM %1$I.evidence_reviews er JOIN %1$I.evidence_uploads u USING(upload_id) WHERE u.case_id=b.case_id)
   OR EXISTS(SELECT 1 FROM %1$I.request_evidence_manifests WHERE binding_id=p_binding)
   OR EXISTS(SELECT 1 FROM %1$I.evidence_issued_credentials WHERE binding_id=p_binding) THEN RAISE EXCEPTION 'no retrospective classification'; END IF;
  INSERT INTO %1$I.request_review_classifications(binding_id,review_context,classified_by,basis_ref) VALUES(p_binding,p_context,p_actor,p_basis);
 END $fn$;
 CREATE FUNCTION %1$I.request_evidence_context(p_principal uuid,p_case text,p_check text) RETURNS jsonb
 LANGUAGE plpgsql VOLATILE SECURITY DEFINER SET search_path=pg_catalog,%1$I,pg_temp AS $fn$
 DECLARE result jsonb; mode text; version bigint;
 BEGIN
  result:=%1$I.request_evidence_context_before_integrity(p_principal,p_case,p_check);
  IF result IS NULL THEN RETURN NULL; END IF;
  SELECT b.request_version,coalesce(c.review_context,'UNCLASSIFIED') INTO version,mode FROM %1$I.request_case_bindings b
   LEFT JOIN %1$I.request_review_classifications c USING(binding_id) WHERE b.binding_id=(result->>'bindingId')::uuid;
  RETURN result||jsonb_build_object('reviewContext',mode,'requestVersion',version,
   'reviewerEligible',p_principal::text<>result->>'subjectId' AND mode IN ('SIMULATED_UAT','AUTHORITATIVE'));
 END $fn$;
 -- The old signature is deliberately unable to record a bound decision.
 CREATE FUNCTION %1$I.record_request_evidence_review(p_principal uuid,p_case text,p_check text,p_upload uuid,p_decision text,p_reason text,p_binding uuid,p_digest text) RETURNS jsonb
 LANGUAGE plpgsql VOLATILE SECURITY DEFINER SET search_path=pg_catalog,%1$I,pg_temp AS $fn$
 BEGIN RAISE EXCEPTION 'evidence_digest_ack_required' USING ERRCODE='22023'; END $fn$;
 CREATE FUNCTION %1$I.record_request_evidence_review(p_principal uuid,p_case text,p_check text,p_upload uuid,p_decision text,p_reason text,p_binding uuid,p_digest text,p_evidence_hash text) RETURNS jsonb
 LANGUAGE plpgsql VOLATILE SECURITY DEFINER SET search_path=pg_catalog,%1$I,pg_temp AS $fn$
 DECLARE context jsonb; result jsonb; actual_hash text;
 BEGIN
  context:=%1$I.request_evidence_context(p_principal,p_case,p_check);
  IF context IS NULL OR (context->>'contextValid')::boolean IS NOT TRUE THEN RAISE EXCEPTION 'bound context unavailable' USING ERRCODE='23514'; END IF;
  IF p_principal::text=context->>'subjectId' THEN RAISE EXCEPTION 'claimant review denied' USING ERRCODE='P1001'; END IF;
  IF (context->>'reviewerEligible')::boolean IS NOT TRUE THEN RAISE EXCEPTION 'review context unclassified' USING ERRCODE='23514'; END IF;
  IF p_binding IS NULL OR p_digest IS NULL OR context->>'bindingId'<>p_binding::text OR context->>'claimDigest'<>p_digest OR (context->>'requiredCheck')::boolean IS NOT TRUE THEN RAISE EXCEPTION 'bound context mismatch' USING ERRCODE='22023'; END IF;
  SELECT evidence_hash INTO actual_hash FROM %1$I.evidence_uploads WHERE upload_id=p_upload AND case_id=p_case AND check_id=p_check AND status='RECEIVED' FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION 'not found' USING ERRCODE='P0002'; END IF;
  IF p_evidence_hash IS NULL OR actual_hash IS DISTINCT FROM p_evidence_hash THEN RAISE EXCEPTION 'evidence digest mismatch' USING ERRCODE='22023'; END IF;
  result:=%1$I.record_unbound_evidence_review(p_principal,p_case,p_check,p_upload,p_decision,p_reason);
  INSERT INTO %1$I.request_evidence_review_contexts(review_id,binding_id,claim_digest,evidence_hash)
   VALUES((result->>'reviewId')::uuid,p_binding,p_digest,p_evidence_hash) ON CONFLICT(review_id) DO NOTHING;
  IF NOT EXISTS(SELECT 1 FROM %1$I.request_evidence_review_contexts WHERE review_id=(result->>'reviewId')::uuid AND binding_id=p_binding AND claim_digest=p_digest AND evidence_hash=p_evidence_hash) THEN RAISE EXCEPTION 'immutable review context mismatch' USING ERRCODE='23514'; END IF;
  RETURN result||jsonb_build_object('kind','UNSIGNED_EVIDENCE_REVIEW_RECEIPT','reviewContext',context->>'reviewContext','requestId',context->>'requestId',
   'bindingId',p_binding,'caseId',p_case,'checkId',p_check,'requestVersion',(context->>'requestVersion')::bigint,'claimDigest',p_digest,'evidenceHash',p_evidence_hash);
 END $fn$;
 CREATE FUNCTION %1$I.guard_claim_review_issuance(p_principal uuid,p_request uuid,p_employer text) RETURNS void
 LANGUAGE plpgsql VOLATILE SECURITY DEFINER SET search_path=pg_catalog,%1$I,pg_temp AS $fn$
 DECLARE b record; c text; mode text;
 BEGIN
  PERFORM %1$I.guard_request_binding();
  IF %1$I.check_employer_hr_assignment(p_principal,p_employer) IS NOT TRUE THEN RAISE EXCEPTION 'not found' USING ERRCODE='P0002'; END IF;
  SELECT case_id INTO c FROM %1$I.request_case_bindings WHERE request_id=p_request;
  IF NOT FOUND THEN RAISE EXCEPTION 'request not bound' USING ERRCODE='P0001'; END IF;
  PERFORM 1 FROM %1$I.cases WHERE case_id=c FOR UPDATE;
  SELECT * INTO STRICT b FROM %1$I.request_case_bindings WHERE request_id=p_request AND employer_id=p_employer FOR SHARE;
  IF p_principal=b.subject_id THEN RAISE EXCEPTION 'claimant issuance denied' USING ERRCODE='P0001'; END IF;
  SELECT review_context INTO mode FROM %1$I.request_review_classifications WHERE binding_id=b.binding_id FOR SHARE;
  IF mode IS DISTINCT FROM 'AUTHORITATIVE' THEN RAISE EXCEPTION 'authoritative review context required' USING ERRCODE='P0001'; END IF;
  IF EXISTS(SELECT 1 FROM %1$I.evidence_reviews er JOIN %1$I.evidence_uploads u USING(upload_id) JOIN %1$I.upload_sessions us USING(upload_session_id)
   JOIN %1$I.request_evidence_review_contexts rc USING(review_id) WHERE rc.binding_id=b.binding_id AND er.decision='APPROVED'
   AND (er.reviewer_id=b.subject_id OR er.reviewer_id=us.principal_id OR rc.evidence_hash IS DISTINCT FROM u.evidence_hash)) THEN RAISE EXCEPTION 'independent contextual review required' USING ERRCODE='P0001'; END IF;
 END $fn$;
 CREATE FUNCTION %1$I.prepare_request_evidence(p_principal uuid,p_request uuid,p_employer text) RETURNS jsonb
 LANGUAGE plpgsql VOLATILE SECURITY DEFINER SET search_path=pg_catalog,%1$I,pg_temp AS $fn$
 DECLARE b uuid;mode text;
 BEGIN
  IF %1$I.check_employer_hr_assignment(p_principal,p_employer) IS NOT TRUE THEN RAISE EXCEPTION 'not found' USING ERRCODE='P0002'; END IF;
  PERFORM 1 FROM %1$I.evt_requests WHERE request_id=p_request AND employer_id=p_employer FOR SHARE;
  IF NOT FOUND THEN RAISE EXCEPTION 'not found' USING ERRCODE='P0002'; END IF;
  SELECT binding_id INTO b FROM %1$I.request_case_bindings WHERE request_id=p_request;
  IF NOT FOUND THEN
   IF NOT EXISTS(SELECT 1 FROM %1$I.evt_requests WHERE request_id=p_request AND employer_id=p_employer) THEN RAISE EXCEPTION 'not found' USING ERRCODE='P0002'; END IF;
   RETURN jsonb_build_object('evidenceReady',false,'reason','REQUEST_NOT_BOUND');
  END IF;
  SELECT review_context INTO mode FROM %1$I.request_review_classifications WHERE binding_id=b;
  IF mode IS DISTINCT FROM 'AUTHORITATIVE' THEN RETURN jsonb_build_object('evidenceReady',false,'reason',CASE WHEN mode='SIMULATED_UAT' THEN 'SIMULATED_REVIEW_ONLY' ELSE 'REVIEW_CONTEXT_UNCLASSIFIED' END); END IF;
  PERFORM %1$I.guard_claim_review_issuance(p_principal,p_request,p_employer);
  RETURN %1$I.prepare_request_evidence_before_integrity(p_principal,p_request,p_employer);
 END $fn$;
 CREATE FUNCTION %1$I.prepare_bound_issuance(p_principal uuid,p_request uuid,p_employer text,p_kid text) RETURNS jsonb
 LANGUAGE plpgsql VOLATILE SECURITY DEFINER SET search_path=pg_catalog,%1$I,pg_temp AS $fn$
 BEGIN
  PERFORM %1$I.guard_claim_review_issuance(p_principal,p_request,p_employer);
  RETURN %1$I.prepare_bound_issuance_before_integrity(p_principal,p_request,p_employer,p_kid);
 END $fn$;
 CREATE FUNCTION %1$I.commit_bound_issuance(p_principal uuid,p_request uuid,p_employer text,p_kid text,p_token text,p_claims jsonb) RETURNS void
 LANGUAGE plpgsql VOLATILE SECURITY DEFINER SET search_path=pg_catalog,%1$I,pg_temp AS $fn$
 BEGIN
  PERFORM %1$I.guard_claim_review_issuance(p_principal,p_request,p_employer);
  PERFORM %1$I.commit_bound_issuance_before_integrity(p_principal,p_request,p_employer,p_kid,p_token,p_claims);
 END $fn$;
 CREATE FUNCTION %1$I.resolve_bound_credential(p_digest text) RETURNS jsonb
 LANGUAGE plpgsql VOLATILE SECURITY DEFINER SET search_path=pg_catalog,%1$I,pg_temp AS $fn$
 DECLARE result jsonb;b record;mode text;
 BEGIN
  result:=%1$I.resolve_bound_credential_before_integrity(p_digest);
  SELECT b0.* INTO STRICT b FROM %1$I.request_case_bindings b0 WHERE binding_id=(result->'Claims'->'evidence_binding'->>'binding_id')::uuid;
  SELECT review_context INTO mode FROM %1$I.request_review_classifications WHERE binding_id=b.binding_id;
  -- Existing signed artifacts with no new classification retain their original
  -- resolution. Classification cannot be added retroactively to issued history.
  IF mode='SIMULATED_UAT' OR EXISTS(SELECT 1 FROM %1$I.evidence_issued_credentials WHERE token_sha256=p_digest AND issued_by=b.subject_id)
   OR EXISTS(SELECT 1 FROM %1$I.request_evidence_review_contexts rc JOIN %1$I.evidence_reviews er USING(review_id) WHERE rc.binding_id=b.binding_id AND er.decision='APPROVED' AND er.reviewer_id=b.subject_id)
  THEN result:=result||jsonb_build_object('BindingValid',false); END IF;
  RETURN result;
 END $fn$;
 -- Defense against internal trust-recording routes and direct legacy request
 -- UPDATE privileges: positive state requires a classified, committed issuance.
 CREATE FUNCTION %1$I.protect_claim_review_promotion() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,%1$I,pg_temp AS $fn$
 DECLARE mode text;b uuid; oldrow jsonb:='{}';newrow jsonb:=to_jsonb(NEW);
 BEGIN
  IF TG_OP='UPDATE' THEN oldrow:=to_jsonb(OLD); END IF;
  IF (NEW.status::text IN ('ATTESTED','VERIFIED') AND newrow->>'status' IS DISTINCT FROM oldrow->>'status')
   OR (coalesce(newrow->>'attestation_jws','')<>'' AND newrow->>'attestation_jws' IS DISTINCT FROM oldrow->>'attestation_jws')
   OR (coalesce(newrow->>'evt_token_jws','')<>'' AND newrow->>'evt_token_jws' IS DISTINCT FROM oldrow->>'evt_token_jws')
   OR (coalesce(newrow->>'trust_result','') LIKE 'VERIFIED%%' AND newrow->>'trust_result' IS DISTINCT FROM oldrow->>'trust_result') THEN
   SELECT binding_id INTO b FROM %1$I.request_case_bindings WHERE request_id=NEW.request_id;
   SELECT review_context INTO mode FROM %1$I.request_review_classifications WHERE binding_id=b;
   IF mode IS DISTINCT FROM 'AUTHORITATIVE' OR NOT EXISTS(SELECT 1 FROM %1$I.evidence_issued_credentials i JOIN %1$I.request_case_bindings rb USING(binding_id) WHERE i.request_id=NEW.request_id AND i.binding_id=b
    AND rb.revoked_at IS NULL AND NEW.candidate_personid=rb.subject_id AND NEW.employer_id=rb.employer_id AND NEW.claim_snapshot::text=i.claim_snapshot_text
    AND NEW.version::bigint>=rb.request_version+1 AND i.issued_by<>rb.subject_id
    AND i.token_sha256=encode(sha256(convert_to(coalesce(NEW.attestation_jws,''),'UTF8')),'hex')) THEN RAISE EXCEPTION 'authoritative committed issuance required' USING ERRCODE='23514'; END IF;
  END IF;
  RETURN NEW;
 END $fn$;
 CREATE TRIGGER claim_review_promotion BEFORE INSERT OR UPDATE ON %1$I.evt_requests FOR EACH ROW EXECUTE FUNCTION %1$I.protect_claim_review_promotion();
 CREATE FUNCTION %1$I.protect_claim_review_issuance_insert() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,%1$I,pg_temp AS $fn$
 BEGIN
  PERFORM %1$I.guard_claim_review_issuance(NEW.issued_by,NEW.request_id,(NEW.signed_claims->'evidence_binding'->>'employer_id'));
  RETURN NEW;
 END $fn$;
 CREATE TRIGGER claim_review_issuance_insert BEFORE INSERT ON %1$I.evidence_issued_credentials FOR EACH ROW EXECUTE FUNCTION %1$I.protect_claim_review_issuance_insert();
 $ddl$,s);
 -- Strip hostile default ACLs, including grants on renamed implementation helpers.
 FOR obj IN SELECT * FROM pg_class WHERE oid=to_regclass(format('%I.request_review_classifications',s)) LOOP
  FOR acl IN SELECT DISTINCT grantee FROM aclexplode(coalesce(obj.relacl,acldefault('r',obj.relowner))) WHERE grantee<>obj.relowner LOOP
   EXECUTE format('REVOKE ALL ON TABLE %I.%I FROM %s',s,obj.relname,CASE WHEN acl.grantee=0 THEN 'PUBLIC' ELSE quote_ident(pg_get_userbyid(acl.grantee)) END);
  END LOOP;
 END LOOP;
 FOR obj IN SELECT * FROM pg_proc WHERE pronamespace=s::regnamespace AND (proname LIKE '%%\_before\_integrity' ESCAPE '\'
  OR proname IN ('classify_request_review','request_evidence_context','record_request_evidence_review','guard_claim_review_issuance','prepare_request_evidence','prepare_bound_issuance','commit_bound_issuance','resolve_bound_credential','protect_claim_review_promotion','protect_claim_review_issuance_insert')) LOOP
  FOR acl IN SELECT DISTINCT grantee FROM aclexplode(coalesce(obj.proacl,acldefault('f',obj.proowner))) WHERE grantee<>obj.proowner LOOP
   EXECUTE format('REVOKE ALL ON FUNCTION %s FROM %s',obj.oid::regprocedure,CASE WHEN acl.grantee=0 THEN 'PUBLIC' ELSE quote_ident(pg_get_userbyid(acl.grantee)) END);
  END LOOP;
  IF saved ? obj.proname THEN
   FOR acl IN SELECT (v->>'grantee')::oid grantee,(v->>'grantable')::boolean grantable FROM jsonb_array_elements(saved->obj.proname) v LOOP
    IF acl.grantee=0 THEN RAISE EXCEPTION 'unexpected PUBLIC access to bound workflow'; END IF;
    EXECUTE format('GRANT EXECUTE ON FUNCTION %s TO %I%s',obj.oid::regprocedure,pg_get_userbyid(acl.grantee),CASE WHEN acl.grantable THEN ' WITH GRANT OPTION' ELSE '' END);
   END LOOP;
  END IF;
 END LOOP;
END $migration$;
