#!/usr/bin/env python3
"""Approved Proposal B SQL release phase, run only in owner's Droplet console.

Backs up evt and password-free role metadata locally, retains current API image,
then commits only migration 000015 + bounded bindings + one NOLOGIN operator.
No grants/cases, credentials, app records, services, Git checkout or merge changes.
"""
import datetime
import hashlib
import json
import os
from pathlib import Path
import shutil
import stat
import subprocess
import sys
import urllib.parse
import urllib.request

ROOT = Path('/opt/cvera/rsa-attestation-engine')
DB = 'evt-api-go-db-1'
BASE = 'f52f5bf282735ae24ac91d3b3855e3e5b0d5de13'
FILES = {
 'migrations/000015_case_evidence_authorization.sql': '2cdf2542845d614d669c95c85c0a7fc041208f233814d383919ece8afc6a9855',
 'operations/evidence-authorization-privileges.sql': 'dbde0b36b157e3fb6bf1ad1b7c94adaf2768743eef09e8e43f90152ec4fede61',
}
SETTINGS = "SET LOCAL search_path=pg_catalog,public,pg_temp; SET LOCAL lock_timeout='5s'; SET LOCAL statement_timeout='60s';"
# DDL chooses current_schema(); select public explicitly after catalog preflight.
PREFLIGHT = """
DO $$ BEGIN
 IF current_database()<>'evt' OR current_user<>'postgres'
 OR (SELECT pg_get_userbyid(datdba) FROM pg_catalog.pg_database WHERE datname=current_database())<>'postgres'
 OR current_setting('server_version_num')::int NOT BETWEEN 160000 AND 169999
 OR EXISTS(SELECT 1 FROM pg_catalog.pg_roles WHERE rolname='cvera_evidence_operator')
 OR NOT EXISTS(SELECT 1 FROM pg_catalog.pg_roles WHERE rolname='cvera_api_runtime' AND rolcanlogin
   AND NOT (rolsuper OR rolcreatedb OR rolcreaterole OR rolbypassrls OR rolreplication))
 OR EXISTS(SELECT 1 FROM pg_catalog.pg_auth_members WHERE member='cvera_api_runtime'::regrole)
 OR pg_catalog.has_schema_privilege('cvera_api_runtime','public','CREATE')
 OR pg_catalog.has_database_privilege('cvera_api_runtime',current_database(),'CREATE')
 OR EXISTS(SELECT 1 FROM pg_catalog.pg_class WHERE relnamespace='public'::regnamespace
   AND relname IN ('cases','verification_checks','case_evidence_upload_grants'))
 OR EXISTS(SELECT 1 FROM pg_catalog.pg_proc WHERE pronamespace='public'::regnamespace
   AND proname IN ('check_evidence_upload','provision_evidence_case','provision_evidence_check',
   'grant_evidence_upload','revoke_evidence_upload','set_evidence_resource_state'))
 OR NOT EXISTS(SELECT 1 FROM pg_catalog.pg_class WHERE oid='public.domain_users'::regclass
   AND relowner='postgres'::regrole AND NOT (relrowsecurity OR relforcerowsecurity))
 THEN RAISE EXCEPTION 'unexpected production prerequisite; stop'; END IF;
END $$;
"""
VERIFY = """
DO $$ DECLARE item record; signature text; r text; BEGIN
 IF EXISTS(SELECT 1 FROM pg_catalog.pg_roles WHERE rolname='cvera_evidence_operator'
   AND (rolcanlogin OR rolsuper OR rolcreatedb OR rolcreaterole OR rolbypassrls OR rolreplication))
 OR EXISTS(SELECT 1 FROM pg_catalog.pg_auth_members WHERE member='cvera_evidence_operator'::regrole OR roleid='cvera_evidence_operator'::regrole)
 THEN RAISE EXCEPTION 'operator boundary failed'; END IF;
 FOR item IN SELECT * FROM pg_catalog.pg_class WHERE relnamespace='public'::regnamespace
   AND relname IN ('cases','verification_checks','case_evidence_upload_grants') LOOP
  IF item.relowner<>'postgres'::regrole OR item.relrowsecurity OR item.relforcerowsecurity
   OR EXISTS(SELECT 1 FROM pg_catalog.aclexplode(coalesce(item.relacl,pg_catalog.acldefault('r',item.relowner))) a WHERE a.grantee<>item.relowner)
   OR EXISTS(SELECT 1 FROM pg_catalog.pg_attribute WHERE attrelid=item.oid AND attacl IS NOT NULL)
  THEN RAISE EXCEPTION 'registry ownership/ACL/RLS failed'; END IF;
 END LOOP;
 IF EXISTS(SELECT 1 FROM public.cases) OR EXISTS(SELECT 1 FROM public.verification_checks)
 OR EXISTS(SELECT 1 FROM public.case_evidence_upload_grants)
 THEN RAISE EXCEPTION 'unexpected implicit resource provisioning'; END IF;
 FOR item IN SELECT * FROM pg_catalog.pg_proc WHERE pronamespace='public'::regnamespace
   AND proname IN ('check_evidence_upload','provision_evidence_case','provision_evidence_check',
    'grant_evidence_upload','revoke_evidence_upload','set_evidence_resource_state') LOOP
  IF item.proowner<>'postgres'::regrole OR NOT item.prosecdef OR item.provolatile<>'v'
   OR NOT coalesce(item.proconfig @> ARRAY['search_path=pg_catalog, public, pg_temp'],false)
   OR EXISTS(SELECT 1 FROM pg_catalog.aclexplode(coalesce(item.proacl,pg_catalog.acldefault('f',item.proowner))) a
     WHERE a.grantee<>item.proowner AND
      (a.grantee<>CASE WHEN item.proname='check_evidence_upload' THEN 'cvera_api_runtime'::regrole ELSE 'cvera_evidence_operator'::regrole END
       OR a.privilege_type<>'EXECUTE' OR a.is_grantable))
  THEN RAISE EXCEPTION 'function owner/path/ACL failed'; END IF;
  r:=CASE WHEN item.proname='check_evidence_upload' THEN 'cvera_api_runtime' ELSE 'cvera_evidence_operator' END;
  IF NOT pg_catalog.has_function_privilege(r,item.oid,'EXECUTE') THEN RAISE EXCEPTION 'required function permission absent'; END IF;
 END LOOP;
END $$;
SET LOCAL ROLE cvera_api_runtime;
DO $$ BEGIN
 IF public.check_evidence_upload(NULL::uuid,'__no_case__','__no_check__') IS DISTINCT FROM false
 OR pg_catalog.to_regprocedure('check_evidence_upload(uuid,text,text)') <> 'public.check_evidence_upload(uuid,text,text)'::regprocedure
 THEN RAISE EXCEPTION 'runtime denial/schema resolution failed'; END IF;
END $$;
RESET ROLE;
"""


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


