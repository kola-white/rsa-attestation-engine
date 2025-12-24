# Changelog

All notable changes to this project will be documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
and adheres to [Semantic Versioning](https://semver.org/).

## [0.1.0-evt] – 2025-12-05

### Added
- Scoped MVP to factual employment verification (employer, subject, title, dates, signature).
- Introduced a phase-based project structure:
  - `specs/PH-1-EVT` for Employment Verification Token (EVT) specifications.
  - `schemas/PH-1-EVT` for EVT JSON schemas.
  - Placeholder directories for future phases: job descriptions (PH-2-JD), résumé tokens (PH-3-RESUME), student portfolios (PH-4-STUDENT), and admissions (PH-5-ADMISSIONS).
- Added the canonical EVT schema:
  - `schemas/PH-1-EVT/employment.verification.v1.json` describing a signed employment verification payload (issuer, subject, employment, validity, signature).
- Restructured repo around Employment Verification Token (EVT) MVP.

---

### Changed
- Clarified that the current MVP focus is **PH-1 EVT** (Employment Verification Tokens) rather than a generic “all human claims” attestation engine.
- Reorganized protocol and PKI documents under `specs/PH-1-EVT/` so they clearly belong to Phase 1.

---

## [0.2.0-evt] – 2025-12-22

### Added
- Introduced **Evidence Integrity Representation** as a normative specification section.
- Standardized evidence integrity hashes to **canonical lowercase hex SHA-256** across:
  - Evidence upload completion
  - Evidence listing APIs
  - Storage and audit records
- Defined explicit **hex ↔ base64url conversion rules** at trust boundaries.
- Added a **normative Evidence vs Claims Boundary**, specifying that:
  - Evidence hashes and metadata are never embedded in issued human-claims tokens.
- Added **inline validation tables** per endpoint covering:
  - Evidence upload initialization
  - Evidence upload completion
  - Evidence listing
  - Token issuance
  - Token revocation
  - Case deletion lifecycle
- Clarified lifecycle sequencing for evidence integrity:
  - Upload → hash → submit → verify → retain → delete.

### Changed
- Updated all relevant endpoint examples to explicitly reference
  **Evidence Integrity Representation**.
- Hardened API contract language to use enforceable, RFC-style normative terms
  (MUST, MUST NOT, SHOULD).
- Clarified trust and responsibility boundaries between:
  - Evidence ingestion and retention
  - Claim derivation and token issuance
  - JOSE-encoded objects and application-level APIs.

### Notes
- This release introduces **no breaking changes**.
- Existing clients already submitting SHA-256 hashes remain compatible,
  provided they conform to the canonical lowercase hex format.
- JOSE encoding requirements remain confined to JOSE objects only.

---

## [0.3-dev] – 2025-12-05

### Added
- Introduced directory structure for:
  - `/cmd/issuer` and `/cmd/verifier`
  - `/trust/` (root-ca, issuing-ca, trust directory)
  - `/schemas/`, `/examples/`, `/scripts/`
- Added TP-1 Trust Path Specification draft under `/trust/`.
- Added canonical JSON examples for:
  - Attestation
  - Verification Receipt
  - Revocation Event
- Added three Golden Test Vectors:
  - `golden-valid.attestation.json`
  - `golden-expired.attestation.json`
  - `golden-revoked.attestation.json`
- Added repo-ready README with integrated diagrams and trust architecture.

### Changed
- Updated README to reflect new Trust Path, layered architecture, and JSON schema alignment.

### Notes
- JWKS and Status List will be added in an upcoming commit once dev keys are generated.
- Additional golden vectors (invalid signatures, invalid schema, not-yet-valid) will be added in the same future batch.

---

## [0.2] – Initial Commit
- Initial attestation MVP structure
- Early schemas, draft JSON formats, baseline documentation

## [0.3.0-pre-wedge] – 2025-12-05

- General attestation experimental phase (roles/skills/outcomes, DP-1, etc.)
- Repo structure and specs for a generic human-claims attestation protocol.
- This state is tagged as `v0.3.0-pre-wedge`.

---