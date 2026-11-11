# Independent evidence-review client

## Delivery boundary

The `Evidence reviews` screen is implemented on `feature/evidence-review-ui`.
It has not been deployed or accepted against genuine NorStar evidence. Stage 1
(credential transaction 3035) and Stage 2 (HR assignment transaction 3059) remain
owner-confirmed PASS; this frontend change repeats neither operation.

The screen uses the existing authenticated app, NativeWind utility classes,
system typography, zinc surfaces, sky action colors, rounded cards, stack
navigation and existing session exchange. No backend, schema, role, grant,
authentication mechanism, dependency or infrastructure change is included.

## Reviewer instructions (after deployment and explicit case authorization)

1. Sign in at **https://mvp.cvera.app/login** with your own account. Never share
   your password or codes. Select **Evidence reviews** in the authenticated
   screen header (or on Home).
2. Select your assigned **case / check**. If the list is empty, ask the operator
   to confirm your case-specific review grant; an HR assignment is insufficient.
3. Read **Employment claim** and its complete asserted facts. Use **Show request
   identifiers and integrity details** when reconciling the exact request/subject.
4. For each attachment, select **Open / download document**. Open the downloaded
   PDF/image in a trusted document viewer. On Android, select a private folder
   and open the saved file there; on iOS, use the system handoff. Unknown types
   use a download alternative rather than inline execution. Do not upload these
   private documents to third-party viewers.
5. After actually inspecting the document and claim, check **I personally
   inspected this document and the employment claim above**. Choose **Approve**
   or **Reject**, then enter your own rationale. Neither decision is preselected.
6. Select **Review decision before submitting**, check the decision and rationale,
   then select **Confirm and record decision**. A rejection is a valid outcome;
   do not approve unsupported facts to complete acceptance.
7. Wait for **Review recorded** with the server's decision, reason, reviewer,
   timestamp and receipt ID. Repeat for every required check. If confirmation is
   ambiguous, use **Refresh recorded reviews** and reconcile the existing record;
   do not assume success or submit an alternate decision. Existing final decisions
   are displayed without editing controls.

Malware **PENDING** remains separate from human approval. Use a trusted viewer
for unscanned documents. Remove explicit downloaded/saved copies when no longer
needed. The app does not assert that initiating a download proves inspection.

## Contracts and safeguards

- `GET /v1/evidence/checks`: reuse assignment discovery; show only `canReview`.
- `GET /v1/cases/{case}/checks/{check}/evidence`: validate exact scope and render
  canonical binding, claim snapshot, received attachments and recorded decisions.
  Missing/stale/unbound request context disables decisions in this employment
  review screen. No binding value is editable or derived from user input.
- `GET /…/evidence/{upload}/download`: internally authenticated fetch, no cookies
  or redirect following. Validate the private octet-stream response, byte count
  and SHA-256 against the listing before handoff. Backend also checks canonical
  storage bytes under authorization locks. PDF/PNG/JPEG are supported; browser
  downloads use a temporary object URL, revoked after handoff. No token is put in
  a URL or file. No evidence is placed in browser persistent storage.
- `POST /…/evidence/{upload}/review`: exact server `bindingId`/`claimDigest`, actual
  decision and trimmed rationale (maximum 1,000 UTF-8 bytes). Only a matching,
  well-formed, attributed server receipt is displayed as successful. Duplicate
  taps are synchronously blocked. An uncertain write remains blocked in the
  current review session, including after refresh; operator reconciliation is
  required if the refreshed record remains pending. No automatic mutation retry.
- Backend case-grant, active-principal, no-self-review, scan, context and immutable
  final-decision controls remain authoritative. UI discovery is not authorization.
  The app cannot establish human independence just from different principal IDs.
- Sessions/principal changes discard the sensitive review UI. Stale async reads
  cannot replace another selected scope. API errors are sanitized; no raw bodies,
  rationale, evidence, credentials or capability URLs are logged by this feature.

## Owner/operator prerequisites

