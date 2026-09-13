package sdjwtvc

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/credential"
)

const (
	testSDJWTKID = "cvera-test-key-1"
	testVCT      = "https://cvera.app/credentials/employment-role/v1"
)

var testNow = time.Date(
	2026,
	time.September,
	13,
	20,
	0,
	0,
	0,
	time.UTC,
)

type testSDJWTVector struct {
	IssuerJWT string

	Disclosures map[string]string

	Compact string
}

func TestSDJWTVC_EndToEnd_MetadataSignatureDisclosureNormalization(
	t *testing.T,
) {
	signingKey := newSDJWTTestRSAKey(t)

	server, issuer := newIssuerMetadataTestServer(
		t,
		&signingKey.PublicKey,
		testSDJWTKID,
		"",
	)
	defer server.Close()

	vector := buildEmploymentSDJWTVector(
		t,
		issuer,
		signingKey,
		testNow,
	)

	resolver := &IssuerMetadataResolver{
		Client: server.Client(),
	}

	profile := New(
		resolver,
		WithExpectedVCT(testVCT),
	)

	got, err := profile.VerifyAndNormalize(
		context.Background(),
		[]byte(vector.Compact),
		credential.VerificationOptions{
			Now: testNow,
		},
	)
	if err != nil {
		t.Fatalf(
			"VerifyAndNormalize() error = %v",
			err,
		)
	}

	assertSDJWTEvidenceResult(
		t,
		"key resolution",
		got.Issuer.KeyResolution.Result,
		credential.EvidenceValid,
	)

	assertSDJWTEvidenceResult(
		t,
		"signature",
		got.Cryptographic.Signature,
		credential.EvidenceValid,
	)

	assertSDJWTEvidenceResult(
		t,
		"validity",
		got.Validity.Result,
		credential.EvidenceValid,
	)

	if got.Issuer.Identifier != issuer {
		t.Fatalf(
			"Issuer.Identifier = %q, want %q",
			got.Issuer.Identifier,
			issuer,
		)
	}

	if got.Issuer.KeyResolution.Method !=
		"jwt-vc-issuer-metadata" {
		t.Fatalf(
			"KeyResolution.Method = %q, want %q",
			got.Issuer.KeyResolution.Method,
			"jwt-vc-issuer-metadata",
		)
	}

	if got.Issuer.KeyResolution.KeyID !=
		testSDJWTKID {
		t.Fatalf(
			"KeyResolution.KeyID = %q, want %q",
			got.Issuer.KeyResolution.KeyID,
			testSDJWTKID,
		)
	}

	if got.Cryptographic.Algorithm != "RS256" {
		t.Fatalf(
			"Cryptographic.Algorithm = %q, want RS256",
			got.Cryptographic.Algorithm,
		)
	}

	if got.ClaimType !=
		credential.ClaimTypeEmploymentRole {
		t.Fatalf(
			"ClaimType = %q, want %q",
			got.ClaimType,
			credential.ClaimTypeEmploymentRole,
		)
	}

	if got.Claim.Subject.ID != "subject-123" {
		t.Fatalf(
			"Subject.ID = %q, want subject-123",
			got.Claim.Subject.ID,
		)
	}

	if got.Claim.Employment.EmployerID !=
		"employer-456" {
		t.Fatalf(
			"EmployerID = %q, want employer-456",
			got.Claim.Employment.EmployerID,
		)
	}

	if got.Claim.Employment.EmployerName !=
		"Example Corporation" {
		t.Fatalf(
			"EmployerName = %q, want Example Corporation",
			got.Claim.Employment.EmployerName,
		)
	}

	if got.Claim.Employment.Title !=
		"Staff Program Manager" {
		t.Fatalf(
			"Title = %q, want Staff Program Manager",
			got.Claim.Employment.Title,
		)
	}

	if got.Claim.Employment.Level == nil ||
		*got.Claim.Employment.Level != "Staff" {
		t.Fatalf(
			"Level = %v, want Staff",
			got.Claim.Employment.Level,
		)
	}

	if got.Claim.Employment.Skill == nil ||
		*got.Claim.Employment.Skill !=
			"Program Management" {
		t.Fatalf(
			"Skill = %v, want Program Management",
			got.Claim.Employment.Skill,
		)
	}

	// Subject identifier exists, but we have deliberately NOT performed
	// cryptographic holder/key binding yet.
	if got.Subject.Result !=
		credential.EvidenceUnknown {
		t.Fatalf(
			"Subject.Result = %q, want unknown",
			got.Subject.Result,
		)
	}

	if got.Subject.Method !=
		"subject-identifier-only" {
		t.Fatalf(
			"Subject.Method = %q, want subject-identifier-only",
			got.Subject.Method,
		)
	}

	// Status processing is deliberately Slice 2.
	if got.Status.Result !=
		credential.EvidenceUnknown {
		t.Fatalf(
			"Status.Result = %q, want unknown",
			got.Status.Result,
		)
	}

	assertContainsClaimPath(
		t,
		got.Disclosure.DisclosedClaims,
		"employment.employer_id",
	)

	assertContainsClaimPath(
		t,
		got.Disclosure.DisclosedClaims,
		"employment.title",
	)

	assertContainsClaimPath(
		t,
		got.Disclosure.DisclosedClaims,
		"employment.employer_name",
	)

	assertContainsClaimPath(
		t,
		got.Disclosure.DisclosedClaims,
		"employment.level",
	)

	assertContainsClaimPath(
		t,
		got.Disclosure.DisclosedClaims,
		"employment.skill",
	)

	if len(got.Errors) != 0 {
		t.Fatalf(
			"expected no evidence errors, got %+v",
			got.Errors,
		)
	}
}

