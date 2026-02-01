import { useColorScheme } from "react-native";
import React, { useEffect, useLayoutEffect, useMemo, useState } from "react";
import { useAuth } from "@/src/auth/AuthContext";
import { useNavigation } from "@react-navigation/native";
import type { NativeStackNavigationProp } from "@react-navigation/native-stack";
import type { AppStackParamList } from "@/src/navigation/MainAppNavigator";
import {
  View,
  Text,
  ScrollView,
  Pressable,
  Alert,
  ActivityIndicator,
  ActionSheetIOS,
  Platform,
} from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import * as DocumentPicker from "expo-document-picker";
import * as ImagePicker from "expo-image-picker";
import * as FileSystem from "expo-file-system/legacy";
import * as Crypto from "expo-crypto";

console.log("[HRReview] module loaded");

type EmployerResponseType =
  | "FULL_MATCH"
  | "PARTIAL_MATCH"
  | "REJECTED_NO_RECORD"
  | "REJECTED_POLICY";

type HRQueueRow = {
  request_id: string;
  status: string;
  claim_snapshot: Record<string, unknown>;
  created_at: string;
  updated_at: string;
};

type HRQueueResp = { items: HRQueueRow[] };

type HRGetResp = HRQueueRow & {
  version: number;
  employer_response_type?: EmployerResponseType | null;
};


type DisplayClaim = {
  employer: string;
  job_title: string;
  start_mm_yyyy: string;
  end_mm_yyyy: string | null;
};

function toDisplayClaim(snap: Record<string, unknown>): DisplayClaim {
  const employer = typeof snap.employer === "string" ? snap.employer : "";
  const jobTitle = typeof snap.job_title === "string" ? snap.job_title : "";
  const start = typeof snap.start_mm_yyyy === "string" ? snap.start_mm_yyyy : "";
  const endRaw = snap.end_mm_yyyy;
  const end =
    endRaw === null ? null : typeof endRaw === "string" ? endRaw : null;

  return {
    employer,
    job_title: jobTitle,
    start_mm_yyyy: start,
    end_mm_yyyy: end,
  };
  
}


type HRNav = NativeStackNavigationProp<AppStackParamList, "HRReview">;

type CaseStatus = "PENDING" | "ATTESTED" | "REJECTED";

function statusToCaseStatus(backendStatus: string): CaseStatus {
  if (backendStatus === "ATTESTED") return "ATTESTED";
  if (backendStatus === "REJECTED") return "REJECTED";
  return "PENDING";
}

function rejectionReasonLabel(
  employerResponseType?: EmployerResponseType | null
): string | null {
  if (employerResponseType === "REJECTED_NO_RECORD") return "No record found";
  if (employerResponseType === "REJECTED_POLICY") return "Policy restriction";
  return null; // ✅ no inference, no default
}

function formatRelativeOrDate(iso: string): string {
  // keep it dead simple: show the raw date for now (no hand-wavy “2h ago”)
  // You can swap this later for a real relative formatter.
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return d.toLocaleString();
}

type EvidenceInitResponse = {
  caseId: string;
  checkId: string;
  expiresAt: string;
  uploads: Array<{
    name: string;
    mimeType: string;
    size: number;
    storageKey: string;
    method: "PUT";
    url: string;
    headers: Record<string, string>;
  }>;
};

type UploadState =
  | { status: "idle" }
  | { status: "picking" }
  | { status: "initing" }
  | { status: "uploading" }
  | { status: "done"; storageKey: string }
  | { status: "error"; message: string };

type EvidenceFile = {
  uri: string; // MUST be file:// for FileSystem.uploadAsync
  name: string; // deterministic
  mimeType: "application/pdf" | "image/jpeg" | "image/png";
  size: number; // bytes
};

// Demo constants
const CASE_ID = "EVT-10324";
const CHECK_ID = "employment.company_and_dates";

// Must be reachable from your phone (NOT 127.0.0.1).
const API_BASE_URL = process.env.EXPO_PUBLIC_EVT_API_BASE_URL;
console.log("API_BASE_URL runtime =", process.env.EXPO_PUBLIC_EVT_API_BASE_URL);

// Policy (client-enforced to avoid wasted init/presign)
const MAX_FILE_BYTES = 5_000_000 as const;
const ALLOWED_MIME_TYPES = [
  "application/pdf",
  "image/jpeg",
  "image/png",
] as const;

function displayFileNameFromKey(key: string): string {
  const parts = key.split("/");
  return parts[parts.length - 1] ?? key;
}

function shortKey(key: string, head = 22, tail = 10): string {
  if (key.length <= head + tail + 3) return key;
  return `${key.slice(0, head)}…${key.slice(-tail)}`;
}

function inferExtensionFromMime(
  mimeType: EvidenceFile["mimeType"]
): "pdf" | "jpg" | "png" {
  switch (mimeType) {
    case "application/pdf":
      return "pdf";
    case "image/jpeg":
      return "jpg";
    case "image/png":
      return "png";
  }
}

function sanitizeFilename(name: string): string {
  // Deterministic, filesystem-safe, Spaces-safe-ish.
  // (You already see spaces converted to underscores in your server-generated key.)
  const trimmed = name.trim();
  // replace path separators and control chars, normalize spaces
  const replaced = trimmed
    .replace(/[\/\\]+/g, "_")
    .replace(/[\u0000-\u001F\u007F]+/g, "")
    .replace(/\s+/g, "_");

  // Avoid empty
  return replaced.length > 0 ? replaced : `evidence-${Date.now()}`;
}

