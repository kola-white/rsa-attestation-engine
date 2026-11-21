-- Read-only disclosure of bound reviews to the active request owner. No data changes.
-- Requires 000021. Existing runtime context EXECUTE recipients are preserved;
-- no reviewer grants, table privileges, roles or signing capabilities are added.
DO $migration$
DECLARE s text:=current_schema(); obj record; acl record; recipients jsonb; name text;
BEGIN
 IF s IS NULL OR s IN ('pg_catalog','information_schema') OR s LIKE 'pg_temp%' THEN RAISE EXCEPTION 'trusted schema required'; END IF;
 IF to_regprocedure(format('%I.record_claim_only_review(uuid,text,text,text,text,uuid,text,bigint)',s)) IS NULL THEN RAISE EXCEPTION '000021 required'; END IF;
 FOREACH name IN ARRAY ARRAY['domain_users','evt_requests','cases','verification_checks','request_case_bindings','request_binding_checks',
  'request_review_classifications','request_evidence_review_contexts','evidence_reviews','evidence_uploads'] LOOP
  SELECT * INTO obj FROM pg_class WHERE oid=to_regclass(format('%I.%I',s,name));
  IF NOT FOUND OR obj.relowner<>current_user::regrole OR obj.relrowsecurity OR obj.relforcerowsecurity THEN RAISE EXCEPTION 'trusted owner-controlled review schema required'; END IF;
 END LOOP;
 SELECT * INTO STRICT obj FROM pg_proc WHERE oid=to_regprocedure(format('%I.request_evidence_context(uuid,text,text)',s));
 IF obj.proowner<>current_user::regrole OR NOT obj.prosecdef THEN RAISE EXCEPTION 'trusted context routine required'; END IF;
 SELECT coalesce(jsonb_agg(jsonb_build_object('grantee',grantee,'grantable',is_grantable)),'[]'::jsonb) INTO recipients
 FROM aclexplode(coalesce(obj.proacl,acldefault('f',obj.proowner))) WHERE grantee<>obj.proowner;
 IF EXISTS(SELECT 1 FROM jsonb_array_elements(recipients) v WHERE (v->>'grantee')::oid=0) THEN RAISE EXCEPTION 'unexpected PUBLIC context access'; END IF;
 EXECUTE format($ddl$
 CREATE FUNCTION %1$I.requestor_review_outcomes(p_principal uuid,p_request uuid) RETURNS jsonb
 LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path=pg_catalog,%1$I,pg_temp AS $fn$
 DECLARE r record; b record; mode text; state text:='NOT_BOUND'; outcomes jsonb:='[]';
 BEGIN
  -- Check ownership and the persisted account, not caller-supplied subject IDs.
  SELECT q.* INTO r FROM %1$I.evt_requests q JOIN %1$I.domain_users d ON d.id=q.candidate_personid
   WHERE q.request_id=p_request AND q.candidate_personid=p_principal AND d.status='active' AND d.role='requestor';
  IF NOT FOUND THEN RAISE EXCEPTION 'not found' USING ERRCODE='P0002'; END IF;
  SELECT * INTO b FROM %1$I.request_case_bindings WHERE request_id=p_request;
  IF FOUND THEN
   SELECT review_context INTO mode FROM %1$I.request_review_classifications WHERE binding_id=b.binding_id;
   IF b.revoked_at IS NOT NULL OR mode IS NULL OR NOT EXISTS(SELECT 1 FROM %1$I.cases WHERE case_id=b.case_id AND status IN ('OPEN','CLOSED')) THEN
    state:='UNAVAILABLE';
   ELSIF b.subject_id<>r.candidate_personid OR b.employer_id<>r.employer_id OR b.request_version<>r.version::bigint
    OR b.claim_digest<>encode(sha256(convert_to(coalesce(r.claim_snapshot,'{}'::jsonb)::text,'UTF8')),'hex') THEN
    -- Never present a review of a different version as a review of this claim.
    state:='STALE';
   ELSE
    state:='CURRENT';
    SELECT coalesce(jsonb_agg(jsonb_build_object(
     'checkId',bc.check_id,'reviewId',er.review_id,'reviewBasis',er.review_basis,
     'decision',coalesce(er.decision,'PENDING'),'reason',er.reason,'reviewedAt',er.reviewed_at
    ) ORDER BY bc.check_id,er.reviewed_at,er.review_id),'[]'::jsonb) INTO outcomes
    FROM %1$I.request_binding_checks bc
    JOIN %1$I.verification_checks vc ON vc.case_id=bc.case_id AND vc.check_id=bc.check_id AND vc.status<>'DELETED'
    LEFT JOIN LATERAL (
     SELECT e.* FROM %1$I.request_evidence_review_contexts rc JOIN %1$I.evidence_reviews e USING(review_id)
     LEFT JOIN %1$I.evidence_uploads u ON u.upload_id=e.upload_id
     WHERE rc.binding_id=b.binding_id AND rc.claim_digest=b.claim_digest AND e.reviewer_id<>b.subject_id
      AND ((e.review_basis='CLAIM_ONLY' AND mode='SIMULATED_UAT' AND e.claim_binding_id=b.binding_id AND e.claim_check_id=bc.check_id
        AND e.upload_id IS NULL AND rc.evidence_hash IS NULL)
       OR (e.review_basis='UPLOADED_EVIDENCE' AND u.case_id=b.case_id AND u.check_id=bc.check_id AND rc.evidence_hash=u.evidence_hash))
    ) er ON true
    WHERE bc.binding_id=b.binding_id AND bc.case_id=b.case_id;
   END IF;
  END IF;
  RETURN jsonb_build_object('requestId',r.request_id,'requestStatus',r.status,'state',state,
   'reviewContext',CASE WHEN state='CURRENT' THEN mode ELSE NULL END,
   'requestVersion',r.version,'outcomes',outcomes);
 END $fn$;
 $ddl$,s);
 -- Strip PUBLIC and hostile inherited default ACLs before granting only the
 -- existing bounded runtime readers access to this ownership-checking routine.
 SELECT * INTO STRICT obj FROM pg_proc WHERE oid=to_regprocedure(format('%I.requestor_review_outcomes(uuid,uuid)',s));
 FOR acl IN SELECT DISTINCT grantee FROM aclexplode(coalesce(obj.proacl,acldefault('f',obj.proowner))) WHERE grantee<>obj.proowner LOOP
  EXECUTE format('REVOKE ALL ON FUNCTION %s FROM %s',obj.oid::regprocedure,CASE WHEN acl.grantee=0 THEN 'PUBLIC' ELSE quote_ident(pg_get_userbyid(acl.grantee)) END);
 END LOOP;
 FOR acl IN SELECT (v->>'grantee')::oid grantee FROM jsonb_array_elements(recipients) v LOOP
  EXECUTE format('GRANT EXECUTE ON FUNCTION %s TO %I',obj.oid::regprocedure,pg_get_userbyid(acl.grantee));
 END LOOP;
END $migration$;
