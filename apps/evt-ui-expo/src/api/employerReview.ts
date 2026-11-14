export type EmployerAccess = { employer_id: string; display_name?: string };
export type EmployerResponseType = "FULL_MATCH" | "PARTIAL_MATCH" | "REJECTED_NO_RECORD" | "REJECTED_POLICY";
export type HRQueueRow = {
  request_id: string; status: string; claim_snapshot: Record<string, unknown>;
  created_at: string; updated_at: string;
};
export type HRDetail = HRQueueRow & { version: number; employer_response_type?: EmployerResponseType | null };
export type EmployerReviewState = {
  employers: EmployerAccess[]; employerId: string | null; queue: HRQueueRow[];
  selected: HRDetail | null; loading: boolean; attesting: EmployerResponseType | null; error: string | null;
};

// An ordinal is an access label, never a fabricated legal/business name.
export function employerLabel(employer: EmployerAccess, index: number): string {
  return employer.display_name?.trim() || `Employer access ${index + 1}`;
}

class EmployerReviewError extends Error {
  constructor(readonly status: number) {
    super(status === 401 ? "Your session expired. Sign in again."
      : status === 403 || status === 404 ? "Employer access is unavailable or has been revoked. Refresh your employer access."
      : status === 409 ? "The request has changed. Refresh before continuing."
      : "Employer review is unavailable. Refresh and try again.");
  }
}

const empty = (): EmployerReviewState => ({ employers: [], employerId: null, queue: [], selected: null, loading: false, attesting: null, error: null });
const record = (v: unknown): v is Record<string, unknown> => !!v && typeof v === "object" && !Array.isArray(v);
const queueRow = (v: unknown): v is HRQueueRow => record(v) && typeof v.request_id === "string" &&
  v.request_id.length > 0 && typeof v.status === "string" && record(v.claim_snapshot);

// Shared by the screen and regression tests. Epochs discard responses from old
// employer/session/focus contexts. Discovery is advisory; every API operation
// still checks the current authenticated principal and assignment on the server.
export class EmployerReview {
  private state = empty();
  private listeners = new Set<() => void>();
  private epoch = 0;
  private pendingDecision = false;
  constructor(private base: string, private token: string, private transport: typeof fetch = fetch) {}
  snapshot = (): EmployerReviewState => this.state;
  subscribe = (listener: () => void) => { this.listeners.add(listener); return () => { this.listeners.delete(listener); }; };
  private update(patch: Partial<EmployerReviewState>) {
    this.state = { ...this.state, ...patch };
    this.listeners.forEach(listener => listener());
  }
  clear = () => { ++this.epoch; this.state = empty(); this.listeners.forEach(listener => listener()); };

  private async request(path: string, body?: unknown): Promise<unknown> {
    if (!this.base || !this.token) throw new EmployerReviewError(401);
    const res = await this.transport(this.base.replace(/\/$/, "") + path, {
      method: body === undefined ? "GET" : "POST",
      credentials: "omit", redirect: "error", cache: "no-store",
      headers: { Accept: "application/json", "Content-Type": "application/json", Authorization: `Bearer ${this.token}` },
      ...(body === undefined ? {} : { body: JSON.stringify(body) }),
    });
    if (!res.ok) throw new EmployerReviewError(res.status);
    return res.json();
  }
  private fail(error: unknown, epoch: number) {
    if (epoch !== this.epoch) return;
    const denied = error instanceof EmployerReviewError && [401, 403, 404].includes(error.status);
    // Clear data even on transient errors; stale queue contents never stay actionable.
    this.update({ queue: [], selected: null, loading: false,
      ...(denied ? { employers: [], employerId: null } : {}),
      error: error instanceof EmployerReviewError ? error.message : "Employer review is unavailable. Refresh and try again." });
  }

