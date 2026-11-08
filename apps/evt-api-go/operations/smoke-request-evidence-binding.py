#!/usr/bin/env python3
"""Approved, non-signing two-request synthetic binding acceptance.

Stage beside the two reviewed evidence smoke helpers. Existing account credentials
are local hidden prompts only. No role/credential/identity changes or live signing.
"""
import base64
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import re
import sys
import uuid

HERE=Path(__file__).resolve().parent
spec=importlib.util.spec_from_file_location('review_smoke',HERE/'smoke-evidence-review.py')
r=importlib.util.module_from_spec(spec);spec.loader.exec_module(r)
s=r.s
PHASE='preflight';RECORD=None;FAILURE_DIAGNOSTIC={}


def persist(record):
    temp=RECORD.with_suffix('.next')
    fd=os.open(temp,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
    with os.fdopen(fd,'w') as stream:
        json.dump(record,stream);stream.flush();os.fsync(stream.fileno())
    os.replace(temp,RECORD)


def fixture_sql(variables):
    # Owner-approved synthetic rows only; status/version are explicit, with no
    # invented default, principal role, employer relationship or real-world claim.
    s.sql("INSERT INTO public.evt_employer_config(employer_id) VALUES(:'employer');"
          "INSERT INTO public.evt_requests(request_id,candidate_personid,employer_id,status,claim_snapshot,version) VALUES"
          "(:'request0'::uuid,:'subject'::uuid,:'employer','ATTESTATION_PENDING',:'snapshot'::jsonb,1),"
          "(:'request1'::uuid,:'subject'::uuid,:'employer','ATTESTATION_PENDING',:'snapshot'::jsonb,1);",variables)


def hr_grant(variables):
    return str(uuid.UUID(s.sql("SET LOCAL ROLE cvera_hr_operator;INSERT INTO public.employer_hr_assignments"
        "(assignment_id,principal_id,employer_id,granted_by,grant_basis_ref)"
        "VALUES(gen_random_uuid(),:'reviewer'::uuid,:'employer',:'actor',:'basis') RETURNING assignment_id;",variables)))


def hr_revoke(variables,assignment):
    s.sql("SET LOCAL ROLE cvera_hr_operator;SELECT id FROM public.lock_employer_hr_principal(:'reviewer'::uuid);"
          "UPDATE public.employer_hr_assignments SET revoked_at=clock_timestamp(),revoked_by=:'actor',revocation_reason=:'basis'"
          " WHERE assignment_id=:'assignment'::uuid AND revoked_at IS NULL;",{**variables,'assignment':assignment})


def exercise(variables,uploader,reviewer,settings,state):
    global PHASE
    tokens=(uploader['access'],reviewer['access']);cases=state['cases'];requests=state['requests'];check='smoke.file'
    def op(statement,values=None):return s.sql(statement,{**variables,**(values or {})},True)
    for i in range(2):
        PHASE='explicit_case_check_binding_'+str(i)
        op("SELECT public.provision_evidence_case(:'case',:'actor',:'basis');SELECT public.provision_evidence_check(:'case',:'check',:'actor',:'basis');",{'case':cases[i]})
        binding=str(uuid.UUID(op("SELECT public.bind_request_evidence(:'request'::uuid,:'case',ARRAY[:'check'],:'actor',:'basis');",{'case':cases[i],'request':requests[i]})))
        state['bindings'].append(binding);persist(state)
        grant=str(uuid.UUID(op("SELECT public.grant_evidence_review(:'reviewer'::uuid,:'case',:'actor',:'basis');",{'case':cases[i]})))
        state['grants'].append(['revoke_evidence_review',grant]);persist(state)
    upload_grant=str(uuid.UUID(op("SELECT public.grant_evidence_upload(:'subject'::uuid,:'case',:'actor',:'basis');",{'case':cases[0]})))
    state['grants'].append(['revoke_evidence_upload',upload_grant]);persist(state)
    self_grant=str(uuid.UUID(op("SELECT public.grant_evidence_review(:'subject'::uuid,:'case',:'actor',:'basis');",{'case':cases[0]})))
    state['grants'].append(['revoke_evidence_review',self_grant]);persist(state)
    data=base64.b64decode('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+a5XkAAAAASUVORK5CYII=')
    PHASE='synthetic_init_upload_complete'
    body={'caseId':cases[0],'checkId':check,'files':[{'name':'synthetic-binding.png','size':len(data),'mimeType':'image/png'}]}
    init=s.expect(s.http(s.API,'/v1/evidence/init','POST',body,tokens[0]),200);upload=init['uploads'][0]
    s.storage_url(upload['url'],upload['storageKey'],settings)
    status,_=s.object_request(upload['url'],'PUT',data,upload['headers'])
    if status not in (200,204):raise RuntimeError('synthetic staging failed')
    completed=s.expect(s.http(s.API,'/v1/evidence/commit','POST',{'uploadSessionId':init['uploadSessionId'],'uploads':[{'uploadId':upload['uploadId'],'size':len(data),'mimeType':'image/png'}]},tokens[0]),200)['evidence']
    if len(completed)!=1 or completed[0].get('evidence_hash')!=hashlib.sha256(data).hexdigest() or completed[0].get('scanStatus')!='PENDING':raise RuntimeError('invalid completion')
    base='/v1/cases/'+cases[0]+'/checks/'+check+'/evidence';review=base+'/'+upload['uploadId']+'/review'
    PHASE='contextual_independent_review'
    listing=s.expect(s.http(s.API,base,token=tokens[1]),200);context=listing['binding']
    if context.get('bindingId')!=state['bindings'][0] or context.get('requestId')!=requests[0] or context.get('subjectId')!=uploader['principal'] or context.get('issuerId')!='https://cvera.app' or context.get('employerId')!=variables['employer'] or context.get('contextValid') is not True:raise RuntimeError('wrong bound context')
    decision={'decision':'APPROVED','reason':'Independent synthetic request-context review'}
    s.expect(s.http(s.API,review,'POST',decision,tokens[1]),400)
    decision.update(bindingId=context['bindingId'],claimDigest=context['claimDigest'])
    s.expect(s.http(s.API,review,'POST',decision,tokens[0]),403)
    first=s.expect(s.http(s.API,review,'POST',decision,tokens[1]),200)
    if first.get('reviewerId')!=reviewer['principal'] or s.expect(s.http(s.API,review,'POST',decision,tokens[1]),200)!=first:raise RuntimeError('bad review attribution/retry')
    endpoint='/v1/employer/requests/'+requests[0]+'/evidence-eligibility';scope={'employer_id':variables['employer']}
    PHASE='eligibility_current_authorization_and_pending_policy'
    s.expect(s.http(s.API,endpoint,'POST',scope),401)
    s.expect(s.http(s.API,endpoint,'POST',scope,tokens[0]),404)
    prepared=s.expect(s.http(s.API,endpoint,'POST',{**scope,'authorityDecision':'AUTHORIZED','eligible':True,'issuerId':'https://wrong.invalid'},tokens[1]),200)
    if prepared.get('evidenceReady') is not True or prepared.get('eligible') is not False or prepared.get('signingEnabled') is not False or prepared.get('reason')!='AUTHORITY_SOURCE_UNAVAILABLE' or prepared.get('scanStatuses')!=['PENDING'] or prepared.get('pendingScanCount')!=1 or prepared.get('manualPolicy')!='MANUAL_REVIEW_PILOT_V1' or not prepared.get('manifestId'):raise RuntimeError('authority or malware boundary violated')
    repeated=s.expect(s.http(s.API,endpoint,'POST',scope,tokens[1]),200)
    if repeated.get('manifestId')!=prepared['manifestId']:raise RuntimeError('non-idempotent manifest')
    PHASE='persisted_cross_request_reuse_denial'
    otherbase='/v1/cases/'+cases[1]+'/checks/'+check+'/evidence'
    other=s.expect(s.http(s.API,otherbase,token=tokens[1]),200)['binding']
    foreign={**decision,'bindingId':other['bindingId'],'claimDigest':other['claimDigest']}
    s.expect(s.http(s.API,otherbase+'/'+upload['uploadId']+'/review','POST',foreign,tokens[1]),404)
    empty=s.expect(s.http(s.API,'/v1/employer/requests/'+requests[1]+'/evidence-eligibility','POST',{**scope,'manifestId':prepared['manifestId']},tokens[1]),200)
    if empty.get('evidenceReady') is not False or empty.get('eligible') is not False or empty.get('reason')!='REQUIRED_EVIDENCE_MISSING':raise RuntimeError('cross-request adoption')
    PHASE='positive_legacy_signing_excluded'
    for kind in ('FULL_MATCH','PARTIAL_MATCH'):
        error=s.expect(s.http(s.API,'/v1/employer/requests/'+requests[0]+'/attest','POST',{**scope,'response_type':kind},tokens[1]),409)
        if error.get('error')!='evidence_backed_signing_not_enabled':raise RuntimeError('signing not excluded')
    PHASE='binding_revocation_denial'
    op("SELECT public.revoke_request_evidence_binding(:'binding'::uuid,:'actor',:'basis');",{'binding':state['bindings'][0]})
    state['bindings'][0]=None;persist(state)
    revoked=s.expect(s.http(s.API,endpoint,'POST',scope,tokens[1]),200)
    if revoked.get('eligible') is not False or revoked.get('evidenceReady') is not False or revoked.get('reason')!='BINDING_REVOKED':raise RuntimeError('revocation ignored')
    return {'normal_two_account_login_exchange':'PASS','immutable_context_acknowledgement':'PASS','no_self_review':'PASS',
        'canonical_issuer':'https://cvera.app','manifest_idempotency':'PASS','pending_scan_visible':'PASS',
        'authority_unavailable_ineligible':'PASS','cross_request_upload_review_manifest_reuse_denied':'PASS',
        'legacy_positive_signing_excluded':'PASS','binding_revocation':'PASS','objects_uploaded':1,
        'synthetic_requests':2,'new_identities':0,'new_attestations':0,'credentials_urls_or_contents_printed':False}


def main():
    global PHASE,RECORD,FAILURE_DIAGNOSTIC
    if os.geteuid()!=0 or len(sys.argv)!=3 or sys.argv[1]!='--expected-main' or not re.fullmatch('[0-9a-f]{40}',sys.argv[2]):raise RuntimeError('owner console and deployed revision required')
    revision=sys.argv[2];os.umask(0o077)
    if s.command(['git','-C',str(s.ROOT),'rev-parse','HEAD']).decode().strip()!=revision or s.command(['git','-C',str(s.ROOT),'status','--porcelain']).strip() or s.expect(s.http(s.API,'/version'),200).get('git_sha')!=revision:raise RuntimeError('wrong revision')
    s.expect(s.http(s.API,'/healthz'),200)
    container=json.loads(s.command(['docker','inspect','evt-api-go-api-1']))[0]
    settings=dict(x.split('=',1) for x in container['Config']['Env'] if '=' in x)
    sessions=[];batch=uuid.uuid4().hex[:20]
    state={'main':revision,'batch':batch,'requests':[str(uuid.uuid4()),str(uuid.uuid4())],
        'cases':['binding-'+batch+'-a','binding-'+batch+'-b'],'bindings':[],'grants':[],'phase':'intent'}
    variables={'request0':state['requests'][0],'request1':state['requests'][1],'employer':'binding-'+batch,
        'check':'smoke.file','actor':'owner-approved-binding-release','basis':'Synthetic binding acceptance '+revision,
        'snapshot':json.dumps({'release_fixture':revision,'test_batch':batch,'title':'Synthetic fixture only; no employment assertion'})}
    assignment=None;app_attempted=False
    try:
        PHASE='normal_uploader_login';uploader=r.login('Uploader',sessions)
        PHASE='normal_hr_reviewer_login';reviewer=r.login('Independent HR reviewer',sessions)
        if uploader['principal']==reviewer['principal']:raise RuntimeError('independent principals required')
        variables.update(subject=uploader['principal'],reviewer=reviewer['principal'])
        PHASE='current_hr_role_preflight'
        allowed=s.sql("SELECT EXISTS(SELECT 1 FROM public.domain_users WHERE id=:'reviewer'::uuid AND role='hr_reviewer' AND status='active');",variables)
        if allowed!='t':raise RuntimeError('existing HR role required; do not change roles')
        absent=json.loads(s.sql("SELECT jsonb_build_object('request0',NOT EXISTS(SELECT 1 FROM public.evt_requests WHERE request_id=:'request0'::uuid),'request1',NOT EXISTS(SELECT 1 FROM public.evt_requests WHERE request_id=:'request1'::uuid),'employer',NOT EXISTS(SELECT 1 FROM public.evt_employer_config WHERE employer_id=:'employer'));",variables))
        if not all(absent.values()):raise RuntimeError('synthetic identifiers already exist')
        RECORD=s.ROOT.parent/('request-binding-smoke-'+batch+'.json');persist(state)
        PHASE='explicit_synthetic_request_fixture';app_attempted=True
        fixture_sql(variables);state['phase']='app_fixture_committed';persist(state)
        assignment=hr_grant(variables);state['assignment']=assignment;persist(state)
        result=exercise(variables,uploader,reviewer,settings,state)
    except Exception:
        FAILURE_DIAGNOSTIC={'http':s.HTTP_DIAGNOSTIC.copy(),'command':s.COMMAND_DIAGNOSTIC.copy()}
        state.update(failed_phase=PHASE,failure_diagnostic=FAILURE_DIAGNOSTIC)
        if RECORD is not None:persist(state)
        raise
    finally:
        failed=[]
        for binding in state['bindings']:
            if binding:
                try:s.sql("SELECT public.revoke_request_evidence_binding(:'binding'::uuid,:'actor',:'basis');",{**variables,'binding':binding},True)
                except Exception:failed.append('binding')
        for function,grant in state['grants']:
            try:s.sql('SELECT public.'+function+"(:'grant'::uuid,:'actor',:'basis');",{**variables,'grant':grant},True)
            except Exception:failed.append('grant')
        for i in range(len(state['bindings'])):
            try:s.sql("SELECT public.set_evidence_resource_state(:'case',NULL,'CLOSED',:'actor',:'basis');",{**variables,'case':state['cases'][i]},True)
            except Exception:failed.append('case')
        if app_attempted:
            for request in state['requests']:
                try:
                    owned=s.sql("SELECT EXISTS(SELECT 1 FROM public.evt_requests WHERE request_id=:'request'::uuid AND candidate_personid=:'subject'::uuid AND employer_id=:'employer' AND claim_snapshot=:'snapshot'::jsonb);",{**variables,'request':request})
                    if owned=='t' and assignment:
                        rejected=s.expect(s.http(s.API,'/v1/employer/requests/'+request+'/attest','POST',{'employer_id':variables['employer'],'response_type':'REJECTED_POLICY'},reviewer['access']),200)
                        if rejected.get('status')!='REJECTED' or rejected.get('attestation',{}).get('present') is not False:raise RuntimeError('unsigned terminal cleanup failed')
                    elif owned=='t':failed.append('request_requires_owner_cleanup')
                except Exception:failed.append('unsigned_request_cleanup')
        if assignment:
            try:hr_revoke(variables,assignment)
            except Exception:failed.append('hr_assignment')
        for session in sessions:
            actions=[]
            if session.get('refresh'):actions.append(lambda item=session:s.expect(s.http(s.API,'/auth/logout','POST',{'refresh_token':item['refresh']}),200))
            actions.append(lambda item=session:s.expect(s.http(s.AUTH,'/self-service/logout/api','DELETE',{'session_token':item['native']}),204))
            for action in actions:
                try:action()
                except Exception:failed.append('session')
        if failed:PHASE='cleanup_failed:'+','.join(failed);raise RuntimeError('keep guarded API; inspect private fixture')
    PHASE='cleanup_and_recovery_verification'
    summary=json.loads(s.sql("SELECT jsonb_build_object('requests_terminal',(SELECT count(*) FROM public.evt_requests WHERE request_id IN (:'request0'::uuid,:'request1'::uuid) AND status::text='REJECTED' AND coalesce(btrim(attestation_jws),'')='')=2,'bindings_revoked',NOT EXISTS(SELECT 1 FROM public.request_case_bindings WHERE request_id IN (:'request0'::uuid,:'request1'::uuid) AND revoked_at IS NULL),'cases_closed',(SELECT count(*) FROM public.cases WHERE case_id IN (:'case0',:'case1') AND status='CLOSED')=2,'grants_revoked',NOT EXISTS(SELECT 1 FROM public.case_evidence_upload_grants WHERE case_id IN (:'case0',:'case1') AND revoked_at IS NULL) AND NOT EXISTS(SELECT 1 FROM public.case_evidence_review_grants WHERE case_id IN (:'case0',:'case1') AND revoked_at IS NULL),'hr_assignment_revoked',EXISTS(SELECT 1 FROM public.employer_hr_assignments WHERE assignment_id=:'assignment'::uuid AND revoked_at IS NOT NULL),'legacy_image_rollback_safe',NOT EXISTS(SELECT 1 FROM public.request_case_bindings b JOIN public.evt_requests r USING(request_id) WHERE r.status::text='ATTESTATION_PENDING'));",{**variables,'case0':state['cases'][0],'case1':state['cases'][1],'assignment':assignment}))
    if not all(summary.values()):raise RuntimeError('terminal cleanup/recovery invariant failed')
    result.update(summary);result.update(main=revision,smoke='PASS',fixture_grants_revoked_cases_closed=True)
    state.update(phase='PASS',result=result);persist(state)
    result['private_record']=str(RECORD);print(json.dumps(result))


if __name__=='__main__':
    try:main()
    except Exception:
        print(json.dumps({'smoke':'STOP','phase':PHASE,'private_record':str(RECORD) if RECORD else None,'failure_diagnostic':FAILURE_DIAGNOSTIC,
            'instruction':'Return only this status. Do not print credentials/contents/URLs, retry provisioning, enable signing or roll back to an older API while bound pending requests remain.'}),file=sys.stderr);sys.exit(1)