function guessMimeTypeFromName(
  filename: string
): EvidenceFile["mimeType"] | null {
  const lower = filename.toLowerCase();
  if (lower.endsWith(".pdf")) return "application/pdf";
  if (lower.endsWith(".jpg") || lower.endsWith(".jpeg")) return "image/jpeg";
  if (lower.endsWith(".png")) return "image/png";
  return null;
}

function ensureFileUri(uri: string): string {
  // Expo pickers should hand back file://, but we defensively normalize.
  if (uri.startsWith("file://")) return uri;
  // Some platforms might return content:// which FileSystem.uploadAsync can't always PUT directly.
  // In that case, we require copy to cache first.
  return uri;
}

async function getFileSizeBytes(uri: string): Promise<number> {
  const info = await FileSystem.getInfoAsync(uri);
  if (!info.exists) throw new Error("File does not exist at provided URI.");
  const size = (info as any).size;
  if (typeof size !== "number" || size <= 0)
    throw new Error("Missing or invalid file size.");
  return size;
}

/**
 * Base64 → Uint8Array (no atob / Buffer dependencies).
 * Input is assumed to be a valid base64 string without data URI prefix.
 */
function base64ToUint8Array(b64: string): Uint8Array {
  const clean = b64.replace(/[\r\n\s]/g, "");
  const len = clean.length;
  if (len % 4 !== 0) {
    throw new Error("Invalid base64 input.");
  }

  const lookup =
    "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";

  const padding =
    clean.endsWith("==") ? 2 : clean.endsWith("=") ? 1 : 0;
  const bytesLen = (len * 3) / 4 - padding;
  const bytes = new Uint8Array(bytesLen);

  let byteIndex = 0;

  for (let i = 0; i < len; i += 4) {
    const c1 = lookup.indexOf(clean[i]);
    const c2 = lookup.indexOf(clean[i + 1]);
    const c3 = lookup.indexOf(clean[i + 2]);
    const c4 = lookup.indexOf(clean[i + 3]);

    const triple =
      (c1 << 18) |
      (c2 << 12) |
      ((c3 & 63) << 6) |
      (c4 & 63);

    if (byteIndex < bytesLen) {
      bytes[byteIndex++] = (triple >> 16) & 0xff;
    }
    if (byteIndex < bytesLen) {
      bytes[byteIndex++] = (triple >> 8) & 0xff;
    }
    if (byteIndex < bytesLen) {
      bytes[byteIndex++] = triple & 0xff;
    }
  }

  return bytes;
}

function arrayBufferToHex(buffer: ArrayBuffer): string {
  const bytes = new Uint8Array(buffer);
  let hex = "";
  for (let i = 0; i < bytes.length; i++) {
    hex += bytes[i].toString(16).padStart(2, "0");
  }
  return hex;
}

/**
 * Evidence digest helper:
 * - Reads full file bytes via FileSystem.readAsStringAsync(uri, Base64)
 * - Decodes base64 to Uint8Array
 * - Hashes with SHA-256 via expo-crypto Crypto.digest
 * - Returns lowercase hex string
 *
 * Client-side digest is:
 * - Session-local only
 * - NEVER sent to the API
 * - Used strictly for UX hard dedupe per Evidence Hashing Contract.
 */
async function computeEvidenceDigestHex(uri: string): Promise<string> {
  const base64 = await FileSystem.readAsStringAsync(uri, {
    encoding: FileSystem.EncodingType.Base64,
  });

  // TypedArray that native expo-crypto expects
  const bytes = base64ToUint8Array(base64);

  // Runtime needs a TypedArray; TS types are too picky about BufferSource,
  // so we bypass them with a cast on Crypto and leave `bytes` as-is.
  const digestBuffer = await (Crypto as any).digest(
    Crypto.CryptoDigestAlgorithm.SHA256,
    bytes
  );

  return arrayBufferToHex(digestBuffer).toLowerCase();
}

/**
 * Deterministic normalization helper:
 * - ensures we have file:// uri we can upload from
 * - name: prefer provided; else evidence-<timestamp>.<ext>
 * - mimeType: prefer provided; else infer from extension; else reject
 * - size: prefer provided; else stat the file
 * - enforces allowlist + max bytes before init
 */
async function normalizeEvidenceFile(input: {
  uri: string;
  name?: string | null;
  mimeType?: string | null;
  size?: number | null;
}): Promise<EvidenceFile> {
  const rawUri = ensureFileUri(input.uri);

  // 1) Determine mimeType
  let mimeType =
    (input.mimeType as EvidenceFile["mimeType"] | undefined) ??
    (input.name ? guessMimeTypeFromName(input.name) : null);

  if (!mimeType) {
    // attempt infer from uri path as a last resort
    const uriLower = rawUri.toLowerCase();
    if (uriLower.endsWith(".pdf")) mimeType = "application/pdf";
    else if (uriLower.endsWith(".jpg") || uriLower.endsWith(".jpeg"))
      mimeType = "image/jpeg";
    else if (uriLower.endsWith(".png")) mimeType = "image/png";
  }

  if (!mimeType || !ALLOWED_MIME_TYPES.includes(mimeType as any)) {
    throw new Error("Unsupported file type. Use PDF, JPG, or PNG.");
  }

  // 2) Determine extension and name
  const ext = inferExtensionFromMime(mimeType);
  const baseName =
    input.name && input.name.trim().length > 0
      ? sanitizeFilename(input.name)
      : `evidence-${Date.now()}.${ext}`;

  // Ensure correct extension matches mimeType deterministically
  const finalName = baseName.toLowerCase().endsWith(`.${ext}`)
    ? baseName
    : `${baseName.replace(/\.[^/.]+$/, "")}.${ext}`;

  // 3) Determine size
  const size =
    typeof input.size === "number" && input.size > 0
      ? input.size
      : await getFileSizeBytes(rawUri);

  // 4) Enforce client-side policy before init
  if (size > MAX_FILE_BYTES) {
    throw new Error(
      `File is too large. Max is ${Math.floor(MAX_FILE_BYTES / 1_000_000)}MB.`
    );
  }

  // 5) Ensure we have a file:// source for PUT uploads
  // DocumentPicker with copyToCacheDirectory gives file://.
  // ImagePicker generally gives file:// as well. If we ever see non-file://,
  // we copy it deterministically into cache.
  let uploadUri = rawUri;
  if (!uploadUri.startsWith("file://")) {
    const dest = `${FileSystem.cacheDirectory}evidence-${Date.now()}-${finalName}`;
    await FileSystem.copyAsync({ from: uploadUri, to: dest });
    uploadUri = dest;
  }

  return {
    uri: uploadUri,
    name: finalName,
    mimeType,
    size,
  };
}

