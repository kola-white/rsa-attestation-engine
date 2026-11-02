#!/usr/bin/env python3
"""Read-only runtime-transition compatibility check; metadata/booleans only.

No DB queries, credential changes, app/config writes, builds or service actions.
Compose preview uses synthetic replacement DSNs and a private temporary file.
Existing credential values are compared only in host memory, never printed.
"""
import json
import os
from pathlib import Path
import pwd
import re
import stat
import subprocess
import sys
import tempfile
import urllib.parse

ROOT=Path('/opt/cvera/rsa-attestation-engine')
BACKUP=Path('/opt/cvera/weekend2-closeout-20261102T004322Z')
REVIEW='8239937f789916cf665de66da4275a5af8276d08'


def run(args,env=None):
    result=subprocess.run(args,env=env,capture_output=True,timeout=60)
    if result.returncode: raise RuntimeError('Metadata/configuration check failed; raw output suppressed')
    return result.stdout


def live_env(container):
    entries=json.loads(run(['docker','inspect','--format','{{json .Config.Env}}',container]))
    return dict(x.split('=',1) for x in entries if '=' in x)


def route(value, database):
    parsed=urllib.parse.urlsplit(value)
    if (parsed.scheme not in ('postgres','postgresql') or not parsed.hostname
            or urllib.parse.unquote(parsed.path.strip('/'))!=database):
        raise RuntimeError('Unexpected connection format/database; do not change connections')
    query=urllib.parse.parse_qs(parsed.query,keep_blank_values=True)
    if query.get('options') or query.get('service') or query.get('search_path'):
        raise RuntimeError('Connection overrides require review')
    return parsed,(parsed.hostname,parsed.port or 5432,parsed.path,query)


def replacement(parsed,role):
    endpoint=parsed.netloc.rsplit('@',1)[-1]
    return urllib.parse.urlunsplit(parsed._replace(netloc=role+':synthetic-rehearsal-only@'+endpoint))


def main():
    if os.geteuid()!=0 or sys.argv[1:]: raise RuntimeError('Run without arguments in the owner root console')
    os.umask(0o077)
    record=json.loads((BACKUP/'sql-preparation-8239937.json').read_text())
    if record.get('state')!='complete: 000014 and service bindings committed and freshly verified':
        raise RuntimeError('Verified production SQL preparation record required')
    api=live_env('evt-api-go-api-1')
    kratos=live_env('evt-api-go-kratos-1')
    courier=live_env('evt-api-go-kratos-courier-1')
    app_uri,_=route(api.get('EVT_DB_DSN',''),'evt')
    provider_uri,provider_route=route(kratos.get('DSN',''),'evt_kratos')
    _,courier_route=route(courier.get('DSN',''),'evt_kratos')
    if provider_route!=courier_route: raise RuntimeError('Provider routes differ; coordinated switch requires review')
    if any(x.get('PGOPTIONS') or x.get('PGSERVICE') for x in (api,kratos,courier)):
        raise RuntimeError('Live startup/service overrides require review')
    env=os.environ.copy()
    env['EVT_DB_DSN']=replacement(app_uri,'cvera_api_runtime')
    env['KRATOS_DB_DSN']=replacement(provider_uri,'cvera_kratos_runtime')
    source=run(['git','-C',str(ROOT),'show',REVIEW+':apps/evt-api-go/docker-compose.yml'])
    with tempfile.TemporaryDirectory(prefix='runtime-preview-',dir=BACKUP) as directory:
        preview=Path(directory)/'compose.yml'
        preview.write_bytes(source)
        preview.chmod(0o600)
        rendered=json.loads(run(['docker','compose','--project-name','evt-api-go',
            '--project-directory',str(ROOT/'apps/evt-api-go'),
            '--env-file',str(ROOT/'apps/evt-api-go/.env'),'-f',str(preview),
            'config','--format','json'],env=env))
    reports={}
    for name,observed,connection in [('api',api,'EVT_DB_DSN'),('kratos',kratos,'DSN'),('kratos-courier',courier,'DSN')]:
        desired=rendered['services'][name]['environment']
        changed=sorted(key for key,value in desired.items()
                       if key!=connection and observed.get(key)!=value)
        reports[name]={'preserved_environment_matches':not changed,
                       'unexpected_changed_names':changed}
    required={'api':['EVT_JWT_SECRET','EVT_REFRESH_TOKEN_HMAC_KEY'],
              'kratos':['KRATOS_SECRET_DEFAULT','KRATOS_SECRET_COOKIE'],
              'kratos-courier':['KRATOS_SECRET_DEFAULT','KRATOS_SECRET_COOKIE']}
    for name,keys in required.items():
        if any(not rendered['services'][name]['environment'].get(key) for key in keys):
            raise RuntimeError('Required persistent secret missing; no transition is safe')
    info=(ROOT/'apps/evt-api-go/.env').stat()
    try: owner=pwd.getpwuid(info.st_uid).pw_name
    except KeyError: owner=None
    # Only successful SSH account names/timestamps; no keys, addresses or raw logs.
    auth=[]
    try:
        result=subprocess.run(['journalctl','-u','ssh.service','-u','sshd.service',
                               '-n','1500','--no-pager','-o','json'],capture_output=True,timeout=15)
        if result.returncode==0:
            for line in result.stdout.splitlines():
                entry=json.loads(line)
                match=re.search(r'Accepted publickey for ([A-Za-z0-9_.-]+) ',entry.get('MESSAGE',''))
                if match: auth.append({'account':match.group(1),'timestamp_us':entry.get('__REALTIME_TIMESTAMP')})
    except (OSError,ValueError,subprocess.TimeoutExpired): pass
    print(json.dumps({'read_only':True,'sql_preparation_verified':True,'synthetic_compose_preview':True,
        'services':reports,'provider_routes_match':True,
        'unused_api_database_url_removed':'DATABASE_URL' not in rendered['services']['api']['environment'],
        'env_file_metadata':{'uid':info.st_uid,'gid':info.st_gid,'owner_account':owner,
                             'mode':oct(stat.S_IMODE(info.st_mode))},
        'recent_successful_ssh_accounts':auth[-5:],
        'credentials_data_configuration_services_changed':False},indent=2))


if __name__=='__main__':
    try:main()
    except Exception:
        print('STOP: runtime compatibility metadata check failed; no raw values emitted and no production state changed.',file=sys.stderr)
        sys.exit(1)
