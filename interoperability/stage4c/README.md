# Stage 4C synthetic reciprocal interoperability

Run `python3 interoperability/stage4c/run.py --isolated` from the repository root.
Requires existing locked npm dependencies, Go 1.26.10, Docker and `node:20`.
`npm run test:presentation` runs natively; it does not establish network isolation.

`generate.mjs` creates ephemeral keys, one successful holder-bound issuance,
two verifier requests/presentations, and positive/negative public inputs. Nothing
is written to the repository by generation. `test.mjs` compares independently
computed Go/JOSE results and expected outcomes. It tests holder gates separately
from verifier acceptance. Private keys never enter the verifier mount.

The CLI takes a trusted session `{inputs: [...], holder: false, concurrent: false}`
on stdin and emits result objects. Only incoming JWTs/presentations are untrusted;
policy, roots, registry, consent and transaction records must come from trusted
local state. This is not a production service.

See [architecture and contract](../../docs/architecture/stage4c-holder-bound-presentation.md)
and [durable evidence](../../docs/validation/stage4c).