/**
 * Permission-on-tap-only ImagePicker entrypoint.
 * - Requests Media Library permission ONLY when user taps "Choose photo"
 * - If denied: throws with a user-friendly message
 */
async function pickPhotoEvidence(): Promise<EvidenceFile | null> {
  // Permission only on tap
  const perm = await ImagePicker.requestMediaLibraryPermissionsAsync();
  if (!perm.granted) {
    Alert.alert(
      "Photos Permission Required",
      "Enable Photos access:\nSettings → Privacy & Security → Photos → Allow this app."
    );
    return null;
  }

  const res = await ImagePicker.launchImageLibraryAsync({
    mediaTypes: ['images', 'livePhotos'],
    allowsMultipleSelection: false,
    quality: 1,
    // Don't force base64 (we upload bytes via FileSystem.uploadAsync)
    base64: false,
    exif: false,
  });

  if (res.canceled) return null;

  const asset = res.assets?.[0];
  
  const mime = asset.mimeType ?? "image/jpeg";

  // 🟥 HEIC / HEIF block (Live Photos on newer iPhones)
  if (mime.includes("heic") || mime.includes("heif")) {
    Alert.alert(
      "Unsupported Format",
      "HEIC / Live Photos are not supported yet.\nPlease choose a regular photo (JPG or PNG)."
    );
    return null;
  }

  if (!asset?.uri) Alert.alert("Image picker returned no image.");

  // Normalize:
  // - For iOS/Android, asset.fileName and asset.mimeType may or may not exist
  const normalized = await normalizeEvidenceFile({
    uri: asset.uri,
    name: (asset as any).fileName ?? null,
    mimeType: (asset as any).mimeType ?? null,
    size:
      typeof (asset as any).fileSize === "number"
        ? (asset as any).fileSize
        : null,
  });

  // Additional guard: ImagePicker should only allow images, but enforce minimum allowlist
  if (
    normalized.mimeType !== "image/jpeg" &&
    normalized.mimeType !== "image/png"
  ) {
    Alert.alert("Unsupported photo type. Use JPG or PNG.");
  }

  return normalized;
}

/**
 * Existing DocumentPicker path, normalized to EvidenceFile for reuse.
 */
async function pickDocumentEvidence(): Promise<EvidenceFile | null> {
  const picked = await DocumentPicker.getDocumentAsync({
    type: ["application/pdf", "image/jpeg", "image/png"],
    copyToCacheDirectory: true,
    multiple: false,
  });

  if (picked.canceled) return null;

  const asset = picked.assets?.[0];
  if (!asset?.uri) throw new Error("Document picker returned no file.");

  // DocumentPicker usually has name; mimeType may be missing.
  const normalized = await normalizeEvidenceFile({
    uri: asset.uri,
    name: asset.name ?? null,
    mimeType:
      asset.mimeType ??
      (asset.name ? guessMimeTypeFromName(asset.name) : null),
    size: typeof asset.size === "number" ? asset.size : null,
  });

  return normalized;
}

/**
 * Reuses your current init+PUT logic exactly:
 * - POST evidence:init with {files:[{name,mimeType,size}]}
 * - PUT to initJson.uploads[0].url with initJson.uploads[0].headers EXACTLY
 * - DOES NOT call evidence:complete (server returns 501 today)
 */
async function uploadViaInitAndPut(
  evidence: EvidenceFile
): Promise<{ storageKey: string }> {
  if (!API_BASE_URL) {
    throw new Error(
      "Missing EXPO_PUBLIC_EVT_API_BASE_URL (device cannot reach 127.0.0.1)."
    );
  }

  // Extra defensive checks (deterministic policy)
  if (!ALLOWED_MIME_TYPES.includes(evidence.mimeType as any)) {
    throw new Error("Unsupported file type. Use PDF, JPG, or PNG.");
  }
  if (!evidence.size || evidence.size <= 0)
    throw new Error("Missing or invalid file size.");
  if (evidence.size > MAX_FILE_BYTES) {
    throw new Error(
      `File is too large. Max is ${Math.floor(MAX_FILE_BYTES / 1_000_000)}MB.`
    );
  }

  const initUrl = `${API_BASE_URL}/v1/cases/${encodeURIComponent(
    CASE_ID
  )}/checks/${encodeURIComponent(CHECK_ID)}/evidence:init`;

  const initRes = await fetch(initUrl, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      caseId: CASE_ID, // keep as-is (your server ignores/accepts)
      checkId: CHECK_ID, // keep as-is (your server ignores/accepts)
      files: [
        { name: evidence.name, mimeType: evidence.mimeType, size: evidence.size },
      ],
    }),
  });

  if (!initRes.ok) {
    const text = await initRes.text();
    throw new Error(`Init failed (${initRes.status}): ${text}`);
  }

  const initJson = (await initRes.json()) as EvidenceInitResponse;
  console.log("evidence:init response =", JSON.stringify(initJson, null, 2));

  const upload = initJson.uploads?.[0];
  if (!upload?.url || !upload?.headers || !upload.storageKey) {
    throw new Error("Init response missing upload url/headers/storageKey.");
  }

  // IMPORTANT CONTRACT ENFORCEMENT:
  // PUT must set required headers EXACTLY.
  // Your legacy response uses `headers` (not `requiredHeaders`).
  const putRes = await FileSystem.uploadAsync(upload.url, evidence.uri, {
    httpMethod: "PUT",
    headers: upload.headers,
    uploadType: FileSystem.FileSystemUploadType.BINARY_CONTENT,
  });

  if (putRes.status !== 200 && putRes.status !== 204) {
    throw new Error(`PUT failed (${putRes.status}). Body: ${putRes.body ?? ""}`);
  }

  // evidence:complete intentionally disabled until server implemented.
  return { storageKey: upload.storageKey };
}

