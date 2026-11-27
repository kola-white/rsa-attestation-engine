import fs from "node:fs";
// Synthetic runtime fixture generator. Private keys never leave this process.
import { generateKeyPairSync, createHash } from "node:crypto";
import { readFileSync, writeFileSync, mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { createServer } from "node:http";
import { deflateSync } from "node:zlib";
import { SignJWT, decodeJwt } from "jose";
import assert from "node:assert/strict";
import { verify, State, VCT, PROFILE, bindEvaluator } from "./verify.mjs";
import { publish, receive, acquire } from "./distribution.mjs";
import { issueBound, Store, present, dcql, hash } from "../stage4c/verify.mjs";
const pair = () => generateKeyPairSync("rsa", { modulusLength: 3072 });
const root = pair(),
  issuer = pair(),
  statusSigner = pair(),
  holder = pair(),
  verifierA = pair(),
  verifierB = pair(),
  wrong = pair();
const jwk = (k, kid) => ({
  ...k.publicKey.export({ format: "jwk" }),
  kid,
  alg: "RS256",
  use: "sig",
  key_ops: ["verify"],
});
const sign = (m, k, typ, kid) =>
  new SignJWT(m)
    .setProtectedHeader({ alg: "RS256", typ, kid })
    .sign(k.privateKey);
const base = JSON.parse(
  readFileSync(new URL("../stage4b/fixtures/vectors.json", import.meta.url)),
).find((x) => x.name === "fresh").input;
const now = 1800000000,
  trustURI = "https://trust.synthetic.example.invalid/bundles/employment",
  listURI = "https://status.synthetic.example.invalid/lists/cohort-a";
const claims = decodeJwt(base.credential.slice(0, -1));
claims.iat = now - 60;
claims.nbf = now - 60;
claims.exp = now + 3600;
claims.status = { status_list: { idx: 37, uri: listURI } };
claims.credential_spki_sha256 = createHash("sha256")
  .update(issuer.publicKey.export({ format: "der", type: "spki" }))
  .digest("hex");
const holderJWK = holder.publicKey.export({ format: "jwk" }),
  nonce = "issuer-stage4d-00000000000000000001";
const proof = await new SignJWT({ aud: claims.iss, nonce, iat: now })
  .setProtectedHeader({
    alg: "RS256",
    typ: "openid4vci-proof+jwt",
    jwk: holderJWK,
  })
  .sign(holder.privateKey);
let issuances = 0;
const credential = await issueBound(
  claims,
  holderJWK,
  proof,
  { issuer: claims.iss, nonce, now, expires: now + 120 },
  new Store(),
  async (c) => {
    issuances++;
    return (await sign(c, issuer, "dc+sd-jwt", "credential")) + "~";
  },
);
const anchor = {
  issuer: "https://trust.synthetic.example.invalid",
  kid: "root",
  key: jwk(root, "root"),
  synthetic: true,
};
const policy = {
  now,
  expectedSubject: claims.sub,
  issuer: claims.iss,
  trustURI,
  allowSynthetic: true,
  authorityMode: "CURRENT",
  minTrustVersion: 0,
  minStatusVersion: 0,
};
const grant = {
  employer_id: claims.employment.employer_id,
  claim_type: claims.claim_type,
  policy: claims.verification_policy,
  from: now - 1000,
  until: now + 5000,
  decision: "AUTHORIZED",
};
const manifest = {
  iss: anchor.issuer,
  sub: trustURI,
  iat: now,
  exp: now + 3600,
  version: 1,
  synthetic: true,
  profile: PROFILE,
  credential_issuer: claims.iss,
  vct: VCT,
  keys: [
    { jwk: jwk(issuer, "credential"), from: now - 1000, until: now + 5000 },
  ],
  authority: [grant],
  status: {
    issuer: "https://status.synthetic.example.invalid",
    uri: listURI,
    purpose: "revocation",
    jwk: jwk(statusSigner, "status"),
  },
};
const list = Buffer.alloc(16384);
const statusClaims = {
  iss: manifest.status.issuer,
  sub: listURI,
  iat: now,
  exp: now + 3600,
  ttl: 900,
  cvera_version: 1,
  synthetic: true,
  cvera_vct: VCT,
  cvera_credential_issuer: claims.iss,
  status_list: {
    bits: 1,
    lst: deflateSync(list, { level: 9 }).toString("base64url"),
  },
};
const trustJWS = (m) => sign(m, root, "cvera-trust-material+jwt", "root"),
  statusJWS = (s) => sign(s, statusSigner, "statuslist+jwt", "status");
const canonical = {
  credential,
  anchor,
  policy,
  trustMaterial: { availability: "AVAILABLE", jws: await trustJWS(manifest) },
  statusMaterial: {
    availability: "AVAILABLE",
    jws: await statusJWS(statusClaims),
  },
};
const fresh = {
  overall: "ELIGIBLE_WITHOUT_HOLDER",
  "credential.cryptographicIntegrity": "VALID",
  "credential.issuerAuthority": "AUTHORIZED",
  "credential.credentialStatus": "GOOD",
  verificationKey: "AUTHENTICATED",
  trustMaterial: "VALID",
};
policy.authorityMode = "CURRENT";
const cases = [];
const add = (name, inputs, expected, extra = {}) =>
  cases.push({
    name,
    inputs: Array.isArray(inputs) ? inputs : [inputs],
    expected: Array.isArray(expected) ? expected : [expected],
    ...extra,
  });
add("authentic-portable-trust", canonical, fresh);
async function alterManifest(
  name,
  change,
  expected = { trustMaterial: "INVALID", overall: "REJECTED" },
) {
  const i = structuredClone(canonical),
    m = structuredClone(manifest);
  change(m);
  i.trustMaterial.jws = await trustJWS(m);
  add(name, i, expected);
  return i;
}
async function alterStatus(
  name,
  change,
  expected = { "credential.credentialStatus": "INVALID", overall: "REJECTED" },
) {
  const i = structuredClone(canonical),
    s = structuredClone(statusClaims);
  change(s);
  i.statusMaterial.jws = await statusJWS(s);
  add(name, i, expected);
  return i;
}
for (const [name, change] of [
  ["unknown-key", (m) => (m.keys[0].jwk.kid = "other")],
  [
    "wrong-issuer",
    (m) => (m.credential_issuer = "https://other.example.invalid"),
  ],
  ["wrong-profile", (m) => (m.profile = "unsupported")],
  ["wrong-type-data", (m) => (m.authority = "bad")],
  ["malformed-JWK", (m) => (m.keys[0].jwk.key_ops = ["sign"])],
  [
    "future-trust",
    (m) => {
      m.iat = now + 1;
    },
  ],
  ["prohibited-reviewer-data", (m) => (m.reviewerId = "prohibited")],
  [
    "substituted-trust-uri",
    (m) => (m.sub = "https://other.example.invalid/bundle"),
  ],
  ["private-evidence-field", (m) => (m.evidence = { proof: "prohibited" })],
])
  await alterManifest(
    name,
    change,
    name === "unknown-key"
      ? {
          trustMaterial: "VALID",
          verificationKey: "UNKNOWN",
          overall: "INDETERMINATE",
        }
      : undefined,
  );
await alterManifest(
  "key-invalid-at-issuance",
  (m) => (m.keys[0].from = now - 1),
  {
    verificationKey: "INVALID",
    "credential.cryptographicIntegrity": "VALID",
    overall: "REJECTED",
  },
);
await alterManifest("stale-trust-keeps-crypto", (m) => (m.iat = now - 901), {
  trustMaterial: "STALE",
  "credential.cryptographicIntegrity": "VALID",
  "credential.issuerAuthority": "UNKNOWN",
  overall: "INDETERMINATE",
});
for (const [name, fn, value] of [
  [
    "unauthorized-authority",
    (g) => (g.decision = "UNAUTHORIZED"),
    "UNAUTHORIZED",
  ],
  ["unknown-authority", (g) => (g.decision = "UNKNOWN"), "UNKNOWN"],
  ["out-of-employer-scope", (g) => (g.employer_id = "other"), "UNKNOWN"],
  ["out-of-claim-scope", (g) => (g.claim_type = "other"), "UNKNOWN"],
  ["out-of-policy-scope", (g) => (g.policy = "other"), "UNKNOWN"],
  ["outside-grant-time", (g) => (g.from = now + 30), "UNKNOWN"],
])
  await alterManifest(name, (m) => fn(m.authority[0]), {
    "credential.issuerAuthority": value,
    overall: value === "UNAUTHORIZED" ? "REJECTED" : "INDETERMINATE",
  });
await alterManifest("missing-authority", (m) => (m.authority = []), {
  "credential.issuerAuthority": "UNKNOWN",
  overall: "INDETERMINATE",
});
await alterManifest(
  "overlapping-authority",
  (m) => m.authority.push({ ...grant }),
  { "credential.issuerAuthority": "INVALID", overall: "REJECTED" },
);
await alterManifest(
  "historical-versus-current-authority",
  (m) => {
    m.authority[0].until = now - 30;
    m.authority.push({ ...grant, from: now - 30, decision: "UNAUTHORIZED" });
  },
  { ...fresh, currentAuthority: "UNAUTHORIZED" },
);
const historicCase = cases.find(
  (c) => c.name === "historical-versus-current-authority",
);
historicCase.inputs[0].policy.authorityMode = "ISSUANCE_TIME";
const currentCase = structuredClone(historicCase.inputs[0]);
currentCase.policy.authorityMode = "CURRENT";
add("current-authority-policy-preserved", currentCase, {
  "credential.issuerAuthority": "UNAUTHORIZED",
  issuanceAuthority: "AUTHORIZED",
  currentAuthority: "UNAUTHORIZED",
  overall: "REJECTED",
});
for (const [name, field] of [
  ["missing-trust", "trustMaterial"],
  ["missing-status", "statusMaterial"],
]) {
  const i = structuredClone(canonical);
  delete i[field];
  add(
    name,
    i,
    field === "statusMaterial"
      ? {
          "credential.cryptographicIntegrity": "VALID",
          "credential.credentialStatus": "UNKNOWN",
          overall: "INDETERMINATE",
        }
      : { trustMaterial: "UNKNOWN", overall: "INDETERMINATE" },
  );
}
for (const [name, field] of [
  ["unavailable-trust", "trustMaterial"],
  ["unavailable-status", "statusMaterial"],
]) {
  const i = structuredClone(canonical);
  i[field] = { availability: "UNAVAILABLE" };
  add(name, i, { overall: "INDETERMINATE" });
}
for (const [name, field] of [
  ["tampered-trust", "trustMaterial"],
  ["tampered-status", "statusMaterial"],
]) {
  const i = structuredClone(canonical);
  i[field].jws = i[field].jws.slice(0, -5) + "AAAAA";
  add(
    name,
    i,
    field === "trustMaterial"
      ? { trustMaterial: "INVALID", overall: "REJECTED" }
      : { "credential.credentialStatus": "INVALID", overall: "REJECTED" },
  );
}
{
  const i = structuredClone(canonical);
  i.anchor.key = jwk(wrong, "root");
  add("unknown-anchor", i, { trustMaterial: "INVALID", overall: "REJECTED" });
}
{
  const i = structuredClone(canonical);
  i.policy.allowSynthetic = false;
  add("synthetic-root-not-production", { ...i }, { error: "INVALID_POLICY" });
}
await alterManifest(
  "synthetic-marker-substitution",
  (m) => (m.synthetic = false),
);
for (const age of [0, 899, 900, 901, 5000, -1])
  await alterStatus(
    "status-age-" + age,
    (s) => (s.iat = now - age),
    age >= 0 && age <= 900
      ? fresh
      : {
          "credential.credentialStatus": age < 0 ? "INVALID" : "STALE",
          overall: age < 0 ? "REJECTED" : "INDETERMINATE",
        },
  );
for (const [name, change] of [
  ["cross-issuer-status", (s) => (s.iss = "https://other.example.invalid")],
  ["cross-profile-status", (s) => (s.cvera_vct = "other")],
  [
    "cross-credential-list",
    (s) => (s.sub = "https://status.synthetic.example.invalid/lists/other"),
  ],
  ["invalid-compression", (s) => (s.status_list.lst = "AAAA")],
  ["unsupported-status-bits", (s) => (s.status_list.bits = 2)],
  [
    "prohibited-credential-enumeration",
    (s) => (s.credentialIds = ["not-published"]),
  ],
  ["malformed-status-version", (s) => (s.cvera_version = 1.5)],
  ["future-status", (s) => (s.iat = now + 1)],
  ["untrusted-policy-field", (s) => (s.maxStatusAgeSeconds = 999999)],
])
  await alterStatus(name, change);
await alterStatus(
  "expired-status",
  (s) => {
    s.exp = now;
    s.iat = now - 1;
  },
  {
    "credential.credentialStatus": "STALE",
    overall: "INDETERMINATE",
  },
);
await alterStatus(
  "signed-ttl-expires-cache",
  (s) => {
    s.iat = now - 121;
    s.ttl = 120;
  },
  { "credential.credentialStatus": "STALE", overall: "INDETERMINATE" },
);
await alterStatus(
  "ttl-cannot-override-policy",
  (s) => {
    s.iat = now - 901;
    s.ttl = 99999;
  },
  { "credential.credentialStatus": "STALE", overall: "INDETERMINATE" },
);
for (const age of [300, 301]) {
  const i = structuredClone(canonical);
  i.policy.maxStatusAgeSeconds = 300;
  i.statusMaterial.jws = await statusJWS({ ...statusClaims, iat: now - age });
  add(
    "configured-freshness-" + age,
    i,
    age === 300
      ? fresh
      : { "credential.credentialStatus": "STALE", overall: "INDETERMINATE" },
  );
}
for (const [name, patch] of [
  ["missing-status-reference", { status: undefined }],
  [
    "missing-status-entry",
    { status: { status_list: { idx: 131072, uri: listURI } } },
  ],
  [
    "wrong-status-reference",
    {
      status: {
        status_list: {
          idx: 37,
          uri: "https://status.synthetic.example.invalid/other",
        },
      },
    },
  ],
]) {
  const i = structuredClone(canonical);
  i.credential =
    (await sign(
      { ...claims, cnf: { jwk: holderJWK }, ...patch },
      issuer,
      "dc+sd-jwt",
      "credential",
    )) + "~";
  add(name, i, {
    "credential.credentialStatus":
      name === "wrong-status-reference" ? "INVALID" : "UNKNOWN",
    overall: name === "wrong-status-reference" ? "REJECTED" : "INDETERMINATE",
  });
}
{
  const i = structuredClone(canonical);
  i.credential = i.credential.slice(0, -6) + "AAAAA~";
  add("credential-reference-tamper", i, {
    "credential.cryptographicIntegrity": "INVALID",
    overall: "REJECTED",
  });
}
const revokedList = Buffer.from(list);
revokedList[37 >> 3] |= 1 << (37 % 8);
const revoked = structuredClone(canonical);
revoked.statusMaterial.jws = await statusJWS({
  ...statusClaims,
  cvera_version: 2,
  status_list: { bits: 1, lst: deflateSync(revokedList).toString("base64url") },
});
add(
  "good-to-revoked-without-reissuance",
  [canonical, revoked],
  [fresh, { "credential.credentialStatus": "REVOKED", overall: "REJECTED" }],
);
add(
  "known-status-rollback",
  [revoked, canonical],
  [
    { "credential.credentialStatus": "REVOKED" },
    { "credential.credentialStatus": "INVALID", overall: "REJECTED" },
  ],
);
const t2 = structuredClone(canonical);
t2.trustMaterial.jws = await trustJWS({ ...manifest, version: 2 });
add(
  "known-trust-rollback",
  [t2, canonical],
  [fresh, { trustMaterial: "ROLLED_BACK", overall: "REJECTED" }],
);
const floor = structuredClone(canonical);
floor.policy.minTrustVersion = 2;
add("first-contact-independent-version-floor", floor, {
  trustMaterial: "ROLLED_BACK",
  overall: "REJECTED",
});
const floorS = structuredClone(canonical);
floorS.policy.minStatusVersion = 2;
add("status-version-floor", floorS, {
  "credential.credentialStatus": "INVALID",
  overall: "REJECTED",
});
const unavailable = structuredClone(canonical);
unavailable.trustMaterial = { availability: "UNAVAILABLE" };
unavailable.statusMaterial = { availability: "UNAVAILABLE" };
add(
  "fresh-cache-with-private-services-unavailable",
  [canonical, unavailable],
  [fresh, fresh],
);
for (const age of [900, 901]) {
  const i = structuredClone(unavailable);
  i.policy.now = now + age;
  add(
    "cache-age-" + age,
    [canonical, i],
    [
      fresh,
      age === 900
        ? fresh
        : {
            trustMaterial: "STALE",
            "credential.cryptographicIntegrity": "VALID",
            overall: "INDETERMINATE",
          },
    ],
  );
}
const equalVersion = structuredClone(canonical);
equalVersion.statusMaterial.jws = await statusJWS({
  ...statusClaims,
  ttl: 899,
});
add(
  "same-version-equivocation",
  [canonical, equalVersion, unavailable],
  [fresh, { "credential.credentialStatus": "INVALID" }, fresh],
);
const poison = structuredClone(canonical);
poison.trustMaterial.jws = await sign(
  { ...manifest, version: 999 },
  wrong,
  "cvera-trust-material+jwt",
  "root",
);
add(
  "invalid-update-does-not-poison-cache",
  [canonical, poison, unavailable],
  [fresh, { trustMaterial: "INVALID" }, fresh],
);
for (const [name, header] of [
  [
    "unsupported-algorithm",
    { alg: "HS256", typ: "cvera-trust-material+jwt", kid: "root" },
  ],
  ["cross-token-type", { alg: "RS256", typ: "statuslist+jwt", kid: "root" }],
  [
    "critical-null",
    { alg: "RS256", typ: "cvera-trust-material+jwt", kid: "root", crit: null },
  ],
]) {
  const i = structuredClone(canonical);
  const p = i.trustMaterial.jws.split(".");
  p[0] = Buffer.from(JSON.stringify(header)).toString("base64url");
  i.trustMaterial.jws = p.join(".");
  add(name, i, { trustMaterial: "INVALID", overall: "REJECTED" });
}
// Reuse Stage4C holder gates with trusted Stage4D evidence, not fixture assertions.
const verifiers = [verifierA, verifierB].map((k, n) => ({
  id: "portable-verifier-" + n,
  kid: "v" + n,
  key: k.publicKey.export({ format: "pem", type: "spki" }),
  responseURI: `https://verifier-${n}.example.invalid/response`,
  allow: true,
}));
const presentations = [];
for (let n = 0; n < 2; n++) {
  const v = verifiers[n],
    request = await sign(
      {
        iss: v.id,
        client_id: v.id,
        aud: "https://self-issued.me/v2",
        iat: now,
        exp: now + 120,
        response_type: "vp_token",
        response_mode: "direct_post",
        response_uri: v.responseURI,
        nonce: "stage4d-verifier-" + n + "-00000000000",
        state: "stage4d-state-" + n + "-000000000000",
        dcql_query: dcql(),
      },
      n ? verifierB : verifierA,
      "oauth-authz-req+jwt",
      v.kid,
    );
  const c = {
    base: { credential, policy: { now, expectedSubject: claims.sub } },
    request,
    verifiers,
    allowFullDisclosure: true,
    consent: {
      granted: true,
      requestHash: hash(request),
      credentialHash: hash(credential),
      expires: now + 60,
    },
    transaction: {
      verifier: v.id,
      nonce: decodeJwt(request).nonce,
      requestHash: hash(request),
      expires: now + 120,
    },
  };
  c.presentation = await present(
    c,
    holder.privateKey,
    bindEvaluator(canonical, new State()),
  );
  presentations.push(c);
  add(
    "portable-presentation-" + n,
    canonical,
    { overall: "ACCEPTED_SYNTHETIC_PRESENTATION" },
    { mode: "presentation", presentations: [c] },
  );
}
add(
  "portable-presentation-replay",
  [canonical, canonical],
  [
    { overall: "ACCEPTED_SYNTHETIC_PRESENTATION" },
    { overall: "DENIED", transactionBinding: "REPLAYED" },
  ],
  { mode: "presentation", presentations: [presentations[0], presentations[0]] },
);
for (const [name, change] of [
  ["denied-holder-consent", (c) => (c.consent.granted = false)],
  ["unauthorized-verifier", (c) => (c.verifiers[0].allow = false)],
  ["unknown-verifier", (c) => (c.verifiers = [])],
]) {
  const c = structuredClone(presentations[0]);
  change(c);
  add(
    "portable-" + name,
    canonical,
    { overall: "DENIED" },
    { mode: "holder", presentations: [c] },
  );
  await assert.rejects(() =>
    present(
      c,
      holder.privateKey,
      async () => (await verify(canonical, new State())).credential,
    ),
  );
}
add(
  "revoked-cannot-present",
  revoked,
  { overall: "DENIED" },
  { mode: "holder", presentations: [presentations[0]] },
);
const badProof = structuredClone(presentations[0]);
badProof.presentation = credential;
add(
  "portable-missing-holder-proof",
  canonical,
  { overall: "DENIED", holderPossession: "INVALID" },
  { mode: "presentation", presentations: [badProof] },
);
// Additional adversarial boundaries, independent of expected bit calculations.
await alterManifest("null-authority-field", (m) => (m.authority = null));
await alterManifest(
  "wrong-authority-field-type",
  (m) => (m.authority[0].employer_id = 7),
);
await alterManifest(
  "unknown-status-purpose",
  (m) => (m.status.purpose = "suspension"),
);
await alterManifest(
  "noncanonical-public-key",
  (m) => (m.keys[0].jwk.n += "\n"),
);
await alterManifest(
  "retired-key-remains-verifiable",
  (m) => (m.keys[0].until = now - 30),
  fresh,
);
await alterManifest(
  "additional-public-key",
  (m) =>
    m.keys.push({ jwk: jwk(wrong, "new-key"), from: now, until: now + 5000 }),
  fresh,
);
await alterStatus(
  "trailing-compressed-data",
  (s) =>
    (s.status_list.lst = Buffer.concat([
      Buffer.from(s.status_list.lst, "base64url"),
      Buffer.from("trailing"),
    ]).toString("base64url")),
);
await alterStatus(
  "noncanonical-list-encoding",
  (s) => (s.status_list.lst += "\n"),
);
await alterStatus(
  "small-privacy-cohort",
  (s) =>
    (s.status_list.lst = deflateSync(Buffer.alloc(2)).toString("base64url")),
);
await alterStatus(
  "oversized-expanded-list",
  (s) =>
    (s.status_list.lst = deflateSync(Buffer.alloc(1048577)).toString(
      "base64url",
    )),
);
for (const [idx, rev] of [
  [0, true],
  [1, false],
  [7, true],
  [8, true],
  [15, true],
]) {
  const bytes = Buffer.alloc(16384);
  bytes[0] = 0xb9;
  bytes[1] = 0xa3;
  const i = structuredClone(canonical);
  i.credential =
    (await sign(
      {
        ...claims,
        cnf: { jwk: holderJWK },
        status: { status_list: { idx, uri: listURI } },
      },
      issuer,
      "dc+sd-jwt",
      "credential",
    )) + "~";
  i.statusMaterial.jws = await statusJWS({
    ...statusClaims,
    status_list: { bits: 1, lst: deflateSync(bytes).toString("base64url") },
  });
  add("draft21-golden-bit-" + idx, i, {
    "credential.credentialStatus": rev ? "REVOKED" : "GOOD",
    overall: rev ? "REJECTED" : "ELIGIBLE_WITHOUT_HOLDER",
  });
}
for (const status of ["malformed", [], { status_list: false }]) {
  const i = structuredClone(canonical);
  i.credential =
    (await sign(
      { ...claims, cnf: { jwk: holderJWK }, status },
      issuer,
      "dc+sd-jwt",
      "credential",
    )) + "~";
  add("malformed-reference-" + JSON.stringify(status), i, {
    "credential.credentialStatus": "INVALID",
    overall: "REJECTED",
  });
}
const timeRollback = structuredClone(t2);
timeRollback.trustMaterial.jws = await trustJWS({
  ...manifest,
  version: 3,
  iat: now - 1,
});
add(
  "signed-timestamp-rollback",
  [t2, timeRollback],
  [fresh, { trustMaterial: "ROLLED_BACK", overall: "REJECTED" }],
);
const firstContact = structuredClone(canonical);
add("first-contact-freshness-not-absolute-rollback", firstContact, fresh);
const substituted = structuredClone(presentations[0]);
substituted.base.credential = base.credential;
add(
  "presentation-credential-context-substitution",
  canonical,
  { overall: "DENIED" },
  { mode: "presentation", presentations: [substituted] },
);

// Real local publication + acquisition; no production namespace, API or credential.
const dir = mkdtempSync(join(tmpdir(), "cvera-stage4d-publish-"));
let distributionChecks = 0;
const distributionResults = [];
try {
  await publish(dir, canonical, anchor, policy);
  const received = receive(dir);
  assert.equal(
    (await verify({ ...canonical, ...received }, new State())).credential
      .credentialStatus,
    "GOOD",
  );
  distributionResults.push({
    name: "publish-and-receive-good",
    result: "PASS",
  });
  distributionChecks = distributionResults.length;
  await publish(dir, revoked, anchor, policy);
  assert.equal(
    (await verify({ ...canonical, ...receive(dir) }, new State())).credential
      .credentialStatus,
    "REVOKED",
  );
  distributionResults.push({
    name: "publish-revocation-without-reissuance",
    result: "PASS",
  });
  distributionChecks = distributionResults.length;
  await assert.rejects(() => publish(dir, canonical, anchor, policy));
  distributionResults.push({
    name: "reject-publication-rollback",
    result: "PASS",
  });
  distributionChecks = distributionResults.length;
  const requests = [];
  const server = createServer((req, res) => {
    requests.push({
      url: req.url,
      cookie: req.headers.cookie,
      authorization: req.headers.authorization,
    });
    const status = req.url === "/list";
    res.setHeader(
      "Content-Type",
      status ? "application/statuslist+jwt" : "application/jwt",
    );
    res.end(status ? revoked.statusMaterial.jws : canonical.trustMaterial.jws);
  });
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  try {
    const port = server.address().port;
    const fetched = await acquire(
      { trustURL: trustURI, statusURL: listURI },
      (url, opts) =>
        fetch(
          `http://127.0.0.1:${port}/${url === listURI ? "list" : "trust"}`,
          opts,
        ),
    );
    assert.equal(
      (await verify({ ...canonical, ...fetched }, new State())).credential
        .credentialStatus,
      "REVOKED",
    );
    assert.deepEqual(
      requests.map((r) => r.url),
      ["/trust", "/list"],
    );
    assert.ok(requests.every((r) => !r.cookie && !r.authorization));
    distributionResults.push({
      name: "bulk-http-no-index-cookie-or-auth",
      result: "PASS",
    });
    distributionChecks = distributionResults.length;
  } finally {
    await new Promise((resolve) => server.close(resolve));
  }
  const failed = await acquire(
    { trustURL: trustURI, statusURL: listURI },
    () => {
      throw new TypeError("unavailable");
    },
  );
  const cache = new State();
  await verify(canonical, cache);
  assert.equal(
    (await verify({ ...canonical, ...failed }, cache)).overall,
    "ELIGIBLE_WITHOUT_HOLDER",
  );
  distributionResults.push({
    name: "retrieval-failure-fresh-cache",
    result: "PASS",
  });
  distributionChecks = distributionResults.length;
  await publish(dir, revoked, anchor, policy);
  distributionResults.push({
    name: "idempotent-exact-publication",
    result: "PASS",
  });
  distributionChecks = distributionResults.length;
  const before = JSON.stringify(receive(dir));
  const late = now + 1000,
    renewed = {
      ...canonical,
      trustMaterial: {
        availability: "AVAILABLE",
        jws: await trustJWS({
          ...manifest,
          version: 2,
          iat: late,
          exp: late + 3600,
        }),
      },
      statusMaterial: {
        availability: "AVAILABLE",
        jws: await statusJWS({
          ...statusClaims,
          cvera_version: 3,
          iat: late,
          exp: late + 3600,
        }),
      },
    };
  await publish(dir, renewed, anchor, { ...policy, now: late });
  assert.equal(
    (
      await verify(
        { ...canonical, ...receive(dir), policy: { ...policy, now: late } },
        new State(),
      )
    ).overall,
    "ELIGIBLE_WITHOUT_HOLDER",
  );
  distributionResults.push({
    name: "renew-expired-publication",
    result: "PASS",
  });
  distributionChecks = distributionResults.length;
  const invalidURLs = await acquire(
    { trustURL: "http://127.0.0.1/forbidden", statusURL: "not a URL" },
    () => {
      throw Error("must not fetch");
    },
  );
  assert.equal(invalidURLs.trustMaterial.availability, "INVALID");
  assert.equal(invalidURLs.statusMaterial.availability, "INVALID");
  distributionResults.push({ name: "unsafe-urls-not-fetched", result: "PASS" });
  distributionChecks = distributionResults.length;
  const badMedia = await acquire(
    { trustURL: trustURI, statusURL: listURI },
    () =>
      new Response("malformed", { headers: { "Content-Type": "text/plain" } }),
  );
  assert.equal(badMedia.trustMaterial.availability, "INVALID");
  distributionResults.push({
    name: "invalid-media-type-rejected",
    result: "PASS",
  });
  distributionChecks = distributionResults.length;
  const pii = {
    ...canonical,
    trustMaterial: {
      availability: "AVAILABLE",
      jws: await trustJWS({ ...manifest, reviewer_email: "prohibited" }),
    },
  };
  await assert.rejects(
    () => publish(dir, pii, anchor, policy),
    /unsupported public fields/,
  );
  distributionResults.push({
    name: "prohibited-private-fields-rejected",
    result: "PASS",
  });
  distributionChecks = distributionResults.length;
  await assert.rejects(
    () => publish(dir, renewed, anchor, { ...policy, now: undefined }),
    /invalid publication policy/,
  );
  distributionResults.push({
    name: "missing-publication-time-rejected",
    result: "PASS",
  });
  distributionChecks = distributionResults.length;
  const renewNext = {
    ...renewed,
    statusMaterial: {
      availability: "AVAILABLE",
      jws: await statusJWS({
        ...statusClaims,
        cvera_version: 4,
        iat: late,
        exp: late + 3600,
      }),
    },
  };
  const originalFsync = fs.fsyncSync;
  try {
    fs.fsyncSync = (fd) => {
      if (fs.fstatSync(fd).isDirectory())
        throw Error("synthetic directory sync failure");
      return originalFsync(fd);
    };
    await assert.rejects(
      () => publish(dir, renewNext, anchor, { ...policy, now: late }),
      (e) => e.commitMayHaveOccurred === true,
    );
  } finally {
    fs.fsyncSync = originalFsync;
  }
  assert.equal(receive(dir).statusMaterial.jws, renewNext.statusMaterial.jws);
  distributionResults.push({
    name: "ambiguous-commit-read-only-reconciliation",
    result: "PASS",
  });
  distributionChecks = distributionResults.length;
  const pointerFile = join(dir, "latest.json"),
    pointerBody = readFileSync(pointerFile, "utf8");
  writeFileSync(
    pointerFile,
    JSON.stringify({ trustFile: "../escape.jwt", statusFile: "../escape.jwt" }),
  );
  assert.equal(receive(dir).trustMaterial.availability, "INVALID");
  writeFileSync(pointerFile, pointerBody);
  distributionResults.push({
    name: "pointer-traversal-rejected",
    result: "PASS",
  });
  distributionChecks = distributionResults.length;
  const statusPath = join(dir, JSON.parse(pointerBody).statusFile),
    savedStatus = readFileSync(statusPath);
  fs.unlinkSync(statusPath);
  assert.equal(receive(dir).trustMaterial.availability, "INVALID");
  await assert.rejects(
    () => publish(dir, renewNext, anchor, { ...policy, now: late }),
    /existing publication corrupt/,
  );
  writeFileSync(statusPath, savedStatus);
  distributionResults.push({
    name: "missing-referenced-file-blocks-history-reset",
    result: "PASS",
  });
  distributionChecks = distributionResults.length;
} finally {
  rmSync(dir, { recursive: true, force: true });
}
assert.equal(issuances, 1);
assert.equal(canonical.credential, revoked.credential);
writeFileSync(
  process.argv[2],
  JSON.stringify(
    {
      cases,
      issuanceCalls: issuances,
      distributionChecks,
      distributionResults,
      unchangedCredential: true,
    },
    null,
    2,
  ),
);
