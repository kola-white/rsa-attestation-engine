-- Proposal B: new operator-provisioned registry, not inferred legacy schema.
-- Trusted owner only; select the verified application schema explicitly.
-- Once-only and atomic even with an autocommit client. No history is invented.
DO $migration$
DECLARE
    s text := current_schema();
    item record;
    recipient record;
BEGIN
    IF s IS NULL OR s IN ('pg_catalog','information_schema') OR s LIKE 'pg_temp%' THEN
        RAISE EXCEPTION 'explicit trusted application schema required';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_catalog.pg_attribute
        WHERE attrelid=pg_catalog.to_regclass(pg_catalog.format('%I.domain_users',s))
        AND attname='id' AND atttypid='uuid'::regtype AND attnotnull AND NOT attisdropped)
       OR NOT EXISTS (SELECT 1 FROM pg_catalog.pg_attribute
        WHERE attrelid=pg_catalog.to_regclass(pg_catalog.format('%I.domain_users',s))
        AND attname='status' AND atttypid='text'::regtype AND attnotnull AND NOT attisdropped) THEN
        RAISE EXCEPTION 'confirmed domain_users UUID/status prerequisites missing';
    END IF;
    EXECUTE pg_catalog.format($ddl$
CREATE TABLE %1$I.cases (
    case_id text PRIMARY KEY CHECK (case_id ~ '^[A-Za-z0-9._-]{1,64}$' AND case_id NOT IN ('.','..')),
    status text NOT NULL DEFAULT 'OPEN' CHECK (status IN ('OPEN','CLOSED','DELETION_SCHEDULED','DELETED')),
    created_at timestamptz NOT NULL DEFAULT pg_catalog.clock_timestamp(),
    created_by text NOT NULL CHECK (btrim(created_by) <> ''),
    creation_basis_ref text NOT NULL CHECK (btrim(creation_basis_ref) <> ''),
    updated_at timestamptz NOT NULL DEFAULT pg_catalog.clock_timestamp(),
    updated_by text NOT NULL CHECK (btrim(updated_by) <> ''),
    update_basis_ref text NOT NULL CHECK (btrim(update_basis_ref) <> '')
);
CREATE TABLE %1$I.verification_checks (
    case_id text NOT NULL REFERENCES %1$I.cases(case_id) ON UPDATE RESTRICT ON DELETE RESTRICT,
    check_id text NOT NULL CHECK (check_id ~ '^[A-Za-z0-9._-]{1,96}$' AND check_id NOT IN ('.','..')),
    status text NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','IN_REVIEW','PASSED','FAILED','DELETED')),
    created_at timestamptz NOT NULL DEFAULT pg_catalog.clock_timestamp(),
    created_by text NOT NULL CHECK (btrim(created_by) <> ''),
    creation_basis_ref text NOT NULL CHECK (btrim(creation_basis_ref) <> ''),
    updated_at timestamptz NOT NULL DEFAULT pg_catalog.clock_timestamp(),
    updated_by text NOT NULL CHECK (btrim(updated_by) <> ''),
    update_basis_ref text NOT NULL CHECK (btrim(update_basis_ref) <> ''),
    PRIMARY KEY (case_id,check_id)
);
CREATE TABLE %1$I.case_evidence_upload_grants (
    grant_id uuid PRIMARY KEY DEFAULT pg_catalog.gen_random_uuid(),
    principal_id uuid NOT NULL REFERENCES %1$I.domain_users(id) ON UPDATE RESTRICT ON DELETE RESTRICT,
    case_id text NOT NULL REFERENCES %1$I.cases(case_id) ON UPDATE RESTRICT ON DELETE RESTRICT,
    granted_at timestamptz NOT NULL DEFAULT pg_catalog.clock_timestamp(),
    granted_by text NOT NULL CHECK (btrim(granted_by) <> ''),
    grant_basis_ref text NOT NULL CHECK (btrim(grant_basis_ref) <> ''),
    granted_session_user text NOT NULL DEFAULT session_user,
    revoked_at timestamptz,
    revoked_by text,
    revocation_reason text,
    revoked_session_user text,
    CHECK ((revoked_at IS NULL AND revoked_by IS NULL AND revocation_reason IS NULL AND revoked_session_user IS NULL)
        OR (revoked_at IS NOT NULL AND revoked_at >= granted_at AND revoked_by IS NOT NULL
            AND btrim(revoked_by) <> '' AND revocation_reason IS NOT NULL
            AND btrim(revocation_reason) <> '' AND revoked_session_user IS NOT NULL))
);
CREATE UNIQUE INDEX case_evidence_one_active_grant ON %1$I.case_evidence_upload_grants(principal_id,case_id) WHERE revoked_at IS NULL;
$ddl$,s);
    EXECUTE pg_catalog.format($definition$
CREATE FUNCTION %1$I.check_evidence_upload(p_principal uuid,p_case text,p_check text) RETURNS boolean
LANGUAGE plpgsql VOLATILE SECURITY DEFINER
SET search_path=pg_catalog,%1$I,pg_temp
AS $body$
DECLARE value text;
BEGIN
    IF current_setting('transaction_isolation') <> 'read committed' THEN
        RAISE EXCEPTION 'evidence authorization requires READ COMMITTED' USING ERRCODE='55000';
    END IF;
    IF EXISTS(SELECT 1 FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace
        WHERE n.nspname=%2$L AND c.relname IN ('domain_users','cases','verification_checks','case_evidence_upload_grants')
        AND (c.relrowsecurity OR c.relforcerowsecurity)) THEN
        RAISE EXCEPTION 'evidence authorization RLS requires review' USING ERRCODE='55000';
    END IF;
    -- Consistent order: principal, case, check, grant. Shared row locks conflict
    -- with status UPDATE and grant revocation, and survive until caller COMMIT.
    SELECT status INTO value FROM %1$I.domain_users WHERE id=p_principal FOR SHARE;
    IF NOT FOUND OR value <> 'active' THEN RETURN FALSE; END IF;
    SELECT status INTO value FROM %1$I.cases WHERE case_id=p_case FOR SHARE;
    IF NOT FOUND OR value <> 'OPEN' THEN RETURN FALSE; END IF;
    SELECT status INTO value FROM %1$I.verification_checks
        WHERE case_id=p_case AND check_id=p_check FOR SHARE;
    IF NOT FOUND OR value NOT IN ('PENDING','IN_REVIEW') THEN RETURN FALSE; END IF;
    PERFORM 1 FROM %1$I.case_evidence_upload_grants
        WHERE principal_id=p_principal AND case_id=p_case AND revoked_at IS NULL FOR SHARE;
    RETURN FOUND;
END;
$body$;
$definition$,s,s);
    EXECUTE pg_catalog.format($definition$
CREATE FUNCTION %1$I.provision_evidence_case(p_case text,p_actor text,p_basis text) RETURNS void
LANGUAGE plpgsql VOLATILE SECURITY DEFINER
SET search_path=pg_catalog,%1$I,pg_temp
AS $body$
BEGIN
    INSERT INTO %1$I.cases(case_id,created_by,creation_basis_ref,updated_by,update_basis_ref)
        VALUES(p_case,p_actor,p_basis,p_actor,p_basis);
END;
$body$;
$definition$,s,s);
    EXECUTE pg_catalog.format($definition$
CREATE FUNCTION %1$I.provision_evidence_check(p_case text,p_check text,p_actor text,p_basis text) RETURNS void
LANGUAGE plpgsql VOLATILE SECURITY DEFINER
SET search_path=pg_catalog,%1$I,pg_temp
AS $body$
BEGIN
    INSERT INTO %1$I.verification_checks(case_id,check_id,created_by,creation_basis_ref,updated_by,update_basis_ref)
        VALUES(p_case,p_check,p_actor,p_basis,p_actor,p_basis);
END;
$body$;
$definition$,s,s);
    EXECUTE pg_catalog.format($definition$
CREATE FUNCTION %1$I.grant_evidence_upload(p_principal uuid,p_case text,p_actor text,p_basis text) RETURNS uuid
LANGUAGE plpgsql VOLATILE SECURITY DEFINER
SET search_path=pg_catalog,%1$I,pg_temp
AS $body$
DECLARE value text; result uuid;
BEGIN
    SELECT status INTO value FROM %1$I.domain_users WHERE id=p_principal FOR SHARE;
    IF NOT FOUND OR value <> 'active' THEN RAISE EXCEPTION 'active principal required'; END IF;
    SELECT status INTO value FROM %1$I.cases WHERE case_id=p_case FOR SHARE;
    IF NOT FOUND OR value <> 'OPEN' THEN RAISE EXCEPTION 'open case required'; END IF;
    INSERT INTO %1$I.case_evidence_upload_grants(principal_id,case_id,granted_by,grant_basis_ref)
        VALUES(p_principal,p_case,p_actor,p_basis) RETURNING grant_id INTO result;
    RETURN result;
END;
$body$;
$definition$,s,s);
    EXECUTE pg_catalog.format($definition$
CREATE FUNCTION %1$I.revoke_evidence_upload(p_grant uuid,p_actor text,p_reason text) RETURNS void
LANGUAGE plpgsql VOLATILE SECURITY DEFINER
SET search_path=pg_catalog,%1$I,pg_temp
AS $body$
BEGIN
    UPDATE %1$I.case_evidence_upload_grants
        SET revoked_at=clock_timestamp(),revoked_by=p_actor,revocation_reason=p_reason,revoked_session_user=session_user
        WHERE grant_id=p_grant AND revoked_at IS NULL;
    IF NOT FOUND THEN RAISE EXCEPTION 'unrevoked grant required'; END IF;
END;
$body$;
$definition$,s,s);
    EXECUTE pg_catalog.format($definition$
CREATE FUNCTION %1$I.set_evidence_resource_state(p_case text,p_check text,p_state text,p_actor text,p_reason text) RETURNS void
LANGUAGE plpgsql VOLATILE SECURITY DEFINER
SET search_path=pg_catalog,%1$I,pg_temp
AS $body$
BEGIN
    -- NULL check means the case itself; no implicit creation or cascading grants.
    IF p_check IS NULL THEN
        UPDATE %1$I.cases SET status=p_state,updated_at=clock_timestamp(),updated_by=p_actor,update_basis_ref=p_reason
            WHERE case_id=p_case;
    ELSE
        UPDATE %1$I.verification_checks SET status=p_state,updated_at=clock_timestamp(),updated_by=p_actor,update_basis_ref=p_reason
            WHERE case_id=p_case AND check_id=p_check;
    END IF;
    IF NOT FOUND THEN RAISE EXCEPTION 'persisted resource required'; END IF;
END;
$body$;
$definition$,s,s);
    FOR item IN SELECT c.oid,c.relname,c.relowner FROM pg_catalog.pg_class c
        WHERE c.relnamespace=pg_catalog.to_regnamespace(s)
        AND c.relname IN ('cases','verification_checks','case_evidence_upload_grants') LOOP
        FOR recipient IN SELECT DISTINCT a.grantee FROM
            pg_catalog.aclexplode(coalesce((SELECT relacl FROM pg_catalog.pg_class WHERE oid=item.oid),pg_catalog.acldefault('r',item.relowner))) a
            WHERE a.grantee<>item.relowner LOOP
            EXECUTE pg_catalog.format('REVOKE ALL ON TABLE %I.%I FROM %s',s,item.relname,
                CASE WHEN recipient.grantee=0 THEN 'PUBLIC' ELSE pg_catalog.quote_ident(pg_catalog.pg_get_userbyid(recipient.grantee)) END);
        END LOOP;
    END LOOP;
    FOR item IN SELECT p.* FROM pg_catalog.pg_proc p WHERE p.oid IN (
        pg_catalog.to_regprocedure(pg_catalog.format('%I.check_evidence_upload(uuid,text,text)',s)),
        pg_catalog.to_regprocedure(pg_catalog.format('%I.provision_evidence_case(text,text,text)',s)),
        pg_catalog.to_regprocedure(pg_catalog.format('%I.provision_evidence_check(text,text,text,text)',s)),
        pg_catalog.to_regprocedure(pg_catalog.format('%I.grant_evidence_upload(uuid,text,text,text)',s)),
        pg_catalog.to_regprocedure(pg_catalog.format('%I.revoke_evidence_upload(uuid,text,text)',s)),
        pg_catalog.to_regprocedure(pg_catalog.format('%I.set_evidence_resource_state(text,text,text,text,text)',s))) LOOP
        FOR recipient IN SELECT DISTINCT a.grantee FROM
            pg_catalog.aclexplode(coalesce(item.proacl,pg_catalog.acldefault('f',item.proowner))) a
            WHERE a.grantee<>item.proowner LOOP
            EXECUTE pg_catalog.format('REVOKE ALL ON FUNCTION %s FROM %s',item.oid::regprocedure,
                CASE WHEN recipient.grantee=0 THEN 'PUBLIC' ELSE pg_catalog.quote_ident(pg_catalog.pg_get_userbyid(recipient.grantee)) END);
        END LOOP;
    END LOOP;
END;
$migration$;
