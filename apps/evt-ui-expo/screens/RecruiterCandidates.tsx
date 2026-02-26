// src/screens/RecruiterCandidates.tsx
import { useAuth } from "@/src/auth/AuthContext";
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
import { useNavigation } from "@react-navigation/native";
import type { NativeStackNavigationProp } from "@react-navigation/native-stack";
import AsyncStorage from "@react-native-async-storage/async-storage";
import type {
  RecruiterStackParamList,
  RecruiterQueryState,
  CandidateRowSnapshot,
} from "@/src/navigation/recruiterTypes";
import { CandidateRow } from "@/components/CandidateRow";
import {
  useRecruiterFiltersStore,
  DEFAULT_RECRUITER_QUERY,
} from "@/src/state/recruiterFiltersStore";

type Nav = NativeStackNavigationProp<RecruiterStackParamList, "RecruiterCandidates">;

type ListResp = { items: CandidateRowSnapshot[]; next_cursor?: string | null };

const STORAGE_KEY_RECRUITER_QUERY = "recruiterCandidates:lastQuery:v1";
const API_BASE_URL = process.env.EXPO_PUBLIC_EVT_API_BASE_URL;

function normalizeQuery(q: RecruiterQueryState): RecruiterQueryState {
  return {
    search: (q.search ?? "").trim(),
    trust_mode: q.trust_mode ?? "any",
    signature_status: [...(q.signature_status ?? [])].sort(),
    company_ids: [...(q.company_ids ?? [])].sort(),
    title_query: q.title_query?.trim() || undefined,
    dates: q.dates
      ? {
          start_after: q.dates.start_after || undefined,
          end_before: q.dates.end_before || undefined,
          include_current: !!q.dates.include_current,
        }
      : undefined,
    sort: q.sort ?? "most_recent",
    // IMPORTANT: cursor must not be part of the “new query” identity
    page: q.page?.limit ? { limit: q.page.limit } : undefined,
  };
}

function queryKey(q: RecruiterQueryState): string {
  return JSON.stringify(normalizeQuery(q));
}

function uniq<T>(arr: T[]): T[] {
  return Array.from(new Set(arr));
}

function encodeQueryParams(q: RecruiterQueryState): string {
  const params = new URLSearchParams();

  const search = (q.search ?? "").trim();
  if (search) params.set("search", search);

  params.set("trust_mode", q.trust_mode ?? "any");

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
  accessToken: string,
  signal: AbortSignal
): Promise<ListResp> {
  if (!API_BASE_URL) {
    throw new Error("Missing EXPO_PUBLIC_EVT_API_BASE_URL");
  }

  const qs = encodeQueryParams(q);
  const url = `${API_BASE_URL}/v1/recruiter/candidates?${qs}`;

  console.log("[CandidatesAPI] GET", url);
  console.log("[CandidatesAPI] q =", q);

  const res = await fetch(url, {
    method: "GET",
    headers: {
      Accept: "application/json",
      Authorization: `Bearer ${accessToken}`,
    },
    signal,
  });

  console.log("[CandidatesAPI] status =", res.status, "ok =", res.ok);

  const text = await res.text();
  console.log("[CandidatesAPI] raw(0..500) =", text.slice(0, 500));

  let data: any = null;
  try {
    data = text ? JSON.parse(text) : null;
  } catch {
    throw new Error("invalid_json_from_server");
  }

  if (!res.ok) {
    const msg = data?.error ? String(data.error) : "candidates_fetch_failed";
    throw new Error(msg);
  }

  const out = data as ListResp;
  console.log(
    "[CandidatesAPI] items =",
    out.items?.length ?? 0,
    "next_cursor =",
    out.next_cursor ?? null
  );

  return out;
}

