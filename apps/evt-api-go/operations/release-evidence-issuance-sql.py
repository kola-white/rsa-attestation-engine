#!/usr/bin/env python3
"""Prepare without production SQL writes; apply only with separate schema approval.

Uses the established owner console, private backup and isolated same-image clone.
Never records authority, registers credentials, signs, creates grants or deploys.
"""
import argparse,datetime,fcntl,hashlib,json,os,re,shutil,stat,subprocess,time,urllib.parse,urllib.request,uuid
from pathlib import Path
ROOT=Path('/opt/cvera/rsa-attestation-engine');DB='evt-api-go-db-1'
BASE='e84b9a64af8225e439e0c4b1029d561181a37aa8'
SOURCE='ab0c0f6cad053beb9873c8fe716dbc2f92fccf75'
PHASE='preflight';DIAGNOSTIC={}
SETTINGS="SET LOCAL search_path=public,pg_temp;SET LOCAL lock_timeout='5s';SET LOCAL statement_timeout='60s';"
# Populated from reviewed repository artifacts; never resolve SQL from live main.
FILES={'migrations/000002_authorization_records.sql': '91d7a21cd652365553c9dc255892ca3936e1c15de33bd4b40f47f431ffa54bcc', 'migrations/000003_authority_adjudications.sql': '44c673a4a566c8b22dd39dabc9806e97ce9a5af0063aebf3a4a2e819a9ae021a', 'migrations/000004_authority_journal.sql': 'f9862612134cbcafbb70766d7c9c9155918a8bed66429bb7c8c9f862feb79b09', 'migrations/000005_authority_event_types.sql': 'd1ad0aa5d2900109dcd11aa1409c2928a8adb9b6e5846e7985bc19fc1458c5b4', 'migrations/000006_authority_immutability.sql': 'fc309b49296f9c24136801d642c7d3703a14edcc8deb6af312b029e0137d7b24', 'migrations/000007_authority_scope_integrity.sql': 'fdc94e26b74716e2e96309cc3084db927ac6e16eea363dcc26de227b395add0a', 'migrations/000008_authority_correction_integrity.sql': 'd4cc48826bd63eb91a46ef7d444d49f009968d7e079b79c6bcf7c346a284336a', 'migrations/000009_authority_correction_fields.sql': '2cf209f331fda3f2fbd5953ad66f89662290ad40b154d1a8b4bc8cc0d84043f6', 'migrations/000010_authority_recording_boundary.sql': 'a322603be4892a24538e6598d0aac2ecce5ec3b42a3448badab912eae964f26a', 'migrations/000011_authority_evidence_recording_positions.sql': '4d7644cffa8059b59681b870018a8e04327d76b603893b8911d490b25fcc915b', 'migrations/000012_authority_recording_enforcement.sql': '1bdebca659e4a36f5a623794178a5c810480e6bf7d2507b1614a53b261d7a4dd', 'migrations/000013_authority_allocator_security.sql': 'c874c8fda2bb37e294eaa40bfa4f7c149d909a358145a2ba33c3822d733e9eb4', 'migrations/000019_evidence_backed_issuance.sql': '07f6df0d159b26702f1cbf4fc3da421c715938cb0c561f71f58d9b38dfefba1f', 'operations/evidence-issuance-privileges.sql': 'b8c0eeb82763d253f702cca1cac9ad5482477a35df9c0155b2c8011b0112d24e', 'operations/evidence-issuance-restore-probe.sql': '7a7ba62906eb55c6f498ff88ee69121f61876b0496c1e689f898b335dd6f2d2e'}
TABLES=['authority_authorizations','authority_adjudications','authority_journal_events','authority_recording_boundary']
AUTH_FUNCTIONS=['reject_authority_mutation()','next_authority_recording_position()','authority_evidence_position_count(bigint)','validate_authority_evidence_recording()','validate_authority_recording_allocation()']
RUNTIME_FUNCTIONS=['active_evidence_signing_credential(text)','read_evidence_authority()','prepare_bound_issuance(uuid,uuid,text,text)','commit_bound_issuance(uuid,uuid,text,text,text,jsonb)','resolve_bound_credential(text)']

