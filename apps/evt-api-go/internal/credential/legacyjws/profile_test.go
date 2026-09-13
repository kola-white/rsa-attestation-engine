package legacyjws

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/credential"
)

const testKID = "legacy-test-key"

type testKeyResolver struct {
	key any
	err error
}

func (r testKeyResolver) ResolveKey(
	_ context.Context,
	_ string,
) (any, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.key, nil
}

type testStatusResolver struct {
	result credential.EvidenceResult
	err    error
}

func (r testStatusResolver) ResolveStatus(
	_ context.Context,
	_ string,
	_ string,
	_ string,
) (credential.EvidenceResult, error) {
	if r.err != nil {
		return credential.EvidenceUnknown, r.err
	}
	return r.result, nil
}

func TestLegacyJWS_GoldenValid_NormalizesToValidEvidence(t *testing.T) {
	privateKey := newTestRSAKey(t)

	payload := loadGoldenFixture(t, "golden-valid.attestation.json")
	now := insideValidityWindow(t, payload)

	compact := signFixture(t, payload, privateKey, testKID)

	profile := New(
		testKeyResolver{key: &privateKey.PublicKey},
		testStatusResolver{result: credential.EvidenceValid},
	)

	got, err := profile.VerifyAndNormalize(
		context.Background(),
		[]byte(compact),
		credential.VerificationOptions{Now: now},
	)
	if err != nil {
		t.Fatalf("VerifyAndNormalize() error = %v", err)
	}

	assertEvidenceResult(
		t,
		"key resolution",
		got.Issuer.KeyResolution.Result,
		credential.EvidenceValid,
	)

	assertEvidenceResult(
		t,
		"signature",
		got.Cryptographic.Signature,
		credential.EvidenceValid,
	)

	assertEvidenceResult(
		t,
		"validity",
		got.Validity.Result,
		credential.EvidenceValid,
	)

	assertEvidenceResult(
		t,
		"status",
		got.Status.Result,
		credential.EvidenceValid,
	)

	if got.ClaimType != credential.ClaimTypeEmploymentRole {
		t.Fatalf(
			"ClaimType = %q, want %q",
			got.ClaimType,
			credential.ClaimTypeEmploymentRole,
		)
	}

	if strings.TrimSpace(got.Claim.Employment.Title) == "" {
		t.Fatal("normalized EmploymentClaim title is empty")
	}

	if hasEvidenceError(got, "BAD_SIGNATURE") {
		t.Fatal("valid fixture unexpectedly contains BAD_SIGNATURE")
	}
}

func TestLegacyJWS_GoldenRevoked_NormalizesStatusInvalid(t *testing.T) {
	privateKey := newTestRSAKey(t)

	payload := loadGoldenFixture(t, "golden-revoked.attestation.json")
	now := insideValidityWindow(t, payload)

	compact := signFixture(t, payload, privateKey, testKID)

	profile := New(
		testKeyResolver{key: &privateKey.PublicKey},
		testStatusResolver{result: credential.EvidenceInvalid},
	)

	got, err := profile.VerifyAndNormalize(
		context.Background(),
		[]byte(compact),
		credential.VerificationOptions{Now: now},
	)
	if err != nil {
		t.Fatalf("VerifyAndNormalize() error = %v", err)
	}

	assertEvidenceResult(
		t,
		"signature",
		got.Cryptographic.Signature,
		credential.EvidenceValid,
	)

	assertEvidenceResult(
		t,
		"status",
		got.Status.Result,
		credential.EvidenceInvalid,
	)

	// Important architectural assertion:
	// revoked is evidence state, not a Cvera final trust decision.
	if got.ProfileID != ProfileID {
		t.Fatalf("ProfileID = %q, want %q", got.ProfileID, ProfileID)
	}
}

func TestLegacyJWS_GoldenNotYetValid_NormalizesValidityInvalid(t *testing.T) {
	privateKey := newTestRSAKey(t)

	payload := loadGoldenFixture(t, "golden-notyetvalid.attestation.json")
	now := beforeValidityWindow(t, payload)

	compact := signFixture(t, payload, privateKey, testKID)

	profile := New(
		testKeyResolver{key: &privateKey.PublicKey},
		testStatusResolver{result: credential.EvidenceValid},
	)

	got, err := profile.VerifyAndNormalize(
		context.Background(),
		[]byte(compact),
		credential.VerificationOptions{Now: now},
	)
	if err != nil {
		t.Fatalf("VerifyAndNormalize() error = %v", err)
	}

	assertEvidenceResult(
		t,
		"signature",
		got.Cryptographic.Signature,
		credential.EvidenceValid,
	)

	assertEvidenceResult(
		t,
		"validity",
		got.Validity.Result,
		credential.EvidenceInvalid,
	)

	if !hasEvidenceError(got, "NOT_VALID_NOW") {
		t.Fatalf(
			"expected NOT_VALID_NOW evidence error; got %+v",
			got.Errors,
		)
	}
}

