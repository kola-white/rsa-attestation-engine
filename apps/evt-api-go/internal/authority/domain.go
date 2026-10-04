package authority

import "time"

// AuthorityGrant represents Cvera's machine-readable evidence that an
// identified issuer is entitled to assert a defined claim type on behalf of
// a defined authority subject.
//
// An AuthorityGrant is not a credential, signing key, verifier policy, or
// final verification decision.
type AuthorityGrant struct {
	GrantID string

	IssuerID           string
	ClaimType          string
	AuthoritySubjectID string

	ValidFrom  time.Time
	ValidUntil *time.Time

	State AuthorityGrantState
}

// AuthorityGrantState describes whether an authority grant is currently
// usable as affirmative authority evidence.
type AuthorityGrantState string

const (
	AuthorityGrantActive   AuthorityGrantState = "active"
	AuthorityGrantInactive AuthorityGrantState = "inactive"
)

// AuthorityDecision is the normalized result of Cvera issuer-authority
// evaluation.
//
// It is deliberately separate from credential evidence and from final
// verifier-policy outcomes such as VERIFIED or REJECTED.
type AuthorityDecision string

const (
	AuthorityAuthorized   AuthorityDecision = "AUTHORIZED"
	AuthorityUnauthorized AuthorityDecision = "UNAUTHORIZED"
	AuthorityReview       AuthorityDecision = "REVIEW"
)

// AuthorityEvaluation records Cvera's issuer-authority decision and the
// authority grant that established it when applicable.
//
// Reason is a deterministic machine-readable reason code.
type AuthorityEvaluation struct {
	Decision AuthorityDecision
	GrantID  string
	Reason   string
}

const AuthorityReasonResolutionFailed = "AUTHORITY_RESOLUTION_FAILED"
