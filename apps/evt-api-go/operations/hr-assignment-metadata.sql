-- REVIEW AND RUN SEPARATELY. Never run automatically during deployment.
-- PostgreSQL 16 catalog metadata only. No application records, pg_authid,
-- passwords, role configuration values, connection strings or private keys.
BEGIN TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY;
SET LOCAL search_path = pg_catalog;
SET LOCAL statement_timeout = '15s';

-- 1. Actual owners and RLS flags; schema_migrations existence/owner only.
SELECT current_database() AS database_name, pg_get_userbyid(d.datdba) AS database_owner,
       n.nspname AS schema_name, c.relname AS object_name,
       pg_get_userbyid(c.relowner) AS object_owner,
       pg_get_userbyid(n.nspowner) AS schema_owner,
       c.relrowsecurity AS rls_enabled, c.relforcerowsecurity AS rls_forced
FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
CROSS JOIN pg_database d
WHERE d.datname=current_database() AND n.nspname='public'
  AND c.relname IN ('domain_users','evt_employer_config','evt_requests',
                   'employer_hr_assignments','schema_migrations')
  AND c.relkind IN ('r','p')
ORDER BY c.relname;

-- 2. Candidate identities/capability flags only; no rolpassword or rolconfig.
-- Operators must identify which identities the API/migration process uses.
SELECT rolname, rolcanlogin, rolsuper, rolcreatedb, rolcreaterole,
       rolinherit, rolbypassrls
FROM pg_roles WHERE rolname !~ '^pg_'
ORDER BY rolname;

-- 3. Membership paths, including predefined roles inherited by custom roles.
SELECT parent.rolname AS granted_role, member.rolname AS member_role,
       m.admin_option, m.inherit_option, m.set_option
FROM pg_auth_members m
JOIN pg_roles parent ON parent.oid=m.roleid
JOIN pg_roles member ON member.oid=m.member
WHERE parent.rolname !~ '^pg_' OR member.rolname !~ '^pg_'
ORDER BY member.rolname,parent.rolname;

-- 4. Direct table/column/schema ACLs and defaults for future tables in public.
-- Effective privileges must be evaluated with the memberships above.
WITH objects AS (
 SELECT c.oid,c.relname,c.relowner,c.relacl,n.nspname FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
 WHERE n.nspname='public' AND c.relname IN
 ('domain_users','evt_employer_config','evt_requests','employer_hr_assignments','schema_migrations')
 AND c.relkind IN ('r','p')
), entries AS (
 SELECT 'table'::text AS scope,format('%I.%I',o.nspname,o.relname) AS object_name,
        NULL::text AS column_name,a.*
 FROM objects o,LATERAL aclexplode(coalesce(o.relacl,acldefault('r',o.relowner))) a
 UNION ALL
 SELECT 'column',format('%I.%I',o.nspname,o.relname),col.attname::text,a.*
 FROM objects o JOIN pg_attribute col ON col.attrelid=o.oid AND col.attnum>0 AND NOT col.attisdropped,
 LATERAL aclexplode(col.attacl) a
 UNION ALL
 SELECT 'database',d.datname,NULL::text,a.*
 FROM pg_database d,LATERAL aclexplode(coalesce(d.datacl,acldefault('d',d.datdba))) a
 WHERE d.datname=current_database()
 UNION ALL
 SELECT 'schema',n.nspname,NULL::text,a.*
 FROM pg_namespace n,LATERAL aclexplode(coalesce(n.nspacl,acldefault('n',n.nspowner))) a
 WHERE n.nspname='public'
 UNION ALL
 SELECT 'default tables',coalesce(n.nspname,'all schemas') || ' / owner ' || pg_get_userbyid(d.defaclrole),NULL::text,a.*
 FROM pg_default_acl d LEFT JOIN pg_namespace n ON n.oid=d.defaclnamespace,
 LATERAL aclexplode(d.defaclacl) a
 WHERE d.defaclobjtype='r' AND (d.defaclnamespace=0 OR n.nspname='public')
)
SELECT scope,object_name,column_name,pg_get_userbyid(grantor) AS grantor,
       CASE WHEN grantee=0 THEN 'PUBLIC' ELSE pg_get_userbyid(grantee) END AS grantee,
       privilege_type,is_grantable
FROM entries ORDER BY scope,object_name,column_name,grantee,privilege_type;

-- 5. RLS policy metadata only. Expressions are intentionally not exported:
-- they may contain literal values. Enabled RLS blocks the binding template
-- until its exact policy definitions receive separate approved review.
SELECT p.polrelid::regclass AS on_table,p.polname,p.polcmd,p.polpermissive,
       ARRAY(SELECT CASE WHEN r=0 THEN 'PUBLIC' ELSE pg_get_userbyid(r) END
             FROM unnest(p.polroles) r) AS policy_roles,
       p.polqual IS NOT NULL AS has_using_expression,
       p.polwithcheck IS NOT NULL AS has_check_expression
FROM pg_policy p JOIN pg_class c ON c.oid=p.polrelid
JOIN pg_namespace n ON n.oid=c.relnamespace
WHERE n.nspname='public' AND c.relname IN
 ('domain_users','evt_employer_config','evt_requests','employer_hr_assignments')
ORDER BY p.polrelid::regclass::text,p.polname;

ROLLBACK;
-- Catalogs cannot establish which migration runner/CI command is used. Supply
-- its tool/version, migration ordering/version-recording convention, target
-- schema/search_path and role identity separately, without connection secrets.
