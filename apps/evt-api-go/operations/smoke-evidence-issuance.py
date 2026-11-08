#!/usr/bin/env python3
"""PREPARED ONLY: one separately approved genuine pilot, never synthetic authority.

Normal login/create/submit/upload/review/issue/retrieve/verify; reviewed operator
functions only for exact case/grants/HR assignment and bounded dispatch window.
Another HUMAN must personally review the evidence and context before approval.
No schema, authority, key/credential activation, account/role changes or deployment.
"""
import argparse,getpass,hashlib,importlib.util,json,os,re,sys,uuid
from pathlib import Path
HERE=Path(__file__).resolve().parent

def load(name,file):
 spec=importlib.util.spec_from_file_location(name,HERE/file);m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m);return m
r=load('review_acceptance','smoke-evidence-review.py');s=r.s
p=load('issuance_acceptance','issuance-acceptance-support.py')
PHASE='preflight';STATE=None;RECORD=None

def persist():
 temp=RECORD.with_suffix('.next');data=json.dumps(STATE).encode()
 fd=os.open(temp,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW,0o600)
 with os.fdopen(fd,'wb') as stream:stream.write(data);stream.flush();os.fsync(stream.fileno())
 os.replace(temp,RECORD)

def config(path):
 import stat
 st=path.lstat()
 if stat.S_ISLNK(st.st_mode) or st.st_uid!=0 or stat.S_IMODE(st.st_mode)!=0o600:raise RuntimeError('root-private completed input required')
 c=json.loads(path.read_text())
 for k in ('approval_ref','actor_id','basis_ref','expected_main','subject_principal','reviewer_principal','hr_principal','authorization_id','authority_evidence_ref','case_id','claim_snapshot','evidence','revoke_claim_after_verification'):
  if k not in c or c[k] is None or c[k]=='':raise RuntimeError('missing actual approved input')
 if type(c['revoke_claim_after_verification']) is not bool or not re.fullmatch('[0-9a-f]{40}',c['expected_main']):raise RuntimeError('invalid approved release choices')
 for k in ('subject_principal','reviewer_principal','hr_principal'):c[k]=str(uuid.UUID(c[k]))
 if c['subject_principal']==c['reviewer_principal'] or c['subject_principal']==c['hr_principal']:raise RuntimeError('distinct existing requestor/HR roles and independent reviewer required')
 if c['claim_snapshot'].get('primary_employment',{}).get('title')!='Managing Member — owner':raise RuntimeError('truthful owner-qualified NorStar pilot required')
 if not re.fullmatch('[A-Za-z0-9_.-]{1,120}',c['case_id']) or not isinstance(c['evidence'],list) or not c['evidence']:raise RuntimeError('explicit case/check/evidence required')
 checks=[]
 for f in c['evidence']:
  if not re.fullmatch('[A-Za-z0-9_.-]{1,120}',f['check_id']) or f['check_id'] in checks:raise RuntimeError('one explicit evidence file per distinct required check')
  checks.append(f['check_id'])
  file=Path(f['path']);st=file.lstat()
  if not file.is_file() or st.st_uid!=0 or stat.S_IMODE(st.st_mode)!=0o600 or stat.S_ISLNK(st.st_mode) or not 0<st.st_size<=5*1024*1024:raise RuntimeError('protected bounded authentic evidence file required')
  if f['mime_type'] not in ('application/pdf','image/png','image/jpeg') or hashlib.sha256(file.read_bytes()).hexdigest()!=f['sha256']:raise RuntimeError('approved evidence integrity/type mismatch')
 return c

def once_issue(path,body,token):
 if STATE.get('issuance_attempted'):raise RuntimeError('never repeat an issuance attempt; reconcile committed subject artifact')
 STATE['issuance_attempted']=True;persist()
 return s.expect(s.http(s.API,path,'POST',body,token),200)

