package legacyjws

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/credential"
)

// ap1Payload represents only the legacy AP-1 fields required to normalize an
// employment.role attestation.
//
// It is intentionally not exported and is NOT Cvera's canonical domain
// model. It exists only inside the legacy credential adapter.
type ap1Payload struct {
	ID        string `json:"id"`
	RequestID string `json:"request_id"`

	SchemaURI string `json:"schema_uri"`
	Version   string `json:"version"`

	Issuer struct {
		// AP-1 documentation uses issuer.id, while an older JSON schema /
		// fixture variant used issuer.request_id. Support both during legacy
		// migration.
		ID        string `json:"id"`
		RequestID string `json:"request_id"`
		Name      string `json:"name"`
	} `json:"issuer"`

	Key struct {
		KID string `json:"kid"`
		Alg string `json:"alg"`
	} `json:"key"`

	Subject struct {
		Binding struct {
			Type             string `json:"type"`
			PubKeyThumbprint string `json:"pubkey_thumbprint"`
			Identifier       string `json:"identifier"`
		} `json:"binding"`
	} `json:"subject"`

	Claim struct {
		Type    string         `json:"type"`
		Context map[string]any `json:"context"`

		Value struct {
			Title string `json:"title"`
			Skill string `json:"skill"`
			Level string `json:"level"`
		} `json:"value"`
	} `json:"claim"`

	Validity struct {
		IssuedAt  string `json:"issued_at"`
		NotBefore string `json:"not_before"`
		NotAfter  string `json:"not_after"`
	} `json:"validity"`

	Revocation struct {
		Method  string `json:"method"`
		Pointer string `json:"pointer"`
		Serial  string `json:"serial"`
	} `json:"revocation"`

	Policy struct {
		PolicyURI string `json:"policy_uri"`
		Assurance string `json:"assurance"`
	} `json:"policy"`

	Disclosure struct {
		Mode            string   `json:"mode"`
		DisclosedFields []string `json:"disclosed_fields"`
	} `json:"disclosure"`

	Hash struct {
		PayloadAlg  string `json:"payload_alg"`
		PayloadHash string `json:"payload_hash"`
	} `json:"hash"`

	Signature struct {
		Mode   string `json:"mode"`
		SigAlg string `json:"sig_alg"`
		Sig    string `json:"sig"`
	} `json:"signature"`
}

func isAP1(p ap1Payload) bool {
	return p.SchemaURI == "schema/employment.role/v1" ||
		p.Claim.Type == credential.ClaimTypeEmploymentRole
}

