# Employer identity and HR reviewer resolution

Status: UUIDv7 architectural decision accepted by the owner; HR remediation
engineering-validated locally, not deployed or production-accepted. No production
migration is introduced or executed. Existing migration fixtures were tested only
in a new disposable, RAM-backed PostgreSQL container during the separately
authorized pre-deployment validation. This document supersedes the HR demo
employer limitation in older readiness documents only after deployment acceptance.

## Identity decision

Cvera's target canonical employer identity is a system-generated UUIDv7. It is
globally unique, permanent, immutable, and independent of names, domains and
business attributes. It must never be replaced, recycled or reassigned. End users
must not allocate canonical identities. Names are mutable metadata, not identity
or authorization evidence.

Additional identifiers may eventually refer to that same identity. Their formats
are deliberately undecided; they cannot redefine the canonical identity. Future
associations require explicit namespaces and appropriate uniqueness constraints
so every supplied identifier resolves unambiguously. Implementing these rules in
the database requires a separate design review and explicit approval.

**This remediation does not make existing identifiers UUIDv7.** In particular,
`cei-uat-sim-001` remains exactly the same existing text identifier.

## Existing architecture and guarantees

| Area | Observed source contract |
| --- | --- |
| Employer configuration | `evt_employer_config.employer_id` is a text primary key. Confirmed metadata is reproduced in `internal/httpapi/testdata/proposal_a_legacy_schema.sql`; it is not a complete production schema dump. Other known fields are dispatch mode and timestamps, with no reliable employer display name. |
| Creation | `Repo.CandidateCreateDraft` calls `ensureEmployerConfig`, inserting the supplied text ID with AUTO dispatch if absent. Operator registration tools also accept explicit reviewed text IDs. There is no universal system-generated employer ID allocator. The requestor UI still accepts an ID and has demo defaults; correcting that separate creation workflow is outside this HR remediation. |
| HR authorization | `employer_hr_assignments` has a UUID assignment-instance key, UUID canonical application-principal FK, text employer FK, provenance and revocation metadata. A partial unique index permits one unrevoked grant per principal/employer. |
| Grant immutability | Migration 000014 rejects changes to grant identity/provenance, permits revocation only, and rejects further updates of revoked grants. Operator deletion is not categorically forbidden by this trigger; runtime has no assignment DML. |
| Employer immutability | Assignment FK uses ON UPDATE/DELETE RESTRICT. Request FKs and immutable request bindings also constrain referenced parents. No inspected registry trigger universally forbids renaming/deleting an unreferenced employer or reusing its former ID. Current runtime has SELECT/INSERT, not UPDATE/DELETE, on config. Identity permanence is therefore partly convention/privilege restriction, not a blanket database invariant. Uninspected production triggers are not assumed. |
| Requests and evidence | Requests carry text employer IDs. `request_case_bindings` freezes request, subject, employer, issuer, snapshot digest and case association. Evidence uploads/reviews are case/check scoped; an HR assignment neither grants upload/review access nor establishes issuer authority. |
| Authority and credentials | Authority subjects are text; immutable authorization/journal records, frozen snapshots/manifests, and signed credentials contain employer identity references. Existing signature/digest commitments cannot be rewritten during a migration. |
| APIs/audit | Existing employer endpoints accept opaque text `employer_id`; request events and credential commitments preserve historical references. No new UUID validation is applied to employer IDs. Only the authenticated application principal is parsed as UUID. |

## Remediation and API contract

New authenticated route: `GET /v1/employer/employers`.

```json
{"items":[{"employer_id":"cei-uat-sim-001"}]}
```

Empty access returns `200 {"items":[]}`. The existing global Bearer authentication
and HR role middleware apply. Missing/invalid authentication is 401; a non-HR JWT
is 403. Database failure/timeout returns sanitized 503. Responses use
`Cache-Control: private, no-store`. Query parameters cannot choose the principal.

