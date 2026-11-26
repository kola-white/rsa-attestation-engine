// Runtime-only synthetic keys and assertions: no private key or signed fixture
// is written to the repository or copied into the isolated verifier environment.
import { generateKeyPairSync, createHash } from "node:crypto";
import { readFileSync, writeFileSync } from "node:fs";
import { decodeJwt, SignJWT } from "jose";
import {
  hash,
  dcql,
  present,
  issueBound,
  Store,
  authorize,
} from "./verify.mjs";
import assert from "node:assert/strict";
const key = () => generateKeyPairSync("rsa", { modulusLength: 3072 });
const issuer = key(),
  holder = key(),
  a = key(),
  b = key(),
  other = key();
const pem = (k) => k.publicKey.export({ type: "spki", format: "pem" });
const jwk = holder.publicKey.export({ format: "jwk" });
const sign = (m, k, typ, kid) =>
  new SignJWT(m)
    .setProtectedHeader({
      alg: "RS256",
      typ,
      ...(typ === "openid4vci-proof+jwt"
        ? { jwk: k.publicKey.export({ format: "jwk" }) }
        : kid
          ? { kid }
          : {}),
    })
    .sign(k.privateKey);
const template = JSON.parse(
  readFileSync(new URL("../stage4b/fixtures/vectors.json", import.meta.url)),
).find((v) => v.name === "fresh").input;
const now = template.policy.now;
const claims = decodeJwt(template.credential.slice(0, -1));
claims.credential_spki_sha256 = createHash("sha256")
  .update(issuer.publicKey.export({ type: "spki", format: "der" }))
  .digest("hex");
const context = {
  issuer: claims.iss,
  nonce: "issuer-challenge-0000000000000001",
  now,
  expires: now + 120,
};
const proof = await sign(
  { aud: context.issuer, nonce: context.nonce, iat: now },
  holder,
  "openid4vci-proof+jwt",
);
let issuanceCalls = 0;
const store = new Store();
const credential = await issueBound(
  claims,
  jwk,
  proof,
  context,
  store,
  async (m) => {
    issuanceCalls++;
    return (await sign(m, issuer, "dc+sd-jwt", template.trust.kid)) + "~";
  },
);
let deniedSignerCalls = 0;
const deniedSigner = () => {
  deniedSignerCalls++;
  return "unexpected";
};
let issuanceNegativeChecks = 1;
await assert.rejects(() =>
  issueBound(claims, jwk, proof, context, store, deniedSigner),
);
for (const [name, p] of [
  ["missing", ""],
  [
    "wrong-key",
    await sign(
      { aud: context.issuer, nonce: context.nonce, iat: now },
      other,
      "openid4vci-proof+jwt",
    ),
  ],
  [
    "expired",
    await sign(
      { aud: context.issuer, nonce: context.nonce, iat: now - 301 },
      holder,
      "openid4vci-proof+jwt",
    ),
  ],
]) {
  await assert.rejects(() =>
    issueBound(claims, jwk, p, context, new Store(), deniedSigner),
  );
  issuanceNegativeChecks++;
}
assert.equal(deniedSignerCalls, 0);
const digest = createHash("sha256").update(credential).digest("hex");
const base = structuredClone(template);
base.credential = credential;
base.trust.publicKey = pem(issuer);
base.trust.authorityKey = pem(issuer);
base.trust.statusKey = pem(issuer);
base.authorityJws = await sign(
  {
    iss: base.trust.authorityIssuer,
    iat: now,
    exp: now + 3600,
    synthetic: true,
    credential_sha256: digest,
    issuer: claims.iss,
    employer_id: claims.evidence_binding.employer_id,
    claim_type: claims.claim_type,
    decision: "AUTHORIZED",
  },
  issuer,
  "cvera-stage4b-fixture-authority+jwt",
  base.trust.authorityKid,
);
const status = async (age = 0, state = "GOOD", extra = {}) =>
  sign(
    {
      iss: base.trust.statusIssuer,
      iat: now - age,
      synthetic: true,
      credential_sha256: digest,
      entries: [{ credential_id: claims.jti, state }],
      ...extra,
    },
    issuer,
    "cvera-stage4b-fixture-status+jwt",
    base.trust.statusKid,
  );