func TestSDJWTVC_IssuerMetadataMismatchFailsKeyResolution(
	t *testing.T,
) {
	signingKey := newSDJWTTestRSAKey(t)

	server, issuer := newIssuerMetadataTestServer(
		t,
		&signingKey.PublicKey,
		testSDJWTKID,
		"https://wrong-issuer.example",
	)
	defer server.Close()

	vector := buildEmploymentSDJWTVector(
		t,
		issuer,
		signingKey,
		testNow,
	)

	profile := New(
		&IssuerMetadataResolver{
			Client: server.Client(),
		},
		WithExpectedVCT(testVCT),
	)

	got, err := profile.VerifyAndNormalize(
		context.Background(),
		[]byte(vector.Compact),
		credential.VerificationOptions{
			Now: testNow,
		},
	)
	if err != nil {
		t.Fatalf(
			"VerifyAndNormalize() error = %v",
			err,
		)
	}

	assertSDJWTEvidenceResult(
		t,
		"key resolution",
		got.Issuer.KeyResolution.Result,
		credential.EvidenceInvalid,
	)

	// Signature MUST NOT become valid when issuer metadata
	// correspondence failed.
	if got.Cryptographic.Signature ==
		credential.EvidenceValid {
		t.Fatal(
			"signature became valid despite issuer metadata mismatch",
		)
	}

	if !hasSDJWTEvidenceError(
		got,
		"KEY_RESOLUTION_FAILED",
	) {
		t.Fatalf(
			"expected KEY_RESOLUTION_FAILED, got %+v",
			got.Errors,
		)
	}
}