The endpoint takes `claims.Sub` (the established `domain_users.id`, not an email
or Kratos identity), enumerates existing config rows, and filters them through
`check_employer_hr_assignment`. That existing bounded SECURITY DEFINER function
checks current active HR role/status, unrevoked assignments, READ COMMITTED and
the principal lock shared with revocation. A disabled/demoted principal with an
old HR JWT receives an empty discovery list. Discovery uses a ten-second context.

This reuses existing runtime config SELECT and checker EXECUTE privileges. It
adds no assignment-table SELECT, new SQL functions, grants, schema or registry.
Enumeration scans the config registry, so very large employer populations may
eventually justify a reviewed bounded discovery routine; that optimization is
not needed for this small pilot.

Discovery is advisory. Existing queue/detail/attest/eligibility/issuance checks
remain authoritative on every call, including after revocation and with forged
employer parameters. There is no new authorization based on cached client state,
display names, email/domain matching, or an identifier's syntax. Previously
authorized in-flight transactions retain the existing revocation lock ordering;
this does not promise cancellation of an operation already committed.

The HR screen now uses `src/api/employerReview.ts`:

- One assignment automatically selects its exact ID and loads queue/detail.
- Multiple assignments display an inline, web/native selection containing only
  discovered employers; switching immediately clears old queue/detail state.
- No assignment displays an access-unavailable state with no fallback.
- Refresh and focus rediscover assignments. Authorization failures clear cached
  employers, queue and selection; all failures remove actionable stale requests.
- Context epochs discard late responses after employer switches, focus loss or
  session changes. Duplicate decisions and context changes during a decision are
  blocked. Ambiguous mutation outcomes require refresh before another decision.
- The API currently supplies no reliable name. The screen uses neutral
  `Employer access 1`, etc., never a fabricated company name or a raw ID. The
  client can render optional `display_name` metadata if a future separately
  reviewed backend supplies it; names never influence access checks. Ordinal
  labels are session-local and can change when the authorized list changes.

No current record, Tara account, assignment, request, evidence, review, authority,
credential or audit entry is modified by discovery. Existing IDs and existing API
parameters remain valid. The only backend mutation used by the screen is the
same pre-existing decision endpoint after an explicit user action.

## Validation and files

Changed implementation: `internal/evt/employer_resolution.go`,
`internal/evt/routes.go`, `screens/HRReviewScreen.SettingsStyle.tsx`, and
`src/api/employerReview.ts`. Root `package.json` adds the new frontend tests to
the existing CI command. No dependency or lockfile change is needed.

New tests: `tests/employer-review.test.ts`,
`internal/evt/employer_resolution_test.go`, and
`internal/httpapi/employer_resolution_integration_test.go`.

Executed locally:

- `npm run test:ci`: PASS, 38 tests including 12 new employer-workflow groups.
- `npm run typecheck:expo`: PASS.
- `go test ./... -count=1 -timeout=180s -json` with the guarded disposable-test
  DSN: PASS, 226 top-level tests / 432 tests including subtests, zero failures or
  skips. Local compiler is Go 1.27.1; release CI pins 1.26.10.
- New PostgreSQL discovery test: PASS using actual restricted runtime privileges.
  Missing/single/multiple/revoked assignments, forged query parameters, current
  role/status, foreign queue denial and historical request/audit preservation
  passed. Existing protected-operation and revocation-order regressions passed.
- `go vet ./...` and `git diff --check`: PASS.
- Fresh Expo web export: PASS, output at `/private/tmp/cvera-hr-predeploy-web`,
  with dotenv/telemetry disabled and a loopback test API URL. This builds static
  assets, not a deployment or UI UAT.

Frontend tests exercise one/many/no assignments, revocation on queue/detail and
decisions, forged selection rejection, rediscovery, stale response races,
blur/session invalidation, duplicate decisions, malformed responses, exact text
ID transport, future UUID-shaped IDs and sanitized errors. Go unit tests cover
ID preservation, empty JSON arrays, database failures and authentication/role
middleware. Database transport is stubbed in those unit tests; they do not prove
SQL behavior.