export const HRReviewScreenSettingsStyle = () => {
  const scheme = useColorScheme(); // "dark" | "light" | null
  const isDark = scheme === "dark";
  console.log("[HRReview] render");
  const insets = useSafeAreaInsets();

  const [uploadState, setUploadState] = useState<UploadState>({
    status: "idle",
  });

  // Session-local set of evidence digests (lowercase hex).
  // Populated ONLY after a successful upload to avoid blocking retries.
  const [evidenceDigests, setEvidenceDigests] = useState<string[]>([]);

  const canUpload = useMemo(() => {
    return (
      uploadState.status !== "picking" &&
      uploadState.status !== "initing" &&
      uploadState.status !== "uploading"
    );
  }, [uploadState.status]);

  async function pickAndUploadDocument() {
    try {
      if (!API_BASE_URL) {
        setUploadState({
          status: "error",
          message:
            "Missing EXPO_PUBLIC_EVT_API_BASE_URL (device cannot reach 127.0.0.1).",
        });
        return;
      }

      setUploadState({ status: "picking" });

      const evidence = await pickDocumentEvidence();
      if (!evidence) {
        setUploadState({ status: "idle" });
        return;
      }

      // Compute client-side digest for hard dedupe (session-local only).
      setUploadState({ status: "initing" });
      const digestHex = await computeEvidenceDigestHex(evidence.uri);

      if (evidenceDigests.includes(digestHex)) {
        // Hard dedupe: do NOT upload, do NOT add second copy.
        setUploadState({ status: "idle" });
        Alert.alert(
          "Already attached",
          "This file is already attached as evidence. No additional upload needed."
        );
        return;
      }

      const { storageKey } = await uploadViaInitAndPut(evidence);

      // Only record digest after successful upload, so failed uploads can be retried.
      setEvidenceDigests((prev) => [...prev, digestHex]);
      setUploadState({ status: "done", storageKey });
    } catch (err: any) {
      const msg = String(err?.message ?? err ?? "");
      const friendly = msg.includes("Network request failed")
        ? "Cannot reach server. Check Wi-Fi/LAN URL."
        : msg.toLowerCase().includes("timed out")
        ? "Request timed out. Check server reachability or try again."
        : msg;

      setUploadState({ status: "error", message: friendly });
    }
  }

  async function pickAndUploadPhoto() {
    try {
      if (!API_BASE_URL) {
        setUploadState({
          status: "error",
          message:
            "Missing EXPO_PUBLIC_EVT_API_BASE_URL (device cannot reach 127.0.0.1).",
        });
        return;
      }

      setUploadState({ status: "picking" });

      const evidence = await pickPhotoEvidence();
      if (!evidence) {
        setUploadState({ status: "idle" });
        return;
      }

      // Compute client-side digest for hard dedupe (session-local only).
      setUploadState({ status: "initing" });
      const digestHex = await computeEvidenceDigestHex(evidence.uri);

      if (evidenceDigests.includes(digestHex)) {
        // Hard dedupe: do NOT upload, do NOT add second copy.
        setUploadState({ status: "idle" });
        Alert.alert(
          "Already attached",
          "This file is already attached as evidence. No additional upload needed."
        );
        return;
      }

      const { storageKey } = await uploadViaInitAndPut(evidence);

      // Only record digest after successful upload, so failed uploads can be retried.
      setEvidenceDigests((prev) => [...prev, digestHex]);
      setUploadState({ status: "done", storageKey });
    } catch (err: any) {
      const msg = String(err?.message ?? err ?? "");
      const friendly = msg.includes("Photo access denied")
        ? msg
        : msg.includes("Network request failed")
        ? "Cannot reach server. Check Wi-Fi/LAN URL."
        : msg.toLowerCase().includes("timed out")
        ? "Request timed out. Check server reachability or try again."
        : msg;

      setUploadState({ status: "error", message: friendly });
    }
  }

  const handleUploadEvidencePress = () => {
    if (!API_BASE_URL) {
      setUploadState({
        status: "error",
        message:
          "Missing EXPO_PUBLIC_EVT_API_BASE_URL (device cannot reach 127.0.0.1).",
      });
      return;
    }

    if (!canUpload) {
      return;
    }

    if (Platform.OS === "ios") {
      ActionSheetIOS.showActionSheetWithOptions(
        {
          title: "Upload evidence",
          message: "Choose a source",
          options: ["Cancel", "Photo library", "Files"],
          cancelButtonIndex: 0,
        },
        (buttonIndex) => {
          if (buttonIndex === 1) {
            void pickAndUploadPhoto();
          } else if (buttonIndex === 2) {
            void pickAndUploadDocument();
          }
        }
      );
    } else {
      Alert.alert("Upload evidence", undefined, [
        {
          text: "Photo library",
          onPress: () => void pickAndUploadPhoto(),
        },
        {
          text: "Files",
          onPress: () => void pickAndUploadDocument(),
        },
        {
          text: "Cancel",
          style: "cancel",
        },
      ]);
    }
  };

  // Auth + Navigation setup logic simple login/logout + header
  const { logout, accessToken } = useAuth();
  const navigation = useNavigation<HRNav>();

  const [loading, setLoading] = useState(true);
  const [queue, setQueue] = useState<HRQueueRow[]>([]);
  const [selected, setSelected] = useState<HRGetResp | null>(null);
  const employerId = "emp_demo_001"; // demo constant for now

  const displayClaim = useMemo(() => {
  const snap = (selected?.claim_snapshot ?? {}) as Record<string, unknown>;
  return toDisplayClaim(snap);
}, [selected]);

const status: CaseStatus = useMemo(() => {
  return selected ? statusToCaseStatus(selected.status) : "PENDING";
}, [selected?.status]);

const rejectLabel = useMemo(() => {
  if (!selected) return null;
  if (selected.status !== "REJECTED") return null;
  return rejectionReasonLabel(selected.employer_response_type);
}, [selected?.status, selected?.employer_response_type]);

const isEmpty = !loading && queue.length === 0;

  const submittedLabel = useMemo(() => {
    // use updated_at as the safest “this is recent” value for now
    return selected?.updated_at ? formatRelativeOrDate(selected.updated_at) : "";
  }, [selected]);

const handleChooseRequest = () => {
  if (!queue.length) return;

  const options = ["Cancel", ...queue.map((q) => q.request_id)];

  if (Platform.OS === "ios") {
    ActionSheetIOS.showActionSheetWithOptions(
      { title: "Select request", options, cancelButtonIndex: 0 },
      (idx) => {
        if (idx <= 0) return;
        const chosen = queue[idx - 1];
        void (async () => {
          setLoading(true);
          try {
            const d = await fetchDetail(chosen.request_id);
            setSelected(d);
          } finally {
            setLoading(false);
          }
        })();
      }
    );
  } else {
    Alert.alert(
      "Select request",
      undefined,
      [
        ...queue.map((q) => ({
          text: q.request_id,
          onPress: () => void (async () => {
            setLoading(true);
            try {
              const d = await fetchDetail(q.request_id);
              setSelected(d);
            } finally {
              setLoading(false);
            }
          })(),
        })),
        { text: "Cancel", style: "cancel" },
      ]
    );
  }
};

async function fetchDetail(requestId: string): Promise<HRGetResp> {
  if (!API_BASE_URL) throw new Error("Missing EXPO_PUBLIC_EVT_API_BASE_URL");
  if (!accessToken) throw new Error("Missing access token (sign in again)");

  const dRes = await fetch(
    `${API_BASE_URL}/v1/employer/requests/${encodeURIComponent(
      requestId
    )}?employer_id=${encodeURIComponent(employerId)}`,
    {
      headers: {
        Accept: "application/json",
        Authorization: `Bearer ${accessToken}`,
      },
    }
  );

  const text = await dRes.text();
  if (!dRes.ok) {
    throw new Error(`Detail failed (${dRes.status}): ${text}`);
  }

  return (text ? JSON.parse(text) : null) as HRGetResp;
}

  const [attesting, setAttesting] = useState<EmployerResponseType | null>(null);

  async function attest(responseType: EmployerResponseType) {

  if (attesting) return;
  try {
    if (!selected) return Alert.alert("No request selected.");
    if (!API_BASE_URL) return Alert.alert("Config error", "Missing API base URL.");
    if (!accessToken) return Alert.alert("Auth error", "Missing access token.");
    if (!selected) return Alert.alert("No request selected.");
    if (queue.length === 0) return Alert.alert("No requests waiting for review.");

    setAttesting(responseType);

    const requestId = selected.request_id;

    const res = await fetch(
      `${API_BASE_URL}/v1/employer/requests/${encodeURIComponent(requestId)}/attest`,
      {
        method: "POST",
        headers: {
          Accept: "application/json",
          "Content-Type": "application/json",
          Authorization: `Bearer ${accessToken}`,
        },
        body: JSON.stringify({
          employer_id: employerId,
          response_type: responseType,
          response_body: {},                 // keep empty for demo
        }),
      }
    );

    const text = await res.text();
    if (!res.ok) {
      let err = text || `HTTP ${res.status}`;
      try {
        const j = JSON.parse(text);
        err = j?.error ?? err;
      } catch {}
      Alert.alert("Attest failed", err);
      return;
    }

    // show result
    const j = text ? JSON.parse(text) : null;
    Alert.alert("Attested", `Status: ${j?.status ?? "ATTESTED"}`);

    // ✅ Optional refresh detail after attest (CONCRETE)
    const refreshed = await fetchDetail(requestId);
    setSelected(refreshed);

    // (Optional) also refresh queue so status changes reflect in list if you render it
    // await refreshQueue();  // only if you have/need it
  } catch (e: any) {
    Alert.alert("Attest failed", String(e?.message ?? e));
  } finally {
    setAttesting(null);
  }
}

  useLayoutEffect(() => {
  navigation.setOptions({
    title: "HR Review",
    headerLargeTitle: true,

    // ✅ make the header match the screen
    headerStyle: { backgroundColor: isDark ? "#18181b" : "#ffffff" }, // zinc-900 / white
    headerShadowVisible: true,
    headerTintColor: isDark ? "#fafafa" : "#18181b", // text/icons in header

    headerRight: () => (
      <View className="flex-row items-center gap-2">
        <Pressable
          onPress={logout}
          accessibilityRole="button"
          accessibilityLabel="Sign out"
          hitSlop={10}
          className="px-3 py-1 rounded-lg bg-zinc-200 dark:bg-zinc-700"
          style={({ pressed }) => [
            {
              opacity: pressed ? 0.8 : 1,
              paddingHorizontal: 12,
              paddingVertical: 6,
              borderRadius: 10,
              backgroundColor: isDark ? "#18181b" : "#e4e4e7", // zinc-900 / zinc-200
            },
          ]}
        >
          <Text 
          style={{
          fontSize: 14,
          fontWeight: "600",
          color: isDark ? "#fafafa" : "#18181b", // zinc-900
          }}>
            Sign out
          </Text>
        </Pressable>
      </View>
    ),
  });
}, [navigation, logout]);

useEffect(() => {
  let cancelled = false;

  async function run() {
    try {
      if (!API_BASE_URL) throw new Error("Missing EXPO_PUBLIC_EVT_API_BASE_URL");
      if (!accessToken) throw new Error("Missing access token (sign in again)");

      setLoading(true);

      const qRes = await fetch(
        `${API_BASE_URL}/v1/employer/requests?employer_id=${encodeURIComponent(employerId)}`,
        {
          headers: {
            Accept: "application/json",
            Authorization: `Bearer ${accessToken}`,
          },
        }
      );

      if (!qRes.ok) {
        const t = await qRes.text();
        throw new Error(`Queue failed (${qRes.status}): ${t}`);
      }

      const qJson = (await qRes.json()) as HRQueueResp;
      if (cancelled) return;

      setQueue(qJson.items ?? []);

      const first = qJson.items?.[0];
      if (!first) {
        setSelected(null);
        return;
      }

      const dRes = await fetch(
        `${API_BASE_URL}/v1/employer/requests/${encodeURIComponent(first.request_id)}?employer_id=${encodeURIComponent(employerId)}`,
        {
          headers: {
            Accept: "application/json",
            Authorization: `Bearer ${accessToken}`,
          },
        }
      );

      if (!dRes.ok) {
        const t = await dRes.text();
        throw new Error(`Detail failed (${dRes.status}): ${t}`);
      }
      const dText = await dRes.text();
      const dJson = (dText ? JSON.parse(dText) : null) as HRGetResp;
      if (cancelled) return;
      setSelected(dJson);


    } catch (e: any) {
      Alert.alert("HR load failed", String(e?.message ?? e));
    } finally {
      if (!cancelled) setLoading(false);
    }
  }

  void run();
  return () => {
    cancelled = true;
  };
}, [API_BASE_URL, accessToken, employerId]);

function handleReject() {
  if (!selected) {
    Alert.alert("No request selected");
    return;
  }

  if (Platform.OS === "ios") {
    ActionSheetIOS.showActionSheetWithOptions(
      {
        title: "Reject reason",
        options: [
          "Cancel",
          "No record found",
          "Policy restriction",
        ],
        cancelButtonIndex: 0,
      },
      (idx) => {
        if (idx === 1) void attest("REJECTED_NO_RECORD");
        if (idx === 2) void attest("REJECTED_POLICY");
      }
    );
  } else {
    Alert.alert(
      "Reject reason",
      undefined,
      [
        {
          text: "No record found",
          onPress: () => void attest("REJECTED_NO_RECORD"),
        },
        {
          text: "Policy restriction",
          onPress: () => void attest("REJECTED_POLICY"),
        },
        { text: "Cancel", style: "cancel" },
      ]
    );
  }
}


  return (
    <View
      className="flex-1 bg-white dark:bg-zinc-900"
      style={{
        paddingLeft: insets.left,
        paddingRight: insets.right,
      }}
    >
      <ScrollView
        contentInsetAdjustmentBehavior="automatic"
        showsVerticalScrollIndicator={false}
        contentContainerStyle={{
          flexGrow: 1,
          paddingBottom: 16 + 88 + insets.bottom,
        }}
      >
        
        <View className="px-4 pt-4 flex-1">
          {/* QUEUE */}
          <SectionHeader title="Queue" />
          <SettingsSection>
            <SettingsRow>
              <Text className="text-xs text-zinc-500 dark:text-zinc-400">
                Pending requests
              </Text>

              <View className="mt-2 flex-row items-center justify-between">
                <Text className="text-base text-zinc-900 dark:text-zinc-100">
                  {loading ? "Loading…" : `${queue.length} request(s)`}
                </Text>

                <Pressable
                  disabled={!queue.length || loading}
                  onPress={handleChooseRequest}
                  accessibilityRole="button"
                  accessibilityLabel="Choose request"
                  className={[
                    "px-3 py-2 rounded-lg",
                    queue.length && !loading
                      ? "bg-zinc-200 dark:bg-zinc-700"
                      : "bg-zinc-100 dark:bg-zinc-800",
                  ].join(" ")}
                  style={({ pressed }) => [{ opacity: pressed ? 0.85 : 1 }]}
                >
                  <Text className="text-sm font-semibold text-zinc-900 dark:text-white">
                    Choose
                  </Text>
                </Pressable>
              </View>

              {selected?.request_id ? (
                <Text className="text-xs text-zinc-500 dark:text-zinc-400 mt-2">
                  Selected: {selected.request_id}
                </Text>
              ) : null}
            </SettingsRow>
          </SettingsSection>
          {isEmpty ? (
            <View className="mt-4 rounded-xl bg-white dark:bg-zinc-800 border border-zinc-200 dark:border-zinc-700 px-4 py-4">
              <Text className="text-base font-semibold text-zinc-900 dark:text-zinc-100">
                No requests waiting for review
              </Text>
              <Text className="text-sm text-zinc-600 dark:text-zinc-300 mt-1">
                New submissions will appear here automatically.
              </Text>
            </View>
          ) : null}
          {!isEmpty ? (
          <>
          {/* CASE */}
          <SectionHeader title="Case" />
          <SettingsSection>
            <SettingsRow>
              <Text className="text-xs text-zinc-500 dark:text-zinc-400">
                Request ID
              </Text>
              <Text className="text-base font-medium text-zinc-900 dark:text-zinc-100 mt-0.5">
                {selected?.request_id ?? "—"}
              </Text>

              {submittedLabel ? (
                <Text className="text-sm text-zinc-600 dark:text-zinc-300 mt-0.5">
                  Updated: {submittedLabel}
                </Text>
              ) : null}
            </SettingsRow>

            <Separator />

            <SettingsRow>
              {/* Keep these hardcoded for now if you don’t yet have employee identity fields in claim_snapshot */}
              <Text className="text-xs text-zinc-500 dark:text-zinc-400">
                Employee
              </Text>
              <Text className="text-base text-zinc-900 dark:text-zinc-100 mt-0.5">
                Jane Doe
              </Text>
              <Text className="text-sm text-zinc-500 dark:text-zinc-300 mt-0.5">
                Worker ID ·{" "}
                <Text className="font-medium text-zinc-700 dark:text-zinc-200">
                  CEI-48219
                </Text>
              </Text>
            </SettingsRow>

            <Separator />

            <SettingsRow>
              <StatusRow status={status} rejectLabel={rejectLabel} />
            </SettingsRow>
          </SettingsSection>

          {/* CLAIM */}
          <SectionHeader title="Claim" />
          <SettingsSection>
            <SettingsRow>
              <Text className="text-xs text-zinc-500 dark:text-zinc-400">
                Employer
              </Text>
              <Text className="text-base text-zinc-900 dark:text-zinc-100 mt-0.5">
                {displayClaim.employer || "Employer will show here"}
              </Text>
            </SettingsRow>

            <Separator />

            <SettingsRow>
              <Text className="text-xs text-zinc-500 dark:text-zinc-400">
                Role
              </Text>
              <Text className="text-base text-zinc-900 dark:text-zinc-100 mt-0.5">
                {displayClaim.job_title || "Job title will show here"}
              </Text>
              <Text className="text-sm text-zinc-600 dark:text-zinc-400 mt-0.5">
                {displayClaim.start_mm_yyyy || "From"} —{" "}
                {displayClaim.end_mm_yyyy ?? "Present"}
              </Text>
            </SettingsRow>
          </SettingsSection>

          {/* DETAILS */}
          <SectionHeader title="Details" />
          <SettingsSection>
            <DisclosureRow
              title="Request details"
              body={[
                "Requester: Mortgage / Loan",
                "Purpose: Employment verification",
                "Consent: On file",
              ]}
            />
            <DisclosureRow
              title="Verification checks"
              body={["Tenure matches HRIS", "Title matches HRIS"]}
              last
            />
          </SettingsSection>

          {/* EVIDENCE UPLOAD */}
          <SectionHeader title="Evidence" />
          <SettingsSection>
            <SettingsRow>
              <Text className="text-xs text-zinc-500 dark:text-zinc-400">
                Upload evidence
              </Text>
              <View className="mt-2">
                {/* Helper text sits ABOVE controls (stable, iOS Settings-like) */}
                <Text className="text-sm text-zinc-600 dark:text-zinc-300">
                  Upload file or photo (max 5MB)
                </Text>
                <View className="mt-3">
                  <Pressable
                    disabled={!canUpload}
                    onPress={handleUploadEvidencePress}
                    accessibilityRole="button"
                    accessibilityLabel="Upload evidence"
                    className={[
                      "w-full rounded-xl px-4 py-3 items-center justify-center",
                      canUpload ? "bg-zinc-700" : "bg-zinc-400",
                    ].join(" ")}
                    style={({ pressed }) => [{ opacity: pressed ? 0.9 : 1 }]}
                  >
                    <Text className="text-base font-semibold text-white">
                      Upload evidence
                    </Text>
                  </Pressable>
                </View>
              </View>
              <View className="mt-3">
                {uploadState.status === "picking" ||
                uploadState.status === "initing" ||
                uploadState.status === "uploading" ? (
                  <View className="flex-row items-center gap-2">
                    <ActivityIndicator />
                    <Text className="text-sm text-zinc-600 dark:text-zinc-300">
                      {uploadState.status === "picking"
                        ? "Opening picker…"
                        : uploadState.status === "initing"
                        ? "Creating upload session…"
                        : "Uploading to storage…"}
                    </Text>
                  </View>
                ) : null}

                {uploadState.status === "done" ? (
                  <View className="mt-1">
                    <Text className="text-sm text-sky-600">
                      Upload complete
                    </Text>

                    <Text className="text-sm text-zinc-800 dark:text-zinc-200 mt-1">
                      {displayFileNameFromKey(uploadState.storageKey)}
                    </Text>

                    <Text
                      className="text-xs text-zinc-500 dark:text-zinc-400 mt-1"
                      numberOfLines={1}
                    >
                      {shortKey(uploadState.storageKey)}
                    </Text>

                    {/* Intentionally no "complete" call yet (server 501). */}
                  </View>
                ) : null}

                {uploadState.status === "error" ? (
                  <View className="mt-1">
                    <Text className="text-sm text-rose-600">
                      Upload failed
                    </Text>
                    <Text className="text-xs text-zinc-500 dark:text-zinc-400 mt-1">
                      {uploadState.message}
                    </Text>
                  </View>
                ) : null}
              </View>
            </SettingsRow>
          </SettingsSection>

          {/* FOOTNOTE */}
          <Footnote
            text="I consent to Cvera processing uploaded evidence solely for the purpose of verifying employment claims. 
          Cvera processes uploaded evidence solely to verify employment claims."
          />
          </> 
        ) : null}
          <View style={{ height: 24 + 88 + insets.bottom }} />
          <DecisionBar
            bottomInset={insets.bottom}
            onApprove={() => void attest("FULL_MATCH")}
            onReject={handleReject}
            disabled={!!attesting || loading || !selected || isEmpty}
            attesting={attesting}
          />
          </View>
      </ScrollView>
    </View>
  );
};
/* ===================== Section Header ===================== */

