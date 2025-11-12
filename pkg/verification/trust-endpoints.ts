import fs from "fs";
import path from "path";
import { TrustCfg, LatestJson, Jwks, StatusList, Policy } from "../../types/trust.js";

/* ---------- Load config + env (typed) ---------- */
function loadTrustConfig(): Required<TrustCfg> {
  const cfgPath = path.resolve(process.cwd(), "config", "trust-endpoints.json");
  let fileCfg: TrustCfg = {};
  try {
    fileCfg = JSON.parse(fs.readFileSync(cfgPath, "utf8")) as TrustCfg;
  } catch {
    // optional
  }

  const env = process.env;

  const base_url = (env.TRUST_BASE_URL || fileCfg.base_url || "").trim();
  const jwks_path = (env.TRUST_JWKS_PATH || fileCfg.jwks_path || "/trust/jwks.json").trim();
  const status_path = (env.TRUST_STATUS_PATH || fileCfg.status_path || "/status/statuslist.json").trim();
  const latest_pointer = (env.TRUST_LATEST_PATH || fileCfg.latest_pointer || "/attestation-engine/latest.json").trim();

  return { base_url, jwks_path, status_path, latest_pointer };
}

/* ---------- Fetch JSON (typed) ---------- */
async function fetchJson<T>(url: string, timeoutMs = 5000): Promise<T> {
  const ctrl = new AbortController();
  const t = setTimeout(() => ctrl.abort(), timeoutMs);
  try {
    const res = await fetch(url, { signal: ctrl.signal });
    if (!res.ok) throw new Error(`HTTP ${res.status} for ${url}`);
    return (await res.json()) as T;
  } finally {
    clearTimeout(t);
  }
}

/* ---------- URL helpers ---------- */
function joinUrl(base: string, p: string) {
  if (!p.startsWith("/")) p = `/${p}`;
  return `${base.replace(/\/+$/, "")}${p}`;
}

type TrustUrls =
  | { mode: "local"; jwksUrl: ""; statusUrl: ""; latestPtrUrl: "" }
  | { mode: "stable" | "pinned"; jwksUrl: string; statusUrl: string; latestPtrUrl: string };

/**
 * Resolve URLs for trust artifacts.
 * mode = "stable" → CDN /trust/jwks.json, /status/statuslist.json
 * mode = "pinned" → reads /attestation-engine/latest.json, prefixes versioned paths
 */
export async function getTrustUrls(mode: "stable" | "pinned" = "stable"): Promise<TrustUrls> {
  const { base_url, jwks_path, status_path, latest_pointer } = loadTrustConfig();

  // Explicit local mode or no remote configured
  if (process.env.TRUST_MODE === "local" || !base_url) {
    return { mode: "local", jwksUrl: "", statusUrl: "", latestPtrUrl: "" };
  }

  if (mode === "pinned") {
    const latestPtrUrl = joinUrl(base_url, latest_pointer);
    const latest = await fetchJson<LatestJson>(latestPtrUrl);
    const prefix = latest?.prefix?.trim();
    if (!prefix) throw new Error(`latest.json missing 'prefix' at ${latestPtrUrl}`);
    return {
      mode: "pinned",
      jwksUrl: joinUrl(base_url, `/${prefix}${jwks_path}`),
      statusUrl: joinUrl(base_url, `/${prefix}${status_path}`),
      latestPtrUrl,
    };
  }

  // stable paths
  return {
    mode: "stable",
    jwksUrl: joinUrl(base_url, jwks_path),
    statusUrl: joinUrl(base_url, status_path),
    latestPtrUrl: joinUrl(base_url, latest_pointer),
  };
}

/* ---------- Fetch artifacts (remote first; local fallback) ---------- */
export async function fetchTrustArtifacts(opts?: { mode?: "stable" | "pinned" }): Promise<{
  source: "local" | "stable" | "pinned";
  jwks: Jwks;
  statuslist: StatusList;
  policy: Policy;
}> {
  const mode = opts?.mode ?? "stable";
  const urls = await getTrustUrls(mode);

  // Local: read from ./trust (dev or TRUST_MODE=local)
  if (urls.mode === "local") {
    const TRUST_DIR = path.resolve(process.cwd(), "trust");
    const jwks = JSON.parse(fs.readFileSync(path.join(TRUST_DIR, "jwks.json"), "utf8")) as Jwks;
    const statuslist = JSON.parse(fs.readFileSync(path.join(TRUST_DIR, "statuslist.json"), "utf8")) as StatusList;

    const policyPath = path.join(TRUST_DIR, "policy.json");
    const policy = fs.existsSync(policyPath)
      ? (JSON.parse(fs.readFileSync(policyPath, "utf8")) as Policy)
      : ({ allowed_assurance: [], schema_uri: "" } as Policy);

    return { source: "local", jwks, statuslist, policy };
  }

  // Remote (CDN)
  const [jwks, statuslist] = await Promise.all([
    fetchJson<Jwks>(urls.jwksUrl),
    fetchJson<StatusList>(urls.statusUrl),
  ]);

  // keep policy local for now (or publish it later)
  const policyPath = path.resolve(process.cwd(), "trust", "policy.json");
  const policy = fs.existsSync(policyPath)
    ? (JSON.parse(fs.readFileSync(policyPath, "utf8")) as Policy)
    : ({ allowed_assurance: [], schema_uri: "" } as Policy);

  return { source: urls.mode, jwks, statuslist, policy };
}
