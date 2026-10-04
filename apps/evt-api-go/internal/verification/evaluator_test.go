package verification

import (
	"testing"

	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/authority"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/credential"
)

const authoritativeEmploymentPolicyID = "employment.authoritative/v1"

func evaluationInput(
	signature credential.EvidenceResult,
	validity credential.EvidenceResult,
	status credential.EvidenceResult,
	subject credential.EvidenceResult,
	authorityDecision authority.AuthorityDecision,
) EvaluationInput {
	return EvaluationInput{
		Evidence: credential.NormalizedCredentialEvidence{
			Cryptographic: credential.CryptographicEvidence{
				Signature: signature,
			},
			Validity: credential.ValidityEvidence{
				Result: validity,
			},
			Status: credential.StatusEvidence{
				Result: status,
			},
			Subject: credential.SubjectBindingEvidence{
				Result: subject,
			},
		},
		Authority: authority.AuthorityEvaluation{
			Decision: authorityDecision,
		},
		Policy: VerifierPolicy{
			PolicyID: authoritativeEmploymentPolicyID,
		},
	}
}

func TestEvaluatorCanonicalCases(t *testing.T) {
	tests := []struct {
		name      string
		signature credential.EvidenceResult
		validity  credential.EvidenceResult
		status    credential.EvidenceResult
		subject   credential.EvidenceResult
		authority authority.AuthorityDecision
		want      VerificationDecision
		reasons   []VerificationReason
	}{
		{
			name:      "all mandatory evidence positive",
			signature: credential.EvidenceValid,
			validity:  credential.EvidenceValid,
			status:    credential.EvidenceValid,
			subject:   credential.EvidenceValid,
			authority: authority.AuthorityAuthorized,
			want:      VerificationVerified,
		},
		{
			name:      "invalid signature",
			signature: credential.EvidenceInvalid,
			validity:  credential.EvidenceValid,
			status:    credential.EvidenceValid,
			subject:   credential.EvidenceValid,
			authority: authority.AuthorityAuthorized,
			want:      VerificationRejected,
			reasons:   []VerificationReason{ReasonSignatureInvalid},
		},
		{
			name:      "unknown signature",
			signature: credential.EvidenceUnknown,
			validity:  credential.EvidenceValid,
			status:    credential.EvidenceValid,
			subject:   credential.EvidenceValid,
			authority: authority.AuthorityAuthorized,
			want:      VerificationReview,
			reasons:   []VerificationReason{ReasonSignatureUnknown},
		},
		{
			name:      "invalid validity",
			signature: credential.EvidenceValid,
			validity:  credential.EvidenceInvalid,
			status:    credential.EvidenceValid,
			subject:   credential.EvidenceValid,
			authority: authority.AuthorityAuthorized,
			want:      VerificationRejected,
			reasons:   []VerificationReason{ReasonValidityInvalid},
		},
		{
			name:      "unknown validity",
			signature: credential.EvidenceValid,
			validity:  credential.EvidenceUnknown,
			status:    credential.EvidenceValid,
			subject:   credential.EvidenceValid,
			authority: authority.AuthorityAuthorized,
			want:      VerificationReview,
			reasons:   []VerificationReason{ReasonValidityUnknown},
		},
		{
			name:      "invalid status",
			signature: credential.EvidenceValid,
			validity:  credential.EvidenceValid,
			status:    credential.EvidenceInvalid,
			subject:   credential.EvidenceValid,
			authority: authority.AuthorityAuthorized,
			want:      VerificationRejected,
			reasons:   []VerificationReason{ReasonStatusInvalid},
		},
		{
			name:      "unknown status",
			signature: credential.EvidenceValid,
			validity:  credential.EvidenceValid,
			status:    credential.EvidenceUnknown,
			subject:   credential.EvidenceValid,
			authority: authority.AuthorityAuthorized,
			want:      VerificationReview,
			reasons:   []VerificationReason{ReasonStatusUnknown},
		},
		{
			name:      "invalid subject binding",
			signature: credential.EvidenceValid,
			validity:  credential.EvidenceValid,
			status:    credential.EvidenceValid,
			subject:   credential.EvidenceInvalid,
			authority: authority.AuthorityAuthorized,
			want:      VerificationRejected,
			reasons:   []VerificationReason{ReasonSubjectBindingInvalid},
		},
		{
			name:      "unknown subject binding",
			signature: credential.EvidenceValid,
			validity:  credential.EvidenceValid,
			status:    credential.EvidenceValid,
			subject:   credential.EvidenceUnknown,
			authority: authority.AuthorityAuthorized,
			want:      VerificationReview,
			reasons:   []VerificationReason{ReasonSubjectBindingUnknown},
		},
		{
			name:      "issuer unauthorized",
			signature: credential.EvidenceValid,
			validity:  credential.EvidenceValid,
			status:    credential.EvidenceValid,
			subject:   credential.EvidenceValid,
			authority: authority.AuthorityUnauthorized,
			want:      VerificationRejected,
			reasons:   []VerificationReason{ReasonIssuerUnauthorized},
		},
		{
			name:      "authority review",
			signature: credential.EvidenceValid,
			validity:  credential.EvidenceValid,
			status:    credential.EvidenceValid,
			subject:   credential.EvidenceValid,
			authority: authority.AuthorityReview,
			want:      VerificationReview,
			reasons:   []VerificationReason{ReasonAuthorityReview},
		},
		{
			name:      "negative outranks indeterminate",
			signature: credential.EvidenceInvalid,
			validity:  credential.EvidenceUnknown,
			status:    credential.EvidenceUnknown,
			subject:   credential.EvidenceUnknown,
			authority: authority.AuthorityReview,
			want:      VerificationRejected,
			reasons: []VerificationReason{
				ReasonSignatureInvalid,
				ReasonValidityUnknown,
				ReasonStatusUnknown,
				ReasonSubjectBindingUnknown,
				ReasonAuthorityReview,
			},
		},
		{
			name:      "all indeterminate",
			signature: credential.EvidenceUnknown,
			validity:  credential.EvidenceUnknown,
			status:    credential.EvidenceUnknown,
			subject:   credential.EvidenceUnknown,
			authority: authority.AuthorityReview,
			want:      VerificationReview,
			reasons: []VerificationReason{
				ReasonSignatureUnknown,
				ReasonValidityUnknown,
				ReasonStatusUnknown,
				ReasonSubjectBindingUnknown,
				ReasonAuthorityReview,
			},
		},
		{
			name:      "all definitive failures",
			signature: credential.EvidenceInvalid,
			validity:  credential.EvidenceInvalid,
			status:    credential.EvidenceInvalid,
			subject:   credential.EvidenceInvalid,
			authority: authority.AuthorityUnauthorized,
			want:      VerificationRejected,
			reasons: []VerificationReason{
				ReasonSignatureInvalid,
				ReasonValidityInvalid,
				ReasonStatusInvalid,
				ReasonSubjectBindingInvalid,
				ReasonIssuerUnauthorized,
			},
		},
		{
			name:      "current SD-JWT state cannot verify",
			signature: credential.EvidenceValid,
			validity:  credential.EvidenceValid,
			status:    credential.EvidenceUnknown,
			subject:   credential.EvidenceUnknown,
			authority: authority.AuthorityAuthorized,
			want:      VerificationReview,
			reasons: []VerificationReason{
				ReasonStatusUnknown,
				ReasonSubjectBindingUnknown,
			},
		},
	}

	evaluator := Evaluator{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := evaluator.Evaluate(evaluationInput(
				tt.signature,
				tt.validity,
				tt.status,
				tt.subject,
				tt.authority,
			))

			if got.Decision != tt.want {
				t.Fatalf(
					"Decision = %q, want %q",
					got.Decision,
					tt.want,
				)
			}

			if got.PolicyID != authoritativeEmploymentPolicyID {
				t.Fatalf(
					"PolicyID = %q, want %q",
					got.PolicyID,
					authoritativeEmploymentPolicyID,
				)
			}

			assertReasons(t, got.Reasons, tt.reasons)
		})
	}
}

