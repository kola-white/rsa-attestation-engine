package legacyjws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/credential"
)

const (
	ProfileID = "legacy-jws"
	MediaType = "application/jwt"
)

var (
	// ErrIssuanceUnsupported is intentional during the migration.
	//
	// Existing legacy issuance remains in the current evt package until
	// SD-JWT VC issuance is introduced. The legacy adapter exists first to
	// place verification/normalization behind CredentialProfile without
	// changing existing issuance behavior.
	ErrIssuanceUnsupported = errors.New("legacy_jws_issuance_unsupported")

	ErrCredentialEmpty = errors.New("legacy_jws_credential_empty")
)

// KeyResolver resolves the public verification key associated with a legacy
// JWS protected-header kid.
//
// The concrete implementation may load keys from the existing JWKS trust
// artifact, a file, a cache, or another source.
//
// Resolving the key establishes cryptographic verification capability only.
// It MUST NOT determine Cvera issuer authority.
type KeyResolver interface {
	ResolveKey(ctx context.Context, kid string) (any, error)
}

// StatusResolver resolves the legacy credential-status state.
//
// This abstraction exists only to preserve compatibility with the existing
// legacy status-list behavior. The standardized status mechanism selected
// for SD-JWT VC will live in that credential profile.
//
// Implementations should return:
//
//	credential.EvidenceValid   -> credential is known not revoked
//	credential.EvidenceInvalid -> credential is revoked/invalid
//	credential.EvidenceUnknown -> status cannot be established
type StatusResolver interface {
	ResolveStatus(
		ctx context.Context,
		method string,
		reference string,
		serial string,
	) (credential.EvidenceResult, error)
}

// Profile implements credential.CredentialProfile for legacy JWS artifacts.
//
// It supports normalization of both:
//
//  1. AP-1 employment.role JWS payloads; and
//  2. the older employer-response envelope produced by AttestationSigner.
//
// The latter is supported for migration compatibility only.
type Profile struct {
	keys   KeyResolver
	status StatusResolver
}

// New creates a legacy JWS credential profile.
//
// keys is required for cryptographic verification.
// status may be nil; if absent, status evidence is reported as unknown.
func New(keys KeyResolver, status StatusResolver) *Profile {
	return &Profile{
		keys:   keys,
		status: status,
	}
}

// Compile-time assertion that Profile satisfies the frozen interface.
var _ credential.CredentialProfile = (*Profile)(nil)

func (p *Profile) ProfileID() string {
	return ProfileID
}

// Issue intentionally does not replace the current legacy issuance path.
//
// The existing Go runtime signs a workflow-oriented employer attestation
// envelope that contains request/employer/HR/response metadata. That shape
// is not the new canonical CredentialIssuanceInput contract.
//
// Rather than silently changing legacy production semantics, issuance stays
// where it currently exists until SD-JWT VC issuance is introduced.
func (p *Profile) Issue(
	_ context.Context,
	_ credential.CredentialIssuanceInput,
) (credential.IssuedCredential, error) {
	return credential.IssuedCredential{}, ErrIssuanceUnsupported
}