base.status = { availability: "AVAILABLE", jws: await status() };
const verifiers = [
  {
    id: "synthetic-verifier-a",
    kid: "a",
    key: pem(a),
    responseURI: "https://a.example.invalid/response",
    allow: true,
  },
  {
    id: "synthetic-verifier-b",
    kid: "b",
    key: pem(b),
    responseURI: "https://b.example.invalid/response",
    allow: true,
  },
];
const requestClaims = (v, n) => ({
  iss: v.id,
  client_id: v.id,
  aud: "https://self-issued.me/v2",
  iat: now,
  exp: now + 120,
  response_type: "vp_token",
  response_mode: "direct_post",
  response_uri: v.responseURI,
  nonce: "challenge-" + n + "-0000000000000000",
  state: "state-" + n + "-000000000000000000",
  dcql_query: dcql(),
});
async function make(v, k, n) {
  const m = requestClaims(v, n),
    request = await sign(m, k, "oauth-authz-req+jwt", v.kid);
  const i = {
    base: structuredClone(base),
    request,
    verifiers: structuredClone(verifiers),
    allowFullDisclosure: true,
    consent: {
      requestHash: hash(request),
      credentialHash: hash(credential),
      expires: now + 60,
      granted: true,
    },
    transaction: {
      verifier: v.id,
      nonce: m.nonce,
      requestHash: hash(request),
      expires: now + 120,
    },
  };
  const check = await authorize(i);
  if (check.overall !== "AUTHORIZED_TO_PRESENT")
    throw Error(JSON.stringify(check));
  i.presentation = await present(i, holder.privateKey);
  return i;
}
const ia = await make(verifiers[0], a, "a"),
  ib = await make(verifiers[1], b, "b");
let holderSigningDenialChecks = 0;
const cases = [];
function add(
  name,
  i,
  expected,
  holderSide = false,
  inputs = null,
  concurrent = false,
) {
  cases.push({
    name,
    holder: holderSide,
    inputs: inputs ?? [i],
    concurrent,
    expected: structuredClone(expected),
  });
}
const accept = {
  overall: "ACCEPTED_SYNTHETIC_PRESENTATION",
  holderPossession: "VALID",
  transactionBinding: "VALID",
};
const deny = { overall: "DENIED" };
add("verifier-a", ia, accept);
add("verifier-b", ib, accept);
function mutate(name, fn, expected = deny, side = false) {
  const i = structuredClone(ia);
  fn(i);
  add(name, i, expected, side);
}
for (const [name, fn] of [
  ["missing-proof", (i) => (i.presentation = credential)],
  ["copied-credential", (i) => (i.presentation = credential)],
  [
    "tampered-presentation",
    (i) => (i.presentation = i.presentation.slice(0, -3) + "AAA"),
  ],
  [
    "cross-verifier",
    (i) => {
      i.request = ib.request;
      i.transaction = ib.transaction;
    },
  ],
  ["missing-transaction", (i) => (i.transaction = {})],
  ["transaction-expired", (i) => (i.transaction.expires = now)],
  ["transaction-substitution", (i) => (i.transaction.requestHash = "x")],
  ["unknown-verifier", (i) => (i.verifiers = [])],
  ["verifier-key-mismatch", (i) => (i.verifiers[0].key = pem(other))],
  ["unauthorized-verifier", (i) => (i.verifiers[0].allow = false)],
  ["denied-disclosure-policy", (i) => (i.allowFullDisclosure = false)],
  ["missing-authority", (i) => (i.base.authorityJws = "")],
  ["missing-status", (i) => delete i.base.status],
  [
    "unavailable-status",
    (i) => (i.base.status = { availability: "UNAVAILABLE" }),
  ],
  [
    "credential-tamper",
    (i) => (i.base.credential = i.base.credential.slice(0, -5) + "AAAA~"),
  ],
])
  mutate(name, fn);
