// src/screens/RecruiterCandidates.tsx

import React, { useLayoutEffect, useMemo, useState } from "react";
import { View, Text, Pressable, ScrollView } from "react-native";
import { useNavigation } from "@react-navigation/native";
import type { NativeStackNavigationProp } from "@react-navigation/native-stack";
import type { RecruiterStackParamList, CandidateRowSnapshot } from "../src/navigation/recruiterTypes";

type Nav = NativeStackNavigationProp<RecruiterStackParamList, "RecruiterCandidates">;

export function RecruiterCandidatesScreen() {
  const navigation = useNavigation<Nav>();

  useLayoutEffect(() => {
    navigation.setOptions({
      title: "Candidates",

    headerTitleStyle: {
      color: "#e5e7eb",   // zinc-200 (light gray, Apple-ish)
      fontWeight: "600",
    },

    // iOS large title color too
    headerLargeTitleStyle: {
      color: "#e5e7eb",
    },

      headerRight: () => (
        <Pressable
          onPress={() =>
            navigation.navigate("RecruiterFilters", {
              initial: {
                search: "",
                trust_mode: "any",
                signature_status: ["verified", "invalid", "unknown"],
                company_ids: [],
                sort: "most_recent",
              },
            })
          }
          accessibilityRole="button"
          className="px-3 py-1 rounded-lg bg-zinc-200 dark:bg-zinc-800"
        >
          <Text className="text-sm font-semibold text-zinc-900 dark:text-zinc-100">
            Filter
          </Text>
        </Pressable>
      ),
    });
  }, [navigation]);

  const [items] = useState<CandidateRowSnapshot[]>([
    {
      candidate_id: "cand_001",
      subject: { full_name: "Michael Smith", employee_id: "CEI-48219" },
      primary_employment: {
        issuer_name: "NorStar Tech",
        title: "Project Manager",
        start_date: "2020-04-01",
        end_date: null,
      },
      primary_evt: { evt_id: "evt_01HY_demo" },
      badges: { signature: "verified", trust: "untrusted" },
      updated_at: new Date().toISOString(),
    },
  ]);

  const rows = useMemo(() => items, [items]);

  return (
    <View className="flex-1 bg-white dark:bg-zinc-900">
      <ScrollView contentInsetAdjustmentBehavior="automatic" className="px-5">
        {rows.map((r) => (
          <Pressable
            key={r.candidate_id}
            onPress={() =>
              navigation.push("CandidateDetail", {
                candidate_id: r.candidate_id,
                subject_ref: r.subject,
                primary_evt_ref: r.primary_evt,
                prefetch_snapshot: r,
              })
            }
            className="py-4 border-b border-zinc-200 dark:border-zinc-800"
          >
            <Text className="text-base font-semibold text-zinc-950 dark:text-zinc-50">
              {r.subject.full_name}
            </Text>
            <Text className="text-sm text-zinc-600 dark:text-zinc-300 mt-1">
              {r.primary_employment.issuer_name} • {r.primary_employment.title}
            </Text>
            <Text className="text-sm text-zinc-500 dark:text-zinc-400 mt-1">
              Signature: {r.badges.signature} • Trust: {r.badges.trust}
            </Text>
          </Pressable>
        ))}
      </ScrollView>
    </View>
  );
}