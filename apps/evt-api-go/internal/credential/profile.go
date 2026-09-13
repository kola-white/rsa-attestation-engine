package credential

import (
	"context"
	"time"
)

// CredentialProfile defines the boundary between Cvera's canonical domain
// model and a concrete credential representation.
//
// Initial implementations are expected to include:
//
//   - legacy-jws
//   - sd-jwt-vc
//
// A CredentialProfile is responsible for:
//
//   - encoding a Cvera domain claim into its credential representation;
//   - validating representation/profile requirements;
//   - resolving and validating issuer verification-key material;
//   - performing cryptographic verification;
//   - extracting normalized domain claims;
//   - reporting temporal, status, disclosure, and subject-binding evidence;
//   - normalizing all of the above into NormalizedCredentialEvidence.
//
// A CredentialProfile MUST NOT:
//
//   - determine whether an issuer is authoritative for a claim type;
//   - evaluate Cvera issuer-authority policy;
//   - apply verifier reliance/trust policy;
//   - execute DP-1 disambiguation;
//   - return AUTHORIZED / UNAUTHORIZED / REVIEW;
//   - return VERIFIED / REJECTED as a Cvera trust decision.
//
// Those responsibilities belong above this interface in the Cvera trust
// engine.
type CredentialProfile interface {
	// ProfileID returns the stable Cvera identifier for the concrete
	// credential profile implementation.
	//
	// Expected examples:
	//
	//   legacy-jws
	//   sd-jwt-vc
	ProfileID() string

	// Issue converts a canonical Cvera domain claim into the concrete
	// credential representation implemented by this profile.
	Issue(
		ctx context.Context,
		input CredentialIssuanceInput,
	) (IssuedCredential, error)

	// VerifyAndNormalize validates the concrete credential representation
	// and converts all established evidence into Cvera's format-neutral
	// NormalizedCredentialEvidence contract.
	//
	// Successful execution of this method does NOT mean that Cvera has
	// authorized the issuer or produced a VERIFIED trust outcome.
	VerifyAndNormalize(
		ctx context.Context,
		credential []byte,
		opts VerificationOptions,
	) (NormalizedCredentialEvidence, error)
}

// CredentialIssuanceInput contains the profile-independent inputs required
// to issue a portable credential.
//
// It deliberately excludes request/workflow state such as request_id,
// dispatch status, recruiter state, employer-response workflow state,
// trust_result, consumption state, and billing state.
type CredentialIssuanceInput struct {
	Claim   EmploymentClaim `json:"claim"`
	Issuer  IssuerReference `json:"issuer"`
	Subject SubjectBindingInput `json:"subject"`

	IssuedAt time.Time `json:"issued_at"`

	Status *CredentialStatusReference `json:"status,omitempty"`
}

// IssuerReference identifies the issuer that will cryptographically issue
// the credential.
//
// Identifier is expected to become the credential issuer identifier for the
// selected profile.
//
// For the primary SD-JWT VC profile, the architecture currently expects an
// HTTPS issuer identifier. This type intentionally does not hard-code that
// requirement so that alternate profiles such as legacy-jws or HAIP/x5c can
// implement their own validation rules.
type IssuerReference struct {
	Identifier string `json:"identifier"`
}

// SubjectBindingInput provides profile-independent subject information to
// the credential issuer.
//
// Method and BindingValue intentionally remain abstract until Cvera freezes
// the concrete SD-JWT VC subject/holder-binding mechanism.
//
// This input does not determine what must be disclosed in a presentation.
type SubjectBindingInput struct {
	SubjectID    string `json:"subject_id"`
	Method       string `json:"method,omitempty"`
	BindingValue string `json:"binding_value,omitempty"`
}

// CredentialStatusReference is the profile-independent reference supplied
// to a credential profile when issuance requires credential-status
// information.
//
// The exact standardized status mechanism is intentionally not frozen in
// this contract.
type CredentialStatusReference struct {
	Method    string `json:"method"`
	Reference string `json:"reference"`
}

// IssuedCredential is the opaque portable credential artifact produced by a
// CredentialProfile.
//
// Bytes contains the concrete serialized credential. Callers outside the
// credential-profile layer should not depend on its internal serialization.
//
// CredentialID may be empty when a profile does not expose a suitable stable
// identifier.
type IssuedCredential struct {
	ProfileID    string `json:"profile_id"`
	MediaType    string `json:"media_type"`
	CredentialID string `json:"credential_id,omitempty"`

	Bytes []byte `json:"bytes"`
}

// VerificationOptions contains profile-independent controls supplied during
// credential verification.
//
// The first contract intentionally keeps this minimal. Additional options
// should only be added when an implementation requirement is demonstrated,
// rather than anticipating OpenID4VP, wallet, presentation, or policy
// behavior prematurely.
type VerificationOptions struct {
	Now time.Time
}