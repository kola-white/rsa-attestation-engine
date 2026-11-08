-- PREPARED ONLY. Separate production-write approval is required.
-- This positive-decision path records REQUESTED, a real APPROVED adjudication,
-- SUBMIT and APPROVE, then STOPS. There is NO ACTIVATE or signing operation.
-- Prerequisites: verified existing 000002-000013 schema and actual signed
-- organizational delegation plus signed first-party adjudication packet.
-- psql -X -v ON_ERROR_STOP=1; session settings supplied via a private input file:
-- cvera.norstar_employer_id/mapping_ref/approval_ref/authorization_id/decision_id/
-- actor_id/evidence_ref/authority_basis/decided_at/outcome/conflict_disclosure_ref.
-- IDs/references are real approved inputs, not placeholder defaults.
-- Never rerun after an error or ambiguous COMMIT; inspect exact IDs/receipt.

BEGIN ISOLATION LEVEL READ COMMITTED;
SET LOCAL search_path=public,pg_temp;
SET LOCAL lock_timeout='5s';SET LOCAL statement_timeout='30s';
DO $preflight$
DECLARE key text;item record; employer text:=current_setting('cvera.norstar_employer_id',true);
BEGIN
 FOREACH key IN ARRAY ARRAY['employer_id','mapping_ref','approval_ref','authorization_id','decision_id','actor_id','evidence_ref','authority_basis','decided_at','outcome','conflict_disclosure_ref'] LOOP
  IF coalesce(btrim(current_setting('cvera.norstar_'||key,true)),'')='' THEN RAISE EXCEPTION 'complete approved private input required'; END IF;
 END LOOP;
 IF current_setting('cvera.norstar_outcome')<>'APPROVED' THEN RAISE EXCEPTION 'not a signed positive decision; stop without approval or activation';END IF;
 IF current_setting('cvera.norstar_decided_at')::timestamptz>clock_timestamp() THEN RAISE EXCEPTION 'future adjudication time'; END IF;
 IF NOT EXISTS(SELECT 1 FROM public.evt_employer_config WHERE employer_id=employer) THEN RAISE EXCEPTION 'reconciled employer registration required';END IF;
 FOR item IN SELECT * FROM pg_catalog.pg_class WHERE oid IN('public.authority_authorizations'::regclass,'public.authority_adjudications'::regclass,'public.authority_journal_events'::regclass,'public.authority_recording_boundary'::regclass) LOOP
  IF item.relowner<>current_user::regrole OR item.relrowsecurity OR item.relforcerowsecurity THEN RAISE EXCEPTION 'trusted authority owner/schema required';END IF;
 END LOOP;
 IF EXISTS(SELECT 1 FROM public.authority_authorizations WHERE authorization_id=current_setting('cvera.norstar_authorization_id') OR
  (issuer_id='https://cvera.app' AND authority_subject_id=employer AND claim_type='employment.role')) THEN
  RAISE EXCEPTION 'existing authorization scope/ID: inspect history before a new recording attempt';
 END IF;
 IF EXISTS(SELECT 1 FROM public.authority_adjudications WHERE decision_id=current_setting('cvera.norstar_decision_id')) THEN RAISE EXCEPTION 'decision ID already recorded';END IF;
END $preflight$;
SELECT public.next_authority_recording_position() AS position \gset
INSERT INTO public.authority_authorizations(authorization_id,issuer_id,authority_subject_id,claim_type,recording_position)
 VALUES(current_setting('cvera.norstar_authorization_id'),'https://cvera.app',current_setting('cvera.norstar_employer_id'),'employment.role',:position);
COMMIT;

BEGIN ISOLATION LEVEL READ COMMITTED;
SET LOCAL search_path=public,pg_temp;
SET LOCAL lock_timeout='5s';SET LOCAL statement_timeout='30s';
SELECT public.next_authority_recording_position() AS position \gset
INSERT INTO public.authority_adjudications(decision_id,authorization_id,issuer_id,authority_subject_id,claim_type,outcome,decision_maker_id,authority_basis,evidence_ref,decided_at,recorded_at,recording_position)
 VALUES(current_setting('cvera.norstar_decision_id'),current_setting('cvera.norstar_authorization_id'),
 'https://cvera.app',current_setting('cvera.norstar_employer_id'),'employment.role','APPROVED',current_setting('cvera.norstar_actor_id'),
 current_setting('cvera.norstar_authority_basis')||'; first-party/operator-adjudicated; conflict disclosure: '||current_setting('cvera.norstar_conflict_disclosure_ref'),
 current_setting('cvera.norstar_evidence_ref'),current_setting('cvera.norstar_decided_at')::timestamptz,clock_timestamp(),:position);