func TestSDJWTVC_InvalidIssuerSignatureProducesInvalidCryptoEvidence(
	t *testing.T,
) {
	trustedKey := newSDJWTTestRSAKey(t)
	untrustedSigningKey := newSDJWTTestRSAKey(t)

	server, issuer := newIssuerMetadataTestServer(
		t,
		&trustedKey.PublicKey,
		testSDJWTKID,
		"",
	)
	defer server.Close()

	vector := buildEmploymentSDJWTVector(
		t,
		issuer,
		untrustedSigningKey,
		testNow,
	)

	profile := New(
		&IssuerMetadataResolver{
			Client: server.Client(),
		},
		WithExpectedVCT(testVCT),
	)

	got, err := profile.VerifyAndNormalize(
		context.Background(),
		[]byte(vector.Compact),
		credential.VerificationOptions{
			Now: testNow,
		},
	)
	if err != nil {
		t.Fatalf(
			"VerifyAndNormalize() error = %v",
			err,
		)
	}

	// Metadata/JWKS resolution itself succeeded.
	assertSDJWTEvidenceResult(
		t,
		"key resolution",
		got.Issuer.KeyResolution.Result,
		credential.EvidenceValid,
	)

	// But the issuer JWT was signed by another key.
	assertSDJWTEvidenceResult(
		t,
		"signature",
		got.Cryptographic.Signature,
		credential.EvidenceInvalid,
	)

	if !hasSDJWTEvidenceError(
		got,
		"BAD_SIGNATURE",
	) {
		t.Fatalf(
			"expected BAD_SIGNATURE, got %+v",
			got.Errors,
		)
	}

	// Failed crypto MUST prevent claim normalization.
	if got.Claim.Employment.Title != "" {
		t.Fatalf(
			"claim normalized despite invalid signature: %+v",
			got.Claim,
		)
	}
}

func TestSDJWTVC_TamperedDisclosureIsRejected(
	t *testing.T,
) {
	signingKey := newSDJWTTestRSAKey(t)

	server, issuer := newIssuerMetadataTestServer(
		t,
		&signingKey.PublicKey,
		testSDJWTKID,
		"",
	)
	defer server.Close()

	vector := buildEmploymentSDJWTVector(
		t,
		issuer,
		signingKey,
		testNow,
	)

	// Create a new disclosure for the same logical property but with a
	// different value. Its digest is therefore different from the digest
	// cryptographically committed into the issuer JWT.
	tamperedTitle, _ := makeObjectDisclosure(
		t,
		"tampered-salt",
		"title",
		"Chief Executive Officer",
	)

	disclosures := orderedDisclosures(
		vector,
	)

	for i, disclosure := range disclosures {
		if disclosure ==
			vector.Disclosures["title"] {
			disclosures[i] = tamperedTitle
			break
		}
	}

	tamperedCompact :=
		vector.IssuerJWT +
			"~" +
			strings.Join(disclosures, "~") +
			"~"

	profile := New(
		&IssuerMetadataResolver{
			Client: server.Client(),
		},
		WithExpectedVCT(testVCT),
	)

	got, err := profile.VerifyAndNormalize(
		context.Background(),
		[]byte(tamperedCompact),
		credential.VerificationOptions{
			Now: testNow,
		},
	)
	if err != nil {
		t.Fatalf(
			"VerifyAndNormalize() error = %v",
			err,
		)
	}

	// The issuer JWT itself remains cryptographically valid.
	assertSDJWTEvidenceResult(
		t,
		"signature",
		got.Cryptographic.Signature,
		credential.EvidenceValid,
	)

	// But the supplied disclosure cannot be matched to a digest committed
	// by the signed SD-JWT.
	if !hasSDJWTEvidenceError(
		got,
		"DISCLOSURE_INVALID",
	) {
		t.Fatalf(
			"expected DISCLOSURE_INVALID, got %+v",
			got.Errors,
		)
	}

	if got.Claim.Employment.Title != "" {
		t.Fatalf(
			"tampered disclosure produced normalized claim: %+v",
			got.Claim,
		)
	}
}

