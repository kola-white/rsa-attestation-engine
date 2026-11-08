#!/usr/bin/env python3
"""Approved request-evidence-binding SQL phase in the existing owner console.

Back up/restore-test evt, retain API recovery, verify parent schema and apply only
000018 with narrow runtime bindings. Never change roles, credentials or services.
All raw SQL/Docker errors stay private; STOP is not authorization to retry.
"""
import datetime, hashlib, json, os, shutil, stat, subprocess, sys, time
import urllib.parse, urllib.request
from pathlib import Path

ROOT=Path('/opt/cvera/rsa-attestation-engine')
DB='evt-api-go-db-1'
PHASE='preflight'
BASE='6e7293e75bf0577c1ce9105744249c718fe65361'
SOURCE='143644d2623a7d6b9ea08dd49380ff7c7e12cbb1'
SETTINGS="SET LOCAL search_path=public,pg_temp; SET LOCAL lock_timeout='5s'; SET LOCAL statement_timeout='60s';"
FILES={'migrations/000018_request_evidence_binding.sql': 'f9f4ac2c45b8efbd2dbbe00a9b53713f524d14df1d83b58fe7ba31fd03ea0760', 'operations/request-evidence-binding-privileges.sql': '6617b2de9f0b3c24ff0798a5f00ec35116c8bf6554d7b1e4079fce362e61764a'}

def run(args, data=None, timeout=120):
 r=subprocess.run(args,input=data,capture_output=True,timeout=timeout)
 if r.returncode: raise RuntimeError('command failed; raw output suppressed')
 return r.stdout


def info(name,field):
 return json.loads(run(['docker','inspect','--format','{{json .'+field+'}}',name]))


def sql(body):
 return run(['docker','exec','-i',DB,'psql','-X','-U','postgres','-d','evt','-Atq','-v','ON_ERROR_STOP=1'],body.encode())