def main():
 if os.geteuid()!=0 or sys.argv[1:]!=['--approved-release']:
  raise RuntimeError('owner root console and approved release flag required')
 os.umask(0o077)
 if run(['git','-C',str(ROOT),'status','--porcelain']).strip():raise RuntimeError('worktree not clean')
 if run(['git','-C',str(ROOT),'rev-parse','HEAD']).decode().strip()!=BASE:raise RuntimeError('unexpected deployed checkout')
 with urllib.request.urlopen('http://127.0.0.1:8080/version',timeout=5) as response:
  if json.load(response).get('git_sha')!=BASE:raise RuntimeError('unexpected running revision')
 before={name:(info(name,'Id'),info(name,'State.StartedAt'),info(name,'Image')) for name in [DB,'evt-api-go-api-1','evt-api-go-kratos-1','evt-api-go-kratos-courier-1']}
 if not all(info(name,'State.Running') and not info(name,'State.OOMKilled') for name in before):raise RuntimeError('service precondition failed')
 env=dict(x.split('=',1) for x in info('evt-api-go-api-1','Config.Env') if '=' in x)
 uri=urllib.parse.urlsplit(env['EVT_DB_DSN']);query=urllib.parse.parse_qs(uri.query)
 if urllib.parse.unquote(uri.username or '')!='cvera_api_runtime' or uri.path!='/evt' or set(query)-{'sslmode','search_path'} or query.get('search_path',['public'])!=['public']:
  raise RuntimeError('unexpected API database configuration')
 envfile=ROOT/'apps/evt-api-go/.env';mode=envfile.stat()
 if (mode.st_uid,mode.st_gid,stat.S_IMODE(mode.st_mode))!=(0,0,0o600):raise RuntimeError('protected configuration changed')
 env_hash=hashlib.sha256(envfile.read_bytes()).hexdigest()
 # Source artifacts come from the fetched reviewed branch, never the live tree.
 source_sha=run(['git','-C',str(ROOT),'rev-parse','refs/remotes/origin/security/weekend-2-case-upload-authorization']).decode().strip()
 sources={}
 for name,expected in FILES.items():
  data=run(['git','-C',str(ROOT),'show',source_sha+':apps/evt-api-go/'+name])
  if hashlib.sha256(data).hexdigest()!=expected:raise RuntimeError('reviewed SQL hash mismatch')
  sources[name]=data.decode()
 sql('BEGIN READ ONLY;'+SETTINGS+PREFLIGHT+'ROLLBACK;')
 size=int(sql('BEGIN READ ONLY;SELECT pg_catalog.pg_database_size(current_database());ROLLBACK;').decode().strip())
 if shutil.disk_usage('/opt/cvera').free < size*3+1024**3:raise RuntimeError('insufficient backup/recovery capacity')
 stamp=datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ')
 backup=Path('/opt/cvera/proposal-b-release-'+stamp);backup.mkdir(mode=0o700)
 private(backup/'release-intent.json',json.dumps({'review':source_sha,'base':BASE,'state':'started; inspect before any retry'}).encode())
 dump=run(['docker','exec',DB,'pg_dump','-U','postgres','-d','evt','-Fc'],timeout=180)
 private(backup/'evt.dump',dump)
 run(['docker','exec','-i',DB,'pg_restore','--list'],data=dump)
 private(backup/'roles-without-passwords.sql',run(['docker','exec',DB,'pg_dumpall','-U','postgres','--roles-only','--no-role-passwords']))
 private(backup/'backup.sha256',(hashlib.sha256(dump).hexdigest()+'  evt.dump\n').encode())
 tag='cvera-api:recovery-proposal-b-'+stamp.lower()
 run(['docker','image','tag',before['evt-api-go-api-1'][2],tag])
 if json.loads(run(['docker','image','inspect','--format','{{json .Id}}',tag]))!=before['evt-api-go-api-1'][2]:raise RuntimeError('recovery image mismatch')
 private(backup/'api-recovery-tag.txt',(tag+'\n').encode())
 # No env/Compose change is included in this release; retain current source and
 # root-private env unchanged. Recovery can recreate only API using this image.
 private(backup/'compose-before.yml',(ROOT/'apps/evt-api-go/docker-compose.yml').read_bytes())
 body='BEGIN ISOLATION LEVEL READ COMMITTED;'+SETTINGS+PREFLIGHT+'''
 SET LOCAL search_path=public,pg_temp;
 CREATE ROLE cvera_evidence_operator NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS;
 SET LOCAL cvera.evidence_runtime_role='cvera_api_runtime';
 SET LOCAL cvera.evidence_operator_role='cvera_evidence_operator';
 '''+sources['migrations/000015_case_evidence_authorization.sql']+'\n'+sources['operations/evidence-authorization-privileges.sql']+'\n'+VERIFY+'\nSELECT pg_catalog.txid_current();\nCOMMIT;'
 private(backup/'sql-intent.json',json.dumps({'migration':'000015 only','review':source_sha,'hashes':FILES}).encode())
 transaction=sql(body).decode().strip()
 private(backup/'sql-commit.json',json.dumps({'migration':'000015 only','review':source_sha,'hashes':FILES,'transaction':transaction,'verification':'PASS','application_records_changed':False}).encode())
 sql('BEGIN ISOLATION LEVEL READ COMMITTED;'+SETTINGS+VERIFY+'ROLLBACK;')
 if hashlib.sha256(envfile.read_bytes()).hexdigest()!=env_hash:raise RuntimeError('configuration changed unexpectedly')
 for name,baseline in before.items():
  if (info(name,'Id'),info(name,'State.StartedAt'),info(name,'Image'))!=baseline or not info(name,'State.Running'):raise RuntimeError('service changed unexpectedly')
 print(json.dumps({'proposal_b_sql':'PASS','review':source_sha,'migration':'000015 only','backup_archive_validated':True,'recovery_image_retained':True,'operator':'cvera_evidence_operator NOLOGIN','runtime':'cvera_api_runtime checker EXECUTE only','resources_or_grants_created':0,'credentials_configuration_services_changed':False,'private_record':str(backup/'sql-commit.json')}))


if __name__=='__main__':
 try:main()
 except Exception:
  print('STOP: Proposal B SQL phase failed or a precondition changed. Do not retry, merge or deploy; return this status for inspection. Raw output suppressed.',file=sys.stderr)
  sys.exit(1)
