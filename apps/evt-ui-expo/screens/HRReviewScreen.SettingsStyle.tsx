import React, { useMemo, useState } from "react";
import {
  View,
  Text,
  ScrollView,
  Pressable,
  Alert,
  ActivityIndicator,
} from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import * as DocumentPicker from "expo-document-picker";
import * as ImagePicker from "expo-image-picker";
import * as FileSystem from "expo-file-system/legacy";

/**
 * NOTE (intentional for MVP):
 * - `evidence:complete` is DISABLED because your server currently returns 501.
 * - This implementation reuses your existing `evidence:init` + presigned PUT flow exactly.
 * - UI “Upload complete” continues to mean “PUT succeeded”.
 */

type CaseStatus = "PENDING" | "APPROVED" | "REJECTED";

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

function inferExtensionFromMime(mimeType: EvidenceFile["mimeType"]): "pdf" | "jpg" | "png" {
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

function guessMimeTypeFromName(filename: string): EvidenceFile["mimeType"] | null {
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
  if (typeof size !== "number" || size <= 0) throw new Error("Missing or invalid file size.");
  return size;
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
    else if (uriLower.endsWith(".jpg") || uriLower.endsWith(".jpeg")) mimeType = "image/jpeg";
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
    throw new Error(`File is too large. Max is ${Math.floor(MAX_FILE_BYTES / 1_000_000)}MB.`);
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
    // Structured message to fit your Alert pattern
    throw new Error(
      "Photo access denied. Enable Photos permission for this app in iOS Settings > Privacy & Security > Photos."
    );
  }

  const res = await ImagePicker.launchImageLibraryAsync({
    mediaTypes: ImagePicker.MediaTypeOptions.Images,
    allowsMultipleSelection: false,
    quality: 1,
    // Don't force base64 (we upload bytes via FileSystem.uploadAsync)
    base64: false,
    exif: false,
  });

  if (res.canceled) return null;

  const asset = res.assets?.[0];
  if (!asset?.uri) throw new Error("Image picker returned no image.");

  // Normalize:
  // - For iOS/Android, asset.fileName and asset.mimeType may or may not exist
  const normalized = await normalizeEvidenceFile({
    uri: asset.uri,
    name: (asset as any).fileName ?? null,
    mimeType: (asset as any).mimeType ?? null,
    size: typeof (asset as any).fileSize === "number" ? (asset as any).fileSize : null,
  });

  // Additional guard: ImagePicker should only allow images, but enforce minimum allowlist
  if (normalized.mimeType !== "image/jpeg" && normalized.mimeType !== "image/png") {
    throw new Error("Unsupported photo type. Use JPG or PNG.");
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
    mimeType: asset.mimeType ?? (asset.name ? guessMimeTypeFromName(asset.name) : null),
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
async function uploadViaInitAndPut(evidence: EvidenceFile): Promise<{ storageKey: string }> {
  if (!API_BASE_URL) {
    throw new Error("Missing EXPO_PUBLIC_EVT_API_BASE_URL (device cannot reach 127.0.0.1).");
  }

  // Extra defensive checks (deterministic policy)
  if (!ALLOWED_MIME_TYPES.includes(evidence.mimeType as any)) {
    throw new Error("Unsupported file type. Use PDF, JPG, or PNG.");
  }
  if (!evidence.size || evidence.size <= 0) throw new Error("Missing or invalid file size.");
  if (evidence.size > MAX_FILE_BYTES) {
    throw new Error(`File is too large. Max is ${Math.floor(MAX_FILE_BYTES / 1_000_000)}MB.`);
  }

  const initUrl = `${API_BASE_URL}/v1/cases/${encodeURIComponent(CASE_ID)}/checks/${encodeURIComponent(
    CHECK_ID
  )}/evidence:init`;

  const initRes = await fetch(initUrl, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      caseId: CASE_ID, // keep as-is (your server ignores/accepts)
      checkId: CHECK_ID, // keep as-is (your server ignores/accepts)
      files: [{ name: evidence.name, mimeType: evidence.mimeType, size: evidence.size }],
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

export default function HRReviewScreenSettingsStyle() {
  const insets = useSafeAreaInsets();
  const status: CaseStatus = "PENDING";

  const [uploadState, setUploadState] = useState<UploadState>({ status: "idle" });

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
          message: "Missing EXPO_PUBLIC_EVT_API_BASE_URL (device cannot reach 127.0.0.1).",
        });
        return;
      }

      setUploadState({ status: "picking" });

      const evidence = await pickDocumentEvidence();
      if (!evidence) {
        setUploadState({ status: "idle" });
        return;
      }

      setUploadState({ status: "initing" });
      const { storageKey } = await uploadViaInitAndPut(evidence);

      setUploadState({ status: "done", storageKey });
    } catch (err: any) {
      const msg = String(err?.message ?? err ?? "");
      const friendly =
        msg.includes("Network request failed")
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
          message: "Missing EXPO_PUBLIC_EVT_API_BASE_URL (device cannot reach 127.0.0.1).",
        });
        return;
      }

      setUploadState({ status: "picking" });

      const evidence = await pickPhotoEvidence();
      if (!evidence) {
        setUploadState({ status: "idle" });
        return;
      }

      setUploadState({ status: "initing" });
      const { storageKey } = await uploadViaInitAndPut(evidence);

      setUploadState({ status: "done", storageKey });
    } catch (err: any) {
      const msg = String(err?.message ?? err ?? "");
      const friendly =
        msg.includes("Photo access denied")
          ? msg
          : msg.includes("Network request failed")
          ? "Cannot reach server. Check Wi-Fi/LAN URL."
          : msg.toLowerCase().includes("timed out")
          ? "Request timed out. Check server reachability or try again."
          : msg;

      setUploadState({ status: "error", message: friendly });
    }
  }

  return (
    <View
      className="flex-1 bg-white dark:bg-zinc-900"
      style={{ paddingTop: insets.top, paddingLeft: insets.left, paddingRight: insets.right }}
    >
      <ScrollView
        showsVerticalScrollIndicator={false}
        contentContainerStyle={{
          flexGrow: 1,
          paddingBottom: 16 + 64 + insets.bottom,
        }}
      >
        <View className="px-4 pt-4 flex-1">
          <TopBar />

          {/* CASE */}
          <SectionHeader title="Case" />
          <SettingsSection>
            <SettingsRow>
              <Text className="text-xs text-zinc-500 dark:text-zinc-400">Case ID</Text>
              <Text className="text-base font-medium text-zinc-900 dark:text-zinc-100 mt-0.5">
                EVT-10324
              </Text>
              <Text className="text-sm text-zinc-600 dark:text-zinc-300 mt-0.5">Submitted 2h ago</Text>
            </SettingsRow>

            <Separator />

            <SettingsRow>
              <Text className="text-xs text-zinc-500 dark:text-zinc-400">Employee</Text>
              <Text className="text-base text-zinc-900 dark:text-zinc-100 mt-0.5">Jane Doe</Text>
              <Text className="text-sm text-zinc-500 dark:text-zinc-300 mt-0.5">
                Worker ID ·{" "}
                <Text className="font-medium text-zinc-700 dark:text-zinc-200">CEI-48219</Text>
              </Text>
            </SettingsRow>

            <Separator />

            <SettingsRow>
              <StatusRow status={status} />
            </SettingsRow>
          </SettingsSection>

          {/* CLAIM */}
          <SectionHeader title="Claim" />
          <SettingsSection>
            <SettingsRow>
              <Text className="text-xs text-zinc-500 dark:text-zinc-400">Employer</Text>
              <Text className="text-base text-zinc-900 dark:text-zinc-100 mt-0.5">
                ACME Electric (AEI)
              </Text>
            </SettingsRow>

            <Separator />

            <SettingsRow>
              <Text className="text-xs text-zinc-500 dark:text-zinc-400">Role</Text>
              <Text className="text-base text-zinc-900 dark:text-zinc-100 mt-0.5">
                Senior Project Manager
              </Text>
              <Text className="text-sm text-zinc-600 dark:text-zinc-400 mt-0.5">Aug 2023 — May 2025</Text>
            </SettingsRow>
          </SettingsSection>

          {/* DETAILS */}
          <SectionHeader title="Details" />
          <SettingsSection>
            <DisclosureRow
              title="Request details"
              body={["Requester: Mortgage / Loan", "Purpose: Employment verification", "Consent: On file"]}
            />
            <DisclosureRow title="Verification checks" body={["Tenure matches HRIS", "Title matches HRIS"]} last />
          </SettingsSection>

          {/* EVIDENCE UPLOAD */}
          <SectionHeader title="Evidence" />
          <SettingsSection>
            <SettingsRow>
              <Text className="text-xs text-zinc-500 dark:text-zinc-400">Upload evidence</Text>

              <View className="mt-2">
            {/* Helper text sits ABOVE controls (stable, iOS Settings-like) */}
            <Text className="text-sm text-zinc-600 dark:text-zinc-300">
              PDF / JPG / PNG (max 5MB)
            </Text>

            {/* Controls are a predictable layout: 2 equal buttons */}
            <View className="mt-3 flex-row gap-2">
              <Pressable
                disabled={!canUpload}
                onPress={() => {
                  if (!API_BASE_URL) {
                    Alert.alert("Missing API base URL", "Set EXPO_PUBLIC_EVT_API_BASE_URL to a reachable host.");
                    return;
                  }
                  pickAndUploadPhoto();
                }}
                accessibilityRole="button"
                accessibilityLabel="Choose photo"
                className={[
                  "flex-1 rounded-xl px-4 py-3 items-center justify-center",
                  canUpload ? "bg-zinc-700" : "bg-zinc-400",
                ].join(" ")}
                style={({ pressed }) => [{ opacity: pressed ? 0.9 : 1 }]}
              >
                <Text className="text-base font-semibold text-white">
                  Choose photo
                </Text>
              </Pressable>

              <Pressable
                disabled={!canUpload}
                onPress={() => {
                  if (!API_BASE_URL) {
                    Alert.alert("Missing API base URL", "Set EXPO_PUBLIC_EVT_API_BASE_URL to a reachable host.");
                    return;
                  }
                  pickAndUploadDocument();
                }}
                accessibilityRole="button"
                accessibilityLabel="Choose file"
                className={[
                  "flex-1 rounded-xl px-4 py-3 items-center justify-center",
                  canUpload ? "bg-zinc-600" : "bg-zinc-400",
                ].join(" ")}
                style={({ pressed }) => [{ opacity: pressed ? 0.9 : 1 }]}
              >
                <Text className="text-base font-semibold text-white">
                  Choose file
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
                    <Text className="text-sm text-emerald-600">Upload complete</Text>

                    <Text className="text-sm text-zinc-800 dark:text-zinc-200 mt-1">
                      {displayFileNameFromKey(uploadState.storageKey)}
                    </Text>

                    <Text className="text-xs text-zinc-500 dark:text-zinc-400 mt-1" numberOfLines={1}>
                      {shortKey(uploadState.storageKey)}
                    </Text>

                    {/* Intentionally no "complete" call yet (server 501). */}
                  </View>
                ) : null}

                {uploadState.status === "error" ? (
                  <View className="mt-1">
                    <Text className="text-sm text-rose-600">Upload failed</Text>
                    <Text className="text-xs text-zinc-500 dark:text-zinc-400 mt-1">{uploadState.message}</Text>
                  </View>
                ) : null}
              </View>
            </SettingsRow>
          </SettingsSection>

          {/* FOOTNOTE */}
          <Footnote
            text="I consent to Certis processing uploaded evidence solely for the purpose of verifying employment claims. 
          Certis processes uploaded evidence solely to verify employment claims. 
          Raw evidence is retained only as long as necessary to complete verification and is then deleted. 
          Certis issues cryptographic verification tokens that do not expose underlying documents."
          />

          <View style={{ paddingBottom: insets.bottom }} />

          <DecisionBar bottomInset={insets.bottom} onApprove={() => console.log("Approve")} onReject={() => console.log("Reject")} />
        </View>
      </ScrollView>
    </View>
  );
}

