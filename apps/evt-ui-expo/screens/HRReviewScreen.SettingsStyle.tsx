import React, { useState } from "react";
import { View, Text, ScrollView, Pressable, } from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";

type CaseStatus = "PENDING" | "APPROVED" | "REJECTED";

export default function HRReviewScreenSettingsStyle() {
  const insets = useSafeAreaInsets();
  const status: CaseStatus = "PENDING";
  
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
