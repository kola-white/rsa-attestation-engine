#!/usr/bin/env python3
"""Synthetic Proposal A checks ONLY in the named, network-isolated restore clone.

Requires checkpoint 13's migration and NOLOGIN bindings. No host/DSN arguments,
production connection, credential reads, request rows or identity-store changes.
Prints only test outcomes. Synthetic rows are removed after successful checks.
"""
import json
import subprocess
import time
import uuid

CLONE = 'cvera-weekend2-restore-20261102T004322Z'
BASE = ['docker', 'exec', '-i', CLONE, 'psql', '-X', '-U', 'postgres',
        '-d', 'evt', '-Atq', '-v', 'ON_ERROR_STOP=1', '-v', 'VERBOSITY=sqlstate']
API = 'cvera_api_runtime'
OPERATOR = 'cvera_hr_operator'


def metadata(name, field):
    return json.loads(subprocess.check_output(
        ['docker', 'inspect', '--format', '{{json .' + field + '}}', name],
        text=True))


if (metadata(CLONE, 'HostConfig.NetworkMode') != 'none'
        or metadata(CLONE, 'Config.Labels').get('cvera.scope') != 'weekend2-rehearsal'
        or not metadata(CLONE, 'State.Running')):
    raise SystemExit('Refusing anything except the running isolated rehearsal clone')
# Refuse shared database storage even if a container is accidentally mislabelled.
clone_mounts = {x['Source'] for x in metadata(CLONE, 'Mounts') if x.get('Source')}
live_mounts = {x['Source'] for x in metadata('evt-api-go-db-1', 'Mounts') if x.get('Source')}
if clone_mounts & live_mounts:
    raise SystemExit('Refusing clone with shared production mounts')


def transaction(query, role=None, app=None):
    return ("BEGIN ISOLATION LEVEL READ COMMITTED; SET LOCAL search_path=public,pg_temp;"
            "SET LOCAL statement_timeout='20s'; SET LOCAL lock_timeout='12s';"
            + (f"SET LOCAL application_name='{app}';" if app else '')
            + (f'SET LOCAL ROLE {role};' if role else '') + query + ';COMMIT;')


def sql(query, role=None, expected=None):
    result = subprocess.run(BASE, input=transaction(query, role), text=True,
                            capture_output=True, timeout=25)
    if expected:
        if result.returncode == 0 or expected not in result.stderr:
            raise RuntimeError('Expected SQLSTATE ' + expected + ' was not observed')
    elif result.returncode:
        raise RuntimeError('Clone SQL failed; no record/error detail emitted')
    return result.stdout.strip()


def require(condition, label):
    if not condition:
        raise RuntimeError(label)


suffix = uuid.uuid4().hex
principal = str(uuid.uuid4())
unknown = str(uuid.uuid4())
employer = 'weekend2-rehearsal-' + suffix
other = 'weekend2-cross-' + suffix
assignment = str(uuid.uuid4())
children = []


def check(target=employer, person=principal):
    return f"SELECT public.check_employer_hr_assignment('{person}'::uuid,'{target}')"


def grant():
    global assignment
    assignment = str(uuid.uuid4())
    sql("INSERT INTO public.employer_hr_assignments"
        "(assignment_id,principal_id,employer_id,granted_by,grant_basis_ref) "
        f"VALUES('{assignment}','{principal}','{employer}',"
        "'isolated-rehearsal','synthetic-only')", OPERATOR)


def revoke():
    return ("UPDATE public.employer_hr_assignments SET revoked_at=clock_timestamp(),"
            "revoked_by='isolated-rehearsal',revocation_reason='synthetic-only' "
            f"WHERE assignment_id='{assignment}'")


def start(query, role, name):
    child = subprocess.Popen(BASE, stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                             stderr=subprocess.PIPE, text=True)
    children.append(child)
    child.stdin.write(transaction(query, role, name))
    child.stdin.close()
    child.stdin = None
    return child


