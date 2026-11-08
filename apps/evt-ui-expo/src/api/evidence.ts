// Internal app transport for the existing evidence API; no native dependencies.
// Canonical hashes are server-generated. Local picker digests never leave the app.
export const MAX_EVIDENCE_BYTES = 5 * 1024 * 1024;
export const EVIDENCE_MIME_TYPES = ["application/pdf", "image/jpeg", "image/png"] as const;
export type EvidenceFile = { uri: string; name: string; mimeType: string; size: number };
export type EvidenceScope = { caseId: string; checkId: string };
export type EvidenceCheck = EvidenceScope & { canUpload: boolean; canReview: boolean };
export type CompletionTicket = EvidenceScope & {
  uploadSessionId: string;
  uploadId: string;
  name: string;
  size: number;
  mimeType: string;
  expiresAt: string;
};
export type EvidenceReceipt = {
  uploadId: string;
  name: string;
  status: "RECEIVED";
  scanStatus: "PENDING";
  evidenceHash: string;
};
export type EvidenceProgress = "initing" | "uploading" | "completing";
type Transport = typeof fetch;
export type EvidencePut = (url: string, uri: string, headers: Record<string, string>) => Promise<{ status: number }>;

export class EvidenceRequestError extends Error {
  constructor(readonly code: string, readonly status: number, message: string) { super(message); }
}
export class EvidenceCompletionPending extends Error {
  constructor(readonly ticket: CompletionTicket) {
    super("The server has not confirmed receipt. Retry completion without uploading again.");
  }
}
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;
const caseID = /^[A-Za-z0-9._-]{1,64}$/;
const checkID = /^[A-Za-z0-9._-]{1,96}$/;
function record(value: unknown): value is Record<string, unknown> { return !!value && typeof value === "object" && !Array.isArray(value); }
function scopeValid(scope: EvidenceScope): boolean {
  return caseID.test(scope.caseId) && checkID.test(scope.checkId) && ![".", ".."].includes(scope.caseId) && ![".", ".."].includes(scope.checkId);
}
function baseURL(value: string): string {
  if (!value.trim()) throw new EvidenceRequestError("unavailable", 0, "Evidence uploads are unavailable.");
  return value.replace(/\/+$/, "");
}
function requireToken(token: string): void {
  if (!token) throw new EvidenceRequestError("unauthorized", 401, "Sign in again to upload evidence.");
}
function requestError(code: string, status: number): EvidenceRequestError {
  const messages: Record<string, string> = {
    unauthorized: "Sign in again to upload evidence.",
    not_found: "That evidence case or check is unavailable.",
    duplicate_evidence: "This evidence is already attached to this case/check.",
    evidence_limit_reached: "This check already has its maximum number of files or pending uploads.",
    upload_session_expired: "The upload session expired. Choose the file again.",
  };
  return new EvidenceRequestError(code, status, messages[code] ?? "Evidence could not be received. Try again.");
}
async function jsonRequest(transport: Transport, url: string, token: string, body?: unknown): Promise<unknown> {
  requireToken(token);
  let response: Response;
  try {
    response = await transport(url, {
      method: body === undefined ? "GET" : "POST",
      headers: { Accept: "application/json", Authorization: `Bearer ${token}`, ...(body === undefined ? {} : { "Content-Type": "application/json" }) },
      ...(body === undefined ? {} : { body: JSON.stringify(body) }),
    });
  } catch { throw requestError("network_unavailable", 0); }
  let data: unknown;
  try { data = await response.json(); } catch { data = null; }
  if (!response.ok) {
    // Do not surface raw bodies, signed URLs, tokens or native storage errors.
    const code = record(data) && typeof data.error === "string" && /^[a-z][a-z0-9_]{0,63}$/.test(data.error) ? data.error : response.status === 401 ? "unauthorized" : "request_failed";
    throw requestError(code, response.status);
  }
  return data;
}
export async function listEvidenceChecks(apiBase: string, accessToken: string, transport: Transport = fetch): Promise<EvidenceCheck[]> {
  const data = await jsonRequest(transport, `${baseURL(apiBase)}/v1/evidence/checks`, accessToken);
  if (!record(data) || !Array.isArray(data.items) || data.items.length > 100) throw requestError("invalid_response", 0);
  return data.items.map((item: unknown) => {
    if (!record(item) || typeof item.caseId !== "string" || typeof item.checkId !== "string" || typeof item.canUpload !== "boolean" || typeof item.canReview !== "boolean" || !scopeValid(item as EvidenceCheck)) throw requestError("invalid_response", 0);
    return { caseId: item.caseId, checkId: item.checkId, canUpload: item.canUpload, canReview: item.canReview };
  });
}
export async function completeEvidence(apiBase: string, accessToken: string, ticket: CompletionTicket, transport: Transport = fetch): Promise<EvidenceReceipt> {
  try {
    const data = await jsonRequest(transport, `${baseURL(apiBase)}/v1/cases/${encodeURIComponent(ticket.caseId)}/checks/${encodeURIComponent(ticket.checkId)}/evidence:complete`, accessToken, {
      uploadSessionId: ticket.uploadSessionId,
      uploads: [{ uploadId: ticket.uploadId, size: ticket.size, mimeType: ticket.mimeType }],
    });
    if (!record(data) || data.caseId !== ticket.caseId || data.checkId !== ticket.checkId || !Array.isArray(data.evidence) || data.evidence.length !== 1) throw requestError("invalid_response", 0);
    const row: unknown = data.evidence[0];
    if (!record(row) || row.uploadId !== ticket.uploadId || row.status !== "RECEIVED" || row.scanStatus !== "PENDING" || typeof row.evidence_hash !== "string" || !/^[0-9a-f]{64}$/.test(row.evidence_hash)) throw requestError("invalid_response", 0);
    return { uploadId: ticket.uploadId, name: ticket.name, status: "RECEIVED", scanStatus: "PENDING", evidenceHash: row.evidence_hash };
  } catch (error) {
    // A transport/server failure can follow a committed receipt. Keep only the
    // non-capability ticket so retries use server idempotency, never another PUT.
    if (error instanceof EvidenceRequestError && error.status >= 400 && error.status < 500) throw error;
    throw new EvidenceCompletionPending(ticket);
  }
}
export async function uploadEvidence(input: {
  apiBase: string; accessToken: string; scope: EvidenceScope; file: EvidenceFile;
  put: EvidencePut; onProgress?: (progress: EvidenceProgress) => void; transport?: Transport;
}): Promise<EvidenceReceipt> {
  const { scope, file } = input;
  requireToken(input.accessToken);
  if (!scopeValid(scope)) throw requestError("invalid_scope", 0);
  if (!file.name.trim() || file.name.trim().length > 200 || !Number.isSafeInteger(file.size) || file.size <= 0 || file.size > MAX_EVIDENCE_BYTES || !(EVIDENCE_MIME_TYPES as readonly string[]).includes(file.mimeType)) throw requestError("invalid_file", 400);
  const transport = input.transport ?? fetch;
  input.onProgress?.("initing");
  const data = await jsonRequest(transport, `${baseURL(input.apiBase)}/v1/cases/${encodeURIComponent(scope.caseId)}/checks/${encodeURIComponent(scope.checkId)}/evidence:init`, input.accessToken, {
    caseId: scope.caseId, checkId: scope.checkId,
    files: [{ name: file.name, size: file.size, mimeType: file.mimeType }],
  });
  if (!record(data) || data.caseId !== scope.caseId || data.checkId !== scope.checkId || typeof data.uploadSessionId !== "string" || !uuid.test(data.uploadSessionId) || typeof data.expiresAt !== "string" || !Number.isFinite(Date.parse(data.expiresAt)) || !Array.isArray(data.uploads) || data.uploads.length !== 1) throw requestError("invalid_response", 0);
  const upload: unknown = data.uploads[0];
  if (!record(upload) || typeof upload.uploadId !== "string" || !uuid.test(upload.uploadId) || upload.size !== file.size || upload.mimeType !== file.mimeType || upload.method !== "PUT" || typeof upload.url !== "string" || !upload.url.startsWith("https://") || !record(upload.headers) || upload.headers["Content-Type"] !== file.mimeType || upload.headers["x-amz-acl"] !== "private" || !Object.values(upload.headers).every(value => typeof value === "string")) throw requestError("invalid_response", 0);
  const ticket: CompletionTicket = { ...scope, uploadSessionId: data.uploadSessionId, uploadId: upload.uploadId, name: file.name, size: file.size, mimeType: file.mimeType, expiresAt: data.expiresAt };
  input.onProgress?.("uploading");
  let result: { status: number };
  try { result = await input.put(upload.url, file.uri, upload.headers as Record<string, string>); }
  catch { throw requestError("storage_unavailable", 0); }
  if (result.status !== 200 && result.status !== 204) throw requestError("storage_unavailable", result.status);
  input.onProgress?.("completing");
  return completeEvidence(input.apiBase, input.accessToken, ticket, transport);
}
