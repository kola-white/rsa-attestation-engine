#!/usr/bin/env python3
"""Local-only Kratos v1.1.0/non-owner permission rehearsal.

Requires the existing cvera-authority-test-pg fixture and cached oryd/kratos:v1.1.0
image. Runs vendor migrations only against a fresh disposable local database,
never accepts a production DSN, and never uses registered production identities.
No courier worker is started. Temporary role/database/container are removed.
"""
import subprocess, pathlib, uuid, os, urllib.parse, json, time, tempfile, re
api = pathlib.Path(__file__).resolve().parents[1]
root = api.parents[1]
suffix = uuid.uuid4().hex
name = 'cvera_kratos_probe_' + suffix
role = 'kratos_runtime_probe_' + suffix
container = 'cvera-kratos-runtime-probe-' + suffix
base = ['docker', 'exec', '-i', 'cvera-authority-test-pg']

def sql(query, db='cvera_authority_test', check=True):
    r = subprocess.run(base + ['psql', '-X', '-U', 'cvera_test', '-d', db, '-v', 'ON_ERROR_STOP=1', '-At'], input=query, text=True, capture_output=True)
    if check and r.returncode:
        raise RuntimeError('Local probe SQL failed: ' + r.stderr)
    return r
assert sql('SELECT current_database(),current_user').stdout.strip() == 'cvera_authority_test|cvera_test'
subprocess.run(['docker', 'image', 'inspect', 'oryd/kratos:v1.1.0'], capture_output=True, check=True)
owner_pass = subprocess.run(base + ['printenv', 'POSTGRES_PASSWORD'], capture_output=True, text=True, check=True).stdout.strip()
pwd = 'synthetic_' + uuid.uuid4().hex
owner_dsn = 'postgres://cvera_test:' + urllib.parse.quote(owner_pass, safe='') + '@127.0.0.1:5432/' + name + '?sslmode=disable'
runtime_dsn = 'postgres://' + role + ':' + pwd + '@127.0.0.1:5432/' + name + '?sslmode=disable'
env = os.environ.copy()

def docker(args, dsn):
    env['DSN'] = dsn
    return subprocess.run(['docker', 'run', '--rm', '--network', 'container:cvera-authority-test-pg', '-e', 'DSN', *args], env=env, text=True, capture_output=True)

def redact(s):
    for secret in [owner_dsn, runtime_dsn, owner_pass, urllib.parse.quote(owner_pass, safe=''), pwd]:
        s = s.replace(secret, '[REDACTED]')
    return re.sub('postgres(?:ql)?://[^\\s"\\x1b]+', '[REDACTED_TEST_DSN]', s)
created = False
role_created = False
started = False
try:
    sql('CREATE DATABASE ' + name)
    created = True
    r = docker(['oryd/kratos:v1.1.0', 'migrate', 'sql', '--read-from-env', '--yes'], owner_dsn)
    if r.returncode:
        raise RuntimeError('Vendor local migration failed: ' + redact(r.stdout + r.stderr)[-5000:])
    print('Vendor local schema:', sql("SELECT string_agg(relname,', ' ORDER BY relname) FROM pg_class WHERE relnamespace='public'::regnamespace AND relkind='r'", name).stdout.strip())
    sql(f"CREATE ROLE {role} LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOBYPASSRLS PASSWORD '{pwd}'")
    role_created = True
    profile = (api / 'operations/kratos-runtime-privileges.sql').read_text()
    sql(f"BEGIN; SET LOCAL search_path=public,pg_temp; SET LOCAL cvera.service_runtime_role='{role}';" + profile + 'COMMIT;', name)
    denied = sql(f'SET ROLE {role}; DELETE FROM schema_migration WHERE false;', name, check=False)
    if denied.returncode == 0 or 'permission denied' not in denied.stderr:
        raise RuntimeError('Vendor ledger write was not denied')
    with tempfile.TemporaryDirectory(prefix='cvera-kratos-role-') as temp:
        config = pathlib.Path(temp) / 'kratos.yml'
        config.write_text((root / 'infra/kratos/kratos.yml').read_text().replace('https://auth.cvera.app', 'http://127.0.0.1:4434'))
        config.chmod(420)
        env.update({'DSN': runtime_dsn, 'KRATOS_SECRET_DEFAULT': 'synthetic-default-secret-for-local-rehearsal', 'KRATOS_SECRET_COOKIE': 'synthetic-cookie-secret-for-local-rehearsal', 'COURIER_SMTP_CONNECTION_URI': 'smtp://synthetic:synthetic@invalid.example:2525/'})
        args = ['docker', 'run', '-d', '--name', container, '--network', 'container:cvera-authority-test-pg', '-e', 'DSN', '-e', 'KRATOS_SECRET_DEFAULT', '-e', 'KRATOS_SECRET_COOKIE', '-e', 'COURIER_SMTP_CONNECTION_URI', '-v', str(config) + ':/etc/config/kratos/kratos.yml:ro', '-v', str(root / 'infra/kratos/identity.schema.json') + ':/etc/config/kratos/identity.schema.json:ro', '-v', str(api / 'email/dist') + ':/etc/config/kratos/courier-templates:ro', 'oryd/kratos:v1.1.0', 'serve', '--config', '/etc/config/kratos/kratos.yml', '--sqa-opt-out']
        r = subprocess.run(args, env=env, capture_output=True, text=True)
        if r.returncode:
            raise RuntimeError('Local provider start failed: ' + redact(r.stderr))
        started = True
        ready = False
        for _ in range(30):
            r = subprocess.run(base + ['bash', '-c', "exec 3<>/dev/tcp/127.0.0.1/4435; printf 'GET /admin/health/ready HTTP/1.0\r\nHost: localhost\r\n\r\n' >&3; head -n 1 <&3"], capture_output=True, text=True)
            if '200 OK' in r.stdout:
                ready = True
                break
            running = subprocess.run(['docker', 'inspect', '--format', '{{.State.Running}}', container], capture_output=True, text=True).stdout.strip()
            if running != 'true':
                break
            time.sleep(1)
        if not ready:
            logs = subprocess.run(['docker', 'logs', container], capture_output=True, text=True)
            raise RuntimeError('Vendor readiness failed: ' + redact(logs.stdout + logs.stderr)[-7000:])
        print(json.dumps({'vendor_version': 'v1.1.0', 'fresh_vendor_migrations_local_only': True, 'dedicated_nonowner_role': True, 'read_only_vendor_ledger': True, 'health_ready': True, 'production_data_or_credentials_used': False}))
finally:
    if started:
        subprocess.run(['docker', 'rm', '-f', container], capture_output=True)
    if created:
        sql('DROP DATABASE ' + name + ' WITH (FORCE)')
    if role_created:
        sql('DROP ROLE ' + role)
