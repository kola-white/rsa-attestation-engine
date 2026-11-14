# First usable Cvera MVP: current readiness

Weekend 2 remains closed with the owner's explicit live-current-key attestation
proof deferral. Completion at c5fdb5a passed production hashing, deduplication,
private storage and revocation acceptance. The evidence pilot was integrated
and deployed as **6e7293e75bf0577c1ce9105744249c718fe65361** (PR #136, including focused
PRs #137 and #138). This assessment distinguishes implementation/delivery from
positive authenticated acceptance; it is not a new security audit.

## November 10, 2026 production reconciliation

The [current NorStar baseline and four-stage STOP/GO checklist](../operations/norstar-production-readiness.md)
is authoritative for the remaining pilot operations. This update uses owner-confirmed
production observations and repository/GitHub evidence; no production access occurred.

- **Implemented and delivered:** PR #141 merged as `bce7b9833885b69b94a2f7d6cf85d415cbc6ebac`;
  its [main CI/deployment run](https://github.com/kola-white/rsa-attestation-engine/actions/runs/42534540050)
  succeeded. Deployment is owner-confirmed. The current November 10 live SHA is
  **UNVERIFIED** here; local main is not a deployment receipt.
- **Configured:** NorStar `norstar-systems-llc`, MANUAL, registration transaction
  `3007`; exact `https://cvera.app / norstar-systems-llc / employment.role`
  authority ACTIVE, activation transaction `3019`. **Never repeat either operation.**
- **Outstanding configuration:** owner-confirmed November 10 read-only checks show
  `issuer-prod-2026-10` issuer credential `registered=false`, `active=false`, and
  **zero** eligible active HR principals with unrevoked NorStar assignments.
  These are operational prerequisites, not demonstrated implementation defects.
- **Acceptance:** independent two-account synthetic evidence-review PASS remains
  valid. Tara's genuine independent review and the first genuine NorStar
  evidence-backed issuance/verification remain **PENDING**; no genuine PASS is claimed.

## Implemented evidence pilot

| Slice | Verified evidence | Remaining gate |
|---|---|---|
| Explicit reviewer grants, private listing/download, attributable manual APPROVED/REJECTED, no self-review | Full PostgreSQL authorization/locking/ACL regressions; owner-confirmed 000017 and narrow existing-role bindings after fresh actual backup restore/rehearsal | API deployed/healthy at 6e7293e; anonymous denial and positive two-account HTTP acceptance PASS |
| Expo real assigned case/check selection, PUT followed by completion, safe same-session retry, awaiting-review display | Seven transport test groups, full Node CI, Expo typecheck/web export; existing Production web deployment for 6e7293e and public client markers confirmed | Actual authenticated file-picker and native device acceptance |
| Bounded owner-run case deletion | Local owner-only freeze/revoke/expiry/manifest/redaction and offline partial-failure/resume/storage tests; runbook and dry-run-default tool committed | No production deletion performed; each destructive run needs separate per-case authority |
| Integrated issuance and independent verification | PR #141 merged; owner-confirmed implementation deployment; existing current authority/evidence/credential checks | Public credential registration, eligible HR assignment, Tara's genuine review and separately authorized one-request acceptance remain pending |

Manual review does **not** mark scans CLEAN: received evidence remains PENDING
until an explicit trusted scanning process exists. Review grants are independent
of upload grants, HR roles and uploader ownership. No implicit assignment or
case/employer/request association is created. Automatic scanning, cleanup workers
and self-service administration are not required for a small manually operated pilot.

See [review contract](evidence-review.md), [client integration](evidence-client.md),
[release evidence](../operations/evidence-review-release.md),
[retention procedure](../operations/evidence-retention.md) and
[deferred issuance preparation](../operations/live-issuance-acceptance.md).

## Minimum remaining acceptance and product decisions

1. **API review acceptance is complete.** Two normal independent accounts passed
   explicit grant separation, private canonical-byte download, unauthorized/wrong
   case/check denial, no self-review, attributable exact retry/conflict, revocation
   and cleanup on deployed 6e7293e. Manual approval left scan status PENDING. One
   synthetic object was uploaded; fixture grants were revoked and case closed.
   No new identity, attestation, credential change or destructive deletion occurred.
2. Exercise a harmless synthetic file through the actual client picker with an
   explicitly assigned case/check, receipt, retry and revoked access. Web asset
   delivery/export is not proof of a native file upload. Existing EAS metadata
   defines a development client only; no production mobile distribution or new
   signing credential is assumed. The [HR employer-resolution remediation](../architecture/employer-identity-and-reviewer-resolution.md)
   now resolves the separate employment queue through authoritative HR assignments
   in local source; deployment and Tara's UAT remain pending. Evidence selection
   remains independently authorized. The same document records the UUIDv7 target
   architecture; no canonical-ID migration is implemented.
3. **The binding/eligibility policy is owner-approved.** One immutable operator-bound
   request/case, explicit required checks, canonical request subject/employer and
   frozen claim context; independent contextual approval; no adoption/reuse; current
   ACTIVE authority for issuer **https://cvera.app**, employer and employment.role.
   [Binding implementation](request-evidence-binding.md) is deployed at e84b9a6
   (PR #140); migration 000018 and final CI/deployment passed. Its separate
   two-account/two-request smoke remains **pending/unconfirmed** and must not be
   rerun or represented as accepted. NorStar authority is now owner-confirmed ACTIVE
   (transaction 3019);
   the integrated source/issuance implementation shipped in PR #141. Missing
   public credential registration and HR eligibility still prevent the genuine
   workflow. No genuine live issuance acceptance is claimed. Bound requests cannot
   bypass this through legacy
   attestation. Upload, review, employer access, issuer authority and final verifier
   reliance remain separate. The [current operational checklist](../operations/norstar-production-readiness.md)
   identifies the remaining work; a preparation receipt is not a signed, trusted claim.
4. Before advertising accepted real-user current-key attestations, resolve public
   credential registration, exact HR eligibility and genuine independent case review,
   then obtain separately scoped authorization for the
   [integrated issuance acceptance](../operations/evidence-backed-issuance-release.md).
   The older legacy signature/tamper preparation is not evidence-backed issuance. Signature validity must remain distinct from
   unsupported/unconfigured legacy trust, status and expiry outcomes. Existing
   internal trust-recording and recruiter-consumption routes are not independent
   cryptographic verification. This does not reopen Weekend 2 closure.
5. For sensitive pilot documents, assign the owner-operated deletion request path
   and communicate the actual request-driven retention policy. The bounded tool
   verifies registered and case-prefixed current-object deletion, not historical
   versions/backups or unregistered legacy flat-key orphans. Do not promise an
   automatic retention duration or comprehensive erasure that is not enforced.

Normal password login/exchange/logout and completion already passed production
acceptance. Explicit principal/case and HR/employer grants can support a small
pilot; blanket permissions and self-service administration are unnecessary.

## Follow-on improvements

- Trusted automated malware-scan consumption/retries after its policy is chosen.
- Scheduled expiry/orphan cleanup after the manual retention policy is exercised.
- Review correction workflows, pagination and richer client review UX when pilot
  volume justifies them; the bounded API records one final decision per upload.
- Self-service administration, additional credential formats/providers, HSMs,
  dashboards and nonessential infrastructure remain outside this MVP slice.

## Integrated issuance milestone: objectives 2–5

[Evidence-backed issuance and independent verification](evidence-backed-issuance.md)
is implemented and owner-confirmed deployed through PR #141. It connects the
accepted authority evaluator to locked eligibility, canonical issuer/subject/request
and snapshot identity, immutable manifest commitments, committed portable credential
retrieval and signer-independent verification of current trusted facts. Signature
validity alone never produces trusted VERIFIED.

The historical non-signing release and local full Go/PostgreSQL, race and vet
results remain recorded below and in the [release runbook](../operations/evidence-backed-issuance-release.md).
Recorded local coverage includes normal request-to-portable-claim issuance,
subject retrieval, rollback and current-fact verification/revocation concurrency.
Those tests were not rerun during this documentation-only task.
They do not prove genuine NorStar acceptance. Current ACTIVE organizational
authority does not replace public credential registration, HR assignment or another
human's contextual case review. Missing/inactive/revoked/unavailable trust still
fails closed. No new schema bootstrap, registration or activation replay is needed
merely to complete this documentation update.

The remaining owner-controlled stages are **public credential registration →
eligible NorStar HR assignment → Tara's account and case-review authorization →
one genuine request/issuance/independent-verification acceptance**. Case-specific
review access is granted only after the accepted case/binding exists, within its
separately approved scope. See the [ordered checklist and gap assessment](../operations/norstar-production-readiness.md).
Native picker/device acceptance and the unconfirmed #140 smoke remain separate;
Weekend 2's earlier closure and signing deferral are not silently changed.

### Historical preparation narrative — superseded for current status

The following paragraphs preserve intermediate observations and preparation
receipts from before completion. Statements that registration, schema, deployment,
authority paperwork or activation were still pending describe those historical
stages, **not November 10 status**. The baseline above supersedes them. Do not
replay a historical operation or infer missing acceptance from a preparation PASS.

NorStar Systems LLC (owner-supplied California entity B20250427754) is the proposed
first-party pilot authority subject for the qualified Managing Member — owner
role. Its internal employer ID, representative's power, signed delegation and
attributable adjudication have not been verified or registered. See the
[NorStar preparation and approval gates](../operations/norstar-authority-pilot.md).
PR #141 engineering checks passed at c7e5239; this candidate nomination does not
activate authority or change production acceptance status.

The owner's read-only NorStar reconciliation returned 12 employer registry entries
and no matching candidates, with no data/configuration change. An internal ID is
still unconfirmed because the registry lacks a legal-entity mapping field; a
no-match search is not proof that no opaque existing ID was assigned. Owner
mapping confirmation, signed delegation and a documented attributable
adjudication remain the actionable prerequisites. Third-party authority adjudication is optional; the
mandatory no-self-review policy for uploaded case evidence remains unchanged.
No authority or signing activation follows from nomination or from the filing number alone.

The [NorStar preactivation sequence](../operations/norstar-preactivation-sequence.md)
now includes executable owner-guarded employer registration and authority recording
that stops at APPROVED_NOT_ACTIVE. Local isolated PostgreSQL regression passed;
no production write occurred. Owner still needs to confirm the legal/internal ID
mapping, finish the authentic signed delegation and separate first-party decision,
nominate an eligible non-uploader case reviewer, and explicitly authorize any future
schema/recording/activation/credential/signing operations. First-party authority
adjudication does not satisfy or waive no-self-review of uploaded evidence.

NorStar's prepared new registration now uses MANUAL rather than AUTO. Source and
local regression confirm MANUAL normal submission remains SUBMITTED, preserves
the mode and creates no HR/review/upload grants, authority or credential. A later
AUTO dispatch window requires explicit approval; there is no existing manual
dispatch route and direct status patching is excluded. The owner-executed registry preflight and fresh actual-backup restore/rehearsal
subsequently passed for the pinned registry operation; this does not establish
authority-schema or live-issuance acceptance. See the
[reviewed preflight/restore sequence](../operations/norstar-schema-backup-review.md).

The owner has authorized the consolidated read-only NorStar preflight/mapping
searches and fresh private backup/isolated same-image rehearsal. The owner executed driver d1c48d0 through the existing root console and returned
PASS: registry schema/collision checks, fresh private backup, isolated same-image
restore, and clone-only MANUAL rollback/commit rehearsal. Zero candidate mappings
were found; no verified existing legal mapping was discovered. Production SQL was
read-only and registry/privileges/services remained unchanged. The private receipt
is /opt/cvera/norstar-preparation-20261108T220104Z-9065a0/preparation-result.json.
Registration and authority recording were explicitly NOT executed.
The owner has since authorized completion of the one-row MANUAL registration.
Its guarded executor and replay/ambiguous-commit regressions are implemented.
Production registration **PASS**: owner-console result for pinned a33f7ba reports
transaction `3007`, `norstar-systems-llc`, MANUAL, California B20250427754, first
insertion with no authority/credential operations or service restarts. Do not
repeat insertion. Signed
delegation, attributable first-party adjudication and a separate case reviewer
remain outstanding. Registration authorizes none of the reserved authority,
credential, migration, deployment or live-signing operations.
Production registration remains separately gated, independent of the still-blocked
signed authority paperwork and case reviewer. No ACTIVE authority or signing is
included in the prerequisite authorization.

### Executable integrated issuance release/acceptance preparation

PR #141 now includes a read-only production preparation / isolated archive-restore
rehearsal and a separately approved atomic authority bootstrap (existing 000002–
000013) plus 000019/five-EXECUTE apply phase. Empty registries fail closed and no
migration history, role, authority or credential is invented. Prepared owner-only
activation/containment SQL and public-credential registration retain the accepted
contracts. The genuine acceptance client uses normal request/evidence/issuance
APIs, another human's explicit contextual review, one durable signing attempt,
portable artifact retrieval, separate public-key signature checking and current
trusted verifier facts. It preserves genuine evidence and PENDING scans.

Local operations regressions pass for rollback/commit/failure, same-image isolated
archive restore, hostile table/sequence ACL stripping, actual workflow/receipt SQL,
activation/containment denials and independent local RSA-3072 verification. These
are not production acceptance. Production authority schema/000019 apply, real
recording/activation, credential registration, deployment and live signing remain
unexecuted without their separate owner approvals. Signed NorStar packet and a
separate human reviewer remain external inputs; they do not block an authorized
non-signing implementation release. See the existing
[executable release procedure](../operations/evidence-backed-issuance-release.md).

Production issuance-schema preparation **PASS**, returned from the existing owner
console using pinned executor d3ee63c and SQL source ab0c0f6: fresh private evt
archive restored on the same PostgreSQL image; rollback/commit, actual normal
request/review/receipt probe and five-EXECUTE privileges passed in the isolated
clone. Production SQL was read-only; no schema was applied and no authority,
credential or service changed. This resolves production schema/default/type
compatibility for the proposed atomic release; it does not activate trust or prove
a production signature. The private preparation receipt is retained on the Droplet
under `issuance-release-20261108T224912Z-02ce57` for the separately approved apply.
