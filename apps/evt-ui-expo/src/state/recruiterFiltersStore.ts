// src/state/recruiterFiltersStore.ts
import { create } from "zustand";
import type { RecruiterQueryState } from "@/src/navigation/recruiterTypes";

export const DEFAULT_RECRUITER_QUERY: RecruiterQueryState = {
  search: "",
  trust_mode: "any",
  signature_status: ["verified", "invalid", "unknown"],
  company_ids: [],
  sort: "most_recent",
  page: { limit: 25 },
};

type State = {
  applied: RecruiterQueryState;
  draft: RecruiterQueryState;
};

type Actions = {
  setApplied: (q: RecruiterQueryState) => void;
  openDraftWithInitial: (initial: RecruiterQueryState) => void;
  setDraft: (patch: Partial<RecruiterQueryState>) => void;
  resetDraftToDefaults: () => void;
  applyDraft: () => void;
};

function cloneQuery(q: RecruiterQueryState): RecruiterQueryState {
  return {
    ...q,
    signature_status: [...(q.signature_status ?? [])],
    company_ids: [...(q.company_ids ?? [])],
    page: q.page ? { ...q.page } : undefined,
    dates: q.dates ? { ...q.dates } : undefined,
  };
}

export const useRecruiterFiltersStore = create<State & Actions>((set, get) => ({
  applied: cloneQuery(DEFAULT_RECRUITER_QUERY),
  draft: cloneQuery(DEFAULT_RECRUITER_QUERY),

  setApplied: (q) => set(() => ({ applied: cloneQuery(q) })),

  openDraftWithInitial: (initial) => set(() => ({ draft: cloneQuery(initial) })),

  setDraft: (patch) => set((s) => ({ draft: { ...s.draft, ...patch } })),

  resetDraftToDefaults: () => set(() => ({ draft: cloneQuery(DEFAULT_RECRUITER_QUERY) })),

  applyDraft: () => {
    const { draft } = get();
    set(() => ({ applied: cloneQuery(draft) }));
  },
}));