The November 14 pre-deployment suite used a newly created PostgreSQL 16
container with synthetic credentials, a random loopback-only port, no persistent
mounts and a RAM-backed data directory. The container was removed afterward;
production and the pre-existing persistent local database were untouched.
Logs and the source hash manifest are retained locally under
`/private/tmp/cvera-hr-predeploy-*`; the validation report is
`/private/tmp/cvera-hr-predeployment-validation.md`. Its earlier release-order
NO-GO recommendation is superseded by the owner's subsequent explicit acceptance
of temporary frontend/backend mismatch for this one-tester MVP release.
No production UAT or mounted browser/native UI acceptance is claimed.

## Future backward-compatible UUIDv7 migration (proposal only)

1. Separately review actual production PKs/FKs, triggers, ACLs, all identifier
   consumers and collision/duplicate-entity cases. Preserve each existing record
   as a distinct identity unless an explicit separate adjudication proves a
   relationship; never merge by name/domain similarity.
2. Under separately approved schema and rollout plans, associate each existing
   identity with one server-generated UUIDv7. Keep its existing text key and all
   old relationships valid. Record attributable, immutable association history;
   repeated backfill must not generate a different UUID for the same identity.
3. Define explicit namespace/uniqueness and canonical immutability enforcement.
   Preserve old Cvera text IDs as permanent legacy references. Do not guess
   whether an input is canonical merely because it looks like a UUID: current
   text IDs can already contain UUID-shaped strings. Version or explicitly type
   new API references and reject ambiguous resolution.
4. Add compatibility resolution at trusted server boundaries before migrating
   new writes/integrations. Resolve to one identity, then enforce the same exact
   principal-employer relationship. Avoid independent alias-level grants that
   could drift or widen authorization. Preserve old clients during rollout.
5. Retain historical signed artifacts, snapshots, manifests and audit bytes
   unchanged. Their old identifiers must resolve through durable historical
   associations; any new credential profile/commitment behavior requires its
   own versioning, verifier and cryptographic review. Do not re-sign history.
6. Migrate relational consumers only in reviewed, backed-up, rehearsed stages,
   with reconciliation of grants, FK integrity, counts, old/new API behavior and
   restore/rollback. Never rename a text PK in place to pretend it was UUIDv7.

Dependencies include requestor-side employer creation/selection, operator SQL
with explicit TEXT prerequisites, FKs, authority subject strings, request-binding
identity comparisons, credential verification commitments, audit exports and
integrations. This remediation treats employer IDs as opaque strings and adds
none of those migration dependencies. No canonical columns or mapping tables
are created now; additional reference formats remain undecided.

## MVP release and UAT procedure — approval required

The owner accepts a temporary frontend/backend version mismatch for the MVP,
provided both production deployments succeed before Tara resumes UAT. Use the
existing feature-branch Vercel previews and main-merge production workflows;
there is no new deployment gate, infrastructure or required two-release split.

1. Push the reviewed feature branch for Vercel preview. The Go workflow triggers
   on pull requests and main pushes, not feature-branch pushes alone. Preview
   builds do not deploy this backend. If the preview uses the existing production
   API, discovery is unavailable until the backend release; record that expected
   mismatch instead of labelling it a client failure. Preview login may also be
   limited by existing origin/return-URL allowlists. Do not broaden them or use
   Tara's credentials to force a preview test.
2. After explicit merge approval, merge the reviewed feature branch into main
   using the normal PR workflow. This triggers the existing Go `CI` test/deploy
   workflow, Vercel production deployment and trust-artifact publishing. Retain
   the previous successful API SHA and Vercel production deployment for rollback.
3. The Go deploy job waits for its tests, SSHes to the existing server and runs
   `scripts/deploy-api.sh <full-current-main-sha>`. It builds/recreates API only,
   then accepts `/healthz` and the exact `/version` SHA. No manual API deployment
   command is required after a successful merge-triggered run. Do not run the
   server-only script locally; it rejects arbitrary old non-main SHAs.
