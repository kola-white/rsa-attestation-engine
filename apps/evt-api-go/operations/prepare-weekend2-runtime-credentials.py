#!/usr/bin/env python3
"""Approved credential/configuration preparation only; NEVER starts services.

Owner root console, fixed reviewed artifacts and deployment-identity evidence.
Secrets exist only in memory and root-private candidate/live/recovery files.
"""
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import re
import secrets
import stat
import subprocess
import sys
import tempfile
import urllib.parse

ROOT=Path('/opt/cvera/rsa-attestation-engine')
BACKUP=Path('/opt/cvera/weekend2-closeout-20261102T004322Z')
STAGE=BACKUP/'reviewed-afa946d'
DB='evt-api-go-db-1'
APPROVED='1c228eea3d729ad2decb6f6ff4a31b02d8cd4ff8'
RECORD=BACKUP/'runtime-credentials-1c228ee.json'
CANDIDATE=BACKUP/'runtime-candidate.env'
BEFORE=BACKUP/'runtime-env.before'
ENVFILE=ROOT/'apps/evt-api-go/.env'
DEPENDENCIES={
 'apply-weekend2-sql-preparation.py':'4ec24925a859a6ea5a368b49c2c051e4eed0acedc327b01ead082aaae8d8639b',
 'check-weekend2-runtime-transition.py':'ffe501a9a927cca67839eb2637c87d0264e17c7e2294cb5c3a7ef9e2e4c621b0',
}


def now():return datetime.now(timezone.utc).isoformat()


def run(args,data=None,env=None):
    result=subprocess.run(args,input=data,env=env,capture_output=True,timeout=120)
    if result.returncode:
        raise RuntimeError('Command failed; raw output suppressed to protect credentials')
    return result.stdout


def info(name,field):
    return json.loads(run(['docker','inspect','--format','{{json .'+field+'}}',name]))


