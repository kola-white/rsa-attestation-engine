export type TrustCfg = {
  base_url?: string;
  jwks_path?: string;
  status_path?: string;
  latest_pointer?: string;
};

export type LatestJson = {
  sha?: string;
  prefix?: string; // "attestation-engine/<sha>"
};

export type Jwk = {
  kty: "RSA";
  use?: "sig";
  kid: string;
  alg?: "RS256";
  n: string;
  e: string;
};

export type Jwks = { keys: Jwk[] };

export type StatusEntry = { serial: string; status: "good" | "revoked" };
export type StatusList = {
  version?: string;
  ttl_s?: number;
  entries: StatusEntry[];
};

export type Policy = {
  schema_uri: string;
  allowed_assurance: string[];
};

// -----------------------------------------------------------------------------
// DP-1: Disambiguation Protocol Types
// -----------------------------------------------------------------------------

/**
 * Final outcome of a DP-1 disambiguation run.
 *
 * SELECTED              → one winning attestation was chosen
 * NO_VALID_ATTESTATIONS → all candidates failed eligibility
 * AMBIGUOUS_RESULT      → tie remained after all tie-break rules
 */
export type Dp1OutcomeCode =
  | "SELECTED"
  | "NO_VALID_ATTESTATIONS"
  | "AMBIGUOUS_RESULT";

/**
 * Eligibility state after DP-1 applies hard gates (signature, revocation, etc.).
 */
export type Dp1Eligibility = "eligible" | "ineligible";

/**
 * Per-dimension scores used to compute the composite DP-1 score.
 * These bounds follow the DP-1 profile: 30 + 25 + 25 + 10 + 10 = 100.
 */
export type Dp1DimensionScores = {
  /** 0–30 — strength of issuer + trust path + key state */
  issuer_confidence: number;
  /** 0–25 — AP-1 schema fidelity and structural quality */
  schema_fidelity: number;
  /** 0–25 — temporal alignment between claim and validity window */
  temporal_alignment: number;
  /** 0–10 — revocation freshness / status list recency */
  revocation_freshness: number;
  /** 0–10 — uniqueness / entropy of claim & context */
  uniqueness_signals: number;
};

/**
 * A single attestation candidate as seen by DP-1.
 * `serial` aligns with the revocation serial used in StatusEntry.
 */
export type Dp1Candidate = {
  /** Revocation serial / attestation identifier (same as StatusEntry.serial) */
  serial: StatusEntry["serial"];

  /** Optional issuer identifier, e.g. did:org:acme-corp */
  issuer_id?: string;

  /**
   * Composite confidence score 0–100 (sum of dimension scores).
   * This is the number DP-1 uses for primary ranking.
   */
  score: number;

  /**
   * Eligibility outcome after applying all hard gates
   * (signature, schema, revocation, policy, etc.).
   */
  eligibility: Dp1Eligibility;

  /**
   * Reasons for ineligibility, if any.
   * These should mirror your existing verification result codes
   * (e.g., "BAD_SIGNATURE", "REVOKED", "SCHEMA_INVALID", ...).
   */
  ineligible_reasons?: string[];

  /**
   * Per-dimension scores that contributed to `score`.
   * Useful for debugging, logging and audit.
   */
  dimensions: Dp1DimensionScores;
};

/**
 * Context describing what was disambiguated in this DP-1 run.
 * This is intentionally minimal and aligned with AP-1 / Policy.
 */
export type Dp1Context = {
  /** Claim type this disambiguation pertains to (AP-1: employment.role) */
  claim_type: "employment.role";

  /** Profile ID of the underlying attestation profile */
  profile_id: "AP-1/employment.role/v1";

  /**
   * Optional policy URI that drove verification / disambiguation,
   * typically the same value used in AP-1 policy.policy_uri.
   */
  policy_uri?: string;

  /**
   * When the disambiguation run was executed (ISO-8601 string).
   * This is useful for audit logs and offline analysis.
   */
  run_at?: string;
};

/**
 * Final decision produced by DP-1.
 */
export type Dp1Decision = {
  /** High-level DP-1 outcome code */
  code: Dp1OutcomeCode;

  /**
   * Serial of the winning attestation when code === "SELECTED".
   * Omitted for NO_VALID_ATTESTATIONS and AMBIGUOUS_RESULT.
   */
  selected_serial?: StatusEntry["serial"];

  /**
   * Human-readable explanation of the decision,
   * e.g. "highest_score", "all_candidates_failed_eligibility_filter",
   * "score_tie_after_tiebreak".
   */
  reason: string;

  /**
   * True if any tie-break rule beyond simple max(score) was used.
   */
  tiebreak_applied?: boolean;

  /**
   * Which tie-break rule resolved the decision, if any.
   * Examples: "trust_path_depth", "newer_key", "narrower_window",
   * "schema_version", "context_richness", "all_rules_exhausted".
   */
  tiebreak_reason?: string;

  /**
   * When code === "AMBIGUOUS_RESULT", the serials that remain
   * indistinguishable after all tie-break rules.
   */
  ambiguous_serials?: StatusEntry["serial"][];
};

/**
 * Canonical DP-1 disambiguation result object (v1).
 * This is what your verifier should emit/log when running DP-1.
 */
export type Dp1DisambiguationResultV1 = {
  /**
   * Unique identifier for the disambiguation run
   * (e.g. ULID or UUID, useful for tracing).
   */
  id: string;

  /**
   * Logical schema URI for the DP-1 result payload.
   * Aligns with the JSON Schema identifier.
   */
  schema_uri: "schema/disambiguation.result/v1";

  /**
   * Version string for this result format (semver: "1.0.0", etc.).
   * This is independent from AP-1 / RP-1 / KP-1 versions but should be
   * kept in sync conceptually.
   */
  version: string;

  /** Context for what was disambiguated and under which profile/policy. */
  context: Dp1Context;

  /**
   * All candidates evaluated, including those that were ineligible.
   * Order is not strictly guaranteed but typically sorted by `score` desc.
   */
  candidates: Dp1Candidate[];

  /** Final DP-1 decision and any tie-break metadata. */
  decision: Dp1Decision;
};

/**
 * Minimal runtime guard you can use where needed.
 */
export const isDp1DisambiguationResultV1 = (
  value: unknown
): value is Dp1DisambiguationResultV1 => {
  return (
    typeof value === "object" &&
    value !== null &&
    (value as { schema_uri?: unknown }).schema_uri ===
      "schema/disambiguation.result/v1"
  );
};

