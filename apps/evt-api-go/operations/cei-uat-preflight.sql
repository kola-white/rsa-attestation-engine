-- Metadata and candidate employer IDs only. No DML/DDL or document contents.
BEGIN READ ONLY;
SET LOCAL search_path=pg_catalog;
SET LOCAL statement_timeout='10s';SET LOCAL lock_timeout='3s';
SELECT jsonb_build_object('database',current_database(),'session_role',current_user,
 'server_version',current_setting('server_version'),
 'registry_owner',(SELECT pg_get_userbyid(relowner) FROM pg_class WHERE oid='public.evt_employer_config'::regclass),
 'registry_rls',(SELECT relrowsecurity OR relforcerowsecurity FROM pg_class WHERE oid='public.evt_employer_config'::regclass),
 'exact_id_collision',EXISTS(SELECT 1 FROM public.evt_employer_config WHERE employer_id='cei-uat-sim-001'),
 'candidate_employer_ids',coalesce((SELECT jsonb_agg(c.employer_id ORDER BY c.employer_id) FROM public.evt_employer_config c
  WHERE c.employer_id ILIKE '%cei%' OR c.employer_id ILIKE '%cupertino%' OR EXISTS(
   SELECT 1 FROM public.evt_requests r WHERE r.employer_id=c.employer_id AND
    regexp_replace(lower(coalesce(r.claim_snapshot #>> '{primary_employment,issuer_name}','')),'[^a-z0-9]','','g')='cupertinoelectricinc')),'[]'::jsonb),
 'registry_columns',(SELECT jsonb_agg(jsonb_build_object('name',a.attname,'type',format_type(a.atttypid,a.atttypmod),'not_null',a.attnotnull,
   'default_review',CASE WHEN ad.oid IS NULL THEN 'none' WHEN pg_get_expr(ad.adbin,ad.adrelid) IN ('now()','CURRENT_TIMESTAMP') THEN 'timestamp'
    WHEN a.attname='dispatch_mode' AND pg_get_expr(ad.adbin,ad.adrelid)='''AUTO''::text' THEN 'AUTO text default; explicit MANUAL overrides' ELSE 'UNREVIEWED' END) ORDER BY a.attnum)
  FROM pg_attribute a LEFT JOIN pg_attrdef ad ON ad.adrelid=a.attrelid AND ad.adnum=a.attnum WHERE a.attrelid='public.evt_employer_config'::regclass AND a.attnum>0 AND NOT a.attisdropped),
 'registry_constraints',(SELECT coalesce(jsonb_agg(jsonb_build_object('name',conname,'type',contype,'validated',convalidated)),'[]'::jsonb) FROM pg_constraint WHERE conrelid='public.evt_employer_config'::regclass),
 'user_insert_trigger_count',(SELECT count(*) FROM pg_trigger WHERE tgrelid='public.evt_employer_config'::regclass AND NOT tgisinternal AND (tgtype::integer & 4)=4),
 'rewrite_rule_count',(SELECT count(*) FROM pg_rewrite WHERE ev_class='public.evt_employer_config'::regclass),
 'authority_tables',(SELECT jsonb_agg(jsonb_build_object('name',n.name,'present',c.oid IS NOT NULL,'owner',pg_get_userbyid(c.relowner),'rls',c.relrowsecurity OR c.relforcerowsecurity) ORDER BY n.name)
  FROM (VALUES('authority_authorizations'),('authority_adjudications'),('authority_journal_events'),('authority_recording_boundary'))n(name)
  LEFT JOIN pg_class c ON c.oid=to_regclass(format('public.%I',n.name))),
 'recording_allocator_present',to_regprocedure('public.next_authority_recording_position()') IS NOT NULL,
 'issuer_credentials_table_present',to_regclass('public.evidence_issuer_credentials') IS NOT NULL,
 'records_or_configuration_changed',false,'legal_mapping_verified',false);
ROLLBACK;