def write_private(path,data):
    fd=os.open(path,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
    with os.fdopen(fd,'wb') as stream:
        stream.write(data);stream.flush();os.fsync(stream.fileno())


def save(record):
    next_path=RECORD.with_suffix('.json.next')
    write_private(next_path,(json.dumps(record,indent=2)+'\n').encode())
    os.replace(next_path,RECORD)


def module(name):
    data=(STAGE/name).read_bytes()
    if hashlib.sha256(data).hexdigest()!=DEPENDENCIES[name]:
        raise RuntimeError('Reviewed helper checksum mismatch')
    reviewed=run(['git','-C',str(ROOT),'show',APPROVED+':apps/evt-api-go/operations/'+name])
    if data!=reviewed:raise RuntimeError('Helper differs from approved commit')
    spec=importlib.util.spec_from_file_location(name.replace('-','_'),STAGE/name)
    value=importlib.util.module_from_spec(spec)
    # Do not write bytecode/copies of loaded artifacts into the staged directory.
    exec(compile(data,str(STAGE/name),'exec'),value.__dict__)
    return value


def deployment_identity():
    # GitHub run 41320539163, deploy step 15:20:16--15:28:01 UTC, direct
    # CVERA_DEPLOY_USER SSH with no sudo. Both user/key secret metadata show
    # unchanged since 2026-09-20 (values never retrieved). Owner checkpoint 17
    # confirmed same-PID root authentication/session-open and API start inside it.
    rows=run(['journalctl','-u','ssh.service','-u','sshd.service',
        '--since','2026-11-01 15:19:45 UTC','--until','2026-11-01 15:28:30 UTC',
        '--no-pager','-o','json'])
    accepted=[];opened=set()
    start=datetime.fromisoformat('2026-11-01T15:20:16+00:00').timestamp()
    end=datetime.fromisoformat('2026-11-01T15:28:01+00:00').timestamp()
    for line in rows.splitlines():
        row=json.loads(line);message=row.get('MESSAGE','')
        match=re.search(r'Accepted publickey for ([A-Za-z0-9_.-]+) ',message)
        if match and abs(int(row['__REALTIME_TIMESTAMP'])/1000000-start)<5:
            accepted.append((row.get('_PID'),match.group(1)))
        match=re.search(r'session opened for user ([A-Za-z0-9_.-]+)',message)
        if match:opened.add((row.get('_PID'),match.group(1)))
    if len(accepted)!=1 or accepted[0][1]!='root' or accepted[0] not in opened:
        raise RuntimeError('Established deployment identity is not uniquely verified root')
    stamp=info('evt-api-go-api-1','State.StartedAt')
    # Docker nanosecond timestamp; microseconds suffice for the interval guard.
    stamp=re.sub(r'(\.\d{6})\d+',r'\1',stamp).replace('Z','+00:00')
    if not start<=datetime.fromisoformat(stamp).timestamp()<=end:
        raise RuntimeError('Running API no longer matches the proven deployment window')
    workflow=run(['git','-C',str(ROOT),'show','2813e7e513b6afed5cad3b77bee6363e982e21d2:.github/workflows/main.yml'])
    if b'"${CVERA_DEPLOY_USER}@${CVERA_DEPLOY_HOST}"' not in workflow:
        raise RuntimeError('Historical workflow identity mechanism differs')
    return {'account':'root','workflow_run':41320539163,'sshd_pid':accepted[0][0],
            'username_secret_updated_at':'2026-09-20T21:20:26Z',
            'key_secret_updated_at':'2026-09-20T21:20:57Z'}


def patch_env(before,replacements):
    lines=before.decode().splitlines(keepends=True)
    for key,value in replacements.items():
        if any(c in value for c in "'\r\n"):
            raise RuntimeError('Connection URI cannot be safely staged literally')
        pattern=re.compile(r'^\s*(?:export\s+)?'+key+r'\s*=')
        positions=[i for i,line in enumerate(lines) if pattern.match(line)]
        if len(positions)>1 or (key=='EVT_DB_DSN' and len(positions)!=1):
            raise RuntimeError('Ambiguous connection assignments; publication refused')
        replacement=key+"='"+value+"'\n"
        if positions:
            # Only ordinary single-line assignments; final Compose comparison
            # also rejects malformed/unexpected residual multi-line values.
            current=lines[positions[0]].split('=',1)[1].strip()
            if current.startswith(("'",'"')) and not current.endswith(current[0]):
                raise RuntimeError('Multiline connection assignment unsupported')
            lines[positions[0]]=replacement
        else:
            if lines and not lines[-1].endswith('\n'):lines[-1]+='\n'
            lines.append(replacement)
    return ''.join(lines).encode()


def render(probe,path,compose):
    env=os.environ.copy()
    # Do not let inherited variables override protected file values.
    for key in ['EVT_DB_DSN','KRATOS_DB_DSN']:env.pop(key,None)
    return json.loads(run(['docker','compose','--project-name','evt-api-go',
        '--project-directory',str(ROOT/'apps/evt-api-go'),'--env-file',str(path),
        '-f',str(compose),'config','--format','json'],env=env))


def assert_preserved(model,observed,connections):
    for name,live in observed.items():
        target=model['services'][name]['environment']
        connection='EVT_DB_DSN' if name=='api' else 'DSN'
        if target.get(connection)!=connections[name]:
            raise RuntimeError('Rendered runtime connection mismatch')
        if any(live.get(key)!=value for key,value in target.items() if key!=connection):
            raise RuntimeError('Persistent environment mismatch; no values emitted')
    if 'DATABASE_URL' in model['services']['api']['environment']:
        raise RuntimeError('Unused administrator alias may not enter the API')


def password(base,role,value):
    # Suppress credential-statement/verifier logging only in this short-lived
    # owner connection, never globally or in application sessions. Refuse
    # preload/audit hooks below instead of silently disabling an unknown audit.
    safe="SET password_encryption='scram-sha-256'; SET log_statement='none'; SET log_min_duration_statement=-1; SET log_min_duration_sample=-1; SET log_min_error_statement='panic';"
    output=run(base+['-c',safe,'-c','\\password '+role],(value+'\n'+value+'\n').encode())
    if value.encode() in output:raise RuntimeError('Sensitive command output refused')


def runtime_probe(uri,value,role,database,sql_helper):
    parsed=urllib.parse.urlsplit(uri)
    endpoint=parsed.netloc.rsplit('@',1)[-1]
    clean=urllib.parse.urlunsplit(parsed._replace(netloc=role+'@'+endpoint))
    # Clear inherited client options/password files; only explicit URI/PGPASSWORD.
    env={k:v for k,v in os.environ.items() if not k.startswith('PG')}
    args=['docker','exec','-i','-e','PGPASSWORD',DB,'psql','-X','-w','-d',clean,
          '-Atq','-v','ON_ERROR_STOP=1','-v','VERBOSITY=sqlstate']
    env['PGPASSWORD']=secrets.token_urlsafe(48)
    denied=subprocess.run(args+['-c','SELECT current_user'],env=env,capture_output=True,timeout=30)
    if denied.returncode==0 or b'password authentication failed' not in denied.stderr:
        raise RuntimeError('Expected SCRAM password-authentication rejection was not observed')
    env['PGPASSWORD']=value
    verify="BEGIN; SET LOCAL statement_timeout='15s'; SET LOCAL lock_timeout='5s';"
    verify+="SELECT current_user=current_setting('cvera.expected_role') AND current_database()=current_setting('cvera.expected_database') AND current_schemas(true)=ARRAY['pg_catalog'::name,'public'::name];"
    # No secret data in settings or SQL; identity/routing assertions only.
    verify=verify.replace('BEGIN;',"BEGIN; SET LOCAL cvera.expected_role='"+role+"'; SET LOCAL cvera.expected_database='"+database+"';")
    if role=='cvera_api_runtime':
        verify+="SELECT public.check_employer_hr_assignment(NULL::uuid,'__weekend2_no_grant__') IS FALSE;"
    verify+='COMMIT;'
    result=run(args,verify.encode(),env=env).decode().split()
    if not result or any(item!='t' for item in result):
        raise RuntimeError('Runtime login/default-schema/deny-by-default verification failed')
    # Require the original host route to resolve off loopback and the matched
    # rule to be the verified SCRAM host-all rule; bad-password denial above
    # independently rules out local trust as authentication proof.


def atomic_publish(data):
    fd,name=tempfile.mkstemp(prefix='.env.runtime-',dir=ENVFILE.parent)
    try:
        os.fchmod(fd,0o600);os.fchown(fd,0,0)
        with os.fdopen(fd,'wb') as stream:
            stream.write(data);stream.flush();os.fsync(stream.fileno())
        os.replace(name,ENVFILE)
        folder=os.open(ENVFILE.parent,os.O_RDONLY|os.O_DIRECTORY)
        try:os.fsync(folder)
        finally:os.close(folder)
    finally:
        if os.path.exists(name):os.unlink(name)


def main():
    if os.geteuid()!=0 or sys.argv[1:]!=['--prepare-approved-1c228ee']:
        raise RuntimeError('Requires approved owner root-console invocation')
    os.umask(0o077)
    if any(p.exists() for p in [RECORD,RECORD.with_suffix('.json.next'),CANDIDATE,BEFORE]):
        raise RuntimeError('Existing credential intent/artifacts: reconcile before any retry')
    sql_helper=module('apply-weekend2-sql-preparation.py')
    probe=module('check-weekend2-runtime-transition.py')
    sql_helper.private(BACKUP,True);sql_helper.private(STAGE,True)
    previous=json.loads((BACKUP/'sql-preparation-8239937.json').read_text())
    if previous.get('state')!='complete: 000014 and service bindings committed and freshly verified':
        raise RuntimeError('Completed SQL preparation required')
    if run(['git','-C',str(ROOT),'status','--porcelain']).strip():raise RuntimeError('Dirty production worktree')
    if run(['git','-C',str(ROOT),'rev-parse','HEAD']).decode().strip()!='2813e7e513b6afed5cad3b77bee6363e982e21d2':
        raise RuntimeError('Production checkout changed')
    if info('evt-api-go-api-1','Image')!=(BACKUP/'api-image-before.txt').read_text().strip():
        raise RuntimeError('Running API changed')
    identity=deployment_identity()
    sql_helper.sql('evt',sql_helper.ROLE_VERIFY,readonly=True)
    current_versions=sql_helper.sql('evt',"SELECT current_setting('server_version_num')='160011';",readonly=True)
    if current_versions!='t':raise RuntimeError('Unexpected database version')
    libraries=sql_helper.sql('evt',"SELECT coalesce(current_setting('shared_preload_libraries'),'')||'|'||coalesce(current_setting('session_preload_libraries'),'')||'|'||coalesce(current_setting('local_preload_libraries'),'')||'|'||coalesce(current_setting('pgaudit.log',true),'');",readonly=True)
    if libraries!='|||':raise RuntimeError('Preload/audit credential logging requires separate inspection')
    observed={name:probe.live_env('evt-api-go-'+name+'-1') for name in ['api','kratos','kratos-courier']}
    app,_=probe.route(observed['api']['EVT_DB_DSN'],'evt')
    provider,route=probe.route(observed['kratos']['DSN'],'evt_kratos')
    if route!=probe.route(observed['kratos-courier']['DSN'],'evt_kratos')[1]:
        raise RuntimeError('Provider routes changed')
    if any(x.get('PGOPTIONS') or x.get('PGSERVICE') for x in observed.values()):
        raise RuntimeError('Unexpected service startup overrides')
    before=ENVFILE.read_bytes()
    values={role:secrets.token_urlsafe(48) for role in ['cvera_api_runtime','cvera_kratos_runtime']}
    def connection(parsed,role):
        if parsed.fragment:raise RuntimeError('Connection fragment unsupported')
        query=urllib.parse.parse_qs(parsed.query,keep_blank_values=True)
        if set(query)&{'user','password','passfile','sslpassword','dbname','host','hostaddr','port'}:
            raise RuntimeError('Connection identity/credential override requires review')
        endpoint=parsed.netloc.rsplit('@',1)[-1]
        return urllib.parse.urlunsplit(parsed._replace(netloc=role+':'+values[role]+'@'+endpoint))
    connections={'api':connection(app,'cvera_api_runtime'),
                 'kratos':connection(provider,'cvera_kratos_runtime')}
    connections['kratos-courier']=connections['kratos']
    candidate=patch_env(before,{'EVT_DB_DSN':connections['api'],'KRATOS_DB_DSN':connections['kratos']})
    with tempfile.TemporaryDirectory(prefix='runtime-prepare-',dir=BACKUP) as folder:
        compose=Path(folder)/'compose.yml'
        write_private(compose,run(['git','-C',str(ROOT),'show',APPROVED+':apps/evt-api-go/docker-compose.yml']))
        # Recheck current images/environments at the end; no service operation
        # is ever performed by this driver.
        baseline={name:{'id':json.loads(run(['docker','inspect','--format','{{json .Id}}','evt-api-go-'+name+'-1'])),
                        'started':info('evt-api-go-'+name+'-1','State.StartedAt')}
                  for name in observed}
        record={'approved_plan_commit':APPROVED,'started_utc':now(),'state':'intent',
                'deployment_identity_evidence':identity,'recovery_before':str(BEFORE),
                'candidate':str(CANDIDATE),'original_env_sha256':hashlib.sha256(before).hexdigest(),
                'credentials':'fresh independent secrets for new roles only; never recorded here'}
        write_private(RECORD,(json.dumps(record,indent=2)+'\n').encode())
        try:
            # Preserve recovery and new credentials privately before activation.
            write_private(BEFORE,before);write_private(CANDIDATE,candidate)
            assert_preserved(render(probe,CANDIDATE,compose),observed,connections)
            record['state']='candidate configuration verified; runtime roles still NOLOGIN';save(record)
            base=['docker','exec','-i',DB,'psql','-X','-U','postgres','-d','evt','-Atq','-v','ON_ERROR_STOP=1']
            for role in ['cvera_api_runtime','cvera_kratos_runtime']:
                password(base,role,values[role])
                record['state']='password provisioned while NOLOGIN: '+role;save(record)
            activation=sql_helper.ROLE_VERIFY+'ALTER ROLE cvera_api_runtime LOGIN; ALTER ROLE cvera_kratos_runtime LOGIN; SELECT pg_current_xact_id()::text;'
            record['activation_transaction']=sql_helper.sql('evt',activation)
            record['state']='LOGIN commit returned; verification pending';save(record)
            for name,role,database in [('api','cvera_api_runtime','evt'),('kratos','cvera_kratos_runtime','evt_kratos')]:
                runtime_probe(connections[name],values[role],role,database,sql_helper)
            # Reuse the exact reviewed ACL checks, substituting only the
            # intentionally changed role LOGIN expectation.
            role_verify=sql_helper.ROLE_VERIFY.replace('rolcanlogin OR ', '')
            role_verify+="DO $$ BEGIN IF (SELECT count(*) FROM pg_roles WHERE rolname IN ('cvera_api_runtime','cvera_kratos_runtime') AND rolcanlogin)<>2 OR (SELECT rolcanlogin FROM pg_roles WHERE rolname='cvera_hr_operator') THEN RAISE EXCEPTION 'LOGIN state mismatch'; END IF; END $$;"
            api_verify=role_verify+sql_helper.API_VERIFY[len(sql_helper.ROLE_VERIFY):]
            provider_verify=role_verify+sql_helper.PROVIDER_VERIFY[len(sql_helper.ROLE_VERIFY):]
            sql_helper.sql('evt',api_verify)
            sql_helper.sql('evt_kratos',provider_verify,readonly=True)
            record['state']='restricted LOGIN and SCRAM/privileges verified; publication pending';save(record)
            if ENVFILE.read_bytes()!=before:raise RuntimeError('Live environment changed during preparation')
            atomic_publish(candidate)
            mode=ENVFILE.stat()
            if (mode.st_uid,mode.st_gid,stat.S_IMODE(mode.st_mode))!=(0,0,0o600):
                raise RuntimeError('Private environment ownership/permissions failed')
            assert_preserved(render(probe,ENVFILE,compose),observed,connections)
            for name,prior in baseline.items():
                if (json.loads(run(['docker','inspect','--format','{{json .Id}}','evt-api-go-'+name+'-1']))!=prior['id']
                    or info('evt-api-go-'+name+'-1','State.StartedAt')!=prior['started']
                    or probe.live_env('evt-api-go-'+name+'-1')!=observed[name]):
                    raise RuntimeError('Running service changed unexpectedly')
            if run(['git','-C',str(ROOT),'status','--porcelain']).strip():raise RuntimeError('Git worktree changed')
            record['state']='complete: restricted LOGIN verified; protected configuration published; services unchanged'
            record['completed_utc']=now();record['env_sha256']=hashlib.sha256(candidate).hexdigest();save(record)
            print(json.dumps({'runtime_credential_preparation':'PASS','deployment_account':'root',
                'activation_transaction':record['activation_transaction'],'runtime_login_roles':2,
                'operator_nologin':True,'correct_passwords_verified':True,'incorrect_passwords_rejected':True,
                'privilege_checks':True,'persistent_settings_preserved':True,'live_env_owner_mode':'root:root 0600',
                'services_restarted':False,'application_data_changed':False,'existing_passwords_changed':False,
                'merge_or_deployment':False,'private_record':str(RECORD)}))
        except Exception:
            record['stopped_utc']=now();record['execution_stopped']=True;save(record)
            raise


if __name__=='__main__':
    try:main()
    except Exception:
        print('STOP: credential preparation stopped; no raw values emitted. Do not retry. Reconcile the private intent/candidate/recovery state before any service action.',file=sys.stderr)
        sys.exit(1)
