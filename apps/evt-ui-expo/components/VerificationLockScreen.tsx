// src/components/VerificationLockScreen.tsx

import React from "react";
import { View, Text, Pressable } from "react-native";
import { useNavigation } from "@react-navigation/native";
import type { NativeStackNavigationProp } from "@react-navigation/native-stack";
import type { RecruiterStackParamList } from "@/src/navigation/recruiterTypes";
import { VERIFICATION_GATE_MACHINE, type GateState } from "../src/navigation/verificationGateMachine";

type Nav = NativeStackNavigationProp<RecruiterStackParamList, "CandidateDetail">;

export function VerificationLockScreen(props: {
  gate: GateState;
  whyCode?: string;
}) {
  const navigation = useNavigation<Nav>();
  const state = VERIFICATION_GATE_MACHINE.states[props.gate];
  const ux = state.ux;

  function runAction(action: "GO_BACK" | "OPEN_FILTERS" | "NONE") {
    if (action === "NONE") return;
    if (action === "GO_BACK") {
      navigation.goBack();
      return;
    }
    if (action === "OPEN_FILTERS") {
      // Deterministic: we always open filters from detail by presenting modal.
      // CandidateDetail is in the same stack as the modal route.
      navigation.navigate("RecruiterFilters", {
        initial: {
          search: "",
          trust_mode: "any",
          signature_status: ["verified", "invalid", "unknown"],
          company_ids: [],
        },
      });
      return;
    }
  }

  return (
    <View className="flex-1 bg-white dark:bg-zinc-900 px-5 justify-center">
      <Text className="text-2xl font-semibold text-zinc-950 dark:text-zinc-50">
        {ux.title}
      </Text>

      <Text className="text-base text-zinc-600 dark:text-zinc-300 mt-3 leading-6">
        {ux.message}
      </Text>

      {props.whyCode ? (
        <Text className="text-sm text-zinc-500 dark:text-zinc-400 mt-3">
          Code: {props.whyCode}
        </Text>
      ) : null}

      <View className="mt-6">
        <Pressable
          onPress={() => runAction(ux.primaryCta.action)}
          accessibilityRole="button"
          className="w-full rounded-xl bg-zinc-900 dark:bg-zinc-100 px-4 py-3 items-center"
        >
          <Text className="text-base font-semibold text-white dark:text-zinc-900">
            {ux.primaryCta.text}
          </Text>
        </Pressable>

        {ux.secondaryCta ? (
          <Pressable
            onPress={() => runAction(ux.secondaryCta!.action)}
            accessibilityRole="button"
            className="w-full rounded-xl bg-zinc-200 dark:bg-zinc-800 px-4 py-3 items-center mt-3"
          >
            <Text className="text-base font-semibold text-zinc-900 dark:text-zinc-100">
              {ux.secondaryCta.text}
            </Text>
          </Pressable>
        ) : null}
      </View>
    </View>
  );
}
