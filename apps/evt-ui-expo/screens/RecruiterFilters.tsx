// src/screens/RecruiterFilters.tsx

import React, { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { View, Text, Pressable, ScrollView, Platform } from "react-native";
import { CommonActions, useNavigation, useRoute } from "@react-navigation/native";
import type { RouteProp } from "@react-navigation/native";
import type { NativeStackNavigationProp } from "@react-navigation/native-stack";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import type {
  RecruiterStackParamList,
  RecruiterQueryState,
} from "../src/navigation/recruiterTypes";

type R = RouteProp<RecruiterStackParamList, "RecruiterFilters">;
type N = NativeStackNavigationProp<RecruiterStackParamList, "RecruiterFilters">;

const FOOTER_ROW_HEIGHT = 56; // button row height (not including safe-area)
const FOOTER_VERTICAL_PADDING = 12; // top+bottom padding inside footer container

export function RecruiterFiltersScreen() {
  const navigation = useNavigation<N>();
  const route = useRoute<R>();
  const insets = useSafeAreaInsets();

  const initial = route.params.initial;
  const initialRef = useRef(cloneQuery(initial));

  const [draft, setDraft] = useState<RecruiterQueryState>(() => cloneQuery(initial));

  useEffect(() => {
    initialRef.current = cloneQuery(initial);
    setDraft(cloneQuery(initial));
  }, [initial]);

  function reset() {
    console.log("[RecruiterFilters] RESET pressed");
    setDraft(cloneQuery(initialRef.current));
  }

  function cloneQuery(q: RecruiterQueryState): RecruiterQueryState {
  return {
    ...q,
    signature_status: [...(q.signature_status ?? [])],
    company_ids: [...(q.company_ids ?? [])],
    page: q.page ? { ...q.page } : undefined,
    dates: q.dates ? { ...q.dates } : undefined,
  };
}

  function done() {
  const applied = { ...draft, page: undefined };
  console.log("[RecruiterFilters] DONE pressed. applied =", applied);

  // Find the route below this modal (should be RecruiterCandidates)
  const state = navigation.getState();
  console.log("[RecruiterFilters] nav state index =", state.index);
  const prev = state.routes[state.index - 1];
  console.log("[RecruiterFilters] prev route =", { name: prev?.name, key: prev?.key });

  if (prev?.name === "RecruiterCandidates") {
    // Set params on the *previous* route explicitly
    navigation.dispatch({
      ...CommonActions.setParams({ query: applied }),
      source: prev.key,
    });
  } else {
    // Fallback (shouldn’t normally happen)
    navigation.navigate("RecruiterCandidates", { query: applied });
  }

  navigation.goBack();
}

  useLayoutEffect(() => {
    navigation.setOptions({
      title: "Filters",
      presentation: "modal",

      // Ensure the native header is legible in light/dark and matches the screen background.
      headerStyle: {
        backgroundColor: Platform.select({
          ios: "transparent",
          default: "transparent",
        }),
      },

      // Tint affects header title + header buttons where applicable (native-stack).
      headerTintColor: undefined, // let native-stack pick based on theme, but we hard-set title style below

      // Force title visibility (prevents “Filters” blending into header background).
      headerTitleStyle: {
        color: "#18181b", // zinc-900
        fontSize: 17,
        fontWeight: "600",
      },

      // Large title tends to crowd content in modal stacks; keep this screen compact/Settings-like.
      headerLargeTitle: false,

      // We intentionally move actions to the sticky footer (always visible).
      headerLeft: undefined,
      headerRight: undefined,
      headerShadowVisible: false,
    });
  }, [navigation]);

  const bottomContentPadding = useMemo(() => {
    // ScrollView content must never sit under the sticky footer.
    // footer total = padding + row height + safe-area
    return (
      FOOTER_VERTICAL_PADDING +
      FOOTER_ROW_HEIGHT +
      Math.max(insets.bottom, 10) +
      16
    );
  }, [insets.bottom]);

  const scrollIndicatorBottomInset = useMemo(() => {
    return (
      FOOTER_VERTICAL_PADDING +
      FOOTER_ROW_HEIGHT +
      Math.max(insets.bottom, 10)
    );
  }, [insets.bottom]);

  return (
    <View className="flex-1 bg-white dark:bg-zinc-900">
      <ScrollView
        className="flex-1"
        contentInsetAdjustmentBehavior="automatic"
        // Keep comfortable spacing under the header/safe-area and ensure bottom is reachable above footer.
        contentContainerStyle={{ paddingBottom: bottomContentPadding }}
        scrollIndicatorInsets={{ bottom: scrollIndicatorBottomInset }}
        keyboardShouldPersistTaps="handled"
      >
        <View className="px-5 pt-4">
          <Text className="text-base font-semibold text-zinc-950 dark:text-zinc-50">
            Trust (platform policy)
          </Text>

          <View className="mt-3">
            <Pressable
              onPress={() => setDraft((p) => ({ ...p, trust_mode: "trusted_only" }))}
              accessibilityRole="button"
              className="py-3"
              style={({ pressed }) => [{ opacity: pressed ? 0.6 : 1 }]}
            >
              <Text className="text-base text-zinc-900 dark:text-zinc-100">
                {draft.trust_mode === "trusted_only" ? "●" : "○"} Trusted issuers only
              </Text>
            </Pressable>

            <Pressable
              onPress={() =>
                setDraft((p) => ({ ...p, trust_mode: "include_untrusted" }))
              }
              accessibilityRole="button"
              className="py-3"
              style={({ pressed }) => [{ opacity: pressed ? 0.6 : 1 }]}
            >
              <Text className="text-base text-zinc-900 dark:text-zinc-100">
                {draft.trust_mode === "include_untrusted" ? "●" : "○"} Allow untrusted issuers
              </Text>
            </Pressable>

            <Pressable
              onPress={() => setDraft((p) => ({ ...p, trust_mode: "any" }))}
              accessibilityRole="button"
              className="py-3"
              style={({ pressed }) => [{ opacity: pressed ? 0.6 : 1 }]}
            >
              <Text className="text-base text-zinc-900 dark:text-zinc-100">
                {draft.trust_mode === "any" ? "●" : "○"} Any
              </Text>
            </Pressable>
          </View>
        </View>
      </ScrollView>

      {/* Sticky footer action bar (always visible, safe-area aware) */}
      <View
        className="absolute bottom-0 left-0 right-0 border-t border-zinc-200 dark:border-zinc-800 bg-white/95 dark:bg-zinc-900/95"
        style={{ paddingBottom: Math.max(insets.bottom, 10) }}
      >
        <View className="px-5 pt-3 pb-3">
          <View className="flex-row items-center justify-between">
            {/* Reset (secondary) */}
            <Pressable
              onPress={reset}
              accessibilityRole="button"
              accessibilityLabel="Reset filters"
              className="h-14 flex-1 items-center justify-center rounded-2xl border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900"
              style={({ pressed }) => [{ opacity: pressed ? 0.7 : 1 }]}
            >
              <Text className="text-[17px] font-semibold text-rose-600">
                Reset
              </Text>
            </Pressable>

            <View className="w-3" />

            {/* Done (primary) */}
            <Pressable
              onPress={done}
              accessibilityRole="button"
              accessibilityLabel="Apply filters and close"
              className="h-14 flex-1 items-center justify-center rounded-2xl bg-blue-600"
              style={({ pressed }) => [{ opacity: pressed ? 0.8 : 1 }]}
            >
              <Text className="text-[17px] font-semibold text-white">Done</Text>
            </Pressable>
          </View>
        </View>
      </View>
    </View>
  );
}
