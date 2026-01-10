// pkg/verification/outcomeHarness.ts

import { readFile } from "node:fs/promises";
import { resolve } from "node:path";
import { verifyOutcomeFromJws } from "../../../rsa-attestation-engine/apps/evt-ui-expo/src/verification/verifyOutcome";

type Vector = { name: string; relPath: string };

async function loadJws(absPath: string): Promise<string> {
  const raw = await readFile(absPath, "utf8");
  return raw.trim();
}

export async function runVerificationOutcomeHarness(): Promise<void> {
  // Banner prints no matter what — if you don't see this, you're not running this file.
  console.log("[outcomeHarness] starting");
  console.log("[outcomeHarness] cwd:", process.cwd());

  const vectors: Vector[] = [
    { name: "golden-valid", relPath: "out/golden-valid.jws" },
    { name: "golden-invalid-signature", relPath: "out/golden-invalid-signature.tampered.jws" },
    { name: "golden-revoked", relPath: "out/golden-revoked.jws" },
  ];

  for (const v of vectors) {
    const abs = resolve(process.cwd(), v.relPath);
    const jws = await loadJws(abs);
    const outcome = await verifyOutcomeFromJws(jws);

    console.log(`\n=== ${v.name} (${v.relPath}) ===`);
    console.log(JSON.stringify(outcome, null, 2));
  }

  console.log("\n[outcomeHarness] done");
}

// ALWAYS run (no import.meta, no require.main guard)
runVerificationOutcomeHarness().catch((err) => {
  console.error("[outcomeHarness] error:", err);
  process.exitCode = 1;
});
