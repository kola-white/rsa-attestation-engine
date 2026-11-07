-- New lifecycle persistence; requires 000015. Once-only, owner-executed, atomic.
-- No legacy data is inferred or migrated. Bind runtime privileges separately.
DO $migration$
DECLARE s text := current_schema(); item record; recipient record;
BEGIN
 IF s IS NULL OR s IN ('pg_catalog','information_schema') OR s LIKE 'pg_temp%' THEN
  RAISE EXCEPTION 'explicit trusted application schema required';
 END IF;
 IF pg_catalog.to_regprocedure(pg_catalog.format('%I.check_evidence_upload(uuid,text,text)',s)) IS NULL THEN
  RAISE EXCEPTION 'migration 000015 required';
 END IF;
 EXECUTE pg_catalog.format($ddl$
 CREATE TABLE %1$I.upload_sessions (
  upload_session_id uuid PRIMARY KEY,
  principal_id uuid NOT NULL REFERENCES %1$I.domain_users(id) ON DELETE RESTRICT,
  case_id text NOT NULL, check_id text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT pg_catalog.clock_timestamp(),
  expires_at timestamptz NOT NULL, closed_at timestamptz,
  FOREIGN KEY(case_id,check_id) REFERENCES %1$I.verification_checks(case_id,check_id) ON DELETE RESTRICT,
  UNIQUE(upload_session_id,case_id,check_id),
  CHECK(expires_at>created_at), CHECK(closed_at IS NULL OR closed_at>=created_at)
 );
 CREATE TABLE %1$I.evidence_uploads (
  upload_id uuid PRIMARY KEY,
  upload_session_id uuid NOT NULL,
  case_id text NOT NULL, check_id text NOT NULL,
  staging_key text NOT NULL UNIQUE, file_name text NOT NULL,
  expected_mime_type text NOT NULL CHECK(expected_mime_type IN ('application/pdf','image/jpeg','image/png')),
  expected_size bigint NOT NULL CHECK(expected_size>0 AND expected_size<=5242880),
  status text NOT NULL DEFAULT 'INITIATED' CHECK(status IN ('INITIATED','RECEIVED')),
  sealed_key text UNIQUE, evidence_hash text, completed_at timestamptz,
  FOREIGN KEY(upload_session_id,case_id,check_id) REFERENCES %1$I.upload_sessions(upload_session_id,case_id,check_id) ON DELETE RESTRICT,
  CHECK((status='INITIATED' AND sealed_key IS NULL AND evidence_hash IS NULL AND completed_at IS NULL)
   OR (status='RECEIVED' AND sealed_key IS NOT NULL AND sealed_key<>staging_key AND sealed_key LIKE 'sealed-evidence/%%' AND evidence_hash IS NOT NULL AND evidence_hash ~ '^[0-9a-f]{64}$' AND completed_at IS NOT NULL))
 );
 CREATE INDEX evidence_uploads_session ON %1$I.evidence_uploads(upload_session_id);
 CREATE INDEX upload_sessions_scope ON %1$I.upload_sessions(case_id,check_id);
 CREATE UNIQUE INDEX evidence_uploads_dedup ON %1$I.evidence_uploads(case_id,check_id,evidence_hash) WHERE status='RECEIVED';
 -- A durable pending work record, not a claim that malware scanning has run.
 CREATE TABLE %1$I.evidence_scan_jobs (
  upload_id uuid PRIMARY KEY REFERENCES %1$I.evidence_uploads(upload_id) ON DELETE RESTRICT,
  created_at timestamptz NOT NULL DEFAULT pg_catalog.clock_timestamp(),
  scan_status text NOT NULL DEFAULT 'PENDING' CHECK(scan_status IN ('PENDING','CLEAN','INFECTED','ERROR'))
 );
 $ddl$,s);
 -- Strip inherited table defaults, including the historical evt-app CRUD ACL.
 FOR item IN SELECT c.oid,c.relname,c.relowner,c.relacl FROM pg_catalog.pg_class c
 WHERE c.relnamespace=pg_catalog.to_regnamespace(s) AND c.relname IN ('upload_sessions','evidence_uploads','evidence_scan_jobs') LOOP
  FOR recipient IN SELECT DISTINCT a.grantee FROM pg_catalog.aclexplode(coalesce(item.relacl,pg_catalog.acldefault('r',item.relowner))) a WHERE a.grantee<>item.relowner LOOP
   EXECUTE pg_catalog.format('REVOKE ALL ON TABLE %I.%I FROM %s',s,item.relname,CASE WHEN recipient.grantee=0 THEN 'PUBLIC' ELSE pg_catalog.quote_ident(pg_catalog.pg_get_userbyid(recipient.grantee)) END);
  END LOOP;
 END LOOP;
END
$migration$;