for (const age of [899, 900, 901, 5000, -1]) {
  const i = structuredClone(ia);
  i.base.status.jws = await status(age);
  add("status-age-" + age, i, age >= 0 && age <= 900 ? accept : deny);
}
for (const [name, jws] of [
  ["revoked", await status(0, "REVOKED")],
  ["missing-entry", await status(0, "GOOD", { entries: [] })],
  [
    "wrong-status-binding",
    await status(0, "GOOD", { credential_sha256: "0".repeat(64) }),
  ],
  ["invalid-status-signature", base.status.jws.slice(0, -4) + "AAAA"],
]) {
  const i = structuredClone(ia);
  i.base.status.jws = jws;
  add(name, i, deny);
}
for (const [name, fn] of [
  ["missing-consent", (i) => delete i.consent],
  ["denied-consent", (i) => (i.consent.granted = false)],
  ["expired-consent", (i) => (i.consent.expires = now)],
  ["wrong-consent-request", (i) => (i.consent.requestHash = "x")],
  ["wrong-consent-credential", (i) => (i.consent.credentialHash = "x")],
  ["policy-denied-holder", (i) => (i.allowFullDisclosure = false)],
  ["unknown-holder-verifier", (i) => (i.verifiers = [])],
]) {
  const i = structuredClone(ia);
  fn(i);
  add(name, i, deny, true);
  await assert.rejects(() => present(i, holder.privateKey));
  holderSigningDenialChecks++;
}
const unbound = structuredClone(ia);
unbound.base = structuredClone(template);
unbound.presentation = template.credential;
add("legacy-unbound-no-assurance", unbound, deny);
add(
  "holder-authorized",
  ia,
  { overall: "AUTHORIZED_TO_PRESENT", holderAuthorization: "AUTHORIZED" },
  true,
);
await assert.rejects(() => present(ia, other.privateKey));
holderSigningDenialChecks++;
for (const [name, patch] of [
  ["missing-audience", { aud: undefined }],
  ["wrong-audience", { aud: verifiers[1].id }],
  ["missing-nonce", { nonce: undefined }],
  ["wrong-nonce", { nonce: "other" }],
  ["future-proof", { iat: now + 1 }],
  ["expired-proof", { iat: now - 301 }],
  ["wrong-sd-hash", { sd_hash: "bad" }],
  ["kb-expired-exp", { exp: now }],
  ["kb-future-nbf", { nbf: now + 1 }],
  ["kb-malformed-exp", { exp: "later" }],
]) {
  const i = structuredClone(ia);
  const kb = {
    aud: verifiers[0].id,
    nonce: i.transaction.nonce,
    iat: now,
    sd_hash: hash(credential),
    ...patch,
  };
  i.presentation = credential + (await sign(kb, holder, "kb+jwt"));
  add(name, i, deny);
}
{
  const i = structuredClone(ia);
  i.presentation =
    credential +
    (await sign(
      {
        aud: verifiers[0].id,
        nonce: i.transaction.nonce,
        iat: now,
        sd_hash: hash(credential),
      },
      other,
      "kb+jwt",
    ));
  add("incorrect-holder-key", i, deny);
}
for (const [name, patch] of [
  ["expired-request", { iat: now - 300, exp: now }],
  ["future-request", { iat: now + 1, exp: now + 120 }],
  [
    "excessive-disclosure",
    { dcql_query: { credentials: [{ id: "all", format: "dc+sd-jwt" }] } },
  ],
  ["untrusted-metadata", { client_metadata: { jwks: {} } }],
  [
    "wrong-response-uri",
    { response_uri: "https://evil.example.invalid/steal" },
  ],
]) {
  const i = structuredClone(ia);
  i.request = await sign(
    { ...requestClaims(verifiers[0], "a"), ...patch },
    a,
    "oauth-authz-req+jwt",
    "a",
  );
  i.transaction.requestHash = hash(i.request);
  i.consent.requestHash = hash(i.request);
  add(name, i, deny);
  await assert.rejects(() => present(i, holder.privateKey));
  holderSigningDenialChecks++;
}
for (const [name, header] of [
  ["unsigned-request", { alg: "none", typ: "oauth-authz-req+jwt" }],
  [
    "critical-null",
    { alg: "RS256", typ: "oauth-authz-req+jwt", kid: "a", crit: null },
  ],
  [
    "untrusted-header-key",
    {
      alg: "RS256",
      typ: "oauth-authz-req+jwt",
      kid: "a",
      jwk: other.publicKey.export({ format: "jwk" }),
    },
  ],
]) {
  const i = structuredClone(ia);
  const parts = i.request.split(".");
  parts[0] = Buffer.from(JSON.stringify(header)).toString("base64url");
  i.request = parts.join(".");
  add(name, i, deny);
}
add(
  "replay-sequential",
  ia,
  [accept, { ...deny, transactionBinding: "REPLAYED" }],
  false,
  [ia, ia],
);
add(
  "concurrent-duplicate",
  ia,
  [accept, { ...deny, transactionBinding: "REPLAYED" }],
  false,
  [ia, ia],
  true,
);
const invalid = structuredClone(ia);
invalid.presentation = credential;
add("invalid-does-not-consume", ia, [deny, accept], false, [invalid, ia]);
const custom = structuredClone(ia);
custom.base.policy.maxStatusAgeSeconds = 10;
custom.base.status.jws = await status(11);
add("trusted-custom-status-policy", custom, deny);
const override = structuredClone(ia);
override.base.status.jws = await status(901, "GOOD", {
  maxStatusAgeSeconds: 999999,
});
add("untrusted-status-policy-override", override, deny);
const leadingRoot = structuredClone(ia);
leadingRoot.verifiers[0].key =
  "unexpected leading bytes\n" + leadingRoot.verifiers[0].key;
