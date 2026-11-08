#!/usr/bin/env python3
"""Approved synthetic two-account review acceptance; no identities/signing/deletion.

Stage alongside the reviewed smoke-evidence-completion.py. Credentials are entered
only at hidden prompts and retained only in memory. Manual approval leaves PENDING.
"""
import base64
import getpass
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import re
import sys
import urllib.error
import urllib.request
import uuid

HERE = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location('completion_smoke', HERE / 'smoke-evidence-completion.py')
s = importlib.util.module_from_spec(spec)
spec.loader.exec_module(s)
PHASE = 'preflight'
RECORD = None


def login(label, sessions):
    flow = s.expect(s.http(s.AUTH, '/self-service/login/api'), 200)
    identifier = getpass.getpass(label + ' existing login identifier (hidden): ')
    password = getpass.getpass(label + ' existing password (hidden): ')
    response = s.expect(s.http(s.AUTH, '/self-service/login?flow=' + str(uuid.UUID(flow['id'])), 'POST',
                              {'method': 'password', 'identifier': identifier, 'password': password}), 200)
    identifier = password = None
    session = {'native': response['session_token']}
    sessions.append(session)
    diagnostic = s.account_diagnostic(s.http(s.AUTH, '/sessions/whoami', kratos=session['native']))
    if diagnostic['whoami_http'] != 200 or not diagnostic['session_active'] or not diagnostic['identity_id_present']:
        raise RuntimeError('active normal account required')
    exchanged = s.expect(s.http(s.API, '/auth/exchange', 'POST',
                               {'kratos_session_token': session['native'], 'device_id': 'evidence-review-release'}), 200)
    session.update(principal=str(uuid.UUID(exchanged['user']['id'])), access=exchanged['access_token'], refresh=exchanged['refresh_token'])
    return session


def download(path, token):
    request = urllib.request.Request(s.API + path, headers={'Authorization': 'Bearer ' + token})
    with s.opener.open(request, timeout=30) as response:
        data = response.read(5 * 1024 * 1024 + 1)
        if response.status != 200 or response.headers.get('Content-Type') != 'application/octet-stream' \
           or response.headers.get('Cache-Control') != 'private, no-store' \
           or response.headers.get('X-Content-Type-Options') != 'nosniff' \
           or response.headers.get('X-Evidence-Scan-Status') != 'PENDING' \
           or not response.headers.get('Content-Disposition', '').startswith('attachment;'):
            raise RuntimeError('unsafe attachment response')
        return data