func TestEvaluatorRejectsOutrankUnknownsAcrossEveryDimension(t *testing.T) {
	tests := []struct {
		name      string
		signature credential.EvidenceResult
		validity  credential.EvidenceResult
		status    credential.EvidenceResult
		subject   credential.EvidenceResult
		authority authority.AuthorityDecision
	}{
		{
			name:      "signature negative",
			signature: credential.EvidenceInvalid,
			validity:  credential.EvidenceUnknown,
			status:    credential.EvidenceUnknown,
			subject:   credential.EvidenceUnknown,
			authority: authority.AuthorityReview,
		},
		{
			name:      "validity negative",
			signature: credential.EvidenceUnknown,
			validity:  credential.EvidenceInvalid,
			status:    credential.EvidenceUnknown,
			subject:   credential.EvidenceUnknown,
			authority: authority.AuthorityReview,
		},
		{
			name:      "status negative",
			signature: credential.EvidenceUnknown,
			validity:  credential.EvidenceUnknown,
			status:    credential.EvidenceInvalid,
			subject:   credential.EvidenceUnknown,
			authority: authority.AuthorityReview,
		},
		{
			name:      "subject negative",
			signature: credential.EvidenceUnknown,
			validity:  credential.EvidenceUnknown,
			status:    credential.EvidenceUnknown,
			subject:   credential.EvidenceInvalid,
			authority: authority.AuthorityReview,
		},
		{
			name:      "authority negative",
			signature: credential.EvidenceUnknown,
			validity:  credential.EvidenceUnknown,
			status:    credential.EvidenceUnknown,
			subject:   credential.EvidenceUnknown,
			authority: authority.AuthorityUnauthorized,
		},
	}

	evaluator := Evaluator{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := evaluator.Evaluate(evaluationInput(
				tt.signature,
				tt.validity,
				tt.status,
				tt.subject,
				tt.authority,
			))

			if got.Decision != VerificationRejected {
				t.Fatalf(
					"Decision = %q, want %q",
					got.Decision,
					VerificationRejected,
				)
			}
		})
	}
}

