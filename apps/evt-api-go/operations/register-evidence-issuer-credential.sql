-- OWNER ONLY, explicit separate production approval required. Public material
-- only: no private-key import, key rotation, role change or authority grant.
-- Execute in one READ COMMITTED transaction with public,pg_temp as search_path.
-- Caller provides LOCAL cvera.credential_kid/public_pem/spki_sha256/valid_from/
-- valid_until/claim_lifetime_seconds/established_by/basis_ref; no defaults.
DO $registration$
DECLARE k text:=current_setting('cvera.credential_kid');p text:=current_setting('cvera.credential_public_pem');
 fingerprint text:=current_setting('cvera.credential_spki_sha256');
 start_at timestamptz:=current_setting('cvera.credential_valid_from')::timestamptz;
 end_at timestamptz:=current_setting('cvera.credential_valid_until')::timestamptz;
 lifetime integer:=current_setting('cvera.credential_claim_lifetime_seconds')::integer;
 actor text:=current_setting('cvera.credential_established_by');basis text:=current_setting('cvera.credential_basis_ref');
 owner_oid oid;actual_fingerprint text;
BEGIN
 SELECT relowner INTO STRICT owner_oid FROM pg_catalog.pg_class WHERE oid=pg_catalog.to_regclass('evidence_issuer_credentials');
 IF owner_oid<>current_user::regrole OR current_setting('transaction_isolation')<>'read committed'
 OR current_schema() NOT IN (SELECT nspname FROM pg_catalog.pg_namespace WHERE oid=(SELECT relnamespace FROM pg_catalog.pg_class WHERE oid='evidence_issuer_credentials'::regclass))
 THEN RAISE EXCEPTION 'trusted owner transaction required';END IF;
 IF coalesce(k,'')='' OR coalesce(actor,'')='' OR coalesce(basis,'')='' OR start_at>clock_timestamp() OR end_at<=clock_timestamp()
 OR lifetime<=0 OR lifetime>31536000 OR fingerprint !~ '^[0-9a-f]{64}$'
 OR p NOT LIKE '-----BEGIN PUBLIC KEY-----%' OR p LIKE '%PRIVATE KEY%' THEN RAISE EXCEPTION 'explicit approved public credential metadata required';END IF;
 actual_fingerprint:=encode(sha256(decode(regexp_replace(replace(replace(p,'-----BEGIN PUBLIC KEY-----',''),'-----END PUBLIC KEY-----',''),'\s','','g'),'base64')),'hex');
 IF actual_fingerprint<>fingerprint THEN RAISE EXCEPTION 'public credential fingerprint mismatch'; END IF;
 INSERT INTO evidence_issuer_credentials(issuer_id,kid,public_key_pem,spki_sha256,valid_from,valid_until,claim_lifetime_seconds,established_by,authority_basis_ref)
 VALUES('https://cvera.app',k,p,fingerprint,start_at,end_at,lifetime,actor,basis);
END $registration$;