// VerifyAndNormalize verifies a legacy compact JWS and converts the result
// into format-neutral NormalizedCredentialEvidence.
//
// Credential validation failures are represented in the returned evidence.
// Operational failures are returned as Go errors.
//
// This method MUST NOT:
//   - authorize the issuer;
//   - evaluate verifier trust/reliance policy;
//   - perform DP-1;
//   - produce Cvera VERIFIED/REJECTED outcomes.
func (p *Profile) VerifyAndNormalize(
	ctx context.Context,
	raw []byte,
	opts credential.VerificationOptions,
) (credential.NormalizedCredentialEvidence, error) {
	out := newEvidence()

	compact := strings.TrimSpace(string(raw))
	if compact == "" {
		return out, ErrCredentialEmpty
	}

	if p == nil || p.keys == nil {
		out.Cryptographic.Signature = credential.EvidenceUnknown
		out.Issuer.KeyResolution.Result = credential.EvidenceUnknown
		addEvidenceError(
			&out,
			"KEY_RESOLVER_UNCONFIGURED",
			"legacy JWS key resolver is not configured",
			"",
		)
		return out, nil
	}

	var resolvedKid string

	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithoutClaimsValidation(),
	)

	token, err := parser.Parse(
		compact,
		func(token *jwt.Token) (any, error) {
			alg, _ := token.Header["alg"].(string)
			if alg != "RS256" {
				return nil, fmt.Errorf("unsupported_alg:%s", alg)
			}

			kid, ok := token.Header["kid"].(string)
			if !ok || strings.TrimSpace(kid) == "" {
				return nil, errors.New("missing_kid")
			}

			resolvedKid = kid

			key, err := p.keys.ResolveKey(ctx, kid)
			if err != nil {
				return nil, fmt.Errorf("resolve key %q: %w", kid, err)
			}
			if key == nil {
				return nil, fmt.Errorf("resolve key %q: nil key", kid)
			}

			return key, nil
		},
	)

	out.Issuer.KeyResolution.Method = "legacy-jwks"
	out.Issuer.KeyResolution.KeyID = resolvedKid
	out.Cryptographic.Algorithm = "RS256"

	if err != nil || token == nil || !token.Valid {
		out.Cryptographic.Signature = credential.EvidenceInvalid

		if resolvedKid == "" {
			out.Issuer.KeyResolution.Result = credential.EvidenceInvalid
		} else {
			out.Issuer.KeyResolution.Result = credential.EvidenceUnknown
		}

		addEvidenceError(
			&out,
			"BAD_SIGNATURE",
			errorMessage(err, "legacy JWS signature verification failed"),
			"",
		)

		return out, nil
	}

	out.Cryptographic.Signature = credential.EvidenceValid
	out.Issuer.KeyResolution.Result = credential.EvidenceValid

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		addEvidenceError(
			&out,
			"PAYLOAD_INVALID",
			"legacy JWS payload is not a JSON object",
			"",
		)
		return out, nil
	}

	payloadBytes, err := json.Marshal(claims)
	if err != nil {
		return out, fmt.Errorf("marshal verified legacy claims: %w", err)
	}

	// Prefer the documented AP-1 portable employment-role shape.
	var ap1 ap1Payload
	if err := json.Unmarshal(payloadBytes, &ap1); err == nil && isAP1(ap1) {
		return p.normalizeAP1(ctx, out, ap1, opts)
	}

	// Fall back to the employer-response envelope currently produced by the
	// Go AttestationSigner.
	var envelope employerAttestationEnvelope
	if err := json.Unmarshal(payloadBytes, &envelope); err == nil && isEmployerEnvelope(envelope) {
		return p.normalizeEmployerEnvelope(out, envelope, opts), nil
	}

	addEvidenceError(
		&out,
		"UNSUPPORTED_LEGACY_PAYLOAD",
		"verified JWS does not match a supported legacy Cvera payload shape",
		"",
	)

	return out, nil
}

func newEvidence() credential.NormalizedCredentialEvidence {
	return credential.NormalizedCredentialEvidence{
		ProfileID: ProfileID,
		ClaimType: credential.ClaimTypeEmploymentRole,

		Issuer: credential.IssuerEvidence{
			KeyResolution: credential.KeyResolutionEvidence{
				Result: credential.EvidenceUnknown,
			},
		},

		Cryptographic: credential.CryptographicEvidence{
			Signature: credential.EvidenceUnknown,
		},

		Validity: credential.ValidityEvidence{
			Result: credential.EvidenceUnknown,
		},

		Status: credential.StatusEvidence{
			Result: credential.EvidenceUnknown,
		},

		Subject: credential.SubjectBindingEvidence{
			Result: credential.EvidenceUnknown,
		},
	}
}

func errorMessage(err error, fallback string) string {
	if err == nil {
		return fallback
	}
	return err.Error()
}

func addEvidenceError(
	out *credential.NormalizedCredentialEvidence,
	code string,
	message string,
	field string,
) {
	out.Errors = append(out.Errors, credential.EvidenceError{
		Code:    code,
		Message: message,
		Field:   field,
	})
}

func effectiveNow(opts credential.VerificationOptions) time.Time {
	if !opts.Now.IsZero() {
		return opts.Now.UTC()
	}
	return time.Now().UTC()
}
