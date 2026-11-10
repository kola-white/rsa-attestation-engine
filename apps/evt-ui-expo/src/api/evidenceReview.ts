import { EvidenceCheck, EvidenceScope, EvidenceRequestError, listEvidenceChecks, MAX_EVIDENCE_BYTES } from "./evidence";

export type Decision = "APPROVED" | "REJECTED";
export type ReviewReceipt = { reviewId: string; uploadId: string; reviewerId: string; decision: Decision; reason: string; reviewedAt: string };
export type ReviewItem = {
  uploadId: string; name: string; mimeType: string; size: number; evidence_hash: string;
  status: "RECEIVED"; scanStatus: string; reviewStatus: "PENDING" | Decision;
  reviewerId: string | null; reviewedAt: string | null; reviewReason: string | null;
  canReview: boolean; canApprove: boolean;
};
export type Binding = {
  bindingId: string; requestId: string; subjectId: string; employerId: string; issuerId: string;
  claimType: string; claimDigest: string; contextValid: boolean; requiredCheck: boolean;
  claimSnapshot: Record<string, unknown> | null;
};
export type ReviewListing = EvidenceScope & { evidence: ReviewItem[]; binding: Binding | null };
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;
const digest = /^[0-9a-f]{64}$/;
const object = (v: unknown): v is Record<string, unknown> => !!v && typeof v === "object" && !Array.isArray(v);
const decision = (v: unknown): v is Decision => v === "APPROVED" || v === "REJECTED";
const timestamp = (v: unknown): v is string => typeof v === "string" && Number.isFinite(Date.parse(v));
export function validRationale(value: string): boolean {
  const reason = value.trim();
  // Hermes compatibility: do not require a global TextEncoder.
  let bytes = 0;
  for (const character of reason) {
    const point = character.codePointAt(0)!;
    bytes += point <= 0x7f ? 1 : point <= 0x7ff ? 2 : point <= 0xffff ? 3 : 4;
  }
  return reason.length > 0 && bytes <= 1000;
}

export class ReviewError extends Error {
  constructor(readonly status: number, readonly uncertain = false) {
    super(status === 401 ? "Your session expired. Sign in again." : status === 403 || status === 404
      ? "This evidence is unavailable or your review access has ended."
      : uncertain ? "The decision could not be confirmed. Refresh to check the recorded review; do not submit again until reconciled."
      : status === 409 ? "The review or request context has changed. Refresh before continuing."
      : "Evidence review is unavailable. Refresh and try again.");
  }
}

