import React, { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { ActivityIndicator, Platform, Pressable, ScrollView, Text, TextInput, View } from "react-native";
import * as Crypto from "expo-crypto";
import { useAuth } from "@/src/auth/AuthContext";
import { API_BASE_URL } from "@/src/config/auth";
import { EvidenceCheck } from "@/src/api/evidence";
import { Binding, Decision, EvidenceReviewClient, ReviewError, ReviewItem, ReviewListing, ReviewReceipt, ReviewSubmissionGate, validRationale } from "@/src/api/evidenceReview";
import { inspectDocument } from "@/src/api/reviewDocument";

const errorMessage = (error: unknown) => error instanceof ReviewError ? error.message : "Evidence review is unavailable. Refresh and try again.";
function Button({ label, onPress, disabled = false, selected = false }: { label: string; onPress: () => void; disabled?: boolean; selected?: boolean }) {
  return <Pressable accessibilityRole="button" accessibilityState={{ disabled, selected }} disabled={disabled} onPress={onPress}
    className={`min-h-12 justify-center rounded-xl border px-4 py-3 focus-visible:outline-2 focus-visible:outline-sky-600 ${disabled ? "border-zinc-200 bg-zinc-100 dark:border-zinc-700 dark:bg-zinc-800" : selected ? "border-sky-600 bg-sky-100 dark:bg-sky-950" : "border-zinc-300 bg-white active:bg-zinc-100 dark:border-zinc-700 dark:bg-zinc-900 dark:active:bg-zinc-800"}`}>
    <Text className={`text-base font-semibold ${disabled ? "text-zinc-500" : "text-sky-700 dark:text-sky-400"}`}>{label}</Text>
  </Pressable>;
}
function ClaimContext({ binding }: { binding: Binding | null }) {
  const [details, setDetails] = useState(false);
  if (!binding) return <Text className="text-base text-red-700 dark:text-red-400">No bound employment request is available. Decisions are disabled.</Text>;
  return <View className="gap-3 rounded-2xl border border-zinc-200 bg-zinc-50 p-4 dark:border-zinc-700 dark:bg-zinc-800">
    <Text className="text-xl font-semibold text-zinc-900 dark:text-zinc-100">Employment claim</Text>
    <Text selectable className="text-base text-zinc-800 dark:text-zinc-200">Employer: {binding.employerId}{"\n"}Claim: {binding.claimType}{"\n"}Issuer: {binding.issuerId}</Text>
    <Text className="text-sm text-zinc-600 dark:text-zinc-300">Read the complete asserted facts below and compare them with each attachment.</Text>
    <Text selectable className="text-sm text-zinc-900 dark:text-zinc-100">{JSON.stringify(binding.claimSnapshot, null, 2)}</Text>
    <Button label={details ? "Hide request identifiers" : "Show request identifiers and integrity details"} onPress={() => setDetails(!details)} />
    {details ? <Text selectable className="text-xs text-zinc-600 dark:text-zinc-300">Subject: {binding.subjectId}{"\n"}Request: {binding.requestId}{"\n"}Binding: {binding.bindingId}{"\n"}Claim SHA-256: {binding.claimDigest}</Text> : null}
    {!binding.contextValid || !binding.requiredCheck ? <Text className="text-base text-red-700 dark:text-red-400">This request context is no longer eligible. Decisions are disabled.</Text> : null}
  </View>;
}

function Attachment({ item, listing, client, principal, refresh, gate }: {
  item: ReviewItem; listing: ReviewListing; client: EvidenceReviewClient; principal: string; refresh: () => void; gate: ReviewSubmissionGate;
}) {
  const [opened, setOpened] = useState(false), [inspected, setInspected] = useState(false);
  const [selected, setSelected] = useState<Decision | null>(null), [reason, setReason] = useState("");
  const [confirming, setConfirming] = useState(false), [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null), [receipt, setReceipt] = useState<ReviewReceipt | null>(null);
  const alive = useRef(true), active = useRef(false);
  useEffect(() => { alive.current = true; return () => { alive.current = false; }; }, []);
  const binding = listing.binding;
  const key = `${principal}/${listing.caseId}/${item.uploadId}`;
  const valid = !!binding?.contextValid && binding.requiredCheck && item.canReview && item.reviewStatus === "PENDING" && !receipt && !gate.has(key);
  const rationaleValid = validRationale(reason);
  const canSubmit = valid && inspected && opened && !!selected && rationaleValid && (selected !== "APPROVED" || item.canApprove);
  async function open() {
    if (active.current) return;
    active.current = true; setBusy(true); setError(null); setConfirming(false); setOpened(false); setInspected(false);
    try {
      const bytes = await client.download(listing, item, async value => {
        const hash = await Crypto.digest(Crypto.CryptoDigestAlgorithm.SHA256, new Uint8Array(value));
        return Array.from(new Uint8Array(hash), b => b.toString(16).padStart(2, "0")).join("");
      });
      if (!alive.current) return;
      await inspectDocument(bytes, item.name, item.mimeType);
      if (alive.current) setOpened(true);
    } catch (e) { if (alive.current) {
      setError(e instanceof ReviewError ? e.message : "The document was not opened or saved. Try again; inspection has not been confirmed.");
      if (e instanceof ReviewError && [401, 403, 404].includes(e.status)) refresh();
    } }
    finally { active.current = false; if (alive.current) setBusy(false); }
  }
  async function submit() {
    if (!canSubmit || !confirming || active.current || !binding || !selected) return;
    active.current = true; setBusy(true); setError(null);
    try {
      const result = await gate.run(key, () => client.submit(listing, item, binding, selected, reason, principal));
      if (alive.current) { setReceipt(result); setConfirming(false); }
    } catch (e) { if (alive.current) {
      setError(errorMessage(e)); setConfirming(false); setOpened(false); setInspected(false);
      if (e instanceof ReviewError && [401, 403, 404, 409].includes(e.status)) refresh();
    } }
    finally { active.current = false; if (alive.current) setBusy(false); }
  }
  return <View className="gap-3 rounded-2xl border border-zinc-200 bg-white p-4 dark:border-zinc-700 dark:bg-zinc-900">
    <Text accessibilityRole="header" className="text-lg font-semibold text-zinc-900 dark:text-zinc-100">{item.name}</Text>
    <Text className="text-sm text-zinc-600 dark:text-zinc-300">{item.mimeType} · {Math.ceil(item.size / 1024)} KB · Received</Text>
    <Text className="text-sm font-semibold text-zinc-800 dark:text-zinc-200">Manual review: {receipt?.decision ?? item.reviewStatus}</Text>
    <Text className="text-sm text-amber-800 dark:text-amber-300">Malware scan: {item.scanStatus}. Human approval is not malware clearance.</Text>
    <Text selectable className="text-xs text-zinc-600 dark:text-zinc-300">Canonical SHA-256: {item.evidence_hash}</Text>
    <Button label={busy ? "Working…" : "Open / download document"} disabled={busy || !["PENDING", "CLEAN"].includes(item.scanStatus)} onPress={() => void open()} />
    <Text className="text-sm text-zinc-600 dark:text-zinc-300">{Platform.OS === "android" ? "Choose a private folder, then open the saved document in your document viewer." : "Open the downloaded document in your PDF or image viewer."} Use a trusted viewer for unscanned files. Delete your downloaded copy when no longer needed.</Text>
    {receipt ? <View accessibilityLiveRegion="polite" className="gap-2 rounded-xl bg-sky-50 p-3 dark:bg-sky-950">
      <Text className="text-base font-semibold text-zinc-900 dark:text-zinc-100">Review recorded: {receipt.decision}</Text>
      <Text selectable className="text-sm text-zinc-800 dark:text-zinc-200">{receipt.reason}{"\n"}Reviewer: {receipt.reviewerId}{"\n"}Recorded: {receipt.reviewedAt}{"\n"}Receipt: {receipt.reviewId}</Text>
    </View> : item.reviewStatus !== "PENDING" ? <Text selectable className="text-sm text-zinc-800 dark:text-zinc-200">Recorded decision: {item.reviewStatus}{"\n"}{item.reviewReason}{"\n"}Reviewer: {item.reviewerId}{"\n"}Recorded: {item.reviewedAt}</Text> : <>
      {!valid ? <Text className="text-sm text-zinc-700 dark:text-zinc-300">A decision is unavailable for this attachment. Refresh to check access, context or an earlier submission.</Text> : null}
      <Pressable accessibilityRole="checkbox" accessibilityState={{ checked: inspected, disabled: !opened || busy || !valid }} disabled={!opened || busy || !valid}
        onPress={() => { setInspected(!inspected); setConfirming(false); }} className="min-h-12 flex-row items-center gap-3 rounded-xl border border-zinc-300 p-3 focus-visible:outline-2 focus-visible:outline-sky-600 dark:border-zinc-700">
        <Text className="flex-1 text-base text-zinc-900 dark:text-zinc-100">{inspected ? "☑" : "☐"} I personally inspected this document and the employment claim above.</Text>
      </Pressable>
      <View className="flex-row flex-wrap gap-3">
        {(["APPROVED", "REJECTED"] as const).map(d => <Button key={d} label={d === "APPROVED" ? "Approve" : "Reject"} selected={selected === d} disabled={!valid || busy || (d === "APPROVED" && !item.canApprove)} onPress={() => { setSelected(d); setConfirming(false); }} />)}
      </View>
      <Text className="text-base font-semibold text-zinc-900 dark:text-zinc-100">Your rationale</Text>
      <TextInput accessibilityLabel={`Rationale for ${item.name}`} multiline editable={valid && !busy} value={reason} maxLength={1000}
        onChangeText={v => { setReason(v); setConfirming(false); }} placeholder="Explain whether the evidence supports the asserted employment facts."
        className="min-h-28 rounded-xl border border-zinc-300 bg-white p-3 text-base text-zinc-900 focus:border-sky-600 dark:border-zinc-700 dark:bg-zinc-800 dark:text-zinc-100" />
      <Text className="text-sm text-zinc-600 dark:text-zinc-300">Required; maximum 1,000 UTF-8 bytes. No decision is selected automatically.</Text>
      {confirming ? <View className="gap-3 rounded-xl border border-sky-400 p-3">
        <Text className="text-base text-zinc-900 dark:text-zinc-100">Record {selected} for “{item.name}” with your rationale? This is your attributable final review, not credential issuance.</Text>
        <Text className="text-base text-zinc-800 dark:text-zinc-200">{reason.trim()}</Text>
        <Button label="Confirm and record decision" disabled={busy || !canSubmit} onPress={() => void submit()} />
        <Button label="Go back" disabled={busy} onPress={() => setConfirming(false)} />
      </View> : <Button label="Review decision before submitting" disabled={busy || !canSubmit} onPress={() => setConfirming(true)} />}
    </>}
    {error ? <View accessibilityLiveRegion="polite" className="gap-2"><Text className="text-base text-red-700 dark:text-red-400">{error}</Text><Button label="Refresh recorded reviews" disabled={busy} onPress={refresh} /></View> : null}
  </View>;
}

// Reset all sensitive UI state when the authenticated principal or session changes.
export function EvidenceReviewScreen() {
  const { user, accessToken, status } = useAuth();
  const session = useRef({ token: accessToken, generation: 0 });
  if (session.current.token !== accessToken) session.current = { token: accessToken, generation: session.current.generation + 1 };
  if (status !== "authenticated" || !user || !accessToken) return <View className="flex-1 bg-white p-5 dark:bg-zinc-900"><Text className="text-base text-zinc-900 dark:text-zinc-100">Sign in again to access evidence reviews.</Text></View>;
  return <ReviewSession key={user.id + session.current.generation} principal={user.id} token={accessToken} />;
}

function ReviewSession({ principal, token }: { principal: string; token: string }) {
  const client = useMemo(() => new EvidenceReviewClient(process.env.EXPO_PUBLIC_EVT_API_BASE_URL || API_BASE_URL, token), [token]);
  const gate = useRef(new ReviewSubmissionGate()).current;
  const [checks, setChecks] = useState<EvidenceCheck[]>([]), [scope, setScope] = useState<EvidenceCheck | null>(null);
  const [listing, setListing] = useState<ReviewListing | null>(null), [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null), [revision, setRevision] = useState(0);
  const generation = useRef(0);
  const refresh = useCallback(() => setRevision(v => v + 1), []);
  useEffect(() => {
    const id = ++generation.current; let alive = true;
    setLoading(true); setError(null); setListing(null);
    void (async () => {
      try {
        const assignments = await client.assignments();
        if (!alive || id !== generation.current) return;
        setChecks(assignments);
        if (scope) {
          if (!assignments.some(c => c.caseId === scope.caseId && c.checkId === scope.checkId)) throw new ReviewError(404);
          const data = await client.listing(scope);
          if (alive && id === generation.current) setListing(data);
        }
      } catch (e) { if (alive && id === generation.current) { setChecks([]); setListing(null); setError(errorMessage(e)); } }
      finally { if (alive && id === generation.current) setLoading(false); }
    })();
    return () => { alive = false; };
  }, [client, scope, revision]);
  return <View className="flex-1 bg-white dark:bg-zinc-900">
    <ScrollView contentInsetAdjustmentBehavior="automatic" className="flex-1" keyboardShouldPersistTaps="handled">
      <View className="mx-auto w-full max-w-3xl gap-4 px-5 py-5">
        <Text accessibilityRole="header" className="text-2xl font-semibold text-zinc-900 dark:text-zinc-100">Evidence reviews</Text>
        <Text className="text-base text-zinc-600 dark:text-zinc-300">Inspect the claim and supporting documents, then record your own decision. Only explicitly assigned cases appear.</Text>
        <Button label="Refresh assignments and reviews" onPress={refresh} disabled={loading} />
        {loading ? <ActivityIndicator accessibilityLabel="Loading authorized evidence reviews" /> : null}
        {error ? <Text accessibilityLiveRegion="polite" className="text-base text-red-700 dark:text-red-400">{error}</Text> : null}
        {!loading && !error && checks.length === 0 ? <Text className="text-base text-zinc-700 dark:text-zinc-300">No evidence review assignments are available. Ask the case operator to confirm your case-specific access; an HR assignment alone is insufficient.</Text> : null}
        <View className="gap-3">{checks.map(c => <Button key={c.caseId + "/" + c.checkId} label={`${c.caseId} / ${c.checkId}`} selected={scope?.caseId === c.caseId && scope.checkId === c.checkId} disabled={loading} onPress={() => setScope(c)} />)}</View>
        {listing ? <>
          <ClaimContext binding={listing.binding} />
          <Text accessibilityRole="header" className="text-xl font-semibold text-zinc-900 dark:text-zinc-100">Required check: {listing.checkId}</Text>
          {listing.evidence.length === 0 ? <Text className="text-base text-zinc-700 dark:text-zinc-300">No received evidence is available for this check. No decision can be recorded.</Text> : null}
          {listing.evidence.map(item => <Attachment key={`${revision}/${listing.caseId}/${listing.checkId}/${item.uploadId}`} item={item} listing={listing} client={client} principal={principal} refresh={refresh} gate={gate} />)}
        </> : null}
      </View>
    </ScrollView>
  </View>;
}
