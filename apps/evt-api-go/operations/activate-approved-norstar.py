#!/usr/bin/env python3
"""Execute only the separately owner-approved NorStar ACTIVATE journal append.

Root console only. Requires the existing recording receipt and deployed authority schema.
Stops on any changed state or previous activation attempt. No credentials,
privileges, cases, grants, claims, signing or service operations are performed.
"""
import sys
import datetime,fcntl,hashlib,json,os,stat,subprocess,uuid
from pathlib import Path


SETTINGS = """SET LOCAL search_path=public,pg_temp;SET LOCAL lock_timeout='5s';SET LOCAL statement_timeout='60s';"""
FINGERPRINT = """SELECT encode(sha256(convert_to(jsonb_build_object(
 'objects',(SELECT jsonb_agg(jsonb_build_array(c.oid,c.relname,c.relowner,c.relacl,c.relrowsecurity,c.relforcerowsecurity) ORDER BY c.oid) FROM pg_catalog.pg_class c WHERE c.relnamespace='public'::regnamespace AND c.relname NOT LIKE 'authority_%' AND c.relname NOT LIKE 'evidence_issu%'),
 'functions',(SELECT jsonb_agg(jsonb_build_array(p.oid,p.proowner,p.proacl,p.proconfig) ORDER BY p.oid) FROM pg_catalog.pg_proc p WHERE p.pronamespace='public'::regnamespace AND p.proname NOT IN ('reject_authority_mutation','next_authority_recording_position','authority_evidence_position_count','validate_authority_evidence_recording','validate_authority_recording_allocation','active_evidence_signing_credential','read_evidence_authority','prepare_bound_issuance','commit_bound_issuance','resolve_bound_credential','guard_evidence_issuance','revoke_bound_credential','protect_evidence_issuance')),
 'roles',(SELECT jsonb_agg(jsonb_build_array(oid,rolname,rolsuper,rolcanlogin,rolcreatedb,rolcreaterole,rolbypassrls) ORDER BY oid) FROM pg_catalog.pg_roles),
 'memberships',(SELECT coalesce(jsonb_agg(to_jsonb(m) ORDER BY roleid,member),'[]'::jsonb) FROM pg_catalog.pg_auth_members m),
 'schema_acl',(SELECT nspacl FROM pg_catalog.pg_namespace WHERE nspname='public'))::text,'UTF8')),'hex')"""

DB='evt-api-go-db-1'
def sql_command(body):
    result=subprocess.run(['docker','exec','-i',DB,'psql','-X','-U','postgres','-d','evt','-Atq','-v','ON_ERROR_STOP=1','-v','VERBOSITY=sqlstate'],input=body.encode(),capture_output=True,timeout=120)
    if result.returncode:raise RuntimeError('SQL failed; raw output suppressed')
    return result.stdout
def read_db(body):return sql_command('BEGIN READ ONLY;'+SETTINGS+body+';ROLLBACK;')
def private(path,data):
    fd=os.open(path,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW,0o600)
    with os.fdopen(fd,'wb') as stream:stream.write(data);stream.flush();os.fsync(stream.fileno())
def secure(path,mode):
    st=path.lstat()
    if stat.S_ISLNK(st.st_mode) or st.st_uid!=0 or stat.S_IMODE(st.st_mode)!=mode:raise RuntimeError('private receipt permissions')

AID='1ace0869-272c-4187-8928-1d7613b67269'
DID='f628529a-5134-4b16-b789-2a711aec6892'
ACTOR='45c026e2-1184-4aa9-bc4e-1bb95f52d29c'
PARAM_SHA='acc7ea33f412dc98f9ed1d2c4e7cd58f8d6cc1ece0b7beaa7ad3bd3480ad0475'
def literal(value):
    assert isinstance(value,str) and '\x00' not in value
    return "'"+value.replace("'","''")+"'"