def run(args,data=None,timeout=120):
 r=subprocess.run(args,input=data,capture_output=True,timeout=timeout)
 if r.returncode:
  DIAGNOSTIC.clear();DIAGNOSTIC['exit_code']=r.returncode
  match=re.search(rb'(?:ERROR|FATAL):\s+([0-9A-Z]{5})\b',r.stderr)
  if match:DIAGNOSTIC['sqlstate']=match.group(1).decode('ascii')
  raise RuntimeError('command failed; private output suppressed')
 return r.stdout

def info(name,field):return json.loads(run(['docker','inspect','--format','{{json .'+field+'}}',name]))
def sql(body,name=None):return run(['docker','exec','-i',name or DB,'psql','-X','-U','postgres','-d','evt','-Atq','-v','ON_ERROR_STOP=1','-v','VERBOSITY=sqlstate'],body.encode())
def read(body,name=None):return sql('BEGIN READ ONLY;'+SETTINGS+body+';ROLLBACK;',name)
def private(path,data):
 fd=os.open(path,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW,0o600)
 with os.fdopen(fd,'wb') as f:f.write(data);f.flush();os.fsync(f.fileno())
def secure(path,mode):
 st=path.lstat()
 if stat.S_ISLNK(st.st_mode) or st.st_uid!=0 or stat.S_IMODE(st.st_mode)!=mode:raise RuntimeError('private recovery permissions')
def digest(path):
 h=hashlib.sha256()
 with path.open('rb') as f:
  for block in iter(lambda:f.read(1024*1024),b''):h.update(block)
 return h.hexdigest()

