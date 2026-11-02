#!/usr/bin/env python3
"""Owner-approved SQL-only preparation; no credentials, data DML or deployment.

Fixed Droplet/DB/review paths. An exclusive private intent record prevents blind
retries after any failure or ambiguous COMMIT. No production access from CI.
"""
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import stat
import subprocess
import sys

REVIEW = '8239937f789916cf665de66da4275a5af8276d08'
ROOT = Path('/opt/cvera/rsa-attestation-engine')
BACKUP = Path('/opt/cvera/weekend2-closeout-20261102T004322Z')
STAGE = BACKUP / 'reviewed-afa946d'
DB = 'evt-api-go-db-1'
CLONE = 'cvera-weekend2-restore-20261102T004322Z'
RECORD = BACKUP / 'sql-preparation-8239937.json'
FILES = {
    '000014_employer_hr_assignments.sql': ('migrations', '666cdeba3b37a6ac18909cceb0f9d21332c2a2f40dcedaba72b3afbca0adfefb'),
    'api-runtime-privileges.sql': ('operations', 'cb479c4a4a551982a92f16632795d75ae38884d1d83a97ba63827d997067ef7d'),
    'hr-assignment-privileges.sql': ('operations', 'c9d5efd8220b63a197857b63119fabc3c30cffa4d4edbaeaba8f6578703fe2a2'),
    'kratos-runtime-privileges.sql': ('operations', 'c8b6f7715a5d82229de984a66e51e29e85fffd2a2b885bf0ec0978085687160a'),
}
ROLES = "('cvera_api_runtime','cvera_kratos_runtime','cvera_hr_operator')"
SETTINGS = """SET LOCAL search_path=public,pg_temp;
SET LOCAL lock_timeout='5s'; SET LOCAL statement_timeout='60s';
SET LOCAL idle_in_transaction_session_timeout='60s';
"""
ROLE_VERIFY = """
DO $$ BEGIN
 IF (SELECT count(*) FROM pg_catalog.pg_roles WHERE rolname IN
 ('cvera_api_runtime','cvera_kratos_runtime','cvera_hr_operator'))<>3
 OR EXISTS(SELECT 1 FROM pg_catalog.pg_roles WHERE rolname IN
 ('cvera_api_runtime','cvera_kratos_runtime','cvera_hr_operator') AND
 (rolcanlogin OR rolsuper OR rolcreatedb OR rolcreaterole OR rolreplication OR rolbypassrls OR rolinherit))
 OR EXISTS(SELECT 1 FROM pg_catalog.pg_auth_members m JOIN pg_catalog.pg_roles r
 ON r.oid=m.member OR r.oid=m.roleid WHERE r.rolname IN
 ('cvera_api_runtime','cvera_kratos_runtime','cvera_hr_operator'))
 THEN RAISE EXCEPTION 'restricted NOLOGIN role verification failed'; END IF;
END $$;
"""
API_VERIFY = ROLE_VERIFY + """
DO $$ DECLARE r text; t oid:='public.employer_hr_assignments'::regclass; BEGIN
 IF (SELECT relowner FROM pg_catalog.pg_class WHERE oid=t) <> 'postgres'::regrole
 OR (SELECT relrowsecurity OR relforcerowsecurity FROM pg_catalog.pg_class WHERE oid=t)
 OR EXISTS(SELECT 1 FROM public.employer_hr_assignments)
 THEN RAISE EXCEPTION 'assignment owner/RLS/empty-state verification failed'; END IF;
 FOREACH r IN ARRAY ARRAY['cvera_api_runtime','evt-app'] LOOP
  IF pg_catalog.has_any_column_privilege(r,t,'SELECT,INSERT,UPDATE')
  OR pg_catalog.has_table_privilege(r,t,'DELETE,TRUNCATE,REFERENCES,TRIGGER')
  THEN RAISE EXCEPTION 'direct assignment access exceeds boundary'; END IF;
 END LOOP;
 IF NOT pg_catalog.has_function_privilege('cvera_api_runtime',
 'public.check_employer_hr_assignment(uuid,text)','EXECUTE')
 OR pg_catalog.has_function_privilege('evt-app',
 'public.check_employer_hr_assignment(uuid,text)','EXECUTE')
 OR pg_catalog.has_function_privilege('cvera_api_runtime',
 'public.lock_employer_hr_principal(uuid)','EXECUTE')
 OR pg_catalog.has_function_privilege('cvera_api_runtime',
 'public.enforce_employer_hr_assignment()','EXECUTE')
 THEN RAISE EXCEPTION 'runtime execution boundary failed'; END IF;
 IF EXISTS(SELECT 1 FROM pg_catalog.pg_proc p WHERE p.oid IN
 ('public.check_employer_hr_assignment(uuid,text)'::regprocedure,
  'public.lock_employer_hr_principal(uuid)'::regprocedure,
  'public.enforce_employer_hr_assignment()'::regprocedure)
 AND (p.proowner<>'postgres'::regrole OR NOT coalesce(p.proconfig @>
 ARRAY['search_path=pg_catalog, public, pg_temp'],false)
 OR p.prosecdef<>(p.proname<>'enforce_employer_hr_assignment')))
 THEN RAISE EXCEPTION 'function ownership/fixed path/security mode failed'; END IF;
 IF EXISTS(SELECT 1 FROM pg_catalog.pg_proc p,
 LATERAL pg_catalog.aclexplode(coalesce(p.proacl,pg_catalog.acldefault('f',p.proowner))) a
 WHERE p.oid IN ('public.check_employer_hr_assignment(uuid,text)'::regprocedure,
 'public.lock_employer_hr_principal(uuid)'::regprocedure,
 'public.enforce_employer_hr_assignment()'::regprocedure)
 AND a.grantee=0 AND a.privilege_type='EXECUTE')
 THEN RAISE EXCEPTION 'PUBLIC execution forbidden'; END IF;
END $$;
SET LOCAL ROLE cvera_api_runtime;
DO $$ BEGIN
 IF public.check_employer_hr_assignment(NULL::uuid,'__weekend2_no_grant__') IS DISTINCT FROM false
 THEN RAISE EXCEPTION 'runtime deny-by-default check failed'; END IF;
END $$;
RESET ROLE;
"""
PROVIDER_VERIFY = ROLE_VERIFY + """
DO $$ BEGIN
 IF (SELECT count(*) FROM pg_catalog.pg_class WHERE relnamespace='public'::regnamespace
 AND relkind IN ('r','p'))<>26
 OR (SELECT count(*) FROM pg_catalog.pg_class c WHERE c.relnamespace='public'::regnamespace
 AND c.relkind IN ('r','p')
 AND pg_catalog.has_table_privilege('cvera_kratos_runtime',c.oid,'SELECT'))<>26
 OR (SELECT count(*) FROM pg_catalog.pg_class c WHERE c.relnamespace='public'::regnamespace
 AND c.relkind IN ('r','p') AND c.relname<>'schema_migration'
 AND pg_catalog.has_table_privilege('cvera_kratos_runtime',c.oid,'INSERT')
 AND pg_catalog.has_table_privilege('cvera_kratos_runtime',c.oid,'UPDATE')
 AND pg_catalog.has_table_privilege('cvera_kratos_runtime',c.oid,'DELETE'))<>25
 OR pg_catalog.has_any_column_privilege('cvera_kratos_runtime','public.schema_migration','INSERT,UPDATE')
 OR pg_catalog.has_table_privilege('cvera_kratos_runtime','public.schema_migration','DELETE,TRUNCATE')
 OR EXISTS(SELECT 1 FROM pg_catalog.pg_class c WHERE c.relnamespace='public'::regnamespace
 AND c.relkind IN ('r','p') AND
 (pg_catalog.has_any_column_privilege('cvera_api_runtime',c.oid,'SELECT,INSERT,UPDATE')
 OR pg_catalog.has_table_privilege('cvera_api_runtime',c.oid,'DELETE,TRUNCATE')))
 THEN RAISE EXCEPTION 'provider privileges/cross-service isolation failed'; END IF;
END $$;
"""


