# ADR-010 — Bounded holder-bound reciprocal presentation

Status: Accepted for Stage 4C local synthetic implementation.
Date: 2026-11-26.

## Decision

Extend the existing pinned SD-JWT profile through an additive local presentation
boundary. Use RFC 9901 KB-JWTs and the OpenID4VP pre-registered client mechanism,
with pinned public keys and response URIs. Keep authentication, disclosure policy,
local holder consent, credential evaluation and holder possession separate.

Require possession before a new synthetic credential receives issuer-protected
`cnf.jwk`. Preserve old production issuance and all Stage 4B semantics. Use a
single-process atomic challenge store for the bounded harness. No novel PKI,
production registry, wallet transport or browser dependency is introduced.

## Consequences

Two independent verifiers can reuse one credential without Cvera services.
Unknown verifiers and unauthorized disclosure are denied. The fully disclosed
profile limits privacy; arbitrary selective disclosure remains unsupported.
Consent cannot be inferred remotely from cryptographic possession. Durable,
distributed replay storage, genuine authority, key custody and production consent
integration require later authorization before production use.

See the [implementation, threat model, normative mapping and reproduction guide](../stage4c-holder-bound-presentation.md).
