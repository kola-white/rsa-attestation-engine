# Changelog

All notable changes to this project will be documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
and adheres to [Semantic Versioning](https://semver.org/).

---

## [0.3-dev] – 2025-XX-XX

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
