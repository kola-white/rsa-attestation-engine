// Platform files supply the implementation to Metro; this fallback supplies TypeScript's contract.
export async function inspectDocument(_bytes: Uint8Array, _name: string, _mime: string): Promise<void> {
  throw new Error("Document inspection is unavailable on this platform.");
}
