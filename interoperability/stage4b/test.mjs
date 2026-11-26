import { spawnSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";
import { createHash } from "node:crypto";
import { fileURLToPath } from "node:url";
import path from "node:path";
import assert from "node:assert/strict";
const here = path.dirname(fileURLToPath(import.meta.url));
const go = process.argv[2];
if (!go) throw Error("Go verifier executable path required");
const vectors = JSON.parse(
  readFileSync(path.join(here, "fixtures/vectors.json")),
);
const manifest = JSON.parse(
  readFileSync(path.join(here, "fixtures/manifest.json")),
);
assert.equal(manifest.issuanceCalls, 1);
assert.equal(manifest.privateKeysWritten, false);
assert.equal(vectors.length, manifest.vectors);
assert.equal(new Set(vectors.map((v) => v.name)).size, vectors.length);
for (const required of [
  "fresh",
  "status-age-899",
  "status-age-900",
  "status-age-901",
  "status-age-86400",
  "status-missing",
  "status-unavailable",
  "status-missing-entry",
  "status-invalid-signature",
  "status-wrong-binding",
  "status-revoked",
  "status-future",
  "configured-60-age-60",
  "configured-60-age-61",
  "untrusted-credential-freshness-override",
  "untrusted-status-freshness-override",
  "holder-UNSUPPORTED",
  "holder-UNRESOLVED",
  "holder-VALID",
])
  assert.ok(
    vectors.some((v) => v.name === required),
    "Required coverage absent: " + required,
  );

const env = {
  PATH: process.env.PATH ?? "/usr/local/bin:/usr/bin:/bin",
  LANG: "C",
};
let evaluations = 0;
function run(program, args, input) {
  const r = spawnSync(program, args, {
    input: JSON.stringify(input),
    encoding: "utf8",
    env,
    timeout: 10000,
    maxBuffer: 1024 * 1024,
  });
  if (r.error) throw r.error;
  return r;
}
for (const v of vectors) {
  for (const [name, program, args] of [
    ["go", go, []],
    ["javascript", process.execPath, [path.join(here, "verify.mjs")]],
  ]) {
    const r = run(program, args, v.input);
    assert.equal(r.status, 0, `${v.name}/${name}: ${r.stderr}`);
    assert.deepEqual(JSON.parse(r.stdout), v.expected, `${v.name}/${name}`);
    evaluations++;
  }
}
const base = vectors.find((v) => v.name === "fresh");
const sha = (x) => createHash("sha256").update(x).digest("hex");
assert.equal(sha(base.input.credential), manifest.credentialSha256);
for (let n = 0; n < 3; n++)
  for (const [program, args] of [
    [go, []],
    [process.execPath, [path.join(here, "verify.mjs")]],
  ]) {
    const r = run(program, args, base.input);
    assert.equal(r.status, 0);
    assert.deepEqual(JSON.parse(r.stdout), base.expected);
    assert.equal(sha(base.input.credential), manifest.credentialSha256);
    evaluations++;
  }
for (const change of [
  (v) => (v.policy.maxStatusAgeSeconds = -1),
  (v) => (v.policy.maxStatusAgeSeconds = 0.5),
  (v) => (v.policy.now = 0),
  (v) => (v.trust.synthetic = false),
]) {
  const input = structuredClone(base.input);
  change(input);
  for (const [program, args] of [
    [go, []],
    [process.execPath, [path.join(here, "verify.mjs")]],
  ])
    assert.equal(
      run(program, args, input).status,
      2,
      "invalid trusted configuration must fail visibly",
    );
}
const result = {
  contract: base.expected.contract,
  passed: true,
  node: process.version,
  vectorCount: vectors.length,
  verifierProcesses: 2,
  evaluations,
  invalidPolicyProbes: 8,
  issuanceCalls: manifest.issuanceCalls,
  credentialSha256: manifest.credentialSha256,
  unchangedCredentialRepetitionsPerVerifier: 3,
  privateKeysAvailable: false,
  authority: "SYNTHETIC ONLY; no real-world mandate",
  holderProof: "NOT IMPLEMENTED; overall never VERIFIED",
};
console.log(JSON.stringify(result, null, 2));
if (process.env.STAGE4B_RESULT_FILE)
  writeFileSync(
    process.env.STAGE4B_RESULT_FILE,
    JSON.stringify(result, null, 2) + "\n",
  );