def exercise(uploader, reviewer, case, check, settings, variables, grants):
    global PHASE
    principal = uploader['principal']; other = reviewer['principal']
    def grant(function, subject):
        value = str(uuid.UUID(s.sql('SELECT public.' + function + "(:'principal'::uuid,:'caseid',:'actor',:'basis');",
                                   {**variables, 'principal': subject}, True)))
        grants.append(('revoke_evidence_upload' if function == 'grant_evidence_upload' else 'revoke_evidence_review', value))
        return value
    PHASE = 'synthetic_upload_and_completion'
    grant('grant_evidence_upload', principal)
    data = base64.b64decode('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+a5XkAAAAASUVORK5CYII=')
    body = {'caseId': case, 'checkId': check, 'files': [{'name': 'synthetic-review.png', 'size': len(data), 'mimeType': 'image/png'}]}
    init = s.expect(s.http(s.API, '/v1/evidence/init', 'POST', body, uploader['access']), 200)
    upload = init['uploads'][0]
    s.storage_url(upload['url'], upload['storageKey'], settings)
    status, _ = s.object_request(upload['url'], 'PUT', data, upload['headers'])
    if status not in (200, 204):
        raise RuntimeError('synthetic upload failed')
    digest = hashlib.sha256(data).hexdigest()
    complete = {'uploadSessionId': init['uploadSessionId'], 'uploads': [{'uploadId': upload['uploadId'], 'size': len(data), 'mimeType': 'image/png'}]}
    receipt = s.expect(s.http(s.API, '/v1/evidence/commit', 'POST', complete, uploader['access']), 200)['evidence']
    if len(receipt) != 1 or receipt[0].get('scanStatus') != 'PENDING' or receipt[0].get('evidence_hash') != digest:
        raise RuntimeError('unexpected completion')
    base = '/v1/cases/' + case + '/checks/' + check + '/evidence'
    attachment = base + '/' + str(uuid.UUID(upload['uploadId'])) + '/download'
    review = base + '/' + upload['uploadId'] + '/review'
    decision = {'decision': 'APPROVED', 'reason': 'Independent synthetic release fixture review'}
    PHASE = 'grant_separation_and_unauthorized_denial'
    for token, expected in ((None, 401), (uploader['access'], 404), (reviewer['access'], 404)):
        for path in (base, attachment):
            s.expect(s.http(s.API, path, token=token), expected)
        s.expect(s.http(s.API, review, 'POST', decision, token), expected)
    grant('grant_evidence_review', principal)
    s.expect(s.http(s.API, review, 'POST', decision, uploader['access']), 403)
    review_grant = grant('grant_evidence_review', other)
    PHASE = 'independent_private_listing_and_download'
    discovery = s.expect(s.http(s.API, '/v1/evidence/checks', token=reviewer['access']), 200)['items']
    if not any(row.get('caseId') == case and row.get('checkId') == check and row.get('canReview') is True and row.get('canUpload') is False for row in discovery):
        raise RuntimeError('incorrect review capabilities')
    listing = s.expect(s.http(s.API, base, token=reviewer['access']), 200)
    evidence = listing['evidence']
    if len(evidence) != 1 or evidence[0].get('scanStatus') != 'PENDING' or evidence[0].get('reviewStatus') != 'PENDING' \
       or evidence[0].get('evidence_hash') != digest or evidence[0].get('uploadId') != upload['uploadId']:
        raise RuntimeError('unexpected evidence listing')
    if any(term in json.dumps(listing) for term in ('sealed-evidence/', 'storageKey', 'X-Amz-', 'https://')):
        raise RuntimeError('unexpected storage disclosure')
    if download(attachment, reviewer['access']) != data:
        raise RuntimeError('attachment does not match sealed bytes')
    for path in (base.replace('/checks/' + check, '/checks/not-the-check'), attachment.replace('/' + case + '/', '/not-the-case/')):
        s.expect(s.http(s.API, path, token=reviewer['access']), 404)
    s.expect(s.http(s.API, '/v1/evidence/init', 'POST', body, reviewer['access']), 404)
    PHASE = 'attributable_final_review_pending_scan'
    first = s.expect(s.http(s.API, review, 'POST', decision, reviewer['access']), 200)
    if first.get('reviewerId') != other or first.get('uploadId') != upload['uploadId'] or first.get('decision') != 'APPROVED' or not first.get('reviewedAt'):
        raise RuntimeError('incorrect review attribution')
    if s.expect(s.http(s.API, review, 'POST', decision, reviewer['access']), 200) != first:
        raise RuntimeError('retry changed final decision')
    s.expect(s.http(s.API, review, 'POST', {'decision': 'REJECTED', 'reason': 'Conflicting synthetic retry'}, reviewer['access']), 409)
    row = s.expect(s.http(s.API, base, token=reviewer['access']), 200)['evidence'][0]
    if row.get('scanStatus') != 'PENDING' or row.get('reviewStatus') != 'APPROVED' or row.get('reviewerId') != other:
        raise RuntimeError('review/scan boundary changed')
    PHASE = 'committed_review_revocation'
    s.sql("SELECT public.revoke_evidence_review(:'grantid'::uuid,:'actor',:'basis');", {**variables, 'grantid': review_grant}, True)
    grants.remove(('revoke_evidence_review', review_grant))
    for path in (base, attachment):
        s.expect(s.http(s.API, path, token=reviewer['access']), 404)
    s.expect(s.http(s.API, review, 'POST', decision, reviewer['access']), 404)
    return {'normal_two_account_login_exchange': 'PASS', 'explicit_grant_separation': 'PASS',
            'unauthorized_wrong_case_check_denial': 'PASS', 'self_review_denied': 'PASS',
            'private_attachment_canonical_bytes': 'PASS', 'attributable_idempotent_review': 'PASS',
            'conflicting_review_denied': 'PASS', 'review_revocation': 'PASS', 'manual_decision': 'APPROVED', 'scan_status': 'PENDING',
            'objects_uploaded': 1, 'new_identities': 0, 'new_attestations': 0}


