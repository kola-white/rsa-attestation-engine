-- PREPARED ONLY. Requires separate explicit production-write authorization.
-- Run with psql -X -v ON_ERROR_STOP=1; supply private, session-scoped settings
-- cvera.norstar_employer_id, cvera.norstar_mapping_ref, cvera.norstar_approval_ref.
-- The reviewed mapping must confirm no existing ID belongs to this legal entity.
-- This creates ONE registry row, not legal authority, a case grant or a signer.
BEGIN ISOLATION LEVEL READ COMMITTED;
SET LOCAL search_path=public,pg_temp;
SET LOCAL lock_timeout='5s';
SET LOCAL statement_timeout='30s';
DO $registration$
DECLARE employer text:=current_setting('cvera.norstar_employer_id',true);
 mapping text:=current_setting('cvera.norstar_mapping_ref',true);
 approval text:=current_setting('cvera.norstar_approval_ref',true); item record;
BEGIN
 IF coalesce(btrim(employer),'')='' OR coalesce(btrim(mapping),'')='' OR coalesce(btrim(approval),'')='' THEN
  RAISE EXCEPTION 'explicit approved employer ID, mapping and production authorization references required';
 END IF;
 SELECT * INTO STRICT item FROM pg_catalog.pg_class WHERE oid='public.evt_employer_config'::regclass;
 IF item.relowner<>current_user::regrole OR item.relrowsecurity OR item.relforcerowsecurity OR item.relkind<>'r'
 THEN RAISE EXCEPTION 'verified table owner without RLS required';END IF;
 IF EXISTS(SELECT 1 FROM public.evt_employer_config WHERE employer_id=employer) THEN
  RAISE EXCEPTION 'ID already exists: reconcile, do not overwrite or blindly retry';
 END IF;
 IF EXISTS(SELECT 1 FROM pg_catalog.pg_trigger WHERE tgrelid=item.oid AND NOT tgisinternal AND (tgtype::integer & 4)=4)
 OR EXISTS(SELECT 1 FROM pg_catalog.pg_rewrite WHERE ev_class=item.oid) THEN
  RAISE EXCEPTION 'unreviewed registry INSERT trigger or rewrite rule requires inspection';
 END IF;
 -- Existing safe non-dispatching mode; no workflow or entitlement operation.
 INSERT INTO public.evt_employer_config(employer_id,dispatch_mode) VALUES(employer,'MANUAL');
END $registration$;
SELECT jsonb_build_object('registered_employer_id',current_setting('cvera.norstar_employer_id'),
 'legal_entity','NorStar Systems LLC','jurisdiction','California','entity_number','B20250427754',
 'dispatch_mode','MANUAL','transaction_id',pg_current_xact_id()::text,
 'mapping_ref',current_setting('cvera.norstar_mapping_ref'),
 'approval_ref',current_setting('cvera.norstar_approval_ref'),
 'authority_or_credentials_created',false);
COMMIT;
