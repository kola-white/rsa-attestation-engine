import fs from "fs";
import path from "path";

const AUDIT_DIR = path.resolve(process.cwd(), "trust/audit-logs");

function ensureDir(dir: string) {
  if (!fs.existsSync(dir)) fs.mkdirSync(dir, { recursive: true });
}
ensureDir(AUDIT_DIR);

export type Outcome = "ACCEPT" | "REJECT";

export function audit(event: {
  stage: "signature" | "shape" | "liveness" | "revocation" | "policy";
  outcome: Outcome;
  reason?: string;
  details?: Record<string, unknown>;
}) {
  const line = JSON.stringify({ ts: new Date().toISOString(), ...event }) + "\n";
  const fname = new Date().toISOString().slice(0, 10) + ".log"; // e.g. 2025-11-09.log
  const fpath = path.join(AUDIT_DIR, fname);

  try {
    fs.appendFileSync(fpath, line, "utf8"); // JSONL append
  } catch {
    // Don't crash verification on logging failure
  }
  process.stdout.write(line);
}
