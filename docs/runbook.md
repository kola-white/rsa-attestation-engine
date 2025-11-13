# Runbook — Trust Artifact Verification & Deployment Issues

This runbook provides **action-oriented procedures** for diagnosing and fixing issues
related to publishing trust artifacts into DigitalOcean Spaces and serving them via CDN.

---

# 1. Symptoms & Diagnoses

## ❌ 1.1 Stable URLs return HTTP 403

**Example:**
````

curl -I [https://hapis.sfo3.cdn.digitaloceanspaces.com/trust/jwks.json](https://hapis.sfo3.cdn.digitaloceanspaces.com/trust/jwks.json)
HTTP/2 403
cf-cache-status: MISS

```

### Cause
Stable artifacts created via `CopyObjectCommand` were published without:

```

ACL: public-read

````

DigitalOcean Spaces does **not inherit ACLs**, so copies default to **private**.

### Fix
Ensure `publish.ts` contains:

```ts
ACL: "public-read"
````

Then re-run a publish (merge to main or manually trigger).

---

## ❌ 1.2 Pinned artifacts accessible, but stable artifacts not updated

### Cause

* Stable copy step failed or skipped.
* CDN cache not purged.
* Wrong prefix passed to `CopyObjectCommand`.

### Fix Procedure

1. Confirm pinned file exists:

   ```bash
   curl -I https://hapis.sfo3.cdn.digitaloceanspaces.com/attestation-engine/<sha>/trust/jwks.json
   ```

2. Confirm stable link points to the same content:

   ```bash
   curl -I https://hapis.sfo3.cdn.digitaloceanspaces.com/trust/jwks.json
   ```

3. Re-run stable promotion:

   * Check GitHub logs for `refreshed_stable` audit.
   * Manually purge CDN (if PAT provided).

---

## ❌ 1.3 CDN returning “MISS” repeatedly

### Expected Behavior

* MISS is common after a new deploy or purge.
* Should transition to HIT after TTL expires.

### Possible Causes

* Very short TTL during high-refresh load.
* “immutable” flags missing from pinned objects.

### Fix

Confirm pinned objects include:

```ts
CacheControl: "public, max-age=31536000, immutable"
```

Stable objects include:

```ts
CacheControl: "public, max-age=60, s-maxage=60"
```

---

## ❌ 1.4 /attestation-engine/latest.json not resolving correctly

### Symptoms

```
{
  "sha": "",
  "prefix": ""
}
```

### Causes

* Publish pipeline failed before writing latest pointer.
* Spaces overwrite issue.

### Fix

1. Check GitHub Action logs for “updated_latest”.
2. Manually GET the file:

   ```bash
   curl -s https://hapis.sfo3.cdn.digitaloceanspaces.com/attestation-engine/latest.json
   ```
3. Re-run publish if incorrect.

---

# 2. Verification Commands (Quick Reference)

### Pinned JWKS

```bash
curl -I https://hapis.sfo3.cdn.digitaloceanspaces.com/attestation-engine/<sha>/trust/jwks.json
```

### Stable JWKS

```bash
curl -I https://hapis.sfo3.cdn.digitaloceanspaces.com/trust/jwks.json
```

### Latest Pointer

```bash
curl -s https://hapis.sfo3.cdn.digitaloceanspaces.com/attestation-engine/latest.json | jq
```

### Check CDN metadata quickly

```bash
curl -I <url> | grep -E 'cf-|cache|etag|last-modified'
```

---

# 3. Release Checklist

Before merging:

* [ ] `publish.ts` contains `ACL: "public-read"` in both `PutObject` and `CopyObject`
* [ ] Pinned objects upload tested locally (optional)
* [ ] `latest.json` will be written last
* [ ] GitHub variables/secrets set:

  * SPACES_KEY
  * SPACES_SECRET
  * SPACES_BUCKET
  * SPACES_REGION
* [ ] CDN purge enabled if DO_PAT + endpoint ID provided

After merge:

* [ ] Publish workflow succeeded
* [ ] Pinned files reachable
* [ ] Stable files reachable
* [ ] No 403 errors
* [ ] `cf-cache-status` eventually becomes HIT for stable URLs

---

# 4. Emergency Procedures

## 🔥 4.1 Stable trust artifacts are broken (403 everywhere)

### Quick Fix

1. Patch `publish.ts` to enforce ACL.
2. Merge into main.
3. Trigger manual workflow dispatch.
4. Purge CDN (if PAT available):

   ```bash
   doctl compute cdn flush <ID> --files "/trust/jwks.json" --files "/status/statuslist.json"
   ```

### Verify:

```bash
curl -I https://hapis.sfo3.cdn.digitaloceanspaces.com/trust/jwks.json
```

---

## 🔥 4.2 Accidentally pushed incorrect trust artifacts

### Prevent fallout:

* Pinned artifacts remain immutable—safe.
* Only stable artifacts are affected.

### Remediation:

1. Re-run publish.
2. Purge CDN.
3. Re-check stable outputs.

---

# 5. Notes for Maintainers

* Spaces does **not** propagate ACLs.
* Pinned mode is your “source of truth” for historical verification.
* Never mutate or delete pinned artifacts.
* Stable artifacts are safe to overwrite (they're pointers).
* CDN “MISS” is normal during propagation.

---

This runbook ensures fast recovery, predictable deployments, and operational clarity for the attestation trust pipeline.

```
