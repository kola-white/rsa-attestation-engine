import React, { useLayoutEffect, useMemo, useEffect, useState, useCallback } from "react";
import { View, Text, Pressable, ScrollView, ActivityIndicator } from "react-native";
import { useNavigation } from "@react-navigation/native";
import type { NativeStackNavigationProp } from "@react-navigation/native-stack";
import type { RequestorStackParamList, RequestRowSnapshot, RequestStatus } from "@/src/navigation/requestorTypes";
import { useAuth } from "@/src/auth/AuthContext";
import { fetchRequestorRequests, claimFromSnapshot } from "@/src/api/requestor";

type Nav = NativeStackNavigationProp<RequestorStackParamList, "RequestorHome">;

const API_BASE_URL = process.env.EXPO_PUBLIC_EVT_API_BASE_URL ?? "";

export const RequestorHomeScreen: React.FC = () => {
  const navigation = useNavigation<Nav>();
  const { logout, user } = useAuth();

  const [loading, setLoading] = useState(true);
  const [errorMsg, setErrorMsg] = useState<string | null>(null);
  const [items, setItems] = useState<RequestRowSnapshot[]>([]);

  useLayoutEffect(() => {
    navigation.setOptions({
      title: "",
      headerLargeTitle: false,
      headerRight: undefined,
    });
  }, [navigation]);

    const load = useCallback(async () => {
      if (status !== "authenticated" || !accessToken) {
        return;
      }

      setLoading(true);
      setErrorMsg(null);

    try {
      const resp = await fetchRequestorRequests(API_BASE_URL, accessToken);

      // Map API rows -> existing UI snapshot shape
      const mapped: RequestRowSnapshot[] = resp.items.map((r) => ({
        request_id: r.request_id,
        status: r.status as RequestStatus,
        claim: claimFromSnapshot(r.claim_snapshot),
        created_at: r.created_at,
        updated_at: r.updated_at,
      }));

      setItems(mapped);
    } catch (e) {
      setErrorMsg(e instanceof Error ? e.message : "Failed to load requests.");
      setItems([]);
    } finally {
      setLoading(false);
    }
  }, []);

  const { status, accessToken } = useAuth();

  useEffect(() => {
    if (status !== "authenticated" || !accessToken) {
      return;
    }

    load();
  }, [status, accessToken]);

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

          <Pressable
            onPress={load}
            className="mt-3 rounded-xl bg-zinc-200 dark:bg-zinc-800 px-4 py-3"
          >
            <Text className="text-zinc-900 dark:text-zinc-50 font-semibold">
              Refresh
            </Text>
          </Pressable>
        </View>

        <Text className="text-[12px] text-zinc-600 dark:text-zinc-400 mt-8 mb-2">
          REQUESTS
        </Text>

        {loading ? (
          <View className="py-10 items-center">
            <ActivityIndicator />
            <Text className="text-sm text-zinc-600 dark:text-zinc-300 mt-3">
              Loading…
            </Text>
          </View>
        ) : errorMsg ? (
          <View className="py-6">
            <Text className="text-sm text-red-600 dark:text-red-400">
              {errorMsg}
            </Text>
          </View>
        ) : rows.length === 0 ? (
          <View className="py-10">
            <Text className="text-sm text-zinc-600 dark:text-zinc-300">
              No requests yet.
            </Text>
          </View>
        ) : (
          rows.map((r) => (
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
          ))
        )}
      </ScrollView>
    </View>
  );
};
