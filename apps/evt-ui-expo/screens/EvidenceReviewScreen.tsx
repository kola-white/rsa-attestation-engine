import { useRoute, type RouteProp } from "@react-navigation/native";
import type { AppStackParamList } from "@/src/navigation/MainAppNavigator";
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
    <Text className="text-base font-semibold text-amber-800 dark:text-amber-300">{binding.reviewContext === "SIMULATED_UAT" ? "Simulated UAT — evidence review only. Not an employer-authorized verification." : binding.reviewContext === "AUTHORITATIVE" ? "Authoritative review context — review alone does not issue or verify a credential." : "Unclassified context — decisions are disabled."}</Text>
    <Text selectable className="text-base text-zinc-800 dark:text-zinc-200">Employer: {binding.employerId}{"\n"}Claim: {binding.claimType}{"\n"}Issuer: {binding.issuerId}</Text>
    <Text className="text-sm text-zinc-600 dark:text-zinc-300">Read the complete submitted claim below. Where supporting documents are present, compare them with the claim.</Text>
    <Text selectable className="text-sm text-zinc-900 dark:text-zinc-100">{JSON.stringify(binding.claimSnapshot, null, 2)}</Text>
    <Text selectable className="text-xs text-zinc-600 dark:text-zinc-300">Frozen claim version: {binding.requestVersion}{"\n"}Claim SHA-256: {binding.claimDigest}</Text>
    <Button label={details ? "Hide request identifiers" : "Show request identifiers and integrity details"} onPress={() => setDetails(!details)} />
    {details ? <Text selectable className="text-xs text-zinc-600 dark:text-zinc-300">Subject: {binding.subjectId}{"\n"}Request: {binding.requestId}{"\n"}Binding: {binding.bindingId}{"\n"}Frozen request version: {binding.requestVersion}{"\n"}Claim SHA-256: {binding.claimDigest}</Text> : null}
    {!binding.contextValid || !binding.requiredCheck ? <Text className="text-base text-red-700 dark:text-red-400">This request context is no longer eligible. Decisions are disabled.</Text> : null}
  </View>;
}

