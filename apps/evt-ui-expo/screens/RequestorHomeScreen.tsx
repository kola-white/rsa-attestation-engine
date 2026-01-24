// screens/RequestorHomeScreen.tsx
import React, { useLayoutEffect, useMemo, useState } from "react";
import { View, Text, Pressable, ScrollView } from "react-native";
import { useNavigation } from "@react-navigation/native";
import type { NativeStackNavigationProp } from "@react-navigation/native-stack";
import type { RequestorStackParamList, RequestRowSnapshot } from "@/src/navigation/requestorTypes";
import { useAuth } from "@/src/auth/AuthContext";

type Nav = NativeStackNavigationProp<RequestorStackParamList, "RequestorHome">;

export const RequestorHomeScreen: React.FC = () => {
  const navigation = useNavigation<Nav>();
  const { logout, user } = useAuth();

  useLayoutEffect(() => {
    navigation.setOptions({
      title: "Back",
      headerLargeTitle: true,
      headerRight: () => (
        <Pressable
          onPress={logout}
          accessibilityRole="button"
          hitSlop={10}
          className="px-3 py-1 rounded-lg bg-zinc-200 dark:bg-zinc-700"
          style={({ pressed }) => [{ opacity: pressed ? 0.8 : 1 }]}
        >
          <Text className="text-sm font-semibold text-zinc-900 dark:text-white">
            Sign out
          </Text>
        </Pressable>
      ),
    });
  }, [navigation, logout]);

  // Placeholder list for now (until API exists)
  const [items] = useState<RequestRowSnapshot[]>([
    {
      request_id: "req_demo_001",
      claim: {
        employer: "ACME Electric (AEI)",
        job_title: "Senior Project Manager",
        start_mm_yyyy: "08/2023",
        end_mm_yyyy: "05/2025",
      },
      status: "IN_REVIEW",
      created_at: new Date().toISOString(),
      updated_at: new Date().toISOString(),
    },
  ]);

  const rows = useMemo(() => items, [items]);

  return (
    <View className="flex-1 bg-white dark:bg-zinc-900">
      <ScrollView contentInsetAdjustmentBehavior="automatic" className="px-5">
        <View className="pt-4">
          <Text className="text-sm text-zinc-600 dark:text-zinc-300">
            {user?.email ?? "Signed in"}
          </Text>

          <Pressable
            onPress={() => navigation.navigate("RequestorNewRequest")}
            className="mt-4 rounded-xl bg-zinc-900 dark:bg-zinc-100 px-4 py-3"
          >
            <Text className="text-white dark:text-zinc-900 font-semibold">
              Start a new request
            </Text>
          </Pressable>
        </View>

        <Text className="text-[12px] text-zinc-600 dark:text-zinc-400 mt-8 mb-2">
          REQUESTS
        </Text>

        {rows.map((r) => (
          <Pressable
            key={r.request_id}
            onPress={() =>
              navigation.navigate("RequestorRequestDetail", {
                request_id: r.request_id,
                snapshot: r,
              })
            }
            className="py-4 border-b border-zinc-200 dark:border-zinc-800"
          >
            <Text className="text-base font-semibold text-zinc-950 dark:text-zinc-50">
              {r.claim.employer}
            </Text>
            <Text className="text-sm text-zinc-600 dark:text-zinc-300 mt-1">
              {r.claim.job_title} • {r.claim.start_mm_yyyy} — {r.claim.end_mm_yyyy ?? "Present"}
            </Text>
            <Text className="text-sm text-zinc-500 dark:text-zinc-400 mt-1">
              Status: {r.status}
            </Text>
          </Pressable>
        ))}
      </ScrollView>
    </View>
  );
};
