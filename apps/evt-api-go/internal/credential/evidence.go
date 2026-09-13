package credential

import "time"

// EvidenceResult is the normalized result vocabulary used by credential
// profiles when reporting what they were able to establish.
//
// These values describe evidence. They are NOT Cvera trust decisions.
//
// In particular:
//
//   EvidenceValid != AUTHORIZED
//   EvidenceValid != VERIFIED
//
// Authorization and final verification outcomes are produced later by the
// Cvera trust engine.
type EvidenceResult string

const (
	EvidenceValid   EvidenceResult = "valid"
	EvidenceInvalid EvidenceResult = "invalid"
	EvidenceUnknown EvidenceResult = "unknown"
)

// NormalizedCredentialEvidence is the format-neutral boundary between
// credential-profile processing and the Cvera trust engine.
//
// CredentialProfile implementations such as legacy-jws and sd-jwt-vc MUST
// normalize their results into this structure rather than leaking
// serialization-specific state into downstream Cvera trust logic.
//
// This structure reports what the credential-processing layer established.
// It MUST NOT contain final issuer-authority or verifier-policy decisions.
type NormalizedCredentialEvidence struct {
	ProfileID    string `json:"profile_id"`
	CredentialID string `json:"credential_id,omitempty"`

	ClaimType string          `json:"claim_type"`
	Claim     EmploymentClaim `json:"claim"`

	Issuer        IssuerEvidence         `json:"issuer"`
	Cryptographic CryptographicEvidence  `json:"cryptographic"`
	Validity      ValidityEvidence       `json:"validity"`
	Status        StatusEvidence         `json:"status"`
	Subject       SubjectBindingEvidence `json:"subject"`
	Disclosure    DisclosureEvidence     `json:"disclosure"`

	Errors []EvidenceError `json:"errors,omitempty"`
}

// IssuerEvidence contains the issuer identifier established from the
// credential together with the result of verification-key resolution.
//
// IMPORTANT:
//
// A successfully resolved issuer identifier does not mean that Cvera has
// determined the issuer to be authoritative for the claim.
//
// Key resolution establishes the cryptographic binding between the
// credential signature and the credential issuer identifier.
//
// Cvera issuer-authority evaluation is a separate downstream operation.
type IssuerEvidence struct {
	Identifier    string                `json:"identifier"`
	KeyResolution KeyResolutionEvidence `json:"key_resolution"`
}

// KeyResolutionEvidence describes the result of resolving and validating
// verification-key material for the credential issuer.
//
// Method is profile-specific but represented using a normalized string.
// Expected examples include:
//
//   - jwt-vc-issuer-metadata
//   - x5c
//   - legacy-jwks
//
// The method does not determine issuer authority.
type KeyResolutionEvidence struct {
	Result EvidenceResult `json:"result"`
	Method string         `json:"method,omitempty"`
	KeyID  string         `json:"key_id,omitempty"`
}

// CryptographicEvidence reports whether the credential's cryptographic
// protection was successfully validated.
//
// Algorithm identifies the cryptographic algorithm actually observed or
// validated by the profile implementation.
//
// A valid signature proves neither issuer authority nor satisfaction of
// verifier policy.
type CryptographicEvidence struct {
	Signature EvidenceResult `json:"signature"`
	Algorithm string         `json:"algorithm,omitempty"`
}

// ValidityEvidence reports the credential's temporal evidence.
//
// Result expresses whether the credential is valid with respect to the
// temporal rules enforced by the credential profile.
//
// Times are pointers because a profile or legacy artifact may not provide
// every value.
type ValidityEvidence struct {
	Result EvidenceResult `json:"result"`

	IssuedAt  *time.Time `json:"issued_at,omitempty"`
	NotBefore *time.Time `json:"not_before,omitempty"`
	NotAfter  *time.Time `json:"not_after,omitempty"`
}

// StatusEvidence reports credential-status evidence produced by the
// credential profile.
//
// Method and Reference deliberately remain format-neutral at this boundary.
// A later standardized status profile may populate these with the selected
// standards mechanism.
//
// The Cvera trust engine decides whether a valid, invalid, or unknown status
// result is acceptable for a particular verifier policy.
type StatusEvidence struct {
	Result    EvidenceResult `json:"result"`
	Method    string         `json:"method,omitempty"`
	Reference string         `json:"reference,omitempty"`
}

// SubjectBindingEvidence reports what the credential profile established
// about the binding between the credential and its subject/holder.
//
// The precise mechanism is intentionally not frozen here. Future profile
// implementations may use identifier binding, key binding, holder binding,
// or another standards-defined mechanism.
//
// Result remains evidence only; downstream Cvera policy determines whether
// the binding is sufficient for a particular trust decision.
type SubjectBindingEvidence struct {
	Result    EvidenceResult `json:"result"`
	Method    string         `json:"method,omitempty"`
	SubjectID string         `json:"subject_id,omitempty"`
}

// DisclosureEvidence records the claim paths or attributes actually
// disclosed by the credential/presentation.
//
// The structure deliberately does not assume that the verifier can know the
// names of undisclosed SD-JWT claims.
type DisclosureEvidence struct {
	DisclosedClaims []string `json:"disclosed_claims,omitempty"`
}

// EvidenceError is a normalized diagnostic emitted by a credential-profile
// implementation.
//
// Code is intended for deterministic programmatic handling.
// Message is diagnostic and MUST NOT be treated as a stable machine contract.
//
// Field may identify the relevant normalized field or claim path when useful.
type EvidenceError struct {
	Code    string `json:"code"`
	Message string `json:"message,omitempty"`
	Field   string `json:"field,omitempty"`
}