4. Vercel independently builds the same merge SHA in the existing `expo-app-mvp`
   Git integration (root `apps/evt-ui-expo`, existing Expo web export and `dist`
   output) and publishes the frontend to `mvp.cvera.app`. Preserve
   `EXPO_PUBLIC_EVT_API_BASE_URL=https://api.cvera.app`. No additional frontend
   CLI deployment command is required. Settings are not changed by this release.
5. Keep Tara paused until both deployments succeed for the merge SHA. Perform
   the following read-only smoke checks before resuming UAT. Health alone is not
   proof of authenticated discovery. No migrations, role grants, employer
   registration, assignment changes or production-data repair are needed.

After the approved merge, monitoring commands are:

```sh
gh run list --branch main --workflow main.yml --limit 5
gh run watch <merge-sha-CI-run-id> --exit-status
gh api repos/kola-white/rsa-attestation-engine/commits/<merge-sha>/status
gh api 'repos/kola-white/rsa-attestation-engine/deployments?environment=Production&per_page=5'
curl --fail --silent --show-error https://api.cvera.app/healthz
curl --fail --silent --show-error https://api.cvera.app/version
```

Match the Vercel deployment SHA/status as well as the Go API revision; the
workflows are independent. No command above merges or deploys by itself.

### Post-deployment smoke checks

- With a normal, explicitly authorized HR session, inspect
  `GET /v1/employer/employers`: 200, `Cache-Control: private, no-store`, and only
  that account's unrevoked employers. For Tara's one existing grant, the internal
  response should contain exactly `cei-uat-sim-001`. Do not capture Bearer tokens.
  Anonymous discovery must deny (401). Do not alter assignments for this smoke.
- Open HR Review: that one employer is selected automatically with no ID entry
  or demo fallback. Its queue/detail requests use the resolved ID, and the
  existing submitted request appears if its status is ATTESTATION_PENDING.
  Confirm its claim context and that refresh remains functional. Missing queue
  entries can reflect MANUAL dispatch/status; never patch data to force success.
- Confirm account/logout/navigation and existing request/evidence read paths
  still work. Existing clients keep the same queue/detail/attest contracts; a
  wrong or unauthorized employer still denies. The old HR frontend retains its
  known demo-ID defect, so compatibility is not a claim that its bug is repaired.
- Single/multiple/none/revoked and stale-response cases have local regression
  coverage. Exercise them with existing approved preview/test fixtures where
  possible; do not create or revoke Tara's production grants for testing.
- Do not approve/reject, sign, upload or mutate a case as part of these checks.
  Actual decisions require separately established authority. Bound requests
  intentionally reject legacy attest and keep the separate evidence-backed path.

### Straightforward rollback

Use Vercel's existing rollback/promote control to restore the recorded previous
successful production frontend first. It remains compatible with the additive
backend route, but restores the known HR demo-ID limitation; keep Tara paused.
If the API also needs rollback, make a reviewed revert of the release commit on
an ordinary feature branch, obtain approval, and merge that revert through the
same main CI/deploy workflow. A frontend-only failure does not require API
rollback. A retained-image recovery is an existing operator alternative if CI
is unavailable, under separate approval. No database rollback, ID renaming,
assignment recreation, credential change or migration is needed.

## Outstanding acceptance risks

Actual preview/browser/native and Tara UAT results must be reported separately
from engineering tests; unavailable preview authentication/backend support does
not invalidate the local implementation tests. Neutral multi-employer labels
lack business-name recognition. Registry-wide immutability, system-generated
employer creation and requestor-side manual ID entry remain separate future work.
Sensitive authentication logging remains unchanged by explicit scope. Temporary
release mismatch is owner-accepted; both deployed revisions and read-only smoke
checks are required before Tara resumes UAT.
