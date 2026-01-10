// src/verification/verifyOutcome.ts
import { verifyOutcomeOnServer } from "./verifyClient";

export type VerificationOutcome = {
  signature: "verified" | "invalid" | "unknown";
  trust: "trusted" | "untrusted" | "unknown";
  why?: { summary: string; code: string };
  checks?: {
    validity_window: "valid_now" | "not_valid_now" | "unknown";
    revocation: "not_revoked" | "revoked" | "unknown";
  };
};

/**
 * Expo-safe adapter.
 * The server is authoritative for verification + trust artifacts.
 * No Node imports. No Buffer. No fs/path. No pkg/** imports.
 */
export async function verifyOutcomeFromJws(jwsCompact: string): Promise<VerificationOutcome> {
  return verifyOutcomeOnServer(jwsCompact);
}
