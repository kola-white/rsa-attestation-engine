package verification

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/authority"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/credential"
)

type stubAuthorityGrantResolver struct {
	grants []authority.AuthorityGrant
	err    error

	called    bool
	issuerID  string
	claimType string
}

func (r *stubAuthorityGrantResolver) ResolveAuthorityGrants(
	_ context.Context,
	issuerID string,
	claimType string,
) ([]authority.AuthorityGrant, error) {
	r.called = true
	r.issuerID = issuerID
	r.claimType = claimType
	return r.grants, r.err
}

func positiveEvidence() credential.NormalizedCredentialEvidence {
	return credential.NormalizedCredentialEvidence{
		ClaimType: credential.ClaimTypeEmploymentRole,
		Claim: credential.EmploymentClaim{
			ClaimType: credential.ClaimTypeEmploymentRole,
			Employment: credential.EmploymentRelationship{
				EmployerID: "employer-1",
			},
		},
		Issuer: credential.IssuerEvidence{
			Identifier: "issuer-1",
		},
		Cryptographic: credential.CryptographicEvidence{
			Signature: credential.EvidenceValid,
		},
		Validity: credential.ValidityEvidence{
			Result: credential.EvidenceValid,
		},
		Status: credential.StatusEvidence{
			Result: credential.EvidenceValid,
		},
		Subject: credential.SubjectBindingEvidence{
			Result: credential.EvidenceValid,
		},
	}
}

func activeGrant(validFrom time.Time) authority.AuthorityGrant {
	return authority.AuthorityGrant{
		GrantID:            "grant-1",
		IssuerID:           "issuer-1",
		ClaimType:          credential.ClaimTypeEmploymentRole,
		AuthoritySubjectID: "employer-1",
		ValidFrom:          validFrom,
		State:              authority.AuthorityGrantActive,
	}
}

func serviceInput(
	evidence credential.NormalizedCredentialEvidence,
	evaluatedAt time.Time,
) ServiceInput {
	return ServiceInput{
		Evidence: evidence,
		Policy: VerifierPolicy{
			PolicyID: "employment.authoritative/v1",
		},
		EvaluatedAt: evaluatedAt,
	}
}

func TestServiceMatchingActiveGrantProducesVerified(t *testing.T) {
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	resolver := &stubAuthorityGrantResolver{
		grants: []authority.AuthorityGrant{
			activeGrant(now.Add(-time.Hour)),
		},
	}

	service := Service{authorityResolver: resolver}
	input := serviceInput(positiveEvidence(), now)

	got := service.Evaluate(context.Background(), input)

	if got.Authority.Decision != authority.AuthorityAuthorized {
		t.Fatalf("authority decision = %q, want %q",
			got.Authority.Decision, authority.AuthorityAuthorized)
	}
	if got.Authority.Reason != "MATCHING_ACTIVE_GRANT" {
		t.Fatalf("authority reason = %q, want MATCHING_ACTIVE_GRANT",
			got.Authority.Reason)
	}
	if got.Verification.Decision != VerificationVerified {
		t.Fatalf("verification decision = %q, want %q",
			got.Verification.Decision, VerificationVerified)
	}
	if got.Verification.PolicyID != input.Policy.PolicyID {
		t.Fatalf("policy ID = %q, want %q",
			got.Verification.PolicyID, input.Policy.PolicyID)
	}
}

func TestServiceSuccessfulEmptyResolutionProducesReview(t *testing.T) {
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	resolver := &stubAuthorityGrantResolver{
		grants: []authority.AuthorityGrant{},
	}

	service := Service{authorityResolver: resolver}
	got := service.Evaluate(
		context.Background(),
		serviceInput(positiveEvidence(), now),
	)

	if got.Authority.Decision != authority.AuthorityReview {
		t.Fatalf("authority decision = %q, want %q",
			got.Authority.Decision, authority.AuthorityReview)
	}
	if got.Authority.Reason != "NO_APPLICABLE_GRANT" {
		t.Fatalf("authority reason = %q, want NO_APPLICABLE_GRANT",
			got.Authority.Reason)
	}
	if got.Verification.Decision != VerificationReview {
		t.Fatalf("verification decision = %q, want %q",
			got.Verification.Decision, VerificationReview)
	}
}

