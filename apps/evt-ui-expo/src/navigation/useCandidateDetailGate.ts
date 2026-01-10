import { useEffect, useMemo, useRef, useState } from "react";
import {
  gateStateFromOutcome,
  type GateState,
  type VerificationOutcome,
} from "./verificationGateMachine";
import { logGateTransition } from "./devGateLogger";

// IMPORTANT: Adjust this import path to match your monorepo TS path mapping.
import { verifyOutcomeFromJws } from "@/src/verification/verifyOutcome";

type GateResult =
  | { status: "checking" }
  | { status: "ready"; gate: GateState; outcome: VerificationOutcome | null };

type CandidateDetailApiResponse = {
  // Minimal shape aligned with your LOCKED doc (extra fields ignored safely).
  records?: Array<{
    evt_id: string;
    why?: { summary: string; code: string };
    checks?: {
      validity_window: "valid_now" | "not_valid_now" | "unknown";
      revocation: "not_revoked" | "revoked" | "unknown";
    };
    badges?: {
      signature: "verified" | "invalid" | "unknown";
      trust: "trusted" | "untrusted" | "unknown";
    };
    // Not in the locked doc, but required to run local verification.
    // If your API already returns a JWS field, map it to this key.
    jws_compact?: string;
  }>;
};

function outcomeFromApiRecord(record: NonNullable<CandidateDetailApiResponse["records"]>[number]): VerificationOutcome {
  return {
    signature: record.badges?.signature ?? "unknown",
    trust: record.badges?.trust ?? "unknown",
    why: record.why,
    checks: record.checks,
  };
}

export function useCandidateDetailGate(params: {
  apiBaseUrl: string;
  candidateId: string;
  primaryEvtId: string;
}) {
  const { apiBaseUrl, candidateId, primaryEvtId } = params;

  const [state, setState] = useState<GateResult>({ status: "checking" });
  const prevGateRef = useRef<GateState | null>(null);

  const detailUrl = useMemo(() => {
    const base = apiBaseUrl.replace(/\/+$/, "");
    return `${base}/v1/recruiter/candidates/${encodeURIComponent(candidateId)}`;
  }, [apiBaseUrl, candidateId]);

  useEffect(() => {
    let cancelled = false;

    async function run() {
      setState({ status: "checking" });

      try {
        const res = await fetch(detailUrl, { method: "GET" });
        if (!res.ok) {
          // Hard safety: if we can’t fetch detail, lock.
          const gate = "LOCK_UNKNOWN" as const;
          if (!cancelled) {
            logGateTransition({ prev: prevGateRef.current, next: gate });
            prevGateRef.current = gate;
            setState({ status: "ready", gate, outcome: null });
          }
          return;
        }

        const json = (await res.json()) as CandidateDetailApiResponse;
        const record = json.records?.find((r) => r.evt_id === primaryEvtId);

        if (!record) {
          const gate = "LOCK_UNKNOWN" as const;
          if (!cancelled) {
            logGateTransition({ prev: prevGateRef.current, next: gate });
            prevGateRef.current = gate;
            setState({ status: "ready", gate, outcome: null });
          }
          return;
        }

        // Build an outcome from API fields (badges/checks/why) as a baseline.
        // Then, if a JWS is present, we treat verifyOutcomeFromJws(jws) as the source of truth.
        let outcome: VerificationOutcome = outcomeFromApiRecord(record);

        if (record.jws_compact && record.jws_compact.trim().length > 0) {
          // Source of truth per your instruction (gate source is verifyOutcome.ts)
          outcome = await verifyOutcomeFromJws(record.jws_compact);
        }

        const gate = gateStateFromOutcome(outcome);

        if (!cancelled) {
          logGateTransition({
            prev: prevGateRef.current,
            next: gate,
            whyCode: outcome.why?.code,
            signature: outcome.signature,
            trust: outcome.trust,
          });
          prevGateRef.current = gate;
          setState({ status: "ready", gate, outcome });
        }
      } catch {
        const gate = "LOCK_UNKNOWN" as const;
        if (!cancelled) {
          logGateTransition({ prev: prevGateRef.current, next: gate });
          prevGateRef.current = gate;
          setState({ status: "ready", gate, outcome: null });
        }
      }
    }

    void run();
    return () => {
      cancelled = true;
    };
  }, [detailUrl, primaryEvtId]);

  return state;
}
