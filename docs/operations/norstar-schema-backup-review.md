# NorStar production preparation commands: review only

**Owner authorization now covers read-only production preflight/mapping searches,
fresh private backup and isolated same-image restore/registry rehearsal only.**
Use the pinned `prepare-norstar-production.py --approved-preparation` wrapper for
that scope through the existing root console. Registration and authority recording
remain unapproved production writes. No dispatch change, ACTIVE authority,
credential activation or live signing is included. Unsigned/unverified paperwork
is not completed evidence; it is not required to run these operational prerequisites.
No new SSH account, password or service restart is needed.

## Dispatch and registry decision

Proposed ID: `norstar-systems-llc`, only if no authoritative existing mapping is
found. Initial mode is **MANUAL**, the supported non-dispatching mode. In
`Repo.CandidateSubmit`, AUTO advances a normal submission to ATTESTATION_PENDING;
MANUAL leaves it SUBMITTED and records DISPATCH_GUARD_BLOCKED. Registration and
submission create no HR/case grants, issuer authority, credential or attestation.
`ensureEmployerConfig` uses ON CONFLICT DO NOTHING, so it does not reset an existing
MANUAL employer to AUTO. No external dispatch worker was found in this repository.

These are source-code findings, not proof of uninspected production trigger behavior.
The registration script refuses user INSERT triggers/rewrite rules. Catalog checks
and actual-schema restore rehearsal must confirm MANUAL and the insert have no
unexpected effects; do not disable or bypass a constraint/trigger to register it.

A later live acceptance will not work by submitting in MANUAL: there is no existing
manual-dispatch API. Any bounded AUTO window and MANUAL restoration need explicit
future authorization and checks for other affected requests. Do not patch status.

## Exact prerequisites

- `evt` on PostgreSQL 16, verified owner connection; live tables owned by postgres,
  public schema resolution verified, no unexpected registry RLS or INSERT side effects.
- Confirmed employer mapping: the previous 12-row/no-candidate result is not proof
  that no opaque existing ID belongs to NorStar. Any exact ID collision or verified
  prior mapping means STOP/reuse review, never INSERT-on-conflict/update/alias.
- Registry schema: employer_id TEXT primary key, dispatch_mode TEXT NOT NULL,
  created_at/updated_at TIMESTAMPTZ with verified safe timestamp defaults. Existing
  constraints must allow MANUAL. Inspect/rehearse actual defaults and constraints
  privately; do not print unknown expressions, trigger arguments or bodies to chat.
- Registry-only insertion needs no authority tables, new privileges or application
  migration. It is a separate one-row operator action, not organizational verification.
- Authority recording additionally requires the compatible existing 000002–000013
  catalog: four owner-controlled authority tables, canonical scope/FKs/immutability,
  recording_position columns, singleton boundary and allocating_xid, the trusted
  allocator and deferred one-row recording guards. Do not assume these are installed.
  Wholly absent state requires a separately reviewed bootstrap/ACL plan; partial
  state is STOP. No 000001/history fabrication or ad hoc grants.
- The later release separately requires 000019, its five bounded runtime EXECUTEs
  and exact application compatibility. Nothing below installs that implicitly.
- Fresh private evt archive, checksum, successful full restore in a network-none
  disposable container on the same PostgreSQL image, no shared live volume. For
  the established 64 MiB rehearsal, retain at least 64 MiB host headroom as well
  and enough disk for backup, restore and recovery. Archive listing is not restore proof.
- Rehearse proposed statements against the restored real schema before production
  authorization. Use explicitly LOCAL-only inputs in the clone, never treat those
  as actual evidence or copy resulting authority rows into production.

## Minimum command sequence for later approved execution

Stage the pinned reviewed files privately; verify their approved SHA-256 values.
Do not switch the running checkout to the feature branch. `CVERA_STAGE` below is
that root-private reviewed stage, not an unverified downloaded script.

**1. Read-only metadata/collision check:**

```sh
docker exec -i evt-api-go-db-1 psql -X -U postgres -d evt -Atq -v ON_ERROR_STOP=1 < "$CVERA_STAGE/norstar-preparation-preflight.sql"
```

This returns only catalog flags and candidate employer IDs. Any UNREVIEWED default
requires private inspection and rehearsal before write authorization. It does not establish
legal mapping or certify schema completeness from table names alone.

**2. Fresh backup and isolated restore:** use `set -euo pipefail` and umask 077.
The following is a reviewable command block, not permission to execute it now.
Capacity checks and any unexpected catalog condition must be resolved first.

