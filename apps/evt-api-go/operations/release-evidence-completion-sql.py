#!/usr/bin/env python3
"""Approved evidence-completion SQL phase in the existing owner console.

Back up/restore-test evt, retain API recovery, verify parent schema and apply only
000016 with narrow runtime bindings. Never change roles, credentials or services.
All raw SQL/Docker errors stay private; STOP is not authorization to retry.
"""
import datetime, hashlib, json, os, shutil, stat, subprocess, sys, time
import urllib.parse, urllib.request
from pathlib import Path

ROOT=Path('/opt/cvera/rsa-attestation-engine')
DB='evt-api-go-db-1'
PHASE='preflight'
BASE='049893422f96833bec3447fb561ac8ab7c8ff460'
SOURCE='f94545fa464cf20486d9be71bf29c33ad079930a'
SETTINGS="SET LOCAL search_path=public,pg_temp; SET LOCAL lock_timeout='5s'; SET LOCAL statement_timeout='60s';"
FILES={'migrations/000016_evidence_completion.sql': 'dfdb24018f84c0466204dc717937371ab63cb8d4275b4ea2d82baa0a29568952', 'operations/evidence-completion-privileges.sql': 'a70df5f9f753b980ad6c955eed9277baf6bb4374f6c9b40a043794155d97256a'}

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
DO $$ DECLARE item record; BEGIN
 IF current_database()<>'evt' OR current_user<>'postgres'
 OR current_setting('server_version_num')::int NOT BETWEEN 160000 AND 169999
 OR (SELECT datdba FROM pg_catalog.pg_database WHERE datname=current_database())<>'postgres'::regrole
 OR EXISTS(SELECT 1 FROM pg_catalog.pg_class WHERE relnamespace='public'::regnamespace
   AND relname IN ('upload_sessions','evidence_uploads','evidence_scan_jobs'))
 OR NOT EXISTS(SELECT 1 FROM pg_catalog.pg_roles WHERE rolname='cvera_api_runtime' AND rolcanlogin
   AND NOT(rolsuper OR rolcreatedb OR rolcreaterole OR rolbypassrls OR rolreplication))
 OR NOT EXISTS(SELECT 1 FROM pg_catalog.pg_roles WHERE rolname='evt-app' AND NOT rolcanlogin)
 OR EXISTS(SELECT 1 FROM pg_catalog.pg_auth_members WHERE member='cvera_api_runtime'::regrole)
 OR pg_catalog.has_schema_privilege('cvera_api_runtime','public','CREATE')
 OR pg_catalog.has_database_privilege('cvera_api_runtime','evt','CREATE')
 THEN RAISE EXCEPTION 'unexpected production prerequisite'; END IF;
 FOR item IN SELECT * FROM (VALUES
  ('domain_users','id','uuid'),('domain_users','status','text'),
  ('cases','case_id','text'),('cases','status','text'),
  ('verification_checks','case_id','text'),('verification_checks','check_id','text'),('verification_checks','status','text'),
  ('case_evidence_upload_grants','principal_id','uuid'),('case_evidence_upload_grants','case_id','text')) AS expected(t,c,ty) LOOP
  IF NOT EXISTS(SELECT 1 FROM pg_catalog.pg_attribute a JOIN pg_catalog.pg_class c ON c.oid=a.attrelid
   WHERE c.oid=pg_catalog.to_regclass('public.'||item.t) AND a.attname=item.c
   AND a.atttypid=pg_catalog.to_regtype(item.ty) AND a.attnotnull AND NOT a.attisdropped
   AND c.relowner='postgres'::regrole AND NOT(c.relrowsecurity OR c.relforcerowsecurity))
  THEN RAISE EXCEPTION 'parent type/owner/RLS mismatch'; END IF;
 END LOOP;
 IF NOT EXISTS(SELECT 1 FROM pg_catalog.pg_constraint WHERE conrelid='public.domain_users'::regclass
   AND contype='p' AND pg_catalog.pg_get_constraintdef(oid)='PRIMARY KEY (id)')
 OR NOT EXISTS(SELECT 1 FROM pg_catalog.pg_constraint WHERE conrelid='public.verification_checks'::regclass
   AND contype='p' AND pg_catalog.pg_get_constraintdef(oid)='PRIMARY KEY (case_id, check_id)')
 OR NOT EXISTS(SELECT 1 FROM pg_catalog.pg_proc WHERE oid=pg_catalog.to_regprocedure('public.check_evidence_upload(uuid,text,text)')
   AND proowner='postgres'::regrole AND prosecdef AND provolatile='v'
   AND proconfig @> ARRAY['search_path=pg_catalog, public, pg_temp'])
 OR NOT pg_catalog.has_function_privilege('cvera_api_runtime','public.check_evidence_upload(uuid,text,text)','EXECUTE')
 OR pg_catalog.has_table_privilege('cvera_api_runtime','public.cases','INSERT,UPDATE,DELETE')
 OR pg_catalog.has_table_privilege('cvera_api_runtime','public.case_evidence_upload_grants','SELECT,INSERT,UPDATE,DELETE')
 THEN RAISE EXCEPTION 'registry authorization prerequisite mismatch'; END IF;