func (p *Profile) normalizeAP1(
	ctx context.Context,
	out credential.NormalizedCredentialEvidence,
	src ap1Payload,
	opts credential.VerificationOptions,
) (credential.NormalizedCredentialEvidence, error) {
	out.CredentialID = strings.TrimSpace(src.ID)
	if out.CredentialID == "" {
		out.CredentialID = strings.TrimSpace(src.RequestID)
	}

	out.ClaimType = src.Claim.Type
	if out.ClaimType == "" {
		out.ClaimType = credential.ClaimTypeEmploymentRole
	}

	issuerID := strings.TrimSpace(src.Issuer.ID)
	if issuerID == "" {
		issuerID = strings.TrimSpace(src.Issuer.RequestID)
	}
	out.Issuer.Identifier = issuerID

	if src.Key.KID != "" &&
		out.Issuer.KeyResolution.KeyID != "" &&
		src.Key.KID != out.Issuer.KeyResolution.KeyID {
		addEvidenceError(
			&out,
			"KID_MISMATCH",
			"payload key.kid does not match protected-header kid",
			"key.kid",
		)
	}

	if src.Key.Alg != "" && src.Key.Alg != "RS256" {
		addEvidenceError(
			&out,
			"KEY_ALG_INVALID",
			"legacy AP-1 key.alg must be RS256",
			"key.alg",
		)
	}

	if src.SchemaURI != "schema/employment.role/v1" {
		addEvidenceError(
			&out,
			"SCHEMA_URI_INVALID",
			"legacy AP-1 schema_uri is not schema/employment.role/v1",
			"schema_uri",
		)
	}

	if src.Claim.Type != credential.ClaimTypeEmploymentRole {
		addEvidenceError(
			&out,
			"CLAIM_TYPE_INVALID",
			"legacy AP-1 claim.type is not employment.role",
			"claim.type",
		)
	}

	if strings.TrimSpace(src.Claim.Value.Title) == "" {
		addEvidenceError(
			&out,
			"SCHEMA_INVALID",
			"legacy AP-1 claim.value.title is required",
			"claim.value.title",
		)
	}

	if src.Disclosure.Mode != "full" {
		addEvidenceError(
			&out,
			"DISCLOSURE_MODE_INVALID",
			"legacy AP-1 disclosure.mode must be full",
			"disclosure.mode",
		)
	}

	if src.Hash.PayloadAlg != "SHA-256" ||
		strings.TrimSpace(src.Hash.PayloadHash) == "" {
		addEvidenceError(
			&out,
			"HASH_INVALID",
			"legacy AP-1 hash must use SHA-256 with a non-empty payload_hash",
			"hash",
		)
	}

	out.Claim = credential.EmploymentClaim{
		ClaimType: credential.ClaimTypeEmploymentRole,
		Subject: credential.EmploymentSubject{
			ID: strings.TrimSpace(src.Subject.Binding.Identifier),
		},
		Employment: credential.EmploymentRelationship{
			EmployerID:   issuerID,
			EmployerName: src.Issuer.Name,
			Title:        src.Claim.Value.Title,
			Level:        optionalString(src.Claim.Value.Level),
			Skill:        optionalString(src.Claim.Value.Skill),
		},
	}

	out.Subject = normalizeAP1SubjectBinding(src)

	issuedAt, issuedErr := parseOptionalRFC3339(src.Validity.IssuedAt)
	notBefore, nbErr := parseOptionalRFC3339(src.Validity.NotBefore)
	notAfter, naErr := parseOptionalRFC3339(src.Validity.NotAfter)

	out.Validity.IssuedAt = issuedAt
	out.Validity.NotBefore = notBefore
	out.Validity.NotAfter = notAfter

	if issuedErr != nil || nbErr != nil || naErr != nil {
		out.Validity.Result = credential.EvidenceInvalid

		addEvidenceError(
			&out,
			"VALIDITY_FORMAT_INVALID",
			"legacy AP-1 validity timestamps must be RFC3339",
			"validity",
		)
	} else {
		out.Validity.Result = evaluateValidity(
			effectiveNow(opts),
			notBefore,
			notAfter,
		)

		if out.Validity.Result == credential.EvidenceInvalid {
			addEvidenceError(
				&out,
				"NOT_VALID_NOW",
				"legacy AP-1 credential is outside its validity window",
				"validity",
			)
		}
	}

	out.Status.Method = src.Revocation.Method
	out.Status.Reference = src.Revocation.Pointer

	switch {
	case strings.TrimSpace(src.Revocation.Serial) == "":
		out.Status.Result = credential.EvidenceUnknown
		addEvidenceError(
			&out,
			"STATUS_SERIAL_MISSING",
			"legacy AP-1 revocation serial is missing",
			"revocation.serial",
		)

	case p.status == nil:
		out.Status.Result = credential.EvidenceUnknown

	default:
		result, err := p.status.ResolveStatus(
			ctx,
			src.Revocation.Method,
			src.Revocation.Pointer,
			src.Revocation.Serial,
		)
		if err != nil {
			out.Status.Result = credential.EvidenceUnknown
			addEvidenceError(
				&out,
				"STATUS_RESOLUTION_FAILED",
				err.Error(),
				"revocation",
			)
		} else {
			out.Status.Result = result
		}
	}

	out.Disclosure.DisclosedClaims = append(
		[]string(nil),
		src.Disclosure.DisclosedFields...,
	)

	return out, nil
}

func normalizeAP1SubjectBinding(
	src ap1Payload,
) credential.SubjectBindingEvidence {
	binding := src.Subject.Binding

	out := credential.SubjectBindingEvidence{
		Result:    credential.EvidenceUnknown,
		Method:    binding.Type,
		SubjectID: binding.Identifier,
	}

	switch binding.Type {
	case "identifier":
		if strings.TrimSpace(binding.Identifier) != "" {
			out.Result = credential.EvidenceValid
		} else {
			out.Result = credential.EvidenceInvalid
		}

	case "pubkey":
		if strings.TrimSpace(binding.PubKeyThumbprint) != "" {
			out.Result = credential.EvidenceValid
		} else {
			out.Result = credential.EvidenceInvalid
		}

	case "both":
		if strings.TrimSpace(binding.Identifier) != "" &&
			strings.TrimSpace(binding.PubKeyThumbprint) != "" {
			out.Result = credential.EvidenceValid
		} else {
			out.Result = credential.EvidenceInvalid
		}

	default:
		out.Result = credential.EvidenceUnknown
	}

	return out
}

func evaluateValidity(
	now time.Time,
	notBefore *time.Time,
	notAfter *time.Time,
) credential.EvidenceResult {
	if notBefore == nil || notAfter == nil {
		return credential.EvidenceUnknown
	}

	if now.Before(*notBefore) {
		return credential.EvidenceInvalid
	}

	if now.After(*notAfter) {
		return credential.EvidenceInvalid
	}

	return credential.EvidenceValid
}

