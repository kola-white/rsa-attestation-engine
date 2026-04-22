// screens/RequestorRequestDetailScreen.tsx
import React, { useLayoutEffect, useMemo, useState } from "react";
import {
  View,
  Text,
  ScrollView,
  Pressable,
  Alert,
  TextInput,
  Modal,
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
type N = NativeStackNavigationProp<
  RequestorStackParamList,
  "RequestorRequestDetail"
>;

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
  const [status, setStatus] = useState<RequestStatus>(
    () => snapshot?.status ?? "DRAFT",
  );
  const [busy, setBusy] = useState(false);
  const [isRequestInfoOpen, setIsRequestInfoOpen] = useState(false);

  useLayoutEffect(() => {
    navigation.setOptions({
      title: "",
      headerLargeTitle: false,
      headerBackTitle: "Back",
      headerBackButtonDisplayMode: "default",
      headerTintColor: undefined,
    });
  }, [navigation]);

  const canEdit = status === "DRAFT";
  const canSubmit = status === "DRAFT";
  const canCancel = status === "DRAFT";

  const apiUrl = useMemo(() => API_BASE_URL.replace(/\/$/, ""), []);

  const displayStatus = useMemo((): string => {
    if (status === "ATTESTED") return "Verified";
    return status;
  }, [status]);

  const createdAt = snapshot?.created_at ?? "Unavailable";
  const updatedAt = snapshot?.updated_at ?? "Unavailable";

  async function patchDraft() {
    if (!apiUrl) {
      Alert.alert("Config error", "EXPO_PUBLIC_EVT_API_BASE_URL is empty.");
      return;
    }
    if (!accessToken) {
      Alert.alert("Auth error", "Missing access token. Sign in again.");
      return;
    }

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
    if (!apiUrl) {
      Alert.alert("Config error", "EXPO_PUBLIC_EVT_API_BASE_URL is empty.");
      return;
    }
    if (!accessToken) {
      Alert.alert("Auth error", "Missing access token. Sign in again.");
      return;
    }

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
      Alert.alert(
        "Submitted",
        `Status: ${newStatus === "ATTESTED" ? "Verified" : newStatus}`,
      );
    } finally {
      setBusy(false);
    }
  }

  async function cancel() {
    if (!apiUrl) {
      Alert.alert("Config error", "EXPO_PUBLIC_EVT_API_BASE_URL is empty.");
      return;
    }
    if (!accessToken) {
      Alert.alert("Auth error", "Missing access token. Sign in again.");
      return;
    }

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
        setStatus("REJECTED");
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

  const lockedFieldTextClassName = canEdit
    ? "text-zinc-950 dark:text-zinc-50"
    : "text-zinc-700 dark:text-zinc-400";

  return (
    <View className="flex-1 bg-white dark:bg-zinc-900">
      <Modal
        visible={isRequestInfoOpen}
        transparent
        animationType="fade"
        onRequestClose={() => setIsRequestInfoOpen(false)}
      >
        <Pressable
          onPress={() => setIsRequestInfoOpen(false)}
          className="flex-1 bg-black/50 justify-center px-5"
        >
          <Pressable
            onPress={() => {}}
            className="rounded-2xl bg-white dark:bg-zinc-900 border border-zinc-200 dark:border-zinc-800 p-5"
          >
            <Text className="text-base font-semibold text-zinc-900 dark:text-zinc-50">
              Request information
            </Text>

            <View className="mt-4">
              <Text className="text-xs text-zinc-500 dark:text-zinc-400">
                Request ID
              </Text>
              <Text className="mt-1 text-sm text-zinc-900 dark:text-zinc-50">
                {request_id}
              </Text>
            </View>

            <View className="mt-4">
              <Text className="text-xs text-zinc-500 dark:text-zinc-400">
                Status
              </Text>
              <Text className="mt-1 text-sm text-zinc-900 dark:text-zinc-50">
                {displayStatus}
              </Text>
            </View>

            <View className="mt-4">
              <Text className="text-xs text-zinc-500 dark:text-zinc-400">
                Expected version
              </Text>
              <Text className="mt-1 text-sm text-zinc-900 dark:text-zinc-50">
                {String(expectedVersion)}
              </Text>
            </View>

            <View className="mt-4">
              <Text className="text-xs text-zinc-500 dark:text-zinc-400">
                Created
              </Text>
              <Text className="mt-1 text-sm text-zinc-900 dark:text-zinc-50">
                {createdAt}
              </Text>
            </View>

            <View className="mt-4">
              <Text className="text-xs text-zinc-500 dark:text-zinc-400">
                Updated
              </Text>
              <Text className="mt-1 text-sm text-zinc-900 dark:text-zinc-50">
                {updatedAt}
              </Text>
            </View>

            <Pressable
              onPress={() => setIsRequestInfoOpen(false)}
              className="mt-6 rounded-xl bg-zinc-900 dark:bg-zinc-100 px-4 py-3 items-center"
              style={({ pressed }) => [{ opacity: pressed ? 0.7 : 1 }]}
            >
              <Text className="font-semibold text-white dark:text-zinc-900">
                Close
              </Text>
            </Pressable>
          </Pressable>
        </Pressable>
      </Modal>

      <ScrollView contentInsetAdjustmentBehavior="automatic" className="px-5">
        <View className="mt-4 flex-row items-center">
          <Text className="text-xs text-zinc-500 dark:text-zinc-400">
            Request ID
          </Text>
          <Pressable
            onPress={() => setIsRequestInfoOpen(true)}
            accessibilityRole="button"
            accessibilityLabel="Request information"
            accessibilityHint="Shows request metadata"
            hitSlop={10}
            className="ml-2 h-5 w-5 items-center justify-center rounded-full border border-zinc-400 dark:border-zinc-500"
          >
            <Text className="text-[11px] font-semibold text-zinc-600 dark:text-zinc-300">
              i
            </Text>
          </Pressable>
        </View>

        <Text className="text-xs text-zinc-500 dark:text-zinc-400 mt-3">
          Status
        </Text>

        <View className="mt-2 self-start rounded-full border border-emerald-700 bg-emerald-100 px-3 py-1 dark:border-emerald-700 dark:bg-emerald-950">
          <Text className="text-sm font-semibold text-emerald-800 dark:text-emerald-300">
            {displayStatus}
          </Text>
        </View>

        <Text className="text-[12px] text-zinc-600 dark:text-zinc-400 mt-6 mb-2">
          Claims
        </Text>

        <TextInput
          editable={canEdit && !busy}
          value={claim.employer}
          onChangeText={(v) => setClaim((c) => ({ ...c, employer: v }))}
          placeholder="Employer (e.g., Cupertino Electric)"
          placeholderTextColor={canEdit ? undefined : "#71717a"}
          className={`px-4 py-3 rounded-xl border border-zinc-200 dark:border-zinc-800 ${lockedFieldTextClassName}`}
        />

        <TextInput
          editable={canEdit && !busy}
          value={claim.job_title}
          onChangeText={(v) => setClaim((c) => ({ ...c, job_title: v }))}
          placeholder="Job title (e.g., Senior Project Manager)"
          placeholderTextColor={canEdit ? undefined : "#71717a"}
          className={`mt-3 px-4 py-3 rounded-xl border border-zinc-200 dark:border-zinc-800 ${lockedFieldTextClassName}`}
        />

        <View className="flex-row gap-3 mt-3">
          <View className="flex-1">
            <TextInput
              editable={canEdit && !busy}
              value={claim.start_mm_yyyy}
              onChangeText={(v) =>
                setClaim((c) => ({ ...c, start_mm_yyyy: v }))
              }
              placeholder="Start (MM/YYYY)"
              placeholderTextColor={canEdit ? undefined : "#71717a"}
              className={`px-4 py-3 rounded-xl border border-zinc-200 dark:border-zinc-800 ${lockedFieldTextClassName}`}
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
              placeholderTextColor={canEdit ? undefined : "#71717a"}
              className={`px-4 py-3 rounded-xl border border-zinc-200 dark:border-zinc-800 ${lockedFieldTextClassName}`}
            />
          </View>
        </View>

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

      <SafeAreaView className="bg-white dark:bg-zinc-900">
        <View className="border-t border-zinc-200 dark:border-zinc-800 px-5 pt-3 pb-4">
          {!canEdit ? (
            <Text className="text-xs text-zinc-500 dark:text-zinc-400 mb-3">
              This claim can’t be edited after submission.
            </Text>
          ) : (
            <Text className="text-xs text-zinc-500 dark:text-zinc-400 mb-3">
              Review your claim, then submit when ready.
            </Text>
          )}

          <View className="flex-row gap-3">
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
              style={({ pressed }) => [
                { opacity: pressed && canCancel && !busy ? 0.7 : 1 },
              ]}
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

            <Pressable
              onPress={submit}
              disabled={!canSubmit || busy}
              className={[
                "flex-1 rounded-xl px-4 py-3 items-center",
                canSubmit && !busy
                  ? "bg-emerald-600"
                  : "bg-zinc-400 dark:bg-zinc-700",
              ].join(" ")}
              style={({ pressed }) => [
                { opacity: pressed && canSubmit && !busy ? 0.7 : 1 },
              ]}
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