#!/usr/bin/env python3
"""Approved read-only production preflight + private backup/isolated rehearsal.

Never registers an employer or records authority in production. Only clone_sql
can execute the pinned registry INSERT; source_read always wraps READ ONLY.
"""
import datetime,hashlib,json,os,shutil,stat,subprocess,sys,time,urllib.request,uuid
from pathlib import Path
ROOT=Path('/opt/cvera/rsa-attestation-engine')
DB='evt-api-go-db-1'
BASE='e84b9a64af8225e439e0c4b1029d561181a37aa8'
SOURCE='8e1add11de7983aacac52a6b2dc8b806f1d1360e'
LABEL='norstar-production-backup-rehearsal'
PHASE='preflight';BACKUP=None;DETAILS={}
FILES={'norstar-preparation-preflight.sql':'819053402d4876e54229789058a0ff5bf4de4d5f0965f14bfa70002e8cba331e',
 'register-norstar-employer.sql':'0c693f55f3752da59726833ffe5673ba150b7f16f8d1ccb4b5b163d3fbd0eb1b'}

def run(args,data=None,timeout=120):
 r=subprocess.run(args,input=data,capture_output=True,timeout=timeout)
 if r.returncode:raise RuntimeError('command failed; raw output suppressed')
 return r.stdout

