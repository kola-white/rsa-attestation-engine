package authority

import (
	"time"

	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/credential"
)

// Evaluator determines whether normalized credential evidence is backed by
// sufficient Cvera issuer-authority evidence.
//
// It does not perform credential cryptographic verification and does not
// evaluate verifier reliance/trust policy.
type Evaluator struct{}

// Evaluate determines the issuer-authority state for normalized credential
// evidence using the supplied authority grants.
//
// Absence of applicable authority evidence results in REVIEW rather than
// UNAUTHORIZED. UNAUTHORIZED requires explicit negative authority evidence,
// represented in this slice by a matching inactive grant.
func (Evaluator) Evaluate(
	evidence credential.NormalizedCredentialEvidence,
	grants []AuthorityGrant,
	now time.Time,
) AuthorityEvaluation {
	var currentInactive *AuthorityGrant
	var matchingButNotCurrent bool

	for i := range grants {
		grant := &grants[i]

		if !matchesAuthorityScope(evidence, *grant) {
			continue
		}

		if !grantCurrent(*grant, now) {
			matchingButNotCurrent = true
			continue
		}

		switch grant.State {
		case AuthorityGrantActive:
			return AuthorityEvaluation{
				Decision: AuthorityAuthorized,
				GrantID:  grant.GrantID,
				Reason:   "MATCHING_ACTIVE_GRANT",
			}

		case AuthorityGrantInactive:
			if currentInactive == nil {
				currentInactive = grant
			}
		}
	}

	if currentInactive != nil {
		return AuthorityEvaluation{
			Decision: AuthorityUnauthorized,
			GrantID:  currentInactive.GrantID,
			Reason:   "MATCHING_INACTIVE_GRANT",
		}
	}

	if matchingButNotCurrent {
		return AuthorityEvaluation{
			Decision: AuthorityReview,
			Reason:   "NO_CURRENT_GRANT",
		}
	}

	return AuthorityEvaluation{
		Decision: AuthorityReview,
		Reason:   "NO_APPLICABLE_GRANT",
	}
}

func matchesAuthorityScope(
	evidence credential.NormalizedCredentialEvidence,
	grant AuthorityGrant,
) bool {
	return grant.IssuerID == evidence.Issuer.Identifier &&
		grant.ClaimType == evidence.ClaimType &&
		grant.AuthoritySubjectID == evidence.Claim.Employment.EmployerID
}

func grantCurrent(
	grant AuthorityGrant,
	now time.Time,
) bool {
	if now.Before(grant.ValidFrom) {
		return false
	}

	if grant.ValidUntil != nil && now.After(*grant.ValidUntil) {
		return false
	}

	return true
}
