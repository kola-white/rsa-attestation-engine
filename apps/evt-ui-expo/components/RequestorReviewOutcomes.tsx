import React from "react";
import { Pressable, Text, View } from "react-native";
import type { RequestorReviewOutcomes as Outcomes } from "../src/api/requestorReviewOutcomes";

// Display only: lifecycle status and all review/issuance actions remain outside this section.
export function RequestorReviewOutcomes({ data, loading, error, onRetry }: {
  data: Outcomes | null; loading: boolean; error: string | null; onRetry: () => void;
}) {
  return <View className="mt-6 gap-3 rounded-2xl border border-zinc-200 bg-zinc-50 p-4 dark:border-zinc-800 dark:bg-zinc-800">
    <Text accessibilityRole="header" className="text-base font-semibold text-purple-800 dark:text-purple-300">Review outcome</Text>
    <View accessibilityLiveRegion="polite" className="gap-3">
      {loading ? <Text className="text-sm text-zinc-600 dark:text-zinc-300">Loading review outcomes…</Text>
        : error ? <>
          <Text className="text-sm text-red-700 dark:text-red-300">{error}</Text>
          <Pressable accessibilityRole="button" onPress={onRetry}
            className="min-h-12 justify-center self-start rounded-xl border border-purple-300 px-4 py-3 dark:border-purple-700">
            <Text className="text-sm font-semibold text-purple-800 dark:text-purple-300">Try again</Text>
          </Pressable>
        </> : data ? <>
          {data.state === "STALE" ? <Text className="text-sm text-zinc-700 dark:text-zinc-300">An earlier review applies to a different claim version. No review outcome is shown for this version.</Text>
            : data.state === "UNAVAILABLE" ? <Text className="text-sm text-zinc-700 dark:text-zinc-300">Review outcomes are unavailable for this claim.</Text>
            : data.outcomes.length === 0 ? <Text className="text-sm text-zinc-700 dark:text-zinc-300">No review outcomes are available for this claim yet.</Text>
            : <>
              <Text className="text-sm text-zinc-700 dark:text-zinc-300">{data.reviewContext === "SIMULATED_UAT"
                ? "Simulated UAT · Unsigned, non-authoritative review. This is not employer verification or a signed credential."
                : "Authoritative review context · This review decision alone does not verify the claim or issue a signed credential."}</Text>
              {data.outcomes.map(row => <View key={`${row.checkId}/${row.reviewId ?? "pending"}`} className="gap-1 border-t border-zinc-200 pt-3 dark:border-zinc-700">
                <Text selectable className="text-xs text-zinc-500 dark:text-zinc-400">Check: {row.checkId}</Text>
                <Text className={row.decision === "APPROVED" ? "text-sm font-semibold text-emerald-800 dark:text-emerald-300"
                  : row.decision === "REJECTED" ? "text-sm font-semibold text-red-800 dark:text-red-300"
                    : "text-sm font-semibold text-zinc-800 dark:text-zinc-200"}>Review decision: {row.decision}</Text>
                {row.decision === "PENDING" ? <Text className="text-sm text-zinc-600 dark:text-zinc-300">No review decision has been recorded for this check yet.</Text> : <>
                  <Text selectable className="text-sm text-zinc-900 dark:text-zinc-100">{row.reason}</Text>
                  <Text className="text-xs text-zinc-600 dark:text-zinc-400">Reviewed: {new Date(row.reviewedAt!).toLocaleString()}</Text>
                </>}
              </View>)}
              <Text className="text-xs text-zinc-600 dark:text-zinc-400">Review decisions are separate from the request status shown above.</Text>
            </>}
        </> : null}
    </View>
  </View>;
}
