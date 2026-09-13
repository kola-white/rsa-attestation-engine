package httpapi

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/credential"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/credential/sdjwtvc"
)

type VerificationOutcome struct {
	Signature string `json:"signature"` // "verified" | "invalid" | "unknown"
	Trust     string `json:"trust"`     // "trusted" | "untrusted" | "unknown"

	Why *struct {
		Summary string `json:"summary"`
		Code    string `json:"code"`
	} `json:"why,omitempty"`

	Checks *struct {
		ValidityWindow string `json:"validity_window"` // "valid_now" | "not_valid_now" | "unknown"
		Revocation     string `json:"revocation"`      // "not_revoked" | "revoked" | "unknown"
	} `json:"checks,omitempty"`
}

type verifyOutcomeRequest struct {
	// Preserve the existing client contract for now.
	//
	// Despite the historical field name, this may now contain either:
	//
	//   legacy compact JWS
	//   SD-JWT VC compact serialization
	//
	// A future API revision can rename this to credential without breaking
	// the current Expo client during the migration.
	JWSCompact string `json:"jwsCompact"`
}

func (s *Server) HandleVerificationOutcome(
	w http.ResponseWriter,
	r *http.Request,
) {
	if r.Method != http.MethodPost {
		http.Error(
			w,
			"method not allowed",
			http.StatusMethodNotAllowed,
		)
		return
	}

	// Defensive request limit.
	r.Body = http.MaxBytesReader(
		w,
		r.Body,
		1<<20,
	)

	var req verifyOutcomeRequest

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(&req); err != nil {
		writeErr(
			w,
			http.StatusBadRequest,
			"invalid_json",
		)
		return
	}

	rawCredential := strings.TrimSpace(
		req.JWSCompact,
	)

	if rawCredential == "" {
		writeErr(
			w,
			http.StatusBadRequest,
			"missing_jws_compact",
		)
		return
	}

	out := verifyCredentialOutcome(
		r,
		rawCredential,
	)

	// Do not log credential contents.
	log.Printf(
		"[verify:outcome] credential_len=%d signature=%s trust=%s why=%s",
		len(rawCredential),
		out.Signature,
		out.Trust,
		out.whyCode(),
	)

	writeJSON(
		w,
		http.StatusOK,
		out,
	)
}

func verifyCredentialOutcome(
	r *http.Request,
	rawCredential string,
) VerificationOutcome {
	if looksLikeSDJWTVC(rawCredential) {
		return verifySDJWTVCOutcome(
			r,
			rawCredential,
		)
	}

	// The legacy adapter exists and is behaviorally tested, but runtime
	// legacy JWKS/status resolution has intentionally not been wired into
	// this HTTP boundary yet.
	//
	// Do NOT silently treat a syntactically valid legacy JWS as verified.
	return newVerificationOutcome(
		"unknown",
		"unknown",
		"LEGACY_VERIFIER_NOT_CONFIGURED",
		"Legacy JWS compatibility verification is not configured on this API boundary.",
		"unknown",
		"unknown",
	)
}

func verifySDJWTVCOutcome(
	r *http.Request,
	rawCredential string,
) VerificationOutcome {
	resolver := &sdjwtvc.IssuerMetadataResolver{}

	profile := sdjwtvc.New(
		resolver,
	)

	evidence, err := profile.VerifyAndNormalize(
		r.Context(),
		[]byte(rawCredential),
		credential.VerificationOptions{
			Now: time.Now().UTC(),
		},
	)

	if err != nil {
		log.Printf(
			"[verify:outcome] sd-jwt-vc operational_error=%v",
			err,
		)

		return newVerificationOutcome(
			"unknown",
			"unknown",
			"SERVER_VERIFICATION_ERROR",
			"Credential verification could not be completed.",
			"unknown",
			"unknown",
		)
	}

	return mapEvidenceToVerificationOutcome(
		evidence,
	)
}

