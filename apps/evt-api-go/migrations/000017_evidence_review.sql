-- Explicit reviewer access and immutable manual decisions. Requires 000016.
-- No implicit grants, case/request associations, CLEAN status or claim issuance.
DO $migration$
DECLARE s text:=current_schema(); item record; recipient record;
BEGIN
 IF s IS NULL OR s IN ('pg_catalog','information_schema') OR s LIKE 'pg_temp%' THEN
  RAISE EXCEPTION 'trusted application schema required';
 END IF;
 IF NOT EXISTS(SELECT 1 FROM pg_catalog.pg_class WHERE oid=pg_catalog.to_regclass(pg_catalog.format('%I.evidence_uploads',s)) AND relowner=current_user::regrole)
 THEN RAISE EXCEPTION 'migration 000016 and its trusted owner required'; END IF;
 IF EXISTS(SELECT 1 FROM pg_catalog.pg_class WHERE relnamespace=pg_catalog.to_regnamespace(s)
  AND relname IN ('case_evidence_review_grants','evidence_reviews'))
 OR EXISTS(SELECT 1 FROM pg_catalog.pg_proc WHERE pronamespace=pg_catalog.to_regnamespace(s)
  AND proname IN ('guard_evidence_review_catalog','check_evidence_review','list_evidence_checks','grant_evidence_review',
   'revoke_evidence_review','list_evidence_for_review','get_evidence_for_review','record_evidence_review')) THEN
  RAISE EXCEPTION 'review objects already exist; verify migration state rather than rerun';
 END IF;
 EXECUTE pg_catalog.format($ddl$
 CREATE TABLE %1$I.case_evidence_review_grants (
  grant_id uuid PRIMARY KEY DEFAULT pg_catalog.gen_random_uuid(),
  principal_id uuid NOT NULL REFERENCES %1$I.domain_users(id) ON DELETE RESTRICT,
  case_id text NOT NULL REFERENCES %1$I.cases(case_id) ON DELETE RESTRICT,
  granted_at timestamptz NOT NULL DEFAULT pg_catalog.clock_timestamp(),
  granted_by text NOT NULL CHECK(btrim(granted_by)<>''),
  grant_basis_ref text NOT NULL CHECK(btrim(grant_basis_ref)<>''),
  granted_session_user text NOT NULL DEFAULT session_user,
  revoked_at timestamptz,revoked_by text,revocation_reason text,revoked_session_user text,
  CHECK((revoked_at IS NULL AND revoked_by IS NULL AND revocation_reason IS NULL AND revoked_session_user IS NULL)
   OR (revoked_at IS NOT NULL AND revoked_at>=granted_at AND revoked_by IS NOT NULL AND btrim(revoked_by)<>''
    AND revocation_reason IS NOT NULL AND btrim(revocation_reason)<>'' AND revoked_session_user IS NOT NULL))
 );
 CREATE UNIQUE INDEX evidence_one_active_review_grant ON %1$I.case_evidence_review_grants(principal_id,case_id) WHERE revoked_at IS NULL;
 CREATE TABLE %1$I.evidence_reviews (
  review_id uuid PRIMARY KEY DEFAULT pg_catalog.gen_random_uuid(),
  upload_id uuid NOT NULL UNIQUE REFERENCES %1$I.evidence_uploads(upload_id) ON DELETE RESTRICT,
  reviewer_id uuid NOT NULL REFERENCES %1$I.domain_users(id) ON DELETE RESTRICT,
  grant_id uuid NOT NULL REFERENCES %1$I.case_evidence_review_grants(grant_id) ON DELETE RESTRICT,
  decision text NOT NULL CHECK(decision IN ('APPROVED','REJECTED')),
  reason text NOT NULL CHECK(btrim(reason)<>'' AND octet_length(reason)<=1000),
  reviewed_at timestamptz NOT NULL DEFAULT pg_catalog.clock_timestamp()
 );
 $ddl$,s);
 EXECUTE pg_catalog.format($definition$
CREATE FUNCTION %1$I.guard_evidence_review_catalog() RETURNS void
LANGUAGE plpgsql VOLATILE SECURITY DEFINER
SET search_path=pg_catalog,%1$I,pg_temp
AS $body$
BEGIN

 IF current_setting('transaction_isolation')<>'read committed' THEN
  RAISE EXCEPTION 'READ COMMITTED required' USING ERRCODE='55000';
 END IF;
 IF EXISTS(SELECT 1 FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace
  WHERE n.nspname=%2$L AND c.relname IN ('domain_users','cases','verification_checks','case_evidence_upload_grants',
   'case_evidence_review_grants','upload_sessions','evidence_uploads','evidence_scan_jobs','evidence_reviews')
  AND (c.relrowsecurity OR c.relforcerowsecurity)) THEN
  RAISE EXCEPTION 'evidence RLS requires review' USING ERRCODE='55000';
 END IF;
END;
$body$;
$definition$,s,s);
 EXECUTE pg_catalog.format($definition$
CREATE FUNCTION %1$I.check_evidence_review(p_principal uuid,p_case text,p_check text) RETURNS uuid
LANGUAGE plpgsql VOLATILE SECURITY DEFINER
SET search_path=pg_catalog,%1$I,pg_temp
AS $body$
DECLARE value text; result uuid;
BEGIN

 PERFORM %1$I.guard_evidence_review_catalog();
 -- Same principal -> case -> check -> grant order as upload authorization.
 SELECT status INTO value FROM %1$I.domain_users WHERE id=p_principal FOR SHARE;
 IF NOT FOUND OR value<>'active' THEN RETURN NULL; END IF;
 SELECT status INTO value FROM %1$I.cases WHERE case_id=p_case FOR SHARE;
 IF NOT FOUND OR value<>'OPEN' THEN RETURN NULL; END IF;
 SELECT status INTO value FROM %1$I.verification_checks WHERE case_id=p_case AND check_id=p_check FOR SHARE;
 IF NOT FOUND OR value NOT IN ('PENDING','IN_REVIEW') THEN RETURN NULL; END IF;
 SELECT grant_id INTO result FROM %1$I.case_evidence_review_grants
  WHERE principal_id=p_principal AND case_id=p_case AND revoked_at IS NULL FOR SHARE;
 RETURN result;
END;
$body$;
$definition$,s,s);
 EXECUTE pg_catalog.format($definition$
CREATE FUNCTION %1$I.list_evidence_checks(p_principal uuid) RETURNS jsonb
LANGUAGE plpgsql VOLATILE SECURITY DEFINER
SET search_path=pg_catalog,%1$I,pg_temp
AS $body$
DECLARE result jsonb;
BEGIN

 PERFORM %1$I.guard_evidence_review_catalog();
 -- Discovery is a bounded snapshot, not a capability: actions reauthorize under locks.
 SELECT coalesce(jsonb_agg(row_to_json(q)::jsonb ORDER BY q."caseId",q."checkId"),'[]'::jsonb) INTO result FROM (
  SELECT c.case_id AS "caseId",v.check_id AS "checkId",
   EXISTS(SELECT 1 FROM %1$I.case_evidence_upload_grants g WHERE g.principal_id=p_principal AND g.case_id=c.case_id AND g.revoked_at IS NULL) AS "canUpload",
   EXISTS(SELECT 1 FROM %1$I.case_evidence_review_grants g WHERE g.principal_id=p_principal AND g.case_id=c.case_id AND g.revoked_at IS NULL) AS "canReview"
  FROM %1$I.cases c JOIN %1$I.verification_checks v ON v.case_id=c.case_id
  WHERE c.status='OPEN' AND v.status IN ('PENDING','IN_REVIEW')
   AND EXISTS(SELECT 1 FROM %1$I.domain_users d WHERE d.id=p_principal AND d.status='active')
   AND (EXISTS(SELECT 1 FROM %1$I.case_evidence_upload_grants g WHERE g.principal_id=p_principal AND g.case_id=c.case_id AND g.revoked_at IS NULL)
     OR EXISTS(SELECT 1 FROM %1$I.case_evidence_review_grants g WHERE g.principal_id=p_principal AND g.case_id=c.case_id AND g.revoked_at IS NULL))
  ORDER BY c.case_id,v.check_id LIMIT 100
 ) q;
 RETURN result;
END;
$body$;
$definition$,s,s);
 EXECUTE pg_catalog.format($definition$
CREATE FUNCTION %1$I.grant_evidence_review(p_principal uuid,p_case text,p_actor text,p_basis text) RETURNS uuid
LANGUAGE plpgsql VOLATILE SECURITY DEFINER
SET search_path=pg_catalog,%1$I,pg_temp
AS $body$
DECLARE value text; result uuid;
BEGIN

 SELECT status INTO value FROM %1$I.domain_users WHERE id=p_principal FOR SHARE;
 IF NOT FOUND OR value<>'active' THEN RAISE EXCEPTION 'active principal required'; END IF;
 SELECT status INTO value FROM %1$I.cases WHERE case_id=p_case FOR SHARE;
 IF NOT FOUND OR value<>'OPEN' THEN RAISE EXCEPTION 'open case required'; END IF;
 INSERT INTO %1$I.case_evidence_review_grants(principal_id,case_id,granted_by,grant_basis_ref)
 VALUES(p_principal,p_case,p_actor,p_basis) RETURNING grant_id INTO result;
 RETURN result;
END;
$body$;
$definition$,s,s);
 EXECUTE pg_catalog.format($definition$
CREATE FUNCTION %1$I.revoke_evidence_review(p_grant uuid,p_actor text,p_reason text) RETURNS void
LANGUAGE plpgsql VOLATILE SECURITY DEFINER
SET search_path=pg_catalog,%1$I,pg_temp
AS $body$
BEGIN

 UPDATE %1$I.case_evidence_review_grants SET revoked_at=clock_timestamp(),revoked_by=p_actor,
  revocation_reason=p_reason,revoked_session_user=session_user WHERE grant_id=p_grant AND revoked_at IS NULL;
 IF NOT FOUND THEN RAISE EXCEPTION 'unrevoked review grant required'; END IF;
END;
$body$;
$definition$,s,s);
 EXECUTE pg_catalog.format($definition$
CREATE FUNCTION %1$I.list_evidence_for_review(p_principal uuid,p_case text,p_check text) RETURNS jsonb
LANGUAGE plpgsql VOLATILE SECURITY DEFINER
SET search_path=pg_catalog,%1$I,pg_temp
AS $body$
DECLARE result jsonb;
BEGIN

 IF %1$I.check_evidence_review(p_principal,p_case,p_check) IS NULL THEN
  RAISE EXCEPTION 'not found' USING ERRCODE='P0002';
 END IF;
 SELECT coalesce(jsonb_agg(jsonb_build_object('uploadId',u.upload_id,'name',u.file_name,
  'mimeType',u.expected_mime_type,'size',u.expected_size,'evidence_hash',u.evidence_hash,'status',u.status,
  'scanStatus',j.scan_status,'reviewStatus',coalesce(r.decision,'PENDING'),'reviewerId',r.reviewer_id,
  'reviewedAt',r.reviewed_at,'reviewReason',r.reason,
  'canReview',r.review_id IS NULL AND s.principal_id<>p_principal,
  'canApprove',r.review_id IS NULL AND s.principal_id<>p_principal AND j.scan_status IN ('PENDING','CLEAN'))
  ORDER BY u.completed_at,u.upload_id),'[]'::jsonb) INTO result
 FROM %1$I.evidence_uploads u JOIN %1$I.upload_sessions s USING(upload_session_id)
 JOIN %1$I.evidence_scan_jobs j USING(upload_id) LEFT JOIN %1$I.evidence_reviews r USING(upload_id)
 WHERE u.case_id=p_case AND u.check_id=p_check AND u.status='RECEIVED';
 RETURN result;
END;
$body$;
$definition$,s,s);
 EXECUTE pg_catalog.format($definition$
CREATE FUNCTION %1$I.get_evidence_for_review(p_principal uuid,p_case text,p_check text,p_upload uuid) RETURNS TABLE(file_name text,mime_type text,file_size bigint,sealed_key text,evidence_hash text,scan_status text)
LANGUAGE plpgsql VOLATILE SECURITY DEFINER
SET search_path=pg_catalog,%1$I,pg_temp
AS $body$
BEGIN

 IF %1$I.check_evidence_review(p_principal,p_case,p_check) IS NULL THEN RETURN; END IF;
 RETURN QUERY SELECT u.file_name,u.expected_mime_type,u.expected_size,u.sealed_key,u.evidence_hash,j.scan_status
 FROM %1$I.evidence_uploads u JOIN %1$I.evidence_scan_jobs j ON j.upload_id=u.upload_id
 WHERE u.upload_id=p_upload AND u.case_id=p_case AND u.check_id=p_check AND u.status='RECEIVED'
  AND j.scan_status IN ('PENDING','CLEAN') FOR SHARE OF u,j;
END;
$body$;
$definition$,s,s);
 EXECUTE pg_catalog.format($definition$
CREATE FUNCTION %1$I.record_evidence_review(p_principal uuid,p_case text,p_check text,p_upload uuid,p_decision text,p_reason text) RETURNS jsonb
LANGUAGE plpgsql VOLATILE SECURITY DEFINER
SET search_path=pg_catalog,%1$I,pg_temp
AS $body$
DECLARE g uuid; uploader uuid; existing record; result record; scan text;
BEGIN

 g:=%1$I.check_evidence_review(p_principal,p_case,p_check);
 IF g IS NULL THEN RAISE EXCEPTION 'not found' USING ERRCODE='P0002'; END IF;
 IF p_decision IS NULL OR p_decision NOT IN ('APPROVED','REJECTED') OR p_reason IS NULL
  OR btrim(p_reason)='' OR octet_length(p_reason)>1000 THEN
  RAISE EXCEPTION 'invalid review input' USING ERRCODE='22023';
 END IF;
 -- Serialize first decisions, including reviewers using different valid grants.
 SELECT s.principal_id INTO uploader FROM %1$I.evidence_uploads u JOIN %1$I.upload_sessions s USING(upload_session_id)
 WHERE u.upload_id=p_upload AND u.case_id=p_case AND u.check_id=p_check AND u.status='RECEIVED' FOR UPDATE OF u;
 IF NOT FOUND THEN RAISE EXCEPTION 'not found' USING ERRCODE='P0002'; END IF;
 IF uploader=p_principal THEN RAISE EXCEPTION 'self review denied' USING ERRCODE='P1001'; END IF;
 SELECT j.scan_status INTO scan FROM %1$I.evidence_scan_jobs j WHERE j.upload_id=p_upload FOR SHARE;
 IF NOT FOUND OR (p_decision='APPROVED' AND scan NOT IN ('PENDING','CLEAN')) THEN
  RAISE EXCEPTION 'unsafe or incomplete evidence' USING ERRCODE='23514';
 END IF;
 SELECT * INTO existing FROM %1$I.evidence_reviews WHERE upload_id=p_upload;
 IF FOUND THEN
  IF existing.reviewer_id<>p_principal OR existing.decision<>p_decision OR existing.reason<>btrim(p_reason) THEN
   RAISE EXCEPTION 'review already recorded' USING ERRCODE='23505';
  END IF;
  result:=existing;
 ELSE
  INSERT INTO %1$I.evidence_reviews(upload_id,reviewer_id,grant_id,decision,reason)
  VALUES(p_upload,p_principal,g,p_decision,btrim(p_reason)) RETURNING * INTO result;
 END IF;
 RETURN jsonb_build_object('reviewId',result.review_id,'uploadId',result.upload_id,'reviewerId',result.reviewer_id,
  'decision',result.decision,'reason',result.reason,'reviewedAt',result.reviewed_at);
 -- Deliberately no scan-status, check-status or employment-attestation mutation.

END;
$body$;
$definition$,s,s);

 FOR item IN SELECT c.* FROM pg_catalog.pg_class c WHERE c.relnamespace=pg_catalog.to_regnamespace(s)
 AND c.relname IN ('case_evidence_review_grants','evidence_reviews') LOOP
  FOR recipient IN SELECT DISTINCT a.grantee FROM pg_catalog.aclexplode(coalesce(item.relacl,pg_catalog.acldefault('r',item.relowner))) a WHERE a.grantee<>item.relowner LOOP
   EXECUTE pg_catalog.format('REVOKE ALL ON TABLE %I.%I FROM %s',s,item.relname,CASE WHEN recipient.grantee=0 THEN 'PUBLIC' ELSE pg_catalog.quote_ident(pg_catalog.pg_get_userbyid(recipient.grantee)) END);
  END LOOP;
 END LOOP;
 FOR item IN SELECT p.* FROM pg_catalog.pg_proc p WHERE p.pronamespace=pg_catalog.to_regnamespace(s)
 AND p.proname IN ('guard_evidence_review_catalog','check_evidence_review','list_evidence_checks','grant_evidence_review','revoke_evidence_review','list_evidence_for_review','get_evidence_for_review','record_evidence_review') LOOP
  FOR recipient IN SELECT DISTINCT a.grantee FROM pg_catalog.aclexplode(coalesce(item.proacl,pg_catalog.acldefault('f',item.proowner))) a WHERE a.grantee<>item.proowner LOOP
   EXECUTE pg_catalog.format('REVOKE ALL ON FUNCTION %s FROM %s',item.oid::regprocedure,CASE WHEN recipient.grantee=0 THEN 'PUBLIC' ELSE pg_catalog.quote_ident(pg_catalog.pg_get_userbyid(recipient.grantee)) END);
  END LOOP;
 END LOOP;
END
$migration$;
