import fs from "fs";
import path from "path";
import { verifyAttestationJws } from "./verify";

async function main() {
  const file = process.argv[2];
  if (!file) {
    console.error("Usage: npm run verify -- <path-to-jws.txt>");
    process.exit(2);
  }
  const jws = fs.readFileSync(path.resolve(file), "utf8").trim();
  const res = await verifyAttestationJws(jws);
  console.log("RESULT:", res);
  process.exit(res.ok ? 0 : 1);
}

main().catch(err => {
  console.error(err);
  process.exit(1);
});
