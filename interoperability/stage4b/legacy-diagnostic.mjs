import fs from "node:fs";
import os from "node:os";
import path from "node:path";
const root = path.resolve(
  path.dirname(new URL(import.meta.url).pathname),
  "../..",
);
const fixture = fs.mkdtempSync("/private/tmp/cvera-stage4-legacy-fixture-");
process.chdir(fixture);
process.env.TRUST_MODE = "local";
fs.mkdirSync("trust");
const jose = await import(root + "/node_modules/jose/dist/webapi/index.js");
const mod = await import(root + "/pkg/verification/verify.ts");
const verify = mod.verifyAttestationJws ?? mod.default?.verifyAttestationJws;
if (typeof verify !== "function")
  throw Error("Cannot locate existing verifier");
const { privateKey, publicKey } = await jose.generateKeyPair("RS256", {
  modulusLength: 2048,
});
const jwk = await jose.exportJWK(publicKey);
jwk.kid = "LOCAL-DIAGNOSTIC-ONLY";
fs.writeFileSync("trust/jwks.json", JSON.stringify({ keys: [jwk] }));
fs.writeFileSync(
  "trust/policy.json",
  JSON.stringify({
    schema_uri: "urn:local:diagnostic",
    allowed_assurance: ["local-only"],
  }),
);
const now = Date.now();
const payload = {
  schema_uri: "urn:local:diagnostic",
  id: "LOCAL-TEST-NOT-A-REAL-CREDENTIAL",
  claim: { value: { title: "LOCAL TEST ONLY" } },
  revocation: { serial: "local-only-serial" },
  validity: {
    not_before: new Date(now - 60000).toISOString(),
    not_after: new Date(now + 60000).toISOString(),
  },
  policy: { assurance: "local-only" },
  disclosure: { mode: "full" },
  hash: { payload_alg: "SHA-256", payload_hash: "not-a-real-payload-digest" },
};
// Synthetic diagnostic JWT only: no production issuer, key, claim or issuance API.
const token = await new jose.SignJWT(payload)
  .setProtectedHeader({ alg: "RS256", kid: jwk.kid })
  .sign(privateKey);
const observations = {};
for (const [name, list] of Object.entries({
  fresh_known: {
    generated_at: new Date(now).toISOString(),
    ttl_s: 900,
    entries: [{ serial: "local-only-serial", status: "good" }],
  },
  stale_known: {
    generated_at: "2000-01-01T00:00:00Z",
    ttl_s: 1,
    entries: [{ serial: "local-only-serial", status: "good" }],
  },
  missing_serial: {
    generated_at: new Date(now).toISOString(),
    ttl_s: 900,
    entries: [],
  },
  explicit_revoked: {
    generated_at: new Date(now).toISOString(),
    ttl_s: 900,
    entries: [{ serial: "local-only-serial", status: "revoked" }],
  },
})) {
  fs.writeFileSync("trust/statuslist.json", JSON.stringify(list));
  observations[name] = await verify(token);
}
const result = {
  scope:
    "synthetic local diagnostic of existing legacy TypeScript verifier only; not the database-backed SD-JWT verifier",
  production_accessed: false,
  fixture_public_status_directory: fixture,
  invalid_payload_hash_was_supplied: true,
  observations,
};
// Only public synthetic evidence is logged; private test key remains in memory.
console.log(JSON.stringify(result));