def exercise(c,uploader,reviewer,hr,settings,folder):
 global PHASE
 variables={'subject':uploader['principal'],'reviewer':reviewer['principal'],'hr':hr['principal'],'employer':'norstar-systems-llc','case':c['case_id'],'actor':c['actor_id'],'basis':c['basis_ref']}
 scope={'employer_id':variables['employer']};tokens=(uploader['access'],reviewer['access'],hr['access'])
 def op(body,extra=None):return s.sql(body,{**variables,**(extra or {})},True)
 PHASE='approved_normal_request_creation'
 draft=s.expect(s.http(s.API,'/v1/requests','POST',{**scope,'claim_snapshot':c['claim_snapshot']},tokens[0]),201)
 request=str(uuid.UUID(draft['request_id']));STATE['request']=request;persist();variables['request']=request
 PHASE='approved_bounded_dispatch_window'
 # This is the existing dispatch mechanism, not a direct request-status patch.
 # Restore MANUAL even if the normal submit fails; never alter another request.
 STATE['dispatch_window_intent']=True;persist()
 try:
  opened=s.sql("UPDATE public.evt_employer_config SET dispatch_mode='AUTO' WHERE employer_id=:'employer' AND dispatch_mode='MANUAL' RETURNING employer_id;",variables)
  if opened!=variables['employer']:raise RuntimeError('dispatch prerequisite changed')
  STATE['dispatch_window_opened']=True;persist()
  submitted=s.expect(s.http(s.API,'/v1/requests/'+request+'/submit','POST',{},tokens[0]),200)
  if submitted.get('status')!='ATTESTATION_PENDING':raise RuntimeError('normal dispatch failed')
 finally:
  s.sql("UPDATE public.evt_employer_config SET dispatch_mode='MANUAL' WHERE employer_id=:'employer' AND dispatch_mode='AUTO';",variables)
  restored=s.sql("SELECT dispatch_mode FROM public.evt_employer_config WHERE employer_id=:'employer';",variables)
  if restored!='MANUAL':raise RuntimeError('dispatch restoration requires inspection')
  STATE['dispatch_window_closed']=True;persist()
 PHASE='approved_operator_binding_and_exact_grants'
 checks=[f['check_id'] for f in c['evidence']]
 op("SELECT public.provision_evidence_case(:'case',:'actor',:'basis');")
 for check in checks:op("SELECT public.provision_evidence_check(:'case',:'check',:'actor',:'basis');",{'check':check})
 binding=str(uuid.UUID(op("SELECT public.bind_request_evidence(:'request'::uuid,:'case',ARRAY(SELECT jsonb_array_elements_text(:'checks'::jsonb)),:'actor',:'basis');",{'checks':json.dumps(checks)})))
 STATE['binding']=binding;persist()
 for function,principal in [('grant_evidence_upload',uploader['principal']),('grant_evidence_review',reviewer['principal'])]:
  grant=str(uuid.UUID(op('SELECT public.'+function+"(:'principal'::uuid,:'case',:'actor',:'basis');",{'principal':principal})))
  STATE['grants'].append(['revoke_evidence_upload' if function=='grant_evidence_upload' else 'revoke_evidence_review',grant]);persist()
 # Reuse an established assignment; never infer it from a review grant or role.
 assignment=s.sql("SELECT assignment_id FROM public.employer_hr_assignments WHERE principal_id=:'hr'::uuid AND employer_id=:'employer' AND revoked_at IS NULL;",variables)
 if not assignment:
  assignment=s.sql("SET LOCAL ROLE cvera_hr_operator;INSERT INTO public.employer_hr_assignments(assignment_id,principal_id,employer_id,granted_by,grant_basis_ref) VALUES(gen_random_uuid(),:'hr'::uuid,:'employer',:'actor',:'basis') RETURNING assignment_id;",variables)
  STATE['new_hr_assignment']=str(uuid.UUID(assignment));persist()
 PHASE='genuine_evidence_submission_and_independent_review'
 for f in c['evidence']:
  data=Path(f['path']).read_bytes();check=f['check_id'];body={'caseId':c['case_id'],'checkId':check,'files':[{'name':Path(f['path']).name,'size':len(data),'mimeType':f['mime_type']}]}
  initialized=s.expect(s.http(s.API,'/v1/evidence/init','POST',body,tokens[0]),200);upload=initialized['uploads'][0]
  STATE['uploads'].append(str(uuid.UUID(upload['uploadId'])));persist()
  s.storage_url(upload['url'],upload['storageKey'],settings)
  if s.object_request(upload['url'],'PUT',data,upload['headers'])[0] not in (200,204):raise RuntimeError('private upload failed')
  completion=s.expect(s.http(s.API,'/v1/evidence/commit','POST',{'uploadSessionId':initialized['uploadSessionId'],'uploads':[{'uploadId':upload['uploadId'],'size':len(data),'mimeType':f['mime_type']}]},tokens[0]),200)['evidence']
  if len(completion)!=1 or completion[0].get('evidence_hash')!=f['sha256'] or completion[0].get('scanStatus')!='PENDING':raise RuntimeError('canonical hash/scan boundary mismatch')
  base='/v1/cases/'+c['case_id']+'/checks/'+check+'/evidence';listing=s.expect(s.http(s.API,base,token=tokens[1]),200);context=listing['binding']
  if context.get('requestId')!=request or context.get('bindingId')!=binding or context.get('subjectId')!=uploader['principal'] or context.get('issuerId')!=p.ISSUER or context.get('employerId')!=variables['employer'] or context.get('contextValid') is not True:raise RuntimeError('review context mismatch')
  canonical=r.download(base+'/'+upload['uploadId']+'/download',tokens[1])
  if hashlib.sha256(canonical).hexdigest()!=f['sha256']:raise RuntimeError('reviewed bytes differ from genuine evidence')
  # No automatic approval: the OTHER person must inspect canonical evidence and
  # immutable context through their authorized client/private inspection files.
  inspection=folder/('review-'+str(len(STATE['uploads'])))
  inspection.mkdir(mode=0o700);private_file(inspection/'canonical-evidence',canonical);private_file(inspection/'context.json',json.dumps(context).encode())
  print('Independent reviewer: personally inspect the canonical evidence and request context in '+str(inspection)+'. No contents are printed.',flush=True)
  decision=getpass.getpass('Independent reviewer decision after personal inspection (APPROVED/REJECTED; hidden): ')
  if decision not in ('APPROVED','REJECTED'):raise RuntimeError('explicit actual review decision required')
  reason=getpass.getpass('Independent reviewer actual rationale (hidden): ').strip()
  if not reason:raise RuntimeError('attributable review reason required')
  s.expect(s.http(s.API,base+'/'+upload['uploadId']+'/review','POST',{'decision':decision,'reason':reason,'bindingId':binding,'claimDigest':context['claimDigest']},tokens[1]),200)
  if decision=='REJECTED':
   STATE['review_rejected']=True;persist();raise RuntimeError('genuine review rejected; stop without signing')
 PHASE='current_eligibility_and_exact_signing_scope'
 endpoint='/v1/employer/requests/'+request;prepared=s.expect(s.http(s.API,endpoint+'/evidence-eligibility','POST',scope,tokens[2]),200)
 if prepared.get('eligible') is not True or prepared.get('evidenceReady') is not True or prepared.get('signingEnabled') is not True or prepared.get('authorityDecision')!='AUTHORIZED' or not prepared.get('authorityGrantId','').startswith(c['authorization_id']+':') or prepared.get('manualPolicy')!=p.MANUAL or prepared.get('scanStatuses')!=['PENDING']:raise RuntimeError('current authority/evidence/credential not eligible')
 pending=s.sql("SELECT count(*) FROM public.evt_requests WHERE employer_id=:'employer' AND status='ATTESTATION_PENDING' AND request_id<>:'request'::uuid;",variables)
 if pending!='0':raise RuntimeError('unexpected other pending request; no signing')
 s.expect(s.http(s.API,endpoint+'/issue-evidence','POST',scope),401)
 s.expect(s.http(s.API,endpoint+'/issue-evidence','POST',scope,tokens[0]),403)
 # Exactly one positive call, protected by durable intent BEFORE network I/O.
 PHASE='one_controlled_signing';issued=once_issue(endpoint+'/issue-evidence',scope,tokens[2]);STATE['credential_id']=str(uuid.UUID(issued['credentialId']));persist()
 artifact=s.expect(s.http(s.API,'/v1/requests/'+request+'/credential',token=tokens[0]),200)['credential']
 if artifact!=issued['credential']:raise RuntimeError('portable subject artifact differs from committed issuance')
 private_file(folder/'credential.sd-jwt',artifact.encode())
 if s.http(s.API,'/v1/requests/'+request+'/credential',token=tokens[1])[0] not in (403,404):raise RuntimeError('cross-subject credential disclosure')
 PHASE='independent_public_signature_and_trusted_manifest_status_verification'
 manifest=json.loads(s.sql("SELECT jsonb_build_object('snapshot',r.claim_snapshot::text,'manifest',m.manifest::text,'request_version',b.request_version) FROM public.evt_requests r JOIN public.request_case_bindings b USING(request_id) JOIN public.request_evidence_manifests m ON m.manifest_id=:'manifest'::uuid WHERE r.request_id=:'request'::uuid AND b.binding_id=:'binding'::uuid AND m.binding_id=b.binding_id;",{**variables,'binding':binding,'manifest':prepared['manifestId']}))
 expected={'subject':uploader['principal'],'employer':variables['employer'],'authorization':c['authorization_id'],'binding':{'request_id':request,'binding_id':binding,'case_id':c['case_id'],'subject_id':uploader['principal'],'employer_id':variables['employer'],'manifest_id':prepared['manifestId'],'request_version':manifest['request_version'],'snapshot_sha256':hashlib.sha256(manifest['snapshot'].encode()).hexdigest(),'manifest_sha256':hashlib.sha256(manifest['manifest'].encode()).hexdigest()}}
 jwks=s.expect(s.http(s.JWKS,''),200)
 claims=p.verify_signature(artifact,s.ROOT/'keys/public/issuer-prod-2026-10.pub.pem',jwks,expected)
 if claims.get('employment',{}).get('title')!=c['claim_snapshot']['primary_employment']['title']:raise RuntimeError('truthful role snapshot mismatch')
 def verify(token=artifact,subject=uploader['principal'],policy=p.POLICY):return s.expect(s.http(s.API,'/v1/verification/outcome','POST',{'jwsCompact':token,'expectedSubject':subject,'policy':policy},tokens[1]),200)
 verified=p.verified(verify())
 if verified.get('commitment')!=claims['evidence_binding']:raise RuntimeError('independent trusted commitment differs')
 for invalid,subject,policy in [(artifact,str(uuid.uuid4()),p.POLICY),(artifact,uploader['principal'],'unsupported-policy')]:
  if verify(invalid,subject,policy).get('trust')=='trusted':raise RuntimeError('wrong subject/policy trusted')
 # Flip one signature byte (valid serialization, invalid crypto); never re-sign.
 pieces=artifact[:-1].split('.');sig=bytearray(p.decode(pieces[2]));sig[0]^=1
 import base64
 pieces[2]=base64.urlsafe_b64encode(sig).rstrip(b'=').decode();modified='.'.join(pieces)+'~'
 if verify(modified).get('trust')=='trusted':raise RuntimeError('modified signature trusted')
 revoked=False
 if c['revoke_claim_after_verification']:
  PHASE='explicitly_approved_claim_revocation'
  s.sql("SELECT public.revoke_bound_credential(:'credential'::uuid,:'actor',:'basis');",{**variables,'credential':STATE['credential_id']})
  if verify().get('trust')=='trusted':raise RuntimeError('claim revocation ignored')
  # Cryptographic signature remains valid; current status now rejects trust.
  p.verify_signature(artifact,s.ROOT/'keys/public/issuer-prod-2026-10.pub.pem',jwks,expected);revoked=True
 return {'normal_request_upload_completion_independent_review':'PASS','canonical_issuer_employer_subject_claim':'PASS','public_key_signature_and_jwks':'PASS','immutable_snapshot_and_manifest_commitment':'PASS','current_trusted_authority_review_status_and_policy':'PASS','wrong_subject_policy_and_modified_signature_denied':'PASS','claim_revocation':'PASS' if revoked else 'NOT_EXECUTED: owner chose retention; non-revoked status verified','malware_scan_status':'PENDING','cryptographic_integrity_is_not_independent_review_fact_proof':True,'new_attestations':1,'private_credential_retained':True}

