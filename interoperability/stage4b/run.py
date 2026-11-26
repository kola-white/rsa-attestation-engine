#!/usr/bin/env python3
"""Build runners, then test public fixtures. --isolated proves network/service isolation.
Never regenerates fixtures or accesses production. Result JSON is reproducible,
except for runtime/image metadata. Synthetic issuance happens only in the explicit
fixture generator, not during verification or this script.
"""
import argparse,json,os,pathlib,shutil,subprocess,tempfile,uuid
ROOT=pathlib.Path(__file__).resolve().parents[2]
p=argparse.ArgumentParser();p.add_argument('--isolated',action='store_true');p.add_argument('--node-image',default='node:20');p.add_argument('--output',type=pathlib.Path);args=p.parse_args()
def run(cmd,**kw):return subprocess.run(cmd,check=True,text=True,**kw)
def capture(cmd,**kw):return subprocess.check_output(cmd,text=True,**kw)
with tempfile.TemporaryDirectory(prefix='cvera-stage4b-') as d:
 stage=pathlib.Path(d);stage.chmod(0o755)
 env={**os.environ,'GOTOOLCHAIN':'go1.26.10','CGO_ENABLED':'0'}
 arch=None;image=None
 if args.isolated:
  image=json.loads(capture(['docker','image','inspect',args.node_image]))[0]
  arch=image['Architecture'];env.update(GOOS='linux',GOARCH=arch)
 run(['go','build','-trimpath','-o',str(stage/'go-verifier'),'./cmd/portable-verify'],cwd=ROOT/'apps/evt-api-go',env=env)
 for f in ['verify.mjs','test.mjs']:shutil.copy2(ROOT/'interoperability/stage4b'/f,stage/f)
 shutil.copytree(ROOT/'interoperability/stage4b/fixtures',stage/'fixtures')
 lock=json.loads((ROOT/'package-lock.json').read_text())
 installed=json.loads((ROOT/'node_modules/jose/package.json').read_text())
 if installed['version']!=lock['packages']['node_modules/jose']['version']:raise RuntimeError('installed JOSE differs from lockfile')
 (stage/'node_modules').mkdir();shutil.copytree(ROOT/'node_modules/jose',stage/'node_modules/jose')
 for file in stage.rglob('*'):
  if file.is_file() and file.suffix in ('.json','.pem') and b'-----BEGIN PRIVATE KEY-----' in file.read_bytes():raise RuntimeError('private material in public runner inputs')
 if args.isolated:
  name='cvera-stage4b-'+uuid.uuid4().hex
  try:
   run(['docker','create','--name',name,'--label','cvera.scope=stage4b-local-test','--network','none','--read-only','--cap-drop','ALL','--security-opt','no-new-privileges','--user','65534:65534','--memory','384m','--cpus','2','--tmpfs','/tmp:rw,noexec,nosuid,size=16m','--mount',f'type=bind,source={stage},target=/work,readonly',args.node_image,'node','/work/test.mjs','/work/go-verifier'],stdout=subprocess.DEVNULL)
   info=json.loads(capture(['docker','inspect',name]))[0]
   if info['HostConfig']['NetworkMode']!='none' or not info['HostConfig']['ReadonlyRootfs'] or len(info['Mounts'])!=1 or info['Mounts'][0]['RW']:raise RuntimeError('isolation not established')
   result=json.loads(capture(['docker','start','-a',name]))
   if not result['node'].startswith('v20.'):raise RuntimeError('CI Node20 runtime required')
   result.update(joseVersion=installed['version'],isolation={'network':'none','readOnlyRoot':True,'user':'65534:65534','mounts':'one read-only public fixture/runner directory; no repository, database, sockets or private keys','nodeImageId':image['Id'],'registryDigests':image.get('RepoDigests',[]),'architecture':arch})
  finally:
   subprocess.run(['docker','rm','-f','-v',name],stdout=subprocess.DEVNULL,check=True)
 else:
  result=json.loads(capture(['node',str(stage/'test.mjs'),str(stage/'go-verifier')]))
  result['isolation']='native child processes with minimal environment; use --isolated for enforced network isolation'
 print(json.dumps(result,indent=2))
 if args.output:args.output.parent.mkdir(parents=True,exist_ok=True);args.output.write_text(json.dumps(result,indent=2)+'\n')
