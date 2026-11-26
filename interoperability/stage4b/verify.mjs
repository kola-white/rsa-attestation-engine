// Independent JS implementation. No Go invocation, legacy CLI or Cvera client.
import { compactVerify, decodeProtectedHeader, importSPKI } from "jose";
import { createHash, createPublicKey } from "node:crypto";
const ISS = "https://cvera.app",
  VCT = ISS + "/credentials/employment-role/v1",
  POLICY = "employment.authoritative/manual-evidence-pilot/v1";
const hash = (x) => createHash("sha256").update(x).digest("hex");
const int = (x) => Number.isSafeInteger(x) && x >= 0;
const uuid = (x) =>
  typeof x === "string" &&
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/.test(x);
const digest = (x) => typeof x === "string" && /^[0-9a-f]{64}$/.test(x);
function publicKey(pem) {
  if (
    typeof pem !== "string" ||
    !/^-----BEGIN PUBLIC KEY-----\n[\s\S]+\n-----END PUBLIC KEY-----\s*$/.test(
      pem,
    )
  )
    throw Error("public SPKI required");
  const k = createPublicKey(pem),
    d = k.asymmetricKeyDetails;
  if (
    k.asymmetricKeyType !== "rsa" ||
    d.modulusLength < 3072 ||
    d.publicExponent !== 65537n
  )
    throw Error("unsupported key");
  return k;
}
async function assertion(raw, pem, iss, kid, typ) {
  publicKey(pem);
  const h = decodeProtectedHeader(raw);
  if (
    h.crit !== undefined ||
    h.b64 !== undefined ||
    h.alg !== "RS256" ||
    h.kid !== kid ||
    h.typ !== typ
  )
    throw Error("header mismatch");
  const { payload } = await compactVerify(raw, await importSPKI(pem, "RS256"), {
    algorithms: ["RS256"],
  });
  const m = JSON.parse(new TextDecoder().decode(payload));
  if (m.iss !== iss || m.synthetic !== true) throw Error("scope mismatch");
  return m;
}
export async function verify(input) {
  const { credential: raw, trust: t, policy: p } = input;
  const max = p?.maxStatusAgeSeconds ?? 900;
  if (!int(p?.now) || p.now === 0 || !int(max) || t?.synthetic !== true)
    throw Error("invalid verifier policy or non-synthetic trust");
  const r = {
    contract: "cvera.stage4b.portable-verification.v1",
    synthetic: true,
    cryptographicIntegrity: "UNKNOWN",
    claimBinding: "UNKNOWN",
    temporalValidity: "UNKNOWN",
    issuerAuthority: "UNKNOWN",
    credentialStatus: "UNKNOWN",
    holderAssurance: input.holder === "UNSUPPORTED" ? "UNSUPPORTED" : "UNKNOWN",
    eligibleWithoutHolder: false,
    overall: "INDETERMINATE",
  };
  const finish = () => {
    r.eligibleWithoutHolder =
      r.cryptographicIntegrity === "VALID" &&
      r.claimBinding === "VALID" &&
      r.temporalValidity === "VALID" &&
      r.issuerAuthority === "AUTHORIZED" &&
      r.credentialStatus === "GOOD";
    if (
      ["INVALID", "UNSUPPORTED"].includes(r.cryptographicIntegrity) ||
      r.claimBinding === "INVALID" ||
      r.temporalValidity === "INVALID" ||
      ["INVALID", "UNAUTHORIZED"].includes(r.issuerAuthority) ||
      ["INVALID", "REVOKED"].includes(r.credentialStatus)
    )
      r.overall = "REJECTED";
    return r;
  };
  if (
    typeof raw !== "string" ||
    Buffer.byteLength(raw) > 1048576 ||
    !raw.endsWith("~") ||
    raw.split("~").length !== 2
  ) {
    r.cryptographicIntegrity = "UNSUPPORTED";
    return finish();
  }
  const jwt = raw.slice(0, -1);
  let h, m, k;
  try {
    h = decodeProtectedHeader(jwt);
  } catch {
    r.cryptographicIntegrity = "INVALID";
    return finish();
  }
  if (h.crit !== undefined || h.b64 !== undefined) {
    r.cryptographicIntegrity = "INVALID";
    return finish();
  }
  if (h.alg !== "RS256" || h.typ !== "dc+sd-jwt") {
    r.cryptographicIntegrity = "UNSUPPORTED";
    return finish();
  }
  try {
    k = publicKey(t.publicKey);
    if (h.kid !== t.kid) throw Error("untrusted kid");
    const v = await compactVerify(jwt, await importSPKI(t.publicKey, "RS256"), {
      algorithms: ["RS256"],
    });
    m = JSON.parse(new TextDecoder().decode(v.payload));
    if (m.iss !== t.issuer) throw Error("untrusted issuer");
  } catch {
    r.cryptographicIntegrity = "INVALID";
    return finish();
  }
  r.cryptographicIntegrity = "VALID";
  if (
    m.vct !== VCT ||
    m.iss !== ISS ||
    t.issuer !== ISS ||
    m.claim_type !== "employment.role" ||
    m.verification_policy !== POLICY
  ) {
    r.cryptographicIntegrity = "UNSUPPORTED";
    return finish();
  }
  const e = m.evidence_binding ?? {},
    em = m.employment ?? {};
  const valid =
    [m.sub, m.jti, e.request_id, e.binding_id, e.manifest_id].every(uuid) &&
    p.expectedSubject &&
    m.sub === p.expectedSubject &&
    m.sub === e.subject_id &&
    typeof em.employer_id === "string" &&
    em.employer_id.length > 0 &&
    em.employer_id === e.employer_id &&
    typeof em.title === "string" &&
    em.title.trim() !== "" &&
    typeof e.case_id === "string" &&
    e.case_id !== "" &&
    e.manual_policy === "MANUAL_REVIEW_PILOT_V1" &&
    typeof e.authority_grant_id === "string" &&
    e.authority_grant_id !== "" &&
    int(e.request_version) &&
    e.request_version > 0 &&
    int(e.authority_recording_position) &&
    e.authority_recording_position > 0 &&
    [e.snapshot_sha256, e.manifest_sha256, m.credential_spki_sha256].every(
      digest,
    ) &&
    m.credential_spki_sha256 ===
      hash(k.export({ type: "spki", format: "der" }));
  r.claimBinding = valid ? "VALID" : "INVALID";
  r.temporalValidity =
    int(m.iat) &&
    int(m.nbf) &&
    int(m.exp) &&
    m.iat === m.nbf &&
    m.exp > m.iat &&
    p.now >= m.nbf &&
    p.now < m.exp
      ? "VALID"
      : "INVALID";
  const dh = hash(raw);
  if (input.authorityJws) {
    try {
      const a = await assertion(
        input.authorityJws,
        t.authorityKey,
        t.authorityIssuer,
        t.authorityKid,
        "cvera-stage4b-fixture-authority+jwt",
      );
      if (
        !int(a.iat) ||
        !int(a.exp) ||
        a.iat > p.now ||
        a.exp <= p.now ||
        a.credential_sha256 !== dh ||
        a.issuer !== m.iss ||
        a.employer_id !== e.employer_id ||
        a.claim_type !== m.claim_type
      )
        throw Error("authority binding");
      r.issuerAuthority = ["AUTHORIZED", "UNAUTHORIZED", "UNKNOWN"].includes(
        a.decision,
      )
        ? a.decision
        : "INVALID";
    } catch {
      r.issuerAuthority = "INVALID";
    }
  }
  if (input.status && input.status.availability !== "UNAVAILABLE") {
    try {
      if (input.status.availability !== "AVAILABLE")
        throw Error("availability");
      const a = await assertion(
        input.status.jws,
        t.statusKey,
        t.statusIssuer,
        t.statusKid,
        "cvera-stage4b-fixture-status+jwt",
      );
      if (
        !int(a.iat) ||
        a.iat > p.now ||
        a.credential_sha256 !== dh ||
        !Array.isArray(a.entries) ||
        a.entries.some((x) => !x || typeof x !== "object" || Array.isArray(x))
      )
        throw Error("status binding");
      const found = a.entries.filter((x) => x.credential_id === m.jti);
      if (found.length > 1) throw Error("ambiguous status");
      if (found.length === 1) {
        if (!["GOOD", "REVOKED"].includes(found[0].state))
          throw Error("invalid status");
        r.credentialStatus = p.now - a.iat > max ? "STALE" : found[0].state;
      }
    } catch {
      r.credentialStatus = "INVALID";
    }
  }
  return finish();
}
// Runner reads one bounded public input from stdin. Importing this file is safe.
if (
  process.argv[1] &&
  import.meta.url === new URL("file://" + process.argv[1]).href
) {
  try {
    const chunks = [];
    let size = 0;
    for await (const chunk of process.stdin) {
      size += chunk.length;
      if (size > 4 * 1024 * 1024) throw Error("invalid input size");
      chunks.push(chunk);
    }
    const b = Buffer.concat(chunks);
    console.log(JSON.stringify(await verify(JSON.parse(b))));
  } catch {
    console.error("invalid verifier input");
    process.exitCode = 2;
  }
}
