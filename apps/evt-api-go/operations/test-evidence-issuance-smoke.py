#!/usr/bin/env python3
"""Offline acceptance-client regressions with a temporary LOCAL RSA key."""
import base64,hashlib,importlib.util,json,subprocess,tempfile,time,uuid
from pathlib import Path
from unittest.mock import patch
HERE=Path(__file__).resolve().parent
spec=importlib.util.spec_from_file_location('live_smoke',HERE/'smoke-evidence-issuance.py');m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
uid=lambda:str(uuid.uuid4());b64=lambda x:base64.urlsafe_b64encode(x).rstrip(b'=').decode()
def run(args,data=None):
 r=subprocess.run(args,input=data,capture_output=True,timeout=30)
 if r.returncode:raise RuntimeError('local fixture command failed')
 return r.stdout
with tempfile.TemporaryDirectory(prefix='issuance-smoke-local-') as work:
 folder=Path(work);key=folder/'LOCAL-ONLY.pem';public=folder/'public.pem'
 run(['openssl','genpkey','-algorithm','RSA','-pkeyopt','rsa_keygen_bits:3072','-out',str(key)]);key.chmod(0o600)
 run(['openssl','pkey','-in',str(key),'-pubout','-out',str(public)])
 der=run(['openssl','pkey','-pubin','-in',str(public),'-outform','DER']);fp=hashlib.sha256(der).hexdigest()
 modulus=run(['openssl','rsa','-pubin','-in',str(public),'-modulus','-noout']).decode().strip().split('=')[1]
 jwks={'keys':[{'kty':'RSA','kid':m.p.KID,'alg':'RS256','use':'sig','n':b64(bytes.fromhex(modulus)),'e':'AQAB'}]}
 m.p.public_binding(public,jwks,fp)
 try:m.p.public_binding(public,jwks,'0'*64)
 except RuntimeError:pass
 else:raise AssertionError('wrong trusted public binding accepted')
 subject,reviewer=uid(),uid();request,binding,manifest,upload,session=uid(),uid(),uid(),uid(),uid();authority='LOCAL-ONLY-AUTHORITY';case='local-only-case'
 u={'principal':subject,'access':'LOCAL_PRIVATE_UPLOADER'};v={'principal':reviewer,'access':'LOCAL_PRIVATE_REVIEWER'}
 snapshot=json.dumps({'primary_employment':{'title':'Managing Member — owner'}});manifest_text=json.dumps({'local_fixture_only':True})
 commitment={'request_id':request,'binding_id':binding,'case_id':case,'subject_id':subject,'employer_id':'norstar-systems-llc','request_version':3,'snapshot_sha256':hashlib.sha256(snapshot.encode()).hexdigest(),'manifest_id':manifest,'manifest_sha256':hashlib.sha256(manifest_text.encode()).hexdigest(),'manual_policy':m.p.MANUAL,'authority_grant_id':authority+':LOCAL-ACTIVATION','authority_recording_position':5}
 claims={'iss':m.p.ISSUER,'sub':subject,'jti':uid(),'iat':int(time.time())-1,'nbf':int(time.time())-1,'exp':int(time.time())+600,'vct':m.p.ISSUER+'/credentials/employment-role/v1','claim_type':'employment.role','employment':{'employer_id':'norstar-systems-llc','title':'Managing Member — owner'},'verification_policy':m.p.POLICY,'credential_spki_sha256':fp,'evidence_binding':commitment}
 message=(b64(json.dumps({'alg':'RS256','kid':m.p.KID,'typ':'dc+sd-jwt'}).encode())+'.'+b64(json.dumps(claims).encode())).encode()
 raw=message.decode()+'.'+b64(run(['openssl','dgst','-sha256','-sign',str(key)],message))+'~'
 evidence=folder/'LOCAL-ONLY-EVIDENCE.pdf';data=b'%PDF-1.4 LOCAL FIXTURE ONLY';evidence.write_bytes(data)
 c={'actor_id':'local-only','basis_ref':'not genuine evidence','case_id':case,'claim_snapshot':json.loads(snapshot),'authorization_id':authority,'evidence':[{'check_id':'employment.role','path':str(evidence),'mime_type':'application/pdf','sha256':hashlib.sha256(data).hexdigest()}],'revoke_claim_after_verification':False}
 actual_signature=m.p.verify_signature
 class Contract:
  def __init__(self,broken=None):self.broken=broken;self.calls=0;self.revoked=False
  def sql(self,body,values=None,operator=False):
   if 'revoke_bound_credential' in body:self.revoked=True;return ''
   if "dispatch_mode='AUTO'" in body and 'RETURNING' in body:return 'norstar-systems-llc'
   if 'SELECT dispatch_mode' in body:return 'MANUAL'
   if 'bind_request_evidence' in body:return binding
   if 'grant_evidence_' in body:return uid()
   if 'SELECT assignment_id' in body:return uid()
   if 'SELECT count(*)' in body:return '0'
   if "'snapshot',r.claim_snapshot" in body:return json.dumps({'snapshot':snapshot,'manifest':manifest_text,'request_version':3})
   return ''
  def http(self,origin,path,method='GET',body=None,token=None,**kw):
   if origin==m.s.JWKS:return 200,jwks
   if path=='/v1/requests':return 201,{'request_id':request,'status':'DRAFT'}
   if path.endswith('/submit'):return 200,{'status':'ATTESTATION_PENDING'}
   if path=='/v1/evidence/init':return 200,{'uploadSessionId':session,'uploads':[{'uploadId':upload,'storageKey':'LOCAL-ONLY','url':'https://local.invalid/PRIVATE_URL','headers':{}}]}
   if path=='/v1/evidence/commit':return 200,{'evidence':[{'evidence_hash':hashlib.sha256(data).hexdigest(),'scanStatus':'PENDING'}]}
   if path.endswith('/evidence-eligibility'):return 200,{'eligible':self.broken!='authority','evidenceReady':True,'signingEnabled':True,'authorityDecision':'AUTHORIZED','authorityGrantId':authority+':LOCAL-ACTIVATION','manualPolicy':m.p.MANUAL,'scanStatuses':['PENDING'],'manifestId':manifest}
   if path.endswith('/issue-evidence'):
    if token is None:return 401,None
    if token==u['access']:return 403,None
    self.calls+=1
    if self.broken=='ambiguous-issue':raise RuntimeError('LOCAL simulated ambiguous HTTP outcome')
    return 200,{'credentialId':claims['jti'],'credential':raw}
   if path.endswith('/credential'):return (200,{'credential':raw}) if token==u['access'] else (403,None)
   if path=='/v1/verification/outcome':
    trusted=not self.revoked and body['jwsCompact']==raw and body['expectedSubject']==subject and body['policy']==m.p.POLICY
    return 200,{'signature':'verified','trust':'trusted' if trusted else 'untrusted','verification':{'decision':'VERIFIED' if trusted else 'REJECTED','signature':'valid','validity':'valid','status':'valid','manifest_integrity':'valid','evidence_review':'valid','authority':{'Decision':'AUTHORIZED'},'scan_policy':m.p.MANUAL,'scan_statuses':['PENDING'],'commitment':commitment}}
   if path.endswith('/review'):return 200,{'reviewerId':reviewer}
   return 200,{'binding':{'requestId':uid() if self.broken=='context' else request,'bindingId':binding,'subjectId':subject,'issuerId':m.p.ISSUER,'employerId':'norstar-systems-llc','contextValid':True,'claimDigest':commitment['snapshot_sha256']}}
 for broken in (None,'context','authority','ambiguous-issue','revoke','review-rejection'):
  c['revoke_claim_after_verification']=broken=='revoke'
  target=folder/str(broken);target.mkdir();m.RECORD=target/'state.json';m.STATE={'grants':[],'uploads':[],'issuance_attempted':False};contract=Contract(broken)
  def signature(token,ignored_public,keys,expected):return actual_signature(token,public,keys,expected,fp)
  with patch.object(m.s,'http',contract.http),patch.object(m.s,'sql',contract.sql),patch.object(m.s,'storage_url',lambda *a:None),patch.object(m.s,'object_request',lambda *a:(204,b'')),patch.object(m.r,'download',lambda *a:data),patch.object(m.getpass,'getpass',side_effect=['REJECTED' if broken=='review-rejection' else 'APPROVED','Actual LOCAL fixture decision only']),patch.object(m.p,'verify_signature',signature):
   failed=False
   try:result=m.exercise(c,u,v,v,{},target)
   except RuntimeError:failed=True
   if broken and broken!='revoke':
    assert failed
    assert contract.calls==(1 if broken=='ambiguous-issue' else 0)
    if broken=='review-rejection':assert m.STATE['review_rejected'] is True
   else:assert not failed and result['new_attestations']==1 and contract.calls==1
   if m.STATE['issuance_attempted']:
    try:m.once_issue('/must-not-retry',{},v['access'])
    except RuntimeError:pass
    else:raise AssertionError('second signing attempt allowed')
  stored=m.RECORD.read_text();assert all(secret not in stored for secret in (u['access'],v['access'],raw,'PRIVATE_URL','LOCAL-ONLY-EVIDENCE'))
 # The independent verdict must contain ALL trust dimensions.
 for missing in ('manifest_integrity','evidence_review','status','authority'):
  response=Contract().http(m.s.API,'/v1/verification/outcome','POST',{'jwsCompact':raw,'expectedSubject':subject,'policy':m.p.POLICY})[1]
  response['verification'].pop(missing)
  try:m.p.verified(response)
  except RuntimeError:pass
  else:raise AssertionError('signature-only trust accepted')
print(json.dumps({'offline_issuance_smoke':'PASS','temporary_local_RSA_3072_signature':True,'canonical_context_and_authority_denial_before_signing':True,'one_issuance_attempt_even_on_ambiguous_outcome':True,'manifest_status_review_policy_required':True,'no_credential_or_content_in_receipt':True,'production_accessed':False}))
