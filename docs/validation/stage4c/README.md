# Stage 4C local validation evidence

Local implementation acceptance: **21/21 PASS**, limited to the documented
synthetic library/CLI execution model. This is not final integration closure or
production readiness. These artifacts record local validation before integration. Subsequent remote CI
and release results are published in the integration PR closure record.

- [Acceptance scorecard](acceptance.json): every criterion, evidence and limitations.
- [Isolated interoperability](isolated-interoperability.json): 75 scenarios,
  156 matching Go/JS evaluations, Node 20.20.2, one successful issuance, two
  distinct verifier presentations of the same credential. Includes six issuer
  negative checks and 23 holder-side no-presentation-on-denial checks.
- [Native interoperability](native-interoperability.json): same outcomes on the
  local Node runtime; isolated Node 20 is the CI-runtime gate.
- [Stage 4B regression](stage4b-regression.json): unchanged 39 vectors and
  84 evaluations. Holder proof remains unimplemented **in Stage 4B**, as intended.
- [Regression summary](regression-results.json): complete Go/PostgreSQL16 tests,
  race check, vet, API compilation, Node20 client/UI tests and Expo TypeScript.
- [Per-test results](go-test-results.json): 547 Go tests/subtests, 250 top-level
  tests; no failures or skips. Sanitized statuses and timestamps, no raw tokens.

The architecture, security boundaries, normative source mapping, input/output
contract and reproducible commands are in the
[Stage 4C guide](../../architecture/stage4c-holder-bound-presentation.md).

Signed fixtures and private keys are generated only at runtime. They are not
tracked here. Source hashes and final quality results are in
[quality results](quality-results.json). No existing production v1 issuance,
verification, authorization, schema or protected-record implementation changed.
Final integration authorization was subsequently granted; its results are recorded
in the integration PR closure record.
