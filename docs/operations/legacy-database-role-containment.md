# Confirmed legacy database credential exposure: proposed containment

Status: **HIGH release blocker; production containment requires a separate owner decision.** The existing credential/configuration approval covered only the two replacement runtime roles and explicitly preserved the legacy role. No legacy role change has been executed.

## Confirmed evidence

The removed tracked Kratos DSN in main 2813e7e contains a publicly known legacy credential. Owner console checkpoint 19 privately loaded that historical configuration and attempted a Docker-network SCRAM connection to evt_kratos as evt-app. Authentication succeeded and SELECT current_user returned the expected role. Only classification/boolean metadata was emitted; no application row was read or changed. The historical credential is not repeated here.

Earlier authoritative metadata establishes that evt-app owns evt_kratos and all 26 identity tables and has CRUD on eight API tables in evt. It is LOGIN, not a superuser, with no memberships. This is a confirmed usable exposed credential, not a finding inferred from missing tooling. Potential impact is unauthorized identity-store and legacy API data access/modification. Internet reachability through the host-published PostgreSQL port remains unverified; the successful internal-network authentication and owner authority are established. Do not infer OS access, production signing-key compromise or observed malicious use.

Replacement cvera_api_runtime/cvera_kratos_runtime credentials and permissions are independently verified, but they do not revoke this still-valid owner credential. Removing the DSN from the current tree does not contain it. The running API/Kratos/Courier still use postgres; their prepared configuration uses the replacement roles. No observed production service depends on evt-app. A fresh consumer/session check is required before changing it.

## Smallest proposed owner-approved action

Through the existing root console, as postgres:

1. Require the completed SQL and runtime-credential preparation records. Recheck all three running service connection identities and the protected prepared connection identities; none may reference evt-app. Reject unreviewed consumers. Query cluster-wide pg_stat_activity metadata for evt-app; if any sessions are present, stop for operator review before changing the role. Do not kill sessions under this proposal.
2. Write an exclusive private containment intent record, without credential/verifier values. Capture the legacy role OID, LOGIN flag, owners/ACL metadata and replacement-role status; retain the existing private recovery material.
3. In a bounded operator transaction acquire the change-specific advisory lock, recheck role/consumer/session prerequisites and perform only:

   ```sql
   ALTER ROLE "evt-app" NOLOGIN;
   ```

   Preserve the role, passwords, memberships, ownership, existing/default privileges, all identity/application rows and running services. Record the actual transaction; do not manufacture a migration version or modify vendor history.
4. Verify the committed NOLOGIN state, unchanged ownership/grants and no remaining evt-app sessions from fresh connections. Reattempt the known exposed credential privately over the same SCRAM host path; it must fail specifically because login is prohibited. Verify both replacement runtime credentials still connect to their intended databases and retain the reviewed privilege/schema boundaries. Emit only metadata/booleans.
5. If a session appears during/after revocation, leave NOLOGIN in place and stop release/rollout for owner review. NOLOGIN does not terminate existing sessions; session termination is outside this guarded proposal. No complete containment claim is made while sessions remain. Do not automatically re-enable a compromised credential to recover a failed check.

NOLOGIN affects authentication, not database/table ownership or direct replacement-role grants. Do not DROP the owner role, transfer ownership, remove legacy grants, reset registered identities, alter postgres credentials or rotate cookie/JWT/signing secrets. A future request to re-enable this role must first replace its exposed password under a separately reviewed purpose; the known password must never regain login capability.

## Failure and recovery

An uncommitted role transaction rolls back normally. If COMMIT is ambiguous, inspect the role/session metadata and private intent rather than retrying. After confirmed NOLOGIN commit, retain containment even if a later verification fails and hold rollout. Current observed services do not use that role, so service restart, database restoration or application-data rollback is unnecessary. Unexpected consumers/sessions require a specific owner decision; no automatic termination or return to LOGIN is authorized here.

## Local verification

A disposable local PostgreSQL database was owned by a LOGIN legacy role, with a separate restricted runtime grant. Both credentials authenticated over SCRAM before containment. After ALTER ROLE NOLOGIN, the same legacy password was rejected specifically as not permitted to log in; the replacement role still authenticated. Database/table ownership and table ACLs were byte-for-byte unchanged, with zero legacy sessions and no application rows modified. All disposable objects were removed. This verifies the proposed mechanism, not an unexecuted production containment.
