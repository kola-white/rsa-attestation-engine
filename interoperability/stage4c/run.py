#!/usr/bin/env python3
"""Build runners, then test public fixtures. --isolated proves network/service isolation.
Generates ephemeral synthetic fixtures locally, never accesses production. Result JSON is reproducible,
except for runtime/image metadata. The separate generator issues once using
ephemeral keys; verification processes receive only public artifacts.
"""
import argparse,base64,json,os,pathlib,shutil,subprocess,tempfile,uuid
ROOT=pathlib.Path(__file__).resolve().parents[2]
p=argparse.ArgumentParser();p.add_argument('--isolated',action='store_true');p.add_argument('--node-image',default='node:20');p.add_argument('--output',type=pathlib.Path);args=p.parse_args()
def run(cmd,**kw):return subprocess.run(cmd,check=True,text=True,**kw)
def capture(cmd,**kw):return subprocess.check_output(cmd,text=True,**kw)
def public_only(v):
 if isinstance(v,dict):
  if v.get('kty') and any(k in v for k in ('d','p','q','dp','dq','qi','oth','k')):raise RuntimeError('private JWK in public fixture')
  for x in v.values():public_only(x)
 elif isinstance(v,list):
  for x in v:public_only(x)
 elif isinstance(v,str):
  if 'PRIVATE KEY' in v:raise RuntimeError('private PEM in public fixture')
  # Also inspect compact JWT headers/payloads, not just their outer JSON.
  for token in v.split('~'):
   parts=token.split('.')
   if len(parts)==3:
    for part in parts[:2]:
     try:decoded=json.loads(base64.urlsafe_b64decode(part+'='*((-len(part))%4)))
     except (ValueError,UnicodeError):continue
     public_only(decoded)
with tempfile.TemporaryDirectory(prefix='cvera-stage4c-') as d:
 stage=pathlib.Path(d);stage.chmod(0o755)
 env={**os.environ,'GOTOOLCHAIN':'go1.26.10','CGO_ENABLED':'0'}
 arch=None;image=None
 if args.isolated:
  image=json.loads(capture(['docker','image','inspect',args.node_image]))[0]
  arch=image['Architecture'];env.update(GOOS='linux',GOARCH=arch)
 run(['go','build','-trimpath','-o',str(stage/'go-verifier'),'./cmd/presentation-verify'],cwd=ROOT/'apps/evt-api-go',env=env)
 (stage/'stage4b').mkdir();(stage/'stage4c').mkdir()
 shutil.copy2(ROOT/'interoperability/stage4b/verify.mjs',stage/'stage4b/verify.mjs')
 for f in ['verify.mjs','test.mjs']:shutil.copy2(ROOT/'interoperability/stage4c'/f,stage/'stage4c'/f)
 if args.isolated:
  shutil.copy2(ROOT/'interoperability/stage4c/generate.mjs',stage/'stage4c/generate.mjs')
  (stage/'stage4b/fixtures').mkdir();shutil.copy2(ROOT/'interoperability/stage4b/fixtures/vectors.json',stage/'stage4b/fixtures/vectors.json')
 else:
  run(['node',str(ROOT/'interoperability/stage4c/generate.mjs'),str(stage/'fixtures.json')])
 for path in stage.rglob('*'):
  if path.is_file() and b'PRIVATE KEY' in path.read_bytes():raise RuntimeError('private material in isolated inputs')
 lock=json.loads((ROOT/'package-lock.json').read_text())
 installed=json.loads((ROOT/'node_modules/jose/package.json').read_text())
 if installed['version']!=lock['packages']['node_modules/jose']['version']:raise RuntimeError('installed JOSE differs from lockfile')
 (stage/'node_modules').mkdir();shutil.copytree(ROOT/'node_modules/jose',stage/'node_modules/jose')
 for file in stage.rglob('*'):
  if file.is_file() and file.suffix in ('.json','.pem') and b'-----BEGIN PRIVATE KEY-----' in file.read_bytes():raise RuntimeError('private material in public runner inputs')
 if args.isolated:
  publicFixtures=capture(['docker','run','--rm','--network','none','--read-only','--cap-drop','ALL','--security-opt','no-new-privileges','--user','65534:65534','--mount',f'type=bind,source={stage},target=/work,readonly',args.node_image,'node','/work/stage4c/generate.mjs','/dev/stdout'])
  public_only(json.loads(publicFixtures))
  (stage/'fixtures.json').write_text(publicFixtures)
  (stage/'stage4c/generate.mjs').unlink();shutil.rmtree(stage/'stage4b/fixtures')
  name='cvera-stage4c-'+uuid.uuid4().hex
  try:
   run(['docker','create','--name',name,'--label','cvera.scope=stage4c-local-test','--network','none','--read-only','--cap-drop','ALL','--security-opt','no-new-privileges','--user','65534:65534','--memory','384m','--cpus','2','--tmpfs','/tmp:rw,noexec,nosuid,size=16m','--mount',f'type=bind,source={stage},target=/work,readonly',args.node_image,'node','/work/stage4c/test.mjs','/work/go-verifier','/work/fixtures.json'],stdout=subprocess.DEVNULL)
   info=json.loads(capture(['docker','inspect',name]))[0]
   if info['HostConfig']['NetworkMode']!='none' or not info['HostConfig']['ReadonlyRootfs'] or len(info['Mounts'])!=1 or info['Mounts'][0]['RW']:raise RuntimeError('isolation not established')
   result=json.loads(capture(['docker','start','-a',name]))
   if not result['node'].startswith('v20.'):raise RuntimeError('CI Node20 runtime required')
   result.update(joseVersion=installed['version'],isolation={'network':'none','readOnlyRoot':True,'user':'65534:65534','mounts':'one read-only public fixture/runner directory; no repository, database, sockets or private keys','nodeImageId':image['Id'],'registryDigests':image.get('RepoDigests',[]),'architecture':arch})
  finally:
   subprocess.run(['docker','rm','-f','-v',name],stdout=subprocess.DEVNULL,check=True)
 else:
  public_only(json.loads((stage/'fixtures.json').read_text()))
  result=json.loads(capture(['node',str(stage/'stage4c/test.mjs'),str(stage/'go-verifier'),str(stage/'fixtures.json')]))
  result['isolation']='native child processes with minimal environment; use --isolated for enforced network isolation'
 print(json.dumps(result,indent=2))
 if args.output:args.output.parent.mkdir(parents=True,exist_ok=True);args.output.write_text(json.dumps(result,indent=2)+'\n')
