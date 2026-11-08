-- PREPARED ONLY: separate explicit owner authorization for production ACTIVATE.
-- Existing private NorStar settings plus activation_approval_ref,
-- activation_event_id, activation_conditions_satisfied and delegation_valid_from.
-- Actual signed evidence must already have been recorded through APPROVED.
BEGIN ISOLATION LEVEL READ COMMITTED;
SET LOCAL search_path=public,pg_temp;
SET LOCAL lock_timeout='5s';SET LOCAL statement_timeout='30s';
DO $gate$
DECLARE k text;owner_oid oid;BEGIN
 FOREACH k IN ARRAY ARRAY['authorization_id','decision_id','actor_id','evidence_ref','activation_approval_ref','activation_event_id','activation_conditions_satisfied','delegation_valid_from'] LOOP
 IF coalesce(btrim(current_setting('cvera.norstar_'||k,true)),'')='' THEN RAISE EXCEPTION 'actual approved activation input required';END IF;END LOOP;
 SELECT relowner INTO STRICT owner_oid FROM pg_catalog.pg_class WHERE oid='public.authority_recording_boundary'::regclass;
 IF owner_oid<>current_user::regrole OR current_setting('cvera.norstar_activation_conditions_satisfied')<>'true'
 OR current_setting('cvera.norstar_delegation_valid_from')::timestamptz>clock_timestamp() THEN RAISE EXCEPTION 'owner, satisfied conditions and current delegation required';END IF;
 -- Serialize validation AND the append with every accepted authority writer.
 PERFORM 1 FROM public.authority_recording_boundary WHERE singleton FOR UPDATE;
 IF NOT EXISTS(SELECT 1 FROM public.authority_authorizations a JOIN public.authority_adjudications d USING(authorization_id)
 WHERE a.authorization_id=current_setting('cvera.norstar_authorization_id') AND a.issuer_id='https://cvera.app' AND a.authority_subject_id='norstar-systems-llc' AND a.claim_type='employment.role'
 AND d.decision_id=current_setting('cvera.norstar_decision_id') AND d.outcome='APPROVED' AND d.decision_maker_id=current_setting('cvera.norstar_actor_id') AND d.evidence_ref=current_setting('cvera.norstar_evidence_ref'))
 OR (SELECT array_agg(event_type ORDER BY journal_sequence) FROM public.authority_journal_events WHERE authorization_id=current_setting('cvera.norstar_authorization_id')) IS DISTINCT FROM ARRAY['SUBMIT','APPROVE']::text[]
 OR EXISTS(SELECT 1 FROM public.authority_journal_events WHERE authorization_id=current_setting('cvera.norstar_authorization_id') AND effective_at>clock_timestamp())
 OR EXISTS(SELECT 1 FROM public.authority_journal_events WHERE event_id=current_setting('cvera.norstar_activation_event_id')) THEN RAISE EXCEPTION 'not exact approved NorStar history; do not retry';END IF;
END $gate$;
SELECT public.next_authority_recording_position() AS position \gset
INSERT INTO public.authority_journal_events(event_id,authorization_id,event_type,effective_at,recorded_at,actor_id,evidence_ref,conditions_satisfied,recording_position)
VALUES(current_setting('cvera.norstar_activation_event_id'),current_setting('cvera.norstar_authorization_id'),'ACTIVATE',clock_timestamp(),clock_timestamp(),current_setting('cvera.norstar_actor_id'),current_setting('cvera.norstar_evidence_ref'),true,:position);
SELECT jsonb_build_object('authority_activation','RECORDED','issuer','https://cvera.app','employer','norstar-systems-llc','claim_type','employment.role','transaction',txid_current(),'runtime_evaluation','MUST CONFIRM through existing eligibility/verifier; SQL append alone is not an authorization verdict');
COMMIT;