function ClaimOnlyReview({ listing, client, principal, refresh, gate }: {
  listing: ReviewListing; client: EvidenceReviewClient; principal: string; refresh: () => void; gate: ReviewSubmissionGate;
}) {
  const [acknowledged, setAcknowledged] = useState(false), [reason, setReason] = useState("");
  const [selected, setSelected] = useState<Decision | null>(null), [confirming, setConfirming] = useState(false);
  const [busy, setBusy] = useState(false), [error, setError] = useState<string | null>(null);
  const [receipt, setReceipt] = useState<ReviewReceipt | null>(listing.binding?.claimOnlyReview ?? null);
  const alive = useRef(true), active = useRef(false);
  useEffect(() => { alive.current = true; return () => { alive.current = false; }; }, []);
  const binding = listing.binding!;
  const key = `${principal}/${binding.bindingId}/${listing.checkId}/CLAIM_ONLY`;
  const valid = binding.claimOnlyAllowed === true && binding.reviewContext === "SIMULATED_UAT" && binding.contextValid
    && binding.requiredCheck && binding.reviewerEligible && binding.subjectId !== principal && !receipt && !gate.has(key);
  const canSubmit = valid && acknowledged && !!selected && validRationale(reason) && !busy;
  async function submit() {
    if (!canSubmit || !confirming || !selected || active.current) return;
    active.current = true; setBusy(true); setError(null);
    try {
      const result = await gate.run(key, () => client.submitClaim(listing, binding, selected, reason, principal));
      if (alive.current) { setReceipt(result); setConfirming(false); }
    } catch (e) { if (alive.current) {
      setError(errorMessage(e)); setConfirming(false); setAcknowledged(false);
      if (e instanceof ReviewError && [401, 403, 404, 409].includes(e.status)) refresh();
    } } finally { active.current = false; if (alive.current) setBusy(false); }
  }
  return <View className="gap-3 rounded-2xl border border-zinc-200 p-4 dark:border-zinc-700">
    <Text className="text-lg font-semibold text-zinc-900 dark:text-zinc-100">Review the submitted claim (simulated UAT)</Text>
    <Text className="text-base text-zinc-700 dark:text-zinc-300">This simulated review permits a decision on the immutable claim above without a document. It creates an unsigned receipt, not an employer verification or credential.</Text>
    {receipt ? <Text selectable accessibilityLiveRegion="polite" className="text-base text-zinc-900 dark:text-zinc-100">Unsigned claim review recorded: {receipt.decision}{"\n"}{receipt.reason}{"\n"}Reviewer: {receipt.reviewerId}{"\n"}Recorded: {receipt.reviewedAt}{"\n"}Receipt: {receipt.reviewId}</Text> : <>
      <Pressable accessibilityRole="checkbox" accessibilityState={{ checked: acknowledged, disabled: !valid || busy }} disabled={!valid || busy}
        onPress={() => { setAcknowledged(!acknowledged); setConfirming(false); }} className="min-h-12 rounded-xl border border-zinc-300 p-3">
        <Text className="text-base text-zinc-900 dark:text-zinc-100">{acknowledged ? "☑" : "☐"} I reviewed the complete submitted claim and acknowledge its frozen version, binding and claim SHA-256 shown above.</Text>
      </Pressable>
      {(["APPROVED", "REJECTED"] as const).map(d => <Button key={d} label={d === "APPROVED" ? "Approve claim (simulated UAT)" : "Reject claim (simulated UAT)"} selected={selected === d} disabled={!valid || busy} onPress={() => { setSelected(d); setConfirming(false); }} />)}
      <TextInput accessibilityLabel="Claim review rationale" multiline editable={valid && !busy} value={reason} maxLength={1000}
        onChangeText={v => { setReason(v); setConfirming(false); }} placeholder="Explain your decision on the submitted employment claim."
        className="min-h-28 rounded-xl border border-zinc-300 p-3 text-base text-zinc-900 dark:text-zinc-100" />
      <Text className="text-sm text-zinc-600 dark:text-zinc-300">Rationale required; maximum 1,000 UTF-8 bytes.</Text>
      {confirming ? <>
        <Text className="text-base text-zinc-900 dark:text-zinc-100">Record {selected} with this rationale? No credential will be issued.{"\n"}{reason.trim()}</Text>
        <Button label="Confirm and record claim decision" disabled={!canSubmit} onPress={() => void submit()} />
        <Button label="Go back" disabled={busy} onPress={() => setConfirming(false)} />
      </> : <Button label="Review claim decision before submitting" disabled={!canSubmit} onPress={() => setConfirming(true)} />}
    </>}
    {error ? <Text accessibilityLiveRegion="polite" className="text-base text-red-700 dark:text-red-400">{error}</Text> : null}
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
  const valid = !!binding?.contextValid && binding.reviewerEligible && binding.subjectId !== principal && binding.reviewContext !== "UNCLASSIFIED" && binding.requiredCheck && item.canReview && item.reviewStatus === "PENDING" && !receipt && !gate.has(key);
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
      <Text className="text-base font-semibold text-zinc-900 dark:text-zinc-100">Unsigned evidence review recorded: {receipt.decision}</Text>
      <Text selectable className="text-sm text-zinc-800 dark:text-zinc-200">{receipt.reason}{"\n"}Context: {receipt.reviewContext}{"\n"}Not a signed employer verification.{"\n"}Reviewer: {receipt.reviewerId}{"\n"}Recorded: {receipt.reviewedAt}{"\n"}Receipt: {receipt.reviewId}</Text>
    </View> : item.reviewStatus !== "PENDING" ? <Text selectable className="text-sm text-zinc-800 dark:text-zinc-200">Recorded decision: {item.reviewStatus}{"\n"}{item.reviewReason}{"\n"}Reviewer: {item.reviewerId}{"\n"}Recorded: {item.reviewedAt}</Text> : <>
      {!valid ? <Text className="text-sm text-zinc-700 dark:text-zinc-300">A decision is unavailable for this attachment. Refresh to check access, context or an earlier submission.</Text> : null}
      <Pressable accessibilityRole="checkbox" accessibilityState={{ checked: inspected, disabled: !opened || busy || !valid }} disabled={!opened || busy || !valid}
        onPress={() => { setInspected(!inspected); setConfirming(false); }} className="min-h-12 flex-row items-center gap-3 rounded-xl border border-zinc-300 p-3 focus-visible:outline-2 focus-visible:outline-sky-600 dark:border-zinc-700">
        <Text className="flex-1 text-base text-zinc-900 dark:text-zinc-100">{inspected ? "☑" : "☐"} I inspected the document and claim, and acknowledge the claim binding and document SHA-256 shown here.</Text>
      </Pressable>
      <View className="flex-row flex-wrap gap-3">
        {(["APPROVED", "REJECTED"] as const).map(d => <Button key={d} label={d === "APPROVED" ? (binding?.reviewContext === "SIMULATED_UAT" ? "Approve evidence (simulated UAT)" : "Approve evidence") : "Reject evidence"} selected={selected === d} disabled={!valid || busy || (d === "APPROVED" && !item.canApprove)} onPress={() => { setSelected(d); setConfirming(false); }} />)}
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
  const route = useRoute<RouteProp<AppStackParamList, "EvidenceReview">>();
  const requestId = route.params?.requestId;
  const session = useRef({ token: accessToken, generation: 0 });
  if (session.current.token !== accessToken) session.current = { token: accessToken, generation: session.current.generation + 1 };
  if (status !== "authenticated" || !user || !accessToken) return <View className="flex-1 bg-white p-5 dark:bg-zinc-900"><Text className="text-base text-zinc-900 dark:text-zinc-100">Sign in again to access evidence reviews.</Text></View>;
  return <ReviewSession key={user.id + session.current.generation + (requestId ?? "")} principal={user.id} token={accessToken} requestId={requestId} />;
}

