package verification

import (
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/authority"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/credential"
)

// VerificationDecision is the canonical Cvera decision produced after
// verifier-policy evaluation.
type VerificationDecision string

const (
	VerificationVerified VerificationDecision = "VERIFIED"
	VerificationReview   VerificationDecision = "REVIEW"
	VerificationRejected VerificationDecision = "REJECTED"
)

// VerificationReason identifies a non-positive input to the verifier-policy
// decision. Reasons are emitted in deterministic dimension order.
type VerificationReason string

const (
	ReasonSignatureInvalid           VerificationReason = "SIGNATURE_INVALID"
	ReasonSignatureUnknown           VerificationReason = "SIGNATURE_UNKNOWN"
	ReasonSignatureUnrecognized      VerificationReason = "SIGNATURE_UNRECOGNIZED"
	ReasonValidityInvalid            VerificationReason = "VALIDITY_INVALID"
	ReasonValidityUnknown            VerificationReason = "VALIDITY_UNKNOWN"
	ReasonValidityUnrecognized       VerificationReason = "VALIDITY_UNRECOGNIZED"
	ReasonStatusInvalid              VerificationReason = "STATUS_INVALID"
	ReasonStatusUnknown              VerificationReason = "STATUS_UNKNOWN"
	ReasonStatusUnrecognized         VerificationReason = "STATUS_UNRECOGNIZED"
	ReasonSubjectBindingInvalid      VerificationReason = "SUBJECT_BINDING_INVALID"
	ReasonSubjectBindingUnknown      VerificationReason = "SUBJECT_BINDING_UNKNOWN"
	ReasonSubjectBindingUnrecognized VerificationReason = "SUBJECT_BINDING_UNRECOGNIZED"
	ReasonIssuerUnauthorized         VerificationReason = "ISSUER_UNAUTHORIZED"
	ReasonAuthorityReview            VerificationReason = "AUTHORITY_REVIEW"
	ReasonAuthorityUnrecognized      VerificationReason = "AUTHORITY_UNRECOGNIZED"
)

// VerifierPolicy identifies the verifier policy under which the evidence is
// evaluated. The first policy's reliance semantics are intentionally owned by
// Evaluator rather than exposed as configurable requirement flags.
type VerifierPolicy struct {
	PolicyID string
}

// EvaluationInput contains normalized credential evidence, the independently
// evaluated issuer-authority result, and the verifier policy being applied.
type EvaluationInput struct {
	Evidence  credential.NormalizedCredentialEvidence
	Authority authority.AuthorityEvaluation
	Policy    VerifierPolicy
}

// VerificationEvaluation is the canonical verifier-policy evaluation result.
type VerificationEvaluation struct {
	Decision VerificationDecision
	PolicyID string
	Reasons  []VerificationReason
}

// Evaluator applies the v1 authoritative-employment verifier-policy semantics.
//
// VERIFIED requires positive evidence for every mandatory dimension.
// Any known negative fact produces REJECTED.
// Otherwise any unknown or unrecognized value produces REVIEW.
type Evaluator struct{}

// Evaluate applies deterministic verifier-policy decision semantics.
//
// Reason order is fixed:
//  1. signature
//  2. validity
//  3. status
//  4. subject binding
//  5. issuer authority
//
// Evidence.Errors is intentionally not interpreted here. Credential-profile
// diagnostics do not constitute an independent verifier-policy dimension.
func (Evaluator) Evaluate(input EvaluationInput) VerificationEvaluation {
	reasons := make([]VerificationReason, 0, 5)
	hasNegative := false
	hasIndeterminate := false

	classifyEvidence(
		input.Evidence.Cryptographic.Signature,
		ReasonSignatureInvalid,
		ReasonSignatureUnknown,
		ReasonSignatureUnrecognized,
		&reasons,
		&hasNegative,
		&hasIndeterminate,
	)

	classifyEvidence(
		input.Evidence.Validity.Result,
		ReasonValidityInvalid,
		ReasonValidityUnknown,
		ReasonValidityUnrecognized,
		&reasons,
		&hasNegative,
		&hasIndeterminate,
	)

	classifyEvidence(
		input.Evidence.Status.Result,
		ReasonStatusInvalid,
		ReasonStatusUnknown,
		ReasonStatusUnrecognized,
		&reasons,
		&hasNegative,
		&hasIndeterminate,
	)

	classifyEvidence(
		input.Evidence.Subject.Result,
		ReasonSubjectBindingInvalid,
		ReasonSubjectBindingUnknown,
		ReasonSubjectBindingUnrecognized,
		&reasons,
		&hasNegative,
		&hasIndeterminate,
	)

	switch input.Authority.Decision {
	case authority.AuthorityAuthorized:
		// Positive authority evidence contributes no reason.
	case authority.AuthorityUnauthorized:
		reasons = append(reasons, ReasonIssuerUnauthorized)
		hasNegative = true
	case authority.AuthorityReview:
		reasons = append(reasons, ReasonAuthorityReview)
		hasIndeterminate = true
	default:
		reasons = append(reasons, ReasonAuthorityUnrecognized)
		hasIndeterminate = true
	}

	decision := VerificationVerified
	if hasNegative {
		decision = VerificationRejected
	} else if hasIndeterminate {
		decision = VerificationReview
	}

	return VerificationEvaluation{
		Decision: decision,
		PolicyID: input.Policy.PolicyID,
		Reasons:  reasons,
	}
}

func classifyEvidence(
	result credential.EvidenceResult,
	invalidReason VerificationReason,
	unknownReason VerificationReason,
	unrecognizedReason VerificationReason,
	reasons *[]VerificationReason,
	hasNegative *bool,
	hasIndeterminate *bool,
) {
	switch result {
	case credential.EvidenceValid:
		// Positive evidence contributes no reason.
	case credential.EvidenceInvalid:
		*reasons = append(*reasons, invalidReason)
		*hasNegative = true
	case credential.EvidenceUnknown:
		*reasons = append(*reasons, unknownReason)
		*hasIndeterminate = true
	default:
		*reasons = append(*reasons, unrecognizedReason)
		*hasIndeterminate = true
	}
}
