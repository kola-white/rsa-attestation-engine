package authority

import (
	"testing"
	"time"

	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/credential"
)

func TestEvaluator_AuthorityDecisions(t *testing.T) {
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	past := now.Add(-24 * time.Hour)
	future := now.Add(24 * time.Hour)

	evidence := credential.NormalizedCredentialEvidence{
		ClaimType: credential.ClaimTypeEmploymentRole,
		Claim: credential.EmploymentClaim{
			ClaimType: credential.ClaimTypeEmploymentRole,
			Employment: credential.EmploymentRelationship{
				EmployerID: "employer-123",
			},
		},
		Issuer: credential.IssuerEvidence{
			Identifier: "https://issuer.example",
		},
	}

	tests := []struct {
		name    string
		grants  []AuthorityGrant
		want    AuthorityDecision
		reason  string
		grantID string
	}{
		{
			name: "matching active grant authorizes issuer",
			grants: []AuthorityGrant{
				{
					GrantID:            "grant-1",
					IssuerID:           "https://issuer.example",
					ClaimType:          credential.ClaimTypeEmploymentRole,
					AuthoritySubjectID: "employer-123",
					ValidFrom:          past,
					ValidUntil:         &future,
					State:              AuthorityGrantActive,
				},
			},
			want:    AuthorityAuthorized,
			reason:  "MATCHING_ACTIVE_GRANT",
			grantID: "grant-1",
		},
		{
			name: "valid from boundary is inclusive",
			grants: []AuthorityGrant{
				{
					GrantID:            "grant-valid-from-boundary",
					IssuerID:           "https://issuer.example",
					ClaimType:          credential.ClaimTypeEmploymentRole,
					AuthoritySubjectID: "employer-123",
					ValidFrom:          now,
					ValidUntil:         &future,
					State:              AuthorityGrantActive,
				},
			},
			want:    AuthorityAuthorized,
			reason:  "MATCHING_ACTIVE_GRANT",
			grantID: "grant-valid-from-boundary",
		},
		{
			name: "valid until boundary is inclusive",
			grants: []AuthorityGrant{
				{
					GrantID:            "grant-valid-until-boundary",
					IssuerID:           "https://issuer.example",
					ClaimType:          credential.ClaimTypeEmploymentRole,
					AuthoritySubjectID: "employer-123",
					ValidFrom:          past,
					ValidUntil:         &now,
					State:              AuthorityGrantActive,
				},
			},
			want:    AuthorityAuthorized,
			reason:  "MATCHING_ACTIVE_GRANT",
			grantID: "grant-valid-until-boundary",
		},
		{
			name: "current active grant takes precedence over current inactive grant",
			grants: []AuthorityGrant{
				{
					GrantID:            "grant-inactive",
					IssuerID:           "https://issuer.example",
					ClaimType:          credential.ClaimTypeEmploymentRole,
					AuthoritySubjectID: "employer-123",
					ValidFrom:          past,
					ValidUntil:         &future,
					State:              AuthorityGrantInactive,
				},
				{
					GrantID:            "grant-active",
					IssuerID:           "https://issuer.example",
					ClaimType:          credential.ClaimTypeEmploymentRole,
					AuthoritySubjectID: "employer-123",
					ValidFrom:          past,
					ValidUntil:         &future,
					State:              AuthorityGrantActive,
				},
			},
			want:    AuthorityAuthorized,
			reason:  "MATCHING_ACTIVE_GRANT",
			grantID: "grant-active",
		},
		{
			name: "expired inactive grant does not establish unauthorized",
			grants: []AuthorityGrant{
				{
					GrantID:            "grant-expired-inactive",
					IssuerID:           "https://issuer.example",
					ClaimType:          credential.ClaimTypeEmploymentRole,
					AuthoritySubjectID: "employer-123",
					ValidFrom:          past.Add(-24 * time.Hour),
					ValidUntil:         &past,
					State:              AuthorityGrantInactive,
				},
			},
			want:   AuthorityReview,
			reason: "NO_CURRENT_GRANT",
		},
		{
			name: "matching inactive grant is unauthorized",
			grants: []AuthorityGrant{
				{
					GrantID:            "grant-2",
					IssuerID:           "https://issuer.example",
					ClaimType:          credential.ClaimTypeEmploymentRole,
					AuthoritySubjectID: "employer-123",
					ValidFrom:          past,
					ValidUntil:         &future,
					State:              AuthorityGrantInactive,
				},
			},
			want:    AuthorityUnauthorized,
			reason:  "MATCHING_INACTIVE_GRANT",
			grantID: "grant-2",
		},
		{
			name:   "no grants requires review",
			grants: nil,
			want:   AuthorityReview,
			reason: "NO_APPLICABLE_GRANT",
		},
		{
			name: "wrong issuer does not establish authority",
			grants: []AuthorityGrant{
				{
					GrantID:            "grant-3",
					IssuerID:           "https://other.example",
					ClaimType:          credential.ClaimTypeEmploymentRole,
					AuthoritySubjectID: "employer-123",
					ValidFrom:          past,
					ValidUntil:         &future,
					State:              AuthorityGrantActive,
				},
			},
			want:   AuthorityReview,
			reason: "NO_APPLICABLE_GRANT",
		},
		{
			name: "wrong claim type does not establish authority",
			grants: []AuthorityGrant{
				{
					GrantID:            "grant-4",
					IssuerID:           "https://issuer.example",
					ClaimType:          "employment.compensation",
					AuthoritySubjectID: "employer-123",
					ValidFrom:          past,
					ValidUntil:         &future,
					State:              AuthorityGrantActive,
				},
			},
			want:   AuthorityReview,
			reason: "NO_APPLICABLE_GRANT",
		},
		{
			name: "wrong authority subject does not establish authority",
			grants: []AuthorityGrant{
				{
					GrantID:            "grant-5",
					IssuerID:           "https://issuer.example",
					ClaimType:          credential.ClaimTypeEmploymentRole,
					AuthoritySubjectID: "different-employer",
					ValidFrom:          past,
					ValidUntil:         &future,
					State:              AuthorityGrantActive,
				},
			},
			want:   AuthorityReview,
			reason: "NO_APPLICABLE_GRANT",
		},
		{
			name: "not yet valid grant does not establish authority",
			grants: []AuthorityGrant{
				{
					GrantID:            "grant-6",
					IssuerID:           "https://issuer.example",
					ClaimType:          credential.ClaimTypeEmploymentRole,
					AuthoritySubjectID: "employer-123",
					ValidFrom:          future,
					State:              AuthorityGrantActive,
				},
			},
			want:   AuthorityReview,
			reason: "NO_CURRENT_GRANT",
		},
		{
			name: "expired grant does not establish authority",
			grants: []AuthorityGrant{
				{
					GrantID:            "grant-7",
					IssuerID:           "https://issuer.example",
					ClaimType:          credential.ClaimTypeEmploymentRole,
					AuthoritySubjectID: "employer-123",
					ValidFrom:          past.Add(-24 * time.Hour),
					ValidUntil:         &past,
					State:              AuthorityGrantActive,
				},
			},
			want:   AuthorityReview,
			reason: "NO_CURRENT_GRANT",
		},
	}

	evaluator := Evaluator{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := evaluator.Evaluate(evidence, tt.grants, now)

			if got.Decision != tt.want {
				t.Fatalf("decision = %q, want %q", got.Decision, tt.want)
			}
			if got.Reason != tt.reason {
				t.Fatalf("reason = %q, want %q", got.Reason, tt.reason)
			}
			if got.GrantID != tt.grantID {
				t.Fatalf("grant_id = %q, want %q", got.GrantID, tt.grantID)
			}
		})
	}
}
