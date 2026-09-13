package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/credential"
)

func TestVerificationOutcome_ValidCryptoDoesNotImplyTrust(t *testing.T) {
	evidence := credential.NormalizedCredentialEvidence{
		ProfileID: "sd-jwt-vc",

		Issuer: credential.IssuerEvidence{
			Identifier: "https://issuer.example",
			KeyResolution: credential.KeyResolutionEvidence{
				Result: credential.EvidenceValid,
				Method: "jwt-vc-issuer-metadata",
				KeyID:  "issuer-key-1",
			},
		},

		Cryptographic: credential.CryptographicEvidence{
			Signature: credential.EvidenceValid,
			Algorithm: "RS256",
		},

		Validity: credential.ValidityEvidence{
			Result: credential.EvidenceValid,
		},

		Status: credential.StatusEvidence{
			Result: credential.EvidenceUnknown,
		},
	}

	got := mapEvidenceToVerificationOutcome(evidence)

	if got.Signature != "verified" {
		t.Fatalf(
			"Signature = %q, want verified",
			got.Signature,
		)
	}

	// Architectural regression lock:
	//
	// cryptographically valid evidence MUST NOT automatically become
	// trusted before issuer-authority and verifier-policy evaluation.
	if got.Trust != "unknown" {
		t.Fatalf(
			"Trust = %q, want unknown",
			got.Trust,
		)
	}

	if got.Why == nil {
		t.Fatal("Why = nil, want reason")
	}

	if got.Why.Code !=
		"EVIDENCE_VERIFIED_TRUST_NOT_EVALUATED" {
		t.Fatalf(
			"Why.Code = %q, want %q",
			got.Why.Code,
			"EVIDENCE_VERIFIED_TRUST_NOT_EVALUATED",
		)
	}

	if got.Checks == nil {
		t.Fatal("Checks = nil")
	}

	if got.Checks.ValidityWindow != "valid_now" {
		t.Fatalf(
			"ValidityWindow = %q, want valid_now",
			got.Checks.ValidityWindow,
		)
	}

	if got.Checks.Revocation != "unknown" {
		t.Fatalf(
			"Revocation = %q, want unknown",
			got.Checks.Revocation,
		)
	}
}

func TestVerificationOutcome_InvalidSignatureRemainsUntrusted(t *testing.T) {
	evidence := credential.NormalizedCredentialEvidence{
		ProfileID: "sd-jwt-vc",

		Issuer: credential.IssuerEvidence{
			Identifier: "https://issuer.example",
			KeyResolution: credential.KeyResolutionEvidence{
				Result: credential.EvidenceValid,
			},
		},

		Cryptographic: credential.CryptographicEvidence{
			Signature: credential.EvidenceInvalid,
			Algorithm: "RS256",
		},

		Validity: credential.ValidityEvidence{
			Result: credential.EvidenceUnknown,
		},

		Status: credential.StatusEvidence{
			Result: credential.EvidenceUnknown,
		},

		Errors: []credential.EvidenceError{
			{
				Code:    "BAD_SIGNATURE",
				Message: "issuer JWT signature is invalid",
			},
		},
	}

	got := mapEvidenceToVerificationOutcome(evidence)

	if got.Signature != "invalid" {
		t.Fatalf(
			"Signature = %q, want invalid",
			got.Signature,
		)
	}

	if got.Trust != "unknown" {
		t.Fatalf(
			"Trust = %q, want unknown",
			got.Trust,
		)
	}

	if got.Why == nil {
		t.Fatal("Why = nil, want BAD_SIGNATURE reason")
	}

	if got.Why.Code != "BAD_SIGNATURE" {
		t.Fatalf(
			"Why.Code = %q, want BAD_SIGNATURE",
			got.Why.Code,
		)
	}
}

func TestVerificationOutcome_ExpiredCredentialMapsValidityWindow(t *testing.T) {
	evidence := credential.NormalizedCredentialEvidence{
		ProfileID: "sd-jwt-vc",

		Issuer: credential.IssuerEvidence{
			KeyResolution: credential.KeyResolutionEvidence{
				Result: credential.EvidenceValid,
			},
		},

		Cryptographic: credential.CryptographicEvidence{
			Signature: credential.EvidenceValid,
		},

		Validity: credential.ValidityEvidence{
			Result: credential.EvidenceInvalid,
		},

		Status: credential.StatusEvidence{
			Result: credential.EvidenceUnknown,
		},

		Errors: []credential.EvidenceError{
			{
				Code:    "EXPIRED",
				Message: "SD-JWT VC has expired",
				Field:   "exp",
			},
		},
	}

	got := mapEvidenceToVerificationOutcome(evidence)

	if got.Signature != "verified" {
		t.Fatalf(
			"Signature = %q, want verified",
			got.Signature,
		)
	}

	if got.Trust != "unknown" {
		t.Fatalf(
			"Trust = %q, want unknown",
			got.Trust,
		)
	}

	if got.Checks == nil {
		t.Fatal("Checks = nil")
	}

	if got.Checks.ValidityWindow !=
		"not_valid_now" {
		t.Fatalf(
			"ValidityWindow = %q, want not_valid_now",
			got.Checks.ValidityWindow,
		)
	}

	if got.Why == nil ||
		got.Why.Code != "EXPIRED" {
		t.Fatalf(
			"Why = %+v, want EXPIRED",
			got.Why,
		)
	}
}

