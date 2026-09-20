# Cvera CI/CD Delivery

This document is the canonical human-readable operational record for
Cvera's CI/CD delivery architecture and implementation milestones.

It records the relationship between source revision, authoritative
validation, durable evidence, deployment, and runtime identity.

The full Git SHA is the correlation key across these stages.

---

## Delivery Model

- GitHub Actions is the authoritative record of validation results.
- DigitalOcean Spaces is the durable secondary evidence archive.
- DigitalOcean runtime will execute an explicitly validated revision.
- The full Git SHA is the correlation key across validation, evidence,
  deployment, and runtime identity.
- Legacy credential trust-artifact publication remains separate from
  application CI/CD.

The operational responsibilities are deliberately separated:

- `CI` — authoritative application validation and CI evidence.
- `CI Artifacts` — legacy/development artifact generation and review.
- `Publish to DigitalOcean Spaces` — legacy trust-artifact distribution.

---

## Phase Status

### Phase 1 — CI Correctness

**Status:** COMPLETE

#### Objective

Establish one authoritative GitHub Actions workflow that validates the
application as a whole and prevents an unsuccessful revision from being
treated as successfully validated.

The required application quality gates are:

- Node validation/tests.
- Go API tests using the project's Go 1.25 build contract.

A failure in either required gate must fail the authoritative CI job.

#### Authoritative Workflow

The authoritative workflow is:

`.github/workflows/main.yml`

Workflow name:

`CI`

Job:

`test`

It runs on:

- pushes to `main`
- pull requests

#### Node Quality Gate

The authoritative Node quality gate runs:

`npm run test:ci`

This currently validates the existing EVT schema and test vectors,
including the expected-invalid vector validation.

The command is blocking. Failure is not suppressed with
`continue-on-error`, `|| true`, or an equivalent failure-swallowing
mechanism.

#### Go Quality Gate

The authoritative workflow installs Go using:

`actions/setup-go@v5`

with the project build contract:

`Go 1.25`

The dependency cache is keyed from:

`apps/evt-api-go/go.sum`

The Go quality gate executes from:

`apps/evt-api-go`

using:

`go test ./...`

The command is blocking.

At the time Phase 1 was established, the test suite included successful
tests for the standards-based credential verification foundation,
including:

- `internal/credential/legacyjws`
- `internal/credential/sdjwtvc`
- `internal/httpapi`

Packages without tests are reported separately by Go as having no test
files.

#### Go Version Contract

The project CI/build Go contract is Go 1.25.

The API Docker build also uses the Go 1.25 toolchain.

A newer compatible Go installation may be used for local development,
but the authoritative CI/build contract remains Go 1.25 unless changed
deliberately.

The DigitalOcean application host does not require a host-installed Go
toolchain. GitHub performs authoritative validation and the Docker
builder is responsible for compiling the Go API.

#### Failure Semantics

The authoritative workflow is fail-closed with respect to its required
quality gates:

- Node failure → CI fails.
- Go failure → CI fails.
- A revision is not considered successfully validated unless both
  required gates pass.

Phase 1 deliberately did not change the separate `CI Artifacts`
workflow, which historically permits some development/test operations
to continue without making them authoritative application quality
gates.

#### Separation from Legacy Workflows

Phase 1 did not modify the responsibilities of the existing legacy
workflows.

`CI Artifacts` remains a legacy/development workflow responsible for
activities including:

- optional development key generation
- JWKS generation
- status-list generation when present
- test-vector generation when present
- review artifact upload

`Publish to DigitalOcean Spaces` remains the legacy trust-artifact
distribution workflow responsible for publishing artifacts including:

- JWKS
- status information
- trust policy
- the legacy latest pointer

Neither workflow is the authoritative application CI quality gate.

#### Deployment Boundary

Phase 1 introduced no application deployment behavior.

A successful CI run establishes that a revision passed the required
application quality gates. It does not, by itself, establish that the
revision was deployed or is currently running in DigitalOcean.

Deployment identity and runtime revision verification are handled by
later phases.

#### Implementation Record

The Phase 1 implementation was squash-merged to `main` as:

`51c7fa6` — `ci: add Go quality gate (#106)`

Local validation before merge confirmed:

- `npm run test:ci` passed.
- `go test ./...` passed.
- `git diff --check` was clean.

#### Operational Validation Note

After the Phase 1 source changes had been merged, it was later
discovered during Phase 2 validation that the GitHub workflow named
`CI` had previously been manually disabled at the repository level.

This did not change the Phase 1 workflow definition in source control,
but it meant that earlier source-level validation did not by itself
prove that GitHub was actively executing the authoritative workflow.

The `CI` workflow was manually re-enabled during Phase 2.

Operational execution of the authoritative workflow was subsequently
proven on `main`, where the `test` job successfully executed both the
Node and Go quality gates.

This closes the distinction between:

- Phase 1 implementation correctness in source control; and
- operational execution of that workflow by GitHub Actions.

#### Phase 1 Acceptance

Phase 1 is considered complete because:

- the authoritative `CI` workflow is defined in
  `.github/workflows/main.yml`;
- Node validation is a blocking quality gate;
- Go 1.25 testing is a blocking quality gate;
- failures are not deliberately suppressed;
- both gates execute as part of the same authoritative `test` job;
- the workflow is enabled and has been successfully executed by GitHub
  Actions;
