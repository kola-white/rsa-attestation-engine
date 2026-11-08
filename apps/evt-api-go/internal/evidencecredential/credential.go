// Package evidencecredential implements the bounded, evidence-backed employment
// issuance profile. Cryptography, authority and verifier reliance stay separate.
package evidencecredential

import (
	"context"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/authority"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/credential"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/requestbinding"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/verification"
)

const (
	Issuer       = requestbinding.IssuerID
	VCT          = Issuer + "/credentials/employment-role/v1"
	Policy       = "employment.authoritative/manual-evidence-pilot/v1"
	ManualPolicy = "MANUAL_REVIEW_PILOT_V1"
)

var ErrUnavailable = errors.New("evidence_issuance_unavailable")
var ErrIneligible = errors.New("evidence_issuance_ineligible")

// Commitment contains identifiers/digests only, never private object locations
// or evidence bytes. SnapshotDigest hashes the immutable PostgreSQL JSONB text;
// it is NOT a claim that arbitrary JSON reserialization has the same digest.
type Commitment struct {
	RequestID         string `json:"request_id"`
	BindingID         string `json:"binding_id"`
	CaseID            string `json:"case_id"`
	EmployerID        string `json:"employer_id"`
	SubjectID         string `json:"subject_id"`
	RequestVersion    int64  `json:"request_version"`
	SnapshotDigest    string `json:"snapshot_sha256"`
	ManifestID        string `json:"manifest_id"`
	ManifestDigest    string `json:"manifest_sha256"`
	ManualPolicy      string `json:"manual_policy"`
	AuthorityGrantID  string `json:"authority_grant_id"`
	AuthorityPosition int64  `json:"authority_recording_position"`
}

type Claims struct {
	jwt.RegisteredClaims
	VCT                   string                            `json:"vct"`
	ClaimType             string                            `json:"claim_type"`
	Employment            credential.EmploymentRelationship `json:"employment"`
	Evidence              Commitment                        `json:"evidence_binding"`
	PolicyID              string                            `json:"verification_policy"`
	CredentialFingerprint string                            `json:"credential_spki_sha256"`
}

// KeyBinding is a trusted operator-established association. Neither a key in
// an arbitrary caller-supplied JWKS nor possession of a private key establishes
// this association. Revocation/validity are checked again under transaction lock.
type KeyBinding struct {
	Issuer, KID, Fingerprint string
	PublicKey                *rsa.PublicKey
	ValidFrom, ValidUntil    time.Time
	Revoked                  bool
}

func Fingerprint(key *rsa.PublicKey) string {
	if key == nil || key.N == nil || key.N.Sign() <= 0 || key.E != 65537 {
		return ""
	}
	b, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		return ""
	}
	return Digest(b)
}
func Digest(b []byte) string { d := sha256.Sum256(b); return hex.EncodeToString(d[:]) }
func (k KeyBinding) valid(now time.Time) bool {
	return k.Issuer == Issuer && k.KID != "" && k.PublicKey != nil && k.PublicKey.N != nil && k.PublicKey.N.Sign() > 0 && k.PublicKey.N.BitLen() >= 3072 && k.PublicKey.E == 65537 &&
		k.Fingerprint == Fingerprint(k.PublicKey) && !k.Revoked && !now.Before(k.ValidFrom) && now.Before(k.ValidUntil)
}
func digestValid(v string) bool {
	b, e := hex.DecodeString(v)
	return e == nil && len(b) == 32 && v == strings.ToLower(v)
}
func (c Claims) valid() bool {
	e := c.Evidence
	ids := []string{c.ID, c.Subject, e.SubjectID, e.RequestID, e.BindingID, e.ManifestID}
	for _, id := range ids {
		if _, err := uuid.Parse(id); err != nil {
			return false
		}
	}
	return c.Issuer == Issuer && c.VCT == VCT && c.ClaimType == credential.ClaimTypeEmploymentRole && c.PolicyID == Policy &&
		c.Subject == e.SubjectID && c.Employment.EmployerID == e.EmployerID && e.EmployerID != "" && strings.TrimSpace(c.Employment.Title) != "" &&
		e.CaseID != "" && e.RequestVersion >= 1 && e.ManualPolicy == ManualPolicy && e.AuthorityGrantID != "" && e.AuthorityPosition > 0 &&
		digestValid(e.SnapshotDigest) && digestValid(e.ManifestDigest) && digestValid(c.CredentialFingerprint) &&
		c.IssuedAt != nil && c.NotBefore != nil && c.ExpiresAt != nil && c.IssuedAt.Equal(c.NotBefore.Time) && c.ExpiresAt.After(c.IssuedAt.Time)
}