aid=literal(AID);did=literal(DID)
SNAPSHOT="""SELECT md5(jsonb_build_object(
 'authorizations',(SELECT coalesce(jsonb_agg(to_jsonb(a) ORDER BY authorization_id),'[]'::jsonb) FROM public.authority_authorizations a),
 'adjudications',(SELECT coalesce(jsonb_agg(to_jsonb(a) ORDER BY decision_id),'[]'::jsonb) FROM public.authority_adjudications a),
 'events',(SELECT coalesce(jsonb_agg(to_jsonb(a) ORDER BY journal_sequence),'[]'::jsonb) FROM public.authority_journal_events a WHERE event_id <> %s),
 'credentials',(SELECT coalesce(jsonb_agg(to_jsonb(a) ORDER BY to_jsonb(a)::text),'[]'::jsonb) FROM public.evidence_issuer_credentials a),
 'issued',(SELECT coalesce(jsonb_agg(to_jsonb(a) ORDER BY to_jsonb(a)::text),'[]'::jsonb) FROM public.evidence_issued_credentials a)
)::text)"""
def activation_sql(event_id,expected):
    snapshot=SNAPSHOT % literal(event_id)
    return """BEGIN ISOLATION LEVEL READ COMMITTED;
SET LOCAL search_path=pg_catalog,public,pg_temp;
SET LOCAL lock_timeout='5s';SET LOCAL statement_timeout='30s';
LOCK TABLE public.authority_recording_boundary IN EXCLUSIVE MODE;
LOCK TABLE public.authority_authorizations,public.authority_adjudications,public.authority_journal_events IN SHARE ROW EXCLUSIVE MODE;
DO $activation$
DECLARE pos bigint; instant timestamptz;
BEGIN
 IF current_user<>'postgres' OR current_database()<>'evt' THEN RAISE EXCEPTION 'trusted owner/database required';END IF;
 IF ("""+snapshot+") IS DISTINCT FROM "+literal(expected)+""" THEN RAISE EXCEPTION 'authority history changed; stop';END IF;
 IF (SELECT committed_position FROM public.authority_recording_boundary WHERE singleton) IS DISTINCT FROM 4::bigint THEN RAISE EXCEPTION 'recording position changed';END IF;
 IF NOT EXISTS(SELECT 1 FROM public.domain_users WHERE id='45c026e2-1184-4aa9-bc4e-1bb95f52d29c'::uuid AND status='active') THEN RAISE EXCEPTION 'inactive acting principal';END IF;
 IF NOT EXISTS(SELECT 1 FROM public.authority_authorizations WHERE authorization_id='1ace0869-272c-4187-8928-1d7613b67269' AND issuer_id='https://cvera.app' AND authority_subject_id='norstar-systems-llc' AND claim_type='employment.role') THEN RAISE EXCEPTION 'scope mismatch';END IF;
 IF (SELECT array_agg(event_type ORDER BY journal_sequence) FROM public.authority_journal_events WHERE authorization_id='1ace0869-272c-4187-8928-1d7613b67269') IS DISTINCT FROM ARRAY['SUBMIT','APPROVE']::text[] THEN RAISE EXCEPTION 'not approved-not-active';END IF;
 instant:=clock_timestamp();
 IF instant<'2026-11-08T23:20:00Z'::timestamptz OR instant<(SELECT max(effective_at) FROM public.authority_journal_events WHERE authorization_id='1ace0869-272c-4187-8928-1d7613b67269') THEN RAISE EXCEPTION 'activation time invalid';END IF;
 pos:=public.next_authority_recording_position();
 INSERT INTO public.authority_journal_events(event_id,authorization_id,event_type,effective_at,recorded_at,actor_id,evidence_ref,conditions_satisfied,recording_position)
 SELECT """+literal(event_id)+""",a.authorization_id,'ACTIVATE',instant,clock_timestamp(),d.decision_maker_id,d.evidence_ref,true,pos
 FROM public.authority_authorizations a JOIN public.authority_adjudications d USING(authorization_id)
 WHERE a.authorization_id='1ace0869-272c-4187-8928-1d7613b67269' AND d.decision_id='f628529a-5134-4b16-b789-2a711aec6892' AND d.outcome='APPROVED';
 IF NOT FOUND THEN RAISE EXCEPTION 'approved decision missing';END IF;
 IF pos<>5 OR ("""+snapshot+") IS DISTINCT FROM "+literal(expected)+""" THEN RAISE EXCEPTION 'unexpected mutation';END IF;
END $activation$;
COMMIT;
"""

