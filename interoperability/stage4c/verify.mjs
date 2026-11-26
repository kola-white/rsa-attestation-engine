// Independent JOSE implementation; only the unchanged credential layer is reused.
import {
  compactVerify,
  decodeJwt,
  decodeProtectedHeader,
  importSPKI,
  importJWK,
  SignJWT,
  exportJWK,
} from "jose";
import { createHash, createPublicKey } from "node:crypto";
import { verify as baseVerify } from "../stage4b/verify.mjs";
export const VCT = "https://cvera.app/credentials/employment-role/v1";
export const hash = (s) => createHash("sha256").update(s).digest("base64url");
const integer = (n) => Number.isSafeInteger(n) && n >= 0;
const window = (m, n) =>
  integer(m.iat) &&
  integer(m.exp) &&
  m.iat <= n &&
  n - m.iat <= 300 &&
  m.exp > n &&
  m.exp > m.iat &&
  m.exp - m.iat <= 300;
export function publicJWK(j) {
  if (
    !j ||
    Object.keys(j).sort().join(",") !== "e,kty,n" ||
    j.kty !== "RSA" ||
    j.e !== "AQAB" ||
    typeof j.n !== "string" ||
    !/^[A-Za-z0-9_-]+$/.test(j.n) ||
    Buffer.from(j.n, "base64url")[0] === 0
  )
    throw Error("public RSA JWK required");
  const k = createPublicKey({ key: j, format: "jwk" });
  if (k.asymmetricKeyDetails.modulusLength < 3072) throw Error("weak key");
  return j;
}
function publicPEM(s) {
  const k = createPublicKey(s);
  if (
    !/^-----BEGIN PUBLIC KEY-----\n[\s\S]+\n-----END PUBLIC KEY-----\s*$/.test(
      s,
    ) ||
    k.asymmetricKeyType !== "rsa" ||
    k.asymmetricKeyDetails.modulusLength < 3072 ||
    k.asymmetricKeyDetails.publicExponent !== 65537n
  )
    throw Error("SPKI required");
  return s;
}
async function signed(raw, key, typ, kid) {
  if (typeof raw !== "string" || raw.length > 1048576) throw Error("JWT size");
  const h = decodeProtectedHeader(raw);
  if (
    Object.keys(h).some(
      (x) =>
        !(
          typ === "openid4vci-proof+jwt"
            ? ["typ", "alg", "jwk"]
            : ["typ", "alg", "kid"]
        ).includes(x),
    ) ||
    h.typ !== typ ||
    h.alg !== "RS256" ||
    (kid && h.kid !== kid)
  )
    throw Error("header");
  if (typ === "openid4vci-proof+jwt") {
    const j = publicJWK(h.jwk),
      expected = await exportJWK(key);
    if (j.n !== expected.n || j.e !== expected.e)
      throw Error("proof key mismatch");
  }
  const { payload } = await compactVerify(raw, key, { algorithms: ["RS256"] });
  return JSON.parse(new TextDecoder().decode(payload));
}
const query = {
  credentials: [
    {
      format: "dc+sd-jwt",
      id: "employment",
      meta: { vct_values: [VCT] },
      require_cryptographic_holder_binding: true,
    },
  ],
};
function canonical(x) {
  if (Array.isArray(x)) return x.map(canonical);
  if (x && typeof x === "object")
    return Object.fromEntries(
      Object.keys(x)
        .sort()
        .map((k) => [k, canonical(x[k])]),
    );
  return x;
}
const queryOK = (q) =>
  JSON.stringify(canonical(q)) === JSON.stringify(canonical(query));
