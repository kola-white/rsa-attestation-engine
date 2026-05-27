# Cvera MJML transactional auth templates

Production-oriented MJML source templates for Cvera authentication emails.

## Included templates

- `verification_code/valid`: account verification email for Ory Kratos verification-code flows.
- `recovery_code/valid`: password recovery code email for Ory Kratos recovery-code flows.
- `security/password_changed`: app/security confirmation template for password-changed notification. This is included as a Cvera-owned notification template; confirm the final sending path before wiring it into production.

## Important production setting

`src/partials/header.mjml` currently uses `{{ .BrandLogoURL }}` as the remote image URL placeholder. Before production, replace that placeholder with a fixed HTTPS PNG URL, for example:

```mjml
<mj-image src="https://assets.cvera.app/email/cvera-logo.png" alt="Cvera" width="42px" align="left" padding="0" />
```

Do not embed SVG, base64 images, or CID attachments in production email bodies.

## Build

```bash
npm install
npm run build
```

Compiled HTML `.gotmpl` files are written to `dist/`.

## Ory Kratos notes

Ory templates use Go template variables. The included Kratos templates use only the flow-specific variables required for code-based flows:

- `{{ .VerificationCode }}`
- `{{ .VerificationURL }}`
- `{{ .RecoveryCode }}`
- `{{ .ExpiresInMinutes }}`

Keep plaintext and HTML templates together. Ory requires both HTML and plaintext body templates when using custom templates.

## Suggested Kratos self-hosted path layout

```text
/conf/courier-template/
  verification_code/valid/
    email.subject.gotmpl
    email.body.gotmpl
    email.body.plaintext.gotmpl
  recovery_code/valid/
    email.subject.gotmpl
    email.body.gotmpl
    email.body.plaintext.gotmpl
```

The password-changed template should be wired only after confirming whether it is sent by Kratos, the Go API, or a future notification worker.

## Compatibility posture

The templates intentionally use conservative MJML structures, table-safe layouts, remote PNG logo loading, strong plaintext fallbacks, and minimal CSS for Gmail, Apple Mail, Outlook, and mobile clients.
