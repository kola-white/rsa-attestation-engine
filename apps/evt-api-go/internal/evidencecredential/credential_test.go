package evidencecredential_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/authority"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/credential"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/credential/sdjwtvc"
	ec "github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/evidencecredential"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/verification"
)

type signer struct{ key *rsa.PrivateKey }

func (s signer) KeyID() string             { return "local-only-fixture" }
func (s signer) PublicKey() *rsa.PublicKey { return &s.key.PublicKey }
func (s signer) SignEvidenceClaims(ctx context.Context, c ec.Claims) (string, error) {
	t := jwt.NewWithClaims(jwt.SigningMethodRS256, c)
	t.Header["kid"] = s.KeyID()
	t.Header["typ"] = "dc+sd-jwt"
	raw, e := t.SignedString(s.key)
	return raw + "~", e
}

type facts struct {
	value ec.Facts
	err   error
}

func (f facts) WithFacts(ctx context.Context, digest string, fn func(ec.Facts) error) error {
	if f.err != nil {
		return f.err
	}
	return fn(f.value)
}
func snapshot(t *testing.T, now time.Time, negative bool) authority.AuthoritySnapshot {
	t.Helper()
	at := now.Add(-time.Hour).UTC().Truncate(time.Microsecond)
	id := uuid.NewString()
	r := authority.AuthorizationRecord{AuthorizationID: id, IssuerID: ec.Issuer, AuthoritySubjectID: "employer", ClaimType: credential.ClaimTypeEmploymentRole, Adjudication: &authority.AuthorizationAdjudication{DecisionID: uuid.NewString(), AuthorizationID: id, IssuerID: ec.Issuer, AuthoritySubjectID: "employer", ClaimType: credential.ClaimTypeEmploymentRole, Outcome: authority.AdjudicationApproved, DecisionMakerID: "local-adjudicator", AuthorityBasis: "local-only-test", EvidenceRef: "local-only-proof", DecidedAt: at.Add(time.Minute), RecordedAt: at.Add(time.Minute)}}
	var journal authority.AuthorizationJournal
	events := []authority.AuthorizationEvent{authority.EventSubmit, authority.EventApprove, authority.EventActivate}
	if negative {
		events = append(events, authority.EventRevoke)
	}
	for i, event := range events {
		stamp := at.Add(time.Duration(i) * time.Minute)
		if err := journal.Append(authority.AuthorizationJournalEvent{EventID: uuid.NewString(), AuthorizationID: id, EventType: event, EffectiveAt: stamp, RecordedAt: stamp, ActorID: "local-adjudicator", EvidenceRef: "local-only-proof", ConditionsSatisfied: true}); err != nil {
			t.Fatal(err)
		}
	}
	return authority.AuthoritySnapshot{Complete: true, RecordingPosition: int64(2 + len(events)), RecordedThrough: now, Entries: []authority.AuthoritySnapshotEntry{{Record: r, Journal: journal}}}
}
func TestBoundCredentialIndependentVerification(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 3072)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	s := signer{key}
	p := ec.Prepared{Context: ec.Commitment{RequestID: uuid.NewString(), BindingID: uuid.NewString(), CaseID: "local-case", EmployerID: "employer", SubjectID: uuid.NewString(), RequestVersion: 1, SnapshotDigest: ec.Digest([]byte("snapshot")), ManifestID: uuid.NewString(), ManifestDigest: ec.Digest([]byte(`{"evidence":[{"scanStatus":"PENDING"}]}`)), ManualPolicy: ec.ManualPolicy}, Employment: credential.EmploymentRelationship{EmployerID: "employer", Title: "Engineer"}, Snapshot: "snapshot", Manifest: `{"evidence":[{"scanStatus":"PENDING"}]}`, Authority: snapshot(t, now, false), Key: ec.KeyBinding{Issuer: ec.Issuer, KID: s.KeyID(), Fingerprint: ec.Fingerprint(s.PublicKey()), PublicKey: s.PublicKey(), ValidFrom: now.Add(-time.Hour), ValidUntil: now.Add(time.Hour)}, Now: now, ExpiresAt: now.Add(30 * time.Minute)}
	manifest, _ := json.Marshal(map[string]any{"bindingId": p.Context.BindingID, "requestId": p.Context.RequestID, "caseId": p.Context.CaseID, "subjectId": p.Context.SubjectID, "employerId": p.Context.EmployerID, "issuerId": ec.Issuer, "claimType": credential.ClaimTypeEmploymentRole, "claimDigest": p.Context.SnapshotDigest, "policy": ec.ManualPolicy, "requiredChecks": []string{"role"}, "evidence": []map[string]string{{"uploadId": uuid.NewString(), "checkId": "role", "evidenceHash": ec.Digest([]byte("synthetic only")), "reviewId": uuid.NewString(), "reviewerId": uuid.NewString(), "reviewGrantId": uuid.NewString(), "scanStatus": "PENDING"}}})
	p.Manifest = string(manifest)
	p.Context.ManifestDigest = ec.Digest(manifest)
	claims, raw, err := ec.IssueLocked(context.Background(), p, s)
	if err != nil {
		t.Fatal(err)
	}
	adapter := sdjwtvc.New(registeredFixtureKey{key: &key.PublicKey}, sdjwtvc.WithExpectedVCT(ec.VCT))
	normalized, err := adapter.VerifyAndNormalize(context.Background(), []byte(raw), credential.VerificationOptions{Now: now})
	if err != nil || normalized.Cryptographic.Signature != credential.EvidenceValid || normalized.Claim.Subject.ID != claims.Subject || normalized.Claim.Employment.EmployerID != p.Employment.EmployerID || normalized.Issuer.Identifier != ec.Issuer {
		t.Fatal("existing SD-JWT adapter compatibility", err, normalized.Errors)
	}
	original := ec.Facts{Key: p.Key, Claims: claims, TokenDigest: ec.Digest([]byte(raw)), Snapshot: p.Snapshot, Manifest: p.Manifest, KnownStatus: true, BindingValid: true, Authority: p.Authority}
	for _, condition := range []string{"valid", "bad-signature", "signature-only", "authority-unavailable", "authority-revoked", "wrong-employer-authority", "issuer-mismatch", "subject-mismatch", "key-revoked", "wrong-key", "claim-revoked", "status-unknown", "binding-revoked", "changed-claims", "manifest-mismatch", "snapshot-mismatch", "expired", "unknown-policy", "token-mismatch"} {
		t.Run(condition, func(t *testing.T) {
			f := original
			subject := claims.Subject
			policy := ec.Policy
			at := now
			presented := raw
			resolver := facts{value: f}
			switch condition {
			case "bad-signature":
				parts := strings.Split(presented, ".")
				parts[1] = base64.RawURLEncoding.EncodeToString([]byte(`{"iss":"https://cvera.app","sub":"tampered"}`))
				presented = strings.Join(parts, ".")
			case "signature-only":
				resolver.err = errors.New("no trusted facts")
			case "authority-unavailable":
				f.Authority.Complete = false
			case "authority-revoked":
				f.Authority = snapshot(t, now, true)
			case "wrong-employer-authority":
				f.Authority = snapshot(t, now, false)
				f.Authority.Entries[0].Record.AuthoritySubjectID = "unrelated"
			case "issuer-mismatch":
				f.Key.Issuer = "https://unrelated.invalid"
			case "subject-mismatch":
				subject = uuid.NewString()
			case "key-revoked":
				f.Key.Revoked = true
			case "wrong-key":
				f.Key.Fingerprint = ec.Digest([]byte("wrong"))
			case "claim-revoked":
				f.ClaimRevoked = true
			case "status-unknown":
				f.KnownStatus = false
			case "binding-revoked":
				f.BindingValid = false
			case "changed-claims":
				f.Claims.Employment.Title = "Changed"
			case "manifest-mismatch":
				f.Manifest = "different"
			case "snapshot-mismatch":
				f.Snapshot = "different"
			case "expired":
				at = now.Add(31 * time.Minute)
			case "unknown-policy":
				policy = "permissive"
			case "token-mismatch":
				f.TokenDigest = ec.Digest([]byte("other"))
			}
			resolver.value = f
			got := ec.Verify(context.Background(), presented, subject, policy, at, resolver)
			if condition == "valid" {
				if got.Decision != verification.VerificationVerified || got.Commitment == nil || got.ScanPolicy != ec.ManualPolicy {
					t.Fatalf("valid: %+v", got)
				}
			} else if got.Decision == verification.VerificationVerified || got.Claim != nil {
				t.Fatalf("false trust %s: %+v", condition, got)
			}
		})
	}
	for _, condition := range []string{"revoked-authority", "unavailable-authority", "stale-snapshot", "stale-manifest", "invalid-key"} {
		t.Run("issue-"+condition, func(t *testing.T) {
			bad := p
			switch condition {
			case "revoked-authority":
				bad.Authority = snapshot(t, now, true)
			case "unavailable-authority":
				bad.Authority.Complete = false
			case "stale-snapshot":
				bad.Snapshot = "changed"
			case "stale-manifest":
				bad.Manifest = "changed"
			case "invalid-key":
				bad.Key.Revoked = true
			}
			_, token, err := ec.IssueLocked(context.Background(), bad, s)
			if err == nil || token != "" {
				t.Fatal("invalid issuance accepted")
			}
		})
	}
}

// Explicit local registration, not arbitrary key trust or production authority.
type registeredFixtureKey struct{ key *rsa.PublicKey }

func (k registeredFixtureKey) ResolveIssuerKey(ctx context.Context, issuer, kid, alg string) (any, string, error) {
	if issuer != ec.Issuer || kid != "local-only-fixture" || alg != "RS256" {
		return nil, "", errors.New("unregistered issuer credential")
	}
	return k.key, "trusted-operator-registration", nil
}
