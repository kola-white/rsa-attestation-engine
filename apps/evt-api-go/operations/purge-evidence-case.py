#!/usr/bin/env python3
"""Owner-console, case-scoped evidence deletion. DRY RUN unless explicitly armed.

No HTTP admin endpoint, worker, role/credential change, account or signing operation.
S3 deletions are irreversible; production use requires explicit per-case authority.
"""
import argparse
import datetime
import hashlib
import json
import os
from pathlib import Path
import re
import stat
import subprocess
import sys
import time
import uuid

from evidence_storage_operator import PrivateObjectStore, StorageFailure

ROOT = Path('/opt/cvera/rsa-attestation-engine')
RECEIPTS = Path('/opt/cvera/evidence-deletions')
DB = 'evt-api-go-db-1'
PHASE = 'preflight'
RECORD = None
MAX_OBJECTS = 1000
CASE = re.compile(r'^[A-Za-z0-9._-]{1,64}$')
UUID = r'[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}'

SNAPSHOT = """
SELECT jsonb_build_object(
 'status',(SELECT status FROM cases WHERE case_id=current_setting('cvera.purge_case')),
 'basis',(SELECT update_basis_ref FROM cases WHERE case_id=current_setting('cvera.purge_case')),
 'files',coalesce((SELECT jsonb_agg(jsonb_build_object('id',upload_id,'staging',staging_key,'sealed',sealed_key) ORDER BY upload_id)
   FROM evidence_uploads WHERE case_id=current_setting('cvera.purge_case')),'[]'::jsonb),
 'fingerprint',(SELECT md5(coalesce(jsonb_agg(to_jsonb(u) ORDER BY upload_id)::text,'[]')) FROM evidence_uploads u WHERE case_id=current_setting('cvera.purge_case')),
 'wait_seconds',greatest(0,coalesce((SELECT ceil(extract(epoch FROM max(expires_at)+interval '60 seconds'-clock_timestamp()))::int
   FROM upload_sessions WHERE case_id=current_setting('cvera.purge_case')),0)),
 'metadata_redacted',NOT EXISTS(SELECT 1 FROM evidence_uploads WHERE case_id=current_setting('cvera.purge_case') AND (file_name<>'[deleted]' OR staging_key<>'deleted-staging/'||upload_id::text OR (status='RECEIVED' AND sealed_key<>'sealed-evidence/deleted/'||upload_id::text)))
  AND NOT EXISTS(SELECT 1 FROM evidence_reviews WHERE upload_id IN (SELECT upload_id FROM evidence_uploads WHERE case_id=current_setting('cvera.purge_case')) AND reason<>'[redacted following authorized evidence deletion]'),
 'active_grants',(SELECT count(*) FROM case_evidence_upload_grants WHERE case_id=current_setting('cvera.purge_case') AND revoked_at IS NULL)
  +(SELECT count(*) FROM case_evidence_review_grants WHERE case_id=current_setting('cvera.purge_case') AND revoked_at IS NULL)
);
"""


def command(args, data=None, timeout=30):
    result = subprocess.run(args, input=data, capture_output=True, timeout=timeout)
    if result.returncode:
        raise RuntimeError('operator command failed; raw output suppressed')
    return result.stdout


class OwnerDatabase:
    def execute(self, body, values, readonly=False):
        args = ['docker', 'exec', '-i', DB, 'psql', '-X', '-U', 'postgres', '-d', 'evt', '-Atq', '-v', 'ON_ERROR_STOP=1']
        for key, value in values.items():
            args += ['-v', key + '=' + str(value)]
        prefix = ('BEGIN READ ONLY;' if readonly else 'BEGIN;') + "SET LOCAL search_path=public,pg_temp;SET LOCAL lock_timeout='5s';SET LOCAL statement_timeout='30s';"
        for key in values:
            prefix += "SELECT set_config('cvera.purge_" + key + "',:'" + key + "',true) IS NOT NULL;"
        raw = command(args, (prefix + body + '\nCOMMIT;').encode()).decode().splitlines()
        return [line for line in raw if line not in ('t', '')]

    def snapshot(self, values):
        return json.loads(self.execute(SNAPSHOT, values, readonly=True)[-1])

    def freeze(self, values):
        self.execute((Path(__file__).with_name('evidence-case-delete-freeze.sql')).read_text(), values)

    def redact(self, values):
        self.execute((Path(__file__).with_name('evidence-case-delete-redact.sql')).read_text(), values)


def marker(record):
    return record['basis'] + ' [purge:' + record['operation'] + ']'


def values(record):
    return {'case': record['case'], 'actor': record['actor'], 'basis': record['basis'], 'operation': record['operation']}


