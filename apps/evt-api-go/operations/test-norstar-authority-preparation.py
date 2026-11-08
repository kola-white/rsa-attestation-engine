#!/usr/bin/env python3
"""Local-only, network-none SQL regression; never opens a production connection."""
import datetime,hashlib,importlib.util,json,subprocess,tempfile,time,uuid
from pathlib import Path
from unittest.mock import patch
ROOT=Path(__file__).resolve().parents[1]
NAME='cvera-norstar-local-'+uuid.uuid4().hex[:12]
LABEL='norstar-preparation-local-test'
def run(args,data=None,check=True):
 r=subprocess.run(args,input=data,text=True,capture_output=True,timeout=60)
 if check and r.returncode:raise RuntimeError('isolated local SQL regression failed; raw output suppressed')
 return r

def sql(body,check=True,name=NAME):
 return run(['docker','exec','-i',name,'psql','-X','-U','postgres','-d','evt','-Atq','-v','ON_ERROR_STOP=1'],body,check)

def main():
 try:
  registration_spec=importlib.util.spec_from_file_location('approved_registration',ROOT/'operations/register-approved-norstar.py')
  registration=importlib.util.module_from_spec(registration_spec);registration_spec.loader.exec_module(registration)
  with tempfile.TemporaryDirectory(prefix='norstar-replay-local-') as folder, patch.object(registration,'secure'):
   receipt=Path(folder)/'receipt';intent=Path(folder)/'intent'
   empty={'registry':[]};present={'registry':[{'employer_id':registration.EMPLOYER,'dispatch_mode':'MANUAL'}]}
   assert registration.existing_result(empty,receipt,intent) is None
   try:registration.existing_result(present,receipt,intent)
   except RuntimeError:pass
   else:raise AssertionError('unexplained existing registration adopted')
   intent.write_text('{}')
   try:registration.existing_result(empty,receipt,intent)
   except RuntimeError:pass
   else:raise AssertionError('ambiguous commit retried')
   receipt.write_text(json.dumps({'norstar_registration':'PASS','approval_ref':registration.APPROVAL,'registered_employer_id':registration.EMPLOYER}))
   assert registration.existing_result(present,receipt,intent)['already_executed'] is True
   for rows in (empty,{'registry':[{'employer_id':registration.EMPLOYER,'dispatch_mode':'AUTO'}]}):
    try:registration.existing_result(rows,receipt,intent)
    except RuntimeError:pass
    else:raise AssertionError('changed registration accepted as previous success')
  run(['docker','run','-d','--pull','never','--name',NAME,'--network','none','--memory','64m','--memory-swap','64m','--cpus','0.5','--shm-size','16m','--label','cvera.scope='+LABEL,
   '-e','POSTGRES_HOST_AUTH_METHOD=trust','-e','POSTGRES_INITDB_ARGS=-c shared_buffers=8MB -c max_connections=10 -c maintenance_work_mem=4MB -c autovacuum=off','postgres:16',
   '-c','shared_buffers=8MB','-c','max_connections=10','-c','work_mem=512kB','-c','maintenance_work_mem=4MB','-c','autovacuum=off','-c','max_worker_processes=0','-c','max_parallel_workers=0'])
  for _ in range(60):
   if run(['docker','exec',NAME,'psql','-X','-h','127.0.0.1','-U','postgres','-Atqc','SELECT 1'],check=False).returncode==0:break
   time.sleep(1)
  else:raise RuntimeError('isolated local PostgreSQL not ready')
  run(['docker','exec',NAME,'psql','-X','-U','postgres','-v','ON_ERROR_STOP=1','-c','CREATE DATABASE evt'])
  sql("CREATE TABLE public.evt_employer_config(employer_id text PRIMARY KEY,dispatch_mode text NOT NULL DEFAULT 'AUTO',created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now());CREATE TABLE public.evt_requests(employer_id text,claim_snapshot jsonb);")
  for path in sorted((ROOT/'migrations').glob('*.sql')):
   if '000002'<=path.name<'000014':sql(path.read_text())
  preflight=(ROOT/'operations/norstar-preparation-preflight.sql').read_text()
  metadata=json.loads(sql(preflight).stdout)
  assert metadata['database']=='evt' and metadata['exact_id_collision'] is False and metadata['records_or_configuration_changed'] is False
  assert metadata['user_insert_trigger_count']==0 and metadata['rewrite_rule_count']==0
  register=(ROOT/'operations/register-norstar-employer.sql').read_text()
  record=(ROOT/'operations/record-norstar-authority-approved.sql').read_text()
  spec=importlib.util.spec_from_file_location('norstar_preparation_driver',ROOT/'operations/prepare-norstar-production.py')
  driver=importlib.util.module_from_spec(spec);spec.loader.exec_module(driver);driver.DB=NAME
  for filename,expected in driver.FILES.items():assert hashlib.sha256((ROOT/'operations'/filename).read_bytes()).hexdigest()==expected
  driver.validate(metadata)
  try:driver.source_read("INSERT INTO public.evt_employer_config(employer_id) VALUES('must-not-be-inserted')")
  except RuntimeError:pass
  else:raise AssertionError('source SQL was not read-only')
  assert sql('SELECT count(*) FROM evt_employer_config;').stdout.strip()=='0'
  with tempfile.TemporaryDirectory(prefix='norstar-driver-local-') as folder:
   backup=Path(folder)
   run(['docker','exec',NAME,'pg_dump','-U','postgres','-d','evt','-Fc','-f','/tmp/norstar-driver-local.dump'])
   run(['docker','cp',NAME+':/tmp/norstar-driver-local.dump',str(backup/'evt.dump')])
   image=json.loads(run(['docker','inspect',NAME]).stdout)[0]['Image']
   stubs=sql("SELECT format('CREATE ROLE %I NOLOGIN;',rolname) FROM pg_catalog.pg_roles WHERE rolname<>'postgres' AND rolname !~ '^pg_' ORDER BY rolname;").stdout.encode()
   rehearsal=driver.rehearse(backup,image,stubs,register,NAME+'-driver-rehearsal')
   assert rehearsal['restore']=='PASS' and rehearsal['rollback']=='PASS' and rehearsal['commit']=='PASS' and rehearsal['unrelated_data_and_privileges_preserved']
  assert sql('SELECT count(*) FROM evt_employer_config;').stdout.strip()=='0'

  # Explicit LOCAL fixtures. These are not evidence or decisions about real NorStar.
  values={'employer_id':'norstar-systems-llc','mapping_ref':'urn:local-only:entity-mapping','approval_ref':'urn:local-only:sql-approval',
   'authorization_id':str(uuid.uuid4()),'decision_id':str(uuid.uuid4()),'actor_id':'local-only-operator',
   'evidence_ref':'urn:local-only:unsigned-synthetic-test','authority_basis':'LOCAL FIXTURE ONLY; never a real delegation',
   'decided_at':datetime.datetime.now(datetime.timezone.utc).isoformat(),'outcome':'APPROVED','conflict_disclosure_ref':'urn:local-only:conflict'}
  settings=''.join("SET cvera.norstar_"+key+"='"+value.replace("'","''")+"';\n" for key,value in values.items())
  assert sql(register,False).returncode!=0
  assert sql('SELECT count(*) FROM evt_employer_config;').stdout.strip()=='0'
  created=json.loads(sql(settings+register).stdout)
  assert created['registered_employer_id']=='norstar-systems-llc' and created['authority_or_credentials_created'] is False and created['dispatch_mode']=='MANUAL'
  assert sql("SELECT dispatch_mode FROM evt_employer_config WHERE employer_id='norstar-systems-llc';").stdout.strip()=='MANUAL'
  assert sql(settings+register,False).returncode!=0
  sql("CREATE FUNCTION local_registry_trigger() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RETURN NEW; END $$;CREATE TRIGGER unexpected_registry_insert BEFORE INSERT ON evt_employer_config FOR EACH ROW EXECUTE FUNCTION local_registry_trigger();")
  blocked=sql(settings+"SET cvera.norstar_employer_id='local-only-trigger-probe';"+register,False)
  assert blocked.returncode!=0 and 'unreviewed registry INSERT trigger' in blocked.stderr
  assert sql('SELECT count(*) FROM evt_employer_config;').stdout.strip()=='1'
  sql('DROP TRIGGER unexpected_registry_insert ON evt_employer_config;')
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
  activation=(ROOT/'operations/activate-norstar-authority.sql').read_text()
  containment=(ROOT/'operations/contain-norstar-authority.sql').read_text()
  extra="SET cvera.norstar_activation_approval_ref='LOCAL ONLY';SET cvera.norstar_activation_event_id='local-activation';SET cvera.norstar_activation_conditions_satisfied='true';SET cvera.norstar_delegation_valid_from='2020-01-01T00:00:00Z';"
  assert sql(settings+activation,False).returncode!=0
  assert sql(settings+extra+"SET cvera.norstar_activation_conditions_satisfied='false';"+activation,False).returncode!=0
  assert sql(settings+extra+"SET ROLE local_runtime;"+activation,False).returncode!=0
  activated=json.loads(sql(settings+extra+activation).stdout)
  assert activated['authority_activation']=='RECORDED'
  assert sql(settings+extra+activation,False).returncode!=0
  assert sql('SELECT committed_position FROM authority_recording_boundary;').stdout.strip()=='5'
  withdrawal="SET cvera.norstar_containment_approval_ref='LOCAL ONLY';SET cvera.norstar_containment_event_id='local-suspend';SET cvera.norstar_containment_event='SUSPEND';"
  assert sql(settings+containment,False).returncode!=0
  assert json.loads(sql(settings+withdrawal+containment).stdout)['authority_containment']=='RECORDED'
  assert sql(settings+withdrawal+containment,False).returncode!=0
  revoked=withdrawal+"SET cvera.norstar_containment_event_id='local-revoke';SET cvera.norstar_containment_event='REVOKE';"
  assert json.loads(sql(settings+revoked+containment).stdout)['event']=='REVOKE'
  assert sql(settings+revoked+containment,False).returncode!=0
  assert sql('SELECT committed_position FROM authority_recording_boundary;').stdout.strip()=='7'

  # Rehearse the documented backup/restore on a second isolated clone. This
  # is a real local archive restore, not a production-backup readiness claim.
  clone=NAME+'-restore'
  image=json.loads(run(['docker','inspect',NAME]).stdout)[0]['Image']
  run(['docker','exec',NAME,'pg_dump','-U','postgres','-d','evt','-Fc','-f','/tmp/norstar-local.dump'])
  with tempfile.TemporaryDirectory(prefix='norstar-backup-local-') as folder:
   archive=Path(folder)/'evt.dump';run(['docker','cp',NAME+':/tmp/norstar-local.dump',str(archive)])
   dump=archive.read_bytes();assert len(dump)>0;digest=hashlib.sha256(dump).hexdigest()
   run(['docker','run','-d','--pull','never','--name',clone,'--network','none','--memory','64m','--memory-swap','64m','--cpus','0.5','--shm-size','16m','--label','cvera.scope='+LABEL,
    '-e','POSTGRES_HOST_AUTH_METHOD=trust','-e','POSTGRES_INITDB_ARGS=-c shared_buffers=8MB -c max_connections=10 -c maintenance_work_mem=4MB -c autovacuum=off',image,
    '-c','shared_buffers=8MB','-c','max_connections=10','-c','work_mem=512kB','-c','maintenance_work_mem=4MB','-c','autovacuum=off','-c','max_worker_processes=0','-c','max_parallel_workers=0'])
   for _ in range(60):
    if run(['docker','exec',clone,'psql','-X','-h','127.0.0.1','-U','postgres','-Atqc','SELECT 1'],check=False).returncode==0:break
    time.sleep(1)
   else:raise RuntimeError('local restore not ready')
   source=json.loads(run(['docker','inspect',NAME]).stdout)[0];target=json.loads(run(['docker','inspect',clone]).stdout)[0]
   assert target['HostConfig']['NetworkMode']=='none'
   assert not {x['Source'] for x in source['Mounts']}.intersection({x['Source'] for x in target['Mounts']})
   stubs=sql("SELECT format('CREATE ROLE %I NOLOGIN;',rolname) FROM pg_catalog.pg_roles WHERE rolname<>'postgres' AND rolname !~ '^pg_' ORDER BY rolname;").stdout
   run(['docker','exec','-i',clone,'psql','-X','-U','postgres','-d','postgres','-q','-v','ON_ERROR_STOP=1'],stubs)
   run(['docker','exec',clone,'createdb','-U','postgres','evt'])
   restored=subprocess.run(['docker','exec','-i',clone,'pg_restore','-U','postgres','-d','evt','--exit-on-error'],input=dump,capture_output=True,timeout=60)
   assert restored.returncode==0 and hashlib.sha256(archive.read_bytes()).hexdigest()==digest
   assert sql("SELECT dispatch_mode FROM evt_employer_config WHERE employer_id='norstar-systems-llc';",name=clone).stdout.strip()=='MANUAL'
   assert sql('SELECT committed_position FROM authority_recording_boundary;',name=clone).stdout.strip()=='7'
   assert sql("SELECT count(*) FROM authority_journal_events WHERE event_type='ACTIVATE';",name=clone).stdout.strip()=='1'
  print(json.dumps({'norstar_preparation_local':'PASS','missing_inputs_denied':True,'duplicate_and_blind_resume_denied':True,
   'production_preparation_driver_rehearsal':True,'source_read_only_enforced':True,'real_local_archive_restore':True,'metadata_preflight_read_only':True,'manual_non_dispatching_registration':True,'unreviewed_insert_trigger_denied':True,'four_recording_transactions':True,'stops_approved_not_active':True,'separate_activation_and_containment':True,'runtime_denied':True,'production_accessed':False,'signing_or_credentials_used':False}))
 finally:
  for name in [NAME+'-driver-rehearsal',NAME+'-restore',NAME]:
   r=run(['docker','inspect',name],check=False)
   if r.returncode==0:
    item=json.loads(r.stdout)[0]
    assert item['HostConfig']['NetworkMode']=='none' and item['Config']['Labels'].get('cvera.scope') in (LABEL,'norstar-production-backup-rehearsal')
    run(['docker','rm','-f','-v',name])
if __name__=='__main__':main()