func TestSDJWTVC_UndisclosedRequiredEmploymentClaimDoesNotNormalize(
	t *testing.T,
) {
	signingKey := newSDJWTTestRSAKey(t)

	server, issuer := newIssuerMetadataTestServer(
		t,
		&signingKey.PublicKey,
		testSDJWTKID,
		"",
	)
	defer server.Close()

	vector := buildEmploymentSDJWTVector(
		t,
		issuer,
		signingKey,
		testNow,
	)

	var presented []string

	for _, disclosure := range orderedDisclosures(
		vector,
	) {
		if disclosure ==
			vector.Disclosures["title"] {
			continue
		}

		presented = append(
			presented,
			disclosure,
		)
	}

	compact :=
		vector.IssuerJWT +
			"~" +
			strings.Join(presented, "~") +
			"~"

	profile := New(
		&IssuerMetadataResolver{
			Client: server.Client(),
		},
		WithExpectedVCT(testVCT),
	)

	got, err := profile.VerifyAndNormalize(
		context.Background(),
		[]byte(compact),
		credential.VerificationOptions{
			Now: testNow,
		},
	)
	if err != nil {
		t.Fatalf(
			"VerifyAndNormalize() error = %v",
			err,
		)
	}

	assertSDJWTEvidenceResult(
		t,
		"signature",
		got.Cryptographic.Signature,
		credential.EvidenceValid,
	)

	// Non-presentation of a disclosure is not itself cryptographic
	// corruption. But title is required by Cvera's EmploymentClaim
	// normalization contract, so the domain claim cannot be constructed.
	if !hasSDJWTEvidenceError(
		got,
		"CLAIM_INVALID",
	) {
		t.Fatalf(
			"expected CLAIM_INVALID, got %+v",
			got.Errors,
		)
	}

	if got.Claim.Employment.Title != "" {
		t.Fatalf(
			"required undisclosed title unexpectedly normalized: %+v",
			got.Claim,
		)
	}
}

func TestSDJWTVC_ExpiredCredentialProducesInvalidTemporalEvidence(
	t *testing.T,
) {
	signingKey := newSDJWTTestRSAKey(t)

	server, issuer := newIssuerMetadataTestServer(
		t,
		&signingKey.PublicKey,
		testSDJWTKID,
		"",
	)
	defer server.Close()

	vector := buildEmploymentSDJWTVector(
		t,
		issuer,
		signingKey,
		testNow,
	)

	profile := New(
		&IssuerMetadataResolver{
			Client: server.Client(),
		},
		WithExpectedVCT(testVCT),
	)

	got, err := profile.VerifyAndNormalize(
		context.Background(),
		[]byte(vector.Compact),
		credential.VerificationOptions{
			Now: testNow.Add(3 * time.Hour),
		},
	)
	if err != nil {
		t.Fatalf(
			"VerifyAndNormalize() error = %v",
			err,
		)
	}

	assertSDJWTEvidenceResult(
		t,
		"signature",
		got.Cryptographic.Signature,
		credential.EvidenceValid,
	)

	assertSDJWTEvidenceResult(
		t,
		"validity",
		got.Validity.Result,
		credential.EvidenceInvalid,
	)

	if !hasSDJWTEvidenceError(
		got,
		"EXPIRED",
	) {
		t.Fatalf(
			"expected EXPIRED, got %+v",
			got.Errors,
		)
	}
}

func TestSDJWTVC_WrongVCTIsRejectedByProfile(
	t *testing.T,
) {
	signingKey := newSDJWTTestRSAKey(t)

	server, issuer := newIssuerMetadataTestServer(
		t,
		&signingKey.PublicKey,
		testSDJWTKID,
		"",
	)
	defer server.Close()

	vector := buildEmploymentSDJWTVector(
		t,
		issuer,
		signingKey,
		testNow,
	)

	profile := New(
		&IssuerMetadataResolver{
			Client: server.Client(),
		},
		WithExpectedVCT(
			"https://cvera.app/credentials/different-type/v1",
		),
	)

	got, err := profile.VerifyAndNormalize(
		context.Background(),
		[]byte(vector.Compact),
		credential.VerificationOptions{
			Now: testNow,
		},
	)
	if err != nil {
		t.Fatalf(
			"VerifyAndNormalize() error = %v",
			err,
		)
	}

	assertSDJWTEvidenceResult(
		t,
		"signature",
		got.Cryptographic.Signature,
		credential.EvidenceValid,
	)

	if !hasSDJWTEvidenceError(
		got,
		"VCT_UNSUPPORTED",
	) {
		t.Fatalf(
			"expected VCT_UNSUPPORTED, got %+v",
			got.Errors,
		)
	}
}