def info(name,key):return json.loads(run(['docker','inspect','--format','{{json .'+key+'}}',name]))
def private(path,data):
 fd=os.open(path,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
 with os.fdopen(fd,'wb') as stream:stream.write(data);stream.flush();os.fsync(stream.fileno())
def digest(path):
 h=hashlib.sha256()
 with path.open('rb') as stream:
  for block in iter(lambda:stream.read(1024*1024),b''):h.update(block)
 return h.hexdigest()
def source_read(query):
 body="BEGIN READ ONLY;SET LOCAL search_path=pg_catalog;SET LOCAL statement_timeout='30s';SET LOCAL lock_timeout='3s';"+query+';ROLLBACK;'
 return run(['docker','exec','-i',DB,'psql','-X','-U','postgres','-d','evt','-Atq','-v','ON_ERROR_STOP=1'],body.encode())
def metadata(preflight):
 # The pinned file itself starts BEGIN READ ONLY and ends ROLLBACK.
 return json.loads(run(['docker','exec','-i',DB,'psql','-X','-U','postgres','-d','evt','-Atq','-v','ON_ERROR_STOP=1'],preflight.encode()))
CATALOG="""SELECT jsonb_build_object(
 'registry',(SELECT coalesce(jsonb_agg(to_jsonb(c) ORDER BY employer_id),'[]'::jsonb) FROM public.evt_employer_config c),
 'roles',(SELECT jsonb_agg(jsonb_build_array(oid,rolname,rolsuper,rolcanlogin,rolcreatedb,rolcreaterole,rolbypassrls) ORDER BY oid) FROM pg_catalog.pg_roles),
 'memberships',(SELECT coalesce(jsonb_agg(to_jsonb(m) ORDER BY roleid,member),'[]'::jsonb) FROM pg_catalog.pg_auth_members m),
 'acl',(SELECT jsonb_agg(jsonb_build_array(c.oid,c.relname,c.relowner,c.relacl,c.relrowsecurity,c.relforcerowsecurity) ORDER BY c.oid) FROM pg_catalog.pg_class c WHERE c.relnamespace='public'::regnamespace),
 'schema_acl',(SELECT nspacl FROM pg_catalog.pg_namespace WHERE oid='public'::regnamespace))"""

def validate(meta):
 if meta['database']!='evt' or meta['session_role']!='postgres' or not meta['server_version'].startswith('16.') or meta['registry_owner']!='postgres' or meta['registry_rls']:raise RuntimeError('unexpected production schema/owner')
 if meta['exact_id_collision'] or meta['candidate_employer_ids']:raise RuntimeError('existing candidate/mapping requires reconciliation; no duplicate registration')
 if meta['user_insert_trigger_count'] or meta['rewrite_rule_count']:raise RuntimeError('unreviewed registry side effects')
 columns={x['name']:x for x in meta['registry_columns']}
 expected={'employer_id':'text','dispatch_mode':'text','created_at':'timestamp with time zone','updated_at':'timestamp with time zone'}
 if set(columns)!=set(expected) or any(columns[k]['type']!=v or not columns[k]['not_null'] or columns[k]['default_review']=='UNREVIEWED' for k,v in expected.items()):raise RuntimeError('registry DDL/default review required')
 if not any(x['type']=='p' and x['validated'] for x in meta['registry_constraints']):raise RuntimeError('registry primary key required')
 if any(not x['validated'] for x in meta['registry_constraints']):raise RuntimeError('unvalidated registry constraint')


def clone_sql(name,body):
 return run(['docker','exec','-i',name,'psql','-X','-U','postgres','-d','evt','-Atq','-v','ON_ERROR_STOP=1'],body.encode())
def clone_rows(name):return json.loads(clone_sql(name,"BEGIN READ ONLY;SELECT coalesce(jsonb_agg(to_jsonb(c) ORDER BY employer_id),'[]'::jsonb) FROM public.evt_employer_config c;ROLLBACK;"))
def other_data(name):
 tables=json.loads(clone_sql(name,"BEGIN READ ONLY;SELECT coalesce(jsonb_agg(format('%I.%I',n.nspname,c.relname) ORDER BY c.relname),'[]'::jsonb) FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND c.relkind IN ('r','p') AND c.relname<>'evt_employer_config';ROLLBACK;"))
 result={}
 for table in tables:
  query="SELECT encode(sha256(convert_to(coalesce(string_agg(h,'' ORDER BY h),''),'UTF8')),'hex') FROM (SELECT encode(sha256(convert_to(to_jsonb(t)::text,'UTF8')),'hex') h FROM "+table+" t) s"
  result[table]=clone_sql(name,'BEGIN READ ONLY;'+query+';ROLLBACK;').strip().decode()
 return result

def rehearse(backup,image,role_stubs,register,clone_name=None):
 name=clone_name or 'cvera-norstar-restore-'+uuid.uuid4().hex[:12]
 if name==DB or not name.startswith('cvera-norstar-'):raise RuntimeError('invalid isolated clone name')
 try:
  run(['docker','run','-d','--pull','never','--name',name,'--network','none','--memory','64m','--memory-swap','64m','--cpus','0.5','--shm-size','16m','--label','cvera.scope='+LABEL,
   '-e','POSTGRES_HOST_AUTH_METHOD=trust','-e','POSTGRES_INITDB_ARGS=-c shared_buffers=8MB -c max_connections=10 -c maintenance_work_mem=4MB -c autovacuum=off',image,
   '-c','shared_buffers=8MB','-c','max_connections=10','-c','work_mem=512kB','-c','maintenance_work_mem=4MB','-c','autovacuum=off','-c','max_worker_processes=0','-c','max_parallel_workers=0'])
  for _ in range(60):
   r=subprocess.run(['docker','exec',name,'psql','-X','-h','127.0.0.1','-U','postgres','-Atqc','SELECT 1'],capture_output=True,timeout=5)
   if r.returncode==0:break
   time.sleep(1)
  else:raise RuntimeError('isolated restore not ready')
  if info(name,'HostConfig.NetworkMode')!='none' or info(name,'Image')!=image:raise RuntimeError('restore isolation/image mismatch')
  if {x['Source'] for x in info(name,'Mounts')}.intersection({x['Source'] for x in info(DB,'Mounts')}):raise RuntimeError('shared source volume')
  run(['docker','exec','-i',name,'psql','-X','-U','postgres','-d','postgres','-q','-v','ON_ERROR_STOP=1'],role_stubs)
  run(['docker','exec',name,'createdb','-U','postgres','evt'])
  with (backup/'evt.dump').open('rb') as stream:
   r=subprocess.run(['docker','exec','-i',name,'pg_restore','-U','postgres','-d','evt','--exit-on-error'],stdin=stream,capture_output=True,timeout=180)
  if r.returncode:raise RuntimeError('backup restore failed')
  before_rows=clone_rows(name);before_catalog=clone_sql(name,'BEGIN READ ONLY;'+CATALOG+';ROLLBACK;');before_other=other_data(name)
  # These are CLONE-ONLY execution fixtures, not real approval/mapping evidence.
  settings="SET cvera.norstar_employer_id='norstar-systems-llc';SET cvera.norstar_mapping_ref='LOCAL RESTORE REHEARSAL ONLY';SET cvera.norstar_approval_ref='LOCAL RESTORE REHEARSAL ONLY';"
  if not register.rstrip().endswith('COMMIT;'):raise RuntimeError('unexpected registration template')
  rollback=register.rstrip()[:-len('COMMIT;')]+'ROLLBACK;'
  response=json.loads(clone_sql(name,settings+rollback))
  if response['dispatch_mode']!='MANUAL' or clone_sql(name,'BEGIN READ ONLY;'+CATALOG+';ROLLBACK;')!=before_catalog or other_data(name)!=before_other:raise RuntimeError('registry rollback rehearsal mismatch')
  response=json.loads(clone_sql(name,settings+register));after_rows=clone_rows(name)
  added=[r for r in after_rows if r['employer_id']=='norstar-systems-llc']
  if response['authority_or_credentials_created'] is not False or len(added)!=1 or added[0]['dispatch_mode']!='MANUAL' or [r for r in after_rows if r['employer_id']!='norstar-systems-llc']!=before_rows or other_data(name)!=before_other:raise RuntimeError('registry commit changed unrelated data')
  # Compare role/ACL/schema catalog, excluding the single rehearsed registry row.
  after_catalog=json.loads(clone_sql(name,'BEGIN READ ONLY;'+CATALOG+';ROLLBACK;'));old=json.loads(before_catalog)
  for k in ('roles','memberships','acl','schema_acl'):
   if after_catalog[k]!=old[k]:raise RuntimeError('rehearsal changed privileges/catalog')
  if info(name,'State.OOMKilled'):raise RuntimeError('restore memory failure')
  result={'restore':'PASS','rollback':'PASS','commit':'PASS','mode':'MANUAL','unrelated_data_and_privileges_preserved':True,'network':'none','memory_limit_mib':64,'clone':name}
  private(backup/'restore-rehearsal.json',json.dumps(result).encode())
  return result
 finally:
  r=subprocess.run(['docker','inspect',name],capture_output=True,timeout=10)
  if r.returncode==0:
   item=json.loads(r.stdout)[0]
   if item['HostConfig']['NetworkMode']!='none' or item['Config']['Labels'].get('cvera.scope')!=LABEL:raise RuntimeError('unexpected clone; do not touch')
   run(['docker','stop','--time','10',name],timeout=30)


def main():
 global PHASE,BACKUP
 if os.geteuid()!=0 or sys.argv[1:]!=['--approved-preparation']:raise RuntimeError('existing root console and approved preparation scope required')
 os.umask(0o077)
 if run(['git','-C',str(ROOT),'status','--porcelain']).strip() or run(['git','-C',str(ROOT),'rev-parse','HEAD']).decode().strip()!=BASE:raise RuntimeError('unexpected running checkout')
 with urllib.request.urlopen('http://127.0.0.1:8080/version',timeout=5) as response:
  if json.load(response).get('git_sha')!=BASE:raise RuntimeError('unexpected running API')
 services=[DB,'evt-api-go-api-1','evt-api-go-kratos-1','evt-api-go-kratos-courier-1']
 baseline={n:(info(n,'Id'),info(n,'Image'),info(n,'State.StartedAt')) for n in services}
 if not all(info(n,'State.Running') and not info(n,'State.OOMKilled') for n in services):raise RuntimeError('unexpected service health')
 sources={}
 for filename,expected in FILES.items():
  data=run(['git','-C',str(ROOT),'show',SOURCE+':apps/evt-api-go/operations/'+filename])
  if hashlib.sha256(data).hexdigest()!=expected:raise RuntimeError('reviewed SQL hash mismatch')
  sources[filename]=data.decode()
 PHASE='read_only_schema_collision_and_mapping_checks'
 meta=metadata(sources['norstar-preparation-preflight.sql']);validate(meta)
 # This can discover candidate mappings, not invent a legal identity column.
 number=source_read("SELECT EXISTS(SELECT 1 FROM public.evt_requests WHERE claim_snapshot::text ILIKE '%B20250427754%')").strip()
 if number!=b'f':raise RuntimeError('existing legal-number candidate requires mapping reconciliation')
 before=source_read(CATALOG)
 size=int(source_read('SELECT pg_database_size(current_database())').strip())
 available=shutil.disk_usage('/opt/cvera').free;required=size*4+1024**3
 if available<required:
  DETAILS.update(available_disk_bytes=available,required_disk_bytes=required);raise RuntimeError('backup capacity')
 mem=next(int(x.split()[1]) for x in Path('/proc/meminfo').read_text().splitlines() if x.startswith('MemAvailable:'))
 if mem<128*1024:
  DETAILS.update(available_memory_kib=mem,required_memory_kib=128*1024);raise RuntimeError('restore memory/headroom')
 stamp=datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ')
 BACKUP=Path('/opt/cvera/norstar-preparation-'+stamp+'-'+uuid.uuid4().hex[:6]);BACKUP.mkdir(mode=0o700)
 private(BACKUP/'intent.json',json.dumps({'scope':'read-only production + backup + isolated clone only','sql_source':SOURCE,'registration_executed':False,'authority_recording_executed':False}).encode())
 private(BACKUP/'schema-preflight.json',json.dumps(meta).encode())
 private(BACKUP/'registry-and-privilege-inventory.private.json',before)
 PHASE='fresh_private_backup'
 archive=BACKUP/'evt.dump'
 with archive.open('xb') as stream:
  os.chmod(archive,0o600)
  r=subprocess.run(['docker','exec',DB,'pg_dump','-U','postgres','-d','evt','-Fc'],stdout=stream,stderr=subprocess.PIPE,timeout=180);stream.flush();os.fsync(stream.fileno())
 if r.returncode:raise RuntimeError('backup failed')
 checksum=digest(archive);private(BACKUP/'backup.sha256',(checksum+'  evt.dump\n').encode())
 with archive.open('rb') as stream:
  r=subprocess.run(['docker','exec','-i',DB,'pg_restore','--list'],stdin=stream,capture_output=True,timeout=30)
 if r.returncode:raise RuntimeError('archive listing failed')
 private(BACKUP/'archive-list.private',r.stdout)
 stubs=source_read("SELECT format('CREATE ROLE %I NOLOGIN;',rolname) FROM pg_catalog.pg_roles WHERE rolname<>'postgres' AND rolname !~ '^pg_' ORDER BY rolname")
 private(BACKUP/'clone-role-stubs.sql',stubs)
 PHASE='isolated_same_image_restore_and_registry_rehearsal'
 rehearse(BACKUP,baseline[DB][1],stubs,sources['register-norstar-employer.sql'])
 PHASE='final_read_only_verification'
 if digest(archive)!=checksum or source_read(CATALOG)!=before:raise RuntimeError('source registry/privileges changed during preparation')
 final=metadata(sources['norstar-preparation-preflight.sql']);validate(final)
 for n,previous in baseline.items():
  if (info(n,'Id'),info(n,'Image'),info(n,'State.StartedAt'))!=previous or not info(n,'State.Running'):raise RuntimeError('production services changed')
 if any(stat.S_IMODE(p.stat().st_mode)!=0o600 or p.stat().st_uid!=0 for p in BACKUP.iterdir() if p.is_file()) or stat.S_IMODE(BACKUP.stat().st_mode)!=0o700:raise RuntimeError('private file permissions')
 result={'norstar_preparation':'PASS','sql_source':SOURCE,'schema_and_collision_checks':'PASS','candidate_mappings_found':0,
  'legal_mapping':'No verified existing mapping discovered; owner registration approval must establish mapping, not infer legal proof from name search',
  'fresh_private_backup':'PASS','isolated_same_image_restore':'PASS','registry_rollback_and_commit_rehearsal':'PASS','proposed_dispatch_mode':'MANUAL',
  'production_sql_read_only':True,'source_registry_and_privileges_unchanged':True,'registration_executed':False,'authority_recording_executed':False,
  'services_restarted':False,'credentials_or_signing_used':False,'private_record':str(BACKUP/'preparation-result.json')}
 private(BACKUP/'preparation-result.json',json.dumps(result).encode());print(json.dumps(result))
if __name__=='__main__':
 try:main()
 except Exception as error:
  reasons={'backup capacity':'INSUFFICIENT_BACKUP_DISK','restore memory/headroom':'INSUFFICIENT_RESTORE_MEMORY_HEADROOM','existing candidate/mapping requires reconciliation; no duplicate registration':'EXISTING_EMPLOYER_CANDIDATE','existing legal-number candidate requires mapping reconciliation':'EXISTING_LEGAL_ENTITY_NUMBER_MATCH','registry DDL/default review required':'UNEXPECTED_REGISTRY_DDL_OR_DEFAULT','unreviewed registry side effects':'UNREVIEWED_INSERT_TRIGGER_OR_RULE','source registry/privileges changed during preparation':'SOURCE_REGISTRY_OR_PRIVILEGES_CHANGED','unexpected running checkout':'UNEXPECTED_RUNNING_CHECKOUT','unexpected running API':'UNEXPECTED_RUNNING_API','unexpected production schema/owner':'UNEXPECTED_SCHEMA_OR_OWNER'}
  reason=reasons.get(str(error),'OPERATION_FAILED_RAW_DETAILS_SUPPRESSED') if type(error) is RuntimeError else 'OPERATION_FAILED_RAW_DETAILS_SUPPRESSED'
  print(json.dumps({'norstar_preparation':'STOP','reason':reason,'diagnostic_metadata':DETAILS,'phase':PHASE,'private_directory':str(BACKUP) if BACKUP else None,'instruction':'Return only this status; do not retry, register, record authority or disclose private files.'}),file=sys.stderr);sys.exit(1)
