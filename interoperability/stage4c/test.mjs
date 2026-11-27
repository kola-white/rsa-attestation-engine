import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { execFileSync } from "node:child_process";
import { verify, authorize, Store, hash } from "./verify.mjs";
const fixtures = JSON.parse(readFileSync(process.argv[3]));
const outcomes = [];
let evaluations = 0;
for (const c of fixtures.cases) {
  const store = new Store();
  let js;
  if (c.concurrent)
    js = await Promise.all(c.inputs.map((i) => verify(i, store)));
  else {
    js = [];
    for (const i of c.inputs)
      js.push(await (c.holder ? authorize(i) : verify(i, store)));
  }
  const go = JSON.parse(
    execFileSync(process.argv[2], [], {
      input: JSON.stringify(c),
      env: { PATH: process.env.PATH },
      maxBuffer: 16 * 1024 * 1024,
    }),
  );
  const sorted = (a) =>
    c.concurrent
      ? [...a].sort((x, y) => x.overall.localeCompare(y.overall))
      : a;
  assert.deepEqual(
    sorted(js),
    sorted(go),
    c.name + " independent outcomes differ",
  );
  const expected = Array.isArray(c.expected) ? c.expected : [c.expected];
  const actual = sorted(js),
    exp = sorted(expected);
  for (let n = 0; n < exp.length; n++)
    for (const [k, v] of Object.entries(exp[n]))
      assert.equal(
        k.split(".").reduce((o, part) => o?.[part], actual[n]),
        v,
        c.name + ": " + k,
      );
  evaluations += js.length * 2;
  outcomes.push({ name: c.name, result: "PASS", evaluations: js.length * 2 });
}
const first = fixtures.cases.find((x) => x.name === "verifier-a").inputs[0],
  second = fixtures.cases.find((x) => x.name === "verifier-b").inputs[0];
assert.equal(first.base.credential, second.base.credential);
assert.equal(hash(first.base.credential), fixtures.credentialHash);
assert.notEqual(first.request, second.request);
assert.notEqual(first.presentation, second.presentation);
assert.equal(fixtures.issuanceCalls, 1);
console.log(
  JSON.stringify(
    {
      contract: "cvera.stage4c.presentation.v1",
      synthetic: true,
      node: process.version,
      issuanceCalls: 1,
      unchangedCredentialReuse: true,
      distinctVerifierPresentations: 2,
      cases: outcomes,
      evaluations,
      issuanceNegativeChecks: fixtures.issuanceNegativeChecks,
      holderSigningDenialChecks: fixtures.holderSigningDenialChecks,
    },
    null,
    2,
  ),
);
