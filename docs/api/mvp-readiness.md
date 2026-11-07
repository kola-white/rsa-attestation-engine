# First usable Cvera MVP: minimum remaining API work

Assessment based on the routes and clients at the accepted PR #134 release, not
a new repository-wide security audit. Weekend 2 remains closed with its explicit
live-current-key attestation proof deferral. Evidence completion is released at `c5fdb5a`: migration 000016, fresh backup restore,
CI/deployment and authenticated synthetic hashing/deduplication/private-storage
acceptance passed. See [release evidence](../operations/evidence-completion-lifecycle.md#production-acceptance--2026-11-07).

## Required for a usable evidence-backed verification pilot

1. **Connect the existing client to completion.** The HR upload helper in
   `apps/evt-ui-expo/screens/HRReviewScreen.SettingsStyle.tsx` still intentionally
   stops after PUT, describes completion as 501, and uses placeholder case/check
   identifiers. It must use the real operator-provisioned pair and returned session/
   upload IDs, call completion after PUT, and display pending/failure/retry state.
   An API endpoint alone does not make the current client finish an upload.
2. **Provide one trusted evidence review path.** The API currently has no authorized
   evidence listing/download or bounded review transition; scan jobs remain pending.
   For a small pilot, explicit operator-controlled manual review is a smaller next
   step than a new scanner platform. Choose a reviewed status policy and access
   boundary before allowing review/download or treating evidence as verified.
   Do not let uploaders or the runtime mark arbitrary evidence CLEAN, and do not
   expose pending files publicly. Keep the existing employment attestation workflow
   separate until an explicit product relationship is designed; no case-to-request
   association may be inferred.
3. **Prove the product's issuance-to-verification path once.** Candidate request
   create/submit, employer-assigned review/attest and recruiter consumption routes
   already exist in `internal/evt/routes.go`. Positive production current-key signing
   and verification was deliberately deferred because no current-key attestation
   existed and new issuance was not authorized. Before advertising working real-user
   attestations, run one specifically authorized controlled issuance/verification
   acceptance and state the existing legacy verifier's trust/status/expiry limits.
   This does not retroactively reopen or mislabel the Weekend 2 acceptance decision.
4. **Define a workable evidence retention/deletion operation before collecting real
   sensitive documents.** Completion currently retains private staging, accepted and
   potentially orphaned objects without an implemented deletion worker. A small,
   bounded operator procedure can be enough for a pilot; a distributed cleanup
   platform is unnecessary. Record what is retained, how deletion is requested and
   confirmed, and avoid claiming a retention SLA the code does not enforce.

Normal password login/exchange and logout have already passed production acceptance.
Explicit HR/employer and principal/case grants can be provisioned by the owner for a
small pilot using the existing bounded functions. Self-service administration is
not required to prove the first workflow. Each real user's access still needs an
explicit, attributable assignment; never grant blanket access to ease onboarding.

## Follow-on improvements

- Automatic malware-scan consumption and retries after a trusted worker policy is
  chosen; preserve PENDING until that process exists.
- Automated expiry/orphan cleanup after the initial retention operation is defined.
- Self-service case/grant administration, pagination and richer evidence lifecycle UX
  once pilot volume justifies them.
- Further credential formats, providers, HSMs, dashboards or infrastructure expansion
  are outside the minimum MVP and are not prerequisites for this slice.

If the initial launch is only private evidence collection, rather than evidence-backed
claims issuance, review/issuance can remain explicitly unavailable. Real-user claims
must not be advertised as verified based only on RECEIVED/PENDING uploads.