def save(path, record):
    temporary = path.with_suffix('.next')
    fd = os.open(temporary, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(fd, 'w') as stream:
        json.dump(record, stream, sort_keys=True)
        stream.flush()
        os.fsync(stream.fileno())
    os.replace(temporary, path)


def validate_key(case, key, registered=False):
    if not isinstance(key, str) or not re.fullmatch(r'[A-Za-z0-9._/-]+', key) or any(part in ('.', '..') for part in key.split('/')):
        raise RuntimeError('unexpected object key')
    case_part = re.escape(case)
    stage = r'cases/' + case_part + r'/checks/[A-Za-z0-9._-]{1,96}/evidence(?:/' + UUID + r'(?:/[A-Za-z0-9._-]{1,120})?)?'
    sealed = r'sealed-evidence/' + case_part + r'/[A-Za-z0-9._-]{1,96}/' + UUID + '/' + UUID
    if re.fullmatch(stage, key) or re.fullmatch(sealed, key):
        return
    if registered and re.fullmatch('sealed-evidence/' + UUID, key):
        return  # Exact registered legacy key only; never sweep the global prefix.
    raise RuntimeError('object does not belong to selected case')


def plan_objects(case, snapshot, storage):
    keys = set()
    for row in snapshot['files']:
        uuid.UUID(row['id'])
        for key in (row['staging'], row['sealed']):
            if key:
                validate_key(case, key, registered=True)
                keys.add(key)
    for prefix in ('cases/' + case + '/checks/', 'sealed-evidence/' + case + '/'):
        for key in storage.list_keys(prefix, MAX_OBJECTS):
            validate_key(case, key)
            keys.add(key)
    if len(keys) > MAX_OBJECTS:
        raise RuntimeError('case object limit exceeded; use a separately reviewed plan')
    return sorted(keys)


def purge(database, storage, record, path, max_wait=720, clock=time.sleep):
    global PHASE
    params = values(record)
    snapshot = database.snapshot(params)
    if snapshot['status'] in ('OPEN', 'CLOSED') and record['phase'] == 'prepared':
        record['phase'] = 'freeze_attempted';save(path, record)
        PHASE = 'freezing_case_and_revoking_grants'
        database.freeze(params)
        record['phase'] = 'frozen';save(path, record)
        snapshot = database.snapshot(params)
    # A completed-but-unacknowledged metadata commit can be verified on resume.
    completed = snapshot['status'] == 'DELETED' and record['phase'] == 'metadata_commit_attempted'
    if snapshot['basis'] != marker(record) or snapshot['active_grants'] != 0 or (snapshot['status'] != 'DELETION_SCHEDULED' and not completed):
        raise RuntimeError('case state/operation/grants changed; inspect before retry')
    PHASE = 'waiting_for_upload_url_expiry'
    if snapshot['wait_seconds'] > max_wait:
        raise RuntimeError('expiry exceeds bounded wait; case remains frozen')
    remaining = max_wait
    while snapshot['wait_seconds'] > 0:
        if remaining <= 0:
            raise RuntimeError('bounded wait exhausted; case remains frozen')
        delay = min(snapshot['wait_seconds'], 5, remaining)
        clock(delay)
        remaining -= delay
        snapshot = database.snapshot(params)
        if snapshot['status'] != 'DELETION_SCHEDULED' or snapshot['basis'] != marker(record) or snapshot['active_grants']:
            raise RuntimeError('frozen case changed')
    PHASE = 'planning_scoped_objects'
    if not completed:
        current = plan_objects(record['case'], snapshot, storage)
        # Original registered legacy keys remain in the plan across partial retries.
        record['targets'] = sorted(set(record.get('targets', [])) | set(current))
        for key in record['targets']:
            validate_key(record['case'], key, registered=True)
        record['fingerprint'] = snapshot['fingerprint']
        record['phase'] = 'deleting';save(path, record)
        PHASE = 'deleting_scoped_objects'
        for key in record['targets']:
            storage.delete(key)
    PHASE = 'verifying_object_absence'
    for key in record['targets']:
        if not storage.absent(key):
            raise RuntimeError('object still present; case remains frozen')
    for prefix in ('cases/' + record['case'] + '/checks/', 'sealed-evidence/' + record['case'] + '/'):
        if storage.list_keys(prefix, MAX_OBJECTS):
            raise RuntimeError('scoped objects remain; inspect late writes before resume')
    if not completed:
        PHASE = 'redacting_metadata_and_marking_deleted'
        record['phase'] = 'metadata_commit_attempted';save(path, record)
        database.redact({**params, 'fingerprint': record['fingerprint']})
        snapshot = database.snapshot(params)
    if snapshot['status'] != 'DELETED' or snapshot['basis'] != marker(record) or snapshot['active_grants'] or not snapshot['metadata_redacted']:
        raise RuntimeError('deleted case verification failed')
    # Remove identifying key strings from the operational receipt on success.
    count = len(record['targets'])
    record['target_manifest_sha256'] = hashlib.sha256(json.dumps(record.pop('targets'), separators=(',', ':')).encode()).hexdigest()
    record['objects_confirmed_absent'] = count
    record['phase'] = 'complete'
    record['completed_at'] = datetime.datetime.now(datetime.timezone.utc).isoformat()
    save(path, record)
    return {'case_evidence_deletion': 'PASS', 'objects_confirmed_absent': count,
            'access_revoked_case_deleted': True, 'file_metadata_redacted': True,
            'verification_scope': 'registered and case-prefixed current objects only',
            'legacy_global_orphan_cleanup': 'not performed', 'versions_and_backups_purged': False,
            'private_record': str(path)}


def main():
    global PHASE, RECORD
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--case', required=True)
    parser.add_argument('--actor', default='')
    parser.add_argument('--basis', default='')
    parser.add_argument('--apply-and-acknowledge-permanent-deletion', action='store_true')
    parser.add_argument('--resume-record')
    args = parser.parse_args()
    if os.geteuid() != 0 or not CASE.fullmatch(args.case) or args.case in ('.', '..'):
        raise RuntimeError('existing owner console and exact case required')
    os.umask(0o077)
    # No secrets are printed/persisted. No private signing-key path is opened.
    container = json.loads(command(['docker', 'inspect', 'evt-api-go-api-1']))[0]
    settings = dict(row.split('=', 1) for row in container['Config']['Env'] if '=' in row)
    storage = PrivateObjectStore(settings)
    database = OwnerDatabase()
    source_names=('purge-evidence-case.py','evidence_storage_operator.py','evidence-case-delete-freeze.sql','evidence-case-delete-redact.sql')
    source_hashes={name:hashlib.sha256(Path(__file__).with_name(name).read_bytes()).hexdigest() for name in source_names}
    record = {'case': args.case, 'actor': args.actor, 'basis': args.basis,
              'operation': str(uuid.uuid4()), 'phase': 'prepared',
              'source_sha256':source_hashes,'storage_identity':{'endpoint':storage.endpoint,'bucket':storage.bucket,'region':storage.region}}
    if not args.apply_and_acknowledge_permanent_deletion:
        snapshot = database.snapshot(values(record))
        if snapshot['status'] not in ('OPEN', 'CLOSED', 'DELETION_SCHEDULED'):
            raise RuntimeError('unknown/deleted case; inspect prior record')
        count = len(plan_objects(args.case, snapshot, storage))
        print(json.dumps({'case_evidence_deletion': 'DRY_RUN', 'changes_made': False,
                          'registered_uploads': len(snapshot['files']), 'object_deletion_candidates': count,
                          'wait_after_freeze_seconds': snapshot['wait_seconds'],
                          'legacy_global_orphan_cleanup': 'not performed'}))
        return
    if not args.actor.strip() or not args.basis.strip() or len(args.actor) > 200 or len(args.basis) > 500:
        raise RuntimeError('explicit actor and deletion authority reference required')
    RECEIPTS.mkdir(mode=0o700, exist_ok=True)
    st = RECEIPTS.stat()
    if RECEIPTS.is_symlink() or st.st_uid != 0 or stat.S_IMODE(st.st_mode) != 0o700:
        raise RuntimeError('private receipt directory required')
    if args.resume_record:
        path = Path(args.resume_record)
        st = path.stat()
        if path.is_symlink() or path.parent.resolve() != RECEIPTS.resolve() or st.st_uid != 0 or stat.S_IMODE(st.st_mode) != 0o600:
            raise RuntimeError('exact root-private prior record required')
        current_identity=record['storage_identity']
        record = json.loads(path.read_text())
        if record.get('source_sha256')!=source_hashes or record.get('storage_identity')!=current_identity:
            raise RuntimeError('resume source/storage changed; inspect before retry')
        if record['case'] != args.case or record['actor'] != args.actor or record['basis'] != args.basis or record['phase'] == 'complete':
            raise RuntimeError('resume authority mismatch or already complete')
    else:
        path = RECEIPTS / (record['operation'] + '.json')
        save(path, record)
    RECORD = path
    print(json.dumps(purge(database, storage, record, path)))


if __name__ == '__main__':
    try:
        main()
    except (Exception, KeyboardInterrupt) as error:
        print(json.dumps({'case_evidence_deletion': 'STOP', 'phase': PHASE,
                          'storage_status': error.status if isinstance(error, StorageFailure) else None,
                          'private_record': str(RECORD) if RECORD else None,
                          'instruction': 'Do not broaden scope or retry without inspecting the private record; case may remain frozen. No raw data/credentials printed.'}), file=sys.stderr)
        sys.exit(1)
