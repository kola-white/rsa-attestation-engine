# Stage 4D local validation evidence

**21/21 PASS** for the documented local implementation and controlled synthetic
validation scope. This is not repository integration closure or genuine production
activation. No push, production inspection, deployment or genuine issuance occurred.

| Evidence                                                    | Observed result                                                                                     |
| ----------------------------------------------------------- | --------------------------------------------------------------------------------------------------- |
| [Acceptance scorecard](acceptance.json)                     | Every criterion individually evaluated with implementation and executed-test references             |
| [Isolated interoperability](isolated-interoperability.json) | 97 scenarios; 218 matching Go/JS evaluations; Node20.20.2; 14 named publication/distribution checks |
| [Native interoperability](native-interoperability.json)     | Same scenario outcomes and publication/distribution results                                         |
| [Stage4B regression](stage4b-regression.json)               | 39 vectors; 84 evaluations; original semantics preserved                                            |
| [Stage4C regression](stage4c-regression.json)               | 75 scenarios; 156 evaluations; original holder/authentication/consent/replay behavior preserved     |
| [Regression summary](regression-results.json)               | Go/PostgreSQL16, race, vet, API build, Node20 client/UI and Expo checks                             |
| [Go per-test results](go-test-results.json)                 | 556 tests/subtests, 253 top-level; no failures or skips; sanitized names/results/timestamps         |
| [Quality/provenance](quality-results.json)                  | Source hashes, syntax/format checks, local links and actual Mermaid rendering evidence              |

The publication tests cover good-to-revoked updates without credential reissuance,
rollback rejection, exact-byte idempotency, expired-material refresh, private-field
rejection, pointer traversal/corruption, no credential-index/authentication leakage,
missing policy, retrieval outage with fresh cache, and fault-injected **post-rename
failure**. That failure produces no successful receipt and marks commit ambiguity;
read-only reconciliation confirms the actual pointer. Missing referenced files
block publication instead of resetting history.

The core trust inputs, authority modes, status format, privacy properties,
limitations, normative requirements and operational prerequisites are in the
[architecture guide](../../architecture/stage4d-portable-trust-status.md) and
[ADR-011](../../architecture/adr/011-portable-trust-and-status.md).

Reproduce with `python3 interoperability/stage4d/run.py --isolated`. It generates
one intended credential and ephemeral keys, rehearses only local publication and
loopback acquisition, then transfers public artifacts to separate network-disabled,
read-only, unprivileged Go/Node verifiers. New JWTs and private keys are not tracked.

The Mermaid diagram was rendered using isolated Mermaid CLI12.2.0 and installed
Chrome, then visually inspected. The document uses ordinary GitHub-compatible
Mermaid flowchart syntax. Live GitHub preview was not accessed: this authorization
does not permit push. Rendering can be reproduced by extracting the guide's Mermaid
fence and running the pinned Mermaid CLI with a configured local Chrome executable.

Remote CI remains a subsequent integration gate. No production trust namespace or
automatic publication behavior was changed. Historical GitGuardian exceptions are
not extended to new material. Long-term validity and complete production lifecycle
assurance are not established by these tests.
