# Expo evidence completion integration

Depends on the review/discovery API in PR #136 and migration 000017. Do not distribute
this client as upload-ready before that backend is deployed and accepted. No native
production build or app distribution is authorized by this implementation document.

The existing HR evidence section now loads `/v1/evidence/checks` using the current
API token, filters explicit `canUpload` assignments and asks the user to choose the
case/check. There is no demo identifier, automatic case creation or mapping from the
selected employment request. The picker explicitly labels evidence as a separate
destination; server authorization remains authoritative even if local state is stale.

The internal app transport performs init, PUT with exactly the returned storage
headers, then completion with the returned session/upload IDs and expected size/MIME.
The API token is sent only to API requests. No client digest, principal assertion,
storage key or URI is submitted as completion authority. The UI acknowledges receipt
only after the response confirms RECEIVED/PENDING and the matching identifiers/hash.
It does not treat a PUT as completed evidence or display a verification verdict.

An uncertain completion retains only a non-capability session/file ticket in memory
and offers Retry completion. This reuses server idempotency without another init or
PUT. Permanent expiry/authorization/conflict failures show a bounded friendly message.
No ticket, signed URL, header, raw storage error body or token is logged or persisted.
Account changes/unmount discard pending UI data; this is not cross-device resume.
Local UX hashes remain in memory, case/check scoped, and are recorded only after a
confirmed receipt. They are never canonical hashes and never transmitted.

Native uploads use the existing binary FileSystem path; the web path preserves the
picker Blob URL and uses its exact bytes. Both retain the existing PDF/JPEG/PNG
allowlist and use the actual API limit of 5,242,880 bytes (5 MiB), correcting the
client's older rounded-down decimal limit. No dependencies, native permissions or
production configuration are changed.

## Verification

- Baseline and updated `npm run typecheck:expo`: PASS.
- `npm run test:evidence-client`: seven lifecycle regression groups PASS, including
  authorization/header isolation, exact IDs/body, absent client hashes, immutable
  PENDING receipt, missing/malformed responses, uncertain completion retry without
  re-upload, permanent failures, privacy and real assignment shapes.
- `npm run test:ci`: existing schema/vector checks plus those regressions PASS.
  They are appended to the existing command; no new workflow or test dependencies.
- Existing Expo CLI web export: PASS, artifacts only in a temporary local directory.
- `git diff --check`: PASS.

Tests use synthetic transport/files only. A physical iOS/Android picker upload,
actual web file selection and production device acceptance remain unverified.
After backend acceptance, test on the existing dev client with an explicitly granted
synthetic pair: choose assigned destination, select PDF or photo, confirm RECEIVED
with PENDING, exercise a completion retry and expired/revoked-case denial, then
revoke/close that synthetic fixture. Do not log credentials, URLs or file contents.
The previously accepted live-signing deferral remains unchanged.