END $$;
"""
VERIFY="""
DO $$ DECLARE item record; a record; expected text[]; BEGIN
 IF (SELECT count(*) FROM pg_catalog.pg_class WHERE relnamespace='public'::regnamespace
 AND relname IN ('upload_sessions','evidence_uploads','evidence_scan_jobs') AND relkind='r')<>3
 THEN RAISE EXCEPTION 'missing lifecycle tables'; END IF;
 FOR item IN SELECT * FROM pg_catalog.pg_class WHERE relnamespace='public'::regnamespace
 AND relname IN ('upload_sessions','evidence_uploads','evidence_scan_jobs') LOOP
  IF item.relowner<>'postgres'::regrole OR item.relrowsecurity OR item.relforcerowsecurity
  OR EXISTS(SELECT 1 FROM pg_catalog.pg_index WHERE indrelid=item.oid AND NOT(indisvalid AND indisready))
  THEN RAISE EXCEPTION 'lifecycle owner/RLS/index mismatch'; END IF;
  FOR a IN SELECT * FROM pg_catalog.aclexplode(coalesce(item.relacl,pg_catalog.acldefault('r',item.relowner))) WHERE grantee<>item.relowner LOOP
   IF a.grantee<>'cvera_api_runtime'::regrole OR a.privilege_type<>'SELECT' OR a.is_grantable
   OR item.relname='evidence_scan_jobs' THEN RAISE EXCEPTION 'excess lifecycle table ACL'; END IF;
  END LOOP;
  FOR a IN SELECT att.attname,acl.* FROM pg_catalog.pg_attribute att,LATERAL pg_catalog.aclexplode(att.attacl) acl
   WHERE att.attrelid=item.oid AND acl.grantee<>item.relowner LOOP
   IF a.grantee<>'cvera_api_runtime'::regrole OR a.is_grantable THEN RAISE EXCEPTION 'unexpected column grantee'; END IF;
   expected:=CASE item.relname
    WHEN 'upload_sessions' THEN CASE a.privilege_type WHEN 'INSERT' THEN ARRAY['upload_session_id','principal_id','case_id','check_id','expires_at'] WHEN 'UPDATE' THEN ARRAY['closed_at'] ELSE ARRAY[]::text[] END
    WHEN 'evidence_uploads' THEN CASE a.privilege_type WHEN 'INSERT' THEN ARRAY['upload_id','upload_session_id','case_id','check_id','staging_key','file_name','expected_mime_type','expected_size'] WHEN 'UPDATE' THEN ARRAY['status','sealed_key','evidence_hash','completed_at'] ELSE ARRAY[]::text[] END
    ELSE CASE a.privilege_type WHEN 'INSERT' THEN ARRAY['upload_id'] ELSE ARRAY[]::text[] END END;
   IF NOT a.attname=ANY(expected) THEN RAISE EXCEPTION 'excess lifecycle column ACL'; END IF;
  END LOOP;
  IF pg_catalog.has_any_column_privilege('cvera_evidence_operator',item.oid,'SELECT')
   OR pg_catalog.has_any_column_privilege('evt-app',item.oid,'SELECT')
   OR pg_catalog.has_any_column_privilege('evt-app',item.oid,'INSERT')
   OR pg_catalog.has_any_column_privilege('evt-app',item.oid,'UPDATE')
  THEN RAISE EXCEPTION 'unexpected operator/legacy lifecycle access'; END IF;
 END LOOP;
 IF EXISTS(SELECT 1 FROM public.upload_sessions) OR EXISTS(SELECT 1 FROM public.evidence_uploads)
 OR EXISTS(SELECT 1 FROM public.evidence_scan_jobs)
 OR pg_catalog.has_column_privilege('cvera_api_runtime','public.evidence_scan_jobs','scan_status','UPDATE')
 OR pg_catalog.has_function_privilege('cvera_api_runtime','public.provision_evidence_case(text,text,text)','EXECUTE')
 THEN RAISE EXCEPTION 'data/scan/admin boundary failed'; END IF;
