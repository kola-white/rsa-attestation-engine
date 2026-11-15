-- Additive claim-only simulated review. Apply transactionally as owner after 000020.
-- No request, binding, classification, grant or credential is created or changed.
DO $migration$
DECLARE s text:=current_schema(); obj record; acl record; recipients jsonb; name text;
BEGIN
 IF s IS NULL OR s IN ('pg_catalog','information_schema') OR s LIKE 'pg_temp%' THEN RAISE EXCEPTION 'trusted schema required'; END IF;
 IF EXISTS(SELECT 1 FROM pg_attribute WHERE attrelid=to_regclass(format('%I.evidence_reviews',s)) AND attname='review_basis' AND NOT attisdropped) THEN RAISE EXCEPTION '000021 already present'; END IF;
 FOREACH name IN ARRAY ARRAY['evidence_reviews','request_evidence_review_contexts','request_review_classifications','request_binding_checks'] LOOP
  SELECT * INTO obj FROM pg_class WHERE oid=to_regclass(format('%I.%I',s,name));
  IF NOT FOUND OR obj.relowner<>current_user::regrole OR obj.relrowsecurity OR obj.relforcerowsecurity THEN RAISE EXCEPTION 'trusted owner-controlled 000020 schema required'; END IF;
 END LOOP;
 SELECT * INTO STRICT obj FROM pg_proc WHERE oid=to_regprocedure(format('%I.record_request_evidence_review(uuid,text,text,uuid,text,text,uuid,text,text)',s));
 IF obj.proowner<>current_user::regrole OR NOT obj.prosecdef THEN RAISE EXCEPTION 'trusted review routine required'; END IF;
 SELECT coalesce(jsonb_agg(jsonb_build_object('grantee',grantee,'grantable',is_grantable)),'[]'::jsonb) INTO recipients
 FROM aclexplode(coalesce(obj.proacl,acldefault('f',obj.proowner))) WHERE grantee<>obj.proowner;
 IF EXISTS(SELECT 1 FROM jsonb_array_elements(recipients) v WHERE (v->>'grantee')::oid=0) THEN RAISE EXCEPTION 'unexpected PUBLIC review access'; END IF;
 EXECUTE format($ddl$
 ALTER TABLE %1$I.evidence_reviews ALTER COLUMN upload_id DROP NOT NULL;
 ALTER TABLE %1$I.evidence_reviews ADD COLUMN review_basis text NOT NULL DEFAULT 'UPLOADED_EVIDENCE',
  ADD COLUMN claim_binding_id uuid, ADD COLUMN claim_check_id text,
  ADD CONSTRAINT review_basis_scope CHECK(
   (review_basis='UPLOADED_EVIDENCE' AND upload_id IS NOT NULL AND claim_binding_id IS NULL AND claim_check_id IS NULL)
   OR (review_basis='CLAIM_ONLY' AND upload_id IS NULL AND claim_binding_id IS NOT NULL AND claim_check_id IS NOT NULL)),
  ADD CONSTRAINT claim_review_check FOREIGN KEY(claim_binding_id,claim_check_id)
   REFERENCES %1$I.request_binding_checks(binding_id,check_id) ON DELETE RESTRICT;
 CREATE UNIQUE INDEX claim_review_once ON %1$I.evidence_reviews(claim_binding_id,claim_check_id) WHERE review_basis='CLAIM_ONLY';
 -- A claim-only decision cannot commit without its immutable digest commitment.
 CREATE FUNCTION %1$I.protect_claim_only_receipt() RETURNS trigger
 LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,%1$I,pg_temp AS $fn$
 BEGIN
  IF NEW.review_basis='CLAIM_ONLY' AND NOT EXISTS(
   SELECT 1 FROM %1$I.request_evidence_review_contexts rc JOIN %1$I.request_case_bindings b USING(binding_id)
   JOIN %1$I.request_review_classifications c USING(binding_id)
   WHERE rc.review_id=NEW.review_id AND rc.binding_id=NEW.claim_binding_id AND rc.claim_digest=b.claim_digest
    AND rc.evidence_hash IS NULL AND c.review_context='SIMULATED_UAT' AND NEW.reviewer_id<>b.subject_id)
  THEN RAISE EXCEPTION 'immutable simulated claim receipt required' USING ERRCODE='23514'; END IF;
  RETURN NEW;
 END $fn$;
 CREATE CONSTRAINT TRIGGER claim_only_receipt_required AFTER INSERT ON %1$I.evidence_reviews
 DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION %1$I.protect_claim_only_receipt();
 CREATE OR REPLACE FUNCTION %1$I.request_evidence_context(p_principal uuid,p_case text,p_check text) RETURNS jsonb
 LANGUAGE plpgsql VOLATILE SECURITY DEFINER SET search_path=pg_catalog,%1$I,pg_temp AS $fn$
 DECLARE result jsonb; mode text; version bigint; receipt jsonb;
 BEGIN
  result:=%1$I.request_evidence_context_before_integrity(p_principal,p_case,p_check);
  IF result IS NULL THEN RETURN NULL; END IF;
  SELECT b.request_version,coalesce(c.review_context,'UNCLASSIFIED') INTO version,mode FROM %1$I.request_case_bindings b
   LEFT JOIN %1$I.request_review_classifications c USING(binding_id) WHERE b.binding_id=(result->>'bindingId')::uuid;
  SELECT jsonb_build_object('kind','UNSIGNED_EVIDENCE_REVIEW_RECEIPT','reviewBasis',er.review_basis,'reviewContext',mode,
   'requestId',result->>'requestId','bindingId',er.claim_binding_id,'caseId',p_case,'checkId',p_check,'requestVersion',version,
   'claimDigest',rc.claim_digest,'evidenceHash',NULL,'uploadId',NULL,'reviewId',er.review_id,
   'reviewerId',er.reviewer_id,'decision',er.decision,'reason',er.reason,'reviewedAt',er.reviewed_at) INTO receipt
   FROM %1$I.evidence_reviews er JOIN %1$I.request_evidence_review_contexts rc USING(review_id)
   WHERE er.claim_binding_id=(result->>'bindingId')::uuid AND er.claim_check_id=p_check AND er.review_basis='CLAIM_ONLY';
  RETURN result||jsonb_build_object('reviewContext',mode,'requestVersion',version,
   'reviewerEligible',p_principal::text<>result->>'subjectId' AND mode IN ('SIMULATED_UAT','AUTHORITATIVE'),
   -- Explicit narrow backend policy: immutable simulated contexts only, never authoritative issuance.
   'claimOnlyAllowed',mode='SIMULATED_UAT' AND p_principal::text<>result->>'subjectId'
    AND (result->>'contextValid')::boolean AND (result->>'requiredCheck')::boolean,
   'claimOnlyReview',receipt);
 END $fn$;
 CREATE FUNCTION %1$I.record_claim_only_review(p_principal uuid,p_case text,p_check text,p_decision text,p_reason text,p_binding uuid,p_digest text,p_version bigint) RETURNS jsonb
 LANGUAGE plpgsql VOLATILE SECURITY DEFINER SET search_path=pg_catalog,%1$I,pg_temp AS $fn$
 DECLARE context jsonb; g uuid; existing record; receipt_id uuid;
 BEGIN
  -- Match existing principal -> case -> check -> grant lock ordering, serialize decisions on the case.
  PERFORM 1 FROM %1$I.domain_users WHERE id=p_principal FOR SHARE;
  PERFORM 1 FROM %1$I.cases WHERE case_id=p_case FOR UPDATE;
  g:=%1$I.check_evidence_review(p_principal,p_case,p_check);
  IF g IS NULL THEN RAISE EXCEPTION 'not found' USING ERRCODE='P0002'; END IF;
  context:=%1$I.request_evidence_context(p_principal,p_case,p_check);
  IF context IS NULL OR (context->>'contextValid')::boolean IS NOT TRUE THEN RAISE EXCEPTION 'bound context unavailable' USING ERRCODE='23514'; END IF;
  IF p_principal::text=context->>'subjectId' THEN RAISE EXCEPTION 'claimant review denied' USING ERRCODE='P1001'; END IF;
  IF (context->>'claimOnlyAllowed')::boolean IS NOT TRUE THEN RAISE EXCEPTION 'claim-only review not permitted' USING ERRCODE='23514'; END IF;
  IF p_binding IS NULL OR p_digest IS NULL OR p_version IS NULL OR context->>'bindingId'<>p_binding::text
   OR context->>'claimDigest'<>p_digest OR (context->>'requestVersion')::bigint<>p_version THEN RAISE EXCEPTION 'bound context mismatch' USING ERRCODE='22023'; END IF;
  IF p_decision IS NULL OR p_decision NOT IN ('APPROVED','REJECTED') OR p_reason IS NULL OR btrim(p_reason)='' OR octet_length(p_reason)>1000 THEN RAISE EXCEPTION 'invalid review input' USING ERRCODE='22023'; END IF;
  SELECT * INTO existing FROM %1$I.evidence_reviews WHERE claim_binding_id=p_binding AND claim_check_id=p_check AND review_basis='CLAIM_ONLY';
  IF FOUND THEN
   IF existing.reviewer_id<>p_principal OR existing.decision<>p_decision OR existing.reason<>btrim(p_reason) THEN RAISE EXCEPTION 'review already recorded' USING ERRCODE='23505'; END IF;
  ELSE
   INSERT INTO %1$I.evidence_reviews(review_basis,claim_binding_id,claim_check_id,reviewer_id,grant_id,decision,reason)
    VALUES('CLAIM_ONLY',p_binding,p_check,p_principal,g,p_decision,btrim(p_reason)) RETURNING review_id INTO receipt_id;
   INSERT INTO %1$I.request_evidence_review_contexts(review_id,binding_id,claim_digest) VALUES(receipt_id,p_binding,p_digest);
  END IF;
  context:=%1$I.request_evidence_context(p_principal,p_case,p_check);
  RETURN context->'claimOnlyReview';
 END $fn$;
 $ddl$,s);
 -- Strip hostile default EXECUTE ACLs. Preserve the existing context API ACL;
 -- new mutation receives exactly the existing bound-review EXECUTE recipients.
 FOR obj IN SELECT * FROM pg_proc WHERE pronamespace=s::regnamespace AND proname IN ('protect_claim_only_receipt','record_claim_only_review') LOOP
  FOR acl IN SELECT DISTINCT grantee FROM aclexplode(coalesce(obj.proacl,acldefault('f',obj.proowner))) WHERE grantee<>obj.proowner LOOP
   EXECUTE format('REVOKE ALL ON FUNCTION %s FROM %s',obj.oid::regprocedure,CASE WHEN acl.grantee=0 THEN 'PUBLIC' ELSE quote_ident(pg_get_userbyid(acl.grantee)) END);
  END LOOP;
  IF obj.proname='record_claim_only_review' THEN
   FOR acl IN SELECT (v->>'grantee')::oid grantee,(v->>'grantable')::boolean grantable FROM jsonb_array_elements(recipients) v LOOP
    EXECUTE format('GRANT EXECUTE ON FUNCTION %s TO %I%s',obj.oid::regprocedure,pg_get_userbyid(acl.grantee),CASE WHEN acl.grantable THEN ' WITH GRANT OPTION' ELSE '' END);
   END LOOP;
  END IF;
 END LOOP;
END $migration$;