export const dcql = () => structuredClone(query);
async function authenticate(inpt) {
  try {
    let m = decodeJwt(inpt.request);
    const vs = (inpt.verifiers ?? []).filter((v) => v.id === m.client_id);
    if (!vs.length) return [null, null, "UNKNOWN"];
    if (vs.length !== 1) throw Error("ambiguous client");
    const v = vs[0];
    m = await signed(
      inpt.request,
      await importSPKI(publicPEM(v.key), "RS256"),
      "oauth-authz-req+jwt",
      v.kid,
    );
    if (
      Object.keys(m).some(
        (k) =>
          ![
            "iss",
            "client_id",
            "aud",
            "iat",
            "exp",
            "response_type",
            "response_mode",
            "response_uri",
            "nonce",
            "state",
            "dcql_query",
          ].includes(k),
      )
    )
      throw Error("unsupported request parameter");
    if (
      typeof v.id !== "string" ||
      !v.id ||
      v.id.includes(":") ||
      m.iss !== v.id ||
      m.aud !== "https://self-issued.me/v2" ||
      !window(m, inpt.base.policy.now) ||
      m.response_type !== "vp_token" ||
      m.response_mode !== "direct_post" ||
      !v.responseURI.startsWith("https://") ||
      m.response_uri !== v.responseURI ||
      typeof m.nonce !== "string" ||
      m.nonce.length < 22 ||
      typeof m.state !== "string" ||
      m.state.length < 22
    )
      throw Error("request context");
    return [m, v, "AUTHENTICATED"];
  } catch {
    return [null, null, "INVALID"];
  }
}
async function initial(i) {
  let br;
  try {
    br = await baseVerify(i.base);
  } catch {
    return [
      {
        contract: "cvera.stage4c.presentation.v1",
        credential: {
          contract: "cvera.stage4b.portable-verification.v1",
          synthetic: true,
          cryptographicIntegrity: "UNKNOWN",
          claimBinding: "UNKNOWN",
          temporalValidity: "UNKNOWN",
          issuerAuthority: "UNKNOWN",
          credentialStatus: "UNKNOWN",
          holderAssurance: "UNKNOWN",
          eligibleWithoutHolder: false,
          overall: "INDETERMINATE",
        },
        verifierAuthentication: "UNKNOWN",
        disclosureAuthorization: "UNKNOWN",
        holderAuthorization: "UNKNOWN",
        holderPossession: "UNKNOWN",
        transactionBinding: "UNKNOWN",
        overall: "DENIED",
      },
      null,
      null,
    ];
  }
  const r = {
    contract: "cvera.stage4c.presentation.v1",
    credential: br,
    verifierAuthentication: "UNKNOWN",
    disclosureAuthorization: "UNKNOWN",
    holderAuthorization: "UNKNOWN",
    holderPossession: "UNKNOWN",
    transactionBinding: "UNKNOWN",
    overall: "DENIED",
  };
  const [m, v, a] = await authenticate(i);
  r.verifierAuthentication = a;
  return [r, m, v];
}
export async function authorize(i) {
  const [r, m, v] = await initial(i);
  if (r.verifierAuthentication !== "AUTHENTICATED") return r;
  if (
    v.allow !== true ||
    i.allowFullDisclosure !== true ||
    !queryOK(m.dcql_query)
  ) {
    r.disclosureAuthorization = "DENIED";
    return r;
  }
  r.disclosureAuthorization = "AUTHORIZED";
  const c = i.consent;
  if (!c) return r;
  if (
    c.granted !== true ||
    !integer(c.expires) ||
    c.expires <= i.base.policy.now ||
    c.requestHash !== hash(i.request) ||
    c.credentialHash !== hash(i.base.credential)
  ) {
    r.holderAuthorization = "DENIED";
    return r;
  }
  r.holderAuthorization = "AUTHORIZED";
  if (r.credential.eligibleWithoutHolder) r.overall = "AUTHORIZED_TO_PRESENT";
  return r;
}
function holderKey(cred) {
  const m = decodeJwt(cred.slice(0, -1));
  if (m._sd_alg !== undefined && m._sd_alg !== "sha-256")
    throw Error("hash algorithm");
  if (!m.cnf || Object.keys(m.cnf).join() !== "jwk") throw Error("cnf");
  return publicJWK(m.cnf.jwk);
}
// Returns no protected bytes on denial; consent is trusted local state, not
// an unsigned assertion forwarded by a verifier or an attacker.
export async function present(i, privateKey) {
  const r = await authorize(i);
  if (r.overall !== "AUTHORIZED_TO_PRESENT") throw Error("disclosure denied");
  const jwk = holderKey(i.base.credential),
    actual = createPublicKey(privateKey).export({ format: "jwk" });
  if (jwk.n !== actual.n || jwk.e !== actual.e)
    throw Error("holder key mismatch");
  const m = decodeJwt(i.request);
  const kb = await new SignJWT({
    iat: i.base.policy.now,
    aud: m.client_id,
    nonce: m.nonce,
    sd_hash: hash(i.base.credential),
  })
    .setProtectedHeader({ alg: "RS256", typ: "kb+jwt" })
    .sign(privateKey);
  return i.base.credential + kb;
}
// Synchronous check-and-mark occurs after all async validation; Promise.all
// duplicates cannot both consume a challenge in this single-process model.
export class Store {
  #seen = new Set();
  consume(k) {
    if (this.#seen.has(k)) return false;
    this.#seen.add(k);
    return true;
  }
}
export async function verify(i, store) {
  const [r, m, v] = await initial(i);
  r.holderAuthorization = "NOT_OBSERVABLE";
  if (r.verifierAuthentication !== "AUTHENTICATED") return r;
  if (
    v.allow !== true ||
    i.allowFullDisclosure !== true ||
    !queryOK(m.dcql_query)
  ) {
    r.disclosureAuthorization = "DENIED";
    return r;
  }
  r.disclosureAuthorization = "AUTHORIZED";
  if (!r.credential.eligibleWithoutHolder) return r;
  r.holderPossession = "INVALID";
  try {
    if (
      typeof i.presentation !== "string" ||
      !i.presentation.startsWith(i.base.credential)
    )
      return r;
    const key = await importJWK(holderKey(i.base.credential), "RS256");
    const kb = await signed(
      i.presentation.slice(i.base.credential.length),
      key,
      "kb+jwt",
    );
    if (
      ["exp", "nbf"].some(
        (k) =>
          Object.hasOwn(kb, k) &&
          (!integer(kb[k]) ||
            (k === "exp"
              ? kb[k] <= i.base.policy.now
              : kb[k] > i.base.policy.now)),
      )
    )
      return r;
    if (
      kb.aud !== v.id ||
      kb.nonce !== m.nonce ||
      kb.sd_hash !== hash(i.base.credential) ||
      !integer(kb.iat) ||
      kb.iat > i.base.policy.now ||
      i.base.policy.now - kb.iat > 300
    )
      return r;
    r.holderPossession = "VALID";
    r.transactionBinding = "INVALID";
    const t = i.transaction;
    if (
      !t ||
      t.verifier !== v.id ||
      t.nonce !== m.nonce ||
      t.requestHash !== hash(i.request) ||
      !integer(t.expires) ||
      t.expires <= i.base.policy.now ||
      t.expires > m.exp ||
      !store
    )
      return r;
    if (!store.consume(t.verifier + "/" + t.nonce)) {
      r.transactionBinding = "REPLAYED";
      return r;
    }
    r.transactionBinding = "VALID";
    r.overall = "ACCEPTED_SYNTHETIC_PRESENTATION";
    return r;
  } catch {
    return r;
  }
}
// Synthetic integration seam: caller has already established issuance authority.
export async function issueBound(
  claims,
  intended,
  proof,
  context,
  store,
  sign,
) {
  publicJWK(intended);
  const { issuer, nonce, now, expires } = context;
  if (
    claims.iss !== issuer ||
    typeof nonce !== "string" ||
    nonce.length < 22 ||
    !integer(now) ||
    now === 0 ||
    typeof issuer !== "string" ||
    issuer === "" ||
    !store ||
    typeof sign !== "function" ||
    !integer(expires) ||
    expires <= now ||
    expires - now > 300
  )
    throw Error("issuer context");
  const p = await signed(
    proof,
    await importJWK(intended, "RS256"),
    "openid4vci-proof+jwt",
  );
  if (
    ["exp", "nbf"].some(
      (k) =>
        Object.hasOwn(p, k) &&
        (!integer(p[k]) || (k === "exp" ? p[k] <= now : p[k] > now)),
    ) ||
    p.aud !== issuer ||
    p.nonce !== nonce ||
    !integer(p.iat) ||
    p.iat > now ||
    now - p.iat > 300
  )
    throw Error("proof");
  if (!store.consume(issuer + "/" + nonce)) throw Error("proof replay");
  return sign({ ...claims, cnf: { jwk: intended } });
}
