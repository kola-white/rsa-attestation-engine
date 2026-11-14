import { RefreshControl, useColorScheme } from "react-native";
import React, { useLayoutEffect, useMemo, useState } from "react";
import { useAuth } from "@/src/auth/AuthContext";
import { EvidenceReviewLink } from "@/components/EvidenceReviewLink";
import { EmployerReview, employerLabel, type EmployerResponseType } from "@/src/api/employerReview";
import {
  uploadEvidence, completeEvidence, listEvidenceChecks,
  EvidenceCompletionPending, EvidenceRequestError,
  MAX_EVIDENCE_BYTES, EVIDENCE_MIME_TYPES,
  type EvidenceScope, type EvidenceCheck, type EvidenceReceipt, type CompletionTicket,
} from "@/src/api/evidence";
import { useFocusEffect, useNavigation } from "@react-navigation/native";
import type { NativeStackNavigationProp } from "@react-navigation/native-stack";
import type { AppStackParamList } from "@/src/navigation/MainAppNavigator";
import {
  View,
  Modal,
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

type UploadState =
  | { status: "idle" | "picking" | "initing" | "uploading" | "completing" }
  | { status: "done"; receipt: EvidenceReceipt }
  | { status: "completion_pending"; ticket: CompletionTicket; digest: string; ownerId: string }
  | { status: "error"; message: string };

type EvidenceFile = {
  uri: string; // MUST be file:// for FileSystem.uploadAsync
  name: string; // deterministic
  mimeType: "application/pdf" | "image/jpeg" | "image/png";
  size: number; // bytes
};

const API_BASE_URL = process.env.EXPO_PUBLIC_EVT_API_BASE_URL;
const MAX_FILE_BYTES = MAX_EVIDENCE_BYTES;
const ALLOWED_MIME_TYPES = EVIDENCE_MIME_TYPES;

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
  if (Platform.OS === "web") {
    const blob = await (await fetch(uri)).blob();
    if (!blob.size) throw new Error("Missing or invalid file size.");
    return blob.size;
  }
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
  const bytes = Platform.OS === "web"
    ? new Uint8Array(await (await fetch(uri)).arrayBuffer())
    : base64ToUint8Array(await FileSystem.readAsStringAsync(uri, { encoding: FileSystem.EncodingType.Base64 }));

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
  if (Platform.OS !== "web" && !uploadUri.startsWith("file://")) {
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

// This storage request uses only the server-required headers, never the API token.
async function putEvidenceFile(url: string, uri: string, headers: Record<string, string>): Promise<{ status: number }> {
  if (Platform.OS === "web") {
    const blob = await (await fetch(uri)).blob();
    const response = await fetch(url, { method: "PUT", headers, body: blob });
    return { status: response.status };
  }
  const response = await FileSystem.uploadAsync(url, uri, {
    httpMethod: "PUT", headers,
    uploadType: FileSystem.FileSystemUploadType.BINARY_CONTENT,
  });
  return { status: response.status };
}

export const HRReviewScreenSettingsStyle = () => {
  const scheme = useColorScheme(); // "dark" | "light" | null
  const isDark = scheme === "dark";
  console.log("[HRReview] render");
  const insets = useSafeAreaInsets();
  const { logout, accessToken, user } = useAuth();
  const navigation = useNavigation<HRNav>();

  const [uploadState, setUploadState] = useState<UploadState>({
    status: "idle",
  });

  // In-memory UX dedupe is scoped to the chosen case/check and current account.
  // These digests are never transmitted or used as canonical evidence hashes.
  const [evidenceDigests, setEvidenceDigests] = useState<Record<string, string[]>>({});
  const [evidenceScopes, setEvidenceScopes] = useState<EvidenceCheck[]>([]);
  const [evidenceScope, setEvidenceScope] = useState<EvidenceScope | null>(null);
  const [scopeLoading, setScopeLoading] = useState(false);
  const [scopeError, setScopeError] = useState<string | null>(null);
  const [scopeRefresh, setScopeRefresh] = useState(0);
  const [scopePickerOpen, setScopePickerOpen] = useState(false);
  const actorRef = React.useRef(user?.id);

  useLayoutEffect(() => {
    actorRef.current = user?.id;
    setEvidenceScopes([]);setEvidenceScope(null);setEvidenceDigests({});
    setUploadState({ status: "idle" });setScopePickerOpen(false);
    return () => { actorRef.current = undefined; };
  }, [user?.id]);

  useFocusEffect(React.useCallback(() => {
    let active = true;
    const owner = user?.id;
    if (!API_BASE_URL || !accessToken || !owner) {
      setScopeError("Sign in to load assigned evidence cases.");setScopeLoading(false);
      return () => { active = false; };
    }
    setScopeLoading(true);setScopeError(null);
    void listEvidenceChecks(API_BASE_URL, accessToken).then(rows => {
      if (!active || actorRef.current !== owner) return;
      const allowed = rows.filter(row => row.canUpload);
      setEvidenceScopes(allowed);
      setEvidenceScope(previous => previous && allowed.some(row => row.caseId === previous.caseId && row.checkId === previous.checkId) ? previous : null);
    }).catch(() => {
      if (!active || actorRef.current !== owner) return;
      setEvidenceScopes([]);setEvidenceScope(null);
      setScopeError("Assigned evidence cases could not be loaded. Try refreshing.");
    }).finally(() => {
      if (active && actorRef.current === owner) setScopeLoading(false);
    });
    return () => { active = false; };
  }, [accessToken, user?.id, scopeRefresh]));

  const uploadBusy = ["picking", "initing", "uploading", "completing", "completion_pending"].includes(uploadState.status);
  const canUpload = !!accessToken && !!user?.id && !!evidenceScope && !scopeLoading && !uploadBusy;

  function markReceived(receipt: EvidenceReceipt, digest: string, scope: EvidenceScope, owner: string) {
    if (actorRef.current !== owner) return;
    const key = JSON.stringify([scope.caseId, scope.checkId]);
    setEvidenceDigests(previous => ({ ...previous, [key]: [...(previous[key] ?? []), digest] }));
    setUploadState({ status: "done", receipt });
  }

  async function pickAndReceive(pick: () => Promise<EvidenceFile | null>) {
    const owner = user?.id, scope = evidenceScope;
    if (!API_BASE_URL || !accessToken || !owner || !scope) {
      setUploadState({ status: "error", message: "Choose an assigned evidence case and check first." });return;
    }
    let digest: string | null = null;
    try {
      setUploadState({ status: "picking" });
      const file = await pick();
      if (actorRef.current !== owner) return;
      if (!file) { setUploadState({ status: "idle" });return; }
      setUploadState({ status: "initing" });
      digest = await computeEvidenceDigestHex(file.uri);
      if (actorRef.current !== owner) return;
      const key = JSON.stringify([scope.caseId, scope.checkId]);
      if (evidenceDigests[key]?.includes(digest)) {
        setUploadState({ status: "idle" });
        Alert.alert("Already received", "This file was already received for this case/check.");return;
      }
      const receipt = await uploadEvidence({ apiBase: API_BASE_URL, accessToken, scope, file,
        put: putEvidenceFile,
        onProgress: status => { if (actorRef.current === owner) setUploadState({ status }); },
      });
      markReceived(receipt, digest, scope, owner);
    } catch (error) {
      if (actorRef.current !== owner) return;
      if (error instanceof EvidenceCompletionPending && digest) {
        setUploadState({ status: "completion_pending", ticket: error.ticket, digest, ownerId: owner });return;
      }
      const message = error instanceof EvidenceRequestError ? error.message
        : error instanceof Error && /^(Unsupported file type|File is too large|Photo access denied)/.test(error.message) ? error.message
        : "The file could not be uploaded. Check access and try again.";
      setUploadState({ status: "error", message });
    }
  }
  const pickAndUploadDocument = () => pickAndReceive(pickDocumentEvidence);
  const pickAndUploadPhoto = () => pickAndReceive(pickPhotoEvidence);

  async function retryEvidenceCompletion() {
    if (uploadState.status !== "completion_pending" || !API_BASE_URL || !accessToken || !user?.id || uploadState.ownerId !== user.id) return;
    const pending = uploadState, owner = user.id;
    setUploadState({ status: "completing" });
    try {
      const receipt = await completeEvidence(API_BASE_URL, accessToken, pending.ticket);
      markReceived(receipt, pending.digest, pending.ticket, owner);
    } catch (error) {
      if (actorRef.current !== owner) return;
      if (error instanceof EvidenceCompletionPending) setUploadState(pending);
      else setUploadState({ status: "error", message: error instanceof EvidenceRequestError ? error.message : "Receipt could not be confirmed. Try again." });
    }
  }

  const handleUploadEvidencePress = () => {
    if (!canUpload) return;

     if (Platform.OS === "web") {
      void pickAndUploadDocument();
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



  const employerReview = useMemo(() => new EmployerReview(API_BASE_URL, accessToken ?? ""), [API_BASE_URL, accessToken]);
  const { employers, employerId, queue, selected, loading, attesting, error: employerError } =
    React.useSyncExternalStore(employerReview.subscribe, employerReview.snapshot, employerReview.snapshot);
  const [refreshing, setRefreshing] = useState(false);

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
  if (loading || attesting || !queue.length) return;
  // Inline selection works on web and native, without manual request-ID entry.
  setRequestPickerOpen(open => !open);
};
const [requestPickerOpen, setRequestPickerOpen] = useState(false);
const onPullRefresh = React.useCallback(async () => {
  setRefreshing(true);
  setRequestPickerOpen(false);
  try { await employerReview.refresh(); } finally { setRefreshing(false); }
}, [employerReview]);
async function attest(responseType: EmployerResponseType) {
  const result = await employerReview.attest(responseType);
  if (result) {
    Alert.alert("Attested", `Status: ${result}`);
    await employerReview.refresh();
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
        <EvidenceReviewLink />
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

useFocusEffect(
  React.useCallback(() => {
    void employerReview.refresh();
    return employerReview.clear;
  }, [employerReview])
);

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
      refreshControl={
        <RefreshControl refreshing={refreshing} onRefresh={onPullRefresh} />
        }
        contentInsetAdjustmentBehavior="automatic"
        showsVerticalScrollIndicator={false}
        contentContainerStyle={{
          flexGrow: 1,
          paddingBottom: 16 + 88 + insets.bottom,
        }}
      >
        
        <View className="px-4 pt-4 flex-1">
          <SectionHeader title="Employer" />
          <SettingsSection>
            <SettingsRow>
              <Text accessibilityLiveRegion="polite" className="text-base text-zinc-900 dark:text-zinc-100">
                {loading && !employers.length ? "Loading employer access…"
                  : employerError ? employerError
                  : !employers.length ? "Employer access unavailable. Ask your Cvera administrator to confirm your assignment."
                  : !employerId ? "Select an employer to review."
                  : employerLabel(employers.find(e => e.employer_id === employerId)!, employers.findIndex(e => e.employer_id === employerId))}
              </Text>
              {employers.length > 1 ? employers.map((employer, index) => (
                <Pressable key={employer.employer_id} accessibilityRole="button"
                  accessibilityState={{ selected: employerId === employer.employer_id, disabled: !!attesting }}
                  disabled={!!attesting}
                  onPress={() => { setRequestPickerOpen(false); void employerReview.selectEmployer(employer.employer_id); }}
                  className="mt-2 rounded-lg border border-zinc-300 p-3">
                  <Text className="text-zinc-900 dark:text-zinc-100">
                    {employerLabel(employer, index)}{employerId === employer.employer_id ? " (selected)" : ""}
                  </Text>
                </Pressable>
              )) : null}
              <Pressable accessibilityRole="button" disabled={loading || !!attesting}
                onPress={() => void onPullRefresh()} className="mt-3 p-2">
                <Text className="text-sky-600">Refresh employer access</Text>
              </Pressable>
            </SettingsRow>
          </SettingsSection>
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
                  disabled={loading || !!attesting || !queue.length}
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

              {requestPickerOpen && !loading && !attesting ? queue.map((request, index) => (
                <Pressable key={request.request_id} accessibilityRole="button"
                  onPress={() => { setRequestPickerOpen(false); void employerReview.selectRequest(request.request_id); }}
                  className="mt-2 p-2">
                  <Text className="text-sky-600">Request {index + 1}</Text>
                </Pressable>
              )) : null}
              {selected?.request_id ? (
                <Text className="text-xs text-zinc-500 dark:text-zinc-400 mt-2">
                  Selected: {selected.request_id}
                </Text>
              ) : null}
            </SettingsRow>
          </SettingsSection>
          {isEmpty && employerId && !employerError ? (
            <View className="mt-4 rounded-xl bg-white dark:bg-zinc-800 border border-zinc-200 dark:border-zinc-700 px-4 py-4">
              <Text className="text-base font-semibold text-zinc-900 dark:text-zinc-100">
                No requests waiting for review
              </Text>
              <Text className="text-sm text-zinc-600 dark:text-zinc-300 mt-1">
                New submissions will appear here automatically.
              </Text>
            </View>
          ) : null}
          {selected && !loading && !employerError ? (
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
            <Modal visible={scopePickerOpen} transparent animationType="fade" onRequestClose={() => setScopePickerOpen(false)}>
              <View style={{ flex: 1, justifyContent: "center", padding: 24, backgroundColor: "rgba(0,0,0,0.5)" }}>
                <View className="rounded-2xl bg-white dark:bg-zinc-900 p-4" style={{ maxHeight: "75%" }}>
                  <Text className="text-lg font-semibold text-zinc-900 dark:text-zinc-100 mb-3">Choose evidence destination</Text>
                  <ScrollView style={{ flexShrink: 1 }}>
                    {evidenceScopes.map(scope => <Pressable key={JSON.stringify([scope.caseId, scope.checkId])} accessibilityRole="button" className="py-3"
                      onPress={() => { setEvidenceScope({ caseId: scope.caseId, checkId: scope.checkId });setUploadState({ status: "idle" });setScopePickerOpen(false); }}>
                      <Text className="text-base text-sky-600">{scope.caseId}</Text>
                      <Text className="text-sm text-zinc-600 dark:text-zinc-300">{scope.checkId}</Text>
                    </Pressable>)}
                  </ScrollView>
                  <Pressable accessibilityRole="button" className="pt-4 pb-2" onPress={() => setScopePickerOpen(false)}><Text className="text-base text-sky-600">Cancel</Text></Pressable>
                </View>
              </View>
            </Modal>

            <SettingsRow>
              <Text className="text-xs text-zinc-500 dark:text-zinc-400">Evidence destination</Text>
              <Pressable accessibilityRole="button" disabled={uploadBusy || scopeLoading || !evidenceScopes.length}
                onPress={() => setScopePickerOpen(true)} className="py-3">
                <Text className="text-base text-sky-600">{scopeLoading ? "Loading assigned cases…" : evidenceScope ? `${evidenceScope.caseId} / ${evidenceScope.checkId}` : "Choose an assigned case and check"}</Text>
              </Pressable>
              <Text className="text-xs text-zinc-500 dark:text-zinc-400">This destination is separate from the selected employment request.</Text>
              {!scopeLoading && !scopeError && evidenceScopes.length === 0 ? <Text className="text-sm text-zinc-600 dark:text-zinc-300 mt-2">No evidence upload assignments are available for this account.</Text> : null}
              {scopeError ? <Text className="text-sm text-rose-600 mt-2">{scopeError}</Text> : null}
              <Pressable disabled={uploadBusy || scopeLoading} accessibilityRole="button" onPress={() => setScopeRefresh(value => value + 1)} className="py-2">
                <Text className="text-sm text-sky-600">Refresh assignments</Text>
              </Pressable>
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
                (uploadState.status === "uploading" || uploadState.status === "completing") ? (
                  <View className="flex-row items-center gap-2">
                    <ActivityIndicator />
                    <Text className="text-sm text-zinc-600 dark:text-zinc-300">
                      {uploadState.status === "picking"
                        ? "Opening picker…"
                        : uploadState.status === "initing"
                        ? "Preparing upload…"
                        : uploadState.status === "completing" ? "Confirming server receipt…" : "Uploading file…"}
                    </Text>
                  </View>
                ) : null}

                {uploadState.status === "completion_pending" ? (
                  <View className="mt-2">
                    <Text className="text-sm text-zinc-700 dark:text-zinc-200">Receipt is not confirmed. Retry completion without uploading another copy.</Text>
                    <Pressable accessibilityRole="button" onPress={() => void retryEvidenceCompletion()} className="py-3">
                      <Text className="text-base text-sky-600">Retry completion</Text>
                    </Pressable>
                  </View>
                ) : null}
                {uploadState.status === "done" ? (
                  <View className="mt-1">
                    <Text className="text-sm text-sky-600">Received — awaiting review</Text>
                    <Text className="text-sm text-zinc-800 dark:text-zinc-200 mt-1">{uploadState.receipt.name}</Text>
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
            disabled={loading || !!attesting || !selected || isEmpty || !!employerError}
            disabledReason={
              attesting
                ? "Decision in progress"
                : !selected
                ? "No request selected"
                : isEmpty
                ? "No requests waiting for review"
                : null
            }
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
  disabledReason,
  attesting
}: {
  bottomInset: number;
  onApprove: () => void;
  onReject: () => void;
  disabled?: boolean;
  disabledReason?: string | null;
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
      {disabled && disabledReason ? (
        <Text className="text-xs text-zinc-500 dark:text-zinc-400 text-center mt-2">
          {disabledReason}
        </Text>
      ) : null}
    </View>
  );
};
