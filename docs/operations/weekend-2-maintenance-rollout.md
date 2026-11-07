# Weekend 2: maintenance rollout for owner review

Status: **executed after separate owner approval**. PR #130 was squash-merged and deployed as `f52f5bf282735ae24ac91d3b3855e3e5b0d5de13`; the subsequent provider transition and Checkpoint 24 passed. No rollback was executed. The owner reconfirmed the existing Checkpoint 24 evidence on 2026-11-07 without rerunning it. See the [recorded production outcome](../security/weekend-2-closeout.md#recorded-production-outcome) for evidence and remaining acceptance gaps. Proposal B remains HIGH; this is not public-release security clearance.

The procedure below is retained as the approved rollout/recovery record. It is not an instruction to repeat the completed rollout.

## Verified prerequisites

| Prerequisite | Evidence |
| --- | --- |
| Actual schema and least-privilege assignment bindings | Production evt transaction 2854; provider binding transaction 2855; fresh verification passed. Only 000014 recorded; no earlier migration history invented. |
| Restricted replacement role authentication and protected configuration | Transaction 2859; real SCRAM correct/incorrect-password and privilege/schema checks passed; root:root 0600 environment; persistent JWT/HMAC/Kratos/courier settings preserved. |
| Exposed legacy owner credential contained | Transaction 2860; evt-app NOLOGIN, exposed credential rejected, zero legacy sessions/terminations, ownership/grants unchanged, replacement connections preserved. |
| Deployment executor | Direct GitHub SSH deployment correlated to root; username/key secret metadata unchanged since before that successful deployment. No key/value retrieval or workstation SSH access. |
| Actual backups/recovery | Both private database dumps restored in the isolated network-none clone; real-schema authorization, revocation and both concurrency orderings passed. Checkpoint 21 verified all three original service connections, retained images and the exact private recovery Compose model. No rollback executed. |
| Application/build tests | Full Go/PostgreSQL tests, relevant race tests, vet, module verification and targeted regressions passed; CI 41394455604 passed for 12a7b36. Final PR head and resulting main must independently pass their CI gates. |
| Targeted dependency fixes | Go 1.26.10 / x/net v0.60.0; source, Linux candidate and final Docker binary checks exclude both targeted advisories. Other findings remain documented. Running production compiler acceptance is still pending. |

Private recovery directory: `/opt/cvera/weekend2-closeout-20261102T004322Z`. The credential-bearing `rollout-recovery.override.json` and captured `rollout-recovery.compose.yml` are mode 0600 under 0700; hashes/image IDs and verification outcomes are in `rollout-recovery.json`. Never print, upload, commit or put their contents in command arguments. Protected live configuration remains separate. No signing private key was copied or inspected by these preparations.

## Explicit release-risk assessment

**Proposal B is still HIGH.** Authenticated callers can obtain upload URLs for arbitrary external cases because no authoritative principal-to-case entitlement model exists. No model, evidence workflow, acceptance lifecycle or expiry was invented or changed. Existing ten-minute capabilities cannot be revoked by a database change; transfers begun before expiry may finish later. Authenticated completion remains 501, not proof that other consumers enforce evidence acceptance.

The rollout adds authentication to already public evidence routes and creates no new route, public port, authentication provider or case permission. API publication remains 8080; Kratos admin host binding becomes loopback. No material increase in Proposal B exposure was found in the reviewed deployment diff. The owner must explicitly accept a maintenance rollout with this documented residual risk; public release and independent Weekend 2 acceptance remain blocked. Do not claim the repository/API is security-cleared.

No employer assignments have been created. All employer access without a reviewed explicit assignment must deny after rollout, including existing HRReviewer accounts. Do not seed or broaden access to improve smoke-test results. Registered identities, cryptographic/session secrets, role ownership, data and migration history are preserved.

## Proposed execution sequence after separate approval

