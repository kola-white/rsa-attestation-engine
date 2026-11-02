#!/usr/bin/env python3
"""Read-only connection/image verification and root-private recovery artifacts.

No SQL mutation, image operation, service start/restart, merge or deployment.
Snapshot contains credentials: never print or publish its contents.
"""
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import urllib.parse

ROOT=Path('/opt/cvera/rsa-attestation-engine')
BACKUP=Path('/opt/cvera/weekend2-closeout-20261102T004322Z')
REVIEW='12a7b36a4cd1dcc2777f2178af85ec287617ce88'
OVERRIDE=BACKUP/'rollout-recovery.override.json'
SOURCE=BACKUP/'rollout-recovery.compose.yml'
RECORD=BACKUP/'rollout-recovery.json'
DB='evt-api-go-db-1'


def run(args,data=None,env=None):
    result=subprocess.run(args,input=data,env=env,capture_output=True,timeout=90)
    if result.returncode:raise RuntimeError('Recovery preflight command failed; raw output suppressed')
    return result.stdout


def info(name,field):
    return json.loads(run(['docker','inspect','--format','{{json .'+field+'}}',name]))


def write_private(path,data):
    fd=os.open(path,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
    with os.fdopen(fd,'wb') as stream:
        stream.write(data);stream.flush();os.fsync(stream.fileno())


def connection(uri,database):
    parsed=urllib.parse.urlsplit(uri)
    query=urllib.parse.parse_qs(parsed.query,keep_blank_values=True)
    if (parsed.scheme not in ('postgres','postgresql') or parsed.username!='postgres'
        or not parsed.password or parsed.path.strip('/')!=database
        or set(query)&{'user','password','passfile','sslpassword','dbname','host','hostaddr','port','options','service','search_path'}):
        raise RuntimeError('Captured pre-rollout connection requires review')
    endpoint=parsed.netloc.rsplit('@',1)[-1]
    clean=urllib.parse.urlunsplit(parsed._replace(netloc='postgres@'+endpoint))
    env={k:v for k,v in os.environ.items() if not k.startswith('PG')}
    env['PGPASSWORD']=urllib.parse.unquote(parsed.password)
    result=run(['docker','exec','-i','-e','PGPASSWORD',DB,'psql','-X','-w','-d',clean,
                '-Atq','-v','ON_ERROR_STOP=1','-c',
                "SELECT current_user,pg_catalog.current_database(),"
                "coalesce(pg_catalog.inet_client_addr() NOT IN ('127.0.0.1'::pg_catalog.inet,'::1'::pg_catalog.inet),false)"
                " AND NOT EXISTS(SELECT 1 FROM pg_catalog.pg_hba_file_rules WHERE error IS NOT NULL"
                " OR (type LIKE 'host%' AND address NOT IN ('127.0.0.1','::1') AND auth_method<>'scram-sha-256'));"],env=env)
    if result.decode().strip()!='postgres|'+database+'|t':raise RuntimeError('Captured connection identity mismatch')


def main():
    if os.geteuid()!=0 or sys.argv[1:]:raise RuntimeError('Run without arguments in owner root console')
    os.umask(0o077)
    mode=BACKUP.stat()
    if mode.st_uid!=0 or mode.st_mode & 0o777!=0o700:raise RuntimeError('Private recovery directory required')
    if any(path.exists() for path in [OVERRIDE,SOURCE,RECORD]):raise RuntimeError('Recovery artifacts already exist; inspect instead of overwriting')
    contained=json.loads((BACKUP/'legacy-role-containment-d646f89.json').read_text())
    if contained.get('state')!='complete: NOLOGIN verified; zero legacy sessions; ownership/grants and replacements preserved':
        raise RuntimeError('Verified legacy containment required')
    if run(['git','-C',str(ROOT),'status','--porcelain']).strip():raise RuntimeError('Unexpected worktree changes')
    if run(['git','-C',str(ROOT),'rev-parse','HEAD']).decode().strip()!='2813e7e513b6afed5cad3b77bee6363e982e21d2':
        raise RuntimeError('Pre-rollout revision changed')
    source=run(['git','-C',str(ROOT),'show',REVIEW+':apps/evt-api-go/docker-compose.yml'])
    recovery_api=(BACKUP/'api-recovery-tag.txt').read_text().strip()
    image_ids={};services={};before={}
    for service in ['api','kratos','kratos-courier']:
        container='evt-api-go-'+service+'-1'
        if not info(container,'State.Running'):raise RuntimeError('Expected running service unavailable')
        image=info(container,'Image');image_ids[service]=image
        environment=dict(x.split('=',1) for x in info(container,'Config.Env') if '=' in x)
        connection(environment['EVT_DB_DSN' if service=='api' else 'DSN'],'evt' if service=='api' else 'evt_kratos')
        reference=recovery_api if service=='api' else 'oryd/kratos:v1.1.0'
        cached=json.loads(run(['docker','image','inspect','--format','{{json .Id}}',reference]))
        if cached!=image:raise RuntimeError('Recovery image reference does not match running image')
        before[service]={'id':info(container,'Id'),'started':info(container,'State.StartedAt'),'environment':environment}
        # Compose interpolation must not change literal dollars in credentials.
        services[service]={'image':reference,'environment':{k:v.replace('$','$$') for k,v in environment.items()}}
    if image_ids['api']!=(BACKUP/'api-image-before.txt').read_text().strip():
        raise RuntimeError('API recovery image differs from original retained image')
    override=(json.dumps({'services':services},indent=2)+'\n').encode()
    envfile=ROOT/'apps/evt-api-go/.env'
    mode=envfile.stat()
    if (mode.st_uid,mode.st_gid,mode.st_mode & 0o777)!=(0,0,0o600):raise RuntimeError('Protected prepared environment required')
    # Validate actual Compose merge privately before preserving final artifacts.
    with tempfile.TemporaryDirectory(prefix='recovery-verify-',dir=BACKUP) as folder:
        compose=Path(folder)/'compose.yml';overlay=Path(folder)/'override.json'
        write_private(compose,source);write_private(overlay,override)
        rendered=json.loads(run(['docker','compose','--project-name','evt-api-go',
            '--project-directory',str(ROOT/'apps/evt-api-go'),'--env-file',str(envfile),
            '-f',str(compose),'-f',str(overlay),'config','--format','json']))
    for service,baseline in before.items():
        expected=baseline['environment'];actual=rendered['services'][service]['environment']
        if set(actual)!=set(expected) or any(actual[k]!=v and actual[k].replace('$$','$')!=v for k,v in expected.items()):
            raise RuntimeError('Recovery environment model differs from captured service')
        container='evt-api-go-'+service+'-1'
        if info(container,'Id')!=baseline['id'] or info(container,'State.StartedAt')!=baseline['started'] or not info(container,'State.Running'):
            raise RuntimeError('Service changed during recovery preparation')
    help_text=run(['docker','compose','up','--help']).decode()
    if any(flag not in help_text for flag in ['--no-deps','--no-build','--pull']):
        raise RuntimeError('Required supported Compose recovery flags absent')
    write_private(SOURCE,source);write_private(OVERRIDE,override)
    record={'reviewed_compose_commit':REVIEW,'captured_main':'2813e7e513b6afed5cad3b77bee6363e982e21d2',
        'image_ids':image_ids,'source_sha256':hashlib.sha256(source).hexdigest(),
        'override_sha256':hashlib.sha256(override).hexdigest(),
        'fresh_original_connections_verified':True,'exact_captured_environment_model_verified':True,
        'scope':'private recovery preparation only; no rollback/deployment executed',
        'security_warning':'Emergency recovery restores baseline application weaknesses/superuser service connections; retain legacy NOLOGIN and schema. Requires rollout/recovery authority.'}
    write_private(RECORD,(json.dumps(record,indent=2)+'\n').encode())
    print(json.dumps({'rollout_recovery_preparation':'PASS','original_connections_verified':3,
        'retained_images_verified':True,'private_compose_model_verified':True,
        'private_artifacts_mode':'root 0600 inside 0700 recovery directory',
        'credentials_data_configuration_services_changed':False,'rollback_or_deployment_executed':False,
        'private_record':str(RECORD)}))


if __name__=='__main__':
    try:main()
    except Exception:
        print('STOP: recovery preflight failed; no raw credentials/output emitted. Do not merge, deploy or overwrite private artifacts.',file=sys.stderr)
        sys.exit(1)
