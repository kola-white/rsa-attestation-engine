#!/usr/bin/env python3
"""Local-only, network-none SQL regression; never opens a production connection."""
import datetime,json,subprocess,time,uuid
from pathlib import Path
ROOT=Path(__file__).resolve().parents[1]
NAME='cvera-norstar-local-'+uuid.uuid4().hex[:12]
LABEL='norstar-preparation-local-test'
def run(args,data=None,check=True):
 r=subprocess.run(args,input=data,text=True,capture_output=True,timeout=60)
 if check and r.returncode:raise RuntimeError('isolated local SQL regression failed; raw output suppressed')
 return r

def sql(body,check=True):
 return run(['docker','exec','-i',NAME,'psql','-X','-U','postgres','-d','evt','-Atq','-v','ON_ERROR_STOP=1'],body,check)

def main():
 try:
  run(['docker','run','-d','--pull','never','--name',NAME,'--network','none','--memory','64m','--memory-swap','64m','--cpus','0.5','--shm-size','16m','--label','cvera.scope='+LABEL,
   '-e','POSTGRES_HOST_AUTH_METHOD=trust','-e','POSTGRES_INITDB_ARGS=-c shared_buffers=8MB -c max_connections=10 -c maintenance_work_mem=4MB -c autovacuum=off','postgres:16',
   '-c','shared_buffers=8MB','-c','max_connections=10','-c','work_mem=512kB','-c','maintenance_work_mem=4MB','-c','autovacuum=off','-c','max_worker_processes=0','-c','max_parallel_workers=0'])
  for _ in range(60):
   if run(['docker','exec',NAME,'psql','-X','-U','postgres','-Atqc','SELECT 1'],check=False).returncode==0:break
   time.sleep(1)
  else:raise RuntimeError('isolated local PostgreSQL not ready')
  run(['docker','exec',NAME,'psql','-X','-U','postgres','-v','ON_ERROR_STOP=1','-c','CREATE DATABASE evt'])
  sql("CREATE TABLE public.evt_employer_config(employer_id text PRIMARY KEY,dispatch_mode text NOT NULL DEFAULT 'AUTO',created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now());")
  for path in sorted((ROOT/'migrations').glob('*.sql')):
   if '000002'<=path.name<'000014':sql(path.read_text())
  register=(ROOT/'operations/register-norstar-employer.sql').read_text()
  record=(ROOT/'operations/record-norstar-authority-approved.sql').read_text()
  # Explicit LOCAL fixtures. These are not evidence or decisions about real NorStar.
  values={'employer_id':'norstar-systems-llc','mapping_ref':'urn:local-only:entity-mapping','approval_ref':'urn:local-only:sql-approval',
   'authorization_id':str(uuid.uuid4()),'decision_id':str(uuid.uuid4()),'actor_id':'local-only-operator',
   'evidence_ref':'urn:local-only:unsigned-synthetic-test','authority_basis':'LOCAL FIXTURE ONLY; never a real delegation',
   'decided_at':datetime.datetime.now(datetime.timezone.utc).isoformat(),'outcome':'APPROVED','conflict_disclosure_ref':'urn:local-only:conflict'}
  settings=''.join("SET cvera.norstar_"+key+"='"+value.replace("'","''")+"';\n" for key,value in values.items())
  assert sql(register,False).returncode!=0
  assert sql('SELECT count(*) FROM evt_employer_config;').stdout.strip()=='0'
  created=json.loads(sql(settings+register).stdout)
  assert created['registered_employer_id']=='norstar-systems-llc' and created['authority_or_credentials_created'] is False
  assert sql(settings+register,False).returncode!=0
  assert sql(record,False).returncode!=0
  assert sql('SELECT committed_position FROM authority_recording_boundary;').stdout.strip()=='0'
  gate=json.loads(sql(settings+record).stdout)
  assert gate['stage']=='APPROVED_NOT_ACTIVE' and gate['activation_event_present'] is False and gate['recorded_position']==4
  assert sql("SELECT string_agg(event_type,',' ORDER BY journal_sequence) FROM authority_journal_events;").stdout.strip()=='SUBMIT,APPROVE'
  assert sql(settings+record,False).returncode!=0
  assert sql(settings+"SET cvera.norstar_outcome='REJECTED';"+record,False).returncode!=0
  assert sql('SELECT committed_position FROM authority_recording_boundary;').stdout.strip()=='4'
  sql('CREATE ROLE local_runtime NOLOGIN;GRANT USAGE ON SCHEMA public TO local_runtime;GRANT SELECT ON evt_employer_config TO local_runtime;')
  assert sql(settings+'SET ROLE local_runtime;'+record,False).returncode!=0
  assert sql("SELECT to_regclass('public.evidence_issuer_credentials') IS NULL;").stdout.strip()=='t'
  print(json.dumps({'norstar_preparation_local':'PASS','missing_inputs_denied':True,'duplicate_and_blind_resume_denied':True,
   'four_recording_transactions':True,'stops_approved_not_active':True,'runtime_denied':True,'production_accessed':False,'signing_or_credentials_used':False}))
 finally:
  r=run(['docker','inspect',NAME],check=False)
  if r.returncode==0:
   item=json.loads(r.stdout)[0]
   assert item['HostConfig']['NetworkMode']=='none' and item['Config']['Labels'].get('cvera.scope')==LABEL
   run(['docker','rm','-f','-v',NAME])
if __name__=='__main__':main()
