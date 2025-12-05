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