PREFLIGHT="""DO $$ DECLARE n text;r record; BEGIN
 IF current_database()<>'evt' OR current_user<>'postgres' OR current_setting('server_version_num')::int NOT BETWEEN 160000 AND 169999
 OR (SELECT datdba FROM pg_catalog.pg_database WHERE datname=current_database())<>'postgres'::regrole
 OR NOT EXISTS(SELECT 1 FROM pg_catalog.pg_roles WHERE rolname='cvera_api_runtime' AND rolcanlogin AND NOT(rolsuper OR rolcreatedb OR rolcreaterole OR rolbypassrls OR rolreplication))
 OR EXISTS(SELECT 1 FROM pg_catalog.pg_auth_members WHERE member='cvera_api_runtime'::regrole)
 OR pg_catalog.has_schema_privilege('cvera_api_runtime','public','CREATE') OR pg_catalog.has_database_privilege('cvera_api_runtime','evt','CREATE')
 THEN RAISE EXCEPTION 'owner/runtime prerequisite mismatch';END IF;
 FOREACH n IN ARRAY ARRAY['domain_users','evt_employer_config','evt_requests','evt_request_events','employer_hr_assignments','cases','verification_checks','case_evidence_upload_grants','upload_sessions','evidence_uploads','evidence_scan_jobs','case_evidence_review_grants','evidence_reviews','request_binding_policy','request_case_bindings','request_binding_checks','request_evidence_review_contexts','request_evidence_manifests'] LOOP
 SELECT * INTO r FROM pg_catalog.pg_class WHERE oid=pg_catalog.to_regclass('public.'||n);
 IF NOT FOUND OR r.relowner<>'postgres'::regrole OR r.relkind<>'r' OR r.relrowsecurity OR r.relforcerowsecurity THEN RAISE EXCEPTION 'parent schema/owner mismatch';END IF;
 END LOOP;
 IF EXISTS(SELECT 1 FROM pg_catalog.pg_class WHERE relnamespace='public'::regnamespace AND relname IN ('authority_authorizations','authority_adjudications','authority_journal_events','authority_recording_boundary','evidence_issuer_credentials','evidence_issued_credentials'))
 OR EXISTS(SELECT 1 FROM pg_catalog.pg_proc WHERE pronamespace='public'::regnamespace AND proname IN ('reject_authority_mutation','next_authority_recording_position','authority_evidence_position_count','validate_authority_evidence_recording','validate_authority_recording_allocation','guard_evidence_issuance','protect_evidence_issuance','active_evidence_signing_credential','read_evidence_authority','prepare_bound_issuance','commit_bound_issuance','resolve_bound_credential','revoke_bound_credential'))
 THEN RAISE EXCEPTION 'existing/partial authority or issuance schema: inspect without bootstrap/retry';END IF;
 IF NOT EXISTS(SELECT 1 FROM public.request_binding_policy WHERE singleton AND issuer_id='https://cvera.app' AND claim_type='employment.role' AND manual_policy='MANUAL_REVIEW_PILOT_V1') THEN RAISE EXCEPTION 'canonical policy mismatch';END IF;
END $$;"""
# Existing authority migrations preserve their state machine, append-only and
# recording semantics. Strip hostile table defaults and PUBLIC function defaults
# only from these NEW objects, in the same transaction as their creation.
PRIVATE_AUTHORITY="""DO $$ DECLARE n text;p record;a record;BEGIN
 FOREACH n IN ARRAY ARRAY['authority_authorizations','authority_adjudications','authority_journal_events','authority_recording_boundary'] LOOP
 FOR a IN SELECT DISTINCT grantee FROM pg_catalog.aclexplode(coalesce((SELECT relacl FROM pg_catalog.pg_class WHERE oid=to_regclass('public.'||n)),pg_catalog.acldefault('r','postgres'::regrole))) WHERE grantee<>'postgres'::regrole LOOP
 EXECUTE format('REVOKE ALL ON TABLE public.%I FROM %s',n,CASE WHEN a.grantee=0 THEN 'PUBLIC' ELSE quote_ident(pg_get_userbyid(a.grantee)) END);END LOOP;
 END LOOP;
 n:=pg_catalog.pg_get_serial_sequence('public.authority_journal_events','journal_sequence');
 FOR a IN SELECT DISTINCT grantee FROM pg_catalog.aclexplode(coalesce((SELECT relacl FROM pg_catalog.pg_class WHERE oid=n::regclass),pg_catalog.acldefault('S','postgres'::regrole))) WHERE grantee<>'postgres'::regrole LOOP
 EXECUTE format('REVOKE ALL ON SEQUENCE %s FROM %s',n,CASE WHEN a.grantee=0 THEN 'PUBLIC' ELSE quote_ident(pg_get_userbyid(a.grantee)) END);END LOOP;
 FOR p IN SELECT oid,proowner FROM pg_catalog.pg_proc WHERE pronamespace='public'::regnamespace AND proname IN ('reject_authority_mutation','next_authority_recording_position','authority_evidence_position_count','validate_authority_evidence_recording','validate_authority_recording_allocation') LOOP
 FOR a IN SELECT DISTINCT grantee FROM pg_catalog.aclexplode(coalesce((SELECT proacl FROM pg_catalog.pg_proc WHERE oid=p.oid),pg_catalog.acldefault('f',p.proowner))) WHERE grantee<>p.proowner LOOP
 EXECUTE format('REVOKE ALL ON FUNCTION %s FROM %s',p.oid::regprocedure,CASE WHEN a.grantee=0 THEN 'PUBLIC' ELSE quote_ident(pg_get_userbyid(a.grantee)) END);END LOOP;
 END LOOP;
END $$;"""
VERIFY="""DO $$ DECLARE n text;r record;sig text;a record;expected oid; BEGIN
 FOREACH n IN ARRAY ARRAY['authority_authorizations','authority_adjudications','authority_journal_events','authority_recording_boundary','evidence_issuer_credentials','evidence_issued_credentials'] LOOP
 SELECT * INTO STRICT r FROM pg_catalog.pg_class WHERE oid=to_regclass('public.'||n);
 IF r.relowner<>'postgres'::regrole OR r.relrowsecurity OR r.relforcerowsecurity OR r.relkind<>'r'
 OR EXISTS(SELECT 1 FROM pg_catalog.aclexplode(coalesce(r.relacl,pg_catalog.acldefault('r',r.relowner))) WHERE grantee<>r.relowner)
 OR EXISTS(SELECT 1 FROM pg_catalog.pg_attribute WHERE attrelid=r.oid AND attacl IS NOT NULL)
 OR EXISTS(SELECT 1 FROM pg_catalog.pg_index WHERE indrelid=r.oid AND NOT(indisready AND indisvalid)) THEN RAISE EXCEPTION 'owner-only table boundary';END IF;
 END LOOP;
 SELECT * INTO STRICT r FROM pg_catalog.pg_class WHERE oid=pg_catalog.pg_get_serial_sequence('public.authority_journal_events','journal_sequence')::regclass;
 IF r.relowner<>'postgres'::regrole OR EXISTS(SELECT 1 FROM pg_catalog.aclexplode(coalesce(r.relacl,pg_catalog.acldefault('S',r.relowner))) WHERE grantee<>r.relowner) THEN RAISE EXCEPTION 'private authority sequence required';END IF;
 FOREACH sig IN ARRAY ARRAY['active_evidence_signing_credential(text)','read_evidence_authority()','prepare_bound_issuance(uuid,uuid,text,text)','commit_bound_issuance(uuid,uuid,text,text,text,jsonb)','resolve_bound_credential(text)','guard_evidence_issuance()','revoke_bound_credential(uuid,text,text)','protect_evidence_issuance()','reject_authority_mutation()','next_authority_recording_position()','authority_evidence_position_count(bigint)','validate_authority_evidence_recording()','validate_authority_recording_allocation()'] LOOP
 SELECT * INTO STRICT r FROM pg_catalog.pg_proc WHERE oid=to_regprocedure('public.'||sig);
 expected:=CASE WHEN r.proname IN ('active_evidence_signing_credential','read_evidence_authority','prepare_bound_issuance','commit_bound_issuance','resolve_bound_credential') THEN 'cvera_api_runtime'::regrole ELSE NULL END;
 IF r.proowner<>'postgres'::regrole THEN RAISE EXCEPTION 'function owner mismatch';END IF;
 IF r.proname IN ('active_evidence_signing_credential','read_evidence_authority','prepare_bound_issuance','commit_bound_issuance','resolve_bound_credential','guard_evidence_issuance','revoke_bound_credential') AND (NOT r.prosecdef OR NOT coalesce('search_path=pg_catalog, public, pg_temp'=ANY(r.proconfig),false)) THEN RAISE EXCEPTION 'definer path mismatch';END IF;
 IF r.proname='protect_evidence_issuance' AND (r.prosecdef OR NOT coalesce('search_path=pg_catalog, public, pg_temp'=ANY(r.proconfig),false)) THEN RAISE EXCEPTION 'private invoker trigger mismatch';END IF;
 IF r.proname='next_authority_recording_position' AND (NOT r.prosecdef OR NOT coalesce('search_path=public, pg_temp'=ANY(r.proconfig),false)) THEN RAISE EXCEPTION 'allocator path mismatch';END IF;
 FOR a IN SELECT * FROM pg_catalog.aclexplode(coalesce(r.proacl,pg_catalog.acldefault('f',r.proowner))) WHERE grantee<>r.proowner LOOP
 IF expected IS NULL OR a.grantee<>expected OR a.privilege_type<>'EXECUTE' OR a.is_grantable THEN RAISE EXCEPTION 'excess function privilege';END IF;END LOOP;
 IF expected IS NOT NULL AND NOT has_function_privilege(expected,r.oid,'EXECUTE') THEN RAISE EXCEPTION 'missing runtime execute';END IF;
 END LOOP;
 IF EXISTS(SELECT 1 FROM public.authority_authorizations) OR EXISTS(SELECT 1 FROM public.authority_adjudications) OR EXISTS(SELECT 1 FROM public.authority_journal_events)
 OR (SELECT committed_position FROM public.authority_recording_boundary WHERE singleton)<>0 OR (SELECT count(*) FROM public.authority_recording_boundary)<>1
 OR EXISTS(SELECT 1 FROM public.evidence_issuer_credentials) OR EXISTS(SELECT 1 FROM public.evidence_issued_credentials) THEN RAISE EXCEPTION 'unexpected authority/credential activation';END IF;
END $$;
SET LOCAL ROLE cvera_api_runtime;
DO $$ DECLARE snapshot jsonb;BEGIN
 snapshot:=public.read_evidence_authority();
 IF snapshot->>'RecordingPosition'<>'0' OR snapshot->'Entries'<>'[]'::jsonb OR public.active_evidence_signing_credential('issuer-prod-2026-10') IS NOT NULL THEN RAISE EXCEPTION 'empty trust must fail closed';END IF;
 BEGIN PERFORM public.prepare_bound_issuance(NULL::uuid,'00000000-0000-0000-0000-000000000000'::uuid,'__missing__','issuer-prod-2026-10');RAISE EXCEPTION 'missing principal permitted';EXCEPTION WHEN SQLSTATE 'P0002' THEN NULL;END;
END $$;RESET ROLE;"""
# Existing objects/privileges and role memberships must remain byte-for-byte.
FINGERPRINT="""SELECT encode(sha256(convert_to(jsonb_build_object(
 'objects',(SELECT jsonb_agg(jsonb_build_array(c.oid,c.relname,c.relowner,c.relacl,c.relrowsecurity,c.relforcerowsecurity) ORDER BY c.oid) FROM pg_catalog.pg_class c WHERE c.relnamespace='public'::regnamespace AND c.relname NOT LIKE 'authority_%' AND c.relname NOT LIKE 'evidence_issu%'),
 'functions',(SELECT jsonb_agg(jsonb_build_array(p.oid,p.proowner,p.proacl,p.proconfig) ORDER BY p.oid) FROM pg_catalog.pg_proc p WHERE p.pronamespace='public'::regnamespace AND p.proname NOT IN ('reject_authority_mutation','next_authority_recording_position','authority_evidence_position_count','validate_authority_evidence_recording','validate_authority_recording_allocation','active_evidence_signing_credential','read_evidence_authority','prepare_bound_issuance','commit_bound_issuance','resolve_bound_credential','guard_evidence_issuance','revoke_bound_credential','protect_evidence_issuance')),
 'roles',(SELECT jsonb_agg(jsonb_build_array(oid,rolname,rolsuper,rolcanlogin,rolcreatedb,rolcreaterole,rolbypassrls) ORDER BY oid) FROM pg_catalog.pg_roles),
 'memberships',(SELECT coalesce(jsonb_agg(to_jsonb(m) ORDER BY roleid,member),'[]'::jsonb) FROM pg_catalog.pg_auth_members m),
 'schema_acl',(SELECT nspacl FROM pg_catalog.pg_namespace WHERE nspname='public'))::text,'UTF8')),'hex')"""