add("leading-verifier-root", leadingRoot, {
  overall: "DENIED",
  verifierAuthentication: "INVALID",
});
const malformedRoot = structuredClone(ia);
malformedRoot.verifiers[0].key += "unexpected trailing bytes";
add("malformed-verifier-root", malformedRoot, {
  overall: "DENIED",
  verifierAuthentication: "INVALID",
});
const missingRegistry = structuredClone(ia);
delete missingRegistry.verifiers;
add("missing-verifier-registry", missingRegistry, {
  overall: "DENIED",
  verifierAuthentication: "UNKNOWN",
});
const crossTransaction = structuredClone(ia);
crossTransaction.request = await sign(
  requestClaims(verifiers[0], "new-transaction"),
  a,
  "oauth-authz-req+jwt",
  "a",
);
crossTransaction.transaction = {
  verifier: verifiers[0].id,
  nonce: requestClaims(verifiers[0], "new-transaction").nonce,
  requestHash: hash(crossTransaction.request),
  expires: now + 120,
};
add("cross-transaction-replay", crossTransaction, deny);
// Holder disclosure gates are evaluated independently of receiver-side results.
for (const name of [
  "unknown-verifier",
  "verifier-key-mismatch",
  "unauthorized-verifier",
  "denied-disclosure-policy",
  "missing-authority",
  "missing-status",
  "unavailable-status",
  "credential-tamper",
]) {
  const i = structuredClone(cases.find((c) => c.name === name).inputs[0]);
  add("holder-gate-" + name, i, deny, true);
  await assert.rejects(() => present(i, holder.privateKey));
  holderSigningDenialChecks++;
}
for (const [name, change] of [
  ["invalid-trusted-policy", (i) => (i.base.policy.maxStatusAgeSeconds = -1)],
  ["non-synthetic-trust", (i) => (i.base.trust.synthetic = false)],
]) {
  const i = structuredClone(ia);
  change(i);
  add(name, i, deny);
  add("holder-" + name, i, deny, true);
  await assert.rejects(() => present(i, holder.privateKey));
  holderSigningDenialChecks++;
}
// Signer failure yields no credential and does not make the consumed proof reusable.
const failedStore = new Store();
let failedCalls = 0;
await assert.rejects(() =>
  issueBound(claims, jwk, proof, context, failedStore, () => {
    failedCalls++;
    throw Error("synthetic signer unavailable");
  }),
);
await assert.rejects(() =>
  issueBound(claims, jwk, proof, context, failedStore, deniedSigner),
);
assert.equal(failedCalls, 1);
assert.equal(deniedSignerCalls, 0);
issuanceNegativeChecks += 2;
assert.equal(issuanceCalls, 1);
// Assert actual enforcement dimensions, not merely a generic denial.
const cryptoDenials = [
  "missing-proof",
  "copied-credential",
  "tampered-presentation",
  "cross-verifier",
  "cross-transaction-replay",
  "missing-audience",
  "wrong-audience",
  "missing-nonce",
  "wrong-nonce",
  "future-proof",
  "expired-proof",
  "wrong-sd-hash",
  "incorrect-holder-key",
  "legacy-unbound-no-assurance",
  "kb-expired-exp",
  "kb-future-nbf",
  "kb-malformed-exp",
];
for (const c of cases) {
  if (cryptoDenials.includes(c.name)) c.expected.holderPossession = "INVALID";
  if (
    [
      "missing-transaction",
      "transaction-expired",
      "transaction-substitution",
    ].includes(c.name)
  ) {
    c.expected.holderPossession = "VALID";
    c.expected.transactionBinding = "INVALID";
  }
  if (
    [
      "unknown-verifier",
      "holder-gate-unknown-verifier",
      "unknown-holder-verifier",
    ].includes(c.name)
  )
    c.expected.verifierAuthentication = "UNKNOWN";
  if (
    [
      "verifier-key-mismatch",
      "expired-request",
      "future-request",
      "wrong-response-uri",
      "untrusted-metadata",
      "unsigned-request",
      "critical-null",
      "untrusted-header-key",
    ].includes(c.name)
  )
    c.expected.verifierAuthentication = "INVALID";
  if (
    [
      "unauthorized-verifier",
      "denied-disclosure-policy",
      "excessive-disclosure",
      "holder-gate-unauthorized-verifier",
      "holder-gate-denied-disclosure-policy",
      "policy-denied-holder",
    ].includes(c.name)
  ) {
    c.expected.verifierAuthentication = "AUTHENTICATED";
    c.expected.disclosureAuthorization = "DENIED";
  }
  if (c.name === "missing-consent") c.expected.holderAuthorization = "UNKNOWN";
  if (
    [
      "denied-consent",
      "expired-consent",
      "wrong-consent-request",
      "wrong-consent-credential",
    ].includes(c.name)
  )
    c.expected.holderAuthorization = "DENIED";
  const states = {
    "status-age-899": "GOOD",
    "status-age-900": "GOOD",
    "status-age-901": "STALE",
    "status-age-5000": "STALE",
    "status-age--1": "INVALID",
    revoked: "REVOKED",
    "missing-entry": "UNKNOWN",
    "wrong-status-binding": "INVALID",
    "invalid-status-signature": "INVALID",
    "trusted-custom-status-policy": "STALE",
    "untrusted-status-policy-override": "STALE",
    "missing-status": "UNKNOWN",
    "unavailable-status": "UNKNOWN",
  };
  if (states[c.name])
    c.expected["credential.credentialStatus"] = states[c.name];
}
writeFileSync(
  process.argv[2],
  JSON.stringify(
    {
      issuanceCalls,
      credentialHash: hash(credential),
      cases,
      issuanceNegativeChecks,
      holderSigningDenialChecks,
    },
    null,
    2,
  ),
);
