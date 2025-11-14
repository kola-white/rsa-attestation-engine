// pkg/verification/verify.ts
import { jwtVerify, createLocalJWKSet, type JWK } from "jose";
import { fetchTrustArtifacts } from "./trust-endpoints.js";
import { audit } from "../logger.js";

const isoNow = () => new Date().toISOString();

export async function verifyAttestationJws(jwsCompact: string) {
  // Remote (CDN) first; set TRUST_MODE=local to force local files
  const { jwks, statuslist, policy } = await fetchTrustArtifacts({ mode: "stable" });

  // Signature + header
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

  // Minimal shape checks (unchanged)
  const schemaUri = payload?.schema_uri;
  const attId     = payload?.id;
  const title     = payload?.claim?.value?.title;
  const serial    = payload?.revocation?.serial;
  const notBefore = payload?.validity?.not_before;
  const notAfter  = payload?.validity?.not_after;
  const assurance = payload?.policy?.assurance;
  const disclosure = payload?.disclosure;
  const hash       = payload?.hash;

  if (schemaUri !== policy.schema_uri) {
    audit({ stage: "shape", outcome: "REJECT", reason: "schema_uri_mismatch", details: { schemaUri, expected: policy.schema_uri } });
    return { ok: false, code: "SCHEMA_URI_INVALID" as const };
  }
  if (!attId || !title || !serial || !notBefore || !notAfter || !assurance) {
    audit({ stage: "shape", outcome: "REJECT", reason: "missing_required_field" });
    return { ok: false, code: "SCHEMA_INVALID" as const };
  }
  audit({ stage: "shape", outcome: "ACCEPT" });

  // AP-1: disclosure.mode MUST be "full"
  const disclosureMode = disclosure?.mode;
  if (disclosureMode !== "full") {
    audit({
      stage: "shape",
      outcome: "REJECT",
      reason: "disclosure_mode_invalid",
      details: { mode: disclosureMode }
    });
    return { ok: false, code: "DISCLOSURE_MODE_INVALID" as const };
  }

  // AP-1: hash.{payload_alg, payload_hash} MUST be present and SHA-256
  const hashAlg = hash?.payload_alg;
  const hashVal = hash?.payload_hash;
  if (hashAlg !== "SHA-256" || typeof hashVal !== "string" || !hashVal.length) {
    audit({
      stage: "shape",
      outcome: "REJECT",
      reason: "hash_invalid",
      details: { payload_alg: hashAlg, hasHash: !!hashVal }
    });
    return { ok: false, code: "HASH_INVALID" as const };
  }

  audit({ stage: "shape", outcome: "ACCEPT" });

  // Liveness
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

  // Revocation
  const entry = statuslist.entries?.find((e) => e.serial === serial);
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

  // Policy
  const policyAllowed = new Set<string>(policy.allowed_assurance ?? []);
  if (!policyAllowed.has(assurance)) {
    audit({ stage: "policy", outcome: "REJECT", reason: "assurance_invalid", details: { assurance, allowed: [...policyAllowed] } });
    return { ok: false, code: "POLICY_ASSURANCE_INVALID" as const };
  }
  audit({ stage: "policy", outcome: "ACCEPT" });

  return { ok: true, code: "VALID" as const };
}
