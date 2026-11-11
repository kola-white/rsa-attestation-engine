import type { EmploymentClaimDraft } from "../navigation/requestorTypes";

const record = (v: unknown): v is Record<string, unknown> => !!v && typeof v === "object" && !Array.isArray(v);
const text = (v: unknown): string | undefined => typeof v === "string" ? v : undefined;
function monthFromDate(value: unknown): string | undefined {
  return typeof value === "string" && /^\d{4}-\d{2}-\d{2}$/.test(value) ? `${value.slice(5, 7)}/${value.slice(0, 4)}` : undefined;
}

/** Canonical fields take precedence; old saved snapshots remain readable. */
export function claimFromSnapshot(snapshot: Record<string, unknown> | null): EmploymentClaimDraft {
  return readDraft(snapshot, false);
}

/** Display placeholders must never become asserted facts when editing an incomplete draft. */
export function employmentDraftFromSnapshot(snapshot: Record<string, unknown> | null): EmploymentClaimDraft {
  return readDraft(snapshot, true);
}

function readDraft(snapshot: Record<string, unknown> | null, editing: boolean): EmploymentClaimDraft {
  const s = snapshot ?? {}, p = record(s.primary_employment) ? s.primary_employment : {};
  return {
    employer: text(p.issuer_name) ?? text(s.employer) ?? (editing ? "" : "Unknown employer"),
    job_title: text(p.title) ?? text(s.job_title) ?? (editing ? "" : "Unknown title"),
    start_mm_yyyy: monthFromDate(p.start_date) ?? text(s.start_mm_yyyy) ?? (editing ? "" : "??/????"),
    end_mm_yyyy: monthFromDate(p.end_date) ?? text(s.end_mm_yyyy) ?? null,
  };
}

/** Write the existing issuance contract alongside legacy display fields; never invent exact dates or identities. */
export function employmentSnapshot(draft: EmploymentClaimDraft, existing: Record<string, unknown> = {}, employerId?: string): Record<string, unknown> {
  if (!draft.job_title.trim()) throw new Error("Enter a job title before saving the employment claim.");
  const primary = { ...(record(existing.primary_employment) ? existing.primary_employment : {}) };
  primary.title = draft.job_title.trim();
  primary.issuer_name = draft.employer.trim(); // Employer name, not the credential issuer identity.
  if (employerId !== undefined) {
    if (!employerId.trim()) throw new Error("An employer ID is required.");
    primary.employer_id = employerId.trim();
  }
  const previous = employmentDraftFromSnapshot(existing);
  // The form has month precision. Preserve existing exact dates if unchanged;
  // remove an edited exact date rather than silently inventing a day or keeping a conflicting date.
  if (draft.start_mm_yyyy.trim() !== previous.start_mm_yyyy.trim()) delete primary.start_date;
  if ((draft.end_mm_yyyy?.trim() || null) !== (previous.end_mm_yyyy?.trim() || null)) delete primary.end_date;
  return {
    ...existing,
    employer: draft.employer.trim(), job_title: draft.job_title.trim(),
    start_mm_yyyy: draft.start_mm_yyyy.trim(), end_mm_yyyy: draft.end_mm_yyyy?.trim() || null,
    primary_employment: primary,
  };
}

export function employmentDraftNeedsSave(draft: EmploymentClaimDraft, existing: Record<string, unknown>): boolean {
  try { return JSON.stringify(employmentSnapshot(draft, existing)) !== JSON.stringify(existing); }
  catch { return true; }
}
