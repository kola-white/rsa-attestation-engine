// pkg/verification/outcome.ts

export type VerificationOutcome = {
  signature: "verified" | "invalid" | "unknown";
  trust: "trusted" | "untrusted" | "unknown";
  why?: { summary: string; code: string };
  checks?: {
    validity_window: "valid_now" | "not_valid_now" | "unknown";
    revocation: "not_revoked" | "revoked" | "unknown";
  };
};
