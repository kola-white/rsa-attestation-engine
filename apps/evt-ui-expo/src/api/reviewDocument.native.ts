import * as FileSystem from "expo-file-system/legacy";
import { Platform, Share } from "react-native";

function base64(bytes: Uint8Array): string {
  const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
  const chunks: string[] = [];
  let chunk = "";
  for (let i = 0; i < bytes.length; i += 3) {
    const n = (bytes[i] << 16) | ((bytes[i + 1] ?? 0) << 8) | (bytes[i + 2] ?? 0);
    chunk += alphabet[n >>> 18] + alphabet[(n >>> 12) & 63] + (i + 1 < bytes.length ? alphabet[(n >>> 6) & 63] : "=") + (i + 2 < bytes.length ? alphabet[n & 63] : "=");
    if (chunk.length >= 8192) { chunks.push(chunk); chunk = ""; }
  }
  return chunks.join("") + chunk;
}

export async function inspectDocument(bytes: Uint8Array, name: string, mime: string): Promise<void> {
  const safeName = name.replace(/[\\/\x00-\x1f]/g, "_") || "evidence";
  const encoded = base64(bytes);
  if (Platform.OS === "android") {
    // Existing SAF lets the reviewer choose a private destination without a new sharing dependency.
    const permission = await FileSystem.StorageAccessFramework.requestDirectoryPermissionsAsync();
    if (!permission.granted) throw new Error("Save cancelled.");
    const safeType = ["application/pdf", "image/png", "image/jpeg"].includes(mime) ? mime : "application/octet-stream";
    const uri = await FileSystem.StorageAccessFramework.createFileAsync(permission.directoryUri, safeName, safeType);
    await FileSystem.writeAsStringAsync(uri, encoded, { encoding: FileSystem.EncodingType.Base64 });
    // User opens the explicitly saved document in their viewer. Never claim that saving proves inspection.
    return;
  }
  if (!FileSystem.cacheDirectory) throw new Error("Document storage unavailable.");
  const directory = FileSystem.cacheDirectory + "review-" + Date.now() + "-" + Math.random().toString(36).slice(2) + "/";
  await FileSystem.makeDirectoryAsync(directory);
  try {
    const uri = directory + safeName;
    await FileSystem.writeAsStringAsync(uri, encoded, { encoding: FileSystem.EncodingType.Base64 });
    const result = await Share.share({ url: uri, title: "Evidence document" });
    if (result.action === Share.dismissedAction) throw new Error("Open cancelled.");
  } finally { await FileSystem.deleteAsync(directory, { idempotent: true }); }
}