1. Recheck the exact PR head, clean worktree, expected base/main and all required checks. Review only changes since the recorded review; do not repeat the completed audit. Require verified private recovery records/artifacts and unchanged approved Compose/configuration. Stop on drift or unmet prerequisites. Use the established squash-merge convention, record the actual resulting main SHA and verify the merged tree contains the reviewed fixes. No direct main commit or auto-merge bypass.
2. Merge only after owner rollout authority and green final-head CI. Main push automatically runs the existing CI and exact-revision SSH deployment through root. Require the main run's test/release-build verification before its deployment job; `scripts/deploy-api.sh` fetches authoritative main, checks out that exact SHA, builds and recreates API only, then checks health/version. Do not bypass exact-main guards or add an ad hoc privileged migration job. Schema work is already applied; do not reapply any migration.
3. Verify API startup/health and exact version SHA, actual configured restricted DB identity, absence of the unused administrator alias, effective schema/function permissions and database connectivity. Inspect only sanitized log/error metadata. Check unauthenticated/invalid-token evidence routes reject before presigning, with empty synthetic requests and no upload or attestation. Keep any authenticated completion smoke test at 501. Do not manufacture a production JWT or access grant.
4. Through the established owner console, verify cached Kratos image identity and protected configuration, then target only the existing identity services using the merged main Compose:

   ```sh
   cd /opt/cvera/rsa-attestation-engine/apps/evt-api-go
   docker compose up -d --no-deps --no-build --pull never kratos kratos-courier
   ```

   This consumes the verified prepared provider DSN without migration, image upgrade, volume recreation, identity reset or secret rotation. Verify both use cvera_kratos_runtime, readiness/courier state and sanitized errors, with admin publication loopback. API-only automatic deployment does not perform this step. Any unexpected dependency/permission error stops acceptance; do not broaden grants without confirmed evidence/review.
5. Verify actual service identities/connections and unchanged legacy NOLOGIN state after transition. The owner should exercise normal existing-account login/exchange/session behavior through the existing UI, returning statuses only, without creating accounts or changing roles/status. Existing identities and secrets remain unchanged. Keep a test marked outstanding if owner interaction is unavailable; invalid-token probes alone do not establish successful production authentication.
6. Record the actual deployed binary's compiler/module/revision metadata and CA hash using only the public binary/bundle and the already cached builder tools where needed. Targeted compiler/module fixes must be present in the deployed build; CI success alone does not identify the running image. Verify public JWKS/key continuity without opening the private PEM or creating a production attestation. Positive issuance/verification is proven locally with a temporary RSA-3072 key; live production issuance and full legacy normalization/status remain explicitly unverified under the current no-production-attestation constraint.
7. Update PR/report evidence with actual merge/deploy SHAs, CI, runtime and smoke results, remaining limitations and any recovery. Do not mark Weekend 2 complete solely for successful deployment: Proposal B prevents independent security acceptance. No new product work is added to the frozen plan.

## Proposed bounded emergency recovery

Checkpoint 21 proves retained images/fresh original credentials/private model availability; it does not execute a recovery or prove every old-image workflow under changed database privileges. The captured model restores the exact original service environments, avoiding an assumption that an old image supports the replacement privilege profile. Its source Compose preserves the read-only signing mount and current network boundary; admin remains loopback. Database rows/schema and legacy NOLOGIN remain intact.

**Material consequence:** restoring the old API image returns the pre-remediation application weaknesses; captured original environments restore postgres service connections. This is a temporary availability fallback to the prior baseline, never a secure accepted state. The owner must explicitly authorize this consequence along with rollout/recovery. No automatic fallback is authorized by preparation alone.

If authorized and needed, first stop/cancel any still-running deployment action to prevent competing recreation. Verify private artifact hashes and cached image IDs against the recovery record. Target only affected services and preserve the protected live environment, schema, role containment and data:

```sh
docker compose --project-name evt-api-go \
  --project-directory /opt/cvera/rsa-attestation-engine/apps/evt-api-go \
  --env-file /opt/cvera/rsa-attestation-engine/apps/evt-api-go/.env \
  -f /opt/cvera/weekend2-closeout-20261102T004322Z/rollout-recovery.compose.yml \
  -f /opt/cvera/weekend2-closeout-20261102T004322Z/rollout-recovery.override.json \
  up -d --no-deps --no-build --pull never api
```

For an identity-service failure, the same verified command may target `kratos kratos-courier` only after checking the affected scope. No database restore, volume deletion, assignment mutation, LOGIN re-enablement, password change, migration rollback or ownership/grant broadening is included. Verify returned health/version against the captured baseline (API 2813e7e) and report failed rollout/recovery accurately. The exact-main deploy script cannot accept the old SHA as a rollback. Destructive recovery or an unverified dependency requires a separate owner decision.
