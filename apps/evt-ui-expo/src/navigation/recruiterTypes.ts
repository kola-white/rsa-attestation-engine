// src/navigation/recruiterTypes.ts

export type SignatureBadge = "verified" | "invalid" | "unknown";
export type TrustBadge = "trusted" | "untrusted" | "unknown";

export type CandidateRowSnapshot = {
  candidate_id: string;
  subject: { full_name: string; employee_id?: string };
  primary_employment: {
    issuer_name: string;
    title: string;
    start_date: string;
    end_date: string | null;
  };
  primary_evt: { evt_id: string };
  badges: {
    signature: SignatureBadge;
    trust: TrustBadge;
  };
  updated_at: string;
};

export type RecruiterTrustMode = "any" | "trusted_only" | "include_untrusted";
export type RecruiterSort = "most_recent" | "name_az" | "trust_first";

export type RecruiterQueryState = {
  search: string;
  trust_mode: RecruiterTrustMode;
  signature_status: SignatureBadge[];
  company_ids: string[];

  title_query?: string;
  dates?: {
    start_after?: string;
    end_before?: string;
    include_current?: boolean;
  };

  sort?: RecruiterSort;
  page?: { cursor?: string; limit?: number };
};

export type CandidateDetailParams = {
  candidate_id: string;
  subject_ref: CandidateRowSnapshot["subject"];
  primary_evt_ref: CandidateRowSnapshot["primary_evt"];
  list_context?: {
    search?: string;
    filters_hash?: string;
    sort?: string;
    anchor_key?: string;
  };
  prefetch_snapshot?: CandidateRowSnapshot;
};

export type RecruiterStackParamList = {
  RecruiterCandidates: undefined;
  RecruiterFilters: undefined; // modal; Zustand owns draft/applied, no params
  CandidateDetail: CandidateDetailParams;
};