func TestLegacyJWS_GoldenExpired_NormalizesValidityInvalid(t *testing.T) {
	privateKey := newTestRSAKey(t)

	payload := loadGoldenFixture(t, "golden-expired.attestation.json")
	now := afterValidityWindow(t, payload)

	compact := signFixture(t, payload, privateKey, testKID)

	profile := New(
		testKeyResolver{key: &privateKey.PublicKey},
		testStatusResolver{result: credential.EvidenceValid},
	)

	got, err := profile.VerifyAndNormalize(
		context.Background(),
		[]byte(compact),
		credential.VerificationOptions{Now: now},
	)
	if err != nil {
		t.Fatalf("VerifyAndNormalize() error = %v", err)
	}

	assertEvidenceResult(
		t,
		"signature",
		got.Cryptographic.Signature,
		credential.EvidenceValid,
	)

	assertEvidenceResult(
		t,
		"validity",
		got.Validity.Result,
		credential.EvidenceInvalid,
	)

	if !hasEvidenceError(got, "NOT_VALID_NOW") {
		t.Fatalf(
			"expected NOT_VALID_NOW evidence error; got %+v",
			got.Errors,
		)
	}
}

func TestLegacyJWS_GoldenInvalidSignature_NormalizesSignatureInvalid(t *testing.T) {
	trustedKey := newTestRSAKey(t)
	untrustedKey := newTestRSAKey(t)

	payload := loadGoldenFixture(
		t,
		"golden-invalid-signature.attestation.json",
	)

	// The fixture supplies the legacy payload scenario. For the adapter
	// boundary test, deliberately sign it using a key other than the key
	// returned by the resolver.
	compact := signFixture(t, payload, untrustedKey, testKID)

	profile := New(
		testKeyResolver{key: &trustedKey.PublicKey},
		testStatusResolver{result: credential.EvidenceValid},
	)

	got, err := profile.VerifyAndNormalize(
		context.Background(),
		[]byte(compact),
		credential.VerificationOptions{
			Now: time.Now().UTC(),
		},
	)
	if err != nil {
		t.Fatalf("VerifyAndNormalize() error = %v", err)
	}

	assertEvidenceResult(
		t,
		"signature",
		got.Cryptographic.Signature,
		credential.EvidenceInvalid,
	)

	if !hasEvidenceError(got, "BAD_SIGNATURE") {
		t.Fatalf(
			"expected BAD_SIGNATURE evidence error; got %+v",
			got.Errors,
		)
	}

	// Signature failure MUST stop before the payload can become trusted
	// normalized evidence.
	if got.Claim.Employment.Title != "" {
		t.Fatalf(
			"claim was normalized despite invalid signature: %+v",
			got.Claim,
		)
	}
}

func TestLegacyJWS_GoldenInvalidLiveness_NormalizesValidityInvalid(t *testing.T) {
	privateKey := newTestRSAKey(t)

	payload := loadGoldenFixture(
		t,
		"golden-invalid-liveness.attestation.json",
	)

	compact := signFixture(t, payload, privateKey, testKID)

	profile := New(
		testKeyResolver{key: &privateKey.PublicKey},
		testStatusResolver{result: credential.EvidenceValid},
	)

	// This fixture represents a temporal edge/failure case. Pick a time
	// outside the fixture's declared window so the adapter deterministically
	// exercises its normalized temporal evidence state.
	now := afterValidityWindow(t, payload)

	got, err := profile.VerifyAndNormalize(
		context.Background(),
		[]byte(compact),
		credential.VerificationOptions{Now: now},
	)
	if err != nil {
		t.Fatalf("VerifyAndNormalize() error = %v", err)
	}

	assertEvidenceResult(
		t,
		"validity",
		got.Validity.Result,
		credential.EvidenceInvalid,
	)
}

func TestLegacyJWS_GoldenInvalidSchema_ProducesSchemaEvidenceError(t *testing.T) {
	privateKey := newTestRSAKey(t)

	payload := loadGoldenFixture(
		t,
		"golden-invalid-schema.attestation.json",
	)

	compact := signFixture(t, payload, privateKey, testKID)

	profile := New(
		testKeyResolver{key: &privateKey.PublicKey},
		testStatusResolver{result: credential.EvidenceValid},
	)

	got, err := profile.VerifyAndNormalize(
		context.Background(),
		[]byte(compact),
		credential.VerificationOptions{
			Now: time.Now().UTC(),
		},
	)
	if err != nil {
		t.Fatalf("VerifyAndNormalize() error = %v", err)
	}

	// The legacy adapter currently performs the AP-1 semantic checks that
	// are required for normalization. It does not embed the complete JSON
	// Schema validator.
	//
	// Therefore the invalid fixture must either:
	//   1. produce one of the normalized AP-1 structural/profile errors; or
	//   2. be rejected as an unsupported legacy payload.
	//
	// It must never silently normalize as completely clean evidence.
	if len(got.Errors) == 0 {
		t.Fatalf(
			"invalid-schema golden fixture produced no evidence errors: %+v",
			got,
		)
	}
}

