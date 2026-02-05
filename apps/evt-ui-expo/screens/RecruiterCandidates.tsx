// src/screens/RecruiterCandidates.tsx

import React, {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import {
  View,
  Text,
  Pressable,
  FlatList,
  ActivityIndicator,
  RefreshControl,
  Animated,
} from "react-native";
import { useFocusEffect, useNavigation, useRoute } from "@react-navigation/native";
import type { RouteProp } from "@react-navigation/native";
import type { NativeStackNavigationProp } from "@react-navigation/native-stack";
import AsyncStorage from "@react-native-async-storage/async-storage";

import type {
  RecruiterStackParamList,
  RecruiterQueryState,
  CandidateRowSnapshot,
} from "../src/navigation/recruiterTypes";

type Nav = NativeStackNavigationProp<RecruiterStackParamList, "RecruiterCandidates">;
type Rte = RouteProp<RecruiterStackParamList, "RecruiterCandidates">;

type ListResp = { items: CandidateRowSnapshot[]; next_cursor?: string | null };

const STORAGE_KEY_RECRUITER_QUERY = "recruiterCandidates:lastQuery:v1";

function defaultRecruiterQuery(): RecruiterQueryState {
  return {
    search: "",
    trust_mode: "any",
    signature_status: ["verified", "invalid", "unknown"],
    company_ids: [],
    sort: "most_recent",
    page: { limit: 25 },
  };
}

function uniq<T>(arr: T[]): T[] {
  return Array.from(new Set(arr));
}

function encodeQueryParams(q: RecruiterQueryState): string {
  const params = new URLSearchParams();

  const search = (q.search ?? "").trim();
  if (search) params.set("search", search);

  params.set("trust_mode", q.trust_mode);

  const sig = uniq(q.signature_status ?? []);
  for (const s of sig) params.append("signature_status", s);

  const companies = uniq(q.company_ids ?? []);
  for (const id of companies) params.append("company_ids", id);

  params.set("sort", q.sort ?? "most_recent");

  const limit = q.page?.limit ?? 25;
  params.set("limit", String(limit));

  const cursor = q.page?.cursor;
  if (cursor) params.set("cursor", cursor);

  return params.toString();
}

function stripCursor(q: RecruiterQueryState): RecruiterQueryState {
  return {
    ...q,
    page: { ...(q.page ?? {}), cursor: undefined, limit: q.page?.limit ?? 25 },
  };
}

async function apiGetRecruiterCandidates(
  q: RecruiterQueryState,
  signal: AbortSignal
): Promise<ListResp> {
  const qs = encodeQueryParams(q);
  const res = await fetch(`/v1/recruiter/candidates?${qs}`, {
    method: "GET",
    headers: { Accept: "application/json" },
    signal,
  });

  if (!res.ok) {
    let msg = "candidates_fetch_failed";
    try {
      const j = await res.json();
      if (j?.error) msg = String(j.error);
    } catch {}
    throw new Error(msg);
  }

  return (await res.json()) as ListResp;
}

export function RecruiterCandidatesScreen() {
  const nav = useNavigation<Nav>();
  const route = useRoute<Rte>();

  const [query, setQuery] = useState<RecruiterQueryState>(() => defaultRecruiterQuery());

  const [items, setItems] = useState<CandidateRowSnapshot[]>([]);
  const [nextCursor, setNextCursor] = useState<string | null>(null);

  const [loading, setLoading] = useState(false); // first page (focus load)
  const [refreshing, setRefreshing] = useState(false); // pull-to-refresh
  const [loadingMore, setLoadingMore] = useState(false); // pagination
  const [err, setErr] = useState<string | null>(null);

  // --- Subtle “Apple-ish” count animation -----------------------------------
  const countAnim = useRef(new Animated.Value(1)).current;
  const lastCountRef = useRef<number>(0);

  useEffect(() => {
    const n = items.length;
    if (n === lastCountRef.current) return;
    lastCountRef.current = n;

    countAnim.setValue(0.96);
    Animated.spring(countAnim, {
      toValue: 1,
      useNativeDriver: true,
      speed: 18,
      bounciness: 8,
    }).start();
  }, [items.length, countAnim]);

  const openFilters = useCallback(() => {
    nav.navigate("RecruiterFilters", { initial: query });
  }, [nav, query]);

  // Keep native iOS header styling + headerRight Filter button
  useLayoutEffect(() => {
    nav.setOptions({
      title: "Candidates",
      headerTitleStyle: {
        color: "#e5e7eb", // zinc-200
        fontWeight: "600",
      },
      headerLargeTitleStyle: {
        color: "#e5e7eb",
      },
      headerRight: () => (
        <Pressable
          onPress={openFilters}
          accessibilityRole="button"
          className="px-3 py-1 rounded-lg bg-zinc-200 dark:bg-zinc-800"
          style={({ pressed }) => [{ opacity: pressed ? 0.6 : 1 }]}
        >
          <Text className="text-sm font-semibold text-zinc-900 dark:text-zinc-100">
            Filter
          </Text>
        </Pressable>
      ),
    });
  }, [nav, openFilters]);

  // --- Persistence: load last-used filters on app launch ---------------------
  const didHydrateRef = useRef(false);

  useEffect(() => {
    let cancelled = false;

    void (async () => {
      try {
        const raw = await AsyncStorage.getItem(STORAGE_KEY_RECRUITER_QUERY);
        if (!raw) return;
        const parsed = JSON.parse(raw) as RecruiterQueryState;

        // Defensive: keep cursor out of persisted query
        const persisted = stripCursor(parsed);

        if (!cancelled) {
          setQuery(persisted);
        }
      } catch {
        // ignore persistence errors
      } finally {
        if (!cancelled) didHydrateRef.current = true;
      }
    })();

    return () => {
      cancelled = true;
    };
  }, []);

  // Persist whenever query changes (after initial hydration)
  useEffect(() => {
    if (!didHydrateRef.current) return;

    const toStore = stripCursor(query);
    void (async () => {
      try {
        await AsyncStorage.setItem(STORAGE_KEY_RECRUITER_QUERY, JSON.stringify(toStore));
      } catch {
        // ignore persistence errors
      }
    })();
  }, [query]);

  // --- Option A: receive applied filters from Filters via route params --------
  useEffect(() => {
    const incoming = route.params?.query;
    if (!incoming) return;

    setQuery(stripCursor(incoming));

    // Clear the param so it doesn't re-apply repeatedly
    nav.setParams({ query: undefined });
  }, [route.params?.query, nav]);

  const baseQuery = useMemo<RecruiterQueryState>(() => stripCursor(query), [query]);

  const onPressRow = useCallback(
    (row: CandidateRowSnapshot) => {
      nav.navigate("CandidateDetail", {
        candidate_id: row.candidate_id,
        subject_ref: row.subject,
        primary_evt_ref: row.primary_evt,
        prefetch_snapshot: row,
      });
    },
    [nav]
  );

  const fetchFirstPage = useCallback(
    async (signal: AbortSignal, mode: "focus" | "refresh") => {
      if (mode === "focus") setLoading(true);
      if (mode === "refresh") setRefreshing(true);

      setErr(null);

      try {
        const resp = await apiGetRecruiterCandidates(baseQuery, signal);
        setItems(resp.items ?? []);
        setNextCursor(resp.next_cursor ?? null);
      } catch (e: any) {
        if (e?.name === "AbortError") return;
        setErr(e?.message ? String(e.message) : "candidates_fetch_failed");
        setItems([]);
        setNextCursor(null);
      } finally {
        if (signal.aborted) return;
        if (mode === "focus") setLoading(false);
        if (mode === "refresh") setRefreshing(false);
      }
    },
    [baseQuery]
  );

  // Fetch first page on focus. Abort safely on blur/unmount.
  useFocusEffect(
    useCallback(() => {
      const ctrl = new AbortController();
      void fetchFirstPage(ctrl.signal, "focus");
      return () => ctrl.abort();
    }, [fetchFirstPage])
  );

  useEffect(() => {
  const ctrl = new AbortController();
  void fetchFirstPage(ctrl.signal, "focus");
  return () => ctrl.abort();
}, [fetchFirstPage]);

  // Pull-to-refresh (iOS-native)
  const onRefresh = useCallback(() => {
  if (loadingMore) return;
  const ctrl = new AbortController();
  void fetchFirstPage(ctrl.signal, "refresh");
}, [fetchFirstPage, loadingMore]);

  const loadMore = useCallback(() => {
    if (!nextCursor || loadingMore || loading || refreshing) return;

    const ctrl = new AbortController();
    setLoadingMore(true);
    setErr(null);

    const q2: RecruiterQueryState = {
      ...query,
      page: { ...(query.page ?? {}), cursor: nextCursor, limit: query.page?.limit ?? 25 },
    };

    void (async () => {
      try {
        const resp = await apiGetRecruiterCandidates(q2, ctrl.signal);
        setItems((prev) => prev.concat(resp.items ?? []));
        setNextCursor(resp.next_cursor ?? null);
      } catch (e: any) {
        if (e?.name === "AbortError") return;
        setErr(e?.message ? String(e.message) : "candidates_fetch_failed");
      } finally {
        if (!ctrl.signal.aborted) setLoadingMore(false);
      }
    })();
  }, [nextCursor, loadingMore, loading, refreshing, query]);

  return (
    <View className="flex-1 bg-white dark:bg-black">
      {/* Subtle count label (Apple-ish) */}
      <View className="px-4 pt-2 pb-2">
        <Animated.View style={{ transform: [{ scale: countAnim }] }}>
          <Text className="text-[13px] text-zinc-500 dark:text-zinc-400">
            {items.length} candidates
          </Text>
        </Animated.View>
      </View>

      {loading && items.length === 0 ? (
        <View className="flex-1 items-center justify-center">
          <ActivityIndicator />
        </View>
      ) : err ? (
        <View className="px-4 py-3">
          <Text className="text-red-500">{err}</Text>
        </View>
      ) : items.length === 0 ? (
        <View className="flex-1 items-center justify-center px-6">
          <Text className="text-[15px] text-zinc-600 dark:text-zinc-300 text-center">
            No candidates match these filters.
          </Text>
        </View>
      ) : null}

      <FlatList
        data={items}
        keyExtractor={(it) => it.candidate_id}
        contentContainerStyle={{ paddingBottom: 24 }}
        onEndReachedThreshold={0.7}
        onEndReached={loadMore}
        refreshControl={
          <RefreshControl refreshing={refreshing} onRefresh={onRefresh} />
        }
        renderItem={({ item }) => {
          // Resilient renderer (partial snapshots won't break UI)
          const fullName = item?.subject?.full_name || "Unknown name";
          const title = item?.primary_employment?.title || "Unknown title";
          const issuer = item?.primary_employment?.issuer_name || "Unknown issuer";
          const sig = item?.badges?.signature ?? "unknown";
          const trust = item?.badges?.trust ?? "unknown";

          return (
            <Pressable
              onPress={() => onPressRow(item)}
              style={({ pressed }) => [{ opacity: pressed ? 0.7 : 1 }]}
              className="px-4 py-3 border-b border-zinc-200 dark:border-zinc-800"
            >
              <Text className="text-[16px] text-zinc-900 dark:text-zinc-100">
                {fullName}
              </Text>
              <Text className="text-[13px] text-zinc-600 dark:text-zinc-400">
                {title} · {issuer}
              </Text>
              <Text className="text-[12px] text-zinc-500 dark:text-zinc-500">
                Signature: {sig} · Trust: {trust}
              </Text>
            </Pressable>
          );
        }}
        ListFooterComponent={
          loadingMore ? (
            <View className="py-4">
              <ActivityIndicator />
            </View>
          ) : null
        }
      />
    </View>
  );
}