export function RecruiterCandidatesScreen() {
  const nav = useNavigation<Nav>();
  const { accessToken } = useAuth();

  // ✅ Single source of truth: store.applied
  const applied = useRecruiterFiltersStore((s) => s.applied);
  const setApplied = useRecruiterFiltersStore((s) => s.setApplied);
  const openDraftWithInitial = useRecruiterFiltersStore((s) => s.openDraftWithInitial);

  const [items, setItems] = useState<CandidateRowSnapshot[]>([]);
  const [nextCursor, setNextCursor] = useState<string | null>(null);

  const [loading, setLoading] = useState(false);
  const [refreshing, setRefreshing] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  // --- “Apple-ish” count animation ------------------------------------------
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

  // --- Persistence: hydrate store.applied on launch --------------------------
  const didHydrateRef = useRef(false);

  useEffect(() => {
    let cancelled = false;

    void (async () => {
      try {
        const raw = await AsyncStorage.getItem(STORAGE_KEY_RECRUITER_QUERY);
        if (!raw) {
          if (!cancelled) setApplied(stripCursor(DEFAULT_RECRUITER_QUERY));
          return;
        }

        const parsed = JSON.parse(raw) as RecruiterQueryState;
        const persisted = stripCursor(parsed);

        if (!cancelled) setApplied(persisted);
      } catch {
        if (!cancelled) setApplied(stripCursor(DEFAULT_RECRUITER_QUERY));
      } finally {
        if (!cancelled) didHydrateRef.current = true;
      }
    })();

    return () => {
      cancelled = true;
    };
  }, [setApplied]);

  // Persist whenever applied changes (after hydration)
  useEffect(() => {
    if (!didHydrateRef.current) return;

    const toStore = stripCursor(applied);
    void (async () => {
      try {
        await AsyncStorage.setItem(
          STORAGE_KEY_RECRUITER_QUERY,
          JSON.stringify(toStore)
        );
      } catch {
        // ignore
      }
    })();
  }, [applied]);

  const baseQuery = useMemo<RecruiterQueryState>(() => stripCursor(applied), [applied]);
  const baseQueryKey = useMemo(() => queryKey(baseQuery), [baseQuery]);

  const fetchFirstPage = useCallback(
    async (signal: AbortSignal, mode: "focus" | "refresh") => {
      const stamp = `[fetchFirstPage ${mode}] key=${baseQueryKey} trust=${baseQuery.trust_mode}`;
      console.log(stamp, "START", "accessToken?", !!accessToken);

      if (mode === "focus") setLoading(true);
      if (mode === "refresh") setRefreshing(true);

      setErr(null);

      try {
        if (!accessToken) {
          setItems([]);
          setNextCursor(null);
          setErr("Missing access token (sign in again)");
          return;
        }

        const resp = await apiGetRecruiterCandidates(baseQuery, accessToken, signal);
        console.log(stamp, "OK items=", resp.items?.length ?? 0);

        setItems(resp.items ?? []);
        setNextCursor(resp.next_cursor ?? null);
      } catch (e: any) {
        console.log(stamp, "ERR", e?.message ?? e);
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
    [baseQuery, baseQueryKey, accessToken]
  );

  // Refetch whenever applied query identity changes
  useEffect(() => {
    const ctrl = new AbortController();
    void fetchFirstPage(ctrl.signal, "focus");
    return () => ctrl.abort();
  }, [fetchFirstPage, baseQueryKey]);

  const onRefresh = useCallback(() => {
    if (loadingMore) return;
    const ctrl = new AbortController();
    void fetchFirstPage(ctrl.signal, "refresh");
  }, [fetchFirstPage, loadingMore]);

  const loadMore = useCallback(() => {
    if (!nextCursor || loadingMore || loading || refreshing) return;
    if (!accessToken) return;

    const ctrl = new AbortController();
    setLoadingMore(true);
    setErr(null);

    const q2: RecruiterQueryState = {
      ...baseQuery,
      page: {
        ...(baseQuery.page ?? {}),
        cursor: nextCursor,
        limit: baseQuery.page?.limit ?? 25,
      },
    };

    void (async () => {
      try {
        const resp = await apiGetRecruiterCandidates(q2, accessToken, ctrl.signal);
        setItems((prev) => prev.concat(resp.items ?? []));
        setNextCursor(resp.next_cursor ?? null);
      } catch (e: any) {
        if (e?.name === "AbortError") return;
        setErr(e?.message ? String(e.message) : "candidates_fetch_failed");
      } finally {
        if (!ctrl.signal.aborted) setLoadingMore(false);
      }
    })();
  }, [nextCursor, loadingMore, loading, refreshing, baseQuery, accessToken]);

  const dedupedItems = useMemo(() => {
    const m = new Map<string, (typeof items)[number]>();
    for (const it of items) m.set(it.candidate_id, it);
    return Array.from(m.values());
  }, [items]);

  useEffect(() => {
    const ids = items.map((x) => x.candidate_id);
    const uniqIds = new Set(ids);
    console.log("[RecruiterCandidates] items=", items.length, "uniqueIds=", uniqIds.size);
  }, [items]);

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

  const openFilters = useCallback(() => {
    // ✅ open modal with current applied query as the draft baseline
    openDraftWithInitial(stripCursor(applied));
    nav.navigate("RecruiterFilters");
  }, [nav, openDraftWithInitial, applied]);

  useLayoutEffect(() => {
    nav.setOptions({
      title: "Candidates",
      headerTitleStyle: {
        color: "#e5e7eb",
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

  return (
    <View className="flex-1 bg-white dark:bg-black">
      <FlatList
        data={dedupedItems}
        keyExtractor={(it) => it.candidate_id}
        contentInsetAdjustmentBehavior="automatic"
        contentContainerStyle={{
          paddingBottom: 24,
          paddingHorizontal: 16,
          paddingTop: 8,
        }}
        refreshControl={<RefreshControl refreshing={refreshing} onRefresh={onRefresh} />}
        onEndReachedThreshold={0.7}
        onEndReached={loadMore}
        ListHeaderComponent={
          <View style={{ paddingTop: 8 }} className="px-4 pb-2">
            <Animated.View style={{ transform: [{ scale: countAnim }] }}>
              <Text className="text-[13px] text-zinc-500 dark:text-zinc-400">
                {dedupedItems.length} candidates
              </Text>
            </Animated.View>
          </View>
        }
        ListEmptyComponent={
          loading ? (
            <View className="flex-1 items-center justify-center" style={{ paddingTop: 24 }}>
              <ActivityIndicator />
            </View>
          ) : err ? (
            <View className="px-4 py-3">
              <Text className="text-red-500">{err}</Text>
            </View>
          ) : (
            <View className="flex-1 items-center justify-center px-6" style={{ paddingTop: 24 }}>
              <Text className="text-[15px] text-zinc-600 dark:text-zinc-300 text-center">
                No candidates match these filters.
              </Text>
            </View>
          )
        }
        renderItem={({ item }) => <CandidateRow item={item} onPress={onPressRow} />}
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