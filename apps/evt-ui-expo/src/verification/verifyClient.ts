// src/verification/verifyClient.ts
import { API_BASE_URL } from "../config/auth";
import type { VerificationOutcome } from "./verifyOutcome";

function devLog(msg: string, meta?: Record<string, unknown>) {
  if (process.env.NODE_ENV === "production") return;
  console.log(`[verifyClient] ${msg}`, meta ?? "");
}

export async function verifyOutcomeOnServer(jwsCompact: string): Promise<VerificationOutcome> {
  if (!API_BASE_URL) {
    return {
      signature: "unknown",
      trust: "unknown",
      why: { summary: "Missing API_BASE_URL", code: "MISSING_API_BASE_URL" },
      checks: { validity_window: "unknown", revocation: "unknown" },
    };
  }

  devLog("request:start", { jws_len: jwsCompact.length });

  const res = await fetch(`${API_BASE_URL}/v1/verification/outcome`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ jwsCompact }),
  });

  if (!res.ok) {
    const text = await res.text().catch(() => "");
    devLog("request:fail", { status: res.status });
    return {
      signature: "unknown",
      trust: "unknown",
      why: {
        summary: "Verification request failed.",
        code: `HTTP_${res.status}`,
      },
      checks: { validity_window: "unknown", revocation: "unknown" },
    };
  }

  const out = (await res.json()) as VerificationOutcome;

  devLog("request:ok", {
    signature: out.signature,
    trust: out.trust,
    why: out.why?.code ?? "",
  });

  return out;
}
