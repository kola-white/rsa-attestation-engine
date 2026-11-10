export async function inspectDocument(bytes: Uint8Array, name: string, mime: string): Promise<void> {
  // Download authenticated bytes, never an authenticated URL. A download is not an inspection assertion.
  const safeType = ["application/pdf", "image/png", "image/jpeg"].includes(mime) ? mime : "application/octet-stream";
  const url = URL.createObjectURL(new Blob([new Uint8Array(bytes).buffer], { type: safeType }));
  const link = document.createElement("a");
  link.href = url; link.download = name.replace(/[\\/\x00-\x1f]/g, "_") || "evidence";
  link.rel = "noopener noreferrer";
  document.body.appendChild(link);
  try { link.click(); } finally { link.remove(); setTimeout(() => URL.revokeObjectURL(url), 60000); }
}
