import fs from "node:fs";
import { createPublicKey } from "node:crypto";
import { exportJWK } from "jose";

function kidFromPath(p) {
  const base = p.split("/").pop() || p;
  return base.replace(/\.pem$/i, "");
}
function stripPad(s) { return s.replace(/=+$/g, ""); }

async function main() {
  const paths = process.argv.slice(2);
  if (paths.length === 0) {
    console.error("Usage: node scripts/pem-to-jwks.mjs <key1.pem> <key2.pem> ...");
    process.exit(1);
  }
  const keys = [];
  for (const pemPath of paths) {
    const pem = fs.readFileSync(pemPath, "utf8");
    const pub = createPublicKey(pem);
    const jwk = await exportJWK(pub);
    keys.push({
      kty: jwk.kty,           // "RSA"
      n: stripPad(jwk.n),     // base64url, no padding
      e: stripPad(jwk.e),     // base64url, no padding
      alg: "RS256",
      use: "sig",
      kid: kidFromPath(pemPath)
    });
  }
  process.stdout.write(JSON.stringify({ keys }, null, 2) + "\n");
}