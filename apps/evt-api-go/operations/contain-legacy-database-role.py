#!/usr/bin/env python3
"""Approved legacy-role NOLOGIN only. No password, data or service operations."""
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile
import urllib.parse

ROOT=Path('/opt/cvera/rsa-attestation-engine')
BACKUP=Path('/opt/cvera/weekend2-closeout-20261102T004322Z')
STAGE=BACKUP/'reviewed-afa946d'
APPROVED='d646f8949a9bcbe6a0fbb47ff0577ac8d296ccdd'
DB='evt-api-go-db-1'
RECORD=BACKUP/'legacy-role-containment-d646f89.json'
ENVFILE=ROOT/'apps/evt-api-go/.env'
HELPER='apply-weekend2-sql-preparation.py'
HELPER_SHA='4ec24925a859a6ea5a368b49c2c051e4eed0acedc327b01ead082aaae8d8639b'
SNAPSHOT="""
SELECT json_build_object(
 'databases',(SELECT json_agg(json_build_array(datname,datdba,datacl::text) ORDER BY oid)
 FROM pg_database WHERE datname IN ('evt','evt_kratos')),
 'schema',(SELECT json_agg(json_build_array(nspname,nspowner,nspacl::text) ORDER BY oid)
 FROM pg_namespace WHERE nspname='public'),
 'relations',(SELECT json_agg(json_build_array(oid,relname,relowner,relacl::text,relrowsecurity,relforcerowsecurity) ORDER BY oid)
 FROM pg_class WHERE relnamespace='public'::regnamespace),
 'columns',(SELECT json_agg(json_build_array(a.attrelid,a.attnum,a.attacl::text) ORDER BY a.attrelid,a.attnum)
 FROM pg_attribute a JOIN pg_class c ON c.oid=a.attrelid
 WHERE c.relnamespace='public'::regnamespace AND a.attnum>0 AND NOT a.attisdropped),
 'functions',(SELECT json_agg(json_build_array(oid,proowner,proacl::text) ORDER BY oid)
 FROM pg_proc WHERE pronamespace='public'::regnamespace),
 'defaults',(SELECT json_agg(json_build_array(oid,defaclrole,defaclnamespace,defaclobjtype,defaclacl::text) ORDER BY oid)
 FROM pg_default_acl),
 'memberships',(SELECT json_agg(json_build_array(roleid,member,grantor,admin_option,inherit_option,set_option) ORDER BY roleid,member)
 FROM pg_auth_members),
 'legacy_flags',(SELECT json_build_array(oid,rolsuper,rolinherit,rolcreaterole,rolcreatedb,rolreplication,rolbypassrls,rolconnlimit)
 FROM pg_roles WHERE rolname='evt-app')
)::text;
"""
PRECONDITION="""DO $$ BEGIN
 IF current_user<>'postgres' OR current_database()<>'evt'
 THEN RAISE EXCEPTION 'unexpected executor/database'; END IF;
 IF (SELECT count(*) FROM pg_roles WHERE rolname='evt-app' AND rolcanlogin
     AND NOT rolsuper AND NOT rolcreaterole AND NOT rolcreatedb AND NOT rolbypassrls)<>1
 OR EXISTS(SELECT 1 FROM pg_auth_members m JOIN pg_roles r
 ON r.oid=m.member OR r.oid=m.roleid WHERE r.rolname='evt-app')
 THEN RAISE EXCEPTION 'legacy role state requires review'; END IF;
 IF EXISTS(SELECT 1 FROM pg_stat_activity WHERE usename='evt-app')
 THEN RAISE EXCEPTION 'legacy sessions require owner review; no session termination authorized'; END IF;
END $$;"""
POSTCONDITION="""DO $$ BEGIN
 IF (SELECT rolcanlogin FROM pg_roles WHERE rolname='evt-app') IS DISTINCT FROM false
 OR EXISTS(SELECT 1 FROM pg_stat_activity WHERE usename='evt-app')
 THEN RAISE EXCEPTION 'NOLOGIN/session verification failed; keep containment and hold rollout'; END IF;
END $$;"""


def run(args,data=None,env=None):
    result=subprocess.run(args,input=data,env=env,capture_output=True,timeout=120)
    if result.returncode:raise RuntimeError('Command failed; raw output suppressed')
    return result.stdout


def info(name,field):
    return json.loads(run(['docker','inspect','--format','{{json .'+field+'}}',name]))