def private(path,data):
 fd=os.open(path,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
 with os.fdopen(fd,'wb') as stream:
  stream.write(data);stream.flush();os.fsync(stream.fileno())


PREFLIGHT="""
DO $$ DECLARE name text; item record; BEGIN
 IF current_database()<>'evt' OR current_user<>'postgres'
 OR current_setting('server_version_num')::int NOT BETWEEN 160000 AND 169999
 OR (SELECT datdba FROM pg_catalog.pg_database WHERE datname=current_database())<>'postgres'::regrole
 OR EXISTS(SELECT 1 FROM pg_catalog.pg_class WHERE relnamespace='public'::regnamespace AND relname IN
  ('request_binding_policy','request_case_bindings','request_binding_checks','request_evidence_review_contexts','request_evidence_manifests'))
 OR EXISTS(SELECT 1 FROM pg_catalog.pg_proc WHERE pronamespace='public'::regnamespace AND proname IN
  ('guard_request_binding','protect_request_binding_records','record_unbound_evidence_review','bind_request_evidence','revoke_request_evidence_binding','request_evidence_context','record_request_evidence_review','prepare_request_evidence','request_requires_evidence_issuance'))
 OR NOT EXISTS(SELECT 1 FROM pg_catalog.pg_roles WHERE rolname='cvera_api_runtime' AND rolcanlogin
  AND NOT(rolsuper OR rolcreatedb OR rolcreaterole OR rolbypassrls OR rolreplication))
 OR NOT EXISTS(SELECT 1 FROM pg_catalog.pg_roles WHERE rolname='cvera_evidence_operator' AND NOT rolcanlogin
  AND NOT(rolsuper OR rolcreatedb OR rolcreaterole OR rolbypassrls OR rolreplication))
 OR NOT EXISTS(SELECT 1 FROM pg_catalog.pg_roles WHERE rolname='evt-app' AND NOT rolcanlogin)
 THEN RAISE EXCEPTION 'unexpected production prerequisite'; END IF;
 FOREACH name IN ARRAY ARRAY['domain_users','evt_employer_config','evt_requests','employer_hr_assignments','cases','verification_checks','case_evidence_upload_grants','case_evidence_review_grants','upload_sessions','evidence_uploads','evidence_scan_jobs','evidence_reviews'] LOOP
  SELECT * INTO item FROM pg_catalog.pg_class WHERE oid=pg_catalog.to_regclass('public.'||name);
  IF NOT FOUND OR item.relowner<>'postgres'::regrole OR item.relkind<>'r' OR item.relrowsecurity OR item.relforcerowsecurity
  THEN RAISE EXCEPTION 'parent owner/RLS mismatch'; END IF;
 END LOOP;
 IF NOT EXISTS(SELECT 1 FROM pg_catalog.pg_attribute WHERE attrelid='public.evt_requests'::regclass AND attname='version'
  AND atttypid IN ('integer'::regtype,'bigint'::regtype) AND NOT attisdropped)
 OR NOT EXISTS(SELECT 1 FROM pg_catalog.pg_attribute WHERE attrelid='public.evt_requests'::regclass AND attname='claim_snapshot'
  AND atttypid='jsonb'::regtype AND NOT attisdropped)
 OR NOT pg_catalog.has_function_privilege('cvera_api_runtime','public.check_employer_hr_assignment(uuid,text)','EXECUTE')
 OR NOT pg_catalog.has_function_privilege('cvera_api_runtime','public.record_evidence_review(uuid,text,text,uuid,text,text)','EXECUTE')
 OR pg_catalog.has_schema_privilege('cvera_api_runtime','public','CREATE')
 OR pg_catalog.has_database_privilege('cvera_api_runtime','evt','CREATE')
 THEN RAISE EXCEPTION 'request context types/existing authorization boundary mismatch'; END IF;
END $$;
"""
VERIFY="""
DO $$ DECLARE item record; permission record; expected oid; signature text; invoker boolean; name text; BEGIN
 FOREACH name IN ARRAY ARRAY['request_binding_policy','request_case_bindings','request_binding_checks','request_evidence_review_contexts','request_evidence_manifests'] LOOP
  SELECT * INTO item FROM pg_catalog.pg_class WHERE oid=pg_catalog.to_regclass('public.'||name);
  IF NOT FOUND OR item.relowner<>'postgres'::regrole OR item.relkind<>'r' OR item.relrowsecurity OR item.relforcerowsecurity
   OR EXISTS(SELECT 1 FROM pg_catalog.pg_index WHERE indrelid=item.oid AND NOT(indisvalid AND indisready))
   OR EXISTS(SELECT 1 FROM pg_catalog.aclexplode(coalesce(item.relacl,pg_catalog.acldefault('r',item.relowner))) WHERE grantee<>item.relowner)
   OR EXISTS(SELECT 1 FROM pg_catalog.pg_attribute WHERE attrelid=item.oid AND attacl IS NOT NULL)
   OR NOT EXISTS(SELECT 1 FROM pg_catalog.pg_trigger WHERE tgrelid=item.oid AND tgname='binding_immutable' AND tgenabled='O')
  THEN RAISE EXCEPTION 'binding table boundary mismatch'; END IF;
 END LOOP;
 IF (SELECT count(*) FROM public.request_binding_policy)<>1
 OR NOT EXISTS(SELECT 1 FROM public.request_binding_policy WHERE singleton AND issuer_id='https://cvera.app' AND claim_type='employment.role' AND manual_policy='MANUAL_REVIEW_PILOT_V1')
 OR EXISTS(SELECT 1 FROM public.request_case_bindings) OR EXISTS(SELECT 1 FROM public.request_binding_checks)
 OR EXISTS(SELECT 1 FROM public.request_evidence_review_contexts) OR EXISTS(SELECT 1 FROM public.request_evidence_manifests)
 THEN RAISE EXCEPTION 'policy/configuration or unexpected binding data'; END IF;
 FOREACH signature IN ARRAY ARRAY['guard_request_binding()','protect_request_binding_records()',
  'record_unbound_evidence_review(uuid,text,text,uuid,text,text)','record_evidence_review(uuid,text,text,uuid,text,text)',
  'bind_request_evidence(uuid,text,text[],text,text)','revoke_request_evidence_binding(uuid,text,text)',
  'request_evidence_context(uuid,text,text)','record_request_evidence_review(uuid,text,text,uuid,text,text,uuid,text)',
  'prepare_request_evidence(uuid,uuid,text)','request_requires_evidence_issuance(uuid,uuid,text)'] LOOP
  SELECT * INTO item FROM pg_catalog.pg_proc WHERE oid=pg_catalog.to_regprocedure('public.'||signature);
  invoker:=signature='protect_request_binding_records()';
  IF NOT FOUND OR item.proowner<>'postgres'::regrole OR item.prosecdef=invoker OR item.provolatile<>'v'
   OR NOT coalesce('search_path=pg_catalog, public, pg_temp'=ANY(item.proconfig),false)
  THEN RAISE EXCEPTION 'binding function boundary mismatch'; END IF;
  expected:=CASE WHEN item.proname IN ('bind_request_evidence','revoke_request_evidence_binding') THEN 'cvera_evidence_operator'::regrole
   WHEN item.proname IN ('guard_request_binding','protect_request_binding_records','record_unbound_evidence_review') THEN NULL ELSE 'cvera_api_runtime'::regrole END;
  FOR permission IN SELECT * FROM pg_catalog.aclexplode(coalesce(item.proacl,pg_catalog.acldefault('f',item.proowner))) WHERE grantee<>item.proowner LOOP
   IF expected IS NULL OR permission.grantee<>expected OR permission.privilege_type<>'EXECUTE' OR permission.is_grantable
   THEN RAISE EXCEPTION 'excess function ACL'; END IF;
  END LOOP;
  IF expected IS NOT NULL AND NOT pg_catalog.has_function_privilege(expected,item.oid,'EXECUTE') THEN RAISE EXCEPTION 'missing function ACL'; END IF;
 END LOOP;
END $$;
SET LOCAL ROLE cvera_api_runtime;
DO $$ BEGIN
 IF current_schema()<>'public' THEN RAISE EXCEPTION 'runtime path mismatch'; END IF;
 BEGIN
  PERFORM public.prepare_request_evidence(NULL::uuid,'00000000-0000-0000-0000-000000000000'::uuid,'__missing__');
  RAISE EXCEPTION 'missing HR authorization allowed';
 EXCEPTION WHEN SQLSTATE 'P0002' THEN NULL; END;
 BEGIN
  PERFORM public.request_requires_evidence_issuance(NULL::uuid,'00000000-0000-0000-0000-000000000000'::uuid,'__missing__');
  RAISE EXCEPTION 'missing HR authorization allowed';
 EXCEPTION WHEN SQLSTATE 'P0002' THEN NULL; END;
END $$;
RESET ROLE;
"""

# Synthetic compatibility probe is invoked only against the isolated restored clone.
RESTORE_PROBE=r"""BEGIN ISOLATION LEVEL READ COMMITTED;
SET LOCAL search_path=public,pg_temp;
SET LOCAL lock_timeout='5s';
SET LOCAL statement_timeout='30s';
DO $probe$
DECLARE subject uuid:=gen_random_uuid(); reviewer uuid:=gen_random_uuid(); request uuid:=gen_random_uuid();
 session_id uuid:=gen_random_uuid(); upload uuid:=gen_random_uuid(); binding uuid; context jsonb; result jsonb;
 employer text:='binding-restore-'||replace(gen_random_uuid()::text,'-','');
 c text:='binding-restore-'||replace(gen_random_uuid()::text,'-','');
BEGIN
 -- Clone-only synthetic records, all rolled back. No production invocation.
 INSERT INTO public.domain_users(id,kratos_identity_id,role,status) VALUES
  (subject,'restore-'||subject::text,'candidate','active'),(reviewer,'restore-'||reviewer::text,'hr_reviewer','active');
 INSERT INTO public.evt_employer_config(employer_id) VALUES(employer);
 INSERT INTO public.evt_requests(request_id,candidate_personid,employer_id,status,claim_snapshot,version)
  VALUES(request,subject,employer,'ATTESTATION_PENDING','{"synthetic":true,"title":"restore probe only"}',1);
 INSERT INTO public.employer_hr_assignments(assignment_id,principal_id,employer_id,granted_by,grant_basis_ref)
  VALUES(gen_random_uuid(),reviewer,employer,'restore-fixture','synthetic-only');
 PERFORM public.provision_evidence_case(c,'restore-fixture','synthetic-only');
 PERFORM public.provision_evidence_check(c,'smoke.file','restore-fixture','synthetic-only');
 binding:=public.bind_request_evidence(request,c,ARRAY['smoke.file'],'restore-fixture','synthetic-only');
 PERFORM public.grant_evidence_upload(subject,c,'restore-fixture','synthetic-only');
 PERFORM public.grant_evidence_review(reviewer,c,'restore-fixture','synthetic-only');
 -- Metadata-only stand-in for already verified bytes; real storage is tested by
 -- post-deployment HTTP acceptance, never by this SQL probe.
 INSERT INTO public.upload_sessions(upload_session_id,principal_id,case_id,check_id,expires_at)
  VALUES(session_id,subject,c,'smoke.file',clock_timestamp()+interval '10 minutes');
 INSERT INTO public.evidence_uploads(upload_id,upload_session_id,case_id,check_id,staging_key,file_name,expected_mime_type,expected_size,status,sealed_key,evidence_hash,completed_at)
  VALUES(upload,session_id,c,'smoke.file','cases/'||c||'/checks/smoke.file/evidence/'||upload::text||'/synthetic.pdf',
   'synthetic.pdf','application/pdf',1,'RECEIVED','sealed-evidence/'||c||'/smoke.file/'||upload::text||'/'||gen_random_uuid()::text,
   encode(sha256(convert_to('a','UTF8')),'hex'),clock_timestamp());
 INSERT INTO public.evidence_scan_jobs(upload_id) VALUES(upload);
 SET LOCAL ROLE cvera_api_runtime;
 context:=public.request_evidence_context(reviewer,c,'smoke.file');
 IF (context->>'contextValid')::boolean IS NOT TRUE OR context->>'issuerId'<>'https://cvera.app' THEN RAISE EXCEPTION 'restored context failed'; END IF;
 BEGIN
  PERFORM public.record_evidence_review(reviewer,c,'smoke.file',upload,'APPROVED','missing acknowledgement');
  RAISE EXCEPTION 'restored context gate failed';
 EXCEPTION WHEN SQLSTATE '22023' THEN NULL; END;
 PERFORM public.record_request_evidence_review(reviewer,c,'smoke.file',upload,'APPROVED','synthetic independent review',binding,context->>'claimDigest');
 result:=public.prepare_request_evidence(reviewer,request,employer);
 IF (result->>'evidenceReady')::boolean IS NOT TRUE OR (result->>'pendingScanCount')::int<>1
  OR public.request_requires_evidence_issuance(reviewer,request,employer) IS NOT TRUE
 THEN RAISE EXCEPTION 'restored manifest/signing guard failed'; END IF;
 -- Exact unsigned legacy rejection DML/event values, with actual enum/default
 -- definitions. Quoted fixture UUID literals adapt to existing UUID/text fields.
 UPDATE public.evt_requests SET status='REJECTED',employer_response_type='REJECTED_POLICY',employer_response='{}'::jsonb,
  attested_at=clock_timestamp(),version=version+1 WHERE request_id=request;
 EXECUTE format('INSERT INTO public.evt_request_events(request_id,actor_role,actor_personid,event_type,from_status,to_status,payload) VALUES(%L,''employer_hr'',%L,''HR_ATTESTED'',''ATTESTATION_PENDING'',''REJECTED'',''{"response_type":"REJECTED_POLICY","attestation_present":false,"kid":""}''::jsonb)',request::text,reviewer::text);
 RESET ROLE;
END $probe$;
ROLLBACK;
"""

# Compare only metadata, never domain records or credential material.
FINGERPRINT="""SELECT md5(coalesce(string_agg(row_to_json(x)::text,'|' ORDER BY kind,identity),'')) FROM (
 SELECT 'table' AS kind,c.oid::text AS identity,c.relowner::text AS owner,c.relacl::text AS acl,
  c.relrowsecurity::text||c.relforcerowsecurity::text AS settings
 FROM pg_catalog.pg_class c WHERE c.oid IN ('public.domain_users'::regclass,'public.evt_requests'::regclass,'public.evt_employer_config'::regclass,'public.employer_hr_assignments'::regclass,'public.case_evidence_review_grants'::regclass,'public.evidence_reviews'::regclass,'public.cases'::regclass,'public.verification_checks'::regclass,'public.case_evidence_upload_grants'::regclass,'public.upload_sessions'::regclass,'public.evidence_uploads'::regclass,'public.evidence_scan_jobs'::regclass)
 UNION ALL SELECT 'function',p.oid::text,p.proowner::text,p.proacl::text,p.proconfig::text
 FROM pg_catalog.pg_proc p WHERE p.pronamespace='public'::regnamespace AND p.proname IN
 ('check_evidence_upload','provision_evidence_case','provision_evidence_check','grant_evidence_upload','revoke_evidence_upload','set_evidence_resource_state','check_employer_hr_assignment','lock_employer_hr_principal','guard_evidence_review_catalog','check_evidence_review','list_evidence_checks','grant_evidence_review','revoke_evidence_review','list_evidence_for_review','get_evidence_for_review')
 UNION ALL SELECT 'role',r.oid::text,r.rolname,NULL,r.rolcanlogin::text||r.rolsuper::text||r.rolcreatedb::text||r.rolcreaterole::text||r.rolbypassrls::text
 FROM pg_catalog.pg_roles r WHERE r.rolname IN ('postgres','evt-app','cvera_api_runtime','cvera_kratos_runtime','cvera_hr_operator','cvera_evidence_operator')
) x;"""

def transaction(sources,final):
 return ('BEGIN ISOLATION LEVEL READ COMMITTED;'+SETTINGS+PREFLIGHT+
 "SET LOCAL cvera.evidence_runtime_role='cvera_api_runtime'; SET LOCAL cvera.evidence_operator_role='cvera_evidence_operator';\n"+
 '\n'.join(sources.values())+'\n'+VERIFY+'\nSELECT pg_catalog.txid_current();\n'+final+';')

def restore_rehearsal(backup,dump,sources):
 global PHASE
 PHASE='isolated_backup_restore_and_migration_rehearsal'
 name='cvera-binding-restore-'+backup.name.rsplit('-',1)[-1].lower()
 image=info(DB,'Image')
 run(['docker','run','-d','--pull','never','--name',name,'--network','none','--memory','64m','--memory-swap','64m','--cpus','0.5','--shm-size','16m',
  '--label','cvera.scope=request-evidence-binding-rehearsal','-e','POSTGRES_HOST_AUTH_METHOD=trust','-e','POSTGRES_INITDB_ARGS=-c shared_buffers=8MB -c max_connections=10 -c work_mem=512kB -c maintenance_work_mem=4MB -c autovacuum=off',image,
  '-c','shared_buffers=8MB','-c','max_connections=10','-c','work_mem=512kB','-c','maintenance_work_mem=4MB','-c','autovacuum=off','-c','max_worker_processes=0','-c','max_parallel_workers=0'])
 def clone_sql(body):
  return run(['docker','exec','-i',name,'psql','-X','-U','postgres','-d','evt','-Atq','-v','ON_ERROR_STOP=1'],body.encode())
 try:
  for _ in range(60):
   ready=subprocess.run(['docker','exec',name,'psql','-X','-U','postgres','-d','postgres','-Atqc','SHOW listen_addresses'],capture_output=True,timeout=5)
   if ready.returncode==0 and ready.stdout.strip()==b'*':break
   time.sleep(1)
  else:raise RuntimeError('restore readiness failed')
  assert info(name,'HostConfig.NetworkMode')=='none'
  live={x['Source'] for x in info(DB,'Mounts') if x.get('Source')}
  assert not live.intersection(x['Source'] for x in info(name,'Mounts') if x.get('Source'))
  roles=json.loads(sql("BEGIN READ ONLY;SELECT json_agg(rolname) FROM pg_catalog.pg_roles WHERE rolname<>'postgres' AND rolname !~ '^pg_';ROLLBACK;"))
  ddl=''.join('CREATE ROLE "'+r.replace('"','""')+'" '+('LOGIN' if r=='cvera_api_runtime' else 'NOLOGIN')+' NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS;' for r in roles)
  run(['docker','exec','-i',name,'psql','-X','-U','postgres','-d','postgres','-v','ON_ERROR_STOP=1'],(ddl+'CREATE DATABASE evt OWNER postgres;').encode())
  run(['docker','exec','-i',name,'pg_restore','-U','postgres','-d','evt','--exit-on-error'],dump,timeout=180)
  before=clone_sql('BEGIN READ ONLY;'+FINGERPRINT+'ROLLBACK;')
  clone_sql(transaction(sources,'ROLLBACK'))
  clone_sql('BEGIN READ ONLY;'+SETTINGS+PREFLIGHT+'ROLLBACK;')
  clone_sql(transaction(sources,'COMMIT'))
  clone_sql('BEGIN;'+SETTINGS+VERIFY+'ROLLBACK;')
  clone_sql(RESTORE_PROBE)
  clone_sql('BEGIN;'+SETTINGS+VERIFY+'ROLLBACK;')
  if clone_sql('BEGIN READ ONLY;'+FINGERPRINT+'ROLLBACK;')!=before:raise RuntimeError('rehearsal modified registry/roles')
  if info(name,'State.OOMKilled') or info(name,'HostConfig.Memory')!=64*1024**2:raise RuntimeError('restore memory boundary failed')
  peak_result=subprocess.run(['docker','exec',name,'cat','/sys/fs/cgroup/memory.peak'],capture_output=True,timeout=10)
  peak=int(peak_result.stdout.strip()) if peak_result.returncode==0 and peak_result.stdout.strip().isdigit() else None
  private(backup/'restore-rehearsal.json',json.dumps({'memory_limit_mib':64,'memory_peak_bytes':peak,'restore':'PASS','migration_rollback':'PASS','migration_commit':'PASS','privileges':'PASS','context_and_manifest':'PASS','unsigned_rejection_compatibility':'PASS','clone':name}).encode())
 finally:
  # Preserve the restored clone/volume for inspection; release its scarce RAM.
  run(['docker','stop','--time','10',name],timeout=30)


def main():
 global PHASE
 if os.geteuid()!=0 or sys.argv[1:]!=['--approved-release']:raise RuntimeError('existing root console and release flag required')
 os.umask(0o077)
 if run(['git','-C',str(ROOT),'status','--porcelain']).strip():raise RuntimeError('unclean production checkout')
 checkout=run(['git','-C',str(ROOT),'rev-parse','HEAD']).decode().strip()
 if checkout not in (BASE,'c4d7f9c7e9175f4c98c847db3dc5835ce2e7beb7'):raise RuntimeError('unexpected checkout')
 with urllib.request.urlopen('http://127.0.0.1:8080/version',timeout=5) as response:
  if json.load(response).get('git_sha')!=BASE:raise RuntimeError('unexpected running API')
 with urllib.request.urlopen('http://127.0.0.1:8080/healthz',timeout=5) as response:
  if json.load(response).get('ok')!='true':raise RuntimeError('unhealthy API')
 services=[DB,'evt-api-go-api-1','evt-api-go-kratos-1','evt-api-go-kratos-courier-1']
 before={n:(info(n,'Id'),info(n,'State.StartedAt'),info(n,'Image')) for n in services}
 if not all(info(n,'State.Running') and not info(n,'State.OOMKilled') for n in services):raise RuntimeError('unhealthy service')
 env=dict(x.split('=',1) for x in info('evt-api-go-api-1','Config.Env') if '=' in x)
 uri=urllib.parse.urlsplit(env['EVT_DB_DSN']);q=urllib.parse.parse_qs(uri.query)
 if urllib.parse.unquote(uri.username or '')!='cvera_api_runtime' or uri.path!='/evt' or set(q)-{'sslmode','search_path'} or q.get('search_path',['public'])!=['public']:raise RuntimeError('unverified DB path/role')
 envfile=ROOT/'apps/evt-api-go/.env';st=envfile.stat()
 if (st.st_uid,st.st_gid,stat.S_IMODE(st.st_mode))!=(0,0,0o600):raise RuntimeError('unprotected runtime configuration')
 env_hash=hashlib.sha256(envfile.read_bytes()).hexdigest()
 sources={}
 for name,expected in FILES.items():
  value=run(['git','-C',str(ROOT),'show',SOURCE+':apps/evt-api-go/'+name])
  if hashlib.sha256(value).hexdigest()!=expected:raise RuntimeError('SQL source mismatch')
  sources[name]=value.decode()
 PHASE='production_schema_preflight'
 sql('BEGIN READ ONLY;'+SETTINGS+PREFLIGHT+'ROLLBACK;')
 fingerprint=sql('BEGIN READ ONLY;'+FINGERPRINT+'ROLLBACK;')
 PHASE='restore_capacity_preflight'
 size=int(sql('BEGIN READ ONLY;SELECT pg_catalog.pg_database_size(current_database());ROLLBACK;'))
 if shutil.disk_usage('/opt/cvera').free<size*4+1024**3:raise RuntimeError('insufficient backup capacity')
 mem=dict((line.split(':',1)[0],int(line.split()[1])) for line in Path('/proc/meminfo').read_text().splitlines() if line.startswith('MemAvailable:'))
 # 64 MiB hard cap for the isolated clone plus 64 MiB host headroom.
 # A failed/OOM rehearsal aborts before any production migration.
 if mem['MemAvailable']<128*1024:raise RuntimeError('insufficient restore-test memory')
 stamp=datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ')
 backup=Path('/opt/cvera/request-evidence-binding-release-'+stamp);backup.mkdir(mode=0o700)
 private(backup/'release-intent.json',json.dumps({'sql_source':SOURCE,'base':BASE,'state':'started; inspect before any retry'}).encode())
 PHASE='fresh_backup_and_recovery'
 dump=run(['docker','exec',DB,'pg_dump','-U','postgres','-d','evt','-Fc'],timeout=180)
 private(backup/'evt.dump',dump)
 run(['docker','exec','-i',DB,'pg_restore','--list'],dump)
 private(backup/'roles-without-passwords.sql',run(['docker','exec',DB,'pg_dumpall','-U','postgres','--roles-only','--no-role-passwords']))
 private(backup/'backup.sha256',(hashlib.sha256(dump).hexdigest()+'  evt.dump\n').encode())
 tag='cvera-api:recovery-binding-'+stamp.lower()
 run(['docker','image','tag',before['evt-api-go-api-1'][2],tag])
 if json.loads(run(['docker','image','inspect','--format','{{json .Id}}',tag]))!=before['evt-api-go-api-1'][2]:raise RuntimeError('recovery image mismatch')
 private(backup/'api-recovery-tag.txt',(tag+'\n').encode())
 private(backup/'api.env.before',envfile.read_bytes())
 private(backup/'compose-before.yml',(ROOT/'apps/evt-api-go/docker-compose.yml').read_bytes())
 private(backup/'api-image-override.json',json.dumps({'services':{'api':{'image':tag}}}).encode())
 restore_rehearsal(backup,dump,sources)
 if hashlib.sha256((backup/'evt.dump').read_bytes()).hexdigest()!=hashlib.sha256(dump).hexdigest():raise RuntimeError('backup checksum mismatch')
 private(backup/'recovery-constraint.txt',b'Older API rollback is permitted only with zero ATTESTATION_PENDING bound requests, including revoked bindings. Otherwise stop for owner-controlled recovery; never bypass the new signing guard.\n')
 private(backup/'sql-intent.json',json.dumps({'migration':'000018 only','source':SOURCE,'hashes':FILES,'rehearsal':'PASS'}).encode())
 PHASE='production_migration_transaction'
 transaction_id=sql(transaction(sources,'COMMIT')).decode().strip()
 # COMMIT ambiguity must stop without retry; the intent file identifies this run.
 private(backup/'sql-commit.json',json.dumps({'migration':'000018 only','source':SOURCE,'hashes':FILES,'transaction':transaction_id,'in_transaction_verification':'PASS','post_commit_verification':'PENDING'}).encode())
 PHASE='post_commit_verification'
 sql('BEGIN;'+SETTINGS+VERIFY+'ROLLBACK;')
 if sql('BEGIN READ ONLY;'+FINGERPRINT+'ROLLBACK;')!=fingerprint:raise RuntimeError('registry/roles changed')
 if hashlib.sha256(envfile.read_bytes()).hexdigest()!=env_hash:raise RuntimeError('configuration changed')
 for name,baseline in before.items():
  if (info(name,'Id'),info(name,'State.StartedAt'),info(name,'Image'))!=baseline or not info(name,'State.Running'):raise RuntimeError('service changed')
 private(backup/'sql-verification.json',json.dumps({'migration':'000018 only','source':SOURCE,'hashes':FILES,'transaction':transaction_id,'verification':'PASS','parent_catalog_and_roles_unchanged':True}).encode())
 print(json.dumps({'request_evidence_binding_sql':'PASS','sql_source':SOURCE,'migration':'000018 only','fresh_backup_restored':True,'rollback_and_commit_rehearsal':True,'restricted_runtime_bindings':True,'parent_catalog_and_roles_unchanged':True,'recovery_image_retained':True,'new_bindings_or_application_records':0,'canonical_issuer':'https://cvera.app','authority_privileges_added':False,'evidence_backed_signing_enabled':False,'identity_policy_seeded':True,'credentials_runtime_configuration_services_changed':False,'private_record':str(backup/'sql-verification.json')}))

if __name__=='__main__':
 try:main()
 except Exception:
  print(json.dumps({'request_evidence_binding_sql':'STOP','phase':PHASE,'instruction':'Do not retry, merge or deploy; return only this status. Raw output suppressed.'}),file=sys.stderr)
  sys.exit(1)