  refresh = async () => {
    if (this.pendingDecision) return;
    const epoch = ++this.epoch;
    const previous = this.state.employerId;
    const request = this.state.selected?.request_id;
    this.update({ employers: [], employerId: null, queue: [], selected: null, loading: true, error: null });
    try {
      const data = await this.request("/v1/employer/employers");
      if (!record(data) || !Array.isArray(data.items)) throw new Error("invalid discovery");
      const employers: EmployerAccess[] = data.items.map((item: unknown) => {
        if (!record(item) || typeof item.employer_id !== "string" || !item.employer_id.trim() ||
          (item.display_name !== undefined && typeof item.display_name !== "string")) throw new Error("invalid employer");
        return { employer_id: item.employer_id, ...(typeof item.display_name === "string" ? { display_name: item.display_name } : {}) };
      });
      if (new Set(employers.map(e => e.employer_id)).size !== employers.length) throw new Error("duplicate employer");
      if (epoch !== this.epoch) return;
      const employerId = employers.some(e => e.employer_id === previous) ? previous : employers.length === 1 ? employers[0].employer_id : null;
      this.update({ employers, employerId });
      if (employerId) await this.loadQueue(employerId, epoch, request);
      else this.update({ loading: false });
    } catch (error) { this.fail(error, epoch); }
  };

  selectEmployer = async (employerId: string) => {
    if (this.pendingDecision || !this.state.employers.some(e => e.employer_id === employerId)) return;
    const epoch = ++this.epoch;
    this.update({ employerId, queue: [], selected: null, loading: true, error: null });
    try { await this.loadQueue(employerId, epoch); } catch (error) { this.fail(error, epoch); }
  };
  private async detail(employerId: string, requestId: string): Promise<HRDetail> {
    const data = await this.request(`/v1/employer/requests/${encodeURIComponent(requestId)}?employer_id=${encodeURIComponent(employerId)}`);
    if (!record(data) || typeof data.version !== "number" || !queueRow(data) || data.request_id !== requestId) throw new Error("invalid detail");
    return data as HRDetail;
  }
  private async loadQueue(employerId: string, epoch: number, preferred?: string) {
    const data = await this.request(`/v1/employer/requests?employer_id=${encodeURIComponent(employerId)}`);
    if (!record(data) || !Array.isArray(data.items) || !data.items.every(queueRow)) throw new Error("invalid queue");
    if (epoch !== this.epoch) return;
    const queue = data.items as HRQueueRow[];
    const requestId = queue.find(row => row.request_id === preferred)?.request_id ?? queue[0]?.request_id;
    const selected = requestId ? await this.detail(employerId, requestId) : null;
    if (epoch === this.epoch) this.update({ queue, selected, loading: false });
  }
  selectRequest = async (requestId: string) => {
    const employerId = this.state.employerId;
    if (!employerId || this.pendingDecision || this.state.loading || !this.state.queue.some(row => row.request_id === requestId)) return;
    const epoch = ++this.epoch;
    this.update({ selected: null, loading: true, error: null });
    try {
      const selected = await this.detail(employerId, requestId);
      if (epoch === this.epoch) this.update({ selected, loading: false });
    } catch (error) { this.fail(error, epoch); }
  };
  attest = async (responseType: EmployerResponseType): Promise<string | null> => {
    const { employerId, selected, loading } = this.state;
    if (!employerId || !selected || loading || this.pendingDecision) return null;
    const epoch = ++this.epoch;
    this.pendingDecision = true;
    this.update({ attesting: responseType, error: null });
    try {
      const data = await this.request(`/v1/employer/requests/${encodeURIComponent(selected.request_id)}/attest`, {
        employer_id: employerId, response_type: responseType, response_body: {},
      });
      if (!record(data) || typeof data.status !== "string") throw new Error("invalid decision");
      if (epoch !== this.epoch) return null;
      this.update({ selected: null, queue: [] });
      return data.status;
    } catch (error) {
      this.fail(error, epoch);
      if (epoch === this.epoch && (!(error instanceof EmployerReviewError) || error.status >= 500)) {
        this.update({ error: "The decision could not be confirmed. Refresh to check its status before submitting again." });
      }
      return null;
    } finally {
      this.pendingDecision = false;
      if (epoch === this.epoch) this.update({ attesting: null });
    }
  };
}
