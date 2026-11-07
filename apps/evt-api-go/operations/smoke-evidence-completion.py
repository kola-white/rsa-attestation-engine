#!/usr/bin/env python3
"""Owner-console evidence-completion acceptance with an existing normal login.

Creates one explicitly release-authorized synthetic case/check/grant, uploads two
copies of a public synthetic PNG fixture, verifies completion/dedup/private access,
and revokes/closes the fixture. No identity, credentials, claims or config changes.
Evidence remains RECEIVED/PENDING. Secrets, object URLs and contents are never output.
"""
import base64
import hashlib
import getpass
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile
import urllib.error
import urllib.parse
import urllib.request
import uuid

M = '' # Required explicit merged/deployed main SHA via --expected-main.
ROOT = Path('/opt/cvera/rsa-attestation-engine')
API = 'https://api.cvera.app'
AUTH = 'https://auth.cvera.app'
JWKS = 'https://hapis.sfo3.cdn.digitaloceanspaces.com/trust/jwks.json'
DB = 'evt-api-go-db-1'
PHASE = 'preflight'
ACCOUNT_DIAGNOSTIC = {}
HTTP_DIAGNOSTIC = {}
COMMAND_DIAGNOSTIC = {}
FAILURE_DIAGNOSTIC = {}

class NoRedirect(urllib.request.HTTPRedirectHandler):
 def redirect_request(self,*args,**kwargs): return None

opener=urllib.request.build_opener(urllib.request.ProxyHandler({}),NoRedirect())

def http(origin,path,method='GET',body=None,token=None,kratos=None):
 global HTTP_DIAGNOSTIC
 headers={'Accept':'application/json','Content-Type':'application/json'}
 if token: headers['Authorization']='Bearer '+token
 if kratos: headers['X-Session-Token']=kratos
 request=urllib.request.Request(origin+path,data=None if body is None else json.dumps(body).encode(),headers=headers,method=method)
 try:
  with opener.open(request,timeout=20) as response: status,raw=response.status,response.read(2**20)
 except urllib.error.HTTPError as error: status,raw=error.code,error.read(2**20)
 HTTP_DIAGNOSTIC={'status':status,'body_present':bool(raw),'json_body':False}
 payload=None
 if raw:
  try:payload=json.loads(raw);HTTP_DIAGNOSTIC['json_body']=True
  except (ValueError,UnicodeError):
   # RequireAuth uses http.Error: a valid denial is text/plain, not JSON.
   # Never print or retain an error body; expected HTTP status still must match.
   if status<400:raise RuntimeError('unexpected non-JSON success/redirect')
 return status,payload

def expect(response,status):
 HTTP_DIAGNOSTIC['expected_status']=status
 if response[0]!=status: raise RuntimeError('unexpected HTTP status')
 return response[1]

def account_diagnostic(response):
 status,body=response
 body=body if isinstance(body,dict) else {}
 identity=body.get('identity')
 identity=identity if isinstance(identity,dict) else {}
 addresses=identity.get('verifiable_addresses')
 email=[a for a in addresses if isinstance(a,dict) and a.get('via')=='email'] if isinstance(addresses,list) else []
 verified=sum(a.get('verified') is True for a in email)
 state=('not_returned' if not isinstance(addresses,list) else
        'not_recorded' if not email else 'verified' if verified else
        'unverified' if all(a.get('verified') is False for a in email) else 'unknown')
 return {'whoami_http':status,'session_active':body.get('active') is True,
         'identity_id_present':isinstance(identity.get('id'),str) and bool(identity['id'].strip()),
         'email_address_count':len(email),'verified_email_address_count':verified,
         'email_verification_status':state}