function SectionHeader({ title }: { title: string }) {
  return (
    <Text className="text-[12px] text-zinc-600 dark:text-zinc-400 mt-6 mb-2">
      {title.toUpperCase()}
    </Text>
  );
}

/* ===================== Settings Group Shell ===================== */

function SettingsSection({ children }: { children: React.ReactNode }) {
  return (
    <View className="rounded-xl bg-white dark:bg-zinc-800 border border-zinc-200 dark:border-zinc-700 overflow-hidden">
      {children}
    </View>
  );
}

function SettingsRow({ children }: { children: React.ReactNode }) {
  return <View className="px-4 py-3">{children}</View>;
}

function Separator() {
  return <View className="h-px bg-zinc-200 dark:bg-zinc-700 ml-4" />;
}

/* ===================== Status Row ===================== */

function StatusRow({
  status,
  rejectLabel,
}: {
  status: CaseStatus;
  rejectLabel?: string | null;
}) {
  const color =
    status === "ATTESTED"
      ? "text-emerald-600"
      : status === "REJECTED"
      ? "text-rose-600"
      : "text-amber-600";

  return (
    <View>
      <View className="flex-row items-center justify-between">
        <Text className="text-base text-zinc-900 dark:text-zinc-100">Status</Text>
        <Text className={`text-base font-medium ${color}`}>{status}</Text>
      </View>

      {status === "REJECTED" && rejectLabel ? (
        <Text className="text-sm text-zinc-600 dark:text-zinc-300 mt-1">
          {rejectLabel}
        </Text>
      ) : null}
    </View>
  );
}

