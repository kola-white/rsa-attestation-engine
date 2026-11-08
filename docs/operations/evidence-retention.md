# Minimal case evidence retention and deletion

This is an owner-run, request-driven pilot procedure, not an automatic retention
schedule. No periodic cleanup, retention SLA or complete personal-data-erasure
claim is made. Collect sensitive pilot evidence only after the owner establishes
who receives deletion requests and which case is authorized for deletion.

The procedure requires the schema through 000017. It runs only in the existing
DigitalOcean root console, using the verified PostgreSQL table owner `postgres`
and existing in-memory storage configuration. Runtime and provisioning operators
receive no deletion privileges. There is no new administration endpoint, service,
credential or role. **No production deletion has been performed or authorized by
this document.** Each destructive run requires explicit owner authority for that
case, including confirmation that any legal retention obligation permits deletion.

## Scope and limits

The procedure freezes one exact case, revokes both upload and review grants, waits
for issued ten-minute PUT capabilities to expire plus a 60-second request margin,
then deletes and confirms absence of its registered and case-prefixed current
objects. It redacts file names, object keys and manual review reasons only after
storage absence is verified, retaining minimal receipt IDs, hash/size/MIME and
review decision/actor/time attribution. It leaves identities, employer requests,
attestations, credentials and migration history unchanged.

New sealed writes use `sealed-evidence/<case>/<check>/<upload UUID>/<nonce UUID>`.
The independently generated server nonce preserves isolation from staging PUTs.
This permits a bounded case-only search for failed-write orphan objects. Previously
accepted flat sealed keys remain readable and are deleted only when registered to
that case. Unregistered legacy global flat keys cannot be attributed safely: they
are **not swept** by this operation. Never broaden a prefix to find them.

Deletion is irreversible for the selected current objects. Backups, historical
object versions, provider media retention and unregistered legacy flat orphans are
outside this proof. Existing private recovery backups may contain metadata; their
separate owner-approved expiry policy must be honored. Do not destroy recovery
material simply to claim erasure. The retained audit is still data, and this is
not a promise that every personal identifier is erased.

## Ordered owner procedure

1. Review the exact case and written deletion basis privately. Check that current
   protected recovery material and its restore evidence are available, and identify
   the approved source commit. A backup aids operational recovery; it does not make
   reinstatement of deleted personal evidence an acceptable default.
2. In the existing root console, stage these four reviewed files together in a
   root-private directory: `purge-evidence-case.py`, `evidence_storage_operator.py`,
   `evidence-case-delete-freeze.sql`, `evidence-case-delete-redact.sql`. Verify their
   hashes against that source commit. Do not print `.env`, DSNs, object keys or
   document contents. The tool reads existing API storage settings in memory.
3. Run the default read-only plan, replacing the placeholder with the exact case:

   ```sh
   python3 /path/to/reviewed/purge-evidence-case.py --case EXACT_CASE
   ```

   It returns counts and the expiry wait only. Object counts are deletion
   candidates, including already absent registered objects. More than 1,000 keys,
   unexpected key shapes, inaccessible storage or an unknown case stop the plan.
4. Only with explicit per-case destructive authorization, execute once:

   ```sh
   python3 /path/to/reviewed/purge-evidence-case.py --case EXACT_CASE \
     --actor OWNER_AUDIT_REFERENCE --basis APPROVED_DELETION_BASIS \
     --apply-and-acknowledge-permanent-deletion
   ```

   Use attributable, non-sensitive audit references, never credentials or document
   contents. The tool first writes a private intent, atomically locks/freezes the
   case and revokes grants, waits for URL expiry, deletes exact scoped targets,
   confirms authenticated HEAD 404 and empty case prefixes, then atomically redacts
   metadata and marks the case DELETED. A 403 is never considered absence.
5. Retain the PASS receipt under `/opt/cvera/evidence-deletions` (0700 directory,
   0600 file). It records source hashes, actor/basis, operation, counts and manifest
   hash without document contents or credentials. Successful receipts discard raw
   target keys. Do not reopen or automatically regrant a deleted case.

## Stop and recovery

STOP leaves the case frozen whenever freezing has committed. It does not grant
access back, terminate sessions or restore objects. Inspect the private receipt
locally; never paste it or raw storage/database errors into public logs.

After resolving the specific storage/availability problem, resume **the same**
operation with the same reviewed files, case, actor and basis:

```sh
python3 /path/to/reviewed/purge-evidence-case.py --case EXACT_CASE \
  --actor OWNER_AUDIT_REFERENCE --basis APPROVED_DELETION_BASIS \
  --apply-and-acknowledge-permanent-deletion \
  --resume-record /opt/cvera/evidence-deletions/EXACT_OPERATION.json
```

The tool verifies source hashes, storage identity, frozen operation marker and
revoked grants. It retains exact legacy registered targets across partial failures.
An ambiguous final SQL commit is verified rather than blindly repeated. Changed
state, another operation, a changed manifest or a completed receipt requires
inspection. Never delete the receipt or change its phase to force a retry.

## Verification

Local disposable PostgreSQL tests verify owner-only SQL, transactional freeze,
revoked access on init/completion/download, expiry enforcement, manifest mismatch
rejection, redaction with decision/audit and PENDING scan status preserved, and
case-attributable sealed bytes. Offline fault-injection tests verify cross-case
isolation, legacy registered-key handling without global orphan deletion, partial
storage failure/resume, no metadata redaction before confirmed absence, expiry
bounds, SDK-compatible signing and malformed-storage failure. These tests do not
claim a production deletion or provider version/physical-media purge.
