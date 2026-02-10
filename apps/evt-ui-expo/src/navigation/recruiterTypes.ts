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
    signature: "verified" | "invalid" | "unknown";
    trust: "trusted" | "untrusted" | "unknown";
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

// UI “initial” state is exactly the same shape as query state
export type RecruiterFiltersInitial = RecruiterQueryState;

export type CandidateDetailParams = {
  candidate_id: string;
  subject_ref: { full_name: string; employee_id?: string };
  primary_evt_ref: { evt_id: string };
  list_context?: {
    search?: string;
    filters_hash?: string;
    sort?: string;
    anchor_key?: string;
  };
  prefetch_snapshot?: CandidateRowSnapshot;
};

export type RecruiterFiltersParams = {
  initial: RecruiterQueryState;
  on_apply_id?: string;
};

export type RecruiterCandidatesParams = {
  query?: RecruiterQueryState;
};

export type RecruiterStackParamList = {
  RecruiterCandidates: { query?: RecruiterQueryState } | undefined;
  CandidateDetail: {
    candidate_id: string;
    subject_ref: CandidateRowSnapshot["subject"];
    primary_evt_ref: CandidateRowSnapshot["primary_evt"];
    prefetch_snapshot?: CandidateRowSnapshot;
  };

  RecruiterFilters: {
    initial: RecruiterFiltersInitial;
    onApply?: (q: RecruiterQueryState) => void;
  };
};