/* ===================== Disclosure Row ===================== */

function DisclosureRow({
  title,
  body,
  last,
}: {
  title: string;
  body: string[];
  last?: boolean;
}) {
  const [open, setOpen] = useState(false);

  return (
    <View>
      <Pressable
        onPress={() => setOpen((v) => !v)}
        className="px-4 py-3 flex-row items-center justify-between"
        accessibilityRole="button"
        accessibilityLabel={`${open ? "Collapse" : "Expand"} ${title}`}
      >
        <Text className="text-base text-zinc-900 dark:text-zinc-100">
          {title}
        </Text>
        <Text className="text-zinc-400 text-base dark:text-zinc-400">
          {open ? "⌃" : "⌄"}
        </Text>
      </Pressable>

      {open ? (
        <View className="px-4 pb-3">
          {body.map((line, i) => (
            <Text
              key={i}
              className="text-sm text-zinc-600 dark:text-zinc-400 mt-1 leading-5"
            >
              {line}
            </Text>
          ))}
        </View>
      ) : null}

      {!last ? <Separator /> : null}
    </View>
  );
}

/* ===================== Footnote ===================== */

function Footnote({ text }: { text: string }) {
  return (
    <Text className="text-[13px] text-zinc-600 dark:text-zinc-400 mt-3 leading-5">
      {text}
    </Text>
  );
}

