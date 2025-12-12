import React, { useState } from "react";
import { View, Text, ScrollView, Pressable, StyleSheet } from "react-native";
import { StatusBar } from "expo-status-bar";
import { SafeAreaView, useSafeAreaInsets } from "react-native-safe-area-context";

export default function HRReviewScreen() {
  const insets = useSafeAreaInsets();
  const status: "PENDING" | "APPROVED" | "REJECTED" = "PENDING";

  return (
    <SafeAreaView style={styles.safe} edges={["top"]}>
      <StatusBar style="dark" />

      <ScrollView
        contentContainerStyle={[
          styles.container,
          { paddingBottom: 24 + 72 + insets.bottom }, // room for sticky DecisionBar
        ]}
      >
        {/* HeaderBar */}
        <HeaderBar />

        {/* IdentityRow */}
        <IdentityRow
          status={status}
          caseId="EVT-10324"
          submittedAt="Submitted 2h ago"
          name="Jane Doe"
          internalIdLabel="Worker ID"
          internalIdValue="CEI-48219"
        />

        {/* ClaimCard */}
        <ClaimCard
          employer="ACME Electric (AEI)"
          role="Senior Project Manager"
          dates="Aug 2023 — May 2025"
        />

        {/* DisclosureSections (Accordion) */}
        <DisclosureSections />
      </ScrollView>

      {/* DecisionBar (sticky) */}
      <DecisionBar
        bottomInset={insets.bottom}
        onApprove={() => console.log("Approve")}
        onReject={() => console.log("Reject")}
      />
    </SafeAreaView>
  );
}

/** HeaderBar */
function HeaderBar() {
  return (
    <View style={styles.header}>
      <Text style={styles.kicker}>EVT · Internal Tool</Text>
      <Text style={styles.title}>HR Review — Employment Verification</Text>
      <Text style={styles.subtitle}>
        Review incoming claims, confirm employment details, and approve or reject
        EVT issuance.
      </Text>
    </View>
  );
}

/** IdentityRow */
function IdentityRow({
  status,
  caseId,
  submittedAt,
  name,
  internalIdLabel,
  internalIdValue,
}: {
  status: "PENDING" | "APPROVED" | "REJECTED";
  caseId: string;
  submittedAt: string;
  name: string;
  internalIdLabel: string;
  internalIdValue: string;
}) {
  return (
    <View style={styles.card}>
      <View style={styles.rowBetween}>
        <View style={{ flex: 1 }}>
          <Text style={styles.cardTitle}>
            {caseId} · {submittedAt}
          </Text>
          <Text style={styles.person}>{name}</Text>
          <Text style={styles.meta}>
            {internalIdLabel}:{" "}
            <Text style={{ fontWeight: "700" }}>{internalIdValue}</Text>
          </Text>
        </View>

        <View style={styles.badge}>
          <Text style={styles.badgeText}>{status}</Text>
        </View>
      </View>
    </View>
  );
}

/** ClaimCard (EVT wedge facts) */
function ClaimCard({
  employer,
  role,
  dates,
}: {
  employer: string;
  role: string; // consider optional later
  dates: string;
}) {
  return (
    <View style={[styles.card, { marginTop: 12 }]}>
      <Text style={styles.cardTitle}>Claim</Text>
      <Text style={styles.person}>{employer}</Text>
      <Text style={styles.role}>{role}</Text>
      <Text style={styles.meta}>{dates}</Text>
    </View>
  );
}

/** DisclosureSections */
function DisclosureSections() {
  return (
    <View style={[styles.card, { marginTop: 12 }]}>
      <Accordion
        title="Request details"
        body={[
          "Requester: Mortgage / Loan",
          "Purpose: Employment verification",
          "Consent: On file",
        ]}
      />

      <Divider />

      <Accordion
        title="Verification checks"
        body={["Tenure matches HRIS", "Title matches HRIS"]}
      />
    </View>
  );
}

