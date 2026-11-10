import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { test, mock } from "node:test";
import { inspectDocument } from "../apps/evt-ui-expo/src/api/reviewDocument.web";
import { EvidenceReviewClient, ReviewError, ReviewSubmissionGate, validRationale, type Binding, type ReviewItem } from "../apps/evt-ui-expo/src/api/evidenceReview";

const base = "https://api.example.test", token = "PRIVATE_SESSION";
const reviewer = "22222222-2222-4222-8222-222222222222";
const scope = { caseId: "case-1", checkId: "employment.role" };
const bytes = new Uint8Array([137, 80, 78, 71]);
const sha = async (data: Uint8Array) => createHash("sha256").update(data).digest("hex");
const item: ReviewItem = {
  uploadId: "11111111-1111-4111-8111-111111111111", name: "authentic.png", mimeType: "image/png", size: bytes.length,
  evidence_hash: createHash("sha256").update(bytes).digest("hex"), status: "RECEIVED", scanStatus: "PENDING", reviewStatus: "PENDING",
  reviewerId: null, reviewedAt: null, reviewReason: null, canReview: true, canApprove: true,
};
const binding: Binding = {
  bindingId: "33333333-3333-4333-8333-333333333333", requestId: "44444444-4444-4444-8444-444444444444", subjectId: "55555555-5555-4555-8555-555555555555",
  employerId: "employer", issuerId: "https://cvera.app", claimType: "employment.role", claimDigest: "a".repeat(64), contextValid: true, requiredCheck: true,
  claimSnapshot: { primary_employment: { title: "Managing Member — owner", issuer_name: "Employer" } },
};
const receipt = { reviewId: "66666666-6666-4666-8666-666666666666", uploadId: item.uploadId, reviewerId: reviewer, decision: "APPROVED", reason: "Inspected original and claim context.", reviewedAt: "2026-11-10T21:00:00Z" };
const json = (value: unknown, status = 200) => new Response(JSON.stringify(value), { status, headers: { "Content-Type": "application/json" } });
const client = (fn: (url: string, init?: RequestInit) => Response | Promise<Response>) => new EvidenceReviewClient(base, token, ((url, init) => fn(String(url), init)) as typeof fetch);

test("discovery exposes only explicit review assignments, including a safe empty state", async () => {
  for (const rows of [[], [{ ...scope, canUpload: true, canReview: false }], [{ ...scope, canUpload: false, canReview: true }]]) {
    const c = client((url, init) => {
      assert.equal(url, base + "/v1/evidence/checks");
      assert.equal((init!.headers as Record<string, string>).Authorization, "Bearer " + token);
      assert.equal(init!.credentials, "omit"); assert.equal(init!.redirect, "error"); assert.equal(init!.cache, "no-store");
      return json({ items: rows });
    });
    assert.deepEqual(await c.assignments(), rows.filter(r => r.canReview));
  }
});

test("unauthorized, expired and revoked discovery/list/download return sanitized errors", async () => {
  for (const status of [401, 403, 404]) {
    const c = client(() => json({ error: "PRIVATE_RESPONSE " + token }, status));
    for (const action of [() => c.assignments(), () => c.listing(scope), () => c.download(scope, item, sha)]) {
      await assert.rejects(action, (e: unknown) => e instanceof ReviewError && e.status === status && !e.message.includes(token) && !e.message.includes("PRIVATE_RESPONSE"));
    }
  }
});

test("listing preserves exact immutable context, manual/scan distinction, existing review and missing attachments", async () => {
  const recorded = { ...item, reviewStatus: "APPROVED", reviewerId: reviewer, reviewedAt: receipt.reviewedAt, reviewReason: receipt.reason, canReview: false, canApprove: false };
  for (const evidence of [[], [item], [recorded]]) {
    const actual = await client(() => json({ ...scope, binding, evidence })).listing(scope);
    assert.deepEqual(actual.binding, binding); assert.deepEqual(actual.evidence, evidence);
    if (actual.evidence.length) assert.equal(actual.evidence[0].scanStatus, "PENDING");
  }
  const stale = { ...binding, contextValid: false, claimSnapshot: null };
  assert.equal((await client(() => json({ ...scope, binding: stale, evidence: [] })).listing(scope)).binding!.contextValid, false);
});

