package sdjwtvc

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/credential"
)

// ExtractEmploymentClaim converts the processed SD-JWT VC payload into
// Cvera's canonical EmploymentClaim.
//
// This file defines the first Cvera SD-JWT VC employment claim mapping.
//
// Credential-format metadata such as:
//
//   - iss
//   - vct
//   - kid
//   - _sd
//   - cnf
//   - status
//
// does NOT enter EmploymentClaim.
//
// Important:
//
// issuer identity and employment.employer_id are deliberately separate.
// Cvera MUST NOT assume that cryptographic issuer identity alone proves that
// issuer is authoritative to assert the employment relationship.
func ExtractEmploymentClaim(
	payload map[string]any,
) (credential.EmploymentClaim, string, error) {
	var zero credential.EmploymentClaim

	if payload == nil {
		return zero, "", errors.New(
			"processed SD-JWT VC payload is nil",
		)
	}

	vct, ok := stringClaim(payload, "vct")
	if !ok || strings.TrimSpace(vct) == "" {
		return zero, "", errors.New(
			"SD-JWT VC vct claim is required",
		)
	}

	employmentRaw, ok := payload["employment"]
	if !ok {
		return zero, vct, errors.New(
			"employment claim object is not disclosed",
		)
	}

	employment, ok := employmentRaw.(map[string]any)
	if !ok {
		return zero, vct, errors.New(
			"employment claim must be a JSON object",
		)
	}

	employerID, ok := stringClaim(
		employment,
		"employer_id",
	)
	if !ok || strings.TrimSpace(employerID) == "" {
		return zero, vct, errors.New(
			"employment.employer_id is required and must be disclosed",
		)
	}

	title, ok := stringClaim(
		employment,
		"title",
	)
	if !ok || strings.TrimSpace(title) == "" {
		return zero, vct, errors.New(
			"employment.title is required and must be disclosed",
		)
	}

	subjectID := ""

	if subjectRaw, ok := payload["subject"]; ok {
		subject, ok := subjectRaw.(map[string]any)
		if !ok {
			return zero, vct, errors.New(
				"subject claim must be a JSON object",
			)
		}

		if value, ok := stringClaim(subject, "id"); ok {
			subjectID = strings.TrimSpace(value)
		}
	}

	if subjectID == "" {
		if sub, ok := stringClaim(payload, "sub"); ok {
			subjectID = strings.TrimSpace(sub)
		}
	}

	employerName, err := optionalStringClaim(
		employment,
		"employer_name",
	)
	if err != nil {
		return zero, vct, err
	}

	level, err := optionalStringPointer(
		employment,
		"level",
	)
	if err != nil {
		return zero, vct, err
	}

	skill, err := optionalStringPointer(
		employment,
		"skill",
	)
	if err != nil {
		return zero, vct, err
	}

	startDate, err := optionalStringPointer(
		employment,
		"start_date",
	)
	if err != nil {
		return zero, vct, err
	}

	endDate, err := optionalStringPointer(
		employment,
		"end_date",
	)
	if err != nil {
		return zero, vct, err
	}

	return credential.EmploymentClaim{
		ClaimType: credential.ClaimTypeEmploymentRole,

		Subject: credential.EmploymentSubject{
			ID: subjectID,
		},

		Employment: credential.EmploymentRelationship{
			EmployerID:   employerID,
			EmployerName: employerName,
			Title:        title,

			Level: level,
			Skill: skill,

			StartDate: startDate,
			EndDate:   endDate,
		},
	}, vct, nil
}

func normalizeTemporalEvidence(
	out *credential.NormalizedCredentialEvidence,
	payload map[string]any,
	now time.Time,
) {
	issuedAt, iatPresent, iatErr := numericDate(
		payload,
		"iat",
	)

	notBefore, nbfPresent, nbfErr := numericDate(
		payload,
		"nbf",
	)

	expiresAt, expPresent, expErr := numericDate(
		payload,
		"exp",
	)

	if iatErr != nil || nbfErr != nil || expErr != nil {
		out.Validity.Result = credential.EvidenceInvalid

		addEvidenceError(
			out,
			"NUMERIC_DATE_INVALID",
			firstNonEmptyError(
				iatErr,
				nbfErr,
				expErr,
			),
			"",
		)

		return
	}

	if iatPresent {
		out.Validity.IssuedAt = &issuedAt
	}

	if nbfPresent {
		out.Validity.NotBefore = &notBefore
	}

	if expPresent {
		out.Validity.NotAfter = &expiresAt
	}

	if nbfPresent && now.Before(notBefore) {
		out.Validity.Result = credential.EvidenceInvalid

		addEvidenceError(
			out,
			"NOT_YET_VALID",
			"SD-JWT VC is not yet valid",
			"nbf",
		)

		return
	}

	// JWT exp semantics require current time to be strictly before exp.
	if expPresent && !now.Before(expiresAt) {
		out.Validity.Result = credential.EvidenceInvalid

		addEvidenceError(
			out,
			"EXPIRED",
			"SD-JWT VC has expired",
			"exp",
		)

		return
	}

	if nbfPresent || expPresent {
		out.Validity.Result = credential.EvidenceValid
		return
	}

	// iat alone records issuance time but does not establish a complete
	// temporal-validity window.
	out.Validity.Result = credential.EvidenceUnknown
}

