// scripts/make-jwks.mjs
import fs from "node:fs";
import { createPublicKey } from "node:crypto";
import { exportJWK } from "jose";

const INPUTS = [
  "keys/issuer-dev-key-1.pem",
  "keys/issuer-dev-key-2.pem",
  "keys/issuer-dev-key-3.pem",
];

const OUT_PATH = "trust/jwks.json";

function kidFromPath(p) {
  const base = p.split("/").pop() || p;
  return base.replace(/\.pem$/i, "");
}

function stripPad(s) {
  return typeof s === "string" ? s.replace(/=+$/g, "") : s;
}

async function run() {
  console.log("Building JWKS from:", INPUTS.join(", "));
  const keys = [];

  for (const pemPath of INPUTS) {
    try {
      const pem = fs.readFileSync(pemPath, "utf8");
      const pub = createPublicKey(pem);
      const jwk = await exportJWK(pub);

      const entry = {
        kty: jwk.kty,
        n: stripPad(jwk.n),
        e: stripPad(jwk.e),
        alg: "RS256",
        use: "sig",
        kid: kidFromPath(pemPath),
      };

      console.log(`  + ${entry.kid}`);
      keys.push(entry);
    } catch (e) {
      console.error(`Failed on ${pemPath}:`, e?.message || e);
      process.exit(2);
    }
  }

  const out = JSON.stringify({ keys }, null, 2) + "\n";
  const tmp = OUT_PATH + ".tmp";
  fs.writeFileSync(tmp, out, "utf8");
  fs.renameSync(tmp, OUT_PATH);

  console.log(`Wrote ${OUT_PATH} (${keys.length} keys)`);
}

run().catch((e) => {
  console.error("Fatal:", e?.message || e);
  process.exit(3);
});