function ReviewSession({ principal, token, requestId }: { principal: string; token: string; requestId?: string }) {
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
        const assignments = requestId ? await client.assignmentsForRequest(requestId) : await client.assignments();
        if (!alive || id !== generation.current) return;
        setChecks(assignments);
        if (!scope && requestId && assignments.length === 1) { setScope(assignments[0]); return; }
        if (scope) {
          if (!assignments.some(c => c.caseId === scope.caseId && c.checkId === scope.checkId)) throw new ReviewError(404);
          const data = await client.listing(scope);
          if (requestId && data.binding?.requestId !== requestId) throw new ReviewError(409);
          if (alive && id === generation.current) setListing(data);
        }
      } catch (e) { if (alive && id === generation.current) { setChecks([]); setListing(null); setError(errorMessage(e)); } }
      finally { if (alive && id === generation.current) setLoading(false); }
    })();
    return () => { alive = false; };
  }, [client, scope, revision, requestId]);
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
          <Text accessibilityRole="header" className="text-xl font-semibold text-zinc-900 dark:text-zinc-100">{listing.binding?.requiredCheck ? "Required check" : "Assigned check"}: {listing.checkId}</Text>
          {listing.binding && (listing.binding.claimOnlyAllowed || listing.binding.claimOnlyReview) ? <ClaimOnlyReview key={`${revision}/${listing.caseId}/${listing.checkId}/claim`} listing={listing} client={client} principal={principal} refresh={refresh} gate={gate} /> : null}
          {listing.evidence.length === 0 && !listing.binding?.claimOnlyAllowed ? <Text className="text-base text-zinc-700 dark:text-zinc-300">No received evidence is available for this check. No decision can be recorded.</Text> : null}
          {listing.evidence.map(item => <Attachment key={`${revision}/${listing.caseId}/${listing.checkId}/${item.uploadId}`} item={item} listing={listing} client={client} principal={principal} refresh={refresh} gate={gate} />)}
        </> : null}
      </View>
    </ScrollView>
  </View>;
}