COMMIT;

BEGIN ISOLATION LEVEL READ COMMITTED;
SET LOCAL search_path=public,pg_temp;
SET LOCAL lock_timeout='5s';SET LOCAL statement_timeout='30s';
SELECT public.next_authority_recording_position() AS position \gset
DO $state$
BEGIN
 IF EXISTS(SELECT 1 FROM public.authority_journal_events WHERE authorization_id=current_setting('cvera.norstar_authorization_id')) THEN RAISE EXCEPTION 'journal already exists; do not resume blindly'; END IF;
END $state$;
INSERT INTO public.authority_journal_events(event_id,authorization_id,event_type,effective_at,recorded_at,actor_id,evidence_ref,conditions_satisfied,recording_position)
 VALUES(gen_random_uuid()::text,current_setting('cvera.norstar_authorization_id'),'SUBMIT',clock_timestamp(),clock_timestamp(),
 current_setting('cvera.norstar_actor_id'),current_setting('cvera.norstar_evidence_ref'),true,:position);
COMMIT;

BEGIN ISOLATION LEVEL READ COMMITTED;
SET LOCAL search_path=public,pg_temp;
SET LOCAL lock_timeout='5s';SET LOCAL statement_timeout='30s';
SELECT public.next_authority_recording_position() AS position \gset
DO $state$
BEGIN
 IF (SELECT count(*) FROM public.authority_journal_events WHERE authorization_id=current_setting('cvera.norstar_authorization_id'))<>1 OR
 NOT EXISTS(SELECT 1 FROM public.authority_journal_events WHERE authorization_id=current_setting('cvera.norstar_authorization_id') AND event_type='SUBMIT') THEN RAISE EXCEPTION 'not pending; stop';END IF;
END $state$;
INSERT INTO public.authority_journal_events(event_id,authorization_id,event_type,effective_at,recorded_at,actor_id,evidence_ref,conditions_satisfied,recording_position)
 SELECT gen_random_uuid()::text,authorization_id,'APPROVE',clock_timestamp(),clock_timestamp(),decision_maker_id,evidence_ref,true,:position
 FROM public.authority_adjudications WHERE decision_id=current_setting('cvera.norstar_decision_id') AND authorization_id=current_setting('cvera.norstar_authorization_id') AND outcome='APPROVED';
COMMIT;

BEGIN READ ONLY;
SET LOCAL search_path=public,pg_temp;
DO $gate$
BEGIN
 IF (SELECT count(*) FROM public.authority_journal_events WHERE authorization_id=current_setting('cvera.norstar_authorization_id'))<>2
 OR EXISTS(SELECT 1 FROM public.authority_journal_events WHERE authorization_id=current_setting('cvera.norstar_authorization_id') AND event_type NOT IN ('SUBMIT','APPROVE'))
 OR NOT EXISTS(SELECT 1 FROM public.authority_journal_events j JOIN public.authority_adjudications d USING(authorization_id)
  WHERE d.decision_id=current_setting('cvera.norstar_decision_id') AND j.event_type='APPROVE' AND j.actor_id=d.decision_maker_id AND j.evidence_ref=d.evidence_ref)
 THEN RAISE EXCEPTION 'activation gate verification failed; inspect, do not retry';END IF;
END $gate$;
SELECT jsonb_build_object('authorization_id',current_setting('cvera.norstar_authorization_id'),
 'authority_subject_id',current_setting('cvera.norstar_employer_id'),'issuer_id','https://cvera.app','claim_type','employment.role',
 'stage','APPROVED_NOT_ACTIVE',
 'activation_event_present',EXISTS(SELECT 1 FROM public.authority_journal_events WHERE authorization_id=current_setting('cvera.norstar_authorization_id') AND event_type='ACTIVATE'),
 'recorded_position',(SELECT committed_position FROM public.authority_recording_boundary WHERE singleton));
ROLLBACK;
