import React, { useLayoutEffect, useMemo } from "react";
import { View, Text, Pressable, ScrollView, Platform } from "react-native";
import { useNavigation } from "@react-navigation/native";
import type { NativeStackNavigationProp } from "@react-navigation/native-stack";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import type { RecruiterStackParamList } from "@/src/navigation/recruiterTypes";
import {
  useRecruiterFiltersStore,
} from "@/src/state/recruiterFiltersStore";

type N = NativeStackNavigationProp<RecruiterStackParamList, "RecruiterFilters">;

const FOOTER_ROW_HEIGHT = 56;
const FOOTER_VERTICAL_PADDING = 12;

export function RecruiterFiltersScreen() {
  const navigation = useNavigation<N>();
  const insets = useSafeAreaInsets();

  const draft = useRecruiterFiltersStore((s) => s.draft);
  const setDraft = useRecruiterFiltersStore((s) => s.setDraft);
  const resetDraftToDefaults = useRecruiterFiltersStore((s) => s.resetDraftToDefaults);
  const applyDraft = useRecruiterFiltersStore((s) => s.applyDraft);

  function reset() {
    console.log("[RecruiterFilters] RESET pressed");
    resetDraftToDefaults();
  }

  function done() {
    console.log("[RecruiterFilters] DONE pressed. committing draft -> applied =", draft);
    applyDraft();
    navigation.goBack();
  }

  useLayoutEffect(() => {
    navigation.setOptions({
      title: "Filters",
      presentation: "modal",
      headerStyle: {
        backgroundColor: Platform.select({
          ios: "transparent",
          default: "transparent",
        }),
      },
      headerTintColor: undefined,
      headerTitleStyle: {
        color: "#18181b",
        fontSize: 17,
        fontWeight: "600",
      },
      headerLargeTitle: false,
      headerLeft: undefined,
      headerRight: undefined,
      headerShadowVisible: false,
    });
  }, [navigation]);

  const bottomContentPadding = useMemo(() => {
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
              onPress={() => setDraft({ trust_mode: "trusted_only" })}
              accessibilityRole="button"
              className="py-3"
              style={({ pressed }) => [{ opacity: pressed ? 0.6 : 1 }]}
            >
              <Text className="text-base text-zinc-900 dark:text-zinc-100">
                {draft.trust_mode === "trusted_only" ? "●" : "○"} Trusted issuers only
              </Text>
            </Pressable>

            <Pressable
              onPress={() => setDraft({ trust_mode: "include_untrusted" })}
              accessibilityRole="button"
              className="py-3"
              style={({ pressed }) => [{ opacity: pressed ? 0.6 : 1 }]}
            >
              <Text className="text-base text-zinc-900 dark:text-zinc-100">
                {draft.trust_mode === "include_untrusted" ? "●" : "○"} Allow untrusted issuers
              </Text>
            </Pressable>

            <Pressable
              onPress={() => setDraft({ trust_mode: "any" })}
              accessibilityRole="button"
              className="py-3"
              style={({ pressed }) => [{ opacity: pressed ? 0.6 : 1 }]}
            >
              <Text className="text-base text-zinc-900 dark:text-zinc-100">
                {draft.trust_mode === "any" ? "●" : "○"} Any
              </Text>
            </Pressable>
          </View>

          {/* If you want Reset to truly go back to "safe defaults":
              this is already handled by resetDraftToDefaults() which uses DEFAULT_RECRUITER_QUERY.
              If you want Reset to restore the entry-state instead, say so and we’ll change it. */}
        </View>
      </ScrollView>

      {/* Sticky footer */}
      <View
        className="absolute bottom-0 left-0 right-0 border-t border-zinc-200 dark:border-zinc-800 bg-white/95 dark:bg-zinc-900/95"
        style={{ paddingBottom: Math.max(insets.bottom, 10) }}
      >
        <View className="px-5 pt-3 pb-3">
          <View className="flex-row items-center justify-between">
            <Pressable
              onPress={reset}
              accessibilityRole="button"
              accessibilityLabel="Reset filters"
              className="h-14 flex-1 items-center justify-center rounded-2xl border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900"
              style={({ pressed }) => [{ opacity: pressed ? 0.7 : 1 }]}
            >
              <Text className="text-[17px] font-semibold text-rose-600">Reset</Text>
            </Pressable>

            <View className="w-3" />

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