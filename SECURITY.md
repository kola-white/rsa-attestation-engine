# Security Policy

## Supported Versions

Security fixes are prioritized for the latest release of Cvera API & Infrastructure V1.

Development branches, historical releases, and demonstration artifacts are not guaranteed security support.

## Reporting a Vulnerability

Please report security vulnerabilities privately using GitHub's
**Report a vulnerability** feature:

https://github.com/kola-white/rsa-attestation-engine/security/advisories/new

Do not disclose exploitable vulnerabilities in public issues or pull requests.

Include:
- A description of the vulnerability and its potential impact.
- Affected components, versions, or commit identifiers.
- Reproduction steps or a proof of concept, where appropriate.
- Suggested mitigations, if known.

Please avoid including real credentials, private keys, personal data, or other sensitive information in reports.

## Security Expectations

- Never commit private keys, credentials, access tokens, or production environment files.
- Use environment variables or externally managed secrets for sensitive configuration.
- Treat example credentials and development artifacts as non-production material.
- Rotate exposed credentials and signing keys immediately.

Security reports will be reviewed as resources permit. No fixed response or remediation timeline is guaranteed.
