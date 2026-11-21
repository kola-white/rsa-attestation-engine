export type ReviewOutcome = {
  checkId: string;
  reviewId: string | null;
  reviewBasis: "CLAIM_ONLY" | "UPLOADED_EVIDENCE" | null;
  decision: "APPROVED" | "REJECTED" | "PENDING";
  reason: string | null;
  reviewedAt: string | null;
};
export type RequestorReviewOutcomes = {
  requestId: string;
  requestStatus: string;
  requestVersion: number;
  state: "NOT_BOUND" | "CURRENT" | "STALE" | "UNAVAILABLE";
  reviewContext: "SIMULATED_UAT" | "AUTHORITATIVE" | null;
  outcomes: ReviewOutcome[];
};
const object = (v: unknown): v is Record<string, unknown> => !!v && typeof v === "object" && !Array.isArray(v);
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
const unavailable = () => new Error("Could not load review outcomes. Try again.");

export async function fetchRequestorReviewOutcomes(
  base: string, token: string, requestId: string, transport: typeof fetch = fetch,
): Promise<RequestorReviewOutcomes> {
  if (!uuid.test(requestId)) throw unavailable();
  try {
    const response = await transport(`${base.replace(/\/$/, "")}/v1/requests/${requestId}/review-outcomes`, {
      method: "GET", headers: { Accept: "application/json", Authorization: `Bearer ${token}` }, cache: "no-store",
    });
    if (!response.ok) throw unavailable();
    const data: unknown = await response.json();
    if (!object(data) || data.requestId !== requestId || typeof data.requestStatus !== "string"
      || !Number.isSafeInteger(data.requestVersion) || (data.requestVersion as number) < 1
      || !["NOT_BOUND", "CURRENT", "STALE", "UNAVAILABLE"].includes(String(data.state)) || !Array.isArray(data.outcomes)) throw unavailable();
    if (data.state === "CURRENT") {
      if (!["SIMULATED_UAT", "AUTHORITATIVE"].includes(String(data.reviewContext))) throw unavailable();
    } else if (data.reviewContext !== null || data.outcomes.length !== 0) throw unavailable();
    for (const row of data.outcomes) {
      if (!object(row) || typeof row.checkId !== "string" || row.checkId.length === 0) throw unavailable();
      if (row.decision === "PENDING") {
        if (row.reviewId !== null || row.reviewBasis !== null || row.reason !== null || row.reviewedAt !== null) throw unavailable();
      } else if (!["APPROVED", "REJECTED"].includes(String(row.decision))
        || typeof row.reviewId !== "string" || !uuid.test(row.reviewId)
        || !["CLAIM_ONLY", "UPLOADED_EVIDENCE"].includes(String(row.reviewBasis))
        || (row.reviewBasis === "CLAIM_ONLY" && data.reviewContext !== "SIMULATED_UAT")
        || typeof row.reason !== "string" || !row.reason.trim() || typeof row.reviewedAt !== "string"
        || !Number.isFinite(Date.parse(row.reviewedAt))) throw unavailable();
    }
    return data as RequestorReviewOutcomes;
  } catch { throw unavailable(); }
}
