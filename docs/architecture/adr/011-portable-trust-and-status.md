# ADR-011 — Portable authenticated authority and OAuth status lists

Status: Accepted for Stage 4D local implementation, genuine production activation pending.
Date: 2026-11-27.

## Decision

Keep the SD-JWT VC draft-18 profile. Use its referenced OAuth Token Status List
**draft-21 JWT** mechanism, with explicit signed revocation-purpose context.
Authenticate scoped public authority projections and public issuer/status JWKs
with existing JOSE algorithms against an explicit publisher anchor. Do not
introduce a new CA hierarchy, a per-credential lookup API or a competing W3C
status mechanism.

Reuse the existing versioned artifact/latest-pointer publication pattern through
an additive local publisher and explicit bulk/offline consumers. Keep legacy
production publishing untouched. Preserve current authority reliance, and require
an explicit policy choice to rely on issuance-time evidence. Report both times.

## Consequences

Independent Go and JS consumers can evaluate trust and status offline within
freshness bounds, and detect known rollback with retained trusted state. Public
artifacts omit private source evidence and credential identities. Strongly bounded
schemas, configurable freshness and version floors fail closed. Stateless first
contact cannot prove no newer artifact exists. Production use requires authorized
source projections, status allocation, signing custody and durable verifier state.

See the [implemented contract, privacy/threat model and normative traceability](../stage4d-portable-trust-status.md).
