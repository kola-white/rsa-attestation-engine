import fs from "node:fs";
import path from "node:path";
import { importPKCS8, SignJWT } from "jose";

function usage() {
  console.error("Usage: node dist/pkg/issuer/sign-file.js --kid <kid> <attestation.json>");
  process.exit(2);
}

const args = process.argv.slice(2);
const kidIdx = args.indexOf("--kid");
if (kidIdx === -1 || !args[kidIdx+1] || !args[kidIdx+2]) usage();
const kid = args[kidIdx+1];
const inPath = args[kidIdx+2];

const pemPath = path.resolve(process.cwd(), `keys/${kid}.pem`);
if (!fs.existsSync(pemPath)) {
  console.error(`Private key not found: ${pemPath}`);
  process.exit(3);
}

async function main() {
  const payload = JSON.parse(fs.readFileSync(inPath, "utf8"));
  const pkcs8 = fs.readFileSync(pemPath, "utf8");
  const key = await importPKCS8(pkcs8, "RS256");

  const jws = await new SignJWT(payload as any)
    .setProtectedHeader({ alg: "RS256", kid, typ: "JWT" })
    .sign(key);

  // Write to stdout (pipe to file if you want)
  process.stdout.write(jws + "\n");
}

main().catch(e => { console.error(e?.message || e); process.exit(1); });
