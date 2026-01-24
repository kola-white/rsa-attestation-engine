// screens/RequestorNewRequestScreen.tsx
import React, { useLayoutEffect, useMemo, useState } from "react";
import { View, Text, TextInput, Pressable, ScrollView, Alert } from "react-native";
import { useNavigation } from "@react-navigation/native";
import type { NativeStackNavigationProp } from "@react-navigation/native-stack";

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
    navigation.setOptions({ title: "New Request" });
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
    <View className="flex-1 bg-white dark:bg-zinc-900">
      <ScrollView contentInsetAdjustmentBehavior="automatic" className="px-5">
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

        <Pressable
          onPress={onCreate}
          disabled={submitting}
          className="mt-8 rounded-xl bg-zinc-900 dark:bg-zinc-100 px-4 py-3"
          style={({ pressed }) => [{ opacity: pressed || submitting ? 0.7 : 1 }]}
        >
          <Text className="text-white dark:text-zinc-900 font-semibold">
            {submitting ? "Creating…" : "Create draft"}
          </Text>
        </Pressable>
      </ScrollView>
    </View>
  );
};