// Signer reuses the configured RSA signer. No API exposes its private material.
type Signer interface {
	KeyID() string
	PublicKey() *rsa.PublicKey
	SignEvidenceClaims(context.Context, Claims) (string, error)
}

// Prepared is supplied solely by the trusted backend under the SAME transaction
// locks used for issuance commit, never from a client's eligibility receipt.
type Prepared struct {
	Context            Commitment
	Employment         credential.EmploymentRelationship
	Snapshot, Manifest string // exact canonical database bytes, not public output
	Authority          authority.AuthoritySnapshot
	Key                KeyBinding
	Now, ExpiresAt     time.Time
}

// IssueLocked must run inside the backend's locked transaction. Caller must
// persist the exact compact artifact before committing and return it only after
// commit succeeds. A failed commit must never publish the signed artifact.
func IssueLocked(ctx context.Context, p Prepared, signer Signer) (Claims, string, error) {
	if signer == nil || !p.Key.valid(p.Now) || signer.KeyID() != p.Key.KID || Fingerprint(signer.PublicKey()) != p.Key.Fingerprint {
		return Claims{}, "", ErrUnavailable
	}
	if !manifestMatches(p.Manifest, p.Context) || !p.ExpiresAt.After(p.Now) || p.ExpiresAt.After(p.Key.ValidUntil) || Digest([]byte(p.Snapshot)) != p.Context.SnapshotDigest || Digest([]byte(p.Manifest)) != p.Context.ManifestDigest {
		return Claims{}, "", ErrIneligible
	}
	resolver, err := authority.NewSnapshotAuthorityResolver(p.Authority)
	if err != nil {
		return Claims{}, "", err
	}
	evidence := credential.NormalizedCredentialEvidence{Issuer: credential.IssuerEvidence{Identifier: Issuer}, ClaimType: credential.ClaimTypeEmploymentRole, Claim: credential.EmploymentClaim{Employment: p.Employment}}
	grants, err := resolver.ResolveAuthorityGrants(ctx, Issuer, credential.ClaimTypeEmploymentRole)
	if err != nil {
		return Claims{}, "", err
	}
	decision := authority.Evaluator{}.Evaluate(evidence, grants, p.Now)
	if decision.Decision != authority.AuthorityAuthorized {
		return Claims{}, "", ErrIneligible
	}
	p.Context.AuthorityGrantID = decision.GrantID
	p.Context.AuthorityPosition = p.Authority.RecordingPosition
	now := p.Now.UTC().Truncate(time.Second)
	end := p.ExpiresAt.UTC().Truncate(time.Second)
	claims := Claims{RegisteredClaims: jwt.RegisteredClaims{Issuer: Issuer, Subject: p.Context.SubjectID, ID: uuid.NewString(), IssuedAt: jwt.NewNumericDate(now), NotBefore: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(end)}, VCT: VCT, ClaimType: credential.ClaimTypeEmploymentRole, Employment: p.Employment, Evidence: p.Context, PolicyID: Policy, CredentialFingerprint: p.Key.Fingerprint}
	if !claims.valid() {
		return Claims{}, "", ErrIneligible
	}
	compact, err := signer.SignEvidenceClaims(ctx, claims)
	if err != nil {
		return Claims{}, "", err
	}
	return claims, compact, nil
}

// Facts is trusted current resolution, not input accepted from a verifier client.
// BindingValid includes exact request/subject/employer/snapshot, unrevoked binding,
// independent contextual reviews and the current bounded manual evidence policy.
// KnownStatus is distinct from "not revoked": absence/unavailability is unknown.
type Facts struct {
	Key                                     KeyBinding
	Claims                                  Claims
	TokenDigest                             string
	Snapshot, Manifest                      string
	KnownStatus, ClaimRevoked, BindingValid bool
	Authority                               authority.AuthoritySnapshot
	EvaluatedAt                             time.Time
}

