export type VerificationOutcome = {
  signature: "verified" | "invalid" | "unknown";
  trust: "trusted" | "untrusted" | "unknown";
  why?: { summary: string; code: string };
  checks?: {
    validity_window: "valid_now" | "not_valid_now" | "unknown";
    revocation: "not_revoked" | "revoked" | "unknown";
  };
};

export type GateState =
  | "ALLOW"
  | "LOCK_UNKNOWN"
  | "LOCK_BAD_SIGNATURE"
  | "LOCK_REVOKED"
  | "LOCK_NOT_VALID_NOW"
  | "LOCK_UNTRUSTED"
  | "LOCK_POLICY";

type GateUX = {
  title: string;
  message: string; // one sentence
  primaryCta: { text: string; action: "GO_BACK" | "OPEN_FILTERS" | "NONE" };
  secondaryCta?: { text: string; action: "GO_BACK" | "OPEN_FILTERS" | "NONE" };
};

function normalizeWhyCode(code: string | undefined): string {
  return (code ?? "").trim().toUpperCase();
}

function isPolicyOrSchemaCode(code: string | undefined): boolean {
  const c = normalizeWhyCode(code);
  if (!c) return false;
  // Covers: schema/policy/shape failures without inventing new contract fields.
  return (
    c.startsWith("POLICY_") ||
    c.startsWith("SCHEMA_") ||
    c.startsWith("SHAPE_") ||
    c.startsWith("POL_") ||
    c.includes("POLICY") ||
    c.includes("SCHEMA") ||
    c.includes("SHAPE")
  );
}

/**
 * Single deterministic classifier: VerificationOutcome -> GateState
 * Safe default: LOCK_UNKNOWN.
 */
export function gateStateFromOutcome(outcome: VerificationOutcome | null): GateState {
  if (!outcome) return "LOCK_UNKNOWN";

  if (outcome.signature === "invalid") return "LOCK_BAD_SIGNATURE";
  if (outcome.signature === "unknown") return "LOCK_UNKNOWN";

  // signature verified
  const rev = outcome.checks?.revocation ?? "unknown";
  if (rev === "revoked") return "LOCK_REVOKED";

  const vw = outcome.checks?.validity_window ?? "unknown";
  if (vw === "not_valid_now") return "LOCK_NOT_VALID_NOW";

  if (outcome.trust === "trusted") return "ALLOW";
  if (outcome.trust === "unknown") return "LOCK_UNKNOWN";

  // trust untrusted
  if (isPolicyOrSchemaCode(outcome.why?.code)) return "LOCK_POLICY";
  return "LOCK_UNTRUSTED";
}

/**
 * Exported state machine object (UI-ready).
 * This is the single source of truth for gate UX + allowed routes.
 */
export const VERIFICATION_GATE_MACHINE: Readonly<{
  states: Record<
    GateState,
    {
      allowedRoutes: ReadonlyArray<"RecruiterCandidates" | "CandidateDetail" | "RecruiterFilters">;
      ux: GateUX;
    }
  >;
}> = {
  states: {
    ALLOW: {
      allowedRoutes: ["RecruiterCandidates", "CandidateDetail", "RecruiterFilters"],
      ux: {
        title: "Verified",
        message: "This employment record is verified and trusted by your company policy.",
        primaryCta: { text: "Continue", action: "NONE" },
      },
    },

    LOCK_UNKNOWN: {
      allowedRoutes: ["RecruiterCandidates", "CandidateDetail", "RecruiterFilters"],
      ux: {
        title: "Verification unavailable",
        message: "This record can’t be verified right now, so details are locked for safety.",
        primaryCta: { text: "Back to candidates", action: "GO_BACK" },
        secondaryCta: { text: "Adjust filters", action: "OPEN_FILTERS" },
      },
    },

    LOCK_BAD_SIGNATURE: {
      allowedRoutes: ["RecruiterCandidates", "CandidateDetail", "RecruiterFilters"],
      ux: {
        title: "Invalid signature",
        message: "This record failed signature verification and cannot be trusted.",
        primaryCta: { text: "Back to candidates", action: "GO_BACK" },
        secondaryCta: { text: "Adjust filters", action: "OPEN_FILTERS" },
      },
    },

    LOCK_REVOKED: {
      allowedRoutes: ["RecruiterCandidates", "CandidateDetail", "RecruiterFilters"],
      ux: {
        title: "Revoked",
        message: "This employment record was revoked and cannot be used for verification.",
        primaryCta: { text: "Back to candidates", action: "GO_BACK" },
        secondaryCta: { text: "Adjust filters", action: "OPEN_FILTERS" },
      },
    },

    LOCK_NOT_VALID_NOW: {
      allowedRoutes: ["RecruiterCandidates", "CandidateDetail", "RecruiterFilters"],
      ux: {
        title: "Not valid now",
        message: "This record is outside its validity window, so details are locked.",
        primaryCta: { text: "Back to candidates", action: "GO_BACK" },
        secondaryCta: { text: "Adjust filters", action: "OPEN_FILTERS" },
      },
    },

    LOCK_UNTRUSTED: {
      allowedRoutes: ["RecruiterCandidates", "CandidateDetail", "RecruiterFilters"],
      ux: {
        title: "Untrusted issuer",
        message: "This issuer is not trusted, so details are locked until policy changes.",
        primaryCta: { text: "Back to candidates", action: "GO_BACK" },
        secondaryCta: { text: "Adjust filters", action: "OPEN_FILTERS" },
      },
    },

    LOCK_POLICY: {
      allowedRoutes: ["RecruiterCandidates", "CandidateDetail", "RecruiterFilters"],
      ux: {
        title: "Blocked by policy",
        message: "Your company policy blocks this record, so details are locked.",
        primaryCta: { text: "Back to candidates", action: "GO_BACK" },
        secondaryCta: { text: "Adjust filters", action: "OPEN_FILTERS" },
      },
    },
  },
};
