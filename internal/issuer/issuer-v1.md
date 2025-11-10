# Issuer Policy v1

URI: `policies/issuer-v1`

- Authorization: WebAuthn / certs for /issue, /revoke, /rotate
- Key storage: dev=software OK; prod=HSM/KMS
- Algorithms: RS256 (MVP)
- Validity windows required
- Revocation pointer + serial required
- Audit logs retained ≥365 days
