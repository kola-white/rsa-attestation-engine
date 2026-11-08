-- PREPARED ONLY: separate owner authority-withdrawal authorization required.
-- Reuse actual NorStar authorization_id/actor_id/evidence_ref, and supply
-- containment_event (SUSPEND or REVOKE), containment_event_id, containment_approval_ref.
-- No delete, correction, reset, credential mutation or fabricated historical time.
BEGIN ISOLATION LEVEL READ COMMITTED;
SET LOCAL search_path=public,pg_temp;
SET LOCAL lock_timeout='5s';SET LOCAL statement_timeout='30s';
DO $gate$
DECLARE k text;last_event text;BEGIN
 FOREACH k IN ARRAY ARRAY['authorization_id','actor_id','evidence_ref','containment_event','containment_event_id','containment_approval_ref'] LOOP
 IF coalesce(btrim(current_setting('cvera.norstar_'||k,true)),'')='' THEN RAISE EXCEPTION 'approved attributable withdrawal inputs required';END IF;END LOOP;
 IF (SELECT relowner FROM pg_catalog.pg_class WHERE oid='public.authority_recording_boundary'::regclass)<>current_user::regrole OR current_setting('cvera.norstar_containment_event') NOT IN ('SUSPEND','REVOKE') THEN RAISE EXCEPTION 'trusted owner and bounded containment required';END IF;
 PERFORM 1 FROM public.authority_recording_boundary WHERE singleton FOR UPDATE;
 IF NOT EXISTS(SELECT 1 FROM public.authority_authorizations WHERE authorization_id=current_setting('cvera.norstar_authorization_id') AND issuer_id='https://cvera.app' AND authority_subject_id='norstar-systems-llc' AND claim_type='employment.role') THEN RAISE EXCEPTION 'scope mismatch';END IF;
 SELECT event_type INTO last_event FROM public.authority_journal_events WHERE authorization_id=current_setting('cvera.norstar_authorization_id') ORDER BY journal_sequence DESC LIMIT 1;
 IF coalesce(last_event,'') NOT IN ('ACTIVATE','REINSTATE') AND NOT(coalesce(last_event,'')='SUSPEND' AND current_setting('cvera.norstar_containment_event')='REVOKE') OR EXISTS(SELECT 1 FROM public.authority_journal_events WHERE authorization_id=current_setting('cvera.norstar_authorization_id') AND effective_at>=clock_timestamp()) THEN RAISE EXCEPTION 'unexpected lifecycle state/time; inspect rather than retry';END IF;
END $gate$;
SELECT public.next_authority_recording_position() AS position \gset
INSERT INTO public.authority_journal_events(event_id,authorization_id,event_type,effective_at,recorded_at,actor_id,evidence_ref,conditions_satisfied,recording_position)
VALUES(current_setting('cvera.norstar_containment_event_id'),current_setting('cvera.norstar_authorization_id'),current_setting('cvera.norstar_containment_event'),clock_timestamp(),clock_timestamp(),current_setting('cvera.norstar_actor_id'),current_setting('cvera.norstar_evidence_ref'),true,:position);
SELECT jsonb_build_object('authority_containment','RECORDED','event',current_setting('cvera.norstar_containment_event'),'transaction',txid_current(),'history_preserved',true);
COMMIT;