type Resolver interface {
	// WithFacts holds credential, claim/binding, evidence and authority locks until
	// fn finishes. It resolves only the exact presented token/issuance record.
	WithFacts(context.Context, string, func(Facts) error) error
}

type Outcome struct {
	Validity     credential.EvidenceResult         `json:"validity"`
	Status       credential.EvidenceResult         `json:"status"`
	ScanStatuses []string                          `json:"scan_statuses,omitempty"`
	Decision     verification.VerificationDecision `json:"decision"`
	Reason       string                            `json:"reason"`
	Signature    credential.EvidenceResult         `json:"signature"`
	Authority    authority.AuthorityEvaluation     `json:"authority"`
	Policy       string                            `json:"policy"`
	Claim        *credential.EmploymentClaim       `json:"claim,omitempty"`
	Commitment   *Commitment                       `json:"commitment,omitempty"`
	// Integrity is not evidence approval. Approval depends on trusted resolution.
	ManifestIntegrity credential.EvidenceResult `json:"manifest_integrity"`
	EvidenceReview    credential.EvidenceResult `json:"evidence_review"`
	ScanPolicy        string                    `json:"scan_policy,omitempty"`
}

// Verify has no access to a private key or issuer signing object. The relying
// caller must explicitly name its expected subject; this proves identifier
// association, NOT possession of a holder key or control of that subject.
func Verify(ctx context.Context, raw, subject, policy string, now time.Time, resolver Resolver) Outcome {
	out := Outcome{Validity: credential.EvidenceUnknown, Status: credential.EvidenceUnknown, Decision: verification.VerificationReview, Reason: "TRUSTED_RESOLUTION_UNAVAILABLE", Signature: credential.EvidenceUnknown, Policy: policy, ManifestIntegrity: credential.EvidenceUnknown, EvidenceReview: credential.EvidenceUnknown}
	if resolver == nil {
		return out
	}
	if subject == "" || policy != Policy {
		out.Reason = "VERIFIER_POLICY_OR_SUBJECT_REQUIRED"
		return out
	}
	// This profile deliberately has no selective disclosures or key-binding JWT.
	// Its SD-JWT serialization still terminates in ~; arbitrary extensions fail.
	if len(raw) > 1<<20 || !strings.HasSuffix(raw, "~") || strings.Count(raw, "~") != 1 {
		out.Decision = verification.VerificationRejected
		out.Reason = "CREDENTIAL_FORMAT_INVALID"
		return out
	}
	err := resolver.WithFacts(ctx, Digest([]byte(raw)), func(f Facts) error {
		if !f.EvaluatedAt.IsZero() {
			now = f.EvaluatedAt.UTC()
		}
		if !f.Key.valid(now) {
			out.Decision = verification.VerificationRejected
			out.Reason = "CREDENTIAL_BINDING_INVALID_OR_REVOKED"
			return nil
		}
		var claims Claims
		token, err := jwt.ParseWithClaims(strings.TrimSuffix(raw, "~"), &claims, func(t *jwt.Token) (any, error) {
			if t.Header["kid"] != f.Key.KID || t.Header["typ"] != "dc+sd-jwt" {
				return nil, ErrIneligible
			}
			return f.Key.PublicKey, nil
		}, jwt.WithValidMethods([]string{"RS256"}), jwt.WithoutClaimsValidation())
		if err != nil || token == nil || !token.Valid {
			out.Decision = verification.VerificationRejected
			out.Reason = "SIGNATURE_OR_VALIDITY_INVALID"
			out.Signature = credential.EvidenceInvalid
			return nil
		}
		out.Signature = credential.EvidenceValid
		validator := jwt.NewValidator(jwt.WithIssuer(Issuer), jwt.WithExpirationRequired(), jwt.WithIssuedAt(), jwt.WithTimeFunc(func() time.Time { return now }))
		if err := validator.Validate(&claims); err != nil {
			out.Decision = verification.VerificationRejected
			out.Reason = "CREDENTIAL_NOT_VALID_NOW"
			out.Validity = credential.EvidenceInvalid
			return nil
		}
		out.Validity = credential.EvidenceValid
		if !claims.valid() || claims.Subject != subject || claims.CredentialFingerprint != f.Key.Fingerprint {
			out.Decision = verification.VerificationRejected
			out.Reason = "CLAIM_IDENTITY_INVALID"
			return nil
		}
		expected, _ := json.Marshal(f.Claims)
		actual, _ := json.Marshal(claims)
		if f.TokenDigest != Digest([]byte(raw)) || string(actual) != string(expected) || Digest([]byte(f.Snapshot)) != claims.Evidence.SnapshotDigest || Digest([]byte(f.Manifest)) != claims.Evidence.ManifestDigest {
			out.Decision = verification.VerificationRejected
			out.Reason = "ISSUANCE_OR_MANIFEST_MISMATCH"
			out.ManifestIntegrity = credential.EvidenceInvalid
			return nil
		}
		if !manifestMatches(f.Manifest, claims.Evidence) {
			out.Decision = verification.VerificationRejected
			out.Reason = "MANIFEST_CONTEXT_INVALID"
			out.ManifestIntegrity = credential.EvidenceInvalid
			return nil
		}
		out.ManifestIntegrity = credential.EvidenceValid
		if !f.KnownStatus {
			out.Reason = "CLAIM_STATUS_UNKNOWN"
			return nil
		}
		if f.ClaimRevoked || !f.BindingValid {
			out.Status = credential.EvidenceInvalid
			out.Decision = verification.VerificationRejected
			out.Reason = "CLAIM_REVOKED_OR_EVIDENCE_DISQUALIFIED"
			return nil
		}
		out.EvidenceReview = credential.EvidenceValid
		out.Status = credential.EvidenceValid
		var manifest struct {
			Evidence []struct {
				ScanStatus string `json:"scanStatus"`
			} `json:"evidence"`
		}
		if err := json.Unmarshal([]byte(f.Manifest), &manifest); err != nil {
			return err
		}
		seen := map[string]bool{}
		for _, file := range manifest.Evidence {
			if !seen[file.ScanStatus] {
				seen[file.ScanStatus] = true
				out.ScanStatuses = append(out.ScanStatuses, file.ScanStatus)
			}
		}
		if f.Authority.RecordingPosition < claims.Evidence.AuthorityPosition || f.Authority.RecordedThrough.Before(claims.IssuedAt.Time) {
			return ErrUnavailable
		}
		resolver, err := authority.NewSnapshotAuthorityResolver(f.Authority)
		if err != nil {
			return err
		}
		grants, err := resolver.ResolveAuthorityGrants(ctx, Issuer, credential.ClaimTypeEmploymentRole)
		if err != nil {
			return err
		}
		evidence := credential.NormalizedCredentialEvidence{ClaimType: credential.ClaimTypeEmploymentRole, Issuer: credential.IssuerEvidence{Identifier: Issuer, KeyResolution: credential.KeyResolutionEvidence{Result: credential.EvidenceValid}}, Claim: credential.EmploymentClaim{ClaimType: credential.ClaimTypeEmploymentRole, Subject: credential.EmploymentSubject{ID: claims.Subject}, Employment: claims.Employment}, Cryptographic: credential.CryptographicEvidence{Signature: credential.EvidenceValid}, Validity: credential.ValidityEvidence{Result: credential.EvidenceValid}, Subject: credential.SubjectBindingEvidence{Result: credential.EvidenceValid, Method: "canonical-identifier", SubjectID: claims.Subject}, Status: credential.StatusEvidence{Result: credential.EvidenceValid}}
		out.Authority = authority.Evaluator{}.Evaluate(evidence, grants, now)
		evaluation := verification.Evaluator{}.Evaluate(verification.EvaluationInput{Evidence: evidence, Authority: out.Authority, Policy: verification.VerifierPolicy{PolicyID: policy}})
		out.Decision = evaluation.Decision
		out.Reason = "AUTHORITY_OR_POLICY_NOT_SATISFIED"
		if out.Decision == verification.VerificationVerified {
			out.Reason = "AUTHORITATIVE_MANUAL_EVIDENCE_PILOT"
			out.Claim = &evidence.Claim
			out.Commitment = &claims.Evidence
			out.ScanPolicy = ManualPolicy
		}
		return nil
	})
	if err != nil {
		out.Decision = verification.VerificationReview
		out.Reason = "TRUSTED_RESOLUTION_UNAVAILABLE"
		out.Claim = nil
		out.Commitment = nil
		out.EvidenceReview = credential.EvidenceUnknown
	}
	return out
}