func TestEvaluatorUnrecognizedValuesFailClosedToReview(t *testing.T) {
	tests := []struct {
		name      string
		signature credential.EvidenceResult
		validity  credential.EvidenceResult
		status    credential.EvidenceResult
		subject   credential.EvidenceResult
		authority authority.AuthorityDecision
		reason    VerificationReason
	}{
		{
			name:      "unrecognized signature",
			signature: credential.EvidenceResult("unrecognized"),
			validity:  credential.EvidenceValid,
			status:    credential.EvidenceValid,
			subject:   credential.EvidenceValid,
			authority: authority.AuthorityAuthorized,
			reason:    ReasonSignatureUnrecognized,
		},
		{
			name:      "unrecognized validity",
			signature: credential.EvidenceValid,
			validity:  credential.EvidenceResult("unrecognized"),
			status:    credential.EvidenceValid,
			subject:   credential.EvidenceValid,
			authority: authority.AuthorityAuthorized,
			reason:    ReasonValidityUnrecognized,
		},
		{
			name:      "unrecognized status",
			signature: credential.EvidenceValid,
			validity:  credential.EvidenceValid,
			status:    credential.EvidenceResult("unrecognized"),
			subject:   credential.EvidenceValid,
			authority: authority.AuthorityAuthorized,
			reason:    ReasonStatusUnrecognized,
		},
		{
			name:      "unrecognized subject",
			signature: credential.EvidenceValid,
			validity:  credential.EvidenceValid,
			status:    credential.EvidenceValid,
			subject:   credential.EvidenceResult("unrecognized"),
			authority: authority.AuthorityAuthorized,
			reason:    ReasonSubjectBindingUnrecognized,
		},
		{
			name:      "unrecognized authority",
			signature: credential.EvidenceValid,
			validity:  credential.EvidenceValid,
			status:    credential.EvidenceValid,
			subject:   credential.EvidenceValid,
			authority: authority.AuthorityDecision("unrecognized"),
			reason:    ReasonAuthorityUnrecognized,
		},
	}

	evaluator := Evaluator{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := evaluator.Evaluate(evaluationInput(
				tt.signature,
				tt.validity,
				tt.status,
				tt.subject,
				tt.authority,
			))

			if got.Decision != VerificationReview {
				t.Fatalf(
					"Decision = %q, want %q",
					got.Decision,
					VerificationReview,
				)
			}

			assertReasons(
				t,
				got.Reasons,
				[]VerificationReason{tt.reason},
			)
		})
	}
}