def command(args,data=None):
 global COMMAND_DIAGNOSTIC
 COMMAND_DIAGNOSTIC={}
 result=subprocess.run(args,input=data,capture_output=True,timeout=60)
 if result.returncode:
  COMMAND_DIAGNOSTIC={'tool':args[0] if args[0] in ('docker','openssl','git') else 'command','exit_code':result.returncode}
  # psql's client-only verbosity emits SQLSTATE without values or query text.
  if 'psql' in args:
   match=re.search(rb'(?:ERROR|FATAL|PANIC):\s+([0-9A-Z]{5})\b',result.stderr)
   if match:COMMAND_DIAGNOSTIC['sqlstate']=match.group(1).decode('ascii')
  raise RuntimeError('command failed; raw output suppressed')
 return result.stdout

def sql(body,variables=None,operator=False):
 args=['docker','exec','-i',DB,'psql','-X','-U','postgres','-d','evt','-Atq','-v','ON_ERROR_STOP=1','-v','VERBOSITY=sqlstate']
 for name,value in (variables or {}).items(): args.extend(['-v',name+'='+value])
 prefix='BEGIN;SET LOCAL search_path=pg_catalog,public,pg_temp;SET LOCAL lock_timeout=\'5s\';SET LOCAL statement_timeout=\'15s\';'
 if operator: prefix+='SET LOCAL ROLE cvera_evidence_operator;'
 return command(args,(prefix+body+'\nCOMMIT;').encode()).decode().strip()


def object_request(url,method,body=None,headers=None):
 # Exact URL from this API's server-generated storage metadata; never redirect.
 request=urllib.request.Request(url,method=method,data=body,headers=headers or {})
 try:
  with opener.open(request,timeout=30) as response:return response.status,response.read(1024)
 except urllib.error.HTTPError as error:return error.code,error.read(1024)

def storage_url(url,storage_key,settings):
 actual=urllib.parse.urlsplit(url);endpoint=urllib.parse.urlsplit(settings['DO_ENDPOINT'])
 bucket=settings['EVT_S3_BUCKET']
 if actual.scheme!='https' or actual.username or actual.password or actual.netloc!=endpoint.netloc or endpoint.scheme!='https':raise RuntimeError('unexpected storage origin')
 expected=endpoint.path.rstrip('/')+'/'+bucket+'/'+storage_key
 if urllib.parse.unquote(actual.path)!=expected:raise RuntimeError('unexpected storage key path')
 return actual