// Check the signed digest's contextual meaning as well as its bytes. This does
// not establish that a review actually occurred: trusted current resolution is
// still mandatory and separately reported by Verify.
func manifestMatches(raw string, c Commitment) bool {
	var m struct {
		BindingID      string   `json:"bindingId"`
		RequestID      string   `json:"requestId"`
		CaseID         string   `json:"caseId"`
		SubjectID      string   `json:"subjectId"`
		EmployerID     string   `json:"employerId"`
		IssuerID       string   `json:"issuerId"`
		ClaimType      string   `json:"claimType"`
		ClaimDigest    string   `json:"claimDigest"`
		Policy         string   `json:"policy"`
		RequiredChecks []string `json:"requiredChecks"`
		Evidence       []struct {
			UploadID   string `json:"uploadId"`
			CheckID    string `json:"checkId"`
			Hash       string `json:"evidenceHash"`
			ReviewID   string `json:"reviewId"`
			ReviewerID string `json:"reviewerId"`
			GrantID    string `json:"reviewGrantId"`
			ScanStatus string `json:"scanStatus"`
		} `json:"evidence"`
	}
	if json.Unmarshal([]byte(raw), &m) != nil || m.BindingID != c.BindingID || m.RequestID != c.RequestID || m.CaseID != c.CaseID || m.SubjectID != c.SubjectID || m.EmployerID != c.EmployerID || m.IssuerID != Issuer || m.ClaimType != credential.ClaimTypeEmploymentRole || m.ClaimDigest != c.SnapshotDigest || m.Policy != ManualPolicy || len(m.RequiredChecks) == 0 || len(m.Evidence) == 0 {
		return false
	}
	checks := map[string]int{}
	uploads := map[string]bool{}
	for _, check := range m.RequiredChecks {
		if check == "" {
			return false
		}
		if _, exists := checks[check]; exists {
			return false
		}
		checks[check] = 0
	}
	for _, e := range m.Evidence {
		if _, exists := checks[e.CheckID]; !exists || !digestValid(e.Hash) || (e.ScanStatus != "PENDING" && e.ScanStatus != "CLEAN") || uploads[e.UploadID] {
			return false
		}
		for _, id := range []string{e.UploadID, e.ReviewID, e.ReviewerID, e.GrantID} {
			if _, err := uuid.Parse(id); err != nil {
				return false
			}
		}
		uploads[e.UploadID] = true
		checks[e.CheckID]++
	}
	for _, count := range checks {
		if count == 0 {
			return false
		}
	}
	return true
}

// AuthorityAt is also rechecked after signing but before persistence, while the
// same recording-boundary lock remains held. Clock-based expiry is not a row
// mutation and must not be mistaken for something a lock can prevent.
func AuthorityAt(ctx context.Context, snapshot authority.AuthoritySnapshot, employer string, now time.Time) (authority.AuthorityEvaluation, error) {
	resolver, err := authority.NewSnapshotAuthorityResolver(snapshot)
	if err != nil {
		return authority.AuthorityEvaluation{}, err
	}
	grants, err := resolver.ResolveAuthorityGrants(ctx, Issuer, credential.ClaimTypeEmploymentRole)
	if err != nil {
		return authority.AuthorityEvaluation{}, err
	}
	return (authority.Evaluator{}).Evaluate(credential.NormalizedCredentialEvidence{Issuer: credential.IssuerEvidence{Identifier: Issuer}, ClaimType: credential.ClaimTypeEmploymentRole, Claim: credential.EmploymentClaim{Employment: credential.EmploymentRelationship{EmployerID: employer}}}, grants, now), nil
}
