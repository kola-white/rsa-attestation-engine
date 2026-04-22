// screens/RequestorNewRequestScreen.tsx
import React, { useLayoutEffect, useMemo, useState } from "react";
import {
  Alert,
  KeyboardAvoidingView,
  Platform,
  Pressable,
  ScrollView,
  Text,
  TextInput,
  View,
} from "react-native";
import { useNavigation } from "@react-navigation/native";
import type { NativeStackNavigationProp } from "@react-navigation/native-stack";
import { SafeAreaView } from "react-native-safe-area-context";

import type { RequestorStackParamList, EmploymentClaimDraft } from "@/src/navigation/requestorTypes";
import { useAuth } from "@/src/auth/AuthContext";

type Nav = NativeStackNavigationProp<RequestorStackParamList, "RequestorNewRequest">;

const API_BASE_URL = process.env.EXPO_PUBLIC_EVT_API_BASE_URL ?? "";

type CreateDraftResp = { request_id: string; status: string };

export const RequestorNewRequestScreen: React.FC = () => {
  const navigation = useNavigation<Nav>();
  const { accessToken, user } = useAuth();

  const [employerId, setEmployerId] = useState("emp_demo_001");
  const [employerName, setEmployerName] = useState("ACME Electric (AEI)");
  const [jobTitle, setJobTitle] = useState("Senior Project Manager");
  const [startMmYyyy, setStartMmYyyy] = useState("08/2023");
  const [endMmYyyy, setEndMmYyyy] = useState("05/2025");

  const [submitting, setSubmitting] = useState(false);

  useLayoutEffect(() => {
  navigation.setOptions({
    title: "",
    headerLargeTitle: false,
    headerBackTitle: "Back",
    headerTintColor: "#0A84FF", // iOS blue
  });
}, [navigation]);

  const claimSnapshot: EmploymentClaimDraft = useMemo(
    () => ({
      employer: employerName,
      job_title: jobTitle,
      start_mm_yyyy: startMmYyyy,
      end_mm_yyyy: endMmYyyy.trim() ? endMmYyyy : null,
    }),
    [employerName, jobTitle, startMmYyyy, endMmYyyy]
  );

  async function onCreate() {
    if (!API_BASE_URL) {
      Alert.alert("Config error", "EXPO_PUBLIC_EVT_API_BASE_URL is empty.");
      return;
    }
    if (!accessToken) {
      Alert.alert("Not signed in", "Missing access token. Please sign in again.");
      return;
    }
    if (!employerId.trim()) {
      Alert.alert("Missing employer", "employer_id is required.");
      return;
    }

    try {
      setSubmitting(true);

      const res = await fetch(`${API_BASE_URL.replace(/\/$/, "")}/v1/requests`, {
        method: "POST",
        headers: {
          Accept: "application/json",
          "Content-Type": "application/json",
          Authorization: `Bearer ${accessToken}`,
        },
        body: JSON.stringify({
          employer_id: employerId.trim(),
          claim_snapshot: claimSnapshot,
        }),
      });

      const text = await res.text();
      const json = text ? (JSON.parse(text) as unknown) : null;

      if (!res.ok) {
        const err =
          (json as any)?.error ??
          (typeof text === "string" && text.length ? text : `HTTP ${res.status}`);
        Alert.alert("Create failed", String(err));
        return;
      }

      const out = json as CreateDraftResp;

      navigation.navigate("RequestorRequestDetail", {
        request_id: out.request_id,
        snapshot: {
          request_id: out.request_id,
          claim: claimSnapshot,
          status: (out.status as any) ?? "DRAFT",
          created_at: new Date().toISOString(),
          updated_at: new Date().toISOString(),
        },
      });
    } catch (e) {
      Alert.alert("Create failed", String(e));
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <KeyboardAvoidingView
      className="flex-1 bg-white dark:bg-zinc-900"
      behavior={Platform.OS === "ios" ? "padding" : undefined}
      keyboardVerticalOffset={Platform.OS === "ios" ? 64 : 0}
    >
      <ScrollView
        contentInsetAdjustmentBehavior="automatic"
        keyboardShouldPersistTaps="handled"
        className="px-5"
        contentContainerStyle={{ paddingBottom: 120 }} // room for sticky footer
      >
        <Text className="text-sm text-zinc-600 dark:text-zinc-300 mt-4">
          Signed in as {user?.email ?? "user"}
        </Text>

        <Text className="text-[12px] text-zinc-600 dark:text-zinc-400 mt-6 mb-2">
          EMPLOYER ID (REQUIRED)
        </Text>
        <TextInput
          value={employerId}
          onChangeText={setEmployerId}
          placeholder="emp_demo_001"
          autoCapitalize="none"
          className="px-4 py-3 rounded-xl border border-zinc-200 dark:border-zinc-800 text-zinc-950 dark:text-zinc-50"
        />

        <Text className="text-[12px] text-zinc-600 dark:text-zinc-400 mt-6 mb-2">
          CLAIM SNAPSHOT
        </Text>
        <TextInput
          value={employerName}
          onChangeText={setEmployerName}
          placeholder="Employer name"
          className="px-4 py-3 rounded-xl border border-zinc-200 dark:border-zinc-800 text-zinc-950 dark:text-zinc-50"
        />
        <TextInput
          value={jobTitle}
          onChangeText={setJobTitle}
          placeholder="Job title"
          className="mt-3 px-4 py-3 rounded-xl border border-zinc-200 dark:border-zinc-800 text-zinc-950 dark:text-zinc-50"
        />

        <View className="flex-row gap-3 mt-3">
          <View className="flex-1">
            <TextInput
              value={startMmYyyy}
              onChangeText={setStartMmYyyy}
              placeholder="Start (MM/YYYY)"
              className="px-4 py-3 rounded-xl border border-zinc-200 dark:border-zinc-800 text-zinc-950 dark:text-zinc-50"
            />
          </View>

          <View className="flex-1">
            <TextInput
              value={endMmYyyy}
              onChangeText={setEndMmYyyy}
              placeholder="End (MM/YYYY) or blank"
              className="px-4 py-3 rounded-xl border border-zinc-200 dark:border-zinc-800 text-zinc-950 dark:text-zinc-50"
            />
          </View>
        </View>
      </ScrollView>

      {/* Sticky safe-area footer */}
        <SafeAreaView
        edges={["bottom"]}
        className="bg-white dark:bg-zinc-900 w-full"
        >
        <View className="border-t border-zinc-200 dark:border-zinc-800 px-5 pt-3 pb-4 w-full">
            <Text className="text-xs text-zinc-500 dark:text-zinc-400 mb-3">
            Review your claim, then create when ready.
            </Text>

            <View className="flex-row gap-3 w-full">
            {/* Cancel */}
            <Pressable
                onPress={() => navigation.goBack()}
                disabled={submitting}
                className={[
                "flex-1 rounded-xl px-4 py-3 items-center",
                !submitting
                    ? "bg-zinc-200 dark:bg-zinc-800"
                    : "bg-zinc-100 dark:bg-zinc-900",
                ].join(" ")}
            >
                <Text
                className={[
                    "font-semibold",
                    !submitting
                    ? "text-zinc-900 dark:text-zinc-50"
                    : "text-zinc-400 dark:text-zinc-600",
                ].join(" ")}
                >
                Cancel
                </Text>
            </Pressable>

            {/* Save / Create */}
            <Pressable
                onPress={onCreate}
                disabled={submitting}
                className={[
                "flex-1 rounded-xl px-4 py-3 items-center",
                !submitting
                    ? "bg-zinc-900 dark:bg-zinc-100"
                    : "bg-zinc-400 dark:bg-zinc-700",
                ].join(" ")}
            >
                <Text className="font-semibold text-white dark:text-zinc-900">
                {submitting ? "Creating…" : "Create draft"}
                </Text>
            </Pressable>
            </View>
        </View>
        </SafeAreaView>
    </KeyboardAvoidingView>
  );
};