func TestEvaluatorCanonicalStateSpace(t *testing.T) {
	evidenceStates := []credential.EvidenceResult{
		credential.EvidenceValid,
		credential.EvidenceInvalid,
		credential.EvidenceUnknown,
	}

	authorityStates := []authority.AuthorityDecision{
		authority.AuthorityAuthorized,
		authority.AuthorityUnauthorized,
		authority.AuthorityReview,
	}

	evaluator := Evaluator{}

	total := 0
	verified := 0

	for _, signature := range evidenceStates {
		for _, validity := range evidenceStates {
			for _, status := range evidenceStates {
				for _, subject := range evidenceStates {
					for _, authorityDecision := range authorityStates {
						total++

						got := evaluator.Evaluate(evaluationInput(
							signature,
							validity,
							status,
							subject,
							authorityDecision,
						))

						want := expectedCanonicalDecision(
							signature,
							validity,
							status,
							subject,
							authorityDecision,
						)

						if got.Decision != want {
							t.Fatalf(
								"(%q,%q,%q,%q,%q): Decision = %q, want %q",
								signature,
								validity,
								status,
								subject,
								authorityDecision,
								got.Decision,
								want,
							)
						}

						if got.Decision == VerificationVerified {
							verified++
						}
					}
				}
			}
		}
	}

	if total != 243 {
		t.Fatalf("evaluated %d combinations, want 243", total)
	}

	if verified != 1 {
		t.Fatalf(
			"VERIFIED combinations = %d, want exactly 1",
			verified,
		)
	}
}

func expectedCanonicalDecision(
	signature credential.EvidenceResult,
	validity credential.EvidenceResult,
	status credential.EvidenceResult,
	subject credential.EvidenceResult,
	authorityDecision authority.AuthorityDecision,
) VerificationDecision {
	if signature == credential.EvidenceInvalid ||
		validity == credential.EvidenceInvalid ||
		status == credential.EvidenceInvalid ||
		subject == credential.EvidenceInvalid ||
		authorityDecision == authority.AuthorityUnauthorized {
		return VerificationRejected
	}

	if signature == credential.EvidenceUnknown ||
		validity == credential.EvidenceUnknown ||
		status == credential.EvidenceUnknown ||
		subject == credential.EvidenceUnknown ||
		authorityDecision == authority.AuthorityReview {
		return VerificationReview
	}

	return VerificationVerified
}

func assertReasons(
	t *testing.T,
	got []VerificationReason,
	want []VerificationReason,
) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf(
			"Reasons length = %d (%v), want %d (%v)",
			len(got),
			got,
			len(want),
			want,
		)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf(
				"Reasons[%d] = %q, want %q; got=%v want=%v",
				i,
				got[i],
				want[i],
				got,
				want,
			)
		}
	}
}

func TestEvaluatorKnownNegativeOutranksUnrecognized(t *testing.T) {
	evaluator := Evaluator{}

	got := evaluator.Evaluate(evaluationInput(
		credential.EvidenceInvalid,
		credential.EvidenceResult("unrecognized"),
		credential.EvidenceValid,
		credential.EvidenceValid,
		authority.AuthorityAuthorized,
	))

	if got.Decision != VerificationRejected {
		t.Fatalf(
			"Decision = %q, want %q",
			got.Decision,
			VerificationRejected,
		)
	}

	assertReasons(
		t,
		got.Reasons,
		[]VerificationReason{
			ReasonSignatureInvalid,
			ReasonValidityUnrecognized,
		},
	)
}