def transaction(sources,final):
 return 'BEGIN ISOLATION LEVEL READ COMMITTED;'+SETTINGS+PREFLIGHT+'\n'+''.join(sources[n]+'\n' for n in FILES if n.startswith('migrations/0000') and '000019' not in n)+PRIVATE_AUTHORITY+sources['migrations/000019_evidence_backed_issuance.sql']+"SET LOCAL cvera.evidence_runtime_role='cvera_api_runtime';"+sources['operations/evidence-issuance-privileges.sql']+VERIFY+'SELECT txid_current();'+final+';'

def sources():
 result={}
 for name,expected in FILES.items():
  data=run(['git','-C',str(ROOT),'show',SOURCE+':apps/evt-api-go/'+name])
  if hashlib.sha256(data).hexdigest()!=expected:raise RuntimeError('reviewed SQL hash mismatch')
  result[name]=data.decode()
 return result

def services():
 names=[DB,'evt-api-go-api-1','evt-api-go-kratos-1','evt-api-go-kratos-courier-1']
 if not all(info(n,'State.Running') and not info(n,'State.OOMKilled') for n in names):raise RuntimeError('unhealthy service')
 return {n:[info(n,'Id'),info(n,'Image'),info(n,'State.StartedAt')] for n in names}

def baseline():
 if run(['git','-C',str(ROOT),'rev-parse','HEAD']).decode().strip()!=BASE or run(['git','-C',str(ROOT),'status','--porcelain']).strip():raise RuntimeError('unexpected checkout')
 for endpoint,key,value in [('version','git_sha',BASE),('healthz','ok','true')]:
  with urllib.request.urlopen('http://127.0.0.1:8080/'+endpoint,timeout=5) as response:
   if json.load(response).get(key)!=value:raise RuntimeError('unexpected running API')
 env=dict(x.split('=',1) for x in info('evt-api-go-api-1','Config.Env') if '=' in x)
 uri=urllib.parse.urlsplit(env.get('EVT_DB_DSN',''));q=urllib.parse.parse_qs(uri.query)
 if urllib.parse.unquote(uri.username or '')!='cvera_api_runtime' or uri.path!='/evt' or set(q)-{'sslmode','search_path'} or q.get('search_path',['public'])!=['public'] or env.get('PGOPTIONS') or env.get('PGSERVICE'):raise RuntimeError('runtime role/path mismatch')
 return services()

