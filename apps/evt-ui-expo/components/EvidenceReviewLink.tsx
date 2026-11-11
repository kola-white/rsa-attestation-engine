import React from "react";
import { Pressable, Text } from "react-native";
import { useNavigation } from "@react-navigation/native";
import type { NativeStackNavigationProp } from "@react-navigation/native-stack";
import type { AppStackParamList } from "@/src/navigation/MainAppNavigator";

export function EvidenceReviewLink() {
  const navigation = useNavigation<NativeStackNavigationProp<AppStackParamList>>();
  return <Pressable accessibilityRole="button" accessibilityLabel="Open evidence reviews"
    className="min-h-12 justify-center rounded-xl px-3 active:bg-zinc-100 focus-visible:outline-2 focus-visible:outline-sky-600 dark:active:bg-zinc-800"
    onPress={() => navigation.navigate("EvidenceReview")}>
    <Text className="text-base font-semibold text-sky-600 dark:text-sky-400">Evidence reviews</Text>
  </Pressable>;
}