Tara must confirm her own account and consent. Resolve its active canonical
principal privately; do not change her role. With separate production approval,
create the actual request, case/check registry and exclusive immutable binding,
then the subject's upload grant and Tara's exact case-review grant using existing
`cvera_evidence_operator` functions. Genuine bytes must be completed into sealed
storage before review. No such resource, grant, upload or review is created here.

## Validation and release preparation

Installed versions: Expo 54, React 19.1, React Native 0.81.5 and NativeWind
5.0.0-preview.2. Existing `expo-crypto`, `expo-file-system/legacy` and React Native
Share are reused. No new native module is required.

From the repository root:

```sh
npm run test:ci
npm run typecheck:expo
git diff --check
```

From `apps/evt-ui-expo`:

```sh
CI=1 ./node_modules/.bin/expo export --platform all --output-dir /private/tmp/cvera-review-ui-export
```

Transport tests cover discovery/empty access, authorization/session/revocation
errors, exact context and existing decisions, private canonical bytes/hash,
missing evidence, both decisions, rationale/context validation, matching receipts,
duplicate prevention, ambiguous writes and temporary web downloads. Export/type
checks establish bundling compatibility, not interactive browser or native-device
acceptance. The in-app browser execution tool was unavailable in this session.

The root `npm run typecheck` is not the Expo check: an isolated unchanged-HEAD
reproduction already fails because the root config includes Expo files without
Expo JSX/alias configuration. Use `typecheck:expo`; do not alter unrelated config.
No separate frontend lint command is configured.

### Deployment (separate authorization required)

The frontend target is **mvp.cvera.app**, served by the existing **Vercel expo-app-mvp**
Git integration, not the DigitalOcean API deploy script or Spaces trust publisher.
Read-only GitHub metadata confirmed successful Vercel Production deployment
7690993571 for `9f300543a36c7cbdc5605a5c1c58b29167e29cfe`, with a Vercel `expo-app`
deployment URL; PR #144's successful Vercel check identifies project `expo-app-mvp`.
This is evidence of the existing release mechanism, not production deployment
of this feature or proof of its custom-domain alias. Actual Vercel project build
settings/domain mapping are not versioned here and must be confirmed before release.

1. Review the focused frontend diff and CI, then exercise the preview with
   explicitly approved test data. Confirm responsive layout, keyboard access,
   PDF/image handoff, decision confirmation and receipt display. Preview-origin
   authentication may not be allowed by the existing production configuration;
   do not broaden CORS or redirect allowlists to force a preview test.
2. Separately authorize integration/release. Existing main pushes also trigger
   API deployment and trust publishing (`.github/workflows/main.yml`, `publish.yml`);
   merging is therefore **not** a production-neutral documentation action.
3. Use the existing Vercel Git integration for the exact reviewed release SHA,
   preserving the existing project root `apps/evt-ui-expo`, Expo web-export build
   and `dist` output. Confirm these settings rather than creating a new project.
   Retain the previous Vercel deployment for rollback. No DB change is needed.
4. Preserve existing public build variables, including
   `EXPO_PUBLIC_EVT_API_BASE_URL=https://api.cvera.app` and
   `EXPO_PUBLIC_KRATOS_BASE_URL=https://auth.cvera.app`; the new client also reuses
   the existing API config fallback. Never add secrets to public Expo variables.
5. Match the GitHub/Vercel deployment SHA and domain mapping, then confirm the
   served bundle contains `Evidence reviews`, `Confirm and record decision` and
   `Show request identifiers and integrity details`. A matching API `/version`
   alone does not prove frontend delivery.
6. Before any genuine decision, test normal reviewer login, empty/authorized case
   discovery, claim context and document download with approved access. Do not
   press the final decision confirmation during a read-only delivery check.
   Positive synthetic decision testing needs its own bounded fixture authorization;
   Tara's genuine decision remains a separate Stage 3 operation.

No merge, deployment, production connection or production mutation is performed
by this implementation task. Browser/device acceptance and the final frontend
project-setting confirmation remain release validation, not demonstrated PASS.