def await_state(name, blocked=False):
    deadline = time.monotonic() + 10
    predicate = 'cardinality(pg_blocking_pids(pid))>0' if blocked else "wait_event='PgSleep'"
    while time.monotonic() < deadline:
        if sql("SELECT EXISTS(SELECT 1 FROM pg_catalog.pg_stat_activity "
               f"WHERE application_name='{name}' AND ({predicate}))") == 't':
            return
        time.sleep(0.1)
    raise RuntimeError('Expected concurrent lock state was not observed')


def finish(child, expected=None):
    output, _ = child.communicate(timeout=25)
    require(child.returncode == 0, 'Concurrent transaction failed')
    if expected:
        require(output.strip() == expected, 'Unexpected concurrent authorization result')


try:
    sql("INSERT INTO public.domain_users(id,kratos_identity_id,role,status) "
        f"VALUES('{principal}','weekend2-{suffix}','hr_reviewer','active');"
        f"INSERT INTO public.evt_employer_config(employer_id) VALUES('{employer}'),('{other}')")
    require(sql(check(), API) == 'f', 'Missing assignment did not deny')
    grant()
    require(sql(check(), API) == 't', 'Explicit assignment did not allow')
    require(sql(check(other), API) == 'f', 'Cross-employer authorization allowed')
    require(sql(check(person=unknown), API) == 'f', 'Unknown principal allowed')
    sql('SELECT * FROM public.employer_hr_assignments LIMIT 0', API, '42501')
    sql(f"SELECT * FROM public.lock_employer_hr_principal('{principal}')", API, '42501')
    sql(revoke(), API, '42501')
    sql(f"UPDATE public.domain_users SET role='cvera' WHERE id='{principal}'", API, '42501')
    sql(f"DELETE FROM public.employer_hr_assignments WHERE assignment_id='{assignment}'", OPERATOR, '42501')
    sql(f"UPDATE public.domain_users SET status='inactive' WHERE id='{principal}'")
    require(sql(check(), API) == 'f', 'Inactive principal allowed')
    sql(f"UPDATE public.domain_users SET status='active',role='candidate' WHERE id='{principal}'")
    require(sql(check(), API) == 'f', 'Non-HR principal allowed')
    sql(f"UPDATE public.domain_users SET role='hr_reviewer' WHERE id='{principal}'")

    # Request first: revocation must wait for the admitted transaction to finish.
    first_name = 'w2-request-' + suffix
    second_name = 'w2-revoke-' + suffix
    first = start(check() + ';SELECT pg_sleep(5)', API, first_name)
    await_state(first_name)
    second = start(revoke(), OPERATOR, second_name)
    await_state(second_name, blocked=True)
    finish(first, 't')
    finish(second)
    require(sql(check(), API) == 'f', 'Committed revocation did not deny')
    sql(revoke(), OPERATOR, '23514')

    # Revocation first: a waiting request must observe the committed revocation.
    grant()
    first_name = 'w2-revoke-first-' + suffix
    second_name = 'w2-request-second-' + suffix
    first = start(revoke() + ';SELECT pg_sleep(5)', OPERATOR, first_name)
    await_state(first_name)
    second = start(check(), API, second_name)
    await_state(second_name, blocked=True)
    finish(first)
    finish(second, 'f')
    results = {'isolated_clone': True, 'deny_by_default': True,
                      'explicit_assignment': True, 'cross_employer_denial': True,
                      'current_status_and_role': True, 'runtime_admin_denial': True,
                      'revocation': True, 'revoked_row_immutable': True,
                      'request_first_concurrency': True, 'revocation_first_concurrency': True}
finally:
    for child in children:
        if child.poll() is None:
            child.terminate()
            child.communicate(timeout=10)
    # Owner cleanup is limited to the random synthetic identifiers above.
    sql(f"DELETE FROM public.employer_hr_assignments WHERE principal_id='{principal}';"
        f"DELETE FROM public.domain_users WHERE id='{principal}';"
        f"DELETE FROM public.evt_employer_config WHERE employer_id IN ('{employer}','{other}')")

print(json.dumps(results))