func buildEmploymentSDJWTVector(
	t *testing.T,
	issuer string,
	signingKey *rsa.PrivateKey,
	now time.Time,
) testSDJWTVector {
	t.Helper()

	employerID, employerIDDigest :=
		makeObjectDisclosure(
			t,
			"salt-employer-id",
			"employer_id",
			"employer-456",
		)

	employerName, employerNameDigest :=
		makeObjectDisclosure(
			t,
			"salt-employer-name",
			"employer_name",
			"Example Corporation",
		)

	title, titleDigest :=
		makeObjectDisclosure(
			t,
			"salt-title",
			"title",
			"Staff Program Manager",
		)

	level, levelDigest :=
		makeObjectDisclosure(
			t,
			"salt-level",
			"level",
			"Staff",
		)

	skill, skillDigest :=
		makeObjectDisclosure(
			t,
			"salt-skill",
			"skill",
			"Program Management",
		)

	startDate, startDateDigest :=
		makeObjectDisclosure(
			t,
			"salt-start-date",
			"start_date",
			"2022-01-01",
		)

	endDate, endDateDigest :=
		makeObjectDisclosure(
			t,
			"salt-end-date",
			"end_date",
			"2026-08-31",
		)

	payload := jwt.MapClaims{
		"iss": issuer,

		"vct": testVCT,

		"jti": "cvera-test-credential-001",

		"iat": now.Add(
			-1 * time.Hour,
		).Unix(),

		"nbf": now.Add(
			-30 * time.Minute,
		).Unix(),

		"exp": now.Add(
			2 * time.Hour,
		).Unix(),

		// Identifier is deliberately visible in this first vector.
		// Holder/key binding is a later slice.
		"sub": "subject-123",

		"_sd_alg": "sha-256",

		"employment": map[string]any{
			"_sd": []any{
				employerIDDigest,
				employerNameDigest,
				titleDigest,
				levelDigest,
				skillDigest,
				startDateDigest,
				endDateDigest,
			},
		},
	}

	token := jwt.NewWithClaims(
		jwt.SigningMethodRS256,
		payload,
	)

	token.Header["typ"] = "dc+sd-jwt"
	token.Header["kid"] = testSDJWTKID

	issuerJWT, err := token.SignedString(
		signingKey,
	)
	if err != nil {
		t.Fatalf(
			"sign issuer JWT: %v",
			err,
		)
	}

	disclosures := map[string]string{
		"employer_id":   employerID,
		"employer_name": employerName,
		"title":         title,
		"level":         level,
		"skill":         skill,
		"start_date":    startDate,
		"end_date":      endDate,
	}

	ordered := []string{
		employerID,
		employerName,
		title,
		level,
		skill,
		startDate,
		endDate,
	}

	return testSDJWTVector{
		IssuerJWT:   issuerJWT,
		Disclosures: disclosures,

		Compact: issuerJWT +
			"~" +
			strings.Join(ordered, "~") +
			"~",
	}
}

func orderedDisclosures(
	vector testSDJWTVector,
) []string {
	return []string{
		vector.Disclosures["employer_id"],
		vector.Disclosures["employer_name"],
		vector.Disclosures["title"],
		vector.Disclosures["level"],
		vector.Disclosures["skill"],
		vector.Disclosures["start_date"],
		vector.Disclosures["end_date"],
	}
}