def run(args, data=None):
    result = subprocess.run(args, input=data, capture_output=True, timeout=180)
    if result.returncode:
        if RECORD.exists():
            fd=os.open(BACKUP/'sql-preparation-8239937-error.txt',os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
            with os.fdopen(fd,'wb') as stream: stream.write(result.stderr)
        raise RuntimeError('Command failed; stopped without emitting command output')
    return result.stdout


def info(name, field):
    return json.loads(run(['docker','inspect','--format','{{json .'+field+'}}',name]))


def sql(database, body, readonly=False):
    begin = 'BEGIN READ ONLY;' if readonly else 'BEGIN ISOLATION LEVEL READ COMMITTED;'
    return run(['docker','exec','-i',DB,'psql','-X','-U','postgres','-d',database,
                '-Atq','-v','ON_ERROR_STOP=1'],
               (begin + SETTINGS + body + '\nCOMMIT;\n').encode()).decode().strip()


def private(path, directory=False):
    mode = path.lstat()
    if (mode.st_uid != 0 or
            not (stat.S_ISDIR(mode.st_mode) if directory else stat.S_ISREG(mode.st_mode))):
        raise RuntimeError('Root-owned private recovery path required')
    expected = 0o700 if directory else 0o600
    if stat.S_IMODE(mode.st_mode) != expected:
        raise RuntimeError('Private recovery permissions do not match verified backup')


def now():
    return datetime.now(timezone.utc).isoformat()


def save(record):
    temporary = RECORD.with_suffix('.json.next')
    fd = os.open(temporary, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(fd,'w') as stream:
        json.dump(record,stream,indent=2)
        stream.write('\n')
        stream.flush()
        os.fsync(stream.fileno())
    os.replace(temporary,RECORD)


def main():
    if sys.argv[1:] != ['--apply-approved-8239937'] or os.geteuid()!=0:
        raise RuntimeError('Requires owner root console and explicit approved-phase argument')
    os.umask(0o077)
    private(BACKUP,True)
    private(STAGE,True)
    if RECORD.exists() or RECORD.with_suffix('.json.next').exists():
        raise RuntimeError('Existing change record: reconcile before any retry')
    if run(['git','-C',str(ROOT),'status','--porcelain']).strip():
        raise RuntimeError('Production worktree must remain clean')
    if run(['git','-C',str(ROOT),'rev-parse','HEAD']).decode().strip() != '2813e7e513b6afed5cad3b77bee6363e982e21d2':
        raise RuntimeError('Unexpected production checkout')
    contents = {}
    for name,(folder,digest) in FILES.items():
        private(STAGE/name)
        staged = (STAGE/name).read_bytes()
        reviewed = run(['git','-C',str(ROOT),'show',REVIEW+':apps/evt-api-go/'+folder+'/'+name])
        if staged != reviewed or hashlib.sha256(staged).hexdigest()!=digest:
            raise RuntimeError('Staged SQL differs from approved commit/digest')
        contents[name] = staged.decode()
    checksum_stat=(BACKUP/'backup.sha256').lstat()
    if not stat.S_ISREG(checksum_stat.st_mode) or checksum_stat.st_uid!=0 or checksum_stat.st_mode & 0o022:
        raise RuntimeError('Checksum manifest must be a root-owned non-writable metadata file')
    expected_files = {BACKUP/name for name in ['evt.dump','evt_kratos.dump','roles-without-passwords.sql']}
    observed = set()
    for line in (BACKUP/'backup.sha256').read_text().splitlines():
        digest, filename = line.split(None,1)
        path = Path(filename.lstrip('*'))
        if not path.is_absolute(): path=BACKUP/path
        if path not in expected_files or path in observed:
            raise RuntimeError('Unexpected backup checksum entry')
        private(path)
        with path.open('rb') as stream:
            hasher=hashlib.sha256()
            for chunk in iter(lambda: stream.read(1024*1024),b''): hasher.update(chunk)
            actual=hasher.hexdigest()
        if actual!=digest: raise RuntimeError('Backup checksum mismatch')
        observed.add(path)
    if observed!=expected_files: raise RuntimeError('Incomplete backup checksums')
    private(BACKUP/'api.env.before')
    recovery=(BACKUP/'api-recovery-tag.txt').read_text().strip()
    image=(BACKUP/'api-image-before.txt').read_text().strip()
    if json.loads(run(['docker','image','inspect','--format','{{json .Id}}',recovery]))!=image:
        raise RuntimeError('Retained recovery image mismatch')
    if info('evt-api-go-api-1','Image')!=image:
        raise RuntimeError('Running API changed since recovery capture')
    postgres_reference=(BACKUP/'postgres-image-before.txt').read_text().strip()
    postgres_image=json.loads(run(['docker','image','inspect','--format','{{json .Id}}',postgres_reference]))
    if info(DB,'Image')!=postgres_image or info(CLONE,'Image')!=postgres_image:
        raise RuntimeError('Database/recovery image changed since restore evidence')
    if info(CLONE,'HostConfig.NetworkMode')!='none' or not info(CLONE,'State.Running'):
        raise RuntimeError('Verified recovery clone unavailable')
    if info(CLONE,'Config.Labels').get('cvera.scope')!='weekend2-rehearsal':
        raise RuntimeError('Unexpected recovery clone identity')
    clone_mounts={x['Source'] for x in info(CLONE,'Mounts') if x.get('Source')}
    if clone_mounts & {x['Source'] for x in info(DB,'Mounts') if x.get('Source')}:
        raise RuntimeError('Recovery storage isolation failed')
    preflight = """DO $$ BEGIN
 IF current_database()<>'evt' OR current_user<>'postgres' OR
 current_setting('server_version_num')::integer<>160011
 THEN RAISE EXCEPTION 'unexpected production database/executor/version'; END IF;
 IF EXISTS(SELECT 1 FROM pg_catalog.pg_roles WHERE rolname IN """+ROLES+""")
 OR to_regclass('public.employer_hr_assignments') IS NOT NULL
 OR to_regprocedure('public.check_employer_hr_assignment(uuid,text)') IS NOT NULL
 OR to_regprocedure('public.lock_employer_hr_principal(uuid)') IS NOT NULL
 OR to_regprocedure('public.enforce_employer_hr_assignment()') IS NOT NULL
 THEN RAISE EXCEPTION 'unexpected existing roles/objects; do not retry blindly'; END IF;
END $$;"""
    sql('evt',preflight,readonly=True)
    procedure=run(['git','-C',str(ROOT),'show',REVIEW+':docs/operations/employer-assignment-migration-procedure.md'])
    recovery_path=BACKUP/'sql-preparation-recovery-procedure-8239937.md'
    if recovery_path.exists():
        private(recovery_path)
        if recovery_path.read_bytes()!=procedure: raise RuntimeError('Recovery procedure mismatch')
    else:
        fd=os.open(recovery_path,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
        with os.fdopen(fd,'wb') as stream: stream.write(procedure)
    record={'approved_review_commit':REVIEW,'operator':'owner root console / postgres',
            'started_utc':now(),'state':'intent','migration':'000014 only',
            'prior_migration_history':'unknown; no ledger created',
            'sql_sha256':{n:v[1] for n,v in FILES.items()},
            'recovery_directory':str(BACKUP),
            'recovery':'Rollback uncommitted transaction; keep committed additive NOLOGIN preparation and hold rollout. No destructive restore authorized.'}
    fd=os.open(RECORD,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
    with os.fdopen(fd,'w') as stream:
        json.dump(record,stream,indent=2);stream.flush();os.fsync(stream.fileno())
    try:
        roles=''.join('CREATE ROLE '+r+' NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS;'
                      for r in ['cvera_api_runtime','cvera_kratos_runtime','cvera_hr_operator'])
        body="SELECT pg_advisory_xact_lock(hashtext('cvera:employer-assignments:000014'));"+preflight+roles
        body+="SET LOCAL cvera.service_runtime_role='cvera_api_runtime'; SET LOCAL cvera.hr_runtime_role='cvera_api_runtime'; SET LOCAL cvera.hr_operator_role='cvera_hr_operator';"
        body+=contents['api-runtime-privileges.sql']+contents['000014_employer_hr_assignments.sql']+contents['hr-assignment-privileges.sql']+API_VERIFY
        body+="SELECT 'transaction_id='||pg_current_xact_id()::text;"
        record['api_transaction']=sql('evt',body)
        record['state']='api_commit_returned; fresh verification pending';save(record)
        # The bounded check takes a row lock, so use READ COMMITTED, not READ ONLY.
        # Its NULL principal cannot match or mutate an existing application row.
        sql('evt',API_VERIFY)
        record['state']='000014 committed and freshly verified; provider binding pending';save(record)
        body="SET LOCAL cvera.service_runtime_role='cvera_kratos_runtime';"
        body+=contents['kratos-runtime-privileges.sql']+PROVIDER_VERIFY
        body+="SELECT 'transaction_id='||pg_current_xact_id()::text;"
        record['provider_transaction']=sql('evt_kratos',body)
        record['state']='provider commit returned; fresh verification pending';save(record)
        sql('evt_kratos',PROVIDER_VERIFY,readonly=True)
        record['state']='complete: 000014 and service bindings committed and freshly verified'
        record['completed_utc']=now();save(record)
        print(json.dumps({'sql_preparation':'PASS','migration':'000014 only',
              'api_transaction':record['api_transaction'],'provider_transaction':record['provider_transaction'],
              'roles':'three restricted NOLOGIN roles','assignments_created':0,
              'credentials_data_configuration_services_changed':False,'private_change_record':str(RECORD)}))
    except Exception:
        record['failure_utc']=now()
        record['execution_stopped']=True
        save(record)
        raise


if __name__=='__main__':
    try: main()
    except Exception as error:
        print('STOP: '+str(error)+'. Do not retry or continue rollout; inspect the private change record and reconcile catalog state.',file=sys.stderr)
        sys.exit(1)
