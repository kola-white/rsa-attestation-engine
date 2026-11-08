#!/usr/bin/env python3
"""PREPARED ONLY: explicit owner-approved public credential registration.

No private key access, key rotation, authority changes, role changes or signing.
"""
import argparse,hashlib,importlib.util,json,os,stat,subprocess,urllib.request
from pathlib import Path
ROOT=Path('/opt/cvera/rsa-attestation-engine');SOURCE='973fb3bb2ab9c80dc965b27458f1df85413f4dee'
SQL_HASH='975f5621056d542f397b9501fb8b0f6e010d4b2f6a6a664db5fc52e7901312e5';PHASE='preflight'
spec=importlib.util.spec_from_file_location('public_credential_support',Path(__file__).with_name('issuance-acceptance-support.py'));p=importlib.util.module_from_spec(spec);spec.loader.exec_module(p)
def run(args,data=None):
 r=subprocess.run(args,input=data,capture_output=True,timeout=30)
 if r.returncode:raise RuntimeError('private output suppressed')
 return r.stdout

def sql(body):return run(['docker','exec','-i','evt-api-go-db-1','psql','-X','-U','postgres','-d','evt','-Atq','-v','ON_ERROR_STOP=1','-v','VERBOSITY=sqlstate'],body.encode())
def private(path,data):
 fd=os.open(path,os.O_CREAT|os.O_EXCL|os.O_WRONLY|os.O_NOFOLLOW,0o600)
 with os.fdopen(fd,'w') as f:json.dump(data,f);f.flush();os.fsync(f.fileno())

def main():
 global PHASE
 parser=argparse.ArgumentParser();parser.add_argument('--approved-public-registration',type=Path,required=True);a=parser.parse_args();os.umask(0o077)
 st=a.approved_public_registration.lstat()
 if os.geteuid()!=0 or st.st_uid!=0 or stat.S_IMODE(st.st_mode)!=0o600 or stat.S_ISLNK(st.st_mode):raise RuntimeError('approved root-private input required')
 c=json.loads(a.approved_public_registration.read_text())
 fields=['valid_from','valid_until','claim_lifetime_seconds','established_by','basis_ref']
 if not c.get('approval_ref') or any(k not in c or c[k] is None or c[k]=='' for k in fields) or type(c['claim_lifetime_seconds']) is not int:raise RuntimeError('actual explicit approved metadata required')
 public=ROOT/'keys/public/issuer-prod-2026-10.pub.pem'
 with urllib.request.urlopen('https://hapis.sfo3.cdn.digitaloceanspaces.com/trust/jwks.json',timeout=10) as r:jwks=json.loads(r.read(1024*1024))
 p.public_binding(public,jwks)
 script=run(['git','-C',str(ROOT),'show',SOURCE+':apps/evt-api-go/operations/register-evidence-issuer-credential.sql'])
 if hashlib.sha256(script).hexdigest()!=SQL_HASH:raise RuntimeError('reviewed public registration SQL mismatch')
 absent=sql("BEGIN READ ONLY;SET LOCAL search_path=pg_catalog;SELECT NOT EXISTS(SELECT 1 FROM public.evidence_issuer_credentials WHERE issuer_id='https://cvera.app' AND kid='issuer-prod-2026-10');ROLLBACK;").strip()
 if absent!=b't':raise RuntimeError('existing credential: do not insert or reactivate')
 folder=a.approved_public_registration.with_suffix('.credential-registration');folder.mkdir(mode=0o700)
 private(folder/'intent.json',{'approval_ref':c['approval_ref'],'source':SOURCE,'sql_sha256':SQL_HASH,'metadata_sha256':hashlib.sha256(a.approved_public_registration.read_bytes()).hexdigest(),'scope':'one canonical public credential binding; no signing/private key/authority'})
 values={k:str(c[k]) for k in fields};values.update(kid=p.KID,public_pem=public.read_text(),spki_sha256=p.SPKI)
 settings=''.join("SELECT set_config('cvera.credential_"+k+"','"+v.replace("'","''")+"',true);\n" for k,v in values.items())
 # Suppress set_config output (which includes public PEM/metadata), retain only
 # the final transaction ID in the private receipt after successful COMMIT.
 PHASE='approved_public_registration_transaction'
 raw=sql("BEGIN ISOLATION LEVEL READ COMMITTED;SET LOCAL search_path=public,pg_temp;SET LOCAL standard_conforming_strings=on;SET LOCAL lock_timeout='5s';SET LOCAL statement_timeout='30s';"+settings+script.decode()+"SELECT jsonb_build_object('transaction',txid_current());COMMIT;")
 # Each psql set_config can span lines; only the final JSON is our tx receipt.
 tx=json.loads(raw.decode().strip().splitlines()[-1])['transaction']
 PHASE='post_commit_verification'
 observed=json.loads(sql("BEGIN READ ONLY;SELECT jsonb_build_object('issuer',issuer_id,'kid',kid,'fingerprint',spki_sha256,'unrevoked',revoked_at IS NULL,'current',clock_timestamp()>=valid_from AND clock_timestamp()<valid_until) FROM public.evidence_issuer_credentials WHERE issuer_id='https://cvera.app' AND kid='issuer-prod-2026-10';ROLLBACK;"))
 if observed!={'issuer':p.ISSUER,'kid':p.KID,'fingerprint':p.SPKI,'unrevoked':True,'current':True}:raise RuntimeError('public credential verification mismatch')
 result={'public_credential_registration':'PASS','transaction':tx,'issuer':p.ISSUER,'kid':p.KID,'jwks_and_public_spki':'PASS','private_key_accessed':False,'authority_or_roles_changed':False,'signing_performed':False,'private_record':str(folder/'result.json')}
 private(folder/'result.json',result);print(json.dumps(result))
if __name__=='__main__':
 try:main()
 except Exception:
  print(json.dumps({'public_credential_registration':'STOP','phase':PHASE,'instruction':'Do not retry/rotate/reactivate or print private output; preserve intent for reconciliation.'}));raise SystemExit(1)
