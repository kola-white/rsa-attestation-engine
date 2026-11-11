// src/api/requestor.ts
import type { RequestStatus } from "@/src/navigation/requestorTypes";
export { claimFromSnapshot } from "./employmentClaim";

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

export type RequestorDetail = RequestorListRow & { version: number };
export async function fetchRequestorRequest(apiBaseUrl: string, accessToken: string, requestId: string, transport: typeof fetch = fetch): Promise<RequestorDetail> {
  const response = await transport(`${apiBaseUrl.replace(/\/$/, "")}/v1/requests/${encodeURIComponent(requestId)}`, {
    headers: { Accept: "application/json", Authorization: `Bearer ${accessToken}` }, cache: "no-store",
  });
  if (!response.ok) throw new Error("Could not load the current request. Go back and try again.");
  const data: unknown = await response.json();
  const statuses: RequestStatus[] = ["DRAFT", "SUBMITTED", "ATTESTATION_PENDING", "ATTESTED", "REJECTED", "VERIFIED", "UNVERIFIED", "CONSUMED", "CLOSED", "CANCELED"];
  if (!isRecord(data) || data.request_id !== requestId || !Number.isSafeInteger(data.version) || (data.version as number) < 1
    || !statuses.includes(data.status as RequestStatus) || (data.claim_snapshot !== null && !isRecord(data.claim_snapshot))
    || typeof data.created_at !== "string" || typeof data.updated_at !== "string") throw new Error("The request response is incomplete. Go back and try again.");
  return data as RequestorDetail;
}

export async function fetchRequestorRequests(
  apiBaseUrl: string,
  accessToken: string
): Promise<RequestorListResp> {
  const base = apiBaseUrl.replace(/\/$/, "");

  const res = await fetch(`${base}/v1/requests`, {
    method: "GET",
    headers: {
      Accept: "application/json",
      Authorization: `Bearer ${accessToken}`,
    },
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