def main():
    global PHASE, RECORD
    if os.geteuid() != 0 or len(sys.argv) != 3 or sys.argv[1] != '--expected-main' or not re.fullmatch('[0-9a-f]{40}', sys.argv[2]):
        raise RuntimeError('explicit deployed revision and owner console required')
    revision = sys.argv[2]; os.umask(0o077)
    if s.command(['git', '-C', str(s.ROOT), 'rev-parse', 'HEAD']).decode().strip() != revision \
       or s.command(['git', '-C', str(s.ROOT), 'status', '--porcelain']).strip() \
       or s.expect(s.http(s.API, '/version'), 200).get('git_sha') != revision:
        raise RuntimeError('unexpected revision')
    s.expect(s.http(s.API, '/healthz'), 200)
    container = json.loads(s.command(['docker', 'inspect', 'evt-api-go-api-1']))[0]
    settings = dict(row.split('=', 1) for row in container['Config']['Env'] if '=' in row)
    case = 'review-release-' + uuid.uuid4().hex[:24]; check = 'smoke.file'
    sessions = []; grants = []; created = False
    RECORD = s.ROOT.parent / ('evidence-review-smoke-' + uuid.uuid4().hex + '.json')
    fd = os.open(RECORD, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(fd, 'w') as stream:
        json.dump({'main': revision, 'synthetic_case': case, 'synthetic_check': check, 'state': 'intent; inspect if STOP'}, stream)
        stream.flush(); os.fsync(stream.fileno())
    variables = {'caseid': case, 'checkid': check, 'actor': 'owner-authorized-review-release', 'basis': 'Synthetic evidence review acceptance ' + revision}
    try:
        PHASE = 'normal_uploader_login'
        uploader = login('Uploader', sessions)
        PHASE = 'normal_independent_reviewer_login'
        reviewer = login('Independent reviewer', sessions)
        if uploader['principal'] == reviewer['principal']:
            raise RuntimeError('two distinct authenticated canonical principals required; no self-review bypass')
        PHASE = 'explicit_synthetic_operator_provisioning'
        s.sql("SELECT public.provision_evidence_case(:'caseid',:'actor',:'basis');SELECT public.provision_evidence_check(:'caseid',:'checkid',:'actor',:'basis');", variables, True)
        created = True
        result = exercise(uploader, reviewer, case, check, settings, variables, grants)
    finally:
        failed = []
        for function, grant in grants:
            try:
                s.sql('SELECT public.' + function + "(:'grantid'::uuid,:'actor',:'basis');", {**variables, 'grantid': grant}, True)
            except Exception:
                failed.append('grant')
        if created:
            try:
                s.sql("SELECT public.set_evidence_resource_state(:'caseid',NULL,'CLOSED',:'actor',:'basis');", variables, True)
            except Exception:
                failed.append('case')
        for session in sessions:
            actions = []
            if session.get('refresh'):
                actions.append(lambda item=session: s.expect(s.http(s.API, '/auth/logout', 'POST', {'refresh_token': item['refresh']}), 200))
            actions.append(lambda item=session: s.expect(s.http(s.AUTH, '/self-service/logout/api', 'DELETE', {'session_token': item['native']}), 204))
            for action in actions:
                try:
                    action()
                except Exception:
                    failed.append('session')
        if failed:
            PHASE = 'cleanup_failed:' + ','.join(failed)
            raise RuntimeError('inspect exact synthetic fixture privately')
    result.update(main=revision, smoke='PASS', credentials_urls_or_contents_printed=False)
    result['fixture_grants_revoked_case_closed'] = True
    intent = json.loads(RECORD.read_text())
    temporary = RECORD.with_suffix('.next')
    fd = os.open(temporary, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(fd, 'w') as stream:
        json.dump({'intent': intent, 'result': result}, stream);stream.flush();os.fsync(stream.fileno())
    os.replace(temporary, RECORD)
    result['private_record'] = str(RECORD)
    print(json.dumps(result))


if __name__ == '__main__':
    try:
        main()
    except Exception:
        print(json.dumps({'smoke': 'STOP', 'phase': PHASE, 'private_record': str(RECORD) if RECORD else None, 'instruction': 'Return only this status; never credentials, URLs, contents or raw responses.'}), file=sys.stderr)
        sys.exit(1)
