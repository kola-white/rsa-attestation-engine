# Combined CP/CPS v1 (MVP)

## 1. Introduction
Purpose, applicability, roles (Root CA, Issuing CA, Issuer, Verifier).

## 2. Identity & Authentication
Admin authentication (WebAuthn, client cert), change control, approvals.

## 3. Key Management
Generation (FIPS libs), storage (HSM/KMS prod), backup, recovery, destruction.

## 4. Certificate/Key Usage
Root: certSign only. Issuing CA: keyCertSign + digitalSignature. Attestation keys: JWS signing.

## 5. Lifecycle
Key rotation schedule, deprecation, grace windows, emergency rotation.

## 6. Publication & Repository
`/.well-known/jwks.json`, `statuslist.json`, schemas, policy URIs, ETags, cache TTLs.

## 7. Revocation
Reasons, timelines, evidence, JSON Status List format, verifier obligations.

## 8. Security Controls
Physical/logical controls, admin MFA, least privilege, logging, retention.

## 9. Audit & Compliance
Signed audit logs, retention, periodic review cadence.

## 10. Incident Response
Compromise indicators, takedown steps, issuer notifications, reissuance.

## 11. Legal
IPR notice, license, warranties/limitations (MVP).

## 12. Versioning & Change Control
Semantic versioning, deprecation policy, migration expectations.