func TestLegacyJWS_KeyResolutionFailure_DoesNotAuthorizeByImplication(t *testing.T) {
	privateKey := newTestRSAKey(t)

	payload := loadGoldenFixture(t, "golden-valid.attestation.json")
	compact := signFixture(t, payload, privateKey, testKID)

	profile := New(
		testKeyResolver{
			err: errors.New("test key not found"),
		},
		testStatusResolver{result: credential.EvidenceValid},
	)

	got, err := profile.VerifyAndNormalize(
		context.Background(),
		[]byte(compact),
		credential.VerificationOptions{
			Now: time.Now().UTC(),
		},
	)
	if err != nil {
		t.Fatalf("VerifyAndNormalize() error = %v", err)
	}

	if got.Cryptographic.Signature == credential.EvidenceValid {
		t.Fatal("signature must not be valid when key resolution fails")
	}

	if got.Issuer.KeyResolution.Result == credential.EvidenceValid {
		t.Fatal("key resolution must not be valid when resolver fails")
	}

	if !hasEvidenceError(got, "BAD_SIGNATURE") {
		t.Fatalf(
			"expected BAD_SIGNATURE evidence error; got %+v",
			got.Errors,
		)
	}
}

func newTestRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey() error = %v", err)
	}

	return key
}

func signFixture(
	t *testing.T,
	payload map[string]any,
	privateKey *rsa.PrivateKey,
	kid string,
) string {
	t.Helper()

	claims := jwt.MapClaims(payload)

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = kid
	token.Header["typ"] = "JWT"

	compact, err := token.SignedString(privateKey)
	if err != nil {
		t.Fatalf("SignedString() error = %v", err)
	}

	return compact
}

func loadGoldenFixture(
	t *testing.T,
	name string,
) map[string]any {
	t.Helper()

	path := filepath.Join(
		repoRoot(t),
		"examples",
		"PH-3-RESUME",
		name,
	)

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden fixture %q: %v", path, err)
	}

	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf(
			"unmarshal golden fixture %q: %v",
			path,
			err,
		)
	}

	return payload
}

func repoRoot(t *testing.T) string {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() failed")
	}

	// currentFile:
	//
	//   <repo>/apps/evt-api-go/internal/credential/legacyjws/profile_test.go
	//
	// Walk back:
	//
	//   legacyjws -> credential -> internal -> evt-api-go -> apps -> repo
	dir := filepath.Dir(currentFile)

	for i := 0; i < 5; i++ {
		dir = filepath.Dir(dir)
	}

	return dir
}

func fixtureValidity(
	t *testing.T,
	payload map[string]any,
) (time.Time, time.Time) {
	t.Helper()

	validityRaw, ok := payload["validity"]
	if !ok {
		t.Fatal("fixture does not contain validity")
	}

	validity, ok := validityRaw.(map[string]any)
	if !ok {
		t.Fatalf(
			"fixture validity has unexpected type %T",
			validityRaw,
		)
	}

	nbRaw, ok := validity["not_before"].(string)
	if !ok || strings.TrimSpace(nbRaw) == "" {
		t.Fatal("fixture does not contain validity.not_before")
	}

	naRaw, ok := validity["not_after"].(string)
	if !ok || strings.TrimSpace(naRaw) == "" {
		t.Fatal("fixture does not contain validity.not_after")
	}

	notBefore, err := time.Parse(time.RFC3339, nbRaw)
	if err != nil {
		t.Fatalf(
			"parse validity.not_before %q: %v",
			nbRaw,
			err,
		)
	}

	notAfter, err := time.Parse(time.RFC3339, naRaw)
	if err != nil {
		t.Fatalf(
			"parse validity.not_after %q: %v",
			naRaw,
			err,
		)
	}

	return notBefore.UTC(), notAfter.UTC()
}

func insideValidityWindow(
	t *testing.T,
	payload map[string]any,
) time.Time {
	t.Helper()

	notBefore, notAfter := fixtureValidity(t, payload)

	if !notAfter.After(notBefore) {
		t.Fatalf(
			"invalid fixture validity window: %s -> %s",
			notBefore,
			notAfter,
		)
	}

	return notBefore.Add(notAfter.Sub(notBefore) / 2)
}

func beforeValidityWindow(
	t *testing.T,
	payload map[string]any,
) time.Time {
	t.Helper()

	notBefore, _ := fixtureValidity(t, payload)
	return notBefore.Add(-time.Second)
}

func afterValidityWindow(
	t *testing.T,
	payload map[string]any,
) time.Time {
	t.Helper()

	_, notAfter := fixtureValidity(t, payload)
	return notAfter.Add(time.Second)
}

func assertEvidenceResult(
	t *testing.T,
	name string,
	got credential.EvidenceResult,
	want credential.EvidenceResult,
) {
	t.Helper()

	if got != want {
		t.Fatalf(
			"%s result = %q, want %q",
			name,
			got,
			want,
		)
	}
}

func hasEvidenceError(
	evidence credential.NormalizedCredentialEvidence,
	code string,
) bool {
	for _, item := range evidence.Errors {
		if item.Code == code {
			return true
		}
	}

	return false
}
