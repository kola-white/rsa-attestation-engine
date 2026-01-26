// src/api/requestor.ts
import type { RequestStatus, EmploymentClaimDraft } from "@/src/navigation/requestorTypes";

export type RequestorListRow = {
  request_id: string;
  status: RequestStatus;
  claim_snapshot: Record<string, unknown> | null;
  created_at: string;
  updated_at: string;
};

export type RequestorListResp = { items: RequestorListRow[] };

function isRecord(v: unknown): v is Record<string, unknown> {
  return !!v && typeof v === "object" && !Array.isArray(v);
}

/**
 * Coerce claim_snapshot -> EmploymentClaimDraft for the existing UI.
 * If fields are missing, we fall back to safe placeholders.
 */
export function claimFromSnapshot(
  snap: Record<string, unknown> | null
): EmploymentClaimDraft {
  const employer =
    isRecord(snap) && typeof snap.employer === "string" ? snap.employer : "Unknown employer";

  const job_title =
    isRecord(snap) && typeof snap.job_title === "string" ? snap.job_title : "Unknown title";

  const start_mm_yyyy =
    isRecord(snap) && typeof snap.start_mm_yyyy === "string" ? snap.start_mm_yyyy : "??/????";

  const end_mm_yyyy =
    isRecord(snap) && (typeof snap.end_mm_yyyy === "string" || snap.end_mm_yyyy === null)
      ? (snap.end_mm_yyyy as string | null)
      : null;

  return { employer, job_title, start_mm_yyyy, end_mm_yyyy };
}

export async function fetchRequestorRequests(apiBaseUrl: string): Promise<RequestorListResp> {
  const base = apiBaseUrl.replace(/\/$/, "");
  const res = await fetch(`${base}/v1/requests`, {
    method: "GET",
    headers: { Accept: "application/json" },
  });

  const text = await res.text();
  let json: unknown = null;
  try {
    json = text ? JSON.parse(text) : null;
  } catch {
    json = null;
  }

  if (!res.ok) {
    const msg =
      (isRecord(json) && typeof json.error === "string" && json.error) ||
      (text || `HTTP ${res.status}`);
    throw new Error(msg);
  }

  // Runtime guard (minimal)
  if (!isRecord(json) || !Array.isArray((json as any).items)) {
    throw new Error("invalid_list_response_shape");
  }

  return json as RequestorListResp;
}
