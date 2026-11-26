# Stage 4B independent interoperability

This is a local synthetic demonstration, not a production verifier or issuer.
See the [contract](../../docs/api/portable-verification-stage4b.md) for normative
inputs, outcome semantics, trust boundaries and the approved freshness policy.

- `fixtures/manifest.json`: one existing synthetic issuance call and base digest.
- `fixtures/vectors.json`: 37 public cases, explicit expectations and public roots.
- `verify.mjs`: independent JavaScript JOSE verifier; no Go/Cvera client imports.
- `test.mjs`: invokes two separate processes, checks results and unchanged reuse.
- `run.py`: builds Go runner; optionally enforces inspected network-free isolation.
- `legacy-diagnostic.mjs`: preserved legacy CLI reproducer; not used by either verifier.
- `evidence/`: durable local execution summaries and the individual AC-01–AC-21 scorecard.

Go implementation: `apps/evt-api-go/internal/portableverify` and
`apps/evt-api-go/cmd/portable-verify`. Fixture-only generator:
`apps/evt-api-go/cmd/portable-fixtures` (uses the existing issuance engine with
synthetic authority/evidence and ephemeral keys; never run against production).

```sh
npm ci
npm run test:portable
# The same CI Node runtime with hard network/service isolation:
docker pull node:20
python3 interoperability/stage4b/run.py --isolated
```

The trusted operator supplies the public roots, deterministic clock, expected
subject and status maximum age. Do not treat sender-provided trust/policy as
trusted configuration. Default maximum status age is 900 seconds inclusive.
All complete outcomes remain INDETERMINATE or REJECTED: holder possession and
real-world authority are not established by these test fixtures.

The original regression run exercised all tests with Go 1.26.10/PostgreSQL16;
frontend checks also passed under Node20 with existing lockfiles. Docker isolation
uses no database or application service. No production behavior was changed and
no genuine credential was issued. Remote CI results are separate from the recorded
local execution evidence.