function Accordion({ title, body }: { title: string; body: string[] }) {
  const [open, setOpen] = useState(false);

  return (
    <View style={{ paddingTop: 12 }}>
      <Pressable onPress={() => setOpen((v) => !v)} style={styles.rowBetween}>
        <Text style={styles.sectionLabel}>{title.toUpperCase()}</Text>
        <Text style={[styles.sectionLabel, { opacity: 0.5 }]}>
          {open ? "HIDE" : "SHOW"}
        </Text>
      </Pressable>

      {open ? (
        <View style={{ marginTop: 10 }}>
          {body.map((line, i) => (
            <Text key={i} style={styles.sectionBody}>
              • {line}
            </Text>
          ))}
        </View>
      ) : null}
    </View>
  );
}

/** DecisionBar (sticky) */
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
      style={[
        styles.decisionBar,
        { paddingBottom: Math.max(bottomInset, 12) },
      ]}
    >
      <View style={styles.actionsRow}>
        <Pressable style={[styles.button, styles.secondary]} onPress={onReject}>
          <Text style={[styles.buttonText, styles.secondaryText]}>Reject</Text>
        </Pressable>

        <Pressable style={[styles.button, styles.primary]} onPress={onApprove}>
          <Text style={[styles.buttonText, styles.primaryText]}>Approve</Text>
        </Pressable>
      </View>
    </View>
  );
}

function Divider() {
  return <View style={styles.divider} />;
}

const styles = StyleSheet.create({
  safe: { flex: 1, backgroundColor: "#fff" },
  container: { padding: 16 },

  header: { marginBottom: 16, gap: 6 },
  kicker: { fontSize: 12, opacity: 0.65, letterSpacing: 0.6 },
  title: { fontSize: 20, fontWeight: "700", lineHeight: 26 },
  subtitle: { fontSize: 14, opacity: 0.7, lineHeight: 20 },

  card: {
    borderWidth: 1,
    borderColor: "rgba(0,0,0,0.08)",
    borderRadius: 16,
    padding: 16,
    backgroundColor: "#fff",
  },

  rowBetween: {
    flexDirection: "row",
    justifyContent: "space-between",
    gap: 12,
    alignItems: "flex-start",
  },

  cardTitle: { fontSize: 14, fontWeight: "700" },
  person: { marginTop: 6, fontSize: 16, fontWeight: "700" },
  role: { marginTop: 2, fontSize: 14, opacity: 0.8 },
  meta: { marginTop: 6, fontSize: 13, opacity: 0.65 },

  badge: {
    alignSelf: "flex-start",
    paddingVertical: 6,
    paddingHorizontal: 10,
    borderRadius: 999,
    backgroundColor: "rgba(0,0,0,0.06)",
  },
  badgeText: {
    fontSize: 12,
    fontWeight: "700",
    letterSpacing: 0.5,
    opacity: 0.8,
  },

  divider: {
    height: 1,
    backgroundColor: "rgba(0,0,0,0.08)",
    marginTop: 14,
  },

  sectionLabel: {
    fontSize: 12,
    fontWeight: "700",
    opacity: 0.65,
    letterSpacing: 0.7,
  },
  sectionBody: {
    marginTop: 6,
    fontSize: 14,
    lineHeight: 20,
    opacity: 0.9,
  },

  decisionBar: {
    position: "absolute",
    left: 0,
    right: 0,
    bottom: 0,
    backgroundColor: "#fff",
    borderTopWidth: 1,
    borderTopColor: "rgba(0,0,0,0.08)",
    paddingHorizontal: 16,
    paddingTop: 12,
  },
  actionsRow: { flexDirection: "row", gap: 12 },

  button: {
    flex: 1,
    borderRadius: 12,
    paddingVertical: 12,
    paddingHorizontal: 14,
    alignItems: "center",
  },
  primary: { backgroundColor: "#111" },
  primaryText: { color: "#fff" },
  secondary: { backgroundColor: "rgba(0,0,0,0.06)" },
  secondaryText: { color: "#111" },
  buttonText: { fontSize: 14, fontWeight: "700" },
});
