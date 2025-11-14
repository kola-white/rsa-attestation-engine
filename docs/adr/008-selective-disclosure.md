# ADR — *Selective Disclosure for Attestations*

### ADR-006 (Draft) — Reviewed & Approved by Nicholas

**Status:** Deferred
**Date:** 2025-11-12
**Subject:** Should the MVP implement SD-JWT or Merkle-based selective disclosure?

---

## 1. **Context**

Our attestation engine signs claims as JWS + JWKS and publishes trust artifacts to DigitalOcean Spaces.
The verifier now supports:

* signature validation
* schema verification
* liveness
* revocation
* policy enforcement
* remote trust fetching

We are evaluating whether the MVP should support **Selective Disclosure**, enabling users to reveal only parts of the claim.

Options:

1. **SD-JWT**
2. **Merkle-based selective proof trees**

---

## 2. **Decision**

**We will NOT implement selective disclosure in MVP v0.3.**
It is explicitly deferred to a **future protocol phase** (DP-1).

---

## 3. **Rationale**

### Why not SD-JWT?

* Requires holder-side disclosure bundling (not in our platform)
* Requires verifier changes to handle salt + claim hashing
* Standards still evolving (IETF work in progress)
* Little immediate product value
* Increases issuer/verifier complexity with no functional gain for this MVP

### Why not Merkle-trees?

* Requires hashing strategy across claim schema
* Requires building and verifying Merkle proofs
* Over-engineering for single-claim or small-claim attestations
* No real-world requirement yet

### Why defer?

Your product is currently:

* issuer-centric
* verifier-centric
* backend-centric
* without a wallet
* without privacy requirements

Selective disclosure would slow shipping **without supporting any current workflows**.

---

## 4. **Consequences (Accepted)**

* Verification stays simple (JWS → JWKS → policy).
* Code is stable, easier to reason about.
* Faster path to pilot/demo.
* Future protocols (wallets, SD-JWT, Merkle proofs) remain compatible with current design.

---

## 5. **Future triggers (When we would revisit this)**

Selective Disclosure becomes a priority IF:

* We build a **holder wallet**
* Customers request **privacy-preserving attestations**
* We issue composite claims (10+ fields)
* External verifiers need “just one claim” without other data
* Regulatory environments require “least privilege claim exposure”

---