test("cross-case, malformed hashes, malformed bindings and invented receipts fail closed", async () => {
  for (const data of [
    { ...scope, caseId: "other", binding, evidence: [] },
    { ...scope, binding, evidence: [{ ...item, evidence_hash: "invalid" }] },
    { ...scope, binding: { ...binding, bindingId: "invalid" }, evidence: [] },
    { ...scope, binding, evidence: [{ ...item, reviewStatus: "APPROVED" }] },
  ]) await assert.rejects(() => client(() => json(data)).listing(scope), ReviewError);
  const c = client(() => { throw new Error("must not call transport"); });
  await assert.rejects(() => c.listing({ ...scope, caseId: ".." }), ReviewError);
});

test("canonical retrieval uses Bearer only, verifies exact bytes, hash, size and private response", async () => {
  const c = client((url, init) => {
    assert.equal(url, base + `/v1/cases/${scope.caseId}/checks/${scope.checkId}/evidence/${item.uploadId}/download`);
    assert.ok(!url.includes(token)); assert.equal(init!.method, "GET");
    return new Response(bytes, { headers: { "Content-Type": "application/octet-stream", "Cache-Control": "private, no-store" } });
  });
  assert.deepEqual(await c.download(scope, item, sha), bytes);
  for (const changed of [{ ...item, size: 1 }, { ...item, evidence_hash: "b".repeat(64) }, { ...item, scanStatus: "INFECTED" }]) {
    await assert.rejects(() => c.download(scope, changed, sha), ReviewError);
  }
  await assert.rejects(() => client(() => new Response(bytes)).download(scope, item, sha), ReviewError);
  await assert.rejects(() => client(() => json({}, 404)).download(scope, item, sha), ReviewError);
});

test("both decisions submit the exact server binding/digest and validate actual attributed receipts", async () => {
  for (const decision of ["APPROVED", "REJECTED"] as const) {
    const actual = { ...receipt, decision };
    const c = client((url, init) => {
      assert.ok(url.endsWith("/review"));
      assert.deepEqual(JSON.parse(init!.body as string), { decision, reason: receipt.reason, bindingId: binding.bindingId, claimDigest: binding.claimDigest });
      return json(actual);
    });
    assert.deepEqual(await c.submit(scope, item, binding, decision, " " + receipt.reason + " ", reviewer), actual);
  }
});

test("blank/oversize reasons, unselected decision, self-review permissions and stale context send no decision", async () => {
  let calls = 0;
  const c = client(() => { calls++; return json(receipt); });
  for (const reason of ["", "  ", "é".repeat(501)]) await assert.rejects(() => c.submit(scope, item, binding, "APPROVED", reason, reviewer), ReviewError);
  for (const changed of [{ ...item, canReview: false }, { ...item, canApprove: false }, { ...item, reviewStatus: "APPROVED" as const }]) {
    await assert.rejects(() => c.submit(scope, changed, binding, "APPROVED", receipt.reason, reviewer), ReviewError);
  }
  for (const changed of [{ ...binding, contextValid: false }, { ...binding, requiredCheck: false }]) {
    await assert.rejects(() => c.submit(scope, item, changed, "REJECTED", receipt.reason, reviewer), ReviewError);
  }
  await assert.rejects(() => c.submit(scope, item, binding, null as never, receipt.reason, reviewer), ReviewError);
  assert.equal(calls, 0);
  assert.equal(validRationale("é".repeat(500)), true);
  assert.equal(validRationale("🙂".repeat(250)), true);
  assert.equal(validRationale("🙂".repeat(251)), false);
});

