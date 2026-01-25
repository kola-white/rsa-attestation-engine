// src/navigation/requestorTypes.ts
export type RequestStatus =
  | "DRAFT"
  | "SUBMITTED"
  | "ATTESTATION_PENDING"
  | "ATTESTED"
  | "REJECTED"
  | "VERIFIED"
  | "UNVERIFIED"
  | "CONSUMED"
  | "CLOSED";

export type EmploymentClaimDraft = {
  employer: string;
  job_title: string;
  start_mm_yyyy: string; // "MM/YYYY"
  end_mm_yyyy: string | null; // null = current
};

export type EvidenceMimeType =
  | "application/pdf"
  | "image/jpeg"
  | "image/png";

export type EvidenceAttachment = {
  name: string;
  mimeType: EvidenceMimeType;
  size: number;
  storageKey: string;
  uploaded_at: string;
};

export type RequestRowSnapshot = {
  request_id: string;
  claim: EmploymentClaimDraft;
  status: RequestStatus;
  created_at: string;
  updated_at: string;
};

export type RequestorStackParamList = {
  RequestorHome: undefined;

  RequestorNewRequest: undefined;

  RequestorRequestDetail: {
    request_id: string;
    snapshot?: RequestRowSnapshot;
  };
};
