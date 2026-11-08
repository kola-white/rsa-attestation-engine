-- Additive issuance persistence only. Does not activate a credential, establish
-- authority, create a role, or claim that authority migrations were deployed.
-- Run in the approved migration transaction after 000018. Never as runtime.
DO $migration$
DECLARE s text:=pg_catalog.current_schema(); item record; recipient record; name text;
 tables text[]:=ARRAY['evidence_issuer_credentials','evidence_issued_credentials'];
 functions text[]:=ARRAY['active_evidence_signing_credential','guard_evidence_issuance','read_evidence_authority','prepare_bound_issuance','commit_bound_issuance','resolve_bound_credential','revoke_bound_credential','protect_evidence_issuance'];
BEGIN
 IF s IS NULL OR s IN ('pg_catalog','information_schema') OR s LIKE 'pg_temp%' THEN RAISE EXCEPTION 'trusted schema required'; END IF;
 IF EXISTS(SELECT 1 FROM pg_catalog.pg_class WHERE relnamespace=pg_catalog.to_regnamespace(s) AND relname=ANY(tables)) THEN RAISE EXCEPTION '000019 already present'; END IF;
 IF NOT EXISTS(SELECT 1 FROM pg_catalog.pg_class WHERE relnamespace=pg_catalog.to_regnamespace(s) AND relname='request_case_bindings' AND relowner=(SELECT oid FROM pg_catalog.pg_roles WHERE rolname=current_user) AND NOT relrowsecurity AND NOT relforcerowsecurity) THEN RAISE EXCEPTION 'owner-controlled 000018 required'; END IF;
 EXECUTE pg_catalog.format($ddl$
 CREATE TABLE %1$I.evidence_issuer_credentials(
  issuer_id text NOT NULL CHECK(issuer_id='https://cvera.app'), kid text NOT NULL CHECK(kid<>''),
  public_key_pem text NOT NULL CHECK(public_key_pem LIKE '-----BEGIN PUBLIC KEY-----%%'),
  spki_sha256 text NOT NULL CHECK(spki_sha256 ~ '^[0-9a-f]{64}$'),
  valid_from timestamptz NOT NULL, valid_until timestamptz NOT NULL,
  claim_lifetime_seconds integer NOT NULL CHECK(claim_lifetime_seconds>0 AND claim_lifetime_seconds<=31536000),
  established_by text NOT NULL CHECK(established_by<>''), authority_basis_ref text NOT NULL CHECK(authority_basis_ref<>''),
  established_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  revoked_at timestamptz,revoked_by text,revocation_reason text,
  PRIMARY KEY(issuer_id,kid),CHECK(valid_until>valid_from),
  CHECK((revoked_at IS NULL AND revoked_by IS NULL AND revocation_reason IS NULL) OR
   (revoked_at IS NOT NULL AND coalesce(revoked_by,'')<>'' AND coalesce(revocation_reason,'')<>''))
 );
 CREATE TABLE %1$I.evidence_issued_credentials(
  credential_id uuid PRIMARY KEY,request_id uuid NOT NULL UNIQUE REFERENCES %1$I.evt_requests(request_id),
  binding_id uuid NOT NULL REFERENCES %1$I.request_case_bindings(binding_id),
  manifest_id uuid NOT NULL REFERENCES %1$I.request_evidence_manifests(manifest_id),
  issuer_id text NOT NULL,kid text NOT NULL, FOREIGN KEY(issuer_id,kid) REFERENCES %1$I.evidence_issuer_credentials(issuer_id,kid),
  token_sha256 text NOT NULL UNIQUE CHECK(token_sha256 ~ '^[0-9a-f]{64}$'),
  signed_claims jsonb NOT NULL,claim_snapshot_text text NOT NULL,manifest_text text NOT NULL,
  issued_by uuid NOT NULL REFERENCES %1$I.domain_users(id),issued_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  revoked_at timestamptz,revoked_by text,revocation_reason text,
  CHECK((revoked_at IS NULL AND revoked_by IS NULL AND revocation_reason IS NULL) OR
   (revoked_at IS NOT NULL AND coalesce(revoked_by,'')<>'' AND coalesce(revocation_reason,'')<>''))
 );
 CREATE FUNCTION %1$I.guard_evidence_issuance() RETURNS void LANGUAGE plpgsql VOLATILE SECURITY DEFINER SET search_path=pg_catalog,%1$I,pg_temp AS $fn$
 BEGIN
  PERFORM %1$I.guard_request_binding();
  IF EXISTS(SELECT 1 FROM pg_catalog.pg_class WHERE relnamespace='%1$I'::regnamespace AND relname IN ('evidence_issuer_credentials','evidence_issued_credentials') AND (relrowsecurity OR relforcerowsecurity OR relowner<>(SELECT oid FROM pg_catalog.pg_roles WHERE rolname=current_user))) THEN RAISE EXCEPTION 'unsafe issuance relation'; END IF;
 END $fn$;
 CREATE FUNCTION %1$I.protect_evidence_issuance() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog,%1$I,pg_temp AS $fn$
 BEGIN
  IF TG_OP='UPDATE' THEN
   IF OLD.revoked_at IS NULL AND NEW.revoked_at IS NOT NULL AND coalesce(NEW.revoked_by,'')<>'' AND coalesce(NEW.revocation_reason,'')<>'' AND
    (to_jsonb(NEW)-'revoked_at'-'revoked_by'-'revocation_reason')=(to_jsonb(OLD)-'revoked_at'-'revoked_by'-'revocation_reason') THEN RETURN NEW; END IF;
  END IF;
  RAISE EXCEPTION 'immutable issuance record' USING ERRCODE='42501';
 END $fn$;
 CREATE FUNCTION %1$I.active_evidence_signing_credential(p_kid text) RETURNS jsonb LANGUAGE plpgsql VOLATILE SECURITY DEFINER SET search_path=pg_catalog,%1$I,pg_temp AS $fn$
 DECLARE k record;stamp timestamptz;
 BEGIN
  PERFORM %1$I.guard_evidence_issuance();
  SELECT * INTO k FROM %1$I.evidence_issuer_credentials WHERE issuer_id='https://cvera.app' AND kid=p_kid FOR SHARE;
  IF NOT FOUND THEN RETURN NULL; END IF;
  stamp:=clock_timestamp();IF k.revoked_at IS NOT NULL OR stamp<k.valid_from OR stamp>=k.valid_until THEN RETURN NULL; END IF;
  RETURN jsonb_build_object('Issuer',k.issuer_id,'KID',k.kid,'Fingerprint',k.spki_sha256,'PublicPEM',k.public_key_pem,'ValidFrom',k.valid_from,'ValidUntil',k.valid_until,'Revoked',false);
 END $fn$;
 CREATE FUNCTION %1$I.read_evidence_authority() RETURNS jsonb LANGUAGE plpgsql VOLATILE SECURITY DEFINER SET search_path=pg_catalog,%1$I,pg_temp AS $fn$
 DECLARE position bigint; observed timestamptz; counts record; result jsonb;
 BEGIN
  PERFORM %1$I.guard_evidence_issuance();
  IF (SELECT count(*) FROM pg_catalog.pg_class WHERE relnamespace='%1$I'::regnamespace AND relname IN ('authority_authorizations','authority_adjudications','authority_journal_events','authority_recording_boundary') AND relowner=(SELECT oid FROM pg_catalog.pg_roles WHERE rolname=current_user) AND NOT relrowsecurity AND NOT relforcerowsecurity)<>4 THEN RAISE EXCEPTION 'authority source unavailable'; END IF;
  SELECT committed_position INTO STRICT position FROM %1$I.authority_recording_boundary WHERE singleton FOR SHARE;
  SELECT count(*) n,count(DISTINCT p) d,coalesce(min(p),0) lo,coalesce(max(p),0) hi INTO counts FROM(
   SELECT recording_position p FROM %1$I.authority_authorizations UNION ALL
   SELECT recording_position FROM %1$I.authority_adjudications UNION ALL SELECT recording_position FROM %1$I.authority_journal_events) q;
  IF position<0 OR counts.n<>position OR counts.d<>position OR (position>0 AND(counts.lo<>1 OR counts.hi<>position)) OR (position=0 AND(counts.lo<>0 OR counts.hi<>0)) THEN RAISE EXCEPTION 'incomplete authority source'; END IF;
  IF EXISTS(SELECT 1 FROM %1$I.authority_authorizations a WHERE (SELECT count(*) FROM %1$I.authority_adjudications d WHERE d.authorization_id=a.authorization_id)<>1) THEN RAISE EXCEPTION 'ambiguous authority adjudication'; END IF;
  observed:=clock_timestamp();
  SELECT coalesce(jsonb_agg(jsonb_build_object('Record',jsonb_build_object(
   'AuthorizationID',a.authorization_id,'IssuerID',a.issuer_id,'AuthoritySubjectID',a.authority_subject_id,'ClaimType',a.claim_type,
   'Adjudication',jsonb_build_object('DecisionID',d.decision_id,'AuthorizationID',d.authorization_id,'IssuerID',d.issuer_id,'AuthoritySubjectID',d.authority_subject_id,'ClaimType',d.claim_type,'Outcome',d.outcome,'DecisionMakerID',d.decision_maker_id,'AuthorityBasis',d.authority_basis,'EvidenceRef',d.evidence_ref,'DecidedAt',d.decided_at,'RecordedAt',d.recorded_at)),
   'Events',coalesce((SELECT jsonb_agg(jsonb_build_object('EventID',j.event_id,'AuthorizationID',j.authorization_id,'EventType',j.event_type,'EffectiveAt',j.effective_at,'RecordedAt',j.recorded_at,'ActorID',j.actor_id,'EvidenceRef',j.evidence_ref,'ConditionsSatisfied',j.conditions_satisfied,'CorrectsEventID',coalesce(j.corrects_event_id,''),'CorrectionReason',coalesce(j.correction_reason,''),'CorrectedField',coalesce(j.corrected_field,'')) ORDER BY j.journal_sequence) FROM %1$I.authority_journal_events j WHERE j.authorization_id=a.authorization_id),'[]'::jsonb)) ORDER BY a.authorization_id),'[]'::jsonb) INTO result
   FROM %1$I.authority_authorizations a JOIN %1$I.authority_adjudications d USING(authorization_id);
  RETURN jsonb_build_object('RecordingPosition',position,'RecordedThrough',observed,'Entries',result);
 END $fn$;
 CREATE FUNCTION %1$I.prepare_bound_issuance(p_principal uuid,p_request uuid,p_employer text,p_kid text) RETURNS jsonb LANGUAGE plpgsql VOLATILE SECURITY DEFINER SET search_path=pg_catalog,%1$I,pg_temp AS $fn$
 DECLARE preparation jsonb; b record;m record;k record;r record; stamp timestamptz;
 BEGIN
  PERFORM %1$I.guard_evidence_issuance();
  preparation:=%1$I.prepare_request_evidence(p_principal,p_request,p_employer);
  IF (preparation->>'evidenceReady')::boolean IS NOT TRUE THEN RAISE EXCEPTION 'evidence ineligible' USING ERRCODE='P0001'; END IF;
  SELECT * INTO STRICT b FROM %1$I.request_case_bindings WHERE request_id=p_request FOR SHARE;
  SELECT * INTO STRICT r FROM %1$I.evt_requests WHERE request_id=p_request FOR SHARE;
  SELECT * INTO STRICT m FROM %1$I.request_evidence_manifests WHERE manifest_id=(preparation->>'manifestId')::uuid FOR SHARE;
  SELECT * INTO k FROM %1$I.evidence_issuer_credentials WHERE issuer_id=b.issuer_id AND kid=p_kid FOR SHARE;
  IF NOT FOUND THEN RAISE EXCEPTION 'issuer credential unavailable' USING ERRCODE='55000'; END IF;
  stamp:=clock_timestamp();
  IF k.revoked_at IS NOT NULL OR stamp<k.valid_from OR stamp>=k.valid_until THEN RAISE EXCEPTION 'credential not active'; END IF;
  RETURN jsonb_build_object('Context',jsonb_build_object('request_id',b.request_id,'binding_id',b.binding_id,'case_id',b.case_id,'employer_id',b.employer_id,'subject_id',b.subject_id,'request_version',b.request_version,'snapshot_sha256',b.claim_digest,'manifest_id',m.manifest_id,'manifest_sha256',m.manifest_digest,'manual_policy','MANUAL_REVIEW_PILOT_V1'),
   'Snapshot',r.claim_snapshot::text,'Manifest',m.manifest::text,'Now',stamp,'ExpiresAt',least(k.valid_until,stamp+make_interval(secs=>k.claim_lifetime_seconds)),
   'Key',jsonb_build_object('Issuer',k.issuer_id,'KID',k.kid,'Fingerprint',k.spki_sha256,'PublicPEM',k.public_key_pem,'ValidFrom',k.valid_from,'ValidUntil',k.valid_until,'Revoked',false));
 END $fn$;
 CREATE FUNCTION %1$I.commit_bound_issuance(p_principal uuid,p_request uuid,p_employer text,p_kid text,p_token text,p_claims jsonb) RETURNS void LANGUAGE plpgsql VOLATILE SECURITY DEFINER SET search_path=pg_catalog,%1$I,pg_temp AS $fn$
 DECLARE prepared jsonb;context jsonb;
 BEGIN
  prepared:=%1$I.prepare_bound_issuance(p_principal,p_request,p_employer,p_kid);context:=prepared->'Context';
  IF p_claims->>'iss' IS DISTINCT FROM 'https://cvera.app' OR p_claims->>'sub' IS DISTINCT FROM context->>'subject_id' OR
   p_claims->>'vct' IS DISTINCT FROM 'https://cvera.app/credentials/employment-role/v1' OR p_claims->>'claim_type' IS DISTINCT FROM 'employment.role' OR
   p_claims->>'verification_policy' IS DISTINCT FROM 'employment.authoritative/manual-evidence-pilot/v1' OR
   p_claims->>'credential_spki_sha256' IS DISTINCT FROM prepared->'Key'->>'Fingerprint' OR
   ((p_claims->'evidence_binding')-'authority_grant_id'-'authority_recording_position') IS DISTINCT FROM context OR
   coalesce(p_claims->'evidence_binding'->>'authority_grant_id','')='' OR coalesce((p_claims->'evidence_binding'->>'authority_recording_position')::bigint,0)<=0 OR
   coalesce((p_claims->>'exp')::numeric,0)<=extract(epoch FROM clock_timestamp()) OR (p_claims->>'exp')::numeric>extract(epoch FROM (prepared->>'ExpiresAt')::timestamptz) OR
   p_token IS NULL OR length(p_token)>1048576 OR right(p_token,1)<>'~' THEN RAISE EXCEPTION 'issuance context mismatch'; END IF;
  INSERT INTO %1$I.evidence_issued_credentials(credential_id,request_id,binding_id,manifest_id,issuer_id,kid,token_sha256,signed_claims,claim_snapshot_text,manifest_text,issued_by)
   VALUES((p_claims->>'jti')::uuid,p_request,(context->>'binding_id')::uuid,(context->>'manifest_id')::uuid,'https://cvera.app',p_kid,encode(sha256(convert_to(p_token,'UTF8')),'hex'),p_claims,prepared->>'Snapshot',prepared->>'Manifest',p_principal);
  UPDATE %1$I.evt_requests SET status='ATTESTED',employer_response_type='FULL_MATCH',employer_response=jsonb_build_object('evidence_credential',p_claims->>'jti'),attestation_jws=p_token,attested_at=clock_timestamp(),version=version+1 WHERE request_id=p_request;
  INSERT INTO %1$I.evt_request_events(request_id,actor_role,actor_personid,event_type,from_status,to_status,payload) VALUES(p_request,'employer_hr',p_principal,'HR_ATTESTED','ATTESTATION_PENDING','ATTESTED',jsonb_build_object('credential_id',p_claims->>'jti','manifest_id',context->>'manifest_id','profile','sd-jwt-vc'));
 END $fn$;
 CREATE FUNCTION %1$I.resolve_bound_credential(p_digest text) RETURNS jsonb LANGUAGE plpgsql VOLATILE SECURITY DEFINER SET search_path=pg_catalog,%1$I,pg_temp AS $fn$
 DECLARE receipt record;b record;r record;k record; valid boolean;
 BEGIN
  PERFORM %1$I.guard_evidence_issuance();
  SELECT * INTO STRICT receipt FROM %1$I.evidence_issued_credentials WHERE token_sha256=p_digest;
  SELECT case_id INTO STRICT b FROM %1$I.request_case_bindings WHERE binding_id=receipt.binding_id;
  PERFORM 1 FROM %1$I.cases WHERE case_id=b.case_id FOR UPDATE;
  SELECT * INTO STRICT b FROM %1$I.request_case_bindings WHERE binding_id=receipt.binding_id FOR SHARE;
  SELECT * INTO STRICT r FROM %1$I.evt_requests WHERE request_id=receipt.request_id FOR SHARE;
  SELECT * INTO STRICT receipt FROM %1$I.evidence_issued_credentials WHERE token_sha256=p_digest FOR SHARE;
  SELECT * INTO STRICT k FROM %1$I.evidence_issuer_credentials WHERE issuer_id=receipt.issuer_id AND kid=receipt.kid FOR SHARE;
  PERFORM 1 FROM %1$I.verification_checks WHERE case_id=b.case_id ORDER BY check_id FOR UPDATE;
  PERFORM 1 FROM %1$I.upload_sessions WHERE case_id=b.case_id ORDER BY upload_session_id FOR UPDATE;
  PERFORM 1 FROM %1$I.evidence_uploads WHERE case_id=b.case_id ORDER BY upload_id FOR UPDATE;
  PERFORM 1 FROM %1$I.evidence_scan_jobs j JOIN %1$I.evidence_uploads u USING(upload_id) WHERE u.case_id=b.case_id ORDER BY j.upload_id FOR SHARE OF j;
  valid:=b.revoked_at IS NULL AND r.status::text IN ('ATTESTED','VERIFIED','CLOSED') AND r.candidate_personid=b.subject_id AND r.employer_id=b.employer_id AND r.version::bigint>=b.request_version+1
   AND r.claim_snapshot::text=receipt.claim_snapshot_text 
   AND encode(sha256(convert_to(coalesce(r.attestation_jws,''),'UTF8')),'hex')=receipt.token_sha256
   AND (receipt.manifest_text::jsonb->'requiredChecks') IS NOT DISTINCT FROM (SELECT jsonb_agg(check_id ORDER BY check_id) FROM %1$I.request_binding_checks WHERE binding_id=b.binding_id)
   AND EXISTS(SELECT 1 FROM %1$I.cases WHERE case_id=b.case_id AND status='OPEN')
   AND NOT EXISTS(SELECT 1 FROM %1$I.request_binding_checks c JOIN %1$I.verification_checks v USING(case_id,check_id) WHERE c.binding_id=b.binding_id AND v.status NOT IN ('PENDING','IN_REVIEW'))
   AND NOT EXISTS(SELECT 1 FROM %1$I.evidence_uploads u JOIN %1$I.upload_sessions us USING(upload_session_id) WHERE u.case_id=b.case_id AND u.check_id IN(SELECT check_id FROM %1$I.request_binding_checks WHERE binding_id=b.binding_id) AND u.status='INITIATED' AND us.expires_at>clock_timestamp())
   AND NOT EXISTS(SELECT 1 FROM %1$I.evidence_uploads u LEFT JOIN %1$I.evidence_scan_jobs j USING(upload_id) WHERE u.case_id=b.case_id AND u.check_id IN(SELECT check_id FROM %1$I.request_binding_checks WHERE binding_id=b.binding_id) AND u.status='RECEIVED' AND (j.scan_status IS NULL OR j.scan_status NOT IN ('PENDING','CLEAN') OR u.staging_key LIKE 'deleted-staging/%%' OR u.sealed_key LIKE 'sealed-evidence/deleted/%%' OR NOT EXISTS(SELECT 1 FROM jsonb_array_elements(receipt.manifest_text::jsonb->'evidence') e WHERE (e->>'uploadId')::uuid=u.upload_id AND e->>'evidenceHash'=u.evidence_hash)))
   AND NOT EXISTS(SELECT 1 FROM jsonb_array_elements(receipt.manifest_text::jsonb->'evidence') e WHERE NOT EXISTS(
    SELECT 1 FROM %1$I.evidence_uploads u JOIN %1$I.upload_sessions us USING(upload_session_id) JOIN %1$I.evidence_reviews er USING(upload_id) JOIN %1$I.request_evidence_review_contexts rc USING(review_id) JOIN %1$I.evidence_scan_jobs j USING(upload_id)
    WHERE u.upload_id=(e->>'uploadId')::uuid AND u.case_id=b.case_id AND u.check_id=e->>'checkId' AND u.evidence_hash=e->>'evidenceHash' AND u.status='RECEIVED' AND er.review_id=(e->>'reviewId')::uuid AND er.reviewer_id=(e->>'reviewerId')::uuid AND er.grant_id=(e->>'reviewGrantId')::uuid AND er.decision='APPROVED' AND er.reviewer_id<>us.principal_id AND rc.binding_id=b.binding_id AND rc.claim_digest=b.claim_digest AND j.scan_status IN ('PENDING','CLEAN') AND u.staging_key NOT LIKE 'deleted-staging/%%' AND u.sealed_key NOT LIKE 'sealed-evidence/deleted/%%'));
  RETURN jsonb_build_object('Claims',receipt.signed_claims,'TokenDigest',receipt.token_sha256,'Snapshot',receipt.claim_snapshot_text,'Manifest',receipt.manifest_text,'KnownStatus',true,'ClaimRevoked',receipt.revoked_at IS NOT NULL,'BindingValid',coalesce(valid,false),
   'Key',jsonb_build_object('Issuer',k.issuer_id,'KID',k.kid,'Fingerprint',k.spki_sha256,'PublicPEM',k.public_key_pem,'ValidFrom',k.valid_from,'ValidUntil',k.valid_until,'Revoked',k.revoked_at IS NOT NULL));
 END $fn$;
 CREATE FUNCTION %1$I.revoke_bound_credential(p_id uuid,p_actor text,p_reason text) RETURNS void LANGUAGE plpgsql VOLATILE SECURITY DEFINER SET search_path=pg_catalog,%1$I,pg_temp AS $fn$
 BEGIN
  PERFORM %1$I.guard_evidence_issuance();
  IF coalesce(p_actor,'')='' OR coalesce(p_reason,'')='' THEN RAISE EXCEPTION 'attribution required'; END IF;
  UPDATE %1$I.evidence_issued_credentials SET revoked_at=clock_timestamp(),revoked_by=p_actor,revocation_reason=p_reason WHERE credential_id=p_id AND revoked_at IS NULL;
  IF NOT FOUND THEN RAISE EXCEPTION 'not found' USING ERRCODE='P0002'; END IF;
 END $fn$;
 $ddl$,s);
 FOREACH name IN ARRAY tables LOOP
  EXECUTE pg_catalog.format('CREATE TRIGGER issuance_immutable BEFORE UPDATE OR DELETE ON %I.%I FOR EACH ROW EXECUTE FUNCTION %I.protect_evidence_issuance()',s,name,s);
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
