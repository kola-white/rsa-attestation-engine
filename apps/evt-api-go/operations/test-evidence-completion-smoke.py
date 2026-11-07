#!/usr/bin/env python3
"""Offline acceptance-tool controller tests. No network, accounts or production SQL."""
import base64,contextlib,hashlib,io,json,runpy,tempfile,uuid
from pathlib import Path
from unittest.mock import patch
spec=runpy.run_path(str(Path(__file__).with_name('smoke-evidence-completion.py')));main=spec['main'];g=main.__globals__
for scenario in ('success','login_failure','wrong_hash','dedup_failure','public_object','storage_failure'):
 with tempfile.TemporaryDirectory() as folder:
  root=Path(folder)/'repo';root.mkdir();g['ROOT']=root;g['PHASE']='preflight';g['HTTP_DIAGNOSTIC']={};g['COMMAND_DIAGNOSTIC']={};g['FAILURE_DIAGNOSTIC']={}
  sha='a'*40;principal=str(uuid.uuid4());grant=str(uuid.uuid4());native='PRIVATE_NATIVE_TOKEN';access='PRIVATE_ACCESS_TOKEN';refresh='PRIVATE_REFRESH_TOKEN'
  state={'created':False,'grant':False,'closed':False,'logout':False,'native_logout':False,'sessions':[],'accepted':False,'case':None}
  data=base64.b64decode('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+a5XkAAAAASUVORK5CYII=');digest=hashlib.sha256(data).hexdigest()
  def command(args,data=None):
   if args[0]=='git':return (sha+'\n').encode() if args[-2:]==['rev-parse','HEAD'] else b''
   if args[:2]==['docker','inspect']:return json.dumps([{'Config':{'Env':['DO_ENDPOINT=https://storage.invalid','EVT_S3_BUCKET=test-bucket']}}]).encode()
   raise AssertionError('unexpected command')
  def sql(query,variables=None,operator=False):
   if 'provision_evidence_case' in query:
    assert operator and variables['principal']==principal;state['created']=True;state['case']=variables['caseid'];return ''
   if 'grant_evidence_upload' in query:assert operator;state['grant']=True;return grant
   if 'revoke_evidence_upload' in query:assert operator;state['grant']=False;return ''
   if 'set_evidence_resource_state' in query:assert operator;state['closed']=True;return ''
   if 'jsonb_build_object' in query:
    assert not operator
    return json.dumps({'sealed':'sealed-evidence/'+str(uuid.uuid4()),'hash':digest,'state':'RECEIVED','scan':'PENDING','owned':True})
   raise AssertionError('unexpected SQL')
  def http(origin,path,method='GET',body=None,token=None,kratos=None):
   if path=='/version':return 200,{'git_sha':sha}
   if path=='/healthz':return 200,{'ok':'true'}
   if path=='/self-service/login/api':return 200,{'id':str(uuid.uuid4())}
   if path.startswith('/self-service/login?'):
    if scenario=='login_failure':return 401,None
    return 200,{'session_token':native}
   if path=='/sessions/whoami':return 200,{'active':True,'identity':{'id':str(uuid.uuid4())}}
   if path=='/auth/exchange':return 200,{'user':{'id':principal},'access_token':access,'refresh_token':refresh}
   if path=='/auth/logout':state['logout']=True;return 200,{}
   if path=='/self-service/logout/api':state['native_logout']=True;return 204,None
   if path.endswith('evidence:init') or path.endswith('/init'):
    if not token:return 401,None
    if not state['grant']:return 404,{}
    session,upload=str(uuid.uuid4()),str(uuid.uuid4());key='cases/'+state['case']+'/checks/smoke.file/evidence/'+upload+'/synthetic-release.png'
    result={'uploadSessionId':session,'uploads':[{'uploadId':upload,'storageKey':key,'url':'https://storage.invalid/test-bucket/'+key+'?X-Amz-Expires=600','headers':{'Content-Type':'image/png','x-amz-acl':'private'}}]};state['sessions'].append(result);return 200,result
   if path.endswith('evidence:complete') or path.endswith('/commit'):
    if not token:return 401,None
    if 'not-the-check' in path or not state['grant']:return 404,{}
    if body['uploadSessionId']==state['sessions'][1]['uploadSessionId']:return (200,{}) if scenario=='dedup_failure' else (409,{})
    state['accepted']=True
    return 200,{'evidence':[{'status':'RECEIVED','scanStatus':'PENDING','evidence_hash':('0'*64 if scenario=='wrong_hash' else digest)}]}
   raise AssertionError('unexpected HTTP route')
  def object_request(url,method,body=None,headers=None):
   if method=='PUT':return (500,b'PRIVATE_BODY') if scenario=='storage_failure' else (200,b'')
   return (200,b'PRIVATE_BODY') if scenario=='public_object' else (403,b'PRIVATE_BODY')
  g.update(command=command,sql=sql,http=http,object_request=object_request)
  out=io.StringIO();failed=False
  with patch('os.geteuid',return_value=0),patch('sys.argv',['smoke','--expected-main',sha]),patch('getpass.getpass',side_effect=['PRIVATE_EMAIL','PRIVATE_PASSWORD']),contextlib.redirect_stdout(out):
   try:main()
   except Exception:failed=True
  if scenario=='success':assert not failed and json.loads(out.getvalue())['smoke']=='PASS'
  else:assert failed,scenario
  if state['created']:assert state['closed'] and not state['grant'] and state['logout'] and state['native_logout'],scenario
  if scenario=='login_failure':assert not state['created']
  for private in ('PRIVATE_EMAIL','PRIVATE_PASSWORD',native,access,refresh,'PRIVATE_BODY','https://storage.invalid','sealed-evidence/'):
   assert private not in out.getvalue(),scenario
print(json.dumps({'offline_smoke_controller':'PASS','scenarios':6,'failure_cleanup':True,'private_output_absent':True}))