def private_file(path,data):
 fd=os.open(path,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW,0o600)
 with os.fdopen(fd,'wb') as stream:stream.write(data);stream.flush();os.fsync(stream.fileno())

def main():
 global PHASE,STATE,RECORD
 parser=argparse.ArgumentParser();parser.add_argument('--approved-live-acceptance',type=Path,required=True);args=parser.parse_args()
 if os.geteuid()!=0:raise RuntimeError('approved existing owner console required')
 with open('/dev/tty','rb') as tty:
  if not os.isatty(tty.fileno()):raise RuntimeError('hidden local terminal prompts required')
 os.umask(0o077);c=config(args.approved_live_acceptance)
 # Durable intent path is fixed to the approved private input; rerunning it
 # cannot create a second genuine request or issue a second credential.
 folder=args.approved_live_acceptance.with_suffix('.acceptance');folder.mkdir(mode=0o700)
 RECORD=folder/'state.json';STATE={'approval_ref':c['approval_ref'],'main':c['expected_main'],'grants':[],'uploads':[],'issuance_attempted':False};persist()
 if s.command(['git','-C',str(s.ROOT),'rev-parse','HEAD']).decode().strip()!=c['expected_main'] or s.command(['git','-C',str(s.ROOT),'status','--porcelain']).strip() or s.expect(s.http(s.API,'/version'),200).get('git_sha')!=c['expected_main']:raise RuntimeError('expected deployed revision required')
 s.expect(s.http(s.API,'/healthz'),200)
 settings=dict(x.split('=',1) for x in json.loads(s.command(['docker','inspect','evt-api-go-api-1']))[0]['Config']['Env'] if '=' in x)
 if settings.get('EVT_ATTESTATION_SIGNER_PEM_PATH')!='/run/secrets/issuer-prod-2026-10.pem':raise RuntimeError('unexpected configured signer path; private key never read')
 p.public_binding(s.ROOT/'keys/public/issuer-prod-2026-10.pub.pem',s.expect(s.http(s.JWKS,''),200))
 sessions=[];result=None
 try:
  PHASE='normal_subject_login';u=r.login('Subject/requestor',sessions)
  PHASE='normal_independent_reviewer_login';v=r.login('Independent human reviewer',sessions)
  h=v if c['hr_principal']==v['principal'] else r.login('Authorized HR issuer',sessions)
  if [u['principal'],v['principal'],h['principal']]!=[c['subject_principal'],c['reviewer_principal'],c['hr_principal']]:raise RuntimeError('authenticated principals differ from approved scope')
  preflight=json.loads(s.sql("SELECT jsonb_build_object('manual',EXISTS(SELECT 1 FROM public.evt_employer_config WHERE employer_id='norstar-systems-llc' AND dispatch_mode='MANUAL'),'new_case',NOT EXISTS(SELECT 1 FROM public.cases WHERE case_id=:'case'),'requestor',EXISTS(SELECT 1 FROM public.domain_users WHERE id=:'subject'::uuid AND status='active' AND role='requestor'),'hr',EXISTS(SELECT 1 FROM public.domain_users WHERE id=:'hr'::uuid AND status='active' AND role='hr_reviewer'),'authority',EXISTS(SELECT 1 FROM public.authority_authorizations a JOIN public.authority_adjudications d USING(authorization_id) WHERE a.authorization_id=:'authorization' AND a.issuer_id='https://cvera.app' AND a.authority_subject_id='norstar-systems-llc' AND a.claim_type='employment.role' AND d.outcome='APPROVED' AND d.evidence_ref=:'evidence'));",{'case':c['case_id'],'subject':u['principal'],'hr':h['principal'],'authorization':c['authorization_id'],'evidence':c['authority_evidence_ref']}))
  if not all(preflight.values()):raise RuntimeError('current approved scope/preconditions mismatch')
  result=exercise(c,u,v,h,settings,folder)
 finally:
  failed=sys.exc_info()[0] is not None;STATE['last_phase']=PHASE
  # Revoke only grants created by this run. Preserve authentic evidence,
  # request, binding, authority, credential and append-only audit history.
  for function,grant in STATE['grants']:
   try:s.sql('SELECT public.'+function+"(:'id'::uuid,:'actor',:'reason');",{'id':grant,'actor':c['actor_id'],'reason':'Pilot temporary access ended: '+c['basis_ref']},True)
   except Exception:STATE['cleanup_outstanding']=True
  if STATE.get('new_hr_assignment'):
   try:s.sql("SET LOCAL ROLE cvera_hr_operator;SELECT public.lock_employer_hr_principal(:'principal'::uuid);UPDATE public.employer_hr_assignments SET revoked_at=clock_timestamp(),revoked_by=:'actor',revocation_reason=:'reason' WHERE assignment_id=:'assignment'::uuid AND revoked_at IS NULL;",{'principal':c['hr_principal'],'assignment':STATE['new_hr_assignment'],'actor':c['actor_id'],'reason':'Pilot temporary access ended: '+c['basis_ref']})
   except Exception:STATE['cleanup_outstanding']=True
  for session in sessions:
   for origin,path,method,body,expected in [(s.API,'/auth/logout','POST',{'refresh_token':session.get('refresh')},200),(s.AUTH,'/self-service/logout/api','DELETE',{'session_token':session['native']},204)]:
    try:s.expect(s.http(origin,path,method,body),expected)
    except Exception:STATE['cleanup_outstanding']=True
  STATE['result']='STOP' if failed or STATE.get('cleanup_outstanding') else 'PASS';persist()
 if STATE['result']!='PASS':raise RuntimeError('cleanup verification incomplete')
 print(json.dumps({'issuance_acceptance':'PASS','main':c['expected_main'],**result,'credentials_urls_or_contents_printed':False,'schema_authority_or_credential_activated_by_tool':False,'private_record':str(RECORD)}))
if __name__=='__main__':
 try:main()
 except Exception:
  print(json.dumps({'issuance_acceptance':'STOP','phase':PHASE,'instruction':'Do not retry signing/provisioning or print private output; preserve state and reconcile the existing subject artifact.'}));sys.exit(1)