func TestVerificationOutcome_LegacyJWSReturnsExplicitUnknown(t *testing.T) {
	body := []byte(`{
		"jwsCompact":
		"eyJhbGciOiJSUzI1NiIsImtpZCI6ImxlZ2FjeS10ZXN0In0.eyJ0ZXN0IjoidmFsdWUifQ.signature"
	}`)

	req := httptest.NewRequest(
		http.MethodPost,
		"/v1/verification/outcome",
		bytes.NewReader(body),
	)

	rec := httptest.NewRecorder()

	var server Server

	server.HandleVerificationOutcome(
		rec,
		req,
	)

	if rec.Code != http.StatusOK {
		t.Fatalf(
			"status = %d, want %d; body=%s",
			rec.Code,
			http.StatusOK,
			rec.Body.String(),
		)
	}

	var got VerificationOutcome

	if err := json.Unmarshal(
		rec.Body.Bytes(),
		&got,
	); err != nil {
		t.Fatalf(
			"decode response: %v; body=%s",
			err,
			rec.Body.String(),
		)
	}

	if got.Signature != "unknown" {
		t.Fatalf(
			"Signature = %q, want unknown",
			got.Signature,
		)
	}

	if got.Trust != "unknown" {
		t.Fatalf(
			"Trust = %q, want unknown",
			got.Trust,
		)
	}

	if got.Why == nil {
		t.Fatal("Why = nil")
	}

	if got.Why.Code !=
		"LEGACY_VERIFIER_NOT_CONFIGURED" {
		t.Fatalf(
			"Why.Code = %q, want %q",
			got.Why.Code,
			"LEGACY_VERIFIER_NOT_CONFIGURED",
		)
	}
}

func TestVerificationOutcome_InvalidJSONReturnsBadRequest(t *testing.T) {
	req := httptest.NewRequest(
		http.MethodPost,
		"/v1/verification/outcome",
		bytes.NewBufferString(`{"jwsCompact":`),
	)

	rec := httptest.NewRecorder()

	var server Server

	server.HandleVerificationOutcome(
		rec,
		req,
	)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf(
			"status = %d, want %d; body=%s",
			rec.Code,
			http.StatusBadRequest,
			rec.Body.String(),
		)
	}
}

func TestVerificationOutcome_MissingCredentialReturnsBadRequest(t *testing.T) {
	req := httptest.NewRequest(
		http.MethodPost,
		"/v1/verification/outcome",
		bytes.NewBufferString(`{"jwsCompact":""}`),
	)

	rec := httptest.NewRecorder()

	var server Server

	server.HandleVerificationOutcome(
		rec,
		req,
	)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf(
			"status = %d, want %d; body=%s",
			rec.Code,
			http.StatusBadRequest,
			rec.Body.String(),
		)
	}
}

func TestVerificationOutcome_RejectsUnknownJSONField(t *testing.T) {
	req := httptest.NewRequest(
		http.MethodPost,
		"/v1/verification/outcome",
		bytes.NewBufferString(
			`{"jwsCompact":"abc.def.ghi","unexpected":"value"}`,
		),
	)

	rec := httptest.NewRecorder()

	var server Server

	server.HandleVerificationOutcome(
		rec,
		req,
	)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf(
			"status = %d, want %d; body=%s",
			rec.Code,
			http.StatusBadRequest,
			rec.Body.String(),
		)
	}
}

func TestVerificationOutcome_MethodGuard(t *testing.T) {
	req := httptest.NewRequest(
		http.MethodGet,
		"/v1/verification/outcome",
		nil,
	)

	rec := httptest.NewRecorder()

	var server Server

	server.HandleVerificationOutcome(
		rec,
		req,
	)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf(
			"status = %d, want %d",
			rec.Code,
			http.StatusMethodNotAllowed,
		)
	}
}
