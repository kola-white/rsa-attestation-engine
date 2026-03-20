import { useMemo } from "react";
import type { CandidateRowSnapshot, VerificationState } from "./recruiterTypes";

export type GateState = "ALLOW" | "LOCK_UNKNOWN" | "LOCK_PENDING";

export type GateResult =
  | { status: "checking" }
  | {
      status: "ready";
      gate: GateState;
      snapshot: CandidateRowSnapshot | null;
      outcome: {
        verification_state?: VerificationState;
      } | null;
    };

function gateFromVerificationState(state?: VerificationState): GateState {
  switch (state) {
    case "verified":
    case "unverified":
      return "ALLOW";
    case "pending":
      return "LOCK_PENDING";
    default:
      return "LOCK_UNKNOWN";
  }
}

export function useCandidateDetailGate(params: {
  apiBaseUrl: string;
  candidateId: string;
  primaryEvtId: string;
  prefetchSnapshot?: CandidateRowSnapshot;
}) {
  const { prefetchSnapshot } = params;

  return useMemo<GateResult>(() => {
    if (!prefetchSnapshot) {
      return {
        status: "ready",
        gate: "LOCK_UNKNOWN",
        snapshot: null,
        outcome: { verification_state: "unknown" },
      };
    }

    const verificationState = prefetchSnapshot.verification?.state;
    const gate = gateFromVerificationState(verificationState);

    return {
      status: "ready",
      gate,
      snapshot: prefetchSnapshot,
      outcome: {
        verification_state: verificationState,
      },
    };
  }, [prefetchSnapshot]);
}