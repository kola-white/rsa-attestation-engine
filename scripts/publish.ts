#!/usr/bin/env tsx
import { S3Client, PutObjectCommand, CopyObjectCommand } from "@aws-sdk/client-s3";
import fs from "fs";
import path from "path";
import { globSync } from "glob";

const {
  SPACES_KEY,
  SPACES_SECRET,
  SPACES_REGION = "sfo3",
  SPACES_ENDPOINT = "https://sfo3.digitaloceanspaces.com",
  SPACES_BUCKET,
  CDN_BASE_URL = "https://hapis.sfo3.cdn.digitaloceanspaces.com",
  GITHUB_SHA,
} = process.env;

if (!SPACES_BUCKET || !SPACES_KEY || !SPACES_SECRET) {
  console.error("Missing SPACES_* env vars");
  process.exit(1);
}

const sha = GITHUB_SHA || run("git", ["rev-parse", "HEAD"]).trim();
const prefix = `attestation-engine/${sha}`;

const s3 = new S3Client({
  region: SPACES_REGION,
  endpoint: SPACES_ENDPOINT,
  credentials: { accessKeyId: SPACES_KEY, secretAccessKey: SPACES_SECRET },
});

type UploadSpec = {
  src: string;          // file or glob
  dst: string;          // either a fixed key or a prefix (if isPrefix = true)
  stable?: boolean;     // also promote to stable copy
  contentType?: string;
  isPrefix?: boolean;   // dst is a "folder" prefix (for globbed files)
};

const specs: UploadSpec[] = [
  // core trust artifacts
  { src: "trust/jwks.json",          dst: `${prefix}/trust/jwks.json`,          contentType: "application/json", stable: true },
  { src: "trust/policy.json",        dst: `${prefix}/trust/policy.json`,        contentType: "application/json", stable: true },
  { src: "status/statuslist.json",   dst: `${prefix}/status/statuslist.json`,   contentType: "application/json", stable: true },

  // vectors & jws (globbed)
  { src: "examples/**/*.json",       dst: `${prefix}/vectors/`,                 contentType: "application/json", isPrefix: true },
  { src: "out/**/*.jws",             dst: `${prefix}/vectors/`,                 contentType: "application/jose", isPrefix: true },

  // audit logs (globbed)
  { src: "audit/**/*.jsonl",         dst: `${prefix}/audit/`,                   contentType: "text/plain",       isPrefix: true },
];

async function main() {
  // ensure local audit dir exists
  fs.mkdirSync("audit", { recursive: true });

  // 1) expand specs into concrete uploads
  const uploads: { src: string; key: string; stable?: boolean; contentType?: string }[] = [];

  for (const spec of specs) {
    const matches = globSync(spec.src, { nodir: true });
    if (matches.length === 0) continue;

    for (const m of matches) {
      const key = spec.isPrefix
        ? spec.dst.replace(/\/+$/, "") + "/" + path.basename(m)
        : spec.dst;

      uploads.push({ src: m, key, stable: spec.stable, contentType: spec.contentType });
    }
  }

  // 2) upload versioned artifacts
  for (const u of uploads) {
    await putObject(u.src, u.key, {
      immutable: true,
      contentType: u.contentType,
    });
    audit("publish", "ACCEPT", "uploaded_versioned", { key: u.key });
  }

  // 3) upload publish audit itself
  //    (this assumes you wrote events into audit/publish-audit.jsonl)
  const localAuditFile = "audit/publish-audit.jsonl";
  if (fs.existsSync(localAuditFile)) {
    await putObject(localAuditFile, `${prefix}/audit/publish-audit.jsonl`, {
      immutable: true,
      contentType: "text/plain",
    });
  }

  // 4) update latest pointer
  const latestKey = "attestation-engine/latest.json";
  const latestBody = JSON.stringify({ sha, prefix }, null, 2);
  await putRaw(latestKey, latestBody, {
    contentType: "application/json",
    cacheControl: "max-age=30, s-maxage=30",
  });
  audit("publish", "ACCEPT", "updated_latest", { key: latestKey, sha });

  // 5) promote stable copies based on stable flag
  for (const u of uploads.filter(u => u.stable)) {
    const stableKey = u.key.replace(prefix + "/", ""); // e.g. trust/jwks.json
    await maybeCopy(u.key, stableKey);
  }

  console.log(`Pinned:  ${CDN_BASE_URL}/${prefix}/`);
  console.log(`Latest:  ${CDN_BASE_URL}/attestation-engine/latest.json`);
  console.log(`Stable:  ${CDN_BASE_URL}/trust/jwks.json`);
}

async function putObject(src: string, key: string, opts: { immutable?: boolean; contentType?: string }) {
  const body = fs.readFileSync(src);
  return putRaw(key, body, {
    contentType: opts.contentType,
    cacheControl: opts.immutable
      ? "public, max-age=31536000, immutable"
      : "public, max-age=60, s-maxage=60",
  });
}

async function maybeCopy(from: string, to: string) {
  await s3.send(new CopyObjectCommand({
    Bucket: SPACES_BUCKET!,
    CopySource: `/${SPACES_BUCKET}/${from}`,
    Key: to,
    MetadataDirective: "REPLACE",
    ContentType: guessContentType(to),
    CacheControl: "public, max-age=60, s-maxage=60",
  }));
  audit("publish", "ACCEPT", "refreshed_stable", { from, to });
}

async function putRaw(key: string, body: Buffer | string, opts: { contentType?: string; cacheControl?: string }) {
  await s3.send(new PutObjectCommand({
    Bucket: SPACES_BUCKET!,
    Key: key,
    Body: body,
    ACL: "public-read",
    ContentType: opts.contentType ?? "application/octet-stream",
    CacheControl: opts.cacheControl,
  }));
}

function guessContentType(key: string) {
  if (key.endsWith(".json")) return "application/json";
  if (key.endsWith(".jws"))  return "application/jose";
  if (key.endsWith(".jsonl")) return "text/plain";
  return "application/octet-stream";
}

function run(cmd: string, args: string[]) {
  const { spawnSync } = require("child_process");
  const r = spawnSync(cmd, args, { encoding: "utf8" });
  if (r.status !== 0) throw new Error(r.stderr || `Failed: ${cmd} ${args.join(" ")}`);
  return r.stdout;
}

function audit(stage: string, outcome: "ACCEPT" | "REJECT", reason: string, details?: Record<string, unknown>) {
  const line = JSON.stringify({ ts: new Date().toISOString(), stage, outcome, reason, details }) + "\n";
  fs.mkdirSync("audit", { recursive: true });
  fs.appendFileSync("audit/publish-audit.jsonl", line);
  console.log(line.trim());
}

main().catch((e) => { console.error(e); process.exit(1); });