def rehearse(folder,artifacts):
 global PHASE
 name='cvera-issuance-restore-'+uuid.uuid4().hex[:12];image=info(DB,'Image')
 try:
  run(['docker','run','-d','--pull','never','--name',name,'--network','none','--memory','64m','--memory-swap','64m','--cpus','0.5','--shm-size','16m','--label','cvera.scope=issuance-schema-rehearsal','-e','POSTGRES_HOST_AUTH_METHOD=trust','-e','POSTGRES_INITDB_ARGS=-c shared_buffers=8MB -c max_connections=10 -c maintenance_work_mem=4MB -c autovacuum=off',image,'-c','shared_buffers=8MB','-c','max_connections=10','-c','work_mem=512kB','-c','maintenance_work_mem=4MB','-c','autovacuum=off','-c','max_worker_processes=0','-c','max_parallel_workers=0'])
  for _ in range(60):
   r=subprocess.run(['docker','exec',name,'psql','-X','-h','127.0.0.1','-U','postgres','-Atqc','SELECT 1'],capture_output=True,timeout=5)
   if r.returncode==0:break
   time.sleep(1)
  else:raise RuntimeError('clone not ready')
  if info(name,'HostConfig.NetworkMode')!='none' or info(name,'Image')!=image or {m['Source'] for m in info(name,'Mounts')}.intersection(m['Source'] for m in info(DB,'Mounts')):raise RuntimeError('clone isolation')
  roles=read("SELECT format('CREATE ROLE %I %s;',rolname,CASE WHEN rolcanlogin THEN 'LOGIN' ELSE 'NOLOGIN' END) FROM pg_catalog.pg_roles WHERE rolname<>'postgres' AND rolname !~ '^pg_' ORDER BY rolname")
  run(['docker','exec','-i',name,'psql','-X','-U','postgres','-d','postgres','-v','ON_ERROR_STOP=1'],roles+b'\nCREATE DATABASE evt OWNER postgres;')
  with (folder/'evt.dump').open('rb') as stream:
   r=subprocess.run(['docker','exec','-i',name,'pg_restore','-U','postgres','-d','evt','--exit-on-error'],stdin=stream,capture_output=True,timeout=180)
  if r.returncode:raise RuntimeError('restore failed')
  before=read(FINGERPRINT,name)
  PHASE='clone_schema_rollback'
  sql(transaction(artifacts,'ROLLBACK'),name);read(PREFLIGHT,name)
  PHASE='clone_schema_commit'
  sql(transaction(artifacts,'COMMIT'),name);sql('BEGIN;'+SETTINGS+VERIFY+'ROLLBACK;',name)
  PHASE='clone_normal_request_review_and_receipt_probe'
  sql(artifacts['operations/evidence-issuance-restore-probe.sql'],name)
  sql('BEGIN;'+SETTINGS+VERIFY+'ROLLBACK;',name)
  if read(FINGERPRINT,name)!=before or info(name,'State.OOMKilled'):raise RuntimeError('parent boundary changed')
  private(folder/'restore-rehearsal.json',json.dumps({'restore':'PASS','rollback':'PASS','commit':'PASS','privileges':'PASS','empty_authority_and_credential_fail_closed':'PASS','clone':name,'image':image,'memory_limit_mib':64}).encode())
 finally:
  r=subprocess.run(['docker','inspect',name],capture_output=True,timeout=10)
  if r.returncode==0:
   if info(name,'Config.Labels').get('cvera.scope')!='issuance-schema-rehearsal' or info(name,'HostConfig.NetworkMode')!='none':raise RuntimeError('unexpected clone')
   run(['docker','stop','--time','10',name],timeout=30)