def main():
 global M,PHASE,ACCOUNT_DIAGNOSTIC,HTTP_DIAGNOSTIC,FAILURE_DIAGNOSTIC
 if os.geteuid()!=0 or len(sys.argv)!=3 or sys.argv[1]!='--expected-main' or not re.fullmatch('[0-9a-f]{40}',sys.argv[2]):raise RuntimeError('explicit deployed main required')
 M=sys.argv[2];os.umask(0o077)
 if command(['git','-C',str(ROOT),'rev-parse','HEAD']).decode().strip()!=M or command(['git','-C',str(ROOT),'status','--porcelain']).strip():raise RuntimeError('unexpected checkout')
 if expect(http(API,'/version'),200).get('git_sha')!=M:raise RuntimeError('unexpected deployed version')
 expect(http(API,'/healthz'),200)
 container=json.loads(command(['docker','inspect','evt-api-go-api-1']))[0]
 settings=dict(x.split('=',1) for x in container['Config']['Env'] if '=' in x)
 identifier=getpass.getpass('Existing account login identifier (hidden): ')
 password=getpass.getpass('Existing account password (hidden): ')
 native=None;refresh=None;grant=None;created=False
 case='completion-release-'+uuid.uuid4().hex[:24];check='smoke.file'
 result={'main':M,'credentials_urls_or_contents_printed':False,'new_identities':0,'new_attestations':0}
 try:
  PHASE='normal_login'
  flow=expect(http(AUTH,'/self-service/login/api'),200);flowid=str(uuid.UUID(flow['id']))
  login=expect(http(AUTH,'/self-service/login?flow='+flowid,'POST',{'method':'password','identifier':identifier,'password':password}),200);password=None;identifier=None
  native=login['session_token']
  ACCOUNT_DIAGNOSTIC=account_diagnostic(http(AUTH,'/sessions/whoami',kratos=native))
  if not ACCOUNT_DIAGNOSTIC['session_active'] or ACCOUNT_DIAGNOSTIC['whoami_http']!=200 or not ACCOUNT_DIAGNOSTIC['identity_id_present']:raise RuntimeError('inactive authenticated identity')
  PHASE='api_exchange'
  exchanged=expect(http(API,'/auth/exchange','POST',{'kratos_session_token':native,'device_id':'evidence-completion-release'}),200)
  principal=str(uuid.UUID(exchanged['user']['id']));access=exchanged['access_token'];refresh=exchanged['refresh_token']
  variables={'caseid':case,'checkid':check,'principal':principal,'basis':'owner-approved evidence completion release smoke '+M}
  PHASE='synthetic_operator_fixture'
  sql("SELECT public.provision_evidence_case(:'caseid',:'principal',:'basis');SELECT public.provision_evidence_check(:'caseid',:'checkid',:'principal',:'basis');",variables,True);created=True
  initpaths=['/v1/cases/'+case+'/checks/'+check+'/evidence:init','/v1/evidence/init']
  completepaths=['/v1/cases/'+case+'/checks/'+check+'/evidence:complete','/v1/evidence/commit']
  # Public, nonpersonal 1x1 PNG test fixture. Never print its bytes.
  data=base64.b64decode('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+a5XkAAAAASUVORK5CYII=')
  body={'caseId':case,'checkId':check,'files':[{'name':'synthetic-release.png','size':len(data),'mimeType':'image/png'}]}
  PHASE='unauthorized_init'
  for path in initpaths:
   expect(http(API,path,'POST',body),401);expect(http(API,path,'POST',body,access),404)
  grant=str(uuid.UUID(sql("SELECT public.grant_evidence_upload(:'principal'::uuid,:'caseid',:'principal',:'basis');",variables,True)))
  variables['grantid']=grant
  PHASE='authorized_init_upload'
  sessions=[]
  for path in initpaths:
   initialized=expect(http(API,path,'POST',body,access),200);u=initialized['uploads'][0]
   parsed=storage_url(u['url'],u['storageKey'],settings)
   if urllib.parse.parse_qs(parsed.query).get('X-Amz-Expires')!=['600']:raise RuntimeError('expiry changed')
   status,_=object_request(u['url'],'PUT',data,u['headers'])
   if status not in (200,204):raise RuntimeError('private staging PUT failed')
   sessions.append((initialized,u,parsed))
  initialized,u,parsed=sessions[0]
  complete={'uploadSessionId':initialized['uploadSessionId'],'uploads':[{'uploadId':u['uploadId'],'size':len(data),'mimeType':'image/png'}]}
  PHASE='unauthorized_completion'
  for path in completepaths:expect(http(API,path,'POST',complete),401)
  wrong='/v1/cases/'+case+'/checks/not-the-check/evidence:complete'
  expect(http(API,wrong,'POST',complete,access),404)
  PHASE='authorized_completion_and_retry'
  digest=hashlib.sha256(data).hexdigest()
  for path in completepaths:
   received=expect(http(API,path,'POST',complete,access),200)['evidence']
   if len(received)!=1 or received[0].get('evidence_hash')!=digest or received[0].get('scanStatus')!='PENDING' or received[0].get('status')!='RECEIVED':raise RuntimeError('completion metadata/hash mismatch')
  PHASE='deduplication'
  second,v,_=sessions[1]
  duplicate={'uploadSessionId':second['uploadSessionId'],'uploads':[{'uploadId':v['uploadId'],'size':len(data),'mimeType':'image/png'}]}
  expect(http(API,completepaths[1],'POST',duplicate,access),409)
  PHASE='private_storage_and_database_acceptance'
  metadata=json.loads(sql("SELECT jsonb_build_object('sealed',u.sealed_key,'hash',u.evidence_hash,'state',u.status,'scan',j.scan_status,'owned',s.principal_id=:'principal'::uuid) FROM public.evidence_uploads u JOIN public.upload_sessions s USING(upload_session_id) JOIN public.evidence_scan_jobs j USING(upload_id) WHERE u.upload_id=:'uploadid'::uuid;",{**variables,'uploadid':u['uploadId']}))
  if metadata['hash']!=digest or metadata['state']!='RECEIVED' or metadata['scan']!='PENDING' or metadata['owned'] is not True or not re.fullmatch(r'sealed-evidence/[0-9a-f-]{36}',metadata['sealed']):raise RuntimeError('accepted DB evidence mismatch')
  endpoint=urllib.parse.urlsplit(settings['DO_ENDPOINT'])
  sealed=urllib.parse.urlunsplit((endpoint.scheme,endpoint.netloc,endpoint.path.rstrip('/')+'/'+urllib.parse.quote(settings['EVT_S3_BUCKET'],safe='')+'/'+urllib.parse.quote(metadata['sealed'],safe='/'),'', ''))
  staging=urllib.parse.urlunsplit((parsed.scheme,parsed.netloc,parsed.path,'',''))
  for url in (staging,sealed):
   status,_=object_request(url,'GET')
   if status not in (401,403):raise RuntimeError('private object anonymous access not denied conclusively')
  PHASE='staging_overwrite_isolation'
  status,_=object_request(u['url'],'PUT',data+b'\n',u['headers'])
  if status not in (200,204):raise RuntimeError('staging overwrite test failed')
  received=expect(http(API,completepaths[1],'POST',complete,access),200)['evidence'][0]
  if received.get('evidence_hash')!=digest:raise RuntimeError('accepted digest mutated')
  PHASE='revocation_denial'
  sql("SELECT public.revoke_evidence_upload(:'grantid'::uuid,:'principal',:'basis');",variables,True);grant=None
  for path in completepaths:expect(http(API,path,'POST',complete,access),404)
  result.update({'normal_login_exchange':'PASS','both_init_and_completion_routes':'PASS','unauthorized_and_wrong_check_denied':'PASS','canonical_hash':'PASS','same_check_deduplication':'PASS','private_staging_and_sealed_storage':'PASS','staging_overwrite_isolation':'PASS','revocation':'PASS','scan_status':'PENDING','objects_uploaded':2,'evidence_received':1})
 except Exception:
  FAILURE_DIAGNOSTIC={'http':HTTP_DIAGNOSTIC.copy(),'command':COMMAND_DIAGNOSTIC.copy()};raise
 finally:
  cleanup=[];failed=[]
  if grant:cleanup.append(('grant',lambda:sql("SELECT public.revoke_evidence_upload(:'grantid'::uuid,:'principal',:'basis');",{**variables,'grantid':grant},True)))
  if created:cleanup.append(('case',lambda:sql("SELECT public.set_evidence_resource_state(:'caseid',NULL,'CLOSED',:'principal',:'basis');",variables,True)))
  if refresh:cleanup.append(('api_session',lambda:expect(http(API,'/auth/logout','POST',{'refresh_token':refresh}),200)))
  if native:cleanup.append(('kratos_session',lambda:expect(http(AUTH,'/self-service/logout/api','DELETE',{'session_token':native}),204)))
  for name,action in cleanup:
   try:action()
   except Exception:failed.append(name)
  if failed:PHASE='cleanup_failed:'+','.join(failed);raise RuntimeError('cleanup incomplete')
 result['fixture_grant_revoked_case_closed']=True;result['smoke']='PASS'
 record=ROOT.parent/('evidence-completion-smoke-'+uuid.uuid4().hex+'.json')
 fd=os.open(record,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
 with os.fdopen(fd,'w') as stream:json.dump(result,stream,indent=2)
 print(json.dumps(result))

if __name__=='__main__':
 try:main()
 except Exception:
  print(json.dumps({'smoke':'STOP','phase':PHASE,'failure_diagnostic':FAILURE_DIAGNOSTIC or {'http':HTTP_DIAGNOSTIC,'command':COMMAND_DIAGNOSTIC},'instruction':'Return only this status; never credentials, URLs, content or raw responses.'}),file=sys.stderr);sys.exit(1)
