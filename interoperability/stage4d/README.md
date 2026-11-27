# Stage 4D portable trust and status interoperability

Run `python3 interoperability/stage4d/run.py --isolated` from the repository root.
Requires existing locked npm dependencies, Go 1.26.10, Docker and cached `node:20`.
The native mode uses a synthetic loopback distribution server; it does not establish
network isolation.

The fixture generator signs one intended credential with its status reference,
rehearses public artifact publication/acquisition, and produces adversarial vectors.
Private keys stay in the generator process. Independent Go and JOSE verifiers
consume the same public artifacts with no Cvera services, accounts or private keys.
Expected outcomes include separate crypto, applicability, authority, status and
presentation decisions. Runtime fixtures are removed by the runner.

`trust-verify` consumes a trusted local session `{inputs:[...],mode:"",presentations:[]}`.
Inputs contain a credential plus **operator-controlled** anchor, policy and artifact
availability. Never populate local trust/policy/state from a holder's request.
`mode=holder` or `mode=presentation` uses the bounded Stage 4C adapter.

See the [architecture and operational guide](../../docs/architecture/stage4d-portable-trust-status.md)
and [validation evidence](../../docs/validation/stage4d/README.md).
