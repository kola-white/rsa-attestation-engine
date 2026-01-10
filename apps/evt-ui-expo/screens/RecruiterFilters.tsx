// src/screens/RecruiterFilters.tsx

import React, { useLayoutEffect, useRef, useState } from "react";
import { View, Text, Pressable } from "react-native";
import { useNavigation, useRoute } from "@react-navigation/native";
import type { RouteProp } from "@react-navigation/native";
import type { NativeStackNavigationProp } from "@react-navigation/native-stack";
import type { RecruiterStackParamList, RecruiterQueryState } from "../src/navigation/recruiterTypes";

type R = RouteProp<RecruiterStackParamList, "RecruiterFilters">;
type N = NativeStackNavigationProp<RecruiterStackParamList, "RecruiterFilters">;

export function RecruiterFiltersScreen() {
  const navigation = useNavigation<N>();
  const route = useRoute<R>();
  const initial = route.params.initial;

  const [draft, setDraft] = useState<RecruiterQueryState>(initial);
  const initialRef = useRef(initial);

  function reset() {
    setDraft(initialRef.current);
  }

  function done() {
    // Phase 1 contract: swipe-down behaves exactly like Done.
    // We treat dismissal as apply, so Done just dismisses.
    navigation.goBack();
    // Applying the draft into list state is intentionally outside this screen in Phase 1;
    // use route.params.on_apply_id + shared store when you wire RecruiterCandidates state.
  }

  useLayoutEffect(() => {
    navigation.setOptions({
      title: "Filters",
      presentation: "modal",
      headerLeft: () => (
        <Pressable onPress={reset} accessibilityRole="button" className="px-2 py-1">
          <Text className="text-base font-semibold text-rose-600">Reset</Text>
        </Pressable>
      ),
      headerRight: () => (
        <Pressable onPress={done} accessibilityRole="button" className="px-2 py-1">
          <Text className="text-base font-semibold text-blue-600">Done</Text>
        </Pressable>
      ),
    });
  }, [navigation]);

  return (
    <View className="flex-1 bg-white dark:bg-zinc-900 px-5 pt-4">
      <Text className="text-base font-semibold text-zinc-950 dark:text-zinc-50">
        Trust (platform policy)
      </Text>

      <View className="mt-3">
        <Pressable
          onPress={() => setDraft((p) => ({ ...p, trust_mode: "trusted_only" }))}
          className="py-3"
        >
          <Text className="text-base text-zinc-900 dark:text-zinc-100">
            {draft.trust_mode === "trusted_only" ? "●" : "○"} Trusted issuers only
          </Text>
        </Pressable>

        <Pressable
          onPress={() => setDraft((p) => ({ ...p, trust_mode: "include_untrusted" }))}
          className="py-3"
        >
          <Text className="text-base text-zinc-900 dark:text-zinc-100">
            {draft.trust_mode === "include_untrusted" ? "●" : "○"} Allow untrusted issuers
          </Text>
        </Pressable>

        <Pressable
          onPress={() => setDraft((p) => ({ ...p, trust_mode: "any" }))}
          className="py-3"
        >
          <Text className="text-base text-zinc-900 dark:text-zinc-100">
            {draft.trust_mode === "any" ? "●" : "○"} Any
          </Text>
        </Pressable>
      </View>
    </View>
  );
}
