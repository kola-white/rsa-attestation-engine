import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { execFileSync } from "node:child_process";
import { verify, State, bindEvaluator } from "./verify.mjs";
import {
  verify as presentation,
  authorize,
  Store,
} from "../stage4c/verify.mjs";
const fixtures = JSON.parse(readFileSync(process.argv[3]));
let evaluations = 0;
const outcomes = [];
for (const name of [
  "authentic-portable-trust",
  "good-to-revoked-without-reissuance",
  "known-trust-rollback",
  "known-status-rollback",
  "fresh-cache-with-private-services-unavailable",
])
  assert.ok(
    fixtures.cases.some((c) => c.name === name),
    "required scenario absent: " + name,
  );
assert.equal(
  new Set(fixtures.cases.map((c) => c.name)).size,
  fixtures.cases.length,
);
assert.equal(fixtures.distributionResults.length, fixtures.distributionChecks);
assert.ok(fixtures.distributionChecks > 0);
for (const c of fixtures.cases) {
  assert.ok(c.inputs.length > 0);
  assert.equal(c.expected.length, c.inputs.length);
  const state = new State(),
    replay = new Store(),
    js = [];
  for (let n = 0; n < c.inputs.length; n++) {
    const i = c.inputs[n];
    try {
      if (c.mode) {
        const ci = c.presentations[n],
          evaluate = bindEvaluator(i, state);
        js.push(
          await (c.mode === "holder"
            ? authorize(ci, evaluate)
            : presentation(ci, replay, evaluate)),
        );
      } else js.push(await verify(i, state));
    } catch {
      js.push({ error: "INVALID_POLICY" });
    }
  }
  const go = JSON.parse(
    execFileSync(process.argv[2], [], {
      input: JSON.stringify(c),
      env: { PATH: process.env.PATH },
      maxBuffer: 32 * 1024 * 1024,
    }),
  );
  assert.deepEqual(go, js, c.name + " independent outcomes differ");
  for (let n = 0; n < c.expected.length; n++)
    for (const [key, value] of Object.entries(c.expected[n]))
      assert.equal(
        key.split(".").reduce((v, k) => v?.[k], js[n]),
        value,
        c.name + ":" + key,
      );
  evaluations += 2 * js.length;
  outcomes.push({ name: c.name, result: "PASS", evaluations: 2 * js.length });
}
assert.equal(fixtures.issuanceCalls, 1);
assert.equal(fixtures.unchangedCredential, true);
console.log(
  JSON.stringify(
    {
      contract: "cvera.stage4d.portable-trust.v1",
      synthetic: true,
      node: process.version,
      issuanceCalls: 1,
      unchangedCredential: true,
      distributionChecks: fixtures.distributionChecks,
      distributionResults: fixtures.distributionResults,
      evaluations,
      cases: outcomes,
    },
    null,
    2,
  ),
);
