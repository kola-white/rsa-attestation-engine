#!/usr/bin/env python3
"""Disposable local Docker verification of the exact migration release procedure."""
import importlib.util,json,os,subprocess,tempfile,uuid
from pathlib import Path
ROOT=Path(__file__).resolve().parents[3]
spec=importlib.util.spec_from_file_location('release',ROOT/'apps/evt-api-go/operations/release-evidence-completion-sql.py');m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
name='cvera-completion-release-test-'+uuid.uuid4().hex[:10]
m.DB=name
m.run(['docker','run','-d','--pull','never','--name',name,'--network','none','--memory','96m','--memory-swap','96m','--shm-size','16m','-e','POSTGRES_HOST_AUTH_METHOD=trust','postgres:16','-c','shared_buffers=8MB','-c','max_connections=10','-c','work_mem=512kB','-c','maintenance_work_mem=4MB','-c','autovacuum=off','-c','max_worker_processes=0','-c','max_parallel_workers=0'])
try:
 import time
 for _ in range(60):
  r=subprocess.run(['docker','exec',name,'psql','-X','-U','postgres','-d','postgres','-Atqc','SHOW listen_addresses'],capture_output=True)
  if r.returncode==0 and r.stdout.strip()==b'*':break
  time.sleep(1)
 else:raise RuntimeError('not ready')
 m.run(['docker','exec','-i',name,'psql','-X','-U','postgres','-d','postgres','-v','ON_ERROR_STOP=1'],b'CREATE DATABASE evt; CREATE ROLE cvera_api_runtime LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS; CREATE ROLE cvera_evidence_operator NOLOGIN; CREATE ROLE "evt-app" NOLOGIN;')
 def source(p):return (ROOT/'apps/evt-api-go'/p).read_text()
 m.sql('''CREATE TABLE public.domain_users(id uuid PRIMARY KEY DEFAULT gen_random_uuid(),kratos_identity_id text NOT NULL UNIQUE,email text,name text,role text NOT NULL DEFAULT 'hr_reviewer',status text NOT NULL DEFAULT 'active',created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now());
 ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT,INSERT,UPDATE,DELETE ON TABLES TO "evt-app";'''+source('migrations/000015_case_evidence_authorization.sql')+'''
 BEGIN;SET LOCAL cvera.evidence_runtime_role='cvera_api_runtime';SET LOCAL cvera.evidence_operator_role='cvera_evidence_operator';'''+source('operations/evidence-authorization-privileges.sql')+'COMMIT;')
 sources={p:source(p) for p in m.FILES}
 m.sql('BEGIN READ ONLY;'+m.SETTINGS+m.PREFLIGHT+'ROLLBACK;')
 dump=m.run(['docker','exec',name,'pg_dump','-U','postgres','-d','evt','-Fc'])
 with tempfile.TemporaryDirectory(prefix='cvera-completion-rehearsal-') as folder:
  # production backup path is timestamped; clone naming derives only last segment.
  backup=Path(folder)/('test-'+uuid.uuid4().hex[:12]);backup.mkdir()
  m.restore_rehearsal(backup,dump,sources)
  record=json.loads((backup/'restore-rehearsal.json').read_text())
  print(json.dumps({'backup_restore':record['restore'],'rollback':record['migration_rollback'],'migration_commit':record['migration_commit'],'runtime_privileges':record['privileges'],'hostile_default_acl_stripped':True,'restore_memory_limit_mib':record['memory_limit_mib'],'restore_peak_bytes':record['memory_peak_bytes']}))
  m.run(['docker','rm','-v',record['clone']])
 # Test the live sequence and repeat prevention on a separate isolated database.
 m.sql(m.transaction(sources,'ROLLBACK'))
 m.sql('BEGIN READ ONLY;'+m.SETTINGS+m.PREFLIGHT+'ROLLBACK;')
 before=m.sql('BEGIN READ ONLY;'+m.FINGERPRINT+'ROLLBACK;')
 m.sql(m.transaction(sources,'COMMIT'))
 m.sql('BEGIN;'+m.SETTINGS+m.VERIFY+'ROLLBACK;')
 assert m.sql('BEGIN READ ONLY;'+m.FINGERPRINT+'ROLLBACK;')==before
 try:m.sql('BEGIN READ ONLY;'+m.SETTINGS+m.PREFLIGHT+'ROLLBACK;')
 except RuntimeError:pass
 else:raise RuntimeError('repeat accepted')
 print(json.dumps({'production_sequence_local':'PASS','repeat_prevented':True,'registry_roles_unchanged':True}))
finally:
 m.run(['docker','rm','-f','-v',name])
