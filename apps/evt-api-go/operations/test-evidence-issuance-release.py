#!/usr/bin/env python3
"""Local-only release transaction and actual archive-restore regressions."""
import base64,datetime,hashlib,importlib.util,io,json,os,subprocess,tempfile,time,uuid
from unittest.mock import patch
from pathlib import Path
APP=Path(__file__).resolve().parents[1]
spec=importlib.util.spec_from_file_location('release',APP/'operations/release-evidence-issuance-sql.py');m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
name='cvera-issuance-local-'+uuid.uuid4().hex[:12];m.DB=name
label='issuance-release-local-test'
def local(path):return (APP/path).read_text()
try:
 m.run(['docker','run','-d','--pull','never','--name',name,'--network','none','--memory','64m','--memory-swap','64m','--shm-size','16m','--label','cvera.scope='+label,'-e','POSTGRES_HOST_AUTH_METHOD=trust','-e','POSTGRES_INITDB_ARGS=-c shared_buffers=8MB -c max_connections=10 -c maintenance_work_mem=4MB -c autovacuum=off','postgres:16','-c','shared_buffers=8MB','-c','max_connections=10','-c','work_mem=512kB','-c','maintenance_work_mem=4MB','-c','autovacuum=off','-c','max_worker_processes=0','-c','max_parallel_workers=0'])
 for _ in range(60):
  ready=subprocess.run(['docker','exec',name,'psql','-X','-h','127.0.0.1','-U','postgres','-Atqc','SELECT 1'],capture_output=True,timeout=5)
  if ready.returncode==0:break
  time.sleep(1)
 else:raise RuntimeError('local PostgreSQL not ready')
 m.run(['docker','exec','-i',name,'psql','-X','-U','postgres','-d','postgres','-v','ON_ERROR_STOP=1'],b'CREATE DATABASE evt;CREATE ROLE cvera_api_runtime LOGIN;CREATE ROLE cvera_evidence_operator NOLOGIN;CREATE ROLE cvera_hr_operator NOLOGIN;CREATE ROLE "evt-app" NOLOGIN;')
 # Confirmed parent keys plus explicit LOCAL scaffolding for unexported workflow
 # DDL. Only the future production archive rehearsal establishes real compatibility.
 m.sql(local('internal/httpapi/testdata/proposal_a_legacy_schema.sql')+'''ALTER TABLE evt_requests ALTER COLUMN request_id SET DEFAULT gen_random_uuid();ALTER TABLE evt_requests ADD COLUMN submitted_at timestamptz;ALTER TABLE evt_requests ADD COLUMN attestation_dispatched_at timestamptz;
 CREATE TABLE evt_request_events(request_id uuid,actor_role text,actor_personid uuid,event_type text,from_status text,to_status text,payload jsonb);
 ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT,INSERT,UPDATE,DELETE ON TABLES TO "evt-app",cvera_api_runtime;
 ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT USAGE,UPDATE ON SEQUENCES TO "evt-app",cvera_api_runtime;
 GRANT USAGE ON SCHEMA public TO cvera_api_runtime,cvera_evidence_operator,cvera_hr_operator;
 GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA public TO cvera_api_runtime;''')
 for number,binding in [(14,'hr-assignment'),(15,'evidence-authorization'),(16,'evidence-completion'),(17,'evidence-review'),(18,'request-evidence-binding')]:
  migration=next((APP/'migrations').glob(f'{number:06d}_*.sql'))
  settings="SET LOCAL cvera.evidence_runtime_role='cvera_api_runtime';SET LOCAL cvera.evidence_operator_role='cvera_evidence_operator';SET LOCAL cvera.hr_runtime_role='cvera_api_runtime';SET LOCAL cvera.hr_operator_role='cvera_hr_operator';"
  m.sql('BEGIN;'+m.SETTINGS+settings+migration.read_text()+local('operations/'+binding+'-privileges.sql')+'COMMIT;')
 m.read(m.PREFLIGHT)
 for n,expected in m.FILES.items():assert hashlib.sha256((APP/n).read_bytes()).hexdigest()==expected
 artifacts={n:local(n) for n in m.FILES};artifacts['operations/evidence-issuance-restore-probe.sql']=local('operations/evidence-issuance-restore-probe.sql')
 fingerprint=m.read(m.FINGERPRINT)
 m.sql(m.transaction(artifacts,'ROLLBACK'));m.read(m.PREFLIGHT)
 assert m.read(m.FINGERPRINT)==fingerprint
 broken=dict(artifacts);broken['migrations/000019_evidence_backed_issuance.sql']+='\nSELECT 1/0;\n'
 try:m.sql(m.transaction(broken,'COMMIT'))
 except RuntimeError:pass
 else:raise AssertionError('injected failure did not abort')
 m.read(m.PREFLIGHT);assert m.read(m.FINGERPRINT)==fingerprint
 with tempfile.TemporaryDirectory(prefix='cvera-issuance-release-local-') as folder:
  backup=Path(folder)
  archive=m.run(['docker','exec',name,'pg_dump','-U','postgres','-d','evt','-Fc']);(backup/'evt.dump').write_bytes(archive)
  m.rehearse(backup,artifacts)
  result=json.loads((backup/'restore-rehearsal.json').read_text());assert result['empty_authority_and_credential_fail_closed']=='PASS'
  clone=result['clone']
  assert not m.info(clone,'State.Running') and m.info(clone,'HostConfig.NetworkMode')=='none'
  m.run(['docker','rm','-v',clone])
 m.sql(m.transaction(artifacts,'COMMIT'));m.sql('BEGIN;'+m.SETTINGS+m.VERIFY+'ROLLBACK;')
 assert m.read(m.FINGERPRINT)==fingerprint
 try:m.read(m.PREFLIGHT)
 except RuntimeError:pass
 else:raise AssertionError('repeat bootstrap permitted')
 m.sql('BEGIN;'+m.SETTINGS+artifacts['operations/evidence-issuance-restore-probe.sql'].replace('BEGIN ISOLATION LEVEL READ COMMITTED;','').replace('ROLLBACK;','')+'ROLLBACK;')
 # Exercise the public registration executor against this LOCAL database.
 # Only the committed PUBLIC key is used; no production key or signing access.
 spec=importlib.util.spec_from_file_location('public_registration',APP/'operations/register-approved-issuer-credential.py')
 credential=importlib.util.module_from_spec(spec);spec.loader.exec_module(credential)
 credential.ROOT=APP.parents[1];credential.sql=m.sql
 public=credential.ROOT/'keys/public/issuer-prod-2026-10.pub.pem'
 modulus=m.run(['openssl','rsa','-pubin','-in',str(public),'-modulus','-noout']).decode().strip().split('=')[1]
 encoded=base64.urlsafe_b64encode(bytes.fromhex(modulus)).rstrip(b'=').decode()
 jwks={'keys':[{'kty':'RSA','kid':credential.p.KID,'use':'sig','alg':'RS256','n':encoded,'e':'AQAB'}]}
 class LocalOwnerPath(type(Path())):
  def lstat(self):
   values=list(super().lstat());values[4]=0;return os.stat_result(values)
 credential.Path=LocalOwnerPath
 with tempfile.TemporaryDirectory(prefix='local-public-registration-') as directory:
  private=Path(directory)/'LOCAL-ONLY.json';now=datetime.datetime.now(datetime.timezone.utc)
  private.write_text(json.dumps({'approval_ref':'LOCAL FIXTURE ONLY','valid_from':(now-datetime.timedelta(minutes=1)).isoformat(),'valid_until':(now+datetime.timedelta(hours=1)).isoformat(),'claim_lifetime_seconds':600,'established_by':'local-fixture','basis_ref':'LOCAL FIXTURE: not production activation'}));private.chmod(0o600)
  with patch.object(credential.os,'geteuid',return_value=0),patch.object(credential.urllib.request,'urlopen',side_effect=lambda *a,**k:io.BytesIO(json.dumps(jwks).encode())),patch('sys.argv',['register','--approved-public-registration',str(private)]):
   credential.main()
   try:credential.main()
   except RuntimeError:pass
   else:raise AssertionError('existing public credential reactivated')
  assert json.loads((private.with_suffix('.credential-registration')/'result.json').read_text())['private_key_accessed'] is False
 print(json.dumps({'issuance_release_local':'PASS','atomic_bootstrap_and_000019':True,'rollback_and_commit':True,'fresh_archive_restored':True,'real_request_workflow_and_receipt_probe':True,'hostile_defaults_stripped':True,'runtime_five_executes_only':True,'unavailable_authority_and_credential_fail_closed':True,'repeat_or_existing_schema_denied':True,'parent_privileges_unchanged':True,'production_accessed':False,'signing_performed':False,'public_only_credential_registration_local':True}))
finally:
 r=subprocess.run(['docker','inspect',name],capture_output=True)
 if r.returncode==0 and m.info(name,'Config.Labels').get('cvera.scope')==label and m.info(name,'HostConfig.NetworkMode')=='none':m.run(['docker','rm','-f','-v',name])
