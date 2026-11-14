import assert from "node:assert/strict";
import { test } from "node:test";
import { EmployerReview, employerLabel } from "../apps/evt-ui-expo/src/api/employerReview";

const base = "https://api.example.test";
const legacy = "cei-uat-sim-001", second = "019a5ace-3b00-7000-8000-000000000001";
const json = (data: unknown, status = 200) => new Response(JSON.stringify(data), { status });
const row = (id = "request-a") => ({ request_id: id, status: "ATTESTATION_PENDING", claim_snapshot: {}, version: 1, created_at: "", updated_at: "" });
type Call = { url: string; init: RequestInit };
function setup(employers: unknown[], reply?: (url: string, init: RequestInit) => Response | Promise<Response>) {
  const calls: Call[] = [];
  const client = new EmployerReview(base, "PRIVATE_TOKEN", (async (input, options) => {
    const url = String(input), init = options!;
    calls.push({ url, init });
    assert.equal((init.headers as Record<string, string>).Authorization, "Bearer PRIVATE_TOKEN");
    assert.equal(init.credentials, "omit"); assert.equal(init.redirect, "error"); assert.equal(init.cache, "no-store");
    assert.ok(!url.includes("PRIVATE_TOKEN"));
    if (url.endsWith("/employers")) return json({ items: employers });
    if (reply) return reply(url, init);
    return url.includes("/requests?") ? json({ items: [row()] }) : init.method === "POST" ? json({ status: "ATTESTED" }) : json(row());
  }) as typeof fetch);
  return { client, calls };
}

test("one assignment automatically loads its text employer queue and detail without ID input", async () => {
  const { client, calls } = setup([{ employer_id: legacy }]);
  await client.refresh();
  assert.equal(client.snapshot().employerId, legacy);
  assert.equal(client.snapshot().selected?.request_id, "request-a");
  assert.equal(calls.length, 3);
  assert.ok(calls[1].url.endsWith("?employer_id=" + legacy));
  assert.equal(await client.attest("FULL_MATCH"), "ATTESTED");
  assert.deepEqual(JSON.parse(calls[3].init.body as string), { employer_id: legacy, response_type: "FULL_MATCH", response_body: {} });
  assert.equal(client.snapshot().selected, null);
});

test("multiple assignments require selection, support switching and reject manipulated selections", async () => {
  const { client, calls } = setup([{ employer_id: legacy }, { employer_id: second }]);
  await client.refresh();
  assert.equal(client.snapshot().employerId, null);
  assert.equal(calls.length, 1);
  await client.selectEmployer("arbitrary-employer");
  assert.equal(calls.length, 1);
  await client.selectEmployer(second);
  assert.equal(client.snapshot().employerId, second);
  assert.ok(calls[1].url.includes(second));
  await client.selectEmployer(legacy);
  assert.equal(client.snapshot().employerId, legacy);
  assert.ok(calls[3].url.includes(legacy));
  await client.refresh();
  assert.equal(client.snapshot().employerId, legacy);
});

test("no assignments never fetches a queue or falls back to a demo employer", async () => {
  const { client, calls } = setup([]);
  await client.refresh();
  await client.selectEmployer(legacy);
  assert.equal(await client.attest("FULL_MATCH"), null);
  assert.equal(calls.length, 1);
  assert.deepEqual(client.snapshot().queue, []);
  assert.equal(client.snapshot().loading, false);
});

test("revoked/unauthorized queue, detail and decision clear previously actionable state", async () => {
  for (const status of [401, 403, 404]) {
    for (const target of ["queue", "detail", "decision"]) {
      let deny = false;
      const { client } = setup([{ employer_id: legacy }], (url, init) => {
        const action = init.method === "POST" ? "decision" : url.includes("/requests?") ? "queue" : "detail";
        if (deny && action === target) return json({ error: "PRIVATE_TOKEN PRIVATE_ERROR" }, status);
        return action === "queue" ? json({ items: [row()] }) : action === "decision" ? json({ status: "ATTESTED" }) : json(row());
      });
      await client.refresh();
      assert.ok(client.snapshot().selected);
      deny = true;
      if (target === "decision") await client.attest("FULL_MATCH");
      else if (target === "detail") await client.selectRequest("request-a");
      else await client.refresh();
      assert.equal(client.snapshot().selected, null);
      assert.equal(client.snapshot().employerId, null);
      assert.deepEqual(client.snapshot().employers, []);
      assert.ok(client.snapshot().error);
      assert.ok(!client.snapshot().error!.includes("PRIVATE"));
    }
  }
});

