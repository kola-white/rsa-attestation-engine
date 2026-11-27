// Independent portable trust consumer. No network, key signing or Cvera client.
import {
  compactVerify,
  decodeJwt,
  decodeProtectedHeader,
  importJWK,
} from "jose";
import { createPublicKey, createHash } from "node:crypto";
import { inflateSync } from "node:zlib";
import { verifyCredential } from "../stage4b/verify.mjs";
export const VCT = "https://cvera.app/credentials/employment-role/v1",
  PROFILE = "draft-ietf-oauth-sd-jwt-vc-18",
  CONTRACT = "cvera.stage4d.portable-trust.v1";
const int = (n) => Number.isSafeInteger(n) && n >= 0;
const max = (n) => n ?? 900;
const times = (a, b) => int(a) && int(b) && b > a;
const uri = (s) => {
  try {
    const u = new URL(s);
    return (
      u.protocol === "https:" &&
      u.hostname !== "" &&
      !u.username &&
      !u.password &&
      !u.search &&
      !u.hash
    );
  } catch {
    return false;
  }
};
function fields(o, keys) {
  if (
    !o ||
    typeof o !== "object" ||
    Array.isArray(o) ||
    Object.keys(o).some((k) => !keys.includes(k))
  )
    throw Error("unsupported public fields");
}
export function publicKey(j) {
  fields(j, ["kty", "n", "e", "kid", "alg", "use", "key_ops"]);
  if (
    j.kty !== "RSA" ||
    j.e !== "AQAB" ||
    typeof j.kid !== "string" ||
    !j.kid ||
    j.alg !== "RS256" ||
    j.use !== "sig" ||
    !Array.isArray(j.key_ops) ||
    j.key_ops.length !== 1 ||
    j.key_ops[0] !== "verify" ||
    typeof j.n !== "string" ||
    !/^[A-Za-z0-9_-]+$/.test(j.n)
  )
    throw Error("public signing key required");
  const raw = Buffer.from(j.n, "base64url");
  if (raw.toString("base64url") !== j.n) throw Error("canonical key");
  if (!raw.length || raw[0] === 0 || raw.length > 1024) throw Error("modulus");
  const k = createPublicKey({ key: j, format: "jwk" });
  if (k.asymmetricKeyDetails.modulusLength < 3072) throw Error("weak RSA");
  return k;
}
async function token(raw, j, typ) {
  if (typeof raw !== "string" || raw.length > 1048576) throw Error("size");
  publicKey(j);
  for (const part of raw.split(".")) {
    if (
      !/^[A-Za-z0-9_-]+$/.test(part) ||
      Buffer.from(part, "base64url").toString("base64url") !== part
    )
      throw Error("canonical base64url");
  }
  const h = decodeProtectedHeader(raw);
  fields(h, ["alg", "typ", "kid"]);
  if (
    Object.keys(h).length !== 3 ||
    h.alg !== "RS256" ||
    h.typ !== typ ||
    h.kid !== j.kid
  )
    throw Error("header");
  const { payload } = await compactVerify(raw, await importJWK(j, "RS256"), {
    algorithms: ["RS256"],
  });
  return JSON.parse(new TextDecoder("utf-8", { fatal: true }).decode(payload));
}
export class State {
  #entries = new Map();
  retainStatus(tk, tr, sk, raw, v, at, floor) {
    if (this.#entries.get(tk)?.raw !== tr) return "TRUST_CHANGED";
    return this.retain(sk, raw, v, at, floor) ? "RETAINED" : "ROLLED_BACK";
  }
  matches(tk, tr, sk, sr) {
    return [
      this.#entries.get(tk)?.raw === tr,
      this.#entries.get(sk)?.raw === sr,
    ];
  }
  cached(k) {
    return this.#entries.get(k)?.raw ?? "";
  }
  retain(k, raw, v, iat, floor) {
    const old = this.#entries.get(k);
    if (
      v < floor ||
      (old &&
        (v < old.version ||
          iat < old.iat ||
          (v === old.version && raw !== old.raw)))
    )
      return false;
    this.#entries.set(k, { raw, version: v, iat });
    return true;
  }
}
const scope = (i) =>
  createHash("sha256")
    .update(
      JSON.stringify([
        i.anchor.issuer,
        i.anchor.kid,
        i.anchor.key.n,
        i.anchor.synthetic,
        i.policy.issuer,
        i.policy.trustURI,
      ]),
    )
    .digest("base64url");
function artifact(a, s, k) {
  if (!a) return ["", "UNKNOWN"];
  if (a.availability === "UNAVAILABLE") return [s.cached(k), "UNKNOWN"];
  if (a.availability !== "AVAILABLE" || !a.jws) return ["", "INVALID"];
  return [a.jws, "VALID"];
}
function manifest(m, i) {
  fields(m, [
    "iss",
    "sub",
    "iat",
    "exp",
    "version",
    "synthetic",
    "profile",
    "credential_issuer",
    "vct",
    "keys",
    "authority",
    "status",
  ]);
  if (
    m.iss !== i.anchor.issuer ||
    m.sub !== i.policy.trustURI ||
    m.synthetic !== i.anchor.synthetic ||
    m.profile !== PROFILE ||
    m.credential_issuer !== i.policy.issuer ||
    m.vct !== VCT ||
    !times(m.iat, m.exp) ||
    m.iat > i.policy.now ||
    !int(m.version) ||
    m.version < 1 ||
    !Array.isArray(m.keys) ||
    !m.keys.length ||
    m.keys.length > 32 ||
    !Array.isArray(m.authority) ||
    m.authority.length > 128
  )
    throw Error("manifest context");
  fields(m.status, ["issuer", "uri", "purpose", "jwk"]);
  if (
    m.status.purpose !== "revocation" ||
    !uri(m.status.uri) ||
    typeof m.status.issuer !== "string" ||
    !m.status.issuer
  )
    throw Error("status context");
  publicKey(m.status.jwk);
  const seen = new Set();
  for (const k of m.keys) {
    fields(k, ["jwk", "from", "until"]);
    publicKey(k.jwk);
    if (seen.has(k.jwk.kid) || !times(k.from, k.until))
      throw Error("key windows");
    seen.add(k.jwk.kid);
  }
  for (const g of m.authority) {
    fields(g, [
      "employer_id",
      "claim_type",
      "policy",
      "from",
      "until",
      "decision",
    ]);
    if (
      typeof g.employer_id !== "string" ||
      !g.employer_id ||
      typeof g.claim_type !== "string" ||
      !g.claim_type ||
      typeof g.policy !== "string" ||
      !g.policy ||
      !times(g.from, g.until) ||
      !["AUTHORIZED", "UNAUTHORIZED", "UNKNOWN"].includes(g.decision)
    )
      throw Error("authority");
  }
}
function authority(gs, c, at) {
  const a = gs.filter(
    (g) =>
      g.employer_id === c.employment.employer_id &&
      g.claim_type === c.claim_type &&
      g.policy === c.verification_policy &&
      g.from <= at &&
      at < g.until,
  );
  return a.length > 1 ? "INVALID" : (a[0]?.decision ?? "UNKNOWN");
}
const blank = (i) => ({
  contract: CONTRACT,
  trustMaterial: "UNKNOWN",
  verificationKey: "UNKNOWN",
  currentAuthority: "UNKNOWN",
  issuanceAuthority: "UNKNOWN",
  credential: {
    contract: "cvera.stage4b.portable-verification.v1",
    synthetic: i.anchor.synthetic,
    cryptographicIntegrity: "UNKNOWN",
    claimBinding: "UNKNOWN",
    temporalValidity: "UNKNOWN",
    issuerAuthority: "UNKNOWN",
    credentialStatus: "UNKNOWN",
    holderAssurance: "UNKNOWN",
    eligibleWithoutHolder: false,
    overall: "INDETERMINATE",
  },
  overall: "INDETERMINATE",
});
export async function verify(i, state) {
  const r = blank(i),
    p = i.policy;
  if (
    !state ||
    !int(p.now) ||
    !p.now ||
    !int(max(p.maxStatusAgeSeconds)) ||
    !int(max(p.maxTrustAgeSeconds)) ||
    !int(p.minTrustVersion) ||
    !int(p.minStatusVersion) ||
    !uri(p.trustURI) ||
    !p.issuer ||
    !["CURRENT", "ISSUANCE_TIME"].includes(p.authorityMode) ||
    !i.anchor.issuer ||
    i.anchor.kid !== i.anchor.key.kid ||
    (i.anchor.synthetic && !p.allowSynthetic)
  )
    throw Error("invalid trusted policy");
  const sc = scope(i);
  let decisionTrust, decisionStatus, statusScope;
  const finish = () => {
    const b = r.credential;
    b.eligibleWithoutHolder =
      r.trustMaterial === "VALID" &&
      r.verificationKey === "AUTHENTICATED" &&
      b.cryptographicIntegrity === "VALID" &&
      b.claimBinding === "VALID" &&
      b.temporalValidity === "VALID" &&
      b.issuerAuthority === "AUTHORIZED" &&
      b.credentialStatus === "GOOD";
    if (b.eligibleWithoutHolder) {
      const [t, s] = state.matches(
        sc + "/trust",
        decisionTrust,
        statusScope,
        decisionStatus,
      );
      if (!t) {
        r.trustMaterial = "ROLLED_BACK";
        b.issuerAuthority = "UNKNOWN";
        r.currentAuthority = "UNKNOWN";
        r.issuanceAuthority = "UNKNOWN";
      }
      if (!s) b.credentialStatus = "INVALID";
      b.eligibleWithoutHolder = t && s;
    }
    if (b.eligibleWithoutHolder) r.overall = "ELIGIBLE_WITHOUT_HOLDER";
    else if (
      r.verificationKey === "INVALID" ||
      ["INVALID", "ROLLED_BACK"].includes(r.trustMaterial) ||
      b.cryptographicIntegrity === "INVALID" ||
      b.claimBinding === "INVALID" ||
      b.temporalValidity === "INVALID" ||
      ["INVALID", "REVOKED"].includes(b.credentialStatus) ||
      ["UNAUTHORIZED", "INVALID"].includes(b.issuerAuthority)
    )
      r.overall = "REJECTED";
    b.overall = r.overall === "REJECTED" ? "REJECTED" : "INDETERMINATE";
    return r;
  };
  let [raw, a] = artifact(i.trustMaterial, state, sc + "/trust");
  if (!raw) {
    r.trustMaterial = a;
    return finish();
  }
  decisionTrust = raw;
  let m;
  try {
    m = await token(raw, i.anchor.key, "cvera-trust-material+jwt");
    manifest(m, i);
  } catch {
    r.trustMaterial = "INVALID";
    return finish();
  }
  if (!state.retain(sc + "/trust", raw, m.version, m.iat, p.minTrustVersion)) {
    r.trustMaterial = "ROLLED_BACK";
    return finish();
  }
  r.trustMaterial = "VALID";
  if (m.exp <= p.now || p.now - m.iat > max(p.maxTrustAgeSeconds))
    r.trustMaterial = "STALE";
  let h;
  try {
    h = decodeProtectedHeader(i.credential.slice(0, -1));
  } catch {
    r.credential.cryptographicIntegrity = "INVALID";
    return finish();
  }
  const selected = m.keys.find((k) => k.jwk.kid === h.kid);
  if (!selected) return finish();
  r.verificationKey = "AUTHENTICATED";
  const pem = publicKey(selected.jwk).export({ format: "pem", type: "spki" });
  r.credential = await verifyCredential({
    credential: i.credential,
    trust: {
      issuer: m.credential_issuer,
      kid: h.kid,
      publicKey: pem,
      synthetic: i.anchor.synthetic,
    },
    policy: {
      now: p.now,
      expectedSubject: p.expectedSubject,
      maxStatusAgeSeconds: p.maxStatusAgeSeconds,
    },
  });
  if (
    r.credential.cryptographicIntegrity !== "VALID" ||
    r.credential.claimBinding !== "VALID" ||
    r.credential.temporalValidity !== "VALID"
  )
    return finish();
  const c = decodeJwt(i.credential.slice(0, -1));
  if (c.iat < selected.from || c.iat >= selected.until) {
    r.verificationKey = "INVALID";
    return finish();
  }
  if (r.trustMaterial !== "VALID") return finish();
  r.issuanceAuthority = authority(m.authority, c, c.iat);
  r.currentAuthority = authority(m.authority, c, p.now);
  r.credential.issuerAuthority =
    p.authorityMode === "CURRENT" ? r.currentAuthority : r.issuanceAuthority;
  if (
    c.status != null &&
    (typeof c.status !== "object" || Array.isArray(c.status))
  ) {
    r.credential.credentialStatus = "INVALID";
    return finish();
  }
  const ref = c.status?.status_list;
  if (ref != null && (typeof ref !== "object" || Array.isArray(ref))) {
    r.credential.credentialStatus = "INVALID";
    return finish();
  }
  if (!ref) return finish();
  if (!int(ref.idx) || ref.uri !== m.status.uri) {
    r.credential.credentialStatus = "INVALID";
    return finish();
  }
  let [stRaw, available] = artifact(
    i.statusMaterial,
    state,
    sc + "/status/" + m.status.uri,
  );
  if (!stRaw) {
    r.credential.credentialStatus = available;
    return finish();
  }
  decisionStatus = stRaw;
  statusScope = sc + "/status/" + m.status.uri;
  let st, list;
  try {
    ({ st, list } = await readStatus(stRaw, m, i));
  } catch {
    r.credential.credentialStatus = "INVALID";
    return finish();
  }
  const retained = state.retainStatus(
    sc + "/trust",
    raw,
    sc + "/status/" + m.status.uri,
    stRaw,
    st.cvera_version,
    st.iat,
    p.minStatusVersion,
  );
  if (retained === "TRUST_CHANGED") {
    r.trustMaterial = "ROLLED_BACK";
    r.credential.issuerAuthority = "UNKNOWN";
    r.currentAuthority = "UNKNOWN";
    r.issuanceAuthority = "UNKNOWN";
    return finish();
  }
  if (retained === "ROLLED_BACK") {
    r.credential.credentialStatus = "INVALID";
    return finish();
  }

  if (ref.idx >= list.length * 8) {
    r.credential.credentialStatus = "UNKNOWN";
    return finish();
  }
  if (
    st.exp <= p.now ||
    p.now - st.iat > max(p.maxStatusAgeSeconds) ||
    p.now - st.iat > st.ttl
  ) {
    r.credential.credentialStatus = "STALE";
    return finish();
  }
  r.credential.credentialStatus =
    list[Math.floor(ref.idx / 8)] & (1 << (ref.idx % 8)) ? "REVOKED" : "GOOD";
  return finish();
}

async function readStatus(raw, m, i) {
  const p = i.policy;
  let st, list;
  st = await token(raw, m.status.jwk, "statuslist+jwt");
  fields(st, [
    "iss",
    "sub",
    "iat",
    "exp",
    "ttl",
    "cvera_version",
    "synthetic",
    "cvera_vct",
    "cvera_credential_issuer",
    "status_list",
  ]);
  fields(st.status_list, ["bits", "lst"]);
  if (
    st.iss !== m.status.issuer ||
    st.sub !== m.status.uri ||
    st.synthetic !== i.anchor.synthetic ||
    st.cvera_vct !== m.vct ||
    st.cvera_credential_issuer !== m.credential_issuer ||
    !times(st.iat, st.exp) ||
    st.iat > p.now ||
    !int(st.ttl) ||
    !st.ttl ||
    !int(st.cvera_version) ||
    st.cvera_version < 1 ||
    st.status_list.bits !== 1 ||
    typeof st.status_list.lst !== "string" ||
    !/^[A-Za-z0-9_-]+$/.test(st.status_list.lst)
  )
    throw Error("status context");
  const buf = Buffer.from(st.status_list.lst, "base64url");
  if (buf.toString("base64url") !== st.status_list.lst)
    throw Error("canonical list");
  if (buf.length > 1048576) throw Error("compressed size");
  const expanded = inflateSync(buf, { maxOutputLength: 1048576, info: true });
  if (expanded.engine.bytesWritten !== buf.length) throw Error("trailing zlib");
  list = expanded.buffer;
  if (list.length < 16384) throw Error("privacy cohort");
  return { st, list };
}
// Publication inspection is schema-whitelisted: no credentials, evidence,
// reviewer identities or arbitrary operator records can enter this format.
export async function inspectPublication(
  trustJWS,
  statusJWS,
  anchor,
  policy,
  historical = false,
) {
  const i = { anchor, policy };
  if (
    !int(policy.now) ||
    policy.now === 0 ||
    !int(max(policy.maxStatusAgeSeconds)) ||
    !int(max(policy.maxTrustAgeSeconds)) ||
    !int(policy.minTrustVersion ?? 0) ||
    !int(policy.minStatusVersion ?? 0) ||
    !uri(policy.trustURI) ||
    !policy.issuer ||
    !["CURRENT", "ISSUANCE_TIME"].includes(policy.authorityMode) ||
    !anchor.issuer ||
    typeof anchor.synthetic !== "boolean"
  )
    throw Error("invalid publication policy");
  const m = await token(trustJWS, anchor.key, "cvera-trust-material+jwt");
  manifest(m, i);
  if (
    anchor.kid !== anchor.key.kid ||
    (anchor.synthetic && !policy.allowSynthetic) ||
    (!historical &&
      (m.exp <= policy.now ||
        policy.now - m.iat > max(policy.maxTrustAgeSeconds)))
  )
    throw Error("publication trust stale");
  const { st } = await readStatus(statusJWS, m, i);
  if (
    !historical &&
    (st.exp <= policy.now ||
      policy.now - st.iat > max(policy.maxStatusAgeSeconds) ||
      policy.now - st.iat > st.ttl ||
      m.version < (policy.minTrustVersion ?? 0) ||
      st.cvera_version < (policy.minStatusVersion ?? 0))
  )
    throw Error("publication freshness or floor");
  return {
    trustVersion: m.version,
    statusVersion: st.cvera_version,
    trustIat: m.iat,
    statusIat: st.iat,
  };
}

// This factory is trusted integration code, never a serialized hint from a holder.
export function bindEvaluator(input, state) {
  return async (base) => {
    if (
      base.credential !== input.credential ||
      base.policy.now !== input.policy.now ||
      base.policy.expectedSubject !== input.policy.expectedSubject ||
      !input.anchor.synthetic
    )
      throw Error("presentation context mismatch");
    return (await verify(input, state)).credential;
  };
}
