import type { GateState, VerificationOutcome } from "./verificationGateMachine";

function isDev(): boolean {
  // Expo supports __DEV__ in Metro
  return typeof __DEV__ !== "undefined" ? __DEV__ : false;
}

export function logGateTransition(params: {
  prev: GateState | null;
  next: GateState;
  whyCode?: string;
  signature?: VerificationOutcome["signature"];
  trust?: VerificationOutcome["trust"];
}): void {
  if (!isDev()) return;

  const prev = params.prev ?? "∅";
  const code = (params.whyCode ?? "").trim();
  const meta = [
    `sig=${params.signature ?? "?"}`,
    `trust=${params.trust ?? "?"}`,
    code ? `why=${code}` : undefined,
  ]
    .filter(Boolean)
    .join(" ");

  // Never log JWS. Never log secrets. Only state + coarse metadata.
  // eslint-disable-next-line no-console
  console.log(`[nav-lock] ${prev} -> ${params.next}${meta ? ` (${meta})` : ""}`);
}
