package verification

import (
	"context"
	"time"

	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/authority"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/credential"
)

// ServiceInput contains normalized credential evidence, the already-selected
// verifier policy, and the explicit time at which authority is evaluated.
type ServiceInput struct {
	Evidence    credential.NormalizedCredentialEvidence
	Policy      VerifierPolicy
	EvaluatedAt time.Time
}

// ServiceResult preserves the normalized evidence and the independently
// produced authority and verifier-policy evaluations.
type ServiceResult struct {
	Evidence     credential.NormalizedCredentialEvidence
	Authority    authority.AuthorityEvaluation
	Verification VerificationEvaluation
}

// Service orchestrates authority resolution, authority evaluation, and
// verifier-policy evaluation over already-normalized credential evidence.
//
// Credential parsing/profile dispatch and authority-establishment lifecycle
// mutation are outside this service.
type Service struct {
	authorityResolver authority.AuthorityGrantResolver
}

// Evaluate produces a Cvera verification result from normalized evidence.
//
// Authority resolution failure is indeterminate authority. It MUST NOT be
// interpreted as successful empty resolution or as evidence of unauthorized
// authority.
func (s Service) Evaluate(
	ctx context.Context,
	input ServiceInput,
) ServiceResult {
	grants, err := s.authorityResolver.ResolveAuthorityGrants(
		ctx,
		input.Evidence.Issuer.Identifier,
		input.Evidence.ClaimType,
	)

	var authorityEvaluation authority.AuthorityEvaluation

	if err != nil {
		authorityEvaluation = authority.AuthorityEvaluation{
			Decision: authority.AuthorityReview,
			Reason:   authority.AuthorityReasonResolutionFailed,
		}
	} else {
		authorityEvaluation = authority.Evaluator{}.Evaluate(
			input.Evidence,
			grants,
			input.EvaluatedAt,
		)
	}

	verificationEvaluation := Evaluator{}.Evaluate(EvaluationInput{
		Evidence:  input.Evidence,
		Authority: authorityEvaluation,
		Policy:    input.Policy,
	})

	return ServiceResult{
		Evidence:     input.Evidence,
		Authority:    authorityEvaluation,
		Verification: verificationEvaluation,
	}
}
