#!/usr/bin/env python3
"""Execute only the owner-approved MANUAL registry insertion; never authority/signing."""
import datetime,fcntl,hashlib,json,os,stat,sys,urllib.request
from pathlib import Path

PREPARATION='d1c48d0b3a08f31b6ae0269255772775446700bb'
HELPER_HASH='9bcff4c1d8f81d3d4e1eb528312ff3cf36941d9d05a7593dee2c51174f16e0c0'
BACKUP=Path('/opt/cvera/norstar-preparation-20261108T220104Z-9065a0')
ROOT=Path('/opt/cvera/rsa-attestation-engine')
EMPLOYER='norstar-systems-llc'
APPROVAL='owner-direction:complete-approved-norstar-manual-registration:2026-11-08'
PHASE='preflight'

def secure(path,mode):
 s=path.lstat()
 if stat.S_ISLNK(s.st_mode) or s.st_uid!=0 or stat.S_IMODE(s.st_mode)!=mode:
  raise RuntimeError('private recovery permissions')

def existing_result(current,receipt,intent):
 """Never adopt an unexplained existing row or retry an ambiguous COMMIT."""
 rows=[r for r in current['registry'] if r['employer_id']==EMPLOYER]
 if receipt.exists():
  secure(receipt,0o600)
  recorded=json.loads(receipt.read_text())
  if recorded.get('norstar_registration')!='PASS' or recorded.get('approval_ref')!=APPROVAL or recorded.get('registered_employer_id')!=EMPLOYER or len(rows)!=1 or rows[0]['dispatch_mode']!='MANUAL':
   raise RuntimeError('recorded registration mismatch')
  return dict(recorded,already_executed=True,insertion_repeated=False)
 if rows or intent.exists():raise RuntimeError('existing row or unresolved intent; inspect without retry')
 return None

