# Weekend 2: approved runtime credential preparation

Status: **production preparation completed in checkpoint 18, activation transaction 2859.** The owner separately authorized fresh runtime credentials, LOGIN and protected configuration preparation, delegated verification of the established deployment identity and retained the prohibition on service operations, merge and deployment. No additional execution approval is required for this preparation scope.

## Evidence and resolved deployment identity

Production checkpoint 15 committed and freshly verified actual 000014/API bindings in evt transaction 2854 and provider bindings in evt_kratos transaction 2855. The new roles are restricted NOLOGIN. Checkpoint 16 confirmed reviewed Compose preserves every configured non-connection setting for API, Kratos and Courier, including persistent JWT/HMAC, Kratos and SMTP settings. Provider connection routes agree. New connection URLs can preserve the existing host, database and TLS/query settings while changing only username/password.

The live environment file is UID 501/GID 50, mode 0644, with no corresponding OS owner account. General root logins alone were insufficient. The established deployment account is now verified as **root** through the following combined evidence, so the protected environment uses **root:root / 0600**, without new accounts, SSH keys, memberships or access-control changes:

- Workflow main.yml connects directly as CVERA_DEPLOY_USER and invokes the exact-revision deployment script without sudo.
- Successful [run 41320539163](https://github.com/kola-white/rsa-attestation-engine/actions/runs/41320539163), job 124890020290, deployed current main 2813e7e; its SSH step ran 2026-11-01 15:20:16–15:28:01 UTC.
- Owner checkpoint 17 recorded a unique matching root public-key authentication at 15:20:16.772184 UTC and root session-open at 15:20:16.776494, both sshd PID 3015574. The current API started at 15:27:55.696917146, inside that same deployment step.
- GitHub secret metadata (names/timestamps only) shows CVERA_DEPLOY_USER unchanged since 2026-09-20T21:20:26Z and its SSH key unchanged since 21:20:57Z that day, before the proven deployment. No secret value or private key was retrieved and no SSH attempt was made.

The driver rechecks unique matching authentication/session-open and the current API deployment timestamp before credential operations; ambiguity fails closed.

## Owner-authorized scope

- Provision fresh independent credentials for existing cvera_api_runtime and cvera_kratos_runtime only, then enable LOGIN on those two restricted roles. cvera_hr_operator stays NOLOGIN.
- Stage and atomically publish protected configuration containing the new EVT_DB_DSN and KRATOS_DB_DSN, preserving all unrelated values and connection-routing/TLS options.
- Verify real SCRAM connections, effective authorization/privilege boundaries and Compose/environment compatibility without changing any application record or restarting any service.
- Keep original postgres/evt-app passwords, ownership and legacy grants unchanged. No registered identity, user password, session, cryptographic signing key, HR assignment or existing migration record is modified.

This is credential/configuration preparation, not rollout. All existing containers retain their current connections until a subsequent authorized recreation. No public-release clearance is implied; Proposal B remains HIGH.

## Ordered execution design

1. Require the owner root console, completed private SQL record, unchanged API/identity container images and the successful configuration preview. Recheck the two runtime roles are NOLOGIN with no unsafe capabilities, ownership or memberships; operator remains NOLOGIN. Hold merge and all service operations. Verify the current environment still matches the recovery snapshot or make a new private current snapshot without overwriting the original. Record owner approval, source commit/digests, deployment-account confirmation, start time and phase in an exclusive root-private record. Stop on unexpected state or any existing/ambiguous activation record; no automatic retries.
2. Generate two independent secrets locally using Python secrets.token_urlsafe(48). Never print them, put them in command arguments/Git/CI output or send them off the Droplet. Replace only role/password in the live API/provider connection URI structures; preserve host, port, database, sslmode and other reviewed query parameters. Reject unsupported URI/startup overrides rather than dropping them. Use the shared provider route for Kratos and Courier.
3. Create a root-private candidate environment in the existing 0700 recovery directory (exclusive creation, file 0600). Preserve unrelated lines/values/comments verbatim. Replace the single EVT_DB_DSN assignment and add/replace KRATOS_DB_DSN; reject duplicate/multiline/unsupported assignments. Quote the generated URI values literally. Render reviewed Compose against that candidate in memory; require all non-connection environment values unchanged from the successful checkpoint and the exact two intended restricted connection identities. Confirm the unused API DATABASE_URL alias remains absent from the rendered API, provider crypto/cookie secrets unchanged and required secrets nonempty. Do not publish or activate on a mismatch.
4. With postgres through container-local psql -X and ON_ERROR_STOP, provision each new role's password using psql's client-encrypting `\password <role>` command with password/confirmation supplied privately on stdin. Set that owner session's password_encryption to scram-sha-256 before the command. Passwords are never interpolated into SQL or command arguments; command output is captured privately, not emitted. Keep the roles NOLOGIN until both password operations succeed. Do not alter any existing postgres/evt-app credential or grant.
5. In one bounded owner transaction verify role prerequisites again, then perform only:

   ```sql
   ALTER ROLE cvera_api_runtime LOGIN;
   ALTER ROLE cvera_kratos_runtime LOGIN;
   ```

   No role capability, ownership, membership or privilege is added. Record the actual committed transaction and reconcile an ambiguous COMMIT rather than retrying.
6. Test the two real credentials over the Docker-network host path subject to the verified SCRAM rule, using the original route and database. Local socket/loopback trust is not password verification. Supply PGPASSWORD privately through docker exec's environment-name forwarding; pass a password-free URI to psql. Never emit DSNs, passwords, verifiers, environment dumps or raw failed command output. Require correct passwords accepted and deliberately incorrect passwords rejected. Verify current_user/current_database, normal schema resolution, restricted flags/no memberships and effective ACLs. API authorization probe must use a NULL UUID and return false in READ COMMITTED; no existing row is returned/altered. Recheck assignment direct/admin access denied, provider ledger read-only and API identity-table access denied using catalog predicates. Operator remains NOLOGIN. No INSERT/UPDATE/DELETE probe on production application tables is necessary.
7. Only after the verified deployment account/read-access decision, atomically replace the live environment from a same-directory temporary file with root:root / 0600 under the root-account proposal. Flush/fsync the file before rename and its directory afterward; preserve the old bytes privately for recovery. Render configuration again and verify the two intended roles and all unrelated values. Retain credentials only in the protected candidate/live files. The current services remain running with unchanged environments; no build, compose up, restart, image pull, migration or deployment occurs in this phase.
8. Record only metadata and actual verified outcomes, including runtime LOGIN/operator NOLOGIN, private environment permissions/hash, credential login/denial and configuration preservation. Check production Git worktree remains clean and no credential file entered the repository. Return a compact PASS result. Effective production runtime isolation is still pending recreation and post-deployment checks; schema plus successful login is not proof the running services have switched.

## Recovery before rollout

Stop on any unexpected result. Preserve the intent/candidate/partial-state record so an operator can reconcile exactly which steps committed. No existing service credential was changed, and current containers still use their original settings.

Before any new-role service has been started, the reviewed recovery is:

1. Confirm no running service has switched and no remaining connection uses either new runtime role. Do not terminate unknown sessions automatically.
2. Atomically restore the saved environment bytes with the approved private ownership/0600 strategy; do not restore the original world-readable permissions/unmapped ownership.
3. In one bounded owner transaction set the two new runtime roles back to NOLOGIN. Leave their new credentials, ownership, migration, ACLs and history intact. No DROP, credential reset, database restore or user-account operation is needed. Recheck roles/zero new-role sessions and record recovery. Existing containers continue unchanged.

This pre-rollout recovery is not the post-deployment rollback procedure. Once any service has switched, use the separately reviewed image/configuration recovery sequence; disabling its active database role first would break it. The existing exact-main deployment script cannot deploy an old SHA as rollback. Do not merge until the later service-transition/recovery procedure and deployment prerequisites are verified.

## Verification already performed locally

- PostgreSQL 16 disposable role: client-encrypted private-stdin password preparation while NOLOGIN, SCRAM correct-password acceptance/incorrect-password rejection over the Docker-network address, then NOLOGIN recovery denying further login. No production access.
- Synthetic environment plus actual reviewed Compose: only both connection values replaced; unrelated bytes/comments/settings preserved, including a literal-dollar secret; parsed non-connection settings unchanged; mode-0600 atomic publication and byte-exact/private rollback passed. This establishes preservation, not an unexecuted production credential transition.
- Production checkpoint 16: all three persistent-environment comparisons passed; provider routes matched; unused API administrator alias absent from the reviewed rendered configuration. No credential or service state changed.

## Guarded execution and confidentiality verification

`operations/prepare-weekend2-runtime-credentials.py --prepare-approved-1c228ee` implements this approved phase through the existing root console. It uses the checksum-pinned SQL/preview helpers, preserves private current environment/candidate files, records intent before activation, checks configuration before publication and stops on unexpected state. An existing intent/candidate prevents blind retries. Only metadata is returned. The original verified backup/recovery material is retained.

Before setting passwords it refuses unreviewed preload/audit hooks. Each short-lived password-setting owner connection uses SCRAM encryption and session-only controls excluding credential statements/verifiers from statement, duration/sampling and error-statement logs; global/application logging is unchanged. Unknown auditing is not silently disabled. Passwords enter client stdin, never SQL interpolation or arguments. See PostgreSQL 16 [psql password handling](https://www.postgresql.org/docs/16/app-psql.html) and [logging controls](https://www.postgresql.org/docs/16/runtime-config-logging.html). Correct-password and specifically password-authentication-failed negative probes both must succeed as expected; an arbitrary connection error does not count as bad-password rejection.

Local disposable PostgreSQL/Compose regressions passed the actual provisioning, SCRAM, effective-role/ACL/schema checks, private candidate publication/restoration, duplicate-assignment rejection and NOLOGIN recovery. With verbose statement/duration logging enabled in the fixture, neither the generated password nor its SCRAM verifier appeared in server logs. Root chown was asserted/simulated on the unprivileged workstation; the real root console driver must assert UID/GID 0 and mode 0600 after publication before success. Identity tests used the exact owner-observed metadata and actual historical workflow and rejected ambiguous sessions. Production checkpoint 18 subsequently passed actual credential/privilege/schema verification and root:root 0600 publication, with unchanged running services.

## Production result and subsequent gate

Checkpoint 18 passed all documented assertions; record runtime-credentials-1c228ee.json is retained privately. Both runtime roles are LOGIN, operator remains NOLOGIN, credentials and protected configuration are ready, and services were not restarted. Do not re-run the preparation driver.

Checkpoint 19 separately confirmed the publicly known legacy evt-app credential still authenticates as the identity-database owner. That role was intentionally unchanged by the preparation approval. Its [NOLOGIN containment proposal](legacy-database-role-containment.md) is a new separate decision and a HIGH release blocker. Do not merge/deploy or describe replacement runtime preparation as containment of this exposed legacy credential.
