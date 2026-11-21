import assert from "node:assert/strict";
import { test } from "node:test";
import { fetchRequestorReviewOutcomes, type RequestorReviewOutcomes } from "../apps/evt-ui-expo/src/api/requestorReviewOutcomes";

const id = "11111111-1111-4111-8111-111111111111";
const result: RequestorReviewOutcomes = {
  requestId: id, requestStatus: "ATTESTATION_PENDING", requestVersion: 3, state: "CURRENT", reviewContext: "SIMULATED_UAT",
  outcomes: [{ checkId: "employment.company_and_dates", reviewId: "22222222-2222-4222-8222-222222222222", reviewBasis: "CLAIM_ONLY", decision: "APPROVED", reason: "Claim is correct.", reviewedAt: "2026-11-18T01:28:01Z" }],
};
const response = (data: unknown, status = 200) => (async () => new Response(JSON.stringify(data), { status })) as typeof fetch;

test("requestor outcomes use the owned-request GET contract without cache or mutation", async () => {
  const before = JSON.stringify(result);
  const out = await fetchRequestorReviewOutcomes("https://api.example.test/", "private-token", id, (async (url, options) => {
    assert.equal(url, `https://api.example.test/v1/requests/${id}/review-outcomes`);
    assert.equal(options?.method, "GET"); assert.equal(options?.cache, "no-store"); assert.equal(options?.body, undefined);
    assert.deepEqual(options?.headers, { Accept: "application/json", Authorization: "Bearer private-token" });
    return new Response(JSON.stringify(result));
  }) as typeof fetch);
  assert.equal(out.outcomes[0].decision, "APPROVED"); assert.equal(out.requestStatus, "ATTESTATION_PENDING");
  assert.equal(JSON.stringify(result), before);
});

test("pending, empty, rejected and authoritative evidence outcomes share the contract", async () => {
  for (const data of [
    { ...result, state: "NOT_BOUND", reviewContext: null, outcomes: [] },
    { ...result, outcomes: [{ ...result.outcomes[0], decision: "PENDING", reviewId: null, reviewBasis: null, reason: null, reviewedAt: null }] },
    { ...result, outcomes: [{ ...result.outcomes[0], decision: "REJECTED" }] },
    { ...result, reviewContext: "AUTHORITATIVE", outcomes: [{ ...result.outcomes[0], reviewBasis: "UPLOADED_EVIDENCE" }] },
  ]) assert.deepEqual(await fetchRequestorReviewOutcomes("https://api.example.test", "token", id, response(data)), data);
});

test("wrong request, contradictory contexts and malformed decisions fail closed", async () => {
  for (const data of [
    { ...result, requestId: "another-request" }, { ...result, requestVersion: 0 },
    { ...result, reviewContext: null }, { ...result, state: "STALE" },
    { ...result, reviewContext: "AUTHORITATIVE" },
    { ...result, outcomes: [{ ...result.outcomes[0], decision: "VERIFIED" }] },
    { ...result, outcomes: [{ ...result.outcomes[0], reviewedAt: "invalid" }] },
    { ...result, outcomes: [{ ...result.outcomes[0], decision: "PENDING" }] },
  ]) await assert.rejects(fetchRequestorReviewOutcomes("https://api.example.test", "token", id, response(data)));
  for (const status of [401, 403, 404, 503]) await assert.rejects(
    fetchRequestorReviewOutcomes("https://api.example.test", "token", id, response({ error: "private database detail" }, status)),
    (e: Error) => !e.message.includes("private database detail"),
  );
});