// Uses the existing in-app Bearer session only. No cookies, redirects, raw errors or credentials in URLs.
export class EvidenceReviewClient {
  constructor(private base: string, private token: string, private transport: typeof fetch = fetch) {
    if (!/^https:\/\//.test(base) || !token || /[?#]/.test(base)) throw new ReviewError(401);
    this.base = base.replace(/\/+$/, "");
  }
  private async request(path: string, body?: unknown, allowErrorStatus = false): Promise<Response> {
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), 30000);
    try {
      const response = await this.transport(this.base + path, {
        method: body === undefined ? "GET" : "POST", redirect: "error", credentials: "omit", cache: "no-store",
        signal: controller.signal,
        headers: { Authorization: `Bearer ${this.token}`, Accept: "application/json", ...(body === undefined ? {} : { "Content-Type": "application/json" }) },
        ...(body === undefined ? {} : { body: JSON.stringify(body) }),
      });
      if (!response.ok && !allowErrorStatus) throw new ReviewError(response.status, body !== undefined && response.status >= 500);
      return response;
    } catch (error) {
      if (error instanceof ReviewError) throw error;
      throw new ReviewError(0, body !== undefined);
    } finally { clearTimeout(timer); }
  }
  private path(scope: EvidenceScope, upload?: string): string {
    if (!/^[A-Za-z0-9_.-]{1,64}$/.test(scope.caseId) || !/^[A-Za-z0-9_.-]{1,96}$/.test(scope.checkId)
      || [".", ".."].includes(scope.caseId) || [".", ".."].includes(scope.checkId) || (upload !== undefined && !uuid.test(upload))) throw new ReviewError(400);
    return `/v1/cases/${encodeURIComponent(scope.caseId)}/checks/${encodeURIComponent(scope.checkId)}/evidence${upload ? `/${upload}` : ""}`;
  }
  async assignments(): Promise<EvidenceCheck[]> {
    // Reuse the existing discovery contract/validator with this client's guarded transport.
    let rows: EvidenceCheck[];
    try { rows = await listEvidenceChecks(this.base, this.token, (async (url) => {
      if (String(url) !== this.base + "/v1/evidence/checks") throw new ReviewError(400);
      return this.request("/v1/evidence/checks", undefined, true);
    }) as typeof fetch); } catch (error) {
      if (error instanceof EvidenceRequestError) throw new ReviewError(error.status);
      if (error instanceof ReviewError) throw error;
      throw new ReviewError(502);
    }
    return rows.filter(row => row.canReview);
  }
  async listing(scope: EvidenceScope): Promise<ReviewListing> {
    let data: unknown;
    try { data = await (await this.request(this.path(scope))).json(); }
    catch (error) { if (error instanceof ReviewError) throw error; throw new ReviewError(502); }
    if (!object(data) || data.caseId !== scope.caseId || data.checkId !== scope.checkId || !Array.isArray(data.evidence)) throw new ReviewError(502);
    for (const row of data.evidence) {
      if (!object(row) || typeof row.uploadId !== "string" || !uuid.test(row.uploadId) || typeof row.name !== "string"
        || typeof row.mimeType !== "string" || !Number.isSafeInteger(row.size) || (row.size as number) <= 0 || (row.size as number) > MAX_EVIDENCE_BYTES
        || typeof row.evidence_hash !== "string" || !digest.test(row.evidence_hash) || row.status !== "RECEIVED"
        || typeof row.scanStatus !== "string" || !(row.reviewStatus === "PENDING" || decision(row.reviewStatus))
        || typeof row.canReview !== "boolean" || typeof row.canApprove !== "boolean"
        || (row.reviewStatus !== "PENDING" && (typeof row.reviewerId !== "string" || !uuid.test(row.reviewerId) || !timestamp(row.reviewedAt) || typeof row.reviewReason !== "string"))) throw new ReviewError(502);
    }
    const b = data.binding;
    if (b != null && (!object(b) || ![b.bindingId, b.requestId, b.subjectId].every(x => typeof x === "string" && uuid.test(x))
      || ![b.employerId, b.issuerId, b.claimType].every(x => typeof x === "string" && x.length > 0)
      || typeof b.claimDigest !== "string" || !digest.test(b.claimDigest) || typeof b.contextValid !== "boolean"
      || typeof b.requiredCheck !== "boolean" || (b.claimSnapshot !== null && !object(b.claimSnapshot))
      || (b.contextValid && b.claimSnapshot === null))) throw new ReviewError(502);
    return { ...scope, evidence: data.evidence as ReviewItem[], binding: (b ?? null) as Binding | null };
  }
  async download(scope: EvidenceScope, item: ReviewItem, sha256: (bytes: Uint8Array) => Promise<string>): Promise<Uint8Array> {
    const response = await this.request(this.path(scope, item.uploadId) + "/download");
    if (response.headers.get("Content-Type") !== "application/octet-stream" || response.headers.get("Cache-Control") !== "private, no-store"
      || !["PENDING", "CLEAN"].includes(item.scanStatus)) throw new ReviewError(502);
    let bytes: Uint8Array;
    try { bytes = new Uint8Array(await response.arrayBuffer()); } catch { throw new ReviewError(502); }
    if (bytes.length !== item.size || bytes.length > MAX_EVIDENCE_BYTES || await sha256(bytes) !== item.evidence_hash) throw new ReviewError(502);
    return bytes;
  }
  async submit(scope: EvidenceScope, item: ReviewItem, binding: Binding, selected: Decision, rationale: string, reviewer: string): Promise<ReviewReceipt> {
    const reason = rationale.trim();
    if (!item.canReview || item.reviewStatus !== "PENDING" || !binding.contextValid || !binding.requiredCheck
      || !decision(selected) || (selected === "APPROVED" && !item.canApprove) || !validRationale(reason)) throw new ReviewError(400);
    const response = await this.request(this.path(scope, item.uploadId) + "/review", { decision: selected, reason, bindingId: binding.bindingId, claimDigest: binding.claimDigest });
    let r: unknown;
    try { r = await response.json(); } catch { throw new ReviewError(502, true); }
    if (!object(r) || typeof r.reviewId !== "string" || !uuid.test(r.reviewId) || r.uploadId !== item.uploadId
      || r.reviewerId !== reviewer || r.decision !== selected || r.reason !== reason || !timestamp(r.reviewedAt)) throw new ReviewError(502, true);
    return r as ReviewReceipt;
  }
}

// Synchronous acquisition prevents two taps before React's pending render. Uncertain writes stay blocked.
export class ReviewSubmissionGate {
  private blocked = new Set<string>();
  async run<T>(key: string, action: () => Promise<T>): Promise<T> {
    if (this.blocked.has(key)) throw new ReviewError(409);
    this.blocked.add(key);
    try { return await action(); }
    catch (error) { if (error instanceof ReviewError && !error.uncertain) this.blocked.delete(key); throw error; }
  }
  has(key: string): boolean { return this.blocked.has(key); }
}
