# Trust Interpretation Contract (MVP) v1.0.0

**Status:** Final (MVP)  
**Author:** Nicholas Whitehouse  
**Organization:** NorStar Systems LLC  
**Scope:** Defines the authoritative interpretation of trust, verification, and policy semantics for the MVP product experience.

---

# 1. Purpose

This document establishes the **canonical meaning of trust and verification fields** across the MVP system, ensuring:

- consistent interpretation across backend, API, and UI
- correct separation of **trust**, **verification**, and **signature presence**
- prevention of **overstating cryptographic guarantees**
- alignment with the system’s **layered verification architecture**

This document governs **interpretation**, not cryptographic computation.

---

# 2. Relationship to Trust Path Specification (TP-1)

This document **does not replace** the Trust Path Specification.

| Layer | Document | Responsibility |
|------|--------|----------------|
| Future-state | TP-1 Trust Path Specification | Full cryptographic trust-path validation |
| MVP | This document | Current trust interpretation and UI/API semantics |

## Critical Boundary

The MVP:

### DOES:
- project recruiter-specific issuer trust
- expose request-level verification state
- drive UI filtering and gating

### DOES NOT:
- perform JWS cryptographic verification
- resolve `kid` or JWKS
- enforce revocation lists
- enforce validity windows
- validate certificate chains

> The full trust-path architecture and cryptographic validation remain defined by the Trust Path Specification.

---

# 3. Architectural Principle

The system enforces strict separation between:

## Protocol Layer (Immutable)
- claim snapshot
- attestation signature
- (future) cryptographic verification

## Interpretation Layer (MVP)
- recruiter policy trust
- verification usability state
- UI gating decisions

> Cryptographic truth is immutable. Trust and policy are re-evaluable interpretations.

The MVP operates in the **interpretation layer**, not full protocol validation.

---

# 4. Scope and Non-Goals

## 4.1 In Scope

- recruiter issuer trust (`recruiter_trusted_issuers`)
- request lifecycle interpretation
- API trust and verification fields
- recruiter filters
- UI labeling and gating

## 4.2 Out of Scope

- cryptographic signature validation
- key resolution (JWKS, `kid`)
- revocation enforcement
- validity window enforcement
- certificate chain validation
- TP-1 trust-path computation

---

# 5. Canonical Type Definitions

These types are **authoritative**. Their meaning must not be reinterpreted.

```ts
type TrustBadge = "trusted" | "untrusted" | "unknown";

type SignatureBadge = "verified" | "invalid" | "unknown";

type VerificationState = "verified" | "unverified" | "pending" | "unknown";

type RequestStatus =
  | "DRAFT"
  | "SUBMITTED"
  | "ATTESTATION_PENDING"
  | "ATTESTED"
  | "VERIFIED"
  | "UNVERIFIED"
  | "REJECTED"
  | "CONSUMED"
  | "CLOSED";

type TrustResult = "VERIFIED" | "VERIFIED_WITH_FLAGS" | "UNVERIFIED";