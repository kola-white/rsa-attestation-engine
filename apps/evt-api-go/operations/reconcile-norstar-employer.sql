-- Read-only candidate discovery, NOT a verified legal-entity mapping.
-- No personal/identity fields, request IDs, documents, credentials or keys output.
BEGIN READ ONLY;
SET LOCAL search_path=pg_catalog;
SET LOCAL statement_timeout='10s';
SET LOCAL lock_timeout='3s';
WITH candidates AS (
 SELECT c.employer_id,c.dispatch_mode='AUTO' AS auto_dispatch
 FROM public.evt_employer_config c
 WHERE c.employer_id ILIKE '%norstar%'
  OR c.employer_id ILIKE '%B20250427754%'
  OR EXISTS (
   SELECT 1 FROM public.evt_requests r
   WHERE r.employer_id=c.employer_id
    AND regexp_replace(lower(coalesce(r.claim_snapshot #>> '{primary_employment,issuer_name}','')),'[^a-z0-9]','','g')='norstarsystemsllc'
  )
)
SELECT jsonb_build_object(
 'candidate_employers',coalesce((SELECT jsonb_agg(jsonb_build_object('employer_id',employer_id,'auto_dispatch',auto_dispatch) ORDER BY employer_id) FROM candidates),'[]'::jsonb),
 'registry_entries',(SELECT count(*) FROM public.evt_employer_config),
 'legal_mapping_verified',false,
 'records_or_configuration_changed',false
);
ROLLBACK;