/* ===================== Top Bar ===================== */

function TopBar() {
  return (
    <View className="mb-4">
      <Text className="text-[12px] text-zinc-500 dark:text-zinc-400">EVT</Text>
      <Text className="text-3xl font-semibold text-zinc-950 dark:text-zinc-50 mt-1">HR Review</Text>
      <Text className="text-sm text-zinc-500 dark:text-zinc-300 mt-1">
        Validate employment details and decide whether to issue an EVT.
      </Text>
    </View>
  );
}

/* ===================== Section Header ===================== */

function SectionHeader({ title }: { title: string }) {
  return <Text className="text-[12px] text-zinc-600 dark:text-zinc-400 mt-6 mb-2">{title.toUpperCase()}</Text>;
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

function StatusRow({ status }: { status: CaseStatus }) {
  const color =
    status === "APPROVED" ? "text-emerald-600" : status === "REJECTED" ? "text-rose-600" : "text-amber-600";

  return (
    <View className="flex-row items-center justify-between">
      <Text className="text-base text-zinc-900 dark:text-zinc-100">Status</Text>
      <Text className={`text-base font-medium ${color}`}>{status}</Text>
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
        <Text className="text-base text-zinc-900 dark:text-zinc-100">{title}</Text>
        <Text className="text-zinc-400 text-base dark:text-zinc-400">{open ? "⌃" : "⌄"}</Text>
      </Pressable>

      {open ? (
        <View className="px-4 pb-3">
          {body.map((line, i) => (
            <Text key={i} className="text-sm text-zinc-600 dark:text-zinc-400 mt-1 leading-5">
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
  return <Text className="text-[13px] text-zinc-600 dark:text-zinc-400 mt-3 leading-5">{text}</Text>;
}

/* ===================== Sticky Decision Bar ===================== */

function DecisionBar({
  bottomInset,
  onApprove,
  onReject,
}: {
  bottomInset: number;
  onApprove: () => void;
  onReject: () => void;
}) {
  return (
    <View
      className="absolute left-0 right-0 bottom-0 bg-white dark:bg-zinc-800 border-t border-zinc-200 dark:border-zinc-700 px-4 pt-3"
      style={{ paddingBottom: bottomInset + 16 }}
    >
      <View className="flex-row gap-3">
        <Pressable className="flex-1 rounded-xl bg-zinc-200 py-3 items-center" onPress={onReject}>
          <Text className="text-sm font-semibold text-zinc-900">Reject</Text>
        </Pressable>

        <Pressable className="flex-1 rounded-xl bg-zinc-700 py-3 items-center" onPress={onApprove}>
          <Text className="text-sm font-semibold text-white">Approve</Text>
        </Pressable>
      </View>
    </View>
  );
}