func TestServiceAuthorityResolutionFailureRemainsIndeterminate(t *testing.T) {
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	resolver := &stubAuthorityGrantResolver{
		err: errors.New("authority store unavailable"),
	}

	service := Service{authorityResolver: resolver}
	got := service.Evaluate(
		context.Background(),
		serviceInput(positiveEvidence(), now),
	)

	if got.Authority.Decision != authority.AuthorityReview {
		t.Fatalf("authority decision = %q, want %q",
			got.Authority.Decision, authority.AuthorityReview)
	}
	if got.Authority.Reason != authority.AuthorityReasonResolutionFailed {
		t.Fatalf("authority reason = %q, want %q",
			got.Authority.Reason, authority.AuthorityReasonResolutionFailed)
	}
	if got.Authority.Reason == "NO_APPLICABLE_GRANT" {
		t.Fatal("resolver failure was collapsed into successful empty resolution")
	}
	if got.Authority.Decision == authority.AuthorityUnauthorized {
		t.Fatal("resolver failure was incorrectly treated as unauthorized")
	}
	if got.Verification.Decision != VerificationReview {
		t.Fatalf("verification decision = %q, want %q",
			got.Verification.Decision, VerificationReview)
	}
}

func TestServiceMatchingInactiveGrantProducesRejected(t *testing.T) {
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	grant := activeGrant(now.Add(-time.Hour))
	grant.State = authority.AuthorityGrantInactive

	resolver := &stubAuthorityGrantResolver{
		grants: []authority.AuthorityGrant{grant},
	}

	service := Service{authorityResolver: resolver}
	got := service.Evaluate(
		context.Background(),
		serviceInput(positiveEvidence(), now),
	)

	if got.Authority.Decision != authority.AuthorityUnauthorized {
		t.Fatalf("authority decision = %q, want %q",
			got.Authority.Decision, authority.AuthorityUnauthorized)
	}
	if got.Authority.Reason != "MATCHING_INACTIVE_GRANT" {
		t.Fatalf("authority reason = %q, want MATCHING_INACTIVE_GRANT",
			got.Authority.Reason)
	}
	if got.Verification.Decision != VerificationRejected {
		t.Fatalf("verification decision = %q, want %q",
			got.Verification.Decision, VerificationRejected)
	}
}

func TestServiceExpiredGrantProducesReview(t *testing.T) {
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	expiredAt := now.Add(-time.Minute)
	grant := activeGrant(now.Add(-2 * time.Hour))
	grant.ValidUntil = &expiredAt

	resolver := &stubAuthorityGrantResolver{
		grants: []authority.AuthorityGrant{grant},
	}

	service := Service{authorityResolver: resolver}
	got := service.Evaluate(
		context.Background(),
		serviceInput(positiveEvidence(), now),
	)

	if got.Authority.Decision != authority.AuthorityReview {
		t.Fatalf("authority decision = %q, want %q",
			got.Authority.Decision, authority.AuthorityReview)
	}
	if got.Authority.Reason != "NO_CURRENT_GRANT" {
		t.Fatalf("authority reason = %q, want NO_CURRENT_GRANT",
			got.Authority.Reason)
	}
	if got.Verification.Decision != VerificationReview {
		t.Fatalf("verification decision = %q, want %q",
			got.Verification.Decision, VerificationReview)
	}
}

func TestServiceAuthorizedCurrentSDJWTShapeStillProducesReview(t *testing.T) {
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	evidence := positiveEvidence()
	evidence.Status.Result = credential.EvidenceUnknown
	evidence.Subject.Result = credential.EvidenceUnknown

	resolver := &stubAuthorityGrantResolver{
		grants: []authority.AuthorityGrant{
			activeGrant(now.Add(-time.Hour)),
		},
	}

	service := Service{authorityResolver: resolver}
	got := service.Evaluate(
		context.Background(),
		serviceInput(evidence, now),
	)

	if got.Authority.Decision != authority.AuthorityAuthorized {
		t.Fatalf("authority decision = %q, want %q",
			got.Authority.Decision, authority.AuthorityAuthorized)
	}
	if got.Verification.Decision != VerificationReview {
		t.Fatalf("verification decision = %q, want %q",
			got.Verification.Decision, VerificationReview)
	}

	wantReasons := []VerificationReason{
		ReasonStatusUnknown,
		ReasonSubjectBindingUnknown,
	}
	if len(got.Verification.Reasons) != len(wantReasons) {
		t.Fatalf("verification reasons = %v, want %v",
			got.Verification.Reasons, wantReasons)
	}
	for i := range wantReasons {
		if got.Verification.Reasons[i] != wantReasons[i] {
			t.Fatalf("verification reasons = %v, want %v",
				got.Verification.Reasons, wantReasons)
		}
	}
}

