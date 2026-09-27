package authority

import "context"

// AuthorityGrantResolver retrieves Cvera authority grants applicable to an
// identified issuer and claim type.
//
// Resolution is deliberately separate from authority evaluation. In
// particular, the resolver does not determine whether a returned grant's
// AuthoritySubjectID matches the authority subject represented in normalized
// credential evidence.
//
// An empty successful result means that no applicable authority grants were
// found. A non-nil error means authority evidence could not be resolved and
// MUST NOT be interpreted as an empty successful result.
type AuthorityGrantResolver interface {
	ResolveAuthorityGrants(
		ctx context.Context,
		issuerID string,
		claimType string,
	) ([]AuthorityGrant, error)
}
