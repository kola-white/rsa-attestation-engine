#!/usr/bin/env python3
"""Rehearse Proposal A recovery using synthetic data in the fixed test container.

Never accepts a production host/DSN. Uses only Python stdlib and PostgreSQL 16
clients already present in cvera-authority-test-pg. Cleans up the mode-0600 dump
and all disposable schemas, roles and databases afterward.
"""

import hashlib
import json
import os
from pathlib import Path
import subprocess
import tempfile
import uuid

suffix = uuid.uuid4().hex
schema = "hr_recovery_" + suffix
runtime = "hr_restore_runtime_" + suffix
operator = "hr_restore_operator_" + suffix
scratch = "cvera_recovery_" + suffix
api_dir = Path(__file__).resolve().parents[1]
base = ["docker", "exec", "-i", "cvera-authority-test-pg"]
path = Path(tempfile.gettempdir()) / ("cvera-employer-recovery-" + suffix + ".dump")


def sql(query, database="cvera_authority_test"):
    result = subprocess.run(
        base + ["psql", "-X", "-U", "cvera_test", "-d", database,
                "-v", "ON_ERROR_STOP=1", "-At"],
        input=query, text=True, capture_output=True, check=True,
    )
    return result.stdout.strip()


if sql("SELECT current_database(), current_user") != "cvera_authority_test|cvera_test":
    raise SystemExit("Refusing unexpected recovery test database/identity")

created_roles = []
created_db = False
try:
    for role in (runtime, operator):
        sql(f"CREATE ROLE {role} NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOBYPASSRLS")
        created_roles.append(role)
    sql(f"CREATE SCHEMA {schema}; SET search_path={schema};"
        + (api_dir / "internal/httpapi/testdata/proposal_a_legacy_schema.sql").read_text()
        + f"; GRANT USAGE ON SCHEMA {schema} TO {runtime};"
        + f"GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA {schema} TO {runtime};"
        + f"ALTER DEFAULT PRIVILEGES IN SCHEMA {schema} "
          f"GRANT SELECT,INSERT,UPDATE,DELETE ON TABLES TO {runtime};")
    sql(f"SET search_path={schema};"
        "INSERT INTO domain_users(id,kratos_identity_id) "
        "VALUES('00000000-0000-4000-8000-000000000001','synthetic-recovery');"
        "INSERT INTO evt_employer_config(employer_id) VALUES('synthetic-recovery');"
        "INSERT INTO evt_requests(request_id,candidate_personid,employer_id) "
        "VALUES('00000000-0000-4000-8000-000000000002',"
        "'00000000-0000-4000-8000-000000000001','synthetic-recovery');")
    # Preserve ownership and ACLs. No production records, credentials or keys.
    dump = subprocess.run(
        base + ["pg_dump", "-U", "cvera_test", "-d", "cvera_authority_test",
                "--format=custom", "--schema=" + schema],
        capture_output=True, check=True,
    ).stdout
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(fd, "wb") as backup:
        backup.write(dump)
    sql(f"CREATE DATABASE {scratch}")
    created_db = True
    subprocess.run(
        base + ["pg_restore", "-U", "cvera_test", "-d", scratch, "--exit-on-error"],
        input=dump, capture_output=True, check=True,
    )
    restored = sql(
        f"SELECT to_regclass('{schema}.domain_users') IS NOT NULL "
        f"AND to_regclass('{schema}.employer_hr_assignments') IS NULL "
        f"AND has_table_privilege('{runtime}','{schema}.domain_users','UPDATE')", scratch,
    )
    if restored != "t":
        raise RuntimeError("Restored parent schema/ACL did not match")

    if sql(f"SELECT count(*) FROM {schema}.evt_requests "
           "WHERE request_id='00000000-0000-4000-8000-000000000002' "
           "AND candidate_personid='00000000-0000-4000-8000-000000000001' "
           "AND employer_id='synthetic-recovery' AND status='DRAFT'", scratch) != "1":
        raise RuntimeError("Restored synthetic request data did not match")

    change = (
        f"BEGIN; SET LOCAL search_path={schema},pg_temp;"
        "SET LOCAL lock_timeout='5s'; SET LOCAL statement_timeout='60s';"
        f"SET LOCAL cvera.hr_runtime_role='{runtime}';"
        f"SET LOCAL cvera.hr_operator_role='{operator}';"
        + (api_dir / "migrations/000014_employer_hr_assignments.sql").read_text()
        + (api_dir / "operations/hr-assignment-privileges.sql").read_text()
    )
    sql(change + "ROLLBACK;")
    if sql(f"SELECT to_regclass('{schema}.employer_hr_assignments') IS NULL") != "t":
        raise RuntimeError("Migration rehearsal left assignment objects")
    sql(change + "COMMIT;")
    bounded = sql(
        f"SELECT has_function_privilege('{runtime}',"
        f"'{schema}.check_employer_hr_assignment(uuid,text)','EXECUTE') "
        f"AND NOT has_any_column_privilege('{runtime}','{schema}.employer_hr_assignments','SELECT') "
        f"AND NOT has_any_column_privilege('{runtime}','{schema}.employer_hr_assignments','UPDATE')"
    )
    if bounded != "t":
        raise RuntimeError("Committed runtime binding exceeded the boundary")
    print(json.dumps({
        "local_only": True,
        "backup_sha256": hashlib.sha256(dump).hexdigest(),
        "backup_mode": oct(path.stat().st_mode & 0o777),
        "fresh_database_restore_and_legacy_acl": True,
        "synthetic_data_restored": True,
        "rollback_rehearsal": True,
        "commit_and_binding_verification": True,
        "source_schema": "supplied-parent-subset; synthetic fixture; not complete production",
    }))
finally:
    if path.exists():
        path.unlink()
    if created_db:
        sql(f"DROP DATABASE {scratch}")
    sql(f"DROP SCHEMA IF EXISTS {schema} CASCADE")
    for role in reversed(created_roles):
        sql(f"DROP OWNED BY {role}; DROP ROLE {role};")
