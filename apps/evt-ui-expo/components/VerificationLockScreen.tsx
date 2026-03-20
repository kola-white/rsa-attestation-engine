import React from "react";
import { View, Text } from "react-native";

type GateState = "ALLOW" | "LOCK_UNKNOWN" | "LOCK_PENDING";

export function VerificationLockScreen({ gate }: { gate: GateState }) {
  const title =
    gate === "LOCK_PENDING"
      ? "Pending verification"
      : "Verification unavailable";

  const message =
    gate === "LOCK_PENDING"
      ? "This record has not yet reached a usable verification outcome, so details are locked for now."
      : "This record does not currently have a usable verification outcome, so details are locked.";

  return (
    <View className="flex-1 bg-white dark:bg-zinc-900 items-center justify-center px-6">
      <Text className="text-xl font-semibold text-zinc-950 dark:text-zinc-50 text-center">
        {title}
      </Text>
      <Text className="text-sm text-zinc-600 dark:text-zinc-300 mt-3 text-center leading-6">
        {message}
      </Text>
    </View>
  );
}