test("wrong reviewer, wrong upload, malformed or missing receipt is uncertain, never success", async () => {
  for (const data of [{}, { ...receipt, reviewerId: binding.subjectId }, { ...receipt, uploadId: binding.requestId }, { ...receipt, decision: "REJECTED" }, { ...receipt, reviewedAt: "bad" }]) {
    await assert.rejects(() => client(() => json(data)).submit(scope, item, binding, "APPROVED", receipt.reason, reviewer), (e: unknown) => e instanceof ReviewError && e.uncertain);
  }
  await assert.rejects(() => client(() => new Response("not-json")).submit(scope, item, binding, "APPROVED", receipt.reason, reviewer), (e: unknown) => e instanceof ReviewError && e.uncertain);
});

test("network/5xx mutation ambiguity remains blocked, while explicit denials never imply approval", async () => {
  for (const response of [() => { throw new Error("PRIVATE_TOKEN " + token); }, () => json({}, 503)]) {
    const c = client(response), gate = new ReviewSubmissionGate();
    await assert.rejects(() => gate.run(item.uploadId, () => c.submit(scope, item, binding, "APPROVED", receipt.reason, reviewer)), (e: unknown) => e instanceof ReviewError && e.uncertain && !e.message.includes(token));
    assert.equal(gate.has(item.uploadId), true);
    await assert.rejects(() => gate.run(item.uploadId, async () => receipt), ReviewError);
  }
  for (const status of [400, 401, 403, 404, 409]) {
    await assert.rejects(() => client(() => json({}, status)).submit(scope, item, binding, "REJECTED", receipt.reason, reviewer), (e: unknown) => e instanceof ReviewError && e.status === status && !e.uncertain);
  }
});

test("double taps acquire synchronously; success cannot be submitted a second time", async () => {
  const gate = new ReviewSubmissionGate();
  let release!: (value: string) => void;
  const pending = gate.run("upload", () => new Promise<string>(resolve => { release = resolve; }));
  await assert.rejects(() => gate.run("upload", async () => "duplicate"), ReviewError);
  release("committed"); assert.equal(await pending, "committed");
  await assert.rejects(() => gate.run("upload", async () => "duplicate"), ReviewError);
});

test("web handoff uses temporary local bytes, a safe attachment name, and revokes its object URL", async () => {
  const original = Object.getOwnPropertyDescriptor(globalThis, "document");
  let blob: Blob | undefined, clicked = false, removed = false, revoked = false;
  let cleanup: (() => void) | undefined;
  const link = { href: "", download: "", rel: "", click() { clicked = true; }, remove() { removed = true; } };
  Object.defineProperty(globalThis, "document", { configurable: true, value: { createElement: () => link, body: { appendChild: () => {} } } });
  const create = mock.method(URL, "createObjectURL", value => { blob = value as Blob; return "blob:local-document"; });
  const revoke = mock.method(URL, "revokeObjectURL", url => { assert.equal(url, "blob:local-document"); revoked = true; });
  const timer = mock.method(globalThis, "setTimeout", ((callback: () => void) => { cleanup = callback; return 1; }) as typeof setTimeout);
  try {
    await inspectDocument(bytes, "private/path.png", "image/png");
    assert.equal(clicked, true); assert.equal(removed, true);
    assert.equal(link.href, "blob:local-document"); assert.equal(link.download, "private_path.png");
    assert.ok(!link.href.includes(token)); assert.equal(blob!.type, "image/png");
    assert.deepEqual(new Uint8Array(await blob!.arrayBuffer()), bytes);
    cleanup!(); assert.equal(revoked, true);
    await inspectDocument(bytes, "unknown", "text/html");
    assert.equal(blob!.type, "application/octet-stream");
    link.click = () => { throw new Error("download blocked"); };
    await assert.rejects(() => inspectDocument(bytes, "file", "application/pdf"));
  } finally {
    create.mock.restore(); revoke.mock.restore(); timer.mock.restore();
    if (original) Object.defineProperty(globalThis, "document", original); else Reflect.deleteProperty(globalThis, "document");
  }
});