test("rediscovery removes revoked assignments instead of trusting a cached selection", async () => {
  let employers = [{ employer_id: legacy }, { employer_id: second }];
  const client = new EmployerReview(base, "token", (async (url) => String(url).endsWith("/employers")
    ? json({ items: employers }) : String(url).includes("/requests?") ? json({ items: [] }) : json(row())) as typeof fetch);
  await client.refresh(); await client.selectEmployer(legacy);
  employers = [{ employer_id: second }];
  await client.refresh();
  assert.equal(client.snapshot().employerId, second);
  employers = [];
  await client.refresh();
  assert.equal(client.snapshot().employerId, null);
  assert.deepEqual(client.snapshot().queue, []);
});

test("late old-employer queue and detail responses cannot replace the new context", async () => {
  for (const delayed of ["queue", "detail"]) {
    let release!: (response: Response) => void;
    const deferred = new Promise<Response>(resolve => { release = resolve; });
    const { client } = setup([{ employer_id: legacy }, { employer_id: second }], (url) => {
      const old = url.includes(legacy), queue = url.includes("/requests?");
      if (old && (queue ? "queue" : "detail") === delayed) return deferred;
      return queue ? json({ items: [row(old ? "request-a" : "request-b")] }) : json(row(old ? "request-a" : "request-b"));
    });
    await client.refresh();
    const old = client.selectEmployer(legacy);
    // Allow the queue promise to reach its deferred detail call.
    await new Promise(resolve => setImmediate(resolve));
    await client.selectEmployer(second);
    release(delayed === "queue" ? json({ items: [row()] }) : json(row()));
    await old;
    assert.equal(client.snapshot().employerId, second);
    assert.equal(client.snapshot().selected?.request_id, "request-b");
  }
});

test("clearing on blur/session change invalidates in-flight discovery", async () => {
  let release!: (response: Response) => void;
  const client = new EmployerReview(base, "token", (() => new Promise(resolve => { release = resolve; })) as typeof fetch);
  const pending = client.refresh(); client.clear();
  release(json({ items: [{ employer_id: legacy }] })); await pending;
  assert.deepEqual(client.snapshot().employers, []);
  assert.equal(client.snapshot().selected, null);
});

test("decision in flight blocks duplicate submissions and employer changes", async () => {
  let release!: (response: Response) => void;
  let posts = 0;
  const { client } = setup([{ employer_id: legacy }, { employer_id: second }], (url, init) => {
    if (init.method === "POST") { ++posts; return new Promise(resolve => { release = resolve; }); }
    return url.includes("/requests?") ? json({ items: [row()] }) : json(row());
  });
  await client.refresh(); await client.selectEmployer(legacy);
  const pending = client.attest("FULL_MATCH");
  assert.equal(await client.attest("FULL_MATCH"), null);
  await client.selectEmployer(second); await client.refresh();
  assert.equal(client.snapshot().employerId, legacy);
  assert.equal(posts, 1);
  release(json({ status: "ATTESTED" }));
  await pending;
  assert.equal(client.snapshot().attesting, null);
});

test("ambiguous decision failures require reconciliation and leave no actionable stale request", async () => {
  for (const failure of ["network", "server", "malformed"]) {
    const { client } = setup([{ employer_id: legacy }], (url, init) => {
      if (init.method === "POST") {
        if (failure === "network") throw new Error("PRIVATE_TOKEN");
        return failure === "server" ? json({ error: "PRIVATE" }, 500) : json({});
      }
      return url.includes("/requests?") ? json({ items: [row()] }) : json(row());
    });
    await client.refresh(); await client.attest("FULL_MATCH");
    assert.equal(client.snapshot().selected, null);
    assert.match(client.snapshot().error!, /could not be confirmed/);
    assert.equal(await client.attest("FULL_MATCH"), null);
  }
});

test("malformed discovery and failed services fail closed without raw errors", async () => {
  for (const employers of [[{ employer_id: "" }], [{ employer_id: legacy }, { employer_id: legacy }], [{ employer_id: 1 }]]) {
    const { client, calls } = setup(employers);
    await client.refresh();
    assert.equal(calls.length, 1);
    assert.equal(client.snapshot().employerId, null);
    assert.ok(client.snapshot().error);
  }
  const { client } = setup([{ employer_id: legacy }], () => json({ error: "PRIVATE" }, 503));
  await client.refresh();
  assert.equal(client.snapshot().selected, null);
  assert.ok(!client.snapshot().error!.includes("PRIVATE"));
});

test("labels use reliable metadata when supplied, otherwise neutral access labels, never identifiers", () => {
  assert.equal(employerLabel({ employer_id: legacy }, 0), "Employer access 1");
  assert.equal(employerLabel({ employer_id: legacy, display_name: "Registered employer name" }, 0), "Registered employer name");
  assert.equal(employerLabel({ employer_id: second }, 1), "Employer access 2");
});

test("opaque text identifiers are encoded without normalization or UUID assumptions", async () => {
  const id = "legacy/name & division";
  const { client, calls } = setup([{ employer_id: id }]);
  await client.refresh();
  assert.equal(client.snapshot().employerId, id);
  assert.ok(calls[1].url.endsWith(encodeURIComponent(id)));
});
