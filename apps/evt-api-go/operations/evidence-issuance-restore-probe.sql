-- ISOLATED RESTORED CLONE ONLY. Never invoke against production.
-- Tests existing request defaults/enums/events, actual 000018 review/manifest
-- functions and 000019 receipt persistence. Placeholder public metadata/token
-- are deliberately NOT a signing credential, signature or trusted authority.
BEGIN ISOLATION LEVEL READ COMMITTED;
SET LOCAL search_path=public,pg_temp;
SET LOCAL lock_timeout='5s';SET LOCAL statement_timeout='30s';
DO $probe$
DECLARE subject uuid:=gen_random_uuid();reviewer uuid:=gen_random_uuid();request uuid;
 session_id uuid:=gen_random_uuid();upload uuid:=gen_random_uuid();binding uuid;
 employer text:='restore-'||replace(gen_random_uuid()::text,'-','');
 c text:='restore-'||replace(gen_random_uuid()::text,'-','');context jsonb;prepared jsonb;claims jsonb;
BEGIN
 INSERT INTO public.domain_users(id,kratos_identity_id,role,status) VALUES
 (subject,'restore-'||subject::text,'requestor','active'),(reviewer,'restore-'||reviewer::text,'hr_reviewer','active');
 INSERT INTO public.evt_employer_config(employer_id,dispatch_mode) VALUES(employer,'AUTO');
 INSERT INTO public.employer_hr_assignments(assignment_id,principal_id,employer_id,granted_by,grant_basis_ref)
 VALUES(gen_random_uuid(),reviewer,employer,'restore-only','synthetic metadata');
 -- Normal request SQL must work without inventing production defaults/columns.
 SET LOCAL ROLE cvera_api_runtime;
 INSERT INTO public.evt_requests(candidate_personid,employer_id,status,claim_snapshot)
 VALUES(subject,employer,'DRAFT','{"primary_employment":{"title":"LOCAL RESTORE FIXTURE ONLY"}}') RETURNING request_id INTO request;
 INSERT INTO public.evt_request_events(request_id,actor_role,actor_personid,event_type,from_status,to_status,payload)
 VALUES(request,'candidate',subject,'DRAFT_CREATED',NULL,'DRAFT',jsonb_build_object('employer_id',employer));
 UPDATE public.evt_requests SET status='SUBMITTED',submitted_at=now(),version=version+1 WHERE request_id=request;
 INSERT INTO public.evt_request_events(request_id,actor_role,actor_personid,event_type,from_status,to_status,payload)
 VALUES(request,'candidate',subject,'REQUEST_SUBMITTED','DRAFT','SUBMITTED','{}');
 UPDATE public.evt_requests SET status='ATTESTATION_PENDING',attestation_dispatched_at=now(),version=version+1 WHERE request_id=request AND status='SUBMITTED';
 INSERT INTO public.evt_request_events(request_id,actor_role,event_type,from_status,to_status,payload)
 VALUES(request,'cvera','ATTESTATION_DISPATCHED','SUBMITTED','ATTESTATION_PENDING','{"dispatch_mode":"AUTO"}');
 RESET ROLE;
 PERFORM public.provision_evidence_case(c,'restore-only','synthetic metadata');
 PERFORM public.provision_evidence_check(c,'employment.role','restore-only','synthetic metadata');
 binding:=public.bind_request_evidence(request,c,ARRAY['employment.role'],'restore-only','synthetic metadata');
 PERFORM public.grant_evidence_upload(subject,c,'restore-only','synthetic metadata');
 PERFORM public.grant_evidence_review(reviewer,c,'restore-only','synthetic metadata');
 INSERT INTO public.upload_sessions(upload_session_id,principal_id,case_id,check_id,expires_at)
 VALUES(session_id,subject,c,'employment.role',clock_timestamp()+interval '10 minutes');
 INSERT INTO public.evidence_uploads(upload_id,upload_session_id,case_id,check_id,staging_key,file_name,expected_mime_type,expected_size,status,sealed_key,evidence_hash,completed_at)
 VALUES(upload,session_id,c,'employment.role','cases/'||c||'/checks/employment.role/evidence/'||upload::text||'/fixture.pdf','fixture.pdf','application/pdf',1,'RECEIVED','sealed-evidence/'||c||'/employment.role/'||upload::text||'/'||gen_random_uuid()::text,encode(sha256(convert_to('a','UTF8')),'hex'),clock_timestamp());
 INSERT INTO public.evidence_scan_jobs(upload_id) VALUES(upload);
 INSERT INTO public.evidence_issuer_credentials(issuer_id,kid,public_key_pem,spki_sha256,valid_from,valid_until,claim_lifetime_seconds,established_by,authority_basis_ref)
 VALUES('https://cvera.app','RESTORE-NOT-A-REAL-KEY','-----BEGIN PUBLIC KEY----- RESTORE PLACEHOLDER ONLY',repeat('0',64),clock_timestamp()-interval '1 hour',clock_timestamp()+interval '1 hour',600,'restore-only','not an activated credential');
 SET LOCAL ROLE cvera_api_runtime;
 context:=public.request_evidence_context(reviewer,c,'employment.role');
 PERFORM public.record_request_evidence_review(reviewer,c,'employment.role',upload,'APPROVED','synthetic clone-only independent review',binding,context->>'claimDigest');
 prepared:=public.prepare_bound_issuance(reviewer,request,employer,'RESTORE-NOT-A-REAL-KEY');
 claims:=jsonb_build_object('jti',gen_random_uuid(),'iss','https://cvera.app','sub',subject,'vct','https://cvera.app/credentials/employment-role/v1','claim_type','employment.role','verification_policy','employment.authoritative/manual-evidence-pilot/v1','credential_spki_sha256',repeat('0',64),'exp',floor(extract(epoch FROM (prepared->>'ExpiresAt')::timestamptz))-60,'evidence_binding',(prepared->'Context')||'{"authority_grant_id":"RESTORE-NOT-REAL-AUTHORITY","authority_recording_position":1}');
 PERFORM public.commit_bound_issuance(reviewer,request,employer,'RESTORE-NOT-A-REAL-KEY','RESTORE.INVALID.SIGNATURE~',claims);
 IF NOT EXISTS(SELECT 1 FROM public.evt_requests WHERE request_id=request AND status='ATTESTED' AND attestation_jws='RESTORE.INVALID.SIGNATURE~') THEN RAISE EXCEPTION 'receipt persistence mismatch';END IF;
 RESET ROLE;
END $probe$;
ROLLBACK;
