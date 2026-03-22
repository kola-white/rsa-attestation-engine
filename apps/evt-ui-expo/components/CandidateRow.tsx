import React, { memo, useMemo } from "react";
import { Pressable, Text, View } from "react-native";
import type { CandidateRowSnapshot } from "@/src/navigation/recruiterTypes";

type TrustBadge = "trusted" | "untrusted" | "unknown";
type VerificationState = "verified" | "unverified" | "pending" | "unknown";

function shortId(id?: string, keep = 8) {
  if (!id) return "";
  const s = String(id);
  return s.length <= keep ? s : s.slice(0, keep);
}

function safeText(v?: string | null) {
  return (v ?? "").trim();
}

/** Lightweight "time ago" without deps. If parsing fails, returns empty string. */
function timeAgo(iso?: string) {
  if (!iso) return "";
  const t = Date.parse(iso);
  if (Number.isNaN(t)) return "";
  const diffSec = Math.max(0, Math.floor((Date.now() - t) / 1000));
  if (diffSec < 60) return `${diffSec}s ago`;
  const diffMin = Math.floor(diffSec / 60);
  if (diffMin < 60) return `${diffMin}m ago`;
  const diffHr = Math.floor(diffMin / 60);
  if (diffHr < 48) return `${diffHr}h ago`;
  const diffDay = Math.floor(diffHr / 24);
  return `${diffDay}d ago`;
}

function VerificationChip({ value }: { value: VerificationState }) {
  const cls =
    value === "verified"
      ? "bg-emerald-100 dark:bg-emerald-900/40 border-emerald-200 dark:border-emerald-800"
      : value === "unverified"
        ? "bg-red-100 dark:bg-red-900/40 border-red-200 dark:border-red-800"
        : value === "pending"
          ? "bg-amber-100 dark:bg-amber-500 border-amber-200 dark:border-amber-800"
          : "bg-amber-100 dark:bg-amber-500 border-amber-200 dark:border-amber-800";

  const textCls =
    value === "verified"
      ? "text-emerald-700 dark:text-emerald-200"
      : value === "unverified"
        ? "text-red-700 dark:text-red-200"
        : value === "pending"
          ? "text-amber-500 dark:text-amber-100"
          : "text-amber-500 dark:text-amber-100";

  const label =
    value === "verified"
      ? "Verified"
      : value === "unverified"
        ? "Unverified"
        : value === "pending"
          ? "Pending"
          : "Unknown";

  return (
    <View className={`px-2 py-1 rounded-full border ${cls}`}>
      <Text className={`text-[12px] font-medium ${textCls}`}>{label}</Text>
    </View>
  );
}

function TrustLine({ trust }: { trust: TrustBadge }) {
  const label =
    trust === "trusted"
      ? "Trusted issuer"
      : trust === "untrusted"
        ? "Untrusted issuer"
        : "Issuer trust unknown";

  const cls =
    trust === "trusted"
      ? "text-emerald-600 dark:text-emerald-300"
      : trust === "untrusted"
        ? "text-red-600 dark:text-red-300"
        : "text-zinc-500 dark:text-zinc-400";

  return <Text className={`text-[12px] ${cls}`}>{label}</Text>;
}

function VerificationLine({ state }: { state?: VerificationState }) {
  const resolved = state ?? "unknown";

  const label =
    resolved === "verified"
      ? "Verified"
      : resolved === "unverified"
        ? "Unverified"
        : resolved === "pending"
          ? "Pending verification"
          : "Verification unavailable";

  return (
    <Text className="mt-1 text-[12px] text-zinc-500 dark:text-zinc-400">
      {label}
    </Text>
  );
}

export const CandidateRow = memo(function CandidateRow({
  item,
  onPress,
}: {
  item: CandidateRowSnapshot;
  onPress: (item: CandidateRowSnapshot) => void;
}) {
  const fullName = safeText(item.subject?.full_name);
  const employeeId = safeText(item.subject?.employee_id ?? "");
  const title = safeText(item.primary_employment?.title);
  const issuer = safeText(item.primary_employment?.issuer_name);

  const trust = (item.badges?.trust ?? "unknown") as TrustBadge;
  const verificationState = (item.verification?.state ?? "unknown") as VerificationState;

  const updated = useMemo(() => timeAgo(item.updated_at), [item.updated_at]);
  const evtShort = useMemo(() => shortId(item.primary_evt?.evt_id, 8), [item.primary_evt?.evt_id]);

  const displayName =
    fullName || (employeeId ? `Employee ${employeeId}` : `Candidate ${shortId(item.candidate_id, 8)}`);

  const subLeft =
    title || issuer ? [title, issuer].filter(Boolean).join(" · ") : "No employment details provided";

  return (
    <Pressable
      onPress={() => onPress(item)}
      style={({ pressed }) => [{ opacity: pressed ? 0.7 : 1 }]}
      className="mx-4 mb-3 rounded-2xl border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 overflow-hidden"
    >
      <View className="p-4">
        <View className="flex-row items-center justify-between">
          <VerificationChip value={verificationState} />
          {!!updated && <Text className="text-[12px] text-zinc-500 dark:text-zinc-400">{updated}</Text>}
        </View>

        <Text numberOfLines={1} className="mt-3 text-[16px] font-semibold text-zinc-900 dark:text-zinc-100">
          {displayName}
        </Text>

        <Text numberOfLines={2} className="mt-1 text-[13px] text-zinc-600 dark:text-zinc-300">
          {subLeft}
        </Text>

        <View className="mt-2">
          <TrustLine trust={trust} />
          <VerificationLine state={verificationState} />
        </View>

        <View className="mt-3 flex-row items-center justify-between">
          <Text className="text-[11px] text-zinc-500 dark:text-zinc-500">EVT {evtShort || "—"}</Text>
          <Text className="text-[11px] text-zinc-500 dark:text-zinc-500">
            Candidate {shortId(item.candidate_id, 8) || "—"}
          </Text>
        </View>
      </View>
    </Pressable>
  );
});