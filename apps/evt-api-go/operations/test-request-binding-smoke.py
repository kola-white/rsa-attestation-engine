#!/usr/bin/env python3
"""Offline smoke client contract checks, with synthetic state and no network."""
import base64,hashlib,importlib.util,json,tempfile,uuid
from pathlib import Path
path=Path(__file__).with_name('smoke-request-evidence-binding.py')
spec=importlib.util.spec_from_file_location('binding_smoke',path);m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
uid=lambda:str(uuid.uuid4())
subject,reviewer,upload,session=uid(),uid(),uid(),uid()
cases=['fixture-a','fixture-b'];requests=[uid(),uid()]
u={'principal':subject,'access':'PRIVATE_SYNTHETIC_UPLOADER'};v={'principal':reviewer,'access':'PRIVATE_SYNTHETIC_REVIEWER'}
variables={'subject':subject,'reviewer':reviewer,'employer':'fixture-employer','check':'smoke.file','actor':'fixture','basis':'local synthetic only'}
data=base64.b64decode('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+a5XkAAAAASUVORK5CYII=')
class Contract:
 def __init__(self,broken=None):self.bindings={};self.revoked=False;self.receipt=None;self.broken=broken;self.signing_calls=0
 def sql(self,body,values=None,operator=False):
  assert operator
  if 'bind_request_evidence(' in body:
   b=uid();self.bindings[values['case']]=b;return b
  if 'revoke_request_evidence_binding' in body:self.revoked=True;return ''
  return uid() if 'grant_evidence_' in body else ''
 def http(self,origin,path,method='GET',body=None,token=None,**kw):
  if path=='/v1/evidence/init':return 200,{'uploadSessionId':session,'uploads':[{'uploadId':upload,'storageKey':'FIXTURE_ONLY','url':'https://private.invalid/SIGNED_URL','headers':{}}]}
  if path=='/v1/evidence/commit':return 200,{'evidence':[{'evidence_hash':hashlib.sha256(data).hexdigest(),'scanStatus':'PENDING'}]}
  if path.endswith('/evidence-eligibility'):
   if token is None:return 401,None
   if token==u['access']:return 404,None
   if requests[1] in path:return 200,{'eligible':False,'evidenceReady':False,'reason':'REQUIRED_EVIDENCE_MISSING'}
   if self.revoked:return 200,{'eligible':False,'evidenceReady':False,'reason':'BINDING_REVOKED'}
   return 200,{'eligible':self.broken=='authority-bypass','evidenceReady':True,'signingEnabled':False,'reason':'AUTHORITY_SOURCE_UNAVAILABLE','scanStatuses':['PENDING'],'pendingScanCount':1,'manualPolicy':'MANUAL_REVIEW_PILOT_V1','manifestId':'fixture-manifest'}
  if path.endswith('/attest'):
   self.signing_calls+=1;return 409,{'error':'evidence_backed_signing_not_enabled'}
  case=cases[1] if cases[1] in path else cases[0]
  if path.endswith('/review'):
   if case==cases[1]:return 404,None
   if 'bindingId' not in body:return 400,None
   if token==u['access']:return 403,None
   if self.receipt is None:self.receipt={'reviewerId':reviewer,'reviewId':uid()}
   return 200,self.receipt
  context={'bindingId':self.bindings[case],'requestId':requests[cases.index(case)],'subjectId':subject,'issuerId':'https://cvera.app','employerId':'fixture-employer','contextValid':True,'claimDigest':'d'*64}
  if self.broken=='wrong-context':context['requestId']=uid()
  return 200,{'binding':context,'evidence':[]}
for broken in (None,'wrong-context','authority-bypass'):
 with tempfile.TemporaryDirectory() as folder:
  m.RECORD=Path(folder)/'private.json';state={'cases':cases,'requests':requests,'bindings':[],'grants':[]}
  c=Contract(broken);m.s.sql=c.sql;m.s.http=c.http;m.s.storage_url=lambda *a:None;m.s.object_request=lambda *a:(204,b'')
  failed=False
  try:result=m.exercise(variables,u,v,{},state)
  except RuntimeError:failed=True
  if broken:assert failed and c.signing_calls==0
  else:
   assert not failed and result['new_attestations']==0 and c.signing_calls==2 and result['binding_revocation']=='PASS'
   assert result['cross_request_upload_review_manifest_reuse_denied']=='PASS'
  stored=m.RECORD.read_text();assert u['access'] not in stored and v['access'] not in stored and 'SIGNED_URL' not in stored
print(json.dumps({'offline_binding_smoke':'PASS','context_and_authority_fail_closed':True,'two_persisted_request_contract':True,'manifest_revocation_signing_exclusion':True,'no_credentials_in_receipt':True,'production_accessed':False}))
