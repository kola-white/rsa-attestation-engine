// screens/RequestorRequestDetailScreen.tsx
import React, { useLayoutEffect, useMemo, useState } from "react";
import {
  View,
  Text,
  ScrollView,
  Pressable,
  Alert,
  TextInput,
} from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { useNavigation, useRoute } from "@react-navigation/native";
import type { RouteProp } from "@react-navigation/native";
import type { NativeStackNavigationProp } from "@react-navigation/native-stack";

import type {
  RequestorStackParamList,
  EmploymentClaimDraft,
  RequestStatus,
} from "@/src/navigation/requestorTypes";
import { useAuth } from "@/src/auth/AuthContext";

type R = RouteProp<RequestorStackParamList, "RequestorRequestDetail">;
type N = NativeStackNavigationProp<RequestorStackParamList, "RequestorRequestDetail">;

const API_BASE_URL = process.env.EXPO_PUBLIC_EVT_API_BASE_URL ?? "";

export const RequestorRequestDetailScreen: React.FC = () => {
  const route = useRoute<R>();
  const navigation = useNavigation<N>();
  const { accessToken } = useAuth();

  const { request_id, snapshot } = route.params;

  const [claim, setClaim] = useState<EmploymentClaimDraft>(() => {
    return (
      snapshot?.claim ?? {
        employer: "",
        job_title: "",
        start_mm_yyyy: "",
        end_mm_yyyy: null,
      }
    );
  });

  const [expectedVersion, setExpectedVersion] = useState<number>(1);
  const [status, setStatus] = useState<RequestStatus>(() => snapshot?.status ?? "DRAFT");
  const [busy, setBusy] = useState(false);

  useLayoutEffect(() => {
    navigation.setOptions({
      title: "Status",
      headerLargeTitle: false,
    });
  }, [navigation]);

  const canEdit = status === "DRAFT";
  const canSubmit = status === "DRAFT";
  const canCancel = status === "DRAFT"; // API returns 409 after submit

  const apiUrl = useMemo(() => API_BASE_URL.replace(/\/$/, ""), []);

  async function patchDraft() {
    if (!apiUrl) return Alert.alert("Config error", "EXPO_PUBLIC_EVT_API_BASE_URL is empty.");
    if (!accessToken) return Alert.alert("Auth error", "Missing access token. Sign in again.");

    try {
      setBusy(true);

      const res = await fetch(`${apiUrl}/v1/requests/${request_id}`, {
        method: "PATCH",
        headers: {
          Accept: "application/json",
          "Content-Type": "application/json",
          Authorization: `Bearer ${accessToken}`,
        },
        body: JSON.stringify({
          expected_version: expectedVersion,
          claim_snapshot: claim,
        }),
      });

      if (res.status === 204) {
        setExpectedVersion((v) => v + 1);
        Alert.alert("Saved", "Changes saved.");
        return;
      }

      const text = await res.text();
      let err: string = text || `HTTP ${res.status}`;
      try {
        const j = JSON.parse(text);
        err = j?.error ?? err;
      } catch {}

      Alert.alert("Update failed", err);
    } finally {
      setBusy(false);
    }
  }

  async function submit() {
    if (!apiUrl) return Alert.alert("Config error", "EXPO_PUBLIC_EVT_API_BASE_URL is empty.");
    if (!accessToken) return Alert.alert("Auth error", "Missing access token. Sign in again.");

    try {
      setBusy(true);

      const res = await fetch(`${apiUrl}/v1/requests/${request_id}/submit`, {
        method: "POST",
        headers: {
          Accept: "application/json",
          Authorization: `Bearer ${accessToken}`,
        },
      });

      const text = await res.text();
      if (!res.ok) {
        let err: string = text || `HTTP ${res.status}`;
        try {
          const j = JSON.parse(text);
          err = j?.error ?? err;
        } catch {}
        Alert.alert("Submit failed", err);
        return;
      }

      const j = text ? JSON.parse(text) : null;
      const newStatus = (j?.status as RequestStatus) ?? "SUBMITTED";
      setStatus(newStatus);
      Alert.alert("Submitted", `Status: ${newStatus}`);
    } finally {
      setBusy(false);
    }
  }

  async function cancel() {
    if (!apiUrl) return Alert.alert("Config error", "EXPO_PUBLIC_EVT_API_BASE_URL is empty.");
    if (!accessToken) return Alert.alert("Auth error", "Missing access token. Sign in again.");

    try {
      setBusy(true);

      const res = await fetch(`${apiUrl}/v1/requests/${request_id}/cancel`, {
        method: "POST",
        headers: {
          Accept: "application/json",
          Authorization: `Bearer ${accessToken}`,
        },
      });

      if (res.status === 204) {
        setStatus("REJECTED"); // until you add CANCELLED to union
        Alert.alert("Cancelled", "Request cancelled.");
        return;
      }

      const text = await res.text();
      let err: string = text || `HTTP ${res.status}`;
      try {
        const j = JSON.parse(text);
        err = j?.error ?? err;
      } catch {}

      Alert.alert("Cancel failed", err);
    } finally {
      setBusy(false);
    }
  }

  return (
    <View className="flex-1 bg-white dark:bg-zinc-900">
      {/* Scroll content */}
      <ScrollView contentInsetAdjustmentBehavior="automatic" className="px-5">
        <Text className="text-xs text-zinc-500 dark:text-zinc-400 mt-4">Request ID</Text>
        <Text className="text-sm text-zinc-900 dark:text-zinc-50">{request_id}</Text>

        <Text className="text-xs text-zinc-500 dark:text-zinc-400 mt-3">Status</Text>
        <Text className="text-base font-semibold text-zinc-900 dark:text-zinc-50">{status}</Text>

        <Text className="text-[12px] text-zinc-600 dark:text-zinc-400 mt-6 mb-2">
          CLAIM {canEdit ? "(DRAFT)" : "(LOCKED)"}
        </Text>

        <TextInput
          editable={canEdit && !busy}
          value={claim.employer}
          onChangeText={(v) => setClaim((c) => ({ ...c, employer: v }))}
          placeholder="Employer (e.g., Cupertino Electric)"
          className="px-4 py-3 rounded-xl border border-zinc-200 dark:border-zinc-800 text-zinc-950 dark:text-zinc-50"
        />

        <TextInput
          editable={canEdit && !busy}
          value={claim.job_title}
          onChangeText={(v) => setClaim((c) => ({ ...c, job_title: v }))}
          placeholder="Job title (e.g., Senior Project Manager)"
          className="mt-3 px-4 py-3 rounded-xl border border-zinc-200 dark:border-zinc-800 text-zinc-950 dark:text-zinc-50"
        />

        <View className="flex-row gap-3 mt-3">
          <View className="flex-1">
            <TextInput
              editable={canEdit && !busy}
              value={claim.start_mm_yyyy}
              onChangeText={(v) => setClaim((c) => ({ ...c, start_mm_yyyy: v }))}
              placeholder="Start (MM/YYYY)"
              className="px-4 py-3 rounded-xl border border-zinc-200 dark:border-zinc-800 text-zinc-950 dark:text-zinc-50"
            />
          </View>

          <View className="flex-1">
            <TextInput
              editable={canEdit && !busy}
              value={claim.end_mm_yyyy ?? ""}
              onChangeText={(v) =>
                setClaim((c) => ({ ...c, end_mm_yyyy: v.trim() ? v : null }))
              }
              placeholder="End (MM/YYYY) — leave blank if current"
              className="px-4 py-3 rounded-xl border border-zinc-200 dark:border-zinc-800 text-zinc-950 dark:text-zinc-50"
            />
          </View>
        </View>

        <Text className="text-xs text-zinc-500 dark:text-zinc-400 mt-3">

        </Text>

        {/* Spacer so content doesn't hide behind footer */}
        <View className="h-28" />
      </ScrollView>

      {canEdit && (
      <Pressable
        onPress={patchDraft}
        disabled={busy}
        className="mt-6 rounded-xl bg-zinc-900 dark:bg-zinc-100 px-4 py-3 items-center"
        style={({ pressed }) => [{ opacity: pressed && !busy ? 0.7 : 1 }]}
      >
        <Text className="font-semibold text-white dark:text-zinc-900">
          {busy ? "Working…" : "Save changes"}
        </Text>
      </Pressable>
    )}

      {/* Sticky safe-area footer (Submit + Cancel side-by-side) */}
      <SafeAreaView className="bg-white dark:bg-zinc-900">
        <View className="border-t border-zinc-200 dark:border-zinc-800 px-5 pt-3 pb-4">
          {!canEdit ? (
            <Text className="text-xs text-zinc-500 dark:text-zinc-400 mb-3">
              Submitted requests can`t be edited or canceled.
            </Text>
          ) : (
            <Text className="text-xs text-zinc-500 dark:text-zinc-400 mb-3">
              Review your claim, then submit when ready.
            </Text>
          )}

          <View className="flex-row gap-3">
            {/* Cancel (left) */}
            <Pressable
              onPress={() =>
                Alert.alert("Cancel request?", "This cannot be undone.", [
                  { text: "No", style: "cancel" },
                  { text: "Yes, cancel", style: "destructive", onPress: cancel },
                ])
              }
              disabled={!canCancel || busy}
              className={[
                "flex-1 rounded-xl px-4 py-3 items-center",
                canCancel && !busy
                  ? "bg-zinc-200 dark:bg-zinc-800"
                  : "bg-zinc-100 dark:bg-zinc-900",
              ].join(" ")}
              style={({ pressed }) => [{ opacity: pressed && canCancel && !busy ? 0.7 : 1 }]}
            >
              <Text
                className={[
                  "font-semibold",
                  canCancel && !busy
                    ? "text-zinc-900 dark:text-zinc-50"
                    : "text-zinc-400 dark:text-zinc-600",
                ].join(" ")}
              >
                {canCancel ? "Cancel" : "Can’t cancel"}
              </Text>
            </Pressable>

            {/* Submit (right) */}
            <Pressable
              onPress={submit}
              disabled={!canSubmit || busy}
              className={[
                "flex-1 rounded-xl px-4 py-3 items-center",
                canSubmit && !busy ? "bg-emerald-600" : "bg-zinc-400 dark:bg-zinc-700",
              ].join(" ")}
              style={({ pressed }) => [{ opacity: pressed && canSubmit && !busy ? 0.7 : 1 }]}
            >
              <Text className="text-white font-semibold">
                {canSubmit ? "Submit request" : "Submitted"}
              </Text>
            </Pressable>
          </View>
        </View>
      </SafeAreaView>
    </View>
  );
};
