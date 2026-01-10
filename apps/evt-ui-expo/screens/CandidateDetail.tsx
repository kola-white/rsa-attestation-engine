// src/screens/CandidateDetail.tsx

import React, { useLayoutEffect } from "react";
import { View, Text, ScrollView, ActivityIndicator } from "react-native";
import { useRoute, useNavigation } from "@react-navigation/native";
import type { RouteProp } from "@react-navigation/native";
import type { NativeStackNavigationProp } from "@react-navigation/native-stack";
import type { RecruiterStackParamList } from "../src/navigation/recruiterTypes";

import { useCandidateDetailGate } from "../src/navigation/useCandidateDetailGate";
import { VerificationLockScreen } from "../components/VerificationLockScreen";

type R = RouteProp<RecruiterStackParamList, "CandidateDetail">;
type N = NativeStackNavigationProp<RecruiterStackParamList, "CandidateDetail">;

const API_BASE_URL = process.env.EXPO_PUBLIC_EVT_API_BASE_URL ?? "";

export function CandidateDetailScreen() {
  const route = useRoute<R>();
  const navigation = useNavigation<N>();

  const { candidate_id, subject_ref, primary_evt_ref } = route.params;

  useLayoutEffect(() => {
    navigation.setOptions({
      title: subject_ref.full_name,
    });
  }, [navigation, subject_ref.full_name]);

  const gateResult = useCandidateDetailGate({
    apiBaseUrl: API_BASE_URL,
    candidateId: candidate_id,
    primaryEvtId: primary_evt_ref.evt_id,
  });

  if (gateResult.status === "checking") {
    return (
      <View className="flex-1 bg-white dark:bg-zinc-900 items-center justify-center">
        <ActivityIndicator />
        <Text className="text-sm text-zinc-600 dark:text-zinc-300 mt-3">
          Verifying…
        </Text>
      </View>
    );
  }

  // Enforce lock: protected content is only rendered when gate === ALLOW.
  if (gateResult.gate !== "ALLOW") {
    return (
      <VerificationLockScreen
        gate={gateResult.gate}
        whyCode={gateResult.outcome?.why?.code}
      />
    );
  }

  // Allowed: render protected detail content (minimal, no redesign).
  return (
    <View className="flex-1 bg-white dark:bg-zinc-900">
      <ScrollView contentInsetAdjustmentBehavior="automatic" className="px-5">
        <Text className="text-sm text-zinc-500 dark:text-zinc-400 mt-4">
          Candidate ID: {candidate_id}
        </Text>

        <Text className="text-xl font-semibold text-zinc-950 dark:text-zinc-50 mt-2">
          {subject_ref.full_name}
        </Text>

        {subject_ref.employee_id ? (
          <Text className="text-sm text-zinc-600 dark:text-zinc-300 mt-1">
            Employee ID: {subject_ref.employee_id}
          </Text>
        ) : null}

        <Text className="text-sm text-zinc-600 dark:text-zinc-300 mt-6 leading-6">
          Detail content is allowed to render because verification is trusted and valid.
        </Text>
      </ScrollView>
    </View>
  );
}