END $$;
SET LOCAL ROLE cvera_api_runtime;
DO $$ BEGIN
 IF public.check_evidence_upload(NULL::uuid,'__missing__','__missing__') IS DISTINCT FROM false
 OR current_schema()<>'public' THEN RAISE EXCEPTION 'runtime denial/path failed'; END IF;
 PERFORM upload_session_id FROM public.upload_sessions LIMIT 0;
 PERFORM upload_id FROM public.evidence_uploads LIMIT 0;
END $$;
RESET ROLE;
"""

# Compare only metadata, never domain records or credential material.
FINGERPRINT="""SELECT md5(coalesce(string_agg(row_to_json(x)::text,'|' ORDER BY kind,identity),'')) FROM (
 SELECT 'table' AS kind,c.oid::text AS identity,c.relowner::text AS owner,c.relacl::text AS acl,
  c.relrowsecurity::text||c.relforcerowsecurity::text AS settings
 FROM pg_catalog.pg_class c WHERE c.oid IN ('public.domain_users'::regclass,'public.cases'::regclass,'public.verification_checks'::regclass,'public.case_evidence_upload_grants'::regclass)
 UNION ALL SELECT 'function',p.oid::text,p.proowner::text,p.proacl::text,p.proconfig::text
 FROM pg_catalog.pg_proc p WHERE p.pronamespace='public'::regnamespace AND p.proname IN
 ('check_evidence_upload','provision_evidence_case','provision_evidence_check','grant_evidence_upload','revoke_evidence_upload','set_evidence_resource_state')
 UNION ALL SELECT 'role',r.oid::text,r.rolname,NULL,r.rolcanlogin::text||r.rolsuper::text||r.rolcreatedb::text||r.rolcreaterole::text||r.rolbypassrls::text
 FROM pg_catalog.pg_roles r WHERE r.rolname IN ('postgres','evt-app','cvera_api_runtime','cvera_kratos_runtime','cvera_hr_operator','cvera_evidence_operator')
) x;"""

def transaction(sources,final):
 return ('BEGIN ISOLATION LEVEL READ COMMITTED;'+SETTINGS+PREFLIGHT+
 "SET LOCAL cvera.evidence_runtime_role='cvera_api_runtime';\n"+
 '\n'.join(sources.values())+'\n'+VERIFY+'\nSELECT pg_catalog.txid_current();\n'+final+';')

def restore_rehearsal(backup,dump,sources):
 global PHASE
 PHASE='isolated_backup_restore_and_migration_rehearsal'
 name='cvera-completion-restore-'+backup.name.rsplit('-',1)[-1].lower()
 image=info(DB,'Image')
 run(['docker','run','-d','--pull','never','--name',name,'--network','none','--memory','96m','--memory-swap','96m','--cpus','0.5','--shm-size','16m',
  '--label','cvera.scope=evidence-completion-rehearsal','-e','POSTGRES_HOST_AUTH_METHOD=trust','-e','POSTGRES_INITDB_ARGS=-c shared_buffers=8MB -c max_connections=10 -c work_mem=512kB -c maintenance_work_mem=4MB -c autovacuum=off',image,
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
  if clone_sql('BEGIN READ ONLY;'+FINGERPRINT+'ROLLBACK;')!=before:raise RuntimeError('rehearsal modified registry/roles')
  private(backup/'restore-rehearsal.json',json.dumps({'restore':'PASS','migration_rollback':'PASS','migration_commit':'PASS','privileges':'PASS','clone':name}).encode())
 finally:
  # Preserve the restored clone/volume for inspection; release its scarce RAM.
  run(['docker','stop','--time','10',name],timeout=30)


def main():
 global PHASE
 if os.geteuid()!=0 or sys.argv[1:]!=['--approved-release']:raise RuntimeError('existing root console and release flag required')
 os.umask(0o077)
 if run(['git','-C',str(ROOT),'status','--porcelain']).strip():raise RuntimeError('unclean production checkout')
 checkout=run(['git','-C',str(ROOT),'rev-parse','HEAD']).decode().strip()
 if checkout not in (BASE,'eeed4f0f348b2d6ae6c3fc37d6420ee83b3c3e72'):raise RuntimeError('unexpected checkout')
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
 size=int(sql('BEGIN READ ONLY;SELECT pg_catalog.pg_database_size(current_database());ROLLBACK;'))
 if shutil.disk_usage('/opt/cvera').free<size*4+1024**3:raise RuntimeError('insufficient backup capacity')
 mem=dict((line.split(':',1)[0],int(line.split()[1])) for line in Path('/proc/meminfo').read_text().splitlines() if line.startswith('MemAvailable:'))
 if mem['MemAvailable']<160*1024:raise RuntimeError('insufficient restore-test memory')
 stamp=datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ')
 backup=Path('/opt/cvera/evidence-completion-release-'+stamp);backup.mkdir(mode=0o700)
 private(backup/'release-intent.json',json.dumps({'sql_source':SOURCE,'base':BASE,'state':'started; inspect before any retry'}).encode())
 PHASE='fresh_backup_and_recovery'
 dump=run(['docker','exec',DB,'pg_dump','-U','postgres','-d','evt','-Fc'],timeout=180)
 private(backup/'evt.dump',dump)
 run(['docker','exec','-i',DB,'pg_restore','--list'],dump)
 private(backup/'roles-without-passwords.sql',run(['docker','exec',DB,'pg_dumpall','-U','postgres','--roles-only','--no-role-passwords']))
 private(backup/'backup.sha256',(hashlib.sha256(dump).hexdigest()+'  evt.dump\n').encode())
 tag='cvera-api:recovery-completion-'+stamp.lower()
 run(['docker','image','tag',before['evt-api-go-api-1'][2],tag])
 if json.loads(run(['docker','image','inspect','--format','{{json .Id}}',tag]))!=before['evt-api-go-api-1'][2]:raise RuntimeError('recovery image mismatch')
 private(backup/'api-recovery-tag.txt',(tag+'\n').encode())
 private(backup/'api.env.before',envfile.read_bytes())
 private(backup/'compose-before.yml',(ROOT/'apps/evt-api-go/docker-compose.yml').read_bytes())
 private(backup/'api-image-override.json',json.dumps({'services':{'api':{'image':tag}}}).encode())
 restore_rehearsal(backup,dump,sources)
 if hashlib.sha256((backup/'evt.dump').read_bytes()).hexdigest()!=hashlib.sha256(dump).hexdigest():raise RuntimeError('backup checksum mismatch')
 private(backup/'sql-intent.json',json.dumps({'migration':'000016 only','source':SOURCE,'hashes':FILES,'rehearsal':'PASS'}).encode())
 PHASE='production_migration_transaction'
 transaction_id=sql(transaction(sources,'COMMIT')).decode().strip()
 # COMMIT ambiguity must stop without retry; the intent file identifies this run.
 private(backup/'sql-commit.json',json.dumps({'migration':'000016 only','source':SOURCE,'hashes':FILES,'transaction':transaction_id,'verification':'PASS'}).encode())
 PHASE='post_commit_verification'
 sql('BEGIN;'+SETTINGS+VERIFY+'ROLLBACK;')
 if sql('BEGIN READ ONLY;'+FINGERPRINT+'ROLLBACK;')!=fingerprint:raise RuntimeError('registry/roles changed')
 if hashlib.sha256(envfile.read_bytes()).hexdigest()!=env_hash:raise RuntimeError('configuration changed')
 for name,baseline in before.items():
  if (info(name,'Id'),info(name,'State.StartedAt'),info(name,'Image'))!=baseline or not info(name,'State.Running'):raise RuntimeError('service changed')
 print(json.dumps({'evidence_completion_sql':'PASS','sql_source':SOURCE,'migration':'000016 only','fresh_backup_restored':True,'rollback_and_commit_rehearsal':True,'restricted_runtime_bindings':True,'registry_and_roles_unchanged':True,'recovery_image_retained':True,'new_resources_or_grants':0,'credentials_configuration_services_changed':False,'private_record':str(backup/'sql-commit.json')}))

if __name__=='__main__':
 try:main()
 except Exception:
  print(json.dumps({'evidence_completion_sql':'STOP','phase':PHASE,'instruction':'Do not retry, merge or deploy; return only this status. Raw output suppressed.'}),file=sys.stderr)
  sys.exit(1)