def prepare(artifacts):
 global PHASE
 before=baseline();PHASE='production_schema_preflight';read(PREFLIGHT);fingerprint=read(FINGERPRINT).decode().strip()
 PHASE='backup_restore_capacity'
 size=int(read('SELECT pg_database_size(current_database())'))
 if shutil.disk_usage('/opt/cvera').free<size*4+1024**3:raise RuntimeError('backup capacity')
 memory=next(int(x.split()[1]) for x in Path('/proc/meminfo').read_text().splitlines() if x.startswith('MemAvailable:'))
 if memory<128*1024:
  DIAGNOSTIC.update(available_memory_kib=memory,required_memory_kib=128*1024);raise RuntimeError('restore headroom')
 folder=Path('/opt/cvera/issuance-release-'+datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ')+'-'+uuid.uuid4().hex[:6]);folder.mkdir(mode=0o700)
 private(folder/'intent.json',json.dumps({'source':SOURCE,'base':BASE,'hashes':FILES,'scope':'read-only production SQL, backup and isolated rehearsal; no activation'}).encode())
 PHASE='backup'
 with (folder/'evt.dump').open('xb') as stream:
  os.chmod(folder/'evt.dump',0o600)
  r=subprocess.run(['docker','exec',DB,'pg_dump','-U','postgres','-d','evt','-Fc'],stdout=stream,stderr=subprocess.PIPE,timeout=180);stream.flush();os.fsync(stream.fileno())
 if r.returncode:raise RuntimeError('backup failed')
 checksum=digest(folder/'evt.dump');private(folder/'backup.sha256',(checksum+'  evt.dump\n').encode())
 tag='cvera-api:recovery-issuance-'+folder.name.removeprefix('issuance-release-').lower();run(['docker','image','tag',before['evt-api-go-api-1'][1],tag])
 if info('evt-api-go-api-1','Image')!=json.loads(run(['docker','image','inspect','--format','{{json .Id}}',tag])):raise RuntimeError('recovery image mismatch')
 env=ROOT/'apps/evt-api-go/.env';secure(env,0o600)
 private(folder/'api.env.before',env.read_bytes());private(folder/'compose-before.yml',(ROOT/'apps/evt-api-go/docker-compose.yml').read_bytes())
 private(folder/'api-image-override.json',json.dumps({'services':{'api':{'image':tag}}}).encode())
 PHASE='isolated_same_image_rehearsal';rehearse(folder,artifacts)
 PHASE='final_preparation_verification'
 if digest(folder/'evt.dump')!=checksum or read(FINGERPRINT).decode().strip()!=fingerprint or baseline()!=before:raise RuntimeError('source changed')
 record={'issuance_schema_preparation':'PASS','source':SOURCE,'base':BASE,'hashes':FILES,'backup_sha256':checksum,'fingerprint':fingerprint,'services':before,'env_sha256':digest(env),'production_sql_read_only':True,'authority_or_credentials_created':False,'schema_applied':False,'private_record':str(folder/'preparation.json')}
 private(folder/'preparation.json',json.dumps(record).encode());print(json.dumps({'issuance_schema_preparation':'PASS','source':SOURCE,'authority_bootstrap':'000002-000013 only','migration':'000019 only','fresh_private_backup_restored':True,'rollback_commit_and_workflow_probe':'PASS','runtime_five_executes_only':True,'production_sql_read_only':True,'authority_or_credentials_created':False,'schema_applied':False,'services_changed':False,'private_record':str(folder/'preparation.json')}))

def apply(folder,artifacts,approval):
 global PHASE
 secure(folder,0o700)
 lock=os.open(folder/'apply.lock',os.O_CREAT|os.O_RDWR|os.O_NOFOLLOW,0o600);fcntl.flock(lock,fcntl.LOCK_EX|fcntl.LOCK_NB)
 for n in ('preparation.json','restore-rehearsal.json','evt.dump','api.env.before'):secure(folder/n,0o600)
 if (folder/'sql-intent.json').exists() or (folder/'sql-commit.json').exists():raise RuntimeError('prior application/ambiguous commit; never retry')
 p=json.loads((folder/'preparation.json').read_text());rehearsal=json.loads((folder/'restore-rehearsal.json').read_text())
 if p['issuance_schema_preparation']!='PASS' or p['source']!=SOURCE or p['hashes']!=FILES or digest(folder/'evt.dump')!=p['backup_sha256'] or any(rehearsal[k]!='PASS' for k in ('restore','rollback','commit','privileges')):raise RuntimeError('preparation mismatch')
 if baseline()!=p['services'] or read(FINGERPRINT).decode().strip()!=p['fingerprint'] or digest(ROOT/'apps/evt-api-go/.env')!=p['env_sha256']:raise RuntimeError('production state changed since rehearsal')
 read(PREFLIGHT)
 private(folder/'sql-intent.json',json.dumps({'approval_ref':approval,'source':SOURCE,'hashes':FILES,'scope':'000002-000013 bootstrap plus 000019 and five EXECUTEs only; no 000001/history or activation'}).encode())
 PHASE='approved_schema_transaction';tx=sql(transaction(artifacts,'COMMIT')).decode().strip()
 private(folder/'sql-commit.json',json.dumps({'transaction':tx,'source':SOURCE,'approval_ref':approval,'verification':'PENDING'}).encode())
 PHASE='post_commit_verification';sql('BEGIN;'+SETTINGS+VERIFY+'ROLLBACK;')
 if read(FINGERPRINT).decode().strip()!=p['fingerprint'] or baseline()!=p['services']:raise RuntimeError('unexpected parent/service change')
 result={'issuance_schema':'PASS','source':SOURCE,'transaction':tx,'authority_bootstrap':'000002-000013 only','migration':'000019 only','runtime':'five EXECUTEs; no new table access','authority_or_credentials_created':False,'services_restarted':False,'private_record':str(folder/'sql-verification.json')}
 private(folder/'sql-verification.json',json.dumps(result).encode());print(json.dumps(result))

def main():
 parser=argparse.ArgumentParser();g=parser.add_mutually_exclusive_group(required=True);g.add_argument('--prepare',action='store_true');g.add_argument('--apply-approved-schema',type=Path);parser.add_argument('--approval-ref')
 args=parser.parse_args()
 if os.geteuid()!=0:raise RuntimeError('existing owner root console required')
 os.umask(0o077);artifacts=sources()
 if args.prepare:prepare(artifacts)
 else:
  if not args.approval_ref or not args.approval_ref.strip():raise RuntimeError('separate schema approval reference required')
  apply(args.apply_approved_schema,artifacts,args.approval_ref)
if __name__=='__main__':
 try:main()
 except Exception:
  print(json.dumps({'issuance_schema':'STOP','phase':PHASE,'diagnostic':DIAGNOSTIC,'instruction':'Do not retry or print private output; preserve preparation/intent for read-only reconciliation.'}));raise SystemExit(1)