func normalizeSubjectBindingEvidence(
	out *credential.NormalizedCredentialEvidence,
	payload map[string]any,
	parsed ParsedSDJWT,
) {
	_, hasCNF := payload["cnf"]

	switch {
	case parsed.KBJWT != "" && hasCNF:
		out.Subject = credential.SubjectBindingEvidence{
			Result: credential.EvidenceUnknown,
			Method: "sd-jwt-kb-present-unverified",
			SubjectID: out.Claim.Subject.ID,
		}

	case hasCNF:
		out.Subject = credential.SubjectBindingEvidence{
			Result: credential.EvidenceUnknown,
			Method: "sd-jwt-cnf-unverified",
			SubjectID: out.Claim.Subject.ID,
		}

	case out.Claim.Subject.ID != "":
		// A disclosed identifier is evidence about which subject the
		// credential names, but is not cryptographic holder binding.
		out.Subject = credential.SubjectBindingEvidence{
			Result: credential.EvidenceUnknown,
			Method: "subject-identifier-only",
			SubjectID: out.Claim.Subject.ID,
		}

	default:
		out.Subject = credential.SubjectBindingEvidence{
			Result: credential.EvidenceUnknown,
		}
	}
}

func numericDate(
	payload map[string]any,
	name string,
) (time.Time, bool, error) {
	raw, exists := payload[name]
	if !exists || raw == nil {
		return time.Time{}, false, nil
	}

	var seconds float64

	switch value := raw.(type) {
	case float64:
		seconds = value

	case float32:
		seconds = float64(value)

	case int:
		seconds = float64(value)

	case int64:
		seconds = float64(value)

	case json.Number:
		parsed, err := value.Float64()
		if err != nil {
			return time.Time{}, true, fmt.Errorf(
				"%s is not a valid NumericDate: %w",
				name,
				err,
			)
		}
		seconds = parsed

	case string:
		parsed, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return time.Time{}, true, fmt.Errorf(
				"%s is not a valid NumericDate",
				name,
			)
		}
		seconds = parsed

	default:
		return time.Time{}, true, fmt.Errorf(
			"%s has unsupported NumericDate type %T",
			name,
			raw,
		)
	}

	if math.IsNaN(seconds) ||
		math.IsInf(seconds, 0) {
		return time.Time{}, true, fmt.Errorf(
			"%s is not a finite NumericDate",
			name,
		)
	}

	whole, fraction := math.Modf(seconds)

	nanos := int64(
		fraction * float64(time.Second),
	)

	t := time.Unix(
		int64(whole),
		nanos,
	).UTC()

	return t, true, nil
}

func stringClaim(
	object map[string]any,
	name string,
) (string, bool) {
	raw, exists := object[name]
	if !exists || raw == nil {
		return "", false
	}

	value, ok := raw.(string)
	return value, ok
}

func optionalStringClaim(
	object map[string]any,
	name string,
) (string, error) {
	raw, exists := object[name]
	if !exists || raw == nil {
		return "", nil
	}

	value, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf(
			"%s must be a string",
			name,
		)
	}

	return strings.TrimSpace(value), nil
}

func optionalStringPointer(
	object map[string]any,
	name string,
) (*string, error) {
	raw, exists := object[name]
	if !exists || raw == nil {
		return nil, nil
	}

	value, ok := raw.(string)
	if !ok {
		return nil, fmt.Errorf(
			"employment.%s must be a string",
			name,
		)
	}

	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}

	return &value, nil
}

func firstNonEmptyError(
	errorsToCheck ...error,
) string {
	for _, err := range errorsToCheck {
		if err != nil {
			return err.Error()
		}
	}

	return "invalid NumericDate"
}