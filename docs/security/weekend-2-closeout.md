# Weekend 2 closeout verification

Owner-authorized closeout follow-up to PR #130 on `security/weekend-2-v1-remediation`, reviewed from `b96988f` against authoritative `origin/main` `2813e7e513b6afed5cad3b77bee6363e982e21d2`. The older reports retain historical evidence; this report records the new build/operational work. **Production migration, merge and deployment remain gated; Weekend 2 is not closed or public-release security-cleared.** Proposal B is deliberately preserved as an acknowledged high-severity gap.

## Full PR review

The complete PR diff, added SQL, operator code, regression fixtures and deployment/publish workflows were reviewed. No further confirmed material application defect was identified in the implemented fixes. Review scope and evidence:

| Area | Result and limits |
| --- | --- |
| Authentication | Both Kratos exchanges verify active server-side sessions; canonical local role/status is read from the upsert transaction and inactive accounts are denied before session creation. JWT type/algorithm/issuer/audience/expiry/subject tests retained. |
| Refresh/logout | Common session lock precedes token mutations; revoked sessions deny refresh. Rotation/reuse/logout and concurrent races pass against PostgreSQL. Serialization conflicts fail closed; access JWTs still expire naturally within 15 minutes. |
| Employer authorization | All three repository operations use authenticated principal UUID plus exact employer assignment, current active HR status and existing request/workflow checks. No role/caller ID alone authorizes access. Both revocation orderings, direct operator SQL, denied signing and assigned temporary-key signing are covered. |
| SQL security | 000014 is atomic and once-only; bounded SECURITY DEFINER routines retain trusted ownership, typed arguments, qualified object references and fixed pg_catalog/trusted-schema/pg_temp paths. Runtime gets only check EXECUTE; new-table default CRUD/PUBLIC execution is stripped. Legacy DML is unchanged. Binding, parent types/FKs, RLS/isolation rejection, temporary collisions, failure rollback and rerun preservation pass. |
| Evidence | All four paths authenticate in the candidate; both completion aliases remain authenticated 501, now explicitly tested. The arbitrary-case init reproduction still succeeds as gap evidence. No entitlement model, upload behavior, limits, hashing lifecycle or expiry was changed. |
| SSRF/body/config | Actual dial addresses are validated/pinned, redirects retain the same transport, proxy bypass is disabled, and existing HTTPS/timeout/body controls remain. Internal-destination, CORS, role and 1 MiB body-cap regressions pass. Kratos fallback credential is removed and the admin port is loopback in source; an API-only deployment does not recreate an existing Kratos container. |
| Signing/JWKS | Assigned local HTTP issuance uses a temporary RSA-3072 key and real signer/verifier paths, rejecting tamper/wrong/unknown keys. A fresh public-only JWKS fetch returned 200, one RS256 kid `issuer-prod-2026-10`, matching the committed RSA-3072 public PEM. No production private key was accessed or production attestation created. Legacy normalization/status/live-issuance limits remain. |

## Release dependency corrections

The former floating `golang:1.25` builder currently resolves to **Go 1.25.14** (official image index digest `sha256:699337d620559a59b4a2bb298ad59611e535d2ee755a34cf2d2a98f37578dc80`, local native linux/arm64 probe). This identifies today's builder, not the compiler in the previously deployed binary. Go 1.25 is no longer the supported patched line for the target advisories.

Docker and CI now use **Go 1.26.10**. Docker disables automatic toolchain replacement. This is the older supported release line with the required fixes, rather than an unrelated latest-feature upgrade. The module minimum is Go 1.26.0 because the necessary `x/net` security update requires it.

