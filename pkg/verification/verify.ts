import { jwtVerify, createLocalJWKSet, type JWK } from "jose";
import fs from "fs";
import path from "path";
import { audit } from "../logger.js";

// ---------- helpers ----------
function loadJson(p: string) {
  return JSON.parse(fs.readFileSync(p, "utf8"));
}
function requireString(obj: any, segs: string[]): string | null {
  let cur = obj;
  for (const s of segs) cur = cur?.[s];
  return (typeof cur === "string" && cur.length > 0) ? cur : null;
}
const isoNow = () => new Date().toISOString();

// ---------- trust artifacts (always from project ./trust) ----------
const TRUST_DIR = path.resolve(process.cwd(), "trust");
const jwks       = loadJson(path.join(TRUST_DIR, "jwks.json"));
const statuslist = loadJson(path.join(TRUST_DIR, "statuslist.json"));
const policy     = loadJson(path.join(TRUST_DIR, "policy.json"));

const policyAllowed   = new Set<string>(Array.isArray(policy.allowed_assurance) ? policy.allowed_assurance : []);
const policySchemaUri = String(policy.schema_uri || "");

// ---------- main API ----------
export async function verifyAttestationJws(jwsCompact: string) {
  // 1) Signature + header (kid, alg)
  const keyset = createLocalJWKSet({ keys: (jwks.keys as JWK[]) || [] });
  let payload: any, hdr: any;
  try {
    const res = await jwtVerify(jwsCompact, keyset, { algorithms: ["RS256"] });
    payload = res.payload; hdr = res.protectedHeader;
  } catch (e: any) {
    audit({ stage: "signature", outcome: "REJECT", reason: "bad_signature", details: { err: e.message } });
    return { ok: false, code: "BAD_SIGNATURE" as const };
  }
  if (hdr.alg !== "RS256" || typeof hdr.kid !== "string") {
    audit({ stage: "signature", outcome: "REJECT", reason: "header_invalid", details: { alg: hdr.alg, kid: hdr.kid } });
    return { ok: false, code: "HEADER_INVALID" as const };
  }
  audit({ stage: "signature", outcome: "ACCEPT" });

  // 2) Minimal shape checks
  const schemaUri = requireString(payload, ["schema_uri"]);
  const attId     = requireString(payload, ["id"]);
  const title     = requireString(payload, ["claim","value","title"]);
  const serial    = requireString(payload, ["revocation","serial"]);
  const notBefore = requireString(payload, ["validity","not_before"]);
  const notAfter  = requireString(payload, ["validity","not_after"]);
  const assurance = requireString(payload, ["policy","assurance"]);

  if (schemaUri !== policySchemaUri) {
    audit({ stage: "shape", outcome: "REJECT", reason: "schema_uri_mismatch", details: { schemaUri, expected: policySchemaUri } });
    return { ok: false, code: "SCHEMA_URI_INVALID" as const };
  }
  if (!attId || !title || !serial || !notBefore || !notAfter || !assurance) {
    audit({ stage: "shape", outcome: "REJECT", reason: "missing_required_field" });
    return { ok: false, code: "SHAPE_INVALID" as const };
  }
  audit({ stage: "shape", outcome: "ACCEPT" });

  // 3) Liveness
  const now = isoNow();
  if (now < notBefore) {
    audit({ stage: "liveness", outcome: "REJECT", reason: "not_yet_valid", details: { now, not_before: notBefore } });
    return { ok: false, code: "NOT_YET_VALID" as const };
  }
  if (now > notAfter) {
    audit({ stage: "liveness", outcome: "REJECT", reason: "expired", details: { now, not_after: notAfter } });
    return { ok: false, code: "EXPIRED" as const };
  }
  audit({ stage: "liveness", outcome: "ACCEPT" });

  // 4) Revocation (fail-open when not listed)
  const entry = statuslist.entries?.find((e: any) => e.serial === serial);
  if (!entry) {
    audit({ stage: "revocation", outcome: "ACCEPT", reason: "unknown_not_listed", details: { serial } });
  } else if (entry.status === "revoked") {
    audit({ stage: "revocation", outcome: "REJECT", reason: "revoked", details: { serial } });
    return { ok: false, code: "REVOKED" as const };
  } else if (entry.status !== "good") {
    audit({ stage: "revocation", outcome: "REJECT", reason: "invalid_status_value", details: { serial, status: entry.status } });
    return { ok: false, code: "REVOCATION_STATUS_INVALID" as const };
  } else {
    audit({ stage: "revocation", outcome: "ACCEPT" });
  }

  // 5) Policy (from trust/policy.json)
  if (!policyAllowed.has(assurance)) {
    audit({ stage: "policy", outcome: "REJECT", reason: "assurance_invalid", details: { assurance, allowed: [...policyAllowed] } });
    return { ok: false, code: "POLICY_ASSURANCE_INVALID" as const };
  }
  audit({ stage: "policy", outcome: "ACCEPT" });

  return { ok: true, code: "VALID" as const };
}