def private_write(path,data):
    fd=os.open(path,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
    with os.fdopen(fd,'wb') as stream:
        stream.write(data);stream.flush();os.fsync(stream.fileno())


def save(record):
    following=RECORD.with_suffix('.json.next')
    private_write(following,(json.dumps(record,indent=2)+'\n').encode())
    os.replace(following,RECORD)


def load_helper():
    source=(STAGE/HELPER).read_bytes()
    reviewed=run(['git','-C',str(ROOT),'show',APPROVED+':apps/evt-api-go/operations/'+HELPER])
    if source!=reviewed or hashlib.sha256(source).hexdigest()!=HELPER_SHA:
        raise RuntimeError('Reviewed helper mismatch')
    namespace={'__name__':'containment_sql_helper','__file__':str(STAGE/HELPER)}
    exec(compile(source,str(STAGE/HELPER),'exec'),namespace)
    return namespace


def consumers():
    names=run(['docker','ps','-a','--format','{{.Names}}']).decode().split()
    required={'evt-api-go-api-1','evt-api-go-kratos-1','evt-api-go-kratos-courier-1'}
    if not required.issubset(names) or not all(info(name,'State.Running') for name in required):
        raise RuntimeError('Expected production service is not running')
    allowed=required|{DB,'cvera-weekend2-restore-20261102T004322Z'}
    if set(names)-allowed:raise RuntimeError('Unreviewed container requires consumer review')
    baseline={}
    for name in names:
        entries=info(name,'Config.Env') or []
        values=dict(x.split('=',1) for x in entries if '=' in x)
        for key,value in values.items():
            if key in ('PGUSER','POSTGRES_USER','DB_USER','DATABASE_USER') and value=='evt-app':
                raise RuntimeError('Configured legacy-role consumer found')
            if value.startswith(('postgres://','postgresql://')):
                parsed=urllib.parse.urlsplit(value)
                query=urllib.parse.parse_qs(parsed.query,keep_blank_values=True)
                if urllib.parse.unquote(parsed.username or '')=='evt-app' or 'evt-app' in query.get('user',[]):
                    raise RuntimeError('Configured legacy-role connection found')
        if name in required:
            baseline[name]={'id':info(name,'Id'),'started':info(name,'State.StartedAt'),'environment':entries}
    return baseline,len(names)


def verify_replacement(uri,role,database):
    parsed=urllib.parse.urlsplit(uri)
    query=urllib.parse.parse_qs(parsed.query,keep_blank_values=True)
    if (urllib.parse.unquote(parsed.username or '')!=role or not parsed.password
        or urllib.parse.unquote(parsed.path.strip('/'))!=database
        or set(query)&{'user','password','passfile','sslpassword','dbname','host','hostaddr','port','options','service','search_path'}):
        raise RuntimeError('Prepared connection identity/options require review')
    endpoint=parsed.netloc.rsplit('@',1)[-1]
    clean=urllib.parse.urlunsplit(parsed._replace(netloc=role+'@'+endpoint))
    env={k:v for k,v in os.environ.items() if not k.startswith('PG')}
    env['PGPASSWORD']=urllib.parse.unquote(parsed.password)
    output=run(['docker','exec','-i','-e','PGPASSWORD',DB,'psql','-X','-w','-d',clean,
        '-Atq','-v','ON_ERROR_STOP=1','-c','SELECT current_user,pg_catalog.current_database();'],env=env).decode().strip()
    if output!=role+'|'+database:raise RuntimeError('Replacement connectivity verification failed')


def main():
    if os.geteuid()!=0 or sys.argv[1:]!=['--contain-approved-d646f89']:
        raise RuntimeError('Approved owner root-console invocation required')
    os.umask(0o077)
    if RECORD.exists() or RECORD.with_suffix('.json.next').exists():
        raise RuntimeError('Existing intent: reconcile, never retry blindly')
    helper=load_helper()
    helper['private'](BACKUP,True);helper['private'](STAGE,True);helper['private'](ENVFILE)
    if ENVFILE.stat().st_uid!=0 or ENVFILE.stat().st_gid!=0:
        raise RuntimeError('Prepared private environment ownership changed')
    prepared=json.loads((BACKUP/'runtime-credentials-1c228ee.json').read_text())
    if prepared.get('state')!='complete: restricted LOGIN verified; protected configuration published; services unchanged':
        raise RuntimeError('Completed runtime preparation required')
    expected_env_hash=prepared.get('env_sha256')
    if not expected_env_hash or hashlib.sha256(ENVFILE.read_bytes()).hexdigest()!=expected_env_hash:
        raise RuntimeError('Prepared environment changed; review consumers before containment')
    if run(['git','-C',str(ROOT),'status','--porcelain']).strip():raise RuntimeError('Unexpected worktree changes')
    if run(['git','-C',str(ROOT),'rev-parse','HEAD']).decode().strip()!='2813e7e513b6afed5cad3b77bee6363e982e21d2':
        raise RuntimeError('Application revision changed; containment preconditions require review')
    baseline,count=consumers()
    # The exposed owner can create objects in the identity database's public
    # schema. Owner metadata/ALTER ROLE sessions must not resolve its functions.
    def sql(database,body,readonly=False):
        return helper['sql'](database,'SET LOCAL search_path=pg_catalog,pg_temp;'+body,readonly=readonly)
    sql('evt',PRECONDITION,readonly=True)
    before={db:hashlib.sha256(sql(db,SNAPSHOT,readonly=True).encode()).hexdigest() for db in ['evt','evt_kratos']}
    # Parse only the prepared model in memory. No credentials appear in the
    # record/output; the temporary Compose source contains no credential values.
    with tempfile.TemporaryDirectory(prefix='containment-preview-',dir=BACKUP) as folder:
        compose=Path(folder)/'compose.yml'
        private_write(compose,run(['git','-C',str(ROOT),'show',APPROVED+':apps/evt-api-go/docker-compose.yml']))
        env=os.environ.copy()
        for key in ['EVT_DB_DSN','KRATOS_DB_DSN']:env.pop(key,None)
        model=json.loads(run(['docker','compose','--project-name','evt-api-go',
            '--project-directory',str(ROOT/'apps/evt-api-go'),'--env-file',str(ENVFILE),
            '-f',str(compose),'config','--format','json'],env=env))
    api_uri=model['services']['api']['environment']['EVT_DB_DSN']
    provider_uri=model['services']['kratos']['environment']['DSN']
    if provider_uri!=model['services']['kratos-courier']['environment']['DSN']:
        raise RuntimeError('Prepared provider routes differ')
    verify_replacement(api_uri,'cvera_api_runtime','evt')
    verify_replacement(provider_uri,'cvera_kratos_runtime','evt_kratos')
    # Capture historical known credential privately; never emit its source/URI.
    old=run(['git','-C',str(ROOT),'show','2813e7e513b6afed5cad3b77bee6363e982e21d2:infra/kratos/kratos.yml']).decode()
    match=re.search(r'^dsn:\s*(\S+)',old,re.MULTILINE)
    if not match:raise RuntimeError('Historical credential source mismatch')
    exposed=urllib.parse.urlsplit(match.group(1).strip('\"\''))
    if urllib.parse.unquote(exposed.username or '')!='evt-app' or not exposed.password:
        raise RuntimeError('Unexpected historical principal')
    record={'approved_commit':APPROVED,'started_utc':datetime.now(timezone.utc).isoformat(),
        'state':'intent','change':'evt-app NOLOGIN only','configured_containers_checked':count,
        'ownership_acl_before_sha256':before,'sessions_terminated':0}
    private_write(RECORD,(json.dumps(record,indent=2)+'\n').encode())
    try:
        current,_=consumers()
        if current!=baseline:raise RuntimeError('Running service changed during preflight')
        change="SELECT pg_advisory_xact_lock(hashtext('cvera:legacy-evt-app-containment'));"+PRECONDITION
        change+='ALTER ROLE "evt-app" NOLOGIN; SELECT pg_current_xact_id()::text;'
        record['transaction']=sql('evt',change)
        record['state']='NOLOGIN commit returned; fresh verification pending';save(record)
        sql('evt',POSTCONDITION,readonly=True)
        after={db:hashlib.sha256(sql(db,SNAPSHOT,readonly=True).encode()).hexdigest() for db in before}
        if before!=after:raise RuntimeError('Ownership/ACL/role-property metadata changed unexpectedly')
        address=run(['docker','exec',DB,'hostname','-i']).decode().split()[0]
        if address in ('127.0.0.1','::1'):raise RuntimeError('SCRAM host path required')
        env={k:v for k,v in os.environ.items() if not k.startswith('PG')}
        env['PGPASSWORD']=urllib.parse.unquote(exposed.password)
        uri='postgres://evt-app@'+address+':5432/evt_kratos?sslmode=disable'
        result=subprocess.run(['docker','exec','-i','-e','PGPASSWORD',DB,'psql','-X','-w',
            '-d',uri,'-Atq','-c','SELECT current_user'],env=env,capture_output=True,timeout=30)
        if result.returncode==0 or b'not permitted to log in' not in result.stderr:
            raise RuntimeError('Exposed credential was not rejected specifically by NOLOGIN')
        verify_replacement(api_uri,'cvera_api_runtime','evt')
        verify_replacement(provider_uri,'cvera_kratos_runtime','evt_kratos')
        current,_=consumers()
        if current!=baseline:raise RuntimeError('Running services changed; keep NOLOGIN and hold rollout')
        sql('evt',POSTCONDITION,readonly=True)
        if hashlib.sha256(ENVFILE.read_bytes()).hexdigest()!=expected_env_hash:
            raise RuntimeError('Prepared configuration changed; keep NOLOGIN and hold rollout')
        record['state']='complete: NOLOGIN verified; zero legacy sessions; ownership/grants and replacements preserved'
        record['completed_utc']=datetime.now(timezone.utc).isoformat();save(record)
        print(json.dumps({'legacy_role_containment':'PASS','legacy_role':'evt-app','legacy_nologin':True,
            'transaction':record['transaction'],'legacy_sessions':0,'sessions_terminated':0,
            'exposed_credential_rejected':True,'ownership_and_grants_unchanged':True,
            'replacement_connectivity':True,'passwords_data_services_migration_history_changed':False,
            'merge_or_deployment':False,'private_record':str(RECORD)}))
    except Exception:
        record['execution_stopped']=True;record['stopped_utc']=datetime.now(timezone.utc).isoformat();save(record)
        raise


if __name__=='__main__':
    try:main()
    except Exception:
        print('STOP: containment halted. Do not retry, terminate sessions, re-enable LOGIN or continue rollout. Return this status without private files/raw errors.',file=sys.stderr)
        sys.exit(1)