/* ===================== Sticky Decision Bar ===================== */
function DecisionBar({
  bottomInset,
  onApprove,
  onReject,
  disabled,
  attesting
}: {
  bottomInset: number;
  onApprove: () => void;
  onReject: () => void;
  disabled?: boolean;
  attesting: EmployerResponseType | null;
}) {
  return (
    <View
      className="absolute left-0 right-0 bottom-0 bg-white dark:bg-zinc-800 border-t border-zinc-200 dark:border-zinc-700 px-4 pt-3"
      style={{ paddingBottom: bottomInset + 16 }}
    >
      <View className="flex-row gap-3">
        <Pressable
          disabled={!!disabled}
          className={[
            "flex-1 rounded-xl py-3 items-center",
            disabled ? "bg-zinc-300" : "bg-zinc-200",
          ].join(" ")}
          onPress={onReject}
          >
          {attesting === "REJECTED_NO_RECORD" || attesting === "REJECTED_POLICY" ? (
          <View className="flex-row items-center gap-2">
            <ActivityIndicator />
            <Text className="text-sm font-semibold text-zinc-900">Rejecting…</Text>
          </View>
        ) : (
          <Text className="text-sm font-semibold text-zinc-900">Reject</Text>
        )}
        </Pressable>

        <Pressable
          disabled={!!disabled}
          className={[
            "flex-1 rounded-xl py-3 items-center",
            disabled ? "bg-zinc-500" : "bg-zinc-700",
          ].join(" ")}
          onPress={onApprove}
        >
         {attesting === "FULL_MATCH" || attesting === "PARTIAL_MATCH" ? (
          <View className="flex-row items-center gap-2">
            <ActivityIndicator />
            <Text className="text-sm font-semibold text-white">Approving…</Text>
          </View>
        ) : (
          <Text className="text-sm font-semibold text-white">Approve</Text>
        )}
        </Pressable>
      </View>
    </View>
  );
};