# First usable Cvera MVP: current readiness

Weekend 2 remains closed with the owner's explicit live-current-key attestation
proof deferral. Completion at c5fdb5a passed production hashing, deduplication,
private storage and revocation acceptance. The next evidence pilot is integrated
and deployed as **6e7293e75bf0577c1ce9105744249c718fe65361** (PR #136, including focused
PRs #137 and #138). This assessment distinguishes implementation/delivery from
positive authenticated acceptance; it is not a new security audit.

## Implemented evidence pilot

| Slice | Verified evidence | Remaining gate |
|---|---|---|
| Explicit reviewer grants, private listing/download, attributable manual APPROVED/REJECTED, no self-review | Full PostgreSQL authorization/locking/ACL regressions; owner-confirmed 000017 and narrow existing-role bindings after fresh actual backup restore/rehearsal | API deployed/healthy at 6e7293e; anonymous denial and positive two-account HTTP acceptance PASS |
| Expo real assigned case/check selection, PUT followed by completion, safe same-session retry, awaiting-review display | Seven transport test groups, full Node CI, Expo typecheck/web export; existing Production web deployment for 6e7293e and public client markers confirmed | Actual authenticated file-picker and native device acceptance |
| Bounded owner-run case deletion | Local owner-only freeze/revoke/expiry/manifest/redaction and offline partial-failure/resume/storage tests; runbook and dry-run-default tool committed | No production deletion performed; each destructive run needs separate per-case authority |
| Live issuance acceptance preparation | Existing authorized candidate/assigned-employer path and exact legacy public-key/tamper proof documented | Production signing remains owner-deferred, not passed |

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
   signing credential is assumed. The existing HR screen still has a demonstration
   employer selector (`emp_demo_001`) for its separate employment queue; evidence
   selection is independent. A real employer queue/attestation client must use
   authoritative employer assignments before that separate workflow is offered.
3. **The binding/eligibility policy is owner-approved.** One immutable operator-bound
   request/case, explicit required checks, canonical request subject/employer and
   frozen claim context; independent contextual approval; no adoption/reuse; current
   ACTIVE authority for issuer **https://cvera.app**, employer and employment.role.
   [Binding implementation](request-evidence-binding.md) is locally verified on its
   feature branch, not yet deployed. Production authority is intentionally
   unconfigured and eligibility therefore fails closed; no authority privileges
   or live signing are enabled. Bound requests cannot bypass this through legacy
   attestation. Upload, review, employer access, issuer authority and final verifier
   reliance remain separate. The linked document lists the exact next issuance/
   independent-verification work rather than treating a preparation receipt as a
   signed, trusted claim.
4. Before advertising working real-user current-key attestations, obtain the
   separately deferred issuance authorization and perform the prepared normal-API
   signature/tamper acceptance. Signature validity must remain distinct from
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
