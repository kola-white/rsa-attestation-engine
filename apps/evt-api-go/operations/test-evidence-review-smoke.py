#!/usr/bin/env python3
"""Offline review smoke contract regression. Synthetic fixture only, no network."""
import base64,hashlib,importlib.util,json,uuid
from pathlib import Path
p=Path(__file__).with_name('smoke-evidence-review.py')
spec=importlib.util.spec_from_file_location('review_smoke',p);m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
case='fixture-case';check='smoke.file';upload=str(uuid.uuid4());session=str(uuid.uuid4())
data=base64.b64decode('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+a5XkAAAAASUVORK5CYII=');digest=hashlib.sha256(data).hexdigest()
uploader={'principal':str(uuid.uuid4()),'access':'SYNTHETIC_UPLOADER'};reviewer={'principal':str(uuid.uuid4()),'access':'SYNTHETIC_REVIEWER'}
class Contract:
 def __init__(self,bad_hash=False):self.readers=set();self.uploaders=set();self.assignments={};self.decision=None;self.bad_hash=bad_hash
 def sql(self,body,variables,operator=False):
  assert operator and variables['caseid']==case
  if 'grant_evidence_' in body:
   gid=str(uuid.uuid4());subject=variables['principal'];kind='review' if 'grant_evidence_review' in body else 'upload'
   self.assignments[gid]=(subject,kind);(self.readers if kind=='review' else self.uploaders).add(subject);return gid
  assert 'revoke_evidence_review' in body
  subject,kind=self.assignments[variables['grantid']];assert kind=='review';self.readers.remove(subject);return ''
 def http(self,origin,path,method='GET',body=None,token=None,**kw):
  principal=uploader['principal'] if token==uploader['access'] else reviewer['principal'] if token==reviewer['access'] else None
  if not principal:return 401,None
  if path=='/v1/evidence/init':
   if principal not in self.uploaders:return 404,None
   return 200,{'uploadSessionId':session,'uploads':[{'uploadId':upload,'url':'https://storage.invalid/SYNTHETIC','storageKey':'SYNTHETIC','headers':{}}]}
  if path=='/v1/evidence/commit':return 200,{'evidence':[{'scanStatus':'PENDING','evidence_hash':'bad' if self.bad_hash else digest}]}
  if principal not in self.readers:return 404,None
  if path=='/v1/evidence/checks':return 200,{'items':[{'caseId':case,'checkId':check,'canReview':True,'canUpload':False}]}
  if 'not-the-case' in path or 'not-the-check' in path:return 404,None
  if path.endswith('/review'):
   if principal==uploader['principal']:return 403,None
   value={'reviewerId':principal,'uploadId':upload,'decision':body['decision'],'reason':body['reason'],'reviewedAt':'2026-11-08T00:00:00Z'}
   if self.decision and value!=self.decision:return 409,None
   self.decision=value;return 200,value
  return 200,{'evidence':[{'scanStatus':'PENDING','reviewStatus':'APPROVED' if self.decision else 'PENDING','evidence_hash':digest,'uploadId':upload,'reviewerId':reviewer['principal'] if self.decision else None}]}
for corrupt in (False,True):
 state=Contract(corrupt);m.s.http=state.http;m.s.sql=state.sql;m.s.storage_url=lambda *a:None;m.s.object_request=lambda *a:(204,b'');m.download=lambda *a:data
 grants=[];failed=False
 try:result=m.exercise(uploader,reviewer,case,check,{}, {'caseid':case,'actor':'fixture','basis':'synthetic'},grants)
 except RuntimeError:failed=True
 if corrupt:assert failed and state.decision is None
 else:
  assert not failed and result['scan_status']=='PENDING' and result['self_review_denied']=='PASS'
  assert len(grants)==2 and reviewer['principal'] not in state.readers and state.decision['reviewerId']==reviewer['principal']
# Reuse the tested normal active-session rule; email verification is diagnostic,
# not an invented prerequisite for existing Cvera accounts.
assert m.s.account_diagnostic((200,{'active':True,'identity':{'id':str(uuid.uuid4())}}))['session_active']
print(json.dumps({'offline_review_smoke':'PASS','two_principal_contract':True,'grant_separation_self_review':True,'attribution_retry_revocation':True,'corrupt_completion_stops_before_review':True,'production_accessed':False}))
