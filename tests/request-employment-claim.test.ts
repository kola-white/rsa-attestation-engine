import assert from "node:assert/strict";
import { test } from "node:test";
import { claimFromSnapshot, employmentDraftFromSnapshot, employmentSnapshot, employmentDraftNeedsSave } from "../apps/evt-ui-expo/src/api/employmentClaim";
import { fetchRequestorRequest } from "../apps/evt-ui-expo/src/api/requestor";

const draft = { employer: "NorStar Systems LLC", job_title: "Managing Member — owner", start_mm_yyyy: "11/2026", end_mm_yyyy: null };
const id = "11111111-1111-4111-8111-111111111111";

test("New Request writes the existing employment.role parser's primary_employment contract and keeps legacy fields", () => {
  const snapshot = employmentSnapshot(draft, {}, "norstar-systems-llc");
  assert.deepEqual(snapshot, {
    ...draft,
    primary_employment: { employer_id: "norstar-systems-llc", issuer_name: "NorStar Systems LLC", title: "Managing Member — owner" },
  });
  assert.equal(snapshot.subject, undefined); assert.equal(snapshot.issuer_id, undefined);
  assert.deepEqual(claimFromSnapshot(snapshot), draft);
  assert.equal(employmentDraftNeedsSave(draft, snapshot), false);
});

test("old four-field snapshots remain readable and are upgraded only by an explicit draft save", () => {
  assert.deepEqual(claimFromSnapshot(draft), draft);
  const before = JSON.stringify(draft);
  assert.equal(employmentDraftNeedsSave(draft, draft), true);
  const upgraded = employmentSnapshot(draft, draft);
  assert.deepEqual(upgraded.primary_employment, { title: draft.job_title, issuer_name: draft.employer });
  assert.equal(JSON.stringify(draft), before);
  // A legacy display name is never invented as an authoritative employer identifier.
  assert.equal((upgraded.primary_employment as Record<string, unknown>).employer_id, undefined);
});

test("canonical fields drive display even when old aliases conflict; canonical-only API snapshots remain usable", () => {
  const existing = { job_title: "stale title", employer: "stale name", primary_employment: { title: "Current title", issuer_name: "Current employer", start_date: "2025-07-16", end_date: "2026-02-09" } };
  assert.deepEqual(claimFromSnapshot(existing), { employer: "Current employer", job_title: "Current title", start_mm_yyyy: "07/2025", end_mm_yyyy: "02/2026" });
});

test("saving edited title preserves the full subject, claim type, employer identity, exact dates and unknown metadata", () => {
  const existing = {
    claim_type: "employment.role", subject: { id, full_name: "Fixture subject" }, original_reference: "opaque-reference",
    primary_employment: { employer_id: "norstar-systems-llc", title: "Previous title", issuer_name: draft.employer, start_date: "2026-11-08", end_date: null, level: "owner", additional: { original: true } },
  };
  const original = JSON.stringify(existing);
  const edited = { ...claimFromSnapshot(existing), job_title: "Updated truthful title" };
  const saved = employmentSnapshot(edited, existing);
  assert.deepEqual(saved.subject, existing.subject); assert.equal(saved.claim_type, existing.claim_type);
  assert.equal(saved.original_reference, existing.original_reference);
  assert.deepEqual(saved.primary_employment, { ...existing.primary_employment, title: edited.job_title });
  assert.equal(JSON.stringify(existing), original);
  assert.equal(employmentDraftNeedsSave(edited, existing), true);
  assert.equal(employmentDraftNeedsSave(edited, saved), false);
  assert.deepEqual(claimFromSnapshot(saved), edited);
});

test("month-only inputs never acquire invented days and editing a month does not retain a conflicting exact date", () => {
  const newSnapshot = employmentSnapshot(draft);
  assert.equal((newSnapshot.primary_employment as Record<string, unknown>).start_date, undefined);
  const existing = { primary_employment: { title: draft.job_title, issuer_name: draft.employer, start_date: "2026-10-17", end_date: "2026-11-08" } };
  const edited = { ...claimFromSnapshot(existing), start_mm_yyyy: "11/2026", end_mm_yyyy: null };
  const saved = employmentSnapshot(edited, existing);
  assert.equal((saved.primary_employment as Record<string, unknown>).start_date, undefined);
  assert.equal((saved.primary_employment as Record<string, unknown>).end_date, undefined);
  assert.equal(saved.start_mm_yyyy, "11/2026"); assert.equal(saved.end_mm_yyyy, null);
});

test("blank canonical titles fail validation; formatting normalization is consistent across create and save", () => {
  const empty = employmentDraftFromSnapshot(null);
  assert.deepEqual(empty, { employer: "", job_title: "", start_mm_yyyy: "", end_mm_yyyy: null });
  assert.throws(() => employmentSnapshot(empty), /job title/);
  assert.equal(claimFromSnapshot(null).job_title, "Unknown title");
  assert.throws(() => employmentSnapshot({ ...draft, job_title: "  " }), /job title/);
  assert.throws(() => employmentSnapshot(draft, {}, "  "), /employer ID/);
  const trimmed = employmentSnapshot({ ...draft, job_title: "  Manager  ", employer: " NorStar Systems LLC " }, {}, " norstar-systems-llc ");
  assert.equal(trimmed.job_title, "Manager");
  assert.deepEqual(trimmed.primary_employment, { title: "Manager", issuer_name: "NorStar Systems LLC", employer_id: "norstar-systems-llc" });
});

test("reopening a draft loads its complete server snapshot and real optimistic-concurrency version", async () => {
  const snapshot = { ...employmentSnapshot(draft, {}, "norstar-systems-llc"), extra_context: { keep: true } };
  const response = { request_id: id, status: "DRAFT", claim_snapshot: snapshot, version: 7, created_at: "2026-11-10T00:00:00Z", updated_at: "2026-11-10T01:00:00Z" };
  const observed = await fetchRequestorRequest("https://api.example.test/", "private-token", id, (async (url, options) => {
    assert.equal(url, `https://api.example.test/v1/requests/${id}`);
    assert.deepEqual(options!.headers, { Accept: "application/json", Authorization: "Bearer private-token" });
    assert.equal(options!.cache, "no-store");
    return new Response(JSON.stringify(response));
  }) as typeof fetch);
  assert.equal(observed.version, 7); assert.deepEqual(observed.claim_snapshot, snapshot);
  const edited = employmentSnapshot({ ...claimFromSnapshot(observed.claim_snapshot), job_title: "Revised title" }, observed.claim_snapshot!);
  assert.deepEqual(edited.extra_context, { keep: true });
});

test("wrong request, missing version, bad snapshot or denied fetch cannot supply editable request state", async () => {
  for (const response of [{ request_id: "other" }, { request_id: id, version: 0 }, { request_id: id, version: 1, status: "DRAFT", claim_snapshot: [] }]) {
    await assert.rejects(() => fetchRequestorRequest("https://api.example.test", "token", id, (async () => new Response(JSON.stringify(response))) as typeof fetch));
  }
  for (const status of [401, 403, 404]) {
    await assert.rejects(() => fetchRequestorRequest("https://api.example.test", "token", id, (async () => new Response("PRIVATE_ERROR", { status })) as typeof fetch), (e: unknown) => e instanceof Error && !e.message.includes("PRIVATE_ERROR"));
  }
});