func TestServiceResolverReceivesNormalizedIssuerAndClaimType(t *testing.T) {
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	evidence := positiveEvidence()
	evidence.Issuer.Identifier = "issuer-normalized"
	evidence.ClaimType = "employment.role.custom"

	resolver := &stubAuthorityGrantResolver{}

	service := Service{authorityResolver: resolver}
	service.Evaluate(
		context.Background(),
		serviceInput(evidence, now),
	)

	if !resolver.called {
		t.Fatal("authority resolver was not called")
	}
	if resolver.issuerID != evidence.Issuer.Identifier {
		t.Fatalf("resolver issuer ID = %q, want %q",
			resolver.issuerID, evidence.Issuer.Identifier)
	}
	if resolver.claimType != evidence.ClaimType {
		t.Fatalf("resolver claim type = %q, want %q",
			resolver.claimType, evidence.ClaimType)
	}
}

func TestServiceUsesExplicitEvaluationTime(t *testing.T) {
	evaluatedAt := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)

	resolver := &stubAuthorityGrantResolver{
		grants: []authority.AuthorityGrant{
			activeGrant(evaluatedAt.Add(time.Second)),
		},
	}

	service := Service{authorityResolver: resolver}
	got := service.Evaluate(
		context.Background(),
		serviceInput(positiveEvidence(), evaluatedAt),
	)

	if got.Authority.Decision != authority.AuthorityReview {
		t.Fatalf("authority decision = %q, want %q",
			got.Authority.Decision, authority.AuthorityReview)
	}
	if got.Authority.Reason != "NO_CURRENT_GRANT" {
		t.Fatalf("authority reason = %q, want NO_CURRENT_GRANT",
			got.Authority.Reason)
	}
}

func TestServiceResultPreservesNormalizedEvidence(t *testing.T) {
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	evidence := positiveEvidence()
	evidence.CredentialID = "credential-123"

	resolver := &stubAuthorityGrantResolver{
		grants: []authority.AuthorityGrant{
			activeGrant(now.Add(-time.Hour)),
		},
	}

	service := Service{authorityResolver: resolver}
	got := service.Evaluate(
		context.Background(),
		serviceInput(evidence, now),
	)

	if got.Evidence.CredentialID != evidence.CredentialID {
		t.Fatalf("result credential ID = %q, want %q",
			got.Evidence.CredentialID, evidence.CredentialID)
	}
}

func TestServiceResolverErrorTakesPrecedenceOverReturnedGrants(t *testing.T) {
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)

	resolver := &stubAuthorityGrantResolver{
		grants: []authority.AuthorityGrant{
			activeGrant(now.Add(-time.Hour)),
		},
		err: errors.New("authority resolution incomplete"),
	}

	service := Service{authorityResolver: resolver}
	got := service.Evaluate(
		context.Background(),
		serviceInput(positiveEvidence(), now),
	)

	if got.Authority.Decision != authority.AuthorityReview {
		t.Fatalf("authority decision = %q, want %q",
			got.Authority.Decision, authority.AuthorityReview)
	}
	if got.Authority.Reason != authority.AuthorityReasonResolutionFailed {
		t.Fatalf("authority reason = %q, want %q",
			got.Authority.Reason, authority.AuthorityReasonResolutionFailed)
	}
	if got.Authority.GrantID != "" {
		t.Fatalf("authority grant ID = %q, want empty on resolution failure",
			got.Authority.GrantID)
	}
	if got.Verification.Decision != VerificationReview {
		t.Fatalf("verification decision = %q, want %q",
			got.Verification.Decision, VerificationReview)
	}
}