- [GO-2026-6611](https://pkg.go.dev/vuln/GO-2026-6611): Go 1.26.10 contains the stdlib HTTP/2 fix (fixed 1.26.9). The first patched-compiler scan still found the independent `x/net/http2` implementation linked through Gin. Updated **x/net v0.43.0 → v0.60.0**, the advisory's fixed version. Its required module graph also updates x/crypto, x/sys, x/text, x/mod, x/sync and x/tools; these are recorded in [build results](weekend-2-closeout-build-results.json). No pgx, Gin, QUIC or AWS upgrade was bundled. Public-destination checks alone were not accepted as a protocol fix.
- [GO-2026-6809](https://pkg.go.dev/vuln/GO-2026-6809): Go 1.26.10 includes the crypto/x509 intermediate-certificate distrust update. This is a compiler/library correction, not proof that the host's or previously running container's CA store was current. Candidate Linux/amd64 source/binary scans report neither target advisory; final Docker CI verifies the exact compiler/target and archives the bundled CA-file hash. Production disposition remains pending deployment of that verified build.

The saved original 16-finding output is unchanged. The final source scan reports four symbol-level advisory IDs (QUIC 4233/5676, pgx 5004 and AWS EventStream 5764); the Linux binary scan also reports QUIC 4017 and pgproto3 Bind.Decode 4771 because binary scanning lacks source call-path precision. Their triggering QUIC listener/transport, simple-protocol query shape and EventStream operations remain absent from the inspected configured paths, as documented in the [existing triage](weekend-2-dependency-triage.md). pgproto3 Bind.Decode is a PostgreSQL frontend-message decoder for a server-side Backend; the application is a PostgreSQL client and contains no NewBackend/Backend.Receive/server implementation. No live exploit is asserted.

Module-only findings include pgproto3 4772, deprecated OpenPGP 5932 and x/net HTML 6860/6866. OpenPGP is not imported by the Linux server dependency build. x/net/html is imported through mimetype charset detection, which calls NewTokenizer; the affected Parse/ParseFragment tree construction and Render calls are absent from both source and binary symbol findings. Tokenization is not the advisory's affected tree-coalescing/rendering operation. QUIC 4017 has the same absent runtime protocol prerequisite as the existing QUIC findings. These dispositions do not erase scanner output or certify the dependencies universally safe. Required x/text v0.42.0 also removes the previous normalization/PRECIS findings. Original raw scanner evidence and the new summary are retained separately.

## Commands and results

Go working directory: `apps/evt-api-go`; toolchain selected with `GOTOOLCHAIN=go1.26.10`. The existing private local test helper supplies only the guarded disposable PostgreSQL DSN in memory. All final Go checks ran **after** x/net and its required module updates.

| Command | Observed result |
| --- | --- |
| `go test ./... -count=1 -timeout=120s` with local test database | PASS; includes actual SQL migration/binding, authentication, revocation/concurrency and local RSA signing tests. |
| `go test -race ./internal/httpapi ./internal/auth ./internal/employeraccess -count=1 -timeout=120s` with local test database | PASS. |
| `go vet ./...` | PASS. |
| `go mod verify` | PASS, all modules verified. |
| `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags '-X github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/buildinfo.GitSHA=closeout-candidate' -o /private/tmp/cvera-closeout-linux ./cmd/server` | PASS. Candidate has modified-worktree metadata and is not labelled an exact committed/deployed image. |
| `go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 -json ./cmd/server` | PASS execution; findings retained, neither target advisory reported. JSON-mode exit 0 does not mean no findings. |
| `go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 -mode=binary -json /private/tmp/cvera-closeout-linux` | PASS execution; neither target advisory reported. Six other symbol-level IDs remain dispositioned above. |
| `python3 apps/evt-api-go/operations/test-employer-recovery.py` (repository root) | PASS: backup mode 0600, fresh database restore with legacy ACL, ROLLBACK absence, COMMIT/bounded binding; cleanup completed. Synthetic parent subset only. |
| `npm run test:ci` | PASS, including expected rejection of invalid vector. |
| `git diff --check` | PASS. |
| `git fsck --no-reflogs --connectivity-only` | PASS, no integrity errors; existing dangling objects reported and left untouched. |

The [summary JSON](weekend-2-closeout-build-results.json) records scanner configuration, versions, candidate hash and all reported IDs by scan level. The authoritative GitHub test job previously passed at b96988f but skipped database integration. The updated existing CI job starts disposable PostgreSQL 16 so those tests run, builds the final Docker image, scans its extracted Linux/amd64 executable and records binary/compiler/CA hashes in the `api-release-verification-<SHA>` artifact. It fails if either target finding remains or expected compiler/scan metadata is absent. Check the exact final PR head's CI result; the old success is not acceptance of these new changes. No scanner service, application endpoint or product capability was added.

## Production baseline and deployment gate

Read-only public baseline probes returned:

- `/healthz`: HTTP 200, `ok=true`; `/version`: HTTP 200, SHA `2813e7e513b6afed5cad3b77bee6363e982e21d2`.
- Both evidence init routes: HTTP 400 for an invalid bearer token with an empty object; both completion aliases: HTTP 501. The empty init body cannot pass resource validation or issue a URL. These responses are consistent with the currently deployed main code lacking the new route authentication. No valid session, object upload, application-record mutation or attestation was used.
- Public JWKS: matches the committed public key as above. These are **pre-deployment observations**, not post-deployment acceptance.

This change reduces access to already public evidence routes through authentication and adds no routes, ports or networking publication. No material increase in Proposal B exposure was found in the PR/deployment diff. Case permission remains missing; a valid authenticated user can still request arbitrary-case URLs. Already issued URLs remain usable until their unchanged ten-minute expiry; transfers started while valid may finish later. Completion remains unavailable, not an evidence-acceptance security certification for other consumers.

The owner confirmed there is no production migration/versioning process and authorized a controlled first-change design. The [updated procedure](../operations/employer-assignment-migration-procedure.md) covers private backup, isolated restore, bounded transaction rehearsal, explicit role binding, owner-controlled recording of **only actual 000014**, post-commit verification and recovery. No earlier migration history is invented and no production ledger is fabricated. Its mechanical backup/restore/rollback/binding steps pass locally against synthetic fixtures. A full production restore and live privileges/schema lookup remain unverified.

**Current gate:** the actual privileged production migration/binding requires a separate owner decision and authenticated execution channel. The available local SSH identity was denied by the verified deployment host; credentials were not retrieved from GitHub secrets or logged. The workflow has SSH deployment credentials, but no migration step. The API cannot safely deploy against the known pre-000014 schema: employer operations would return 500 even while health/version pass. Therefore no merge or main-triggered deployment is performed until compatibility is established. Do not bypass this by weakening authorization, inventing grants, creating a privileged workflow probe or labelling the old script's exact-main-only path a rollback mechanism.

## Closeout status

Code/dependency corrections and local verification are complete; exact final Docker/CI evidence and production migration readiness govern the next action. The PR can be marked ready for review after its final checks pass, while merge remains blocked by the immediate deployment incompatibility. Proposal B is acknowledged rather than remediated and prevents a public-release security clearance. No new production commit has been deployed by this work. Weekend 2 end-to-end closeout and Weekend 3 readiness must not be claimed until the remaining current deployment gate and agreed production verification are resolved.
