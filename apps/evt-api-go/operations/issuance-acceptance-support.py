"""Public-key verification and strict acceptance assertions; no private signer."""
import base64,hashlib,json,subprocess,tempfile,time
from pathlib import Path
ISSUER='https://cvera.app';POLICY='employment.authoritative/manual-evidence-pilot/v1';MANUAL='MANUAL_REVIEW_PILOT_V1'
KID='issuer-prod-2026-10';SPKI='35fabf72cfd7dfa88f41aa7a40d7a506966b6ddd797a54c98bd1cd3cf9ec007c'
def decode(s):return base64.urlsafe_b64decode(s+'='*(-len(s)%4))
def tlv(tag,value):
 size=len(value);encoded=bytes([size]) if size<128 else bytes([128+(size.bit_length()+7)//8])+size.to_bytes((size.bit_length()+7)//8,'big')
 return bytes([tag])+encoded+value

def jwk_spki(jwk):
 if jwk.get('kty')!='RSA' or jwk.get('alg')!='RS256' or jwk.get('use')!='sig':raise RuntimeError('unexpected signing key metadata')
 def integer(s):
  value=decode(s).lstrip(b'\x00') or b'\x00'
  return tlv(2,(b'\x00' if value[0]&128 else b'')+value)
 n=int.from_bytes(decode(jwk['n']),'big');e=int.from_bytes(decode(jwk['e']),'big')
 if n.bit_length()<3072 or e!=65537:raise RuntimeError('invalid RSA credential')
 rsa=tlv(48,integer(jwk['n'])+integer(jwk['e']))
 return tlv(48,bytes.fromhex('300d06092a864886f70d0101010500')+tlv(3,b'\x00'+rsa))

def public_binding(public,jwks,fingerprint=SPKI,kid=KID):
 keys=[k for k in jwks.get('keys',[]) if k.get('kid')==kid]
 if len(keys)!=1 or hashlib.sha256(jwk_spki(keys[0])).hexdigest()!=fingerprint:raise RuntimeError('trusted JWKS binding mismatch')
 result=subprocess.run(['openssl','pkey','-pubin','-in',str(public),'-outform','DER'],capture_output=True,timeout=10)
 if result.returncode or result.stdout!=jwk_spki(keys[0]):raise RuntimeError('committed public key differs from trusted JWKS')

def verify_signature(raw,public,jwks,expected,fingerprint=SPKI,kid=KID):
 public_binding(public,jwks,fingerprint,kid)
 if not raw.endswith('~') or raw.count('~')!=1:raise RuntimeError('unexpected credential profile')
 parts=raw[:-1].split('.')
 if len(parts)!=3:raise RuntimeError('invalid compact serialization')
 header=json.loads(decode(parts[0]));claims=json.loads(decode(parts[1]))
 if header.get('alg')!='RS256' or header.get('kid')!=kid or header.get('typ')!='dc+sd-jwt':raise RuntimeError('unexpected signed header')
 with tempfile.TemporaryDirectory(prefix='public-signature-') as folder:
  signature=Path(folder)/'signature';signature.write_bytes(decode(parts[2]));signature.chmod(0o600)
  result=subprocess.run(['openssl','dgst','-sha256','-verify',str(public),'-signature',str(signature)],input=(parts[0]+'.'+parts[1]).encode(),capture_output=True,timeout=10)
 if result.returncode:raise RuntimeError('invalid cryptographic signature')
 if claims.get('iss')!=ISSUER or claims.get('claim_type')!='employment.role' or claims.get('vct')!=ISSUER+'/credentials/employment-role/v1' or claims.get('verification_policy')!=POLICY or claims.get('credential_spki_sha256')!=fingerprint:raise RuntimeError('canonical signed scope mismatch')
 if claims.get('sub')!=expected['subject'] or claims.get('employment',{}).get('employer_id')!=expected['employer']:raise RuntimeError('canonical subject/employer mismatch')
 binding=claims.get('evidence_binding',{})
 for field,value in expected['binding'].items():
  if binding.get(field)!=value:raise RuntimeError('signed evidence/request context mismatch')
 if not binding.get('authority_grant_id','').startswith(expected['authorization']+':') or binding.get('authority_recording_position',0)<=0 or binding.get('manual_policy')!=MANUAL:raise RuntimeError('authority or manual policy commitment mismatch')
 now=time.time()
 if not all(type(claims.get(k)) is int for k in ('nbf','iat','exp')) or claims['iat']!=claims['nbf'] or not claims['nbf']<=now<claims['exp']:raise RuntimeError('signed time bounds invalid')
 return claims

def verified(out):
 v=out.get('verification',{})
 if out.get('signature')!='verified' or out.get('trust')!='trusted' or v.get('decision')!='VERIFIED' or any(v.get(k)!='valid' for k in ('signature','validity','status','manifest_integrity','evidence_review')) or v.get('authority',{}).get('Decision')!='AUTHORIZED' or v.get('scan_policy')!=MANUAL or v.get('scan_statuses')!=['PENDING']:
  raise RuntimeError('signature alone is not trusted acceptance')
 return v