def main():
 global PHASE
 if os.geteuid()!=0 or sys.argv[1:]!=['--execute-approved-registration']:
  raise RuntimeError('approved root console execution required')
 os.umask(0o077);secure(BACKUP,0o700)
 # Serialize console invocations, including a repeated command after lost output.
 lock=os.open(BACKUP/'registration.lock',os.O_CREAT|os.O_RDWR|os.O_NOFOLLOW,0o600)
 fcntl.flock(lock,fcntl.LOCK_EX|fcntl.LOCK_NB)
 import subprocess
 helper=subprocess.run(['git','-C',str(ROOT),'show',PREPARATION+':apps/evt-api-go/operations/prepare-norstar-production.py'],capture_output=True,check=True).stdout
 if hashlib.sha256(helper).hexdigest()!=HELPER_HASH:raise RuntimeError('reviewed helper mismatch')
 # Load exactly the reviewed helper without running its preparation entry point.
 h={'__name__':'norstar_reviewed_helpers'};exec(compile(helper,'reviewed-norstar-preparation','exec'),h)
 read=h['source_read'];run=h['run'];private=h['private'];info=h['info']
 receipt=BACKUP/'registration-result.json';intent=BACKUP/'registration-intent.json'
 current=json.loads(read(h['CATALOG']))
 previous=existing_result(current,receipt,intent)
 if previous:print(json.dumps(previous));return
 for name in ('preparation-result.json','restore-rehearsal.json','registry-and-privilege-inventory.private.json','evt.dump','backup.sha256'):
  secure(BACKUP/name,0o600)
 prepared=json.loads((BACKUP/'preparation-result.json').read_text())
 restored=json.loads((BACKUP/'restore-rehearsal.json').read_text())
 if prepared.get('norstar_preparation')!='PASS' or prepared.get('registration_executed') is not False or prepared.get('sql_source')!=h['SOURCE']:
  raise RuntimeError('missing verified preparation')
 if any(restored.get(k)!='PASS' for k in ('restore','rollback','commit')) or restored.get('mode')!='MANUAL' or not restored.get('unrelated_data_and_privileges_preserved'):
  raise RuntimeError('missing verified recovery rehearsal')
 if h['digest'](BACKUP/'evt.dump')!=(BACKUP/'backup.sha256').read_text().split()[0]:raise RuntimeError('backup checksum mismatch')
 if current!=json.loads((BACKUP/'registry-and-privilege-inventory.private.json').read_text()):raise RuntimeError('registry or privileges changed since preparation')
 if run(['git','-C',str(ROOT),'rev-parse','HEAD']).decode().strip()!=h['BASE'] or run(['git','-C',str(ROOT),'status','--porcelain']).strip():raise RuntimeError('running checkout changed')
 with urllib.request.urlopen('http://127.0.0.1:8080/version',timeout=5) as response:
  if json.load(response).get('git_sha')!=h['BASE']:raise RuntimeError('running API changed')
 services=[h['DB'],'evt-api-go-api-1','evt-api-go-kratos-1','evt-api-go-kratos-courier-1']
 baseline={n:(info(n,'Id'),info(n,'State.StartedAt')) for n in services}
 if not all(info(n,'State.Running') for n in services):raise RuntimeError('service not running')
 sources={}
 for name,expected in h['FILES'].items():
  data=run(['git','-C',str(ROOT),'show',h['SOURCE']+':apps/evt-api-go/operations/'+name])
  if hashlib.sha256(data).hexdigest()!=expected:raise RuntimeError('reviewed SQL mismatch')
  sources[name]=data.decode()
 h['validate'](h['metadata'](sources['norstar-preparation-preflight.sql']))
 if read("SELECT EXISTS(SELECT 1 FROM public.evt_requests WHERE claim_snapshot::text ILIKE '%B20250427754%')").strip()!=b'f':raise RuntimeError('legal-number candidate requires reconciliation')
 mapping=str(BACKUP/'registration-intent.json')+'#owner-approved-legal-mapping'
 private(intent,json.dumps({'approval_ref':APPROVAL,'legal_entity':'NorStar Systems LLC','jurisdiction':'California','entity_number':'B20250427754','employer_id':EMPLOYER,'dispatch_mode':'MANUAL','mapping_ref':mapping,'actor':'authorized owner via existing root console','time':datetime.datetime.now(datetime.timezone.utc).isoformat(),'scope':'one registry row only; not delegation or authority evidence'}).encode())
 PHASE='approved_registry_transaction'
 settings="SET cvera.norstar_employer_id='"+EMPLOYER+"';SET cvera.norstar_mapping_ref='"+mapping+"';SET cvera.norstar_approval_ref='"+APPROVAL+"';"
 result=json.loads(run(['docker','exec','-i',h['DB'],'psql','-X','-U','postgres','-d','evt','-Atq','-v','ON_ERROR_STOP=1'],(settings+sources['register-norstar-employer.sql']).encode()))
 PHASE='post_commit_verification'
 after=json.loads(read(h['CATALOG']))
 added=[r for r in after['registry'] if r['employer_id']==EMPLOYER]
 if len(added)!=1 or added[0]['dispatch_mode']!='MANUAL' or [r for r in after['registry'] if r['employer_id']!=EMPLOYER]!=current['registry']:raise RuntimeError('unexpected registry result')
 if any(after[k]!=current[k] for k in ('roles','memberships','acl','schema_acl')):raise RuntimeError('unexpected privilege change')
 if any((info(n,'Id'),info(n,'State.StartedAt'))!=baseline[n] or not info(n,'State.Running') for n in services):raise RuntimeError('unexpected service change')
 result.update(norstar_registration='PASS',already_executed=False,insertion_repeated=False,authority_recorded=False,credentials_or_signing_used=False,services_restarted=False)
 private(receipt,json.dumps(result).encode());print(json.dumps(result))

if __name__=='__main__':
 try:main()
 except Exception:
  print(json.dumps({'norstar_registration':'STOP','phase':PHASE,'instruction':'Do not retry insertion or disclose private output; preserve intent/receipt for read-only reconciliation.'}));sys.exit(1)