// makeObjectDisclosure builds an RFC 9901 object-property Disclosure:
//
//	[salt, claim-name, claim-value]
//
// The digest is calculated by the production disclosureDigest()
// implementation over the base64url-encoded Disclosure itself.
func makeObjectDisclosure(
	t *testing.T,
	salt string,
	name string,
	value any,
) (encoded string, digest string) {
	t.Helper()

	raw, err := json.Marshal(
		[]any{
			salt,
			name,
			value,
		},
	)
	if err != nil {
		t.Fatalf(
			"marshal disclosure %q: %v",
			name,
			err,
		)
	}

	encoded = base64.RawURLEncoding.EncodeToString(
		raw,
	)

	digest, err = disclosureDigest(
		encoded,
		"sha-256",
	)
	if err != nil {
		t.Fatalf(
			"digest disclosure %q: %v",
			name,
			err,
		)
	}

	return encoded, digest
}

func newIssuerMetadataTestServer(
	t *testing.T,
	publicKey *rsa.PublicKey,
	kid string,
	metadataIssuerOverride string,
) (*httptest.Server, string) {
	t.Helper()

	jwks := testJWKS(
		t,
		publicKey,
		kid,
	)

	var issuer string

	handler := http.HandlerFunc(
		func(
			w http.ResponseWriter,
			r *http.Request,
		) {
			switch r.URL.Path {
			case "/.well-known/jwt-vc-issuer/issuers/acme":
				metadataIssuer := issuer

				if metadataIssuerOverride != "" {
					metadataIssuer =
						metadataIssuerOverride
				}

				writeTestJSON(
					t,
					w,
					map[string]any{
						"issuer": metadataIssuer,
						"jwks_uri": strings.TrimSuffix(
							issuer,
							"/issuers/acme",
						) + "/jwks.json",
					},
				)

			case "/jwks.json":
				writeTestJSON(
					t,
					w,
					jwks,
				)

			default:
				http.NotFound(
					w,
					r,
				)
			}
		},
	)

	server := httptest.NewTLSServer(
		handler,
	)

	issuer = server.URL +
		"/issuers/acme"

	return server, issuer
}

func testJWKS(
	t *testing.T,
	publicKey *rsa.PublicKey,
	kid string,
) map[string]any {
	t.Helper()

	if publicKey == nil ||
		publicKey.N == nil {
		t.Fatal(
			"test RSA public key is nil",
		)
	}

	return map[string]any{
		"keys": []any{
			map[string]any{
				"kty": "RSA",
				"kid": kid,
				"use": "sig",
				"alg": "RS256",

				"n": base64.RawURLEncoding.EncodeToString(
					publicKey.N.Bytes(),
				),

				"e": base64.RawURLEncoding.EncodeToString(
					rsaExponentBytes(
						publicKey.E,
					),
				),
			},
		},
	}
}

func rsaExponentBytes(
	exponent int,
) []byte {
	if exponent == 0 {
		return []byte{0}
	}

	var reversed []byte

	for exponent > 0 {
		reversed = append(
			reversed,
			byte(exponent&0xff),
		)

		exponent >>= 8
	}

	out := make(
		[]byte,
		len(reversed),
	)

	for i := range reversed {
		out[len(reversed)-1-i] =
			reversed[i]
	}

	return out
}

func writeTestJSON(
	t *testing.T,
	w http.ResponseWriter,
	value any,
) {
	t.Helper()

	w.Header().Set(
		"Content-Type",
		"application/json",
	)

	if err := json.NewEncoder(w).Encode(
		value,
	); err != nil {
		t.Errorf(
			"encode HTTP test response: %v",
			err,
		)
	}
}

func newSDJWTTestRSAKey(
	t *testing.T,
) *rsa.PrivateKey {
	t.Helper()

	key, err := rsa.GenerateKey(
		rand.Reader,
		2048,
	)
	if err != nil {
		t.Fatalf(
			"rsa.GenerateKey() error = %v",
			err,
		)
	}

	return key
}

func assertSDJWTEvidenceResult(
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

func hasSDJWTEvidenceError(
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

func assertContainsClaimPath(
	t *testing.T,
	paths []string,
	want string,
) {
	t.Helper()

	for _, path := range paths {
		if path == want {
			return
		}
	}

	t.Fatalf(
		"disclosed claims %v do not contain %q",
		paths,
		want,
	)
}