func mapEvidenceToVerificationOutcome(
	evidence credential.NormalizedCredentialEvidence,
) VerificationOutcome {
	signature := mapSignatureEvidence(
		evidence.Cryptographic.Signature,
	)

	validity := mapValidityEvidence(
		evidence.Validity.Result,
	)

	revocation := mapStatusEvidence(
		evidence.Status.Result,
	)

	// IMPORTANT:
	//
	// This endpoint currently exposes credential evidence only.
	//
	// Successful cryptographic verification does NOT establish:
	//
	//   issuer authority
	//   verifier trust-policy satisfaction
	//   Cvera VERIFIED
	//
	// Therefore Trust MUST remain unknown until the Cvera trust engine is
	// wired above NormalizedCredentialEvidence.
	trust := "unknown"

	code, summary := evidenceOutcomeReason(
		evidence,
	)

	return newVerificationOutcome(
		signature,
		trust,
		code,
		summary,
		validity,
		revocation,
	)
}

func evidenceOutcomeReason(
	evidence credential.NormalizedCredentialEvidence,
) (string, string) {
	if len(evidence.Errors) > 0 {
		first := evidence.Errors[0]

		code := strings.TrimSpace(
			first.Code,
		)

		if code == "" {
			code = "CREDENTIAL_EVIDENCE_INVALID"
		}

		summary := strings.TrimSpace(
			first.Message,
		)

		if summary == "" {
			summary =
				"Credential evidence could not be validated."
		}

		return code, summary
	}

	switch {
	case evidence.Cryptographic.Signature ==
		credential.EvidenceInvalid:

		return "BAD_SIGNATURE",
			"Credential signature is invalid."

	case evidence.Issuer.KeyResolution.Result ==
		credential.EvidenceInvalid:

		return "KEY_RESOLUTION_FAILED",
			"Issuer verification key could not be validated."

	case evidence.Validity.Result ==
		credential.EvidenceInvalid:

		return "NOT_VALID_NOW",
			"Credential is outside its valid time window."

	case evidence.Status.Result ==
		credential.EvidenceInvalid:

		return "CREDENTIAL_STATUS_INVALID",
			"Credential status indicates that it is not currently valid."

	case evidence.Cryptographic.Signature ==
		credential.EvidenceValid &&
		evidence.Issuer.KeyResolution.Result ==
			credential.EvidenceValid:

		return "EVIDENCE_VERIFIED_TRUST_NOT_EVALUATED",
			"Credential evidence is cryptographically verified; issuer authority and verifier trust policy have not yet been evaluated."

	default:
		return "EVIDENCE_INCOMPLETE",
			"Credential evidence could not be fully evaluated."
	}
}

func mapSignatureEvidence(
	result credential.EvidenceResult,
) string {
	switch result {
	case credential.EvidenceValid:
		return "verified"

	case credential.EvidenceInvalid:
		return "invalid"

	default:
		return "unknown"
	}
}

func mapValidityEvidence(
	result credential.EvidenceResult,
) string {
	switch result {
	case credential.EvidenceValid:
		return "valid_now"

	case credential.EvidenceInvalid:
		return "not_valid_now"

	default:
		return "unknown"
	}
}

func mapStatusEvidence(
	result credential.EvidenceResult,
) string {
	switch result {
	case credential.EvidenceValid:
		return "not_revoked"

	case credential.EvidenceInvalid:
		return "revoked"

	default:
		return "unknown"
	}
}

func looksLikeSDJWTVC(
	raw string,
) bool {
	// RFC 9901 SD-JWT compact serialization uses "~" separators.
	//
	// This is intentionally only a dispatch heuristic. The sdjwtvc profile
	// remains authoritative for actual parsing and validation.
	return strings.Contains(
		raw,
		"~",
	)
}

func newVerificationOutcome(
	signature string,
	trust string,
	code string,
	summary string,
	validityWindow string,
	revocation string,
) VerificationOutcome {
	out := VerificationOutcome{
		Signature: signature,
		Trust:     trust,

		Checks: &struct {
			ValidityWindow string `json:"validity_window"`
			Revocation     string `json:"revocation"`
		}{
			ValidityWindow: validityWindow,
			Revocation:     revocation,
		},
	}

	if code != "" || summary != "" {
		out.Why = &struct {
			Summary string `json:"summary"`
			Code    string `json:"code"`
		}{
			Summary: summary,
			Code:    code,
		}
	}

	return out
}

func (o VerificationOutcome) whyCode() string {
	if o.Why == nil {
		return ""
	}

	return o.Why.Code
}