def facts():
    return read("SELECT jsonb_build_object('authorization',(SELECT to_jsonb(a)||jsonb_build_object('xid',a.xmin::text) FROM public.authority_authorizations a WHERE authorization_id=" + aid + "),'adjudication',(SELECT to_jsonb(d)||jsonb_build_object('xid',d.xmin::text) FROM public.authority_adjudications d WHERE decision_id=" + did + "),'events',(SELECT coalesce(jsonb_agg(to_jsonb(e)||jsonb_build_object('xid',e.xmin::text) ORDER BY journal_sequence),'[]'::jsonb) FROM public.authority_journal_events e WHERE authorization_id=" + aid + "),'boundary',(SELECT committed_position FROM public.authority_recording_boundary WHERE singleton),'credential_rows',(SELECT count(*) FROM public.evidence_issuer_credentials),'issued_rows',(SELECT count(*) FROM public.evidence_issued_credentials),'manual',EXISTS(SELECT 1 FROM public.evt_employer_config WHERE employer_id='norstar-systems-llc' AND dispatch_mode='MANUAL'))")
def stamp(value):
    return datetime.datetime.fromisoformat(value.replace('Z', '+00:00'))
def verify(data):
    a = data['authorization']
    d = data['adjudication']
    events = data['events']
    assert a['authorization_id'] == params['authorization_id'] and d['decision_id'] == params['decision_id'] and (d['authorization_id'] == a['authorization_id'])
    for row in (a, d):
        assert row['issuer_id'] == 'https://cvera.app' and row['authority_subject_id'] == 'norstar-systems-llc' and (row['claim_type'] == 'employment.role')
    assert d['outcome'] == 'APPROVED' and d['decision_maker_id'] == params['actor_id'] and (d['evidence_ref'] == params['evidence_ref'])
    assert d['authority_basis'] == params['authority_basis'] + '; first-party/operator-adjudicated; conflict disclosure: ' + params['conflict_disclosure_ref']
    assert stamp(d['decided_at']) == stamp(params['decided_at']) and stamp(d['recorded_at']) >= stamp(d['decided_at'])
    assert [e['event_type'] for e in events] == ['SUBMIT', 'APPROVE']
    for e in events:
        uuid.UUID(e['event_id'])
        assert e['authorization_id'] == a['authorization_id'] and e['actor_id'] == params['actor_id'] and (e['evidence_ref'] == params['evidence_ref']) and (e['conditions_satisfied'] is True)
        assert stamp(e['recorded_at']) >= stamp(e['effective_at']) >= stamp(d['recorded_at'])
    assert stamp(events[1]['effective_at']) >= stamp(events[0]['effective_at'])
    rows = [a, d] + events
    assert [r['recording_position'] for r in rows] == [1, 2, 3, 4] and len({r['xid'] for r in rows}) == 4
    assert data['boundary'] == 4 and data['credential_rows'] == 0 and (data['issued_rows'] == 0) and (data['manual'] is True)
    return [r['xid'] for r in rows]

