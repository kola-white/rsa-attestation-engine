# Deployment Guide — Trust Artifacts Publishing (DigitalOcean Spaces + CDN)

This document describes how the attestation engine publishes **trust artifacts** to
DigitalOcean Spaces and how they are served through the CDN. It includes operational
requirements, ACL behavior, cache semantics, and validation steps.

## Overview

The publish pipeline produces two classes of artifacts:

### 🔹 Stable (Mutable, CDN-cached)
```

/trust/jwks.json
/status/statuslist.json

```

- Always reflects the latest keyset and revocation data.
- Short TTL (≈ 60 seconds).
- Used by production verifiers and ATS systems.

### 🔹 Pinned (Immutable, Versioned per Commit)
```

/attestation-engine/<sha>/trust/jwks.json
/attestation-engine/<sha>/status/statuslist.json
/attestation-engine/<sha>/audit/publish-audit.jsonl
/attestation-engine/latest.json

````

- Immutable once published.
- Used for auditing, forensics, reproducibility, and offline verification.

## Publish Pipeline Summary

The GitHub Action (`publish.yml`) performs:

1. Build trust artifacts.
2. Upload **pinned** artifacts via `PutObjectCommand`.
3. Upload publish audit logs.
4. Write `/attestation-engine/latest.json`.
5. Promote selected files to **stable** via `CopyObjectCommand`.
6. Purge CDN cache (if configured).

Pinned objects are uploaded with:

```ts
ACL: "public-read"
CacheControl: "public, max-age=31536000, immutable"
````

## ⚠️ Important: DigitalOcean Spaces ACL Behavior

DigitalOcean Spaces **does not inherit ACLs when copying objects inside a bucket.**

This differs from AWS S3.

If you use `CopyObjectCommand` without specifying ACL, the resulting object becomes:

```
ACL: private   (default)
```

This causes:

```
HTTP/2 403 Forbidden
cf-cache-status: MISS
```

### Required fix for stable copies

Stable copies must explicitly set ACL:

```ts
await s3.send(new CopyObjectCommand({
  Bucket: SPACES_BUCKET!,
  CopySource: `/${SPACES_BUCKET}/${from}`,
  Key: to,
  ACL: "public-read",            // REQUIRED
  MetadataDirective: "REPLACE",
  ContentType: guessContentType(to),
  CacheControl: "public, max-age=60, s-maxage=60"
}));
```

Without this, stable URLs **cannot** be fetched by verifiers.

## CDN Cache Semantics

### Stable files

* Cached for ~60 seconds.
* Updated in place.
* Clients see new keys and revocations quickly.

### Pinned files

* Immutable.
* Cached for one year (`immutable + max-age`).
* Safe for reproducible verification.

## Validation Checklist (Post-Deployment)

After merging into `main`:

1. Confirm GitHub publish workflow succeeded:

   ```bash
   gh run watch --exit-status
   ```

2. Confirm pinned artifacts are reachable:

   ```bash
   curl -I https://hapis.sfo3.cdn.digitaloceanspaces.com/attestation-engine/<sha>/trust/jwks.json
   ```

3. Confirm stable artifacts are reachable:

   ```bash
   curl -I https://hapis.sfo3.cdn.digitaloceanspaces.com/trust/jwks.json
   ```

4. Confirm the latest pointer is correct:

   ```bash
   curl -s https://hapis.sfo3.cdn.digitaloceanspaces.com/attestation-engine/latest.json | jq
   ```

5. Confirm no HTTP 403 errors:

   * If present → check ACL on stable copies.

## Environment Variables (GitHub Actions)

| Variable          | Description                    |
| ----------------- | ------------------------------ |
| `SPACES_KEY`      | Access key for Spaces (secret) |
| `SPACES_SECRET`   | Secret key for Spaces (secret) |
| `SPACES_BUCKET`   | Bucket name                    |
| `SPACES_REGION`   | Region (sfo3)                  |
| `SPACES_ENDPOINT` | API endpoint for Spaces        |
| `CDN_BASE_URL`    | Public CDN endpoint            |

## When to Use Pinned vs Stable

* **Stable**: OPS workloads, real-time verification.
* **Pinned**: Compliance, audits, offline validation, reproducibility.

This dual-mode strategy ensures the attestation engine is both **high-performance** and **cryptographically deterministic**.

````