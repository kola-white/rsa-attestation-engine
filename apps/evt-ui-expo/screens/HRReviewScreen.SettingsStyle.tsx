import React, { useMemo, useState } from "react";
import { View, Text, ScrollView, Pressable, Alert, ActivityIndicator, } from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import * as DocumentPicker from "expo-document-picker";
import * as FileSystem from "expo-file-system/legacy";


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

const CASE_ID = "EVT-10324";
const CHECK_ID = "employment.company_and_dates";

// Must be reachable from your phone (NOT 127.0.0.1).
const API_BASE_URL = process.env.EXPO_PUBLIC_EVT_API_BASE_URL;
console.log("API_BASE_URL runtime =", process.env.EXPO_PUBLIC_EVT_API_BASE_URL);

function displayFileNameFromKey(key: string): string {
  const parts = key.split("/");
  return parts[parts.length - 1] ?? key;
}

function shortKey(key: string, head = 22, tail = 10): string {
  if (key.length <= head + tail + 3) return key;
  return `${key.slice(0, head)}…${key.slice(-tail)}`;
}


export default function HRReviewScreenSettingsStyle() {
  const insets = useSafeAreaInsets();
  const status: CaseStatus = "PENDING";

  const [uploadState, setUploadState] = useState<UploadState>({ status: "idle" });

  const canUpload = useMemo(() => {
    return uploadState.status !== "picking" && uploadState.status !== "initing" && uploadState.status !== "uploading";
  }, [uploadState.status]);

  async function pickAndUploadEvidence() {
  try {
    if (!API_BASE_URL) {
      setUploadState({
        status: "error",
        message: "Missing EXPO_PUBLIC_EVT_API_BASE_URL (device cannot reach 127.0.0.1).",
      });
      return;
    }

    setUploadState({ status: "picking" });

    const picked = await DocumentPicker.getDocumentAsync({
      type: ["application/pdf", "image/jpeg", "image/png"],
      copyToCacheDirectory: true,
      multiple: false,
    });

    if (picked.canceled) {
      setUploadState({ status: "idle" });
      return;
    }

    const asset = picked.assets?.[0];
    if (!asset?.uri || !asset.name) {
      setUploadState({ status: "error", message: "Document picker returned no file." });
      return;
    }

    const name = asset.name;
    const mimeType = asset.mimeType ?? guessMimeType(asset.name);
    const size = typeof asset.size === "number" ? asset.size : undefined;

    const ALLOWED_MIME_TYPES = [
      "application/pdf",
      "image/jpeg",
      "image/png",
    ] as const;

    if (!mimeType || !ALLOWED_MIME_TYPES.includes(mimeType as any)) {
      setUploadState({
        status: "error",
        message: "Unsupported file type. Use PDF, JPG, or PNG.",
      });
      return;
    }

    if (!size || size <= 0) {
      setUploadState({
        status: "error",
        message: "Missing or invalid file size (required for upload).",
      });
      return;
    }


    function guessMimeType(filename: string): string | null {
    const lower = filename.toLowerCase();
    if (lower.endsWith(".pdf")) return "application/pdf";
    if (lower.endsWith(".jpg") || lower.endsWith(".jpeg")) return "image/jpeg";
    if (lower.endsWith(".png")) return "image/png";
    return null;
  }

    if (!mimeType) {
      setUploadState({ status: "error", message: "Unsupported file type. Use PDF/JPG/PNG." });
      return;
    }
    if (!size || size <= 0) {
      setUploadState({ status: "error", message: "Missing file size from picker (required for policy)." });
      return;
    }

    setUploadState({ status: "initing" });

    const initUrl = `${API_BASE_URL}/v1/cases/${encodeURIComponent(CASE_ID)}/checks/${encodeURIComponent(
      CHECK_ID
    )}/evidence:init`;

    const initRes = await fetch(initUrl, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        caseId: CASE_ID,
        checkId: CHECK_ID,
        files: [{ name, mimeType, size }],
      }),
    });

    if (!initRes.ok) {
      const text = await initRes.text();
      setUploadState({ status: "error", message: `Init failed (${initRes.status}): ${text}` });
      return;
    }

    const initJson = (await initRes.json()) as EvidenceInitResponse;
    const upload = initJson.uploads?.[0];
    if (!upload?.url || !upload?.headers) {
      setUploadState({ status: "error", message: "Init response missing upload URL/headers." });
      return;
    }

    setUploadState({ status: "uploading" });

    const putRes = await FileSystem.uploadAsync(upload.url, asset.uri, {
      httpMethod: "PUT",
      headers: upload.headers,
      uploadType: FileSystem.FileSystemUploadType.BINARY_CONTENT,
    });

    if (putRes.status !== 200 && putRes.status !== 204) {
      setUploadState({
        status: "error",
        message: `PUT failed (${putRes.status}). Body: ${putRes.body ?? ""}`,
      });
      return;
    }

    setUploadState({ status: "done", storageKey: upload.storageKey });
  } catch (err: any) {
    const msg = String(err?.message ?? err ?? "");
    const friendly =
      msg.includes("Network request failed") ? "Cannot reach server. Check Wi-Fi/LAN URL." :
      msg.toLowerCase().includes("timed out") ? "Request timed out. Check server reachability or try again." :
      msg;

    setUploadState({ status: "error", message: friendly });
  }
}
  
  return (
    <View className="flex-1 bg-white dark:bg-zinc-900" style={{ paddingTop: insets.top, paddingLeft: insets.left, paddingRight: insets.right }}>
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
              <Text className="text-sm text-zinc-600 dark:text-zinc-300 mt-0.5">
                Submitted 2h ago
              </Text>
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
              <Text className="text-sm text-zinc-600 dark:text-zinc-400 mt-0.5">
                Aug 2023 — May 2025
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
              <Text className="text-xs text-zinc-500 dark:text-zinc-400">Upload evidence</Text>

              <View className="mt-2 flex-row items-center justify-between">
                <Text className="text-sm text-zinc-700 dark:text-zinc-200">PDF / JPG / PNG (max 5MB)</Text>

                <Pressable
                  disabled={!canUpload}
                  onPress={() => {
                    if (!API_BASE_URL) {
                      Alert.alert("Missing API base URL", "Set EXPO_PUBLIC_EVT_API_BASE_URL to a reachable host.");
                      return;
                    }
                    pickAndUploadEvidence();
                  }}
                  className={`rounded-xl px-4 py-2 ${canUpload ? "bg-zinc-700" : "bg-zinc-400"}`}
                >
                  <Text className="text-sm font-semibold text-white">
                    {uploadState.status === "uploading" ? "Uploading..." : "Choose file"}
                  </Text>
                </Pressable>
              </View>

              <View className="mt-3">
                {uploadState.status === "picking" || uploadState.status === "initing" || uploadState.status === "uploading" ? (
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
          <Footnote text="I consent to Certis processing uploaded evidence solely for the purpose of verifying employment claims. 
          Certis processes uploaded evidence solely to verify employment claims. 
          Raw evidence is retained only as long as necessary to complete verification and is then deleted. 
          Certis issues cryptographic verification tokens that do not expose underlying documents." />
          <View style={{ paddingBottom: insets.bottom }} /> 
            <DecisionBar
              bottomInset={insets.bottom}
              onApprove={() => console.log("Approve")}
              onReject={() => console.log("Reject")}
            />
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
      <Text className="text-3xl font-semibold text-zinc-950 dark:text-zinc-50 mt-1">
        HR Review
      </Text>
      <Text className="text-sm text-zinc-500 dark:text-zinc-300 mt-1">
        Validate employment details and decide whether to issue an EVT.
      </Text>
    </View>
  );
}

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

function StatusRow({ status }: { status: CaseStatus }) {
  const color =
    status === "APPROVED"
      ? "text-emerald-600"
      : status === "REJECTED"
      ? "text-rose-600"
      : "text-amber-600";

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
        <Pressable
          className="flex-1 rounded-xl bg-zinc-200 py-3 items-center"
          onPress={onReject}
        >
          <Text className="text-sm font-semibold text-zinc-900">Reject</Text>
        </Pressable>

        <Pressable
          className="flex-1 rounded-xl bg-zinc-700 py-3 items-center"
          onPress={onApprove}
        >
          <Text className="text-sm font-semibold text-white">Approve</Text>
        </Pressable>
      </View>
    </View>
  );
}
