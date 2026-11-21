import assert from "node:assert/strict";
import { test } from "node:test";
import { createRequire } from "node:module";
import { resolve } from "node:path";
import { readFileSync } from "node:fs";
import ts from "typescript";
import type { RequestorReviewOutcomes } from "../apps/evt-ui-expo/src/api/requestorReviewOutcomes";

const id = "11111111-1111-4111-8111-111111111111";
const result: RequestorReviewOutcomes = {
  requestId: id, requestStatus: "ATTESTATION_PENDING", requestVersion: 3, state: "CURRENT", reviewContext: "SIMULATED_UAT",
  outcomes: [{ checkId: "employment.company_and_dates", reviewId: "22222222-2222-4222-8222-222222222222", reviewBasis: "CLAIM_ONLY", decision: "APPROVED", reason: "Claim is correct.", reviewedAt: "2026-11-18T01:28:01Z" }],
};

// Render the actual React component. Only native host primitives are substituted
// for HTML hosts; its branches, labels, rationale and date rendering run unchanged.
const appRequire = createRequire(resolve("apps/evt-ui-expo/package.json"));
const React = appRequire("react");
const { renderToStaticMarkup } = appRequire("react-dom/server");
const compiled = ts.transpileModule(readFileSync("apps/evt-ui-expo/components/RequestorReviewOutcomes.tsx", "utf8"), {
  compilerOptions: { module: ts.ModuleKind.CommonJS, jsx: ts.JsxEmit.React, esModuleInterop: true },
}).outputText;
const componentModule = { exports: {} as { RequestorReviewOutcomes?: unknown } };
new Function("require", "module", "exports", compiled)((name: string) => {
  if (name === "react") return React;
  if (name === "react-native") return Object.fromEntries(["View", "Text", "Pressable"].map(name => [name,
    ({ children, className, accessibilityRole }: { children: unknown; className: string; accessibilityRole?: string }) => React.createElement(name === "Pressable" ? "button" : "div", { className, role: accessibilityRole }, children),
  ]));
  throw new Error(`Unexpected component dependency: ${name}`);
}, componentModule, componentModule.exports);
const render = (props: Record<string, unknown>) => renderToStaticMarkup(React.createElement(componentModule.exports.RequestorReviewOutcomes, { data: result, loading: false, error: null, onRetry: () => {}, ...props }));

test("APPROVED and REJECTED render as simulated unsigned decisions with rationale and date", () => {
  for (const decision of ["APPROVED", "REJECTED"]) {
    const html = render({ data: { ...result, outcomes: [{ ...result.outcomes[0], decision }] } });
    assert.match(html, new RegExp(`Review decision: ${decision}`));
    assert.match(html, /Claim is correct\./); assert.match(html, /Reviewed:/);
    assert.match(html, /Simulated UAT/); assert.match(html, /Unsigned, non-authoritative review/);
    assert.match(html, /not employer verification or a signed credential/);
    assert.match(html, /separate from the request status/);
    assert.doesNotMatch(html, /Confirm|Approve claim|Verified|Reviewer:/);
  }
});

test("authoritative reviews do not claim credential issuance or simulated classification", () => {
  const html = render({ data: { ...result, reviewContext: "AUTHORITATIVE", outcomes: [{ ...result.outcomes[0], reviewBasis: "UPLOADED_EVIDENCE" }] } });
  assert.match(html, /Authoritative review context/); assert.match(html, /alone does not verify the claim or issue a signed credential/);
  assert.doesNotMatch(html, /Simulated UAT|Unsigned, non-authoritative/);
});

test("review rationale is rendered as text rather than executable markup", () => {
  const html = render({ data: { ...result, outcomes: [{ ...result.outcomes[0], reason: '<script>alert("review")</script>' }] } });
  assert.match(html, /&lt;script&gt;/); assert.doesNotMatch(html, /<script>/);
});

test("pending, empty, loading, stale and error states never display an earlier approval", () => {
  const pending = render({ data: { ...result, outcomes: [{ ...result.outcomes[0], decision: "PENDING", reason: null, reviewedAt: null, reviewId: null, reviewBasis: null }] } });
  assert.match(pending, /Review decision: PENDING/); assert.match(pending, /No review decision has been recorded/); assert.doesNotMatch(pending, /Reviewed:/);
  for (const [props, expected] of [
    [{ loading: true }, /Loading review outcomes/],
    [{ error: "Could not load review outcomes. Try again." }, /Try again/],
    [{ data: { ...result, state: "NOT_BOUND", reviewContext: null, outcomes: [] } }, /No review outcomes/],
    [{ data: { ...result, state: "STALE", reviewContext: null, outcomes: [] } }, /different claim version/],
    [{ data: { ...result, state: "UNAVAILABLE", reviewContext: null, outcomes: [] } }, /unavailable for this claim/],
  ] as const) {
    const html = render(props); assert.match(html, expected); assert.doesNotMatch(html, /APPROVED|Claim is correct/);
  }
});