- legacy artifact-generation and publication workflows remain separate;
- no application deployment behavior was introduced.

**Phase 1 — CI Correctness: COMPLETE.**

---

### Phase 2 — Immutable CI Evidence Archival

**Status:** COMPLETE

#### Objective

Preserve durable evidence of every successfully validated `main`
revision without changing GitHub Actions' role as the authoritative
record of CI execution.

DigitalOcean Spaces acts as the secondary durable evidence archive.

#### Evidence Hierarchy

Successful authoritative `main` validation is archived under:

`ci-evidence/YYYY-MM-DD/<full-git-sha>/`

`YYYY-MM-DD` represents the UTC date of the successful authoritative
CI validation.

The full 40-character Git SHA identifies the validated revision.

Each evidence package contains:

- `manifest.json`
- `node-tests.txt`
- `go-tests.txt`

The date provides human-readable chronological organization while the
Git SHA remains the identity of the evidence.

No mutable CI `latest` pointer is maintained.

#### Evidence Manifest

`manifest.json` records the validation event, including:

- schema version
- repository
- full Git SHA
- Git ref
- GitHub workflow
- GitHub workflow run ID
- GitHub workflow run attempt
- UTC validation timestamp
- Node command and result
- Go version contract, command, and result
- overall result

#### Test Evidence

`node-tests.txt` retains the actual output from:

`npm run test:ci`

`go-tests.txt` retains the actual output from:

`go test ./...`

Test output is captured with `tee` while Bash `pipefail` preserves the
exit status of the underlying test command.

Therefore, evidence capture cannot convert a failing quality gate into
a successful CI result.

#### Archival Semantics

Evidence is archived only for successful authoritative pushes to
`main`.

Pull requests still execute the authoritative Node and Go quality
gates, but do not publish durable CI evidence to DigitalOcean Spaces.

Failed CI does not proceed through the normal successful evidence
archival path.

Existing evidence objects are protected from silent overwrite using
conditional object creation:

`IfNoneMatch: "*"`

CI evidence is archival operational evidence. It is separate from the
legacy public trust-artifact publication mechanism.

#### Implementation Boundary

Phase 2 did not:

- deploy application code;
- modify the DigitalOcean runtime;
- modify the `CI Artifacts` workflow;
- modify `scripts/publish.ts`;
- modify the existing `Publish to DigitalOcean Spaces` workflow; or
- change legacy trust-artifact publication behavior.

The Phase 2 implementation was squash-merged to `main` as:

`72019b0` — `ci: archive immutable validation evidence (#107)`

#### First Proven Evidence Revision

**Git SHA:**

`c81a3001319b2b145c3b68237a863183406e99ae`

**Validated:**

`2026-09-20 UTC`

The authoritative GitHub Actions workflow `CI`, job `test`,
successfully executed:

- Node quality gate
- Go 1.25 quality gate
- CI evidence manifest generation
- DigitalOcean Spaces evidence archival

The corresponding evidence package was confirmed present in
DigitalOcean Spaces under:

`ci-evidence/2026-09-20/c81a3001319b2b145c3b68237a863183406e99ae/`

The package contained:

- `manifest.json`
- `node-tests.txt`
- `go-tests.txt`

The archived manifest recorded:

- repository `kola-white/rsa-attestation-engine`
- Git SHA `c81a3001319b2b145c3b68237a863183406e99ae`
- Git ref `refs/heads/main`
- workflow `CI`
- workflow run ID `35514839624`
- workflow run attempt `1`
- validation timestamp `2026-09-20T13:54:58Z`
- Node result `passed`
- Go 1.25 result `passed`
- overall result `passed`

The manifest SHA exactly matched the revision validated by GitHub
Actions and the local `main` HEAD.

The archived Go evidence contained substantive test results, including
successful execution of:

- `internal/credential/legacyjws`
- `internal/credential/sdjwtvc`
- `internal/httpapi`

#### Phase 2 Acceptance

Phase 2 is considered complete because:

- evidence is archived under
  `ci-evidence/YYYY-MM-DD/<full-git-sha>/`;
- the date represents the UTC validation date;
- the full 40-character Git SHA is retained;
- evidence corresponds to the exact SHA validated by GitHub;
- Node output is retained;
- Go output is retained;
- the manifest is retained and correctly populated;
- GitHub Actions remains the authoritative validation record;
- DigitalOcean Spaces provides the durable secondary archive;
- evidence capture does not mask test failure;
- Node and Go failures remain blocking;
- failed CI is not represented as successfully validated evidence;
- no mutable CI `latest` pointer is introduced;
- existing evidence is protected from silent overwrite;
- no application deployment occurs;
- no DigitalOcean runtime changes occur; and
- legacy artifact generation and publication remain separate.

**Phase 2 — Immutable CI Evidence Archival: COMPLETE.**

---

### Phase 3 — Runtime Revision Identity

**Status:** NOT STARTED

#### Objective

Preserve `/healthz` as the existing operational liveness endpoint and
introduce runtime revision identity so the running Go API can report
the exact Git SHA from which it was built.

The revision identity must be compiled into the application at build
time rather than inferred from a runtime `.git` checkout.

Phase 3 establishes the runtime identity mechanism required before
implementing exact-SHA deployment in Phase 4.