```sh
set -euo pipefail
umask 077
CVERA_REQUIRED=$(docker exec evt-api-go-db-1 psql -X -U postgres -d evt -Atq -v ON_ERROR_STOP=1 -c 'SELECT pg_database_size(current_database())*4+1073741824')
python3 - "$CVERA_REQUIRED" <<'PY'
from pathlib import Path
import shutil,sys
assert shutil.disk_usage('/opt/cvera').free>=int(sys.argv[1]), 'STOP: backup/restore disk capacity'
mem=next(int(line.split()[1]) for line in Path('/proc/meminfo').read_text().splitlines() if line.startswith('MemAvailable:'))
assert mem>=128*1024, 'STOP: insufficient restore memory/headroom'
PY
CVERA_BACKUP=$(mktemp -d /opt/cvera/norstar-preparation-backup.XXXXXX)
CVERA_CLONE="cvera-norstar-restore-${CVERA_BACKUP##*.}"
CVERA_IMAGE=$(docker inspect --format '{{.Image}}' evt-api-go-db-1)
docker exec evt-api-go-db-1 pg_dump -U postgres -d evt -Fc > "$CVERA_BACKUP/evt.dump"
sha256sum "$CVERA_BACKUP/evt.dump" > "$CVERA_BACKUP/backup.sha256"
sha256sum -c "$CVERA_BACKUP/backup.sha256"
docker exec -i evt-api-go-db-1 pg_restore --list < "$CVERA_BACKUP/evt.dump" > "$CVERA_BACKUP/archive-list.private"
# Only role NAMES are copied for restore ownership/ACL references; no passwords,
# memberships, production role attributes or private role configurations.
docker exec evt-api-go-db-1 psql -X -U postgres -d evt -Atq -v ON_ERROR_STOP=1 -c "SELECT format('CREATE ROLE %I NOLOGIN;',rolname) FROM pg_catalog.pg_roles WHERE rolname<>'postgres' AND rolname !~ '^pg_' ORDER BY rolname" > "$CVERA_BACKUP/clone-role-stubs.sql"
docker run -d --pull never --name "$CVERA_CLONE" --network none \
 --memory 64m --memory-swap 64m --cpus 0.5 --shm-size 16m \
 --label cvera.scope=norstar-backup-rehearsal \
 -e POSTGRES_HOST_AUTH_METHOD=trust \
 -e 'POSTGRES_INITDB_ARGS=-c shared_buffers=8MB -c max_connections=10 -c maintenance_work_mem=4MB -c autovacuum=off' \
 "$CVERA_IMAGE" -c shared_buffers=8MB -c max_connections=10 \
 -c work_mem=512kB -c maintenance_work_mem=4MB -c autovacuum=off \
 -c max_worker_processes=0 -c max_parallel_workers=0
# TCP readiness excludes the temporary Unix-socket-only initialization server.
CVERA_READY=false
for attempt in $(seq 1 90); do
 if docker exec "$CVERA_CLONE" psql -X -h 127.0.0.1 -U postgres -d postgres -Atqc 'SELECT 1' >/dev/null 2>&1; then CVERA_READY=true; break; fi
 sleep 1
done
[[ "$CVERA_READY" == true ]]
[[ "$(docker inspect --format '{{.HostConfig.NetworkMode}}' "$CVERA_CLONE")" == none ]]
# Before restoring, verify this clone has no source volume shared with the live DB.
docker inspect --format '{{range .Mounts}}{{.Source}}{{println}}{{end}}' "$CVERA_CLONE" > "$CVERA_BACKUP/clone-mounts.private"
docker inspect --format '{{range .Mounts}}{{.Source}}{{println}}{{end}}' evt-api-go-db-1 > "$CVERA_BACKUP/live-mounts.private"
python3 - "$CVERA_BACKUP" <<'PY'
from pathlib import Path
import sys
p=Path(sys.argv[1])
a=set((p/'clone-mounts.private').read_text().splitlines())-{''}
b=set((p/'live-mounts.private').read_text().splitlines())-{''}
assert not a.intersection(b), 'STOP: shared production volume'
PY
docker exec -i "$CVERA_CLONE" psql -X -U postgres -d postgres -q -v ON_ERROR_STOP=1 < "$CVERA_BACKUP/clone-role-stubs.sql"
docker exec "$CVERA_CLONE" createdb -U postgres evt
docker exec -i "$CVERA_CLONE" pg_restore -U postgres -d evt --exit-on-error < "$CVERA_BACKUP/evt.dump"
```

Keep that clone network-none and private. Rehearse the reviewed registry/script
steps only there and verify unchanged existing records/ACLs, MANUAL behavior and
APPROVED-not-ACTIVE. If schema is absent, rehearse only the separately approved
bootstrap on the clone first. Stop the clone afterward to free RAM; retain the
archive/receipts for recovery. Never substitute successful fixture tests or an
archive listing for this actual-backup restore/rehearsal. Raw dump/listing/input
files are not terminal output or attachments.

**3. Only after explicit one-row registration approval:**

```sh
cat "$CVERA_REGISTRATION_INPUT" "$CVERA_STAGE/register-norstar-employer.sql" | docker exec -i evt-api-go-db-1 psql -X -U postgres -d evt -Atq -v ON_ERROR_STOP=1 -v VERBOSITY=sqlstate
```

Inputs are real, reviewed, root-private session SET statements (0600 inside a 0700
directory), not command-line secrets or placeholders. The three required settings
are employer ID, verified mapping reference and actual production approval reference.
This command inserts MANUAL, never AUTO; an existing ID/unknown side effect stops it.

**4. Only after separate schema verification AND approval to record completed evidence:**

```sh
cat "$CVERA_DECISION_INPUT" "$CVERA_STAGE/record-norstar-authority-approved.sql" | docker exec -i evt-api-go-db-1 psql -X -U postgres -d evt -Atq -v ON_ERROR_STOP=1 -v VERBOSITY=sqlstate
```

The [preactivation input contract](norstar-preactivation-sequence.md) specifies all
actual signed/verified decision inputs. The SQL checks required references; it
cannot authenticate paper signatures merely from a nonempty reference. The operator
must finish the documented evidence checks before authorizing APPROVED. The four
transactions stop at APPROVED_NOT_ACTIVE; ACTIVATE, credential activation, dispatch
changes and live signing are excluded. An ambiguous result/partial commit is STOP:
inspect exact recorded IDs, never delete audit, blindly rerun or force activation.

Recovery at this stage is to stop, roll back any uncommitted transaction and leave
additive committed records/history intact pending reviewed correction. No service
or password changed, so no application rollback/credential restoration is implied.

## Consolidated authorized prerequisite operation

`operations/prepare-norstar-production.py --approved-preparation` automates the
already authorized read-only preflight, candidate/legal-number search, private
registry/privilege inventory, capacity checks, fresh backup, same-image network-none
restore, and registry rollback/commit rehearsal **only in the clone**. It pins the
reviewed SQL at 8e1add1, verifies source registry/privileges and running services
remain unchanged, stops the clone to release RAM, retains recovery material root-
private, and returns only PASS/STOP JSON. It never invokes the authority recording
script, a production registry INSERT, a signer or a credential operation.

Candidate absence is not fabricated legal proof: existing registry data has no
legal-entity-number mapping. A PASS records no verified existing mapping discovered;
the next bounded registration approval must explicitly establish the proposed
legal mapping, subject to any authoritative owner records. If a candidate or exact
ID is found, STOP for reconciliation rather than create a duplicate. This is not
another approval cycle for preflight or backup; they are already authorized.

The consolidated driver passed the existing isolated local regression, including
source READ ONLY enforcement, actual fixture archive restore, both registry
transaction outcomes, unchanged unrelated data/ACLs and no source insertion. The
production result must still come from its execution in the actual owner console.
Return only the final JSON; do not send backup, inventory, dump listing or private
files. Registration/delegation/reviewer readiness does not gate this prerequisite
operation; actual registration still needs its separate explicit authorization.

## Actual production prerequisite result

Owner returned **PASS** from pinned driver d1c48d0 (reviewed SQL 8e1add1): registry
schema/collision checks, zero candidate mappings, fresh private archive, isolated
same-image restore, and registry rollback/commit rehearsal only in that clone.
Source registry/privileges were unchanged; no service restarted, no production
registration/authority recording occurred, and no credential/signing was used.
The root-private receipt is
`/opt/cvera/norstar-preparation-20261108T220104Z-9065a0/preparation-result.json`.

The next decision is only whether to establish the owner-approved legal mapping
NorStar Systems LLC / California B20250427754 to the new internal ID
`norstar-systems-llc` and insert ONE MANUAL registry row using pinned
`register-norstar-employer.sql` (8e1add1; SHA-256
`0c693f55f3752da59726833ffe5673ba150b7f16f8d1ccb4b5b163d3fbd0eb1b`).
Any newly found authoritative existing mapping/collision stops creation. That
registration does not require the separate signed delegation, first-party authority
decision or case reviewer, and authorizes none of those later operations.

The owner subsequently directed completion of this exact registration. The bounded
executor `operations/register-approved-norstar.py --execute-approved-registration`
uses that authorization and the existing verified private backup, rechecks source
SQL hashes/collisions/catalog and runs only the pinned MANUAL insertion. It keeps
the owner-approved legal mapping and transaction receipt private. A verified prior
receipt returns PASS without inserting; an unexplained existing row or unresolved
intent returns STOP rather than adopting a mapping or repeating a possible COMMIT.
No authority, credential, migration, grant, dispatch or service operation is included.
Production registration remains unconfirmed until its console result is returned.