if __name__=='__main__':
    phase='preflight'
    try:
        assert sys.argv[1:] in (['--preflight-only'],['--execute-approved-activation'])
        assert os.geteuid()==0
        os.umask(0o077)
        base=Path('/opt/cvera/norstar-preparation-20261108T220104Z-9065a0')
        previous=base/'authority-recording-1ace0869';target=base/'authority-activation-1ace0869'
        h={'secure':secure,'sql':sql_command,'read':read_db,'private':private,'FINGERPRINT':FINGERPRINT}
        secure(base,0o700);secure(previous,0o700)
        lock=os.open(base/'authority-recording.lock',os.O_CREAT|os.O_RDWR|os.O_NOFOLLOW,0o600);fcntl.flock(lock,fcntl.LOCK_EX|fcntl.LOCK_NB)
        if target.exists():raise RuntimeError('existing activation attempt: inspect receipt without retry')
        def read(query):
            raw=h['read'](query).decode('utf-8').strip()
            return True if raw=='t' else False if raw=='f' else json.loads(raw)
        for name in ('verification.json','intent.json','audit-rows.private.json'):h['secure'](previous/name,0o600)
        receipt=json.loads((previous/'verification.json').read_text());intent=json.loads((previous/'intent.json').read_text());prior=json.loads((previous/'audit-rows.private.json').read_text())
        assert receipt['authority_recording']=='PASS' and receipt['state']=='APPROVED_NOT_ACTIVE' and receipt['parameters_sha256']==PARAM_SHA
        assert hashlib.sha256(json.dumps(intent['parameters'],sort_keys=True,separators=(',',':')).encode()).hexdigest()==PARAM_SHA
        params=intent['parameters'];assert params['authorization_id']==AID and params['decision_id']==DID and params['actor_id']==ACTOR
        current=facts();verify(current);assert current==prior
        assert read("SELECT current_user='postgres' AND current_database()='evt' AND EXISTS(SELECT 1 FROM public.domain_users WHERE id='45c026e2-1184-4aa9-bc4e-1bb95f52d29c'::uuid AND status='active')") is True
        owners=read("SELECT bool_and(relowner='postgres'::regrole AND NOT relrowsecurity AND NOT relforcerowsecurity) FROM pg_catalog.pg_class WHERE oid IN ('public.authority_authorizations'::regclass,'public.authority_adjudications'::regclass,'public.authority_journal_events'::regclass,'public.authority_recording_boundary'::regclass)");assert owners is True
        if sys.argv[1:]==['--preflight-only']:
            print(json.dumps({'authority_activation_preflight':'PASS','authorization_id':AID,'state':'APPROVED_NOT_ACTIVE','scope_matches':True,'recorded_approval_and_evidence_match':True,'prior_history_unchanged':True,'activation_intent_absent':True,'database_changes':False}));raise SystemExit(0)
        fingerprint=h['read'](h['FINGERPRINT']);event_id=str(uuid.uuid4())
        before=h['read'](SNAPSHOT % literal(event_id)).decode('utf-8').strip()
        sql=activation_sql(event_id,before);sql_sha=hashlib.sha256(sql.encode()).hexdigest()
        target.mkdir(mode=0o700)
        h['private'](target/'intent.json',json.dumps({'authorization_id':AID,'event_id':event_id,'approval_ref':'owner-explicit-norstar-activation-1ace0869','required_state':'APPROVED_NOT_ACTIVE','target_state':'ACTIVE','sql_sha256':sql_sha,'prior_snapshot_md5':before,'documented_start':'2026-11-08T23:20:00Z','scope':'single current-time ACTIVATE journal append; no credentials, grants, cases, claims or privileges'}).encode())
        h['private'](target/'approved-activation.sql',sql.encode())
        phase='single_approved_activation_transaction'
        h['sql'](sql)
        phase='read_only_final_verification'
        after=facts();events=after['events'];assert [e['event_type'] for e in events]==['SUBMIT','APPROVE','ACTIVATE']
        last=events[-1];uuid.UUID(last['event_id'])
        assert last['event_id']==event_id and last['authorization_id']==AID and last['actor_id']==ACTOR and last['evidence_ref']==params['evidence_ref'] and last['conditions_satisfied'] is True
        assert last['recording_position']==5 and after['boundary']==5 and last['corrects_event_id'] is None
        assert stamp(last['recorded_at'])>=stamp(last['effective_at'])>=stamp(events[-2]['effective_at']) and stamp(last['effective_at'])>=stamp('2026-11-08T23:20:00Z')
        without=json.loads(json.dumps(after));without['events']=events[:-1];without['boundary']=4;assert without==prior
        assert h['read'](SNAPSHOT % literal(event_id)).decode('utf-8').strip()==before and h['read'](h['FINGERPRINT'])==fingerprint
        result={'authority_activation':'PASS','authorization_id':AID,'decision_id':DID,'issuer':'https://cvera.app','employer':'norstar-systems-llc','claim_type':'employment.role','actor_id':ACTOR,'previous_state':'APPROVED_NOT_ACTIVE','state':'ACTIVE','event_id':event_id,'effective_at':last['effective_at'],'recorded_at':last['recorded_at'],'recording_position':5,'transaction_id':last['xid'],'audit_events':['SUBMIT','APPROVE','ACTIVATE'],'unrelated_authority_and_history_unchanged':True,'roles_and_privileges_unchanged':True,'credentials_or_claims_created':0,'manual_dispatch_unchanged':True,'private_record':str(target/'verification.json')}
        h['private'](target/'audit-rows.private.json',json.dumps(after).encode());h['private'](target/'verification.json',json.dumps(result).encode());print(json.dumps(result))
    except Exception:
        print(json.dumps({'authority_activation':'STOP','phase':phase,'instruction':'Do not retry, sign or print private data. Preserve intent and receipt for read-only reconciliation.'}));raise SystemExit(1)