func parseOptionalRFC3339(v string) (*time.Time, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil, nil
	}

	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return nil, err
	}

	t = t.UTC()
	return &t, nil
}

func optionalString(v string) *string {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	return &v
}

// employerAttestationEnvelope matches the actual payload currently produced
// by evt.AttestationSigner.
//
// These field names are Go's default JSON encoding of AttestationClaims,
// hence PascalCase.
type employerAttestationEnvelope struct {
	RequestID    string          `json:"RequestID"`
	EmployerID   string          `json:"EmployerID"`
	HRPersonID   string          `json:"HRPersonID"`
	ResponseType string          `json:"ResponseType"`
	ResponseBody json.RawMessage `json:"ResponseBody"`
	IssuedAtUnix int64           `json:"IssuedAtUnix"`
}

func isEmployerEnvelope(p employerAttestationEnvelope) bool {
	return strings.TrimSpace(p.RequestID) != "" &&
		strings.TrimSpace(p.EmployerID) != ""
}

// legacyResponseBody captures the claim shapes already visible in the
// current MVP.
//
// It supports the claim_snapshot-like representation currently consumed by
// recruiter code, while remaining tolerant of additional legacy fields.
type legacyResponseBody struct {
	Subject struct {
		FullName   string `json:"full_name"`
		EmployeeID string `json:"employee_id"`
	} `json:"subject"`

	PrimaryEmployment struct {
		IssuerName string `json:"issuer_name"`
		Title      string `json:"title"`
		StartDate  string `json:"start_date"`
		EndDate    string `json:"end_date"`
	} `json:"primary_employment"`

	Claim struct {
		Type string `json:"type"`

		Value struct {
			Title string `json:"title"`
			Skill string `json:"skill"`
			Level string `json:"level"`
		} `json:"value"`
	} `json:"claim"`
}

func (p *Profile) normalizeEmployerEnvelope(
	out credential.NormalizedCredentialEvidence,
	src employerAttestationEnvelope,
	opts credential.VerificationOptions,
) credential.NormalizedCredentialEvidence {
	out.CredentialID = src.RequestID
	out.ClaimType = credential.ClaimTypeEmploymentRole
	out.Issuer.Identifier = src.EmployerID

	var body legacyResponseBody
	if len(src.ResponseBody) > 0 {
		if err := json.Unmarshal(src.ResponseBody, &body); err != nil {
			addEvidenceError(
				&out,
				"LEGACY_RESPONSE_BODY_INVALID",
				"employer attestation ResponseBody is not valid JSON",
				"ResponseBody",
			)
		}
	}

	title := strings.TrimSpace(body.PrimaryEmployment.Title)
	if title == "" {
		title = strings.TrimSpace(body.Claim.Value.Title)
	}

	employerName := strings.TrimSpace(body.PrimaryEmployment.IssuerName)

	subjectID := strings.TrimSpace(body.Subject.EmployeeID)

	out.Claim = credential.EmploymentClaim{
		ClaimType: credential.ClaimTypeEmploymentRole,
		Subject: credential.EmploymentSubject{
			ID: subjectID,
		},
		Employment: credential.EmploymentRelationship{
			EmployerID:   src.EmployerID,
			EmployerName: employerName,
			Title:        title,
			Level:        optionalString(body.Claim.Value.Level),
			Skill:        optionalString(body.Claim.Value.Skill),
			StartDate:    optionalString(body.PrimaryEmployment.StartDate),
			EndDate:      optionalString(body.PrimaryEmployment.EndDate),
		},
	}

	if title == "" {
		addEvidenceError(
			&out,
			"LEGACY_RESPONSE_BODY_UNMAPPABLE",
			"verified employer-response JWS does not contain a recognizable employment title",
			"ResponseBody",
		)
	}

	if subjectID != "" {
		out.Subject = credential.SubjectBindingEvidence{
			Result:    credential.EvidenceUnknown,
			Method:    "legacy-identifier",
			SubjectID: subjectID,
		}
	}

	if src.IssuedAtUnix > 0 {
		issuedAt := time.Unix(src.IssuedAtUnix, 0).UTC()
		out.Validity.IssuedAt = &issuedAt
	}

	// The current employer-response envelope has no not_before/not_after
	// fields, so temporal liveness cannot be established from the artifact.
	out.Validity.Result = credential.EvidenceUnknown

	// The current employer-response envelope contains no credential-status
	// reference, so status cannot be established from the artifact.
	out.Status.Result = credential.EvidenceUnknown

	// The envelope does not implement AP-1 presentation disclosure
	// semantics.
	out.Disclosure.DisclosedClaims = nil

	_ = opts // reserved for deterministic compatibility behavior

	return out
}
