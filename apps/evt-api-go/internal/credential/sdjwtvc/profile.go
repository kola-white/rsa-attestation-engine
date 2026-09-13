package sdjwtvc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/credential"
)

const (
	ProfileID = "sd-jwt-vc"
	MediaType = "application/dc+sd-jwt"

	// SpecVersion records the deliberately pinned SD-JWT VC draft used by
	// this implementation.
	//
	// Changes to a later draft or final RFC are credential-profile migration
	// work and MUST NOT alter Cvera domain/trust semantics.
	SpecVersion = "draft-ietf-oauth-sd-jwt-vc-18"
)

var (
	ErrIssuanceUnsupported = errors.New("sd_jwt_vc_issuance_unsupported")
	ErrCredentialEmpty     = errors.New("sd_jwt_vc_credential_empty")
)

// IssuerKeyResolver resolves and validates the verification key associated
// with an SD-JWT VC issuer.
//
// Successful resolution establishes the cryptographic binding between the
// credential signature and issuer identifier. It MUST NOT determine whether
// that issuer is authoritative for an employment claim.
type IssuerKeyResolver interface {
	ResolveIssuerKey(
		ctx context.Context,
		issuer string,
		kid string,
		alg string,
	) (key any, method string, err error)
}

// Option configures the SD-JWT VC profile.
type Option func(*Profile)

// WithExpectedVCT restricts this profile to one exact credential type.
//
// Leaving ExpectedVCT unset means that this first migration slice requires
// vct to be present but does not yet enforce a specific Cvera vct value.
//
// This is intentional: Cvera has not yet frozen its production employment
// credential-type identifier.
func WithExpectedVCT(vct string) Option {
	return func(p *Profile) {
		p.expectedVCT = strings.TrimSpace(vct)
	}
}

// Profile implements credential.CredentialProfile for SD-JWT VC.
type Profile struct {
	keys        IssuerKeyResolver
	expectedVCT string
}

// New creates the primary Cvera SD-JWT VC credential profile.
func New(keys IssuerKeyResolver, opts ...Option) *Profile {
	p := &Profile{
		keys: keys,
	}

	for _, opt := range opts {
		if opt != nil {
			opt(p)
		}
	}

	return p
}

var _ credential.CredentialProfile = (*Profile)(nil)

func (p *Profile) ProfileID() string {
	return ProfileID
}

// Issue is deliberately not implemented in the first migration slice.
//
// We first establish parsing, issuer/key resolution, cryptographic
// verification, selective-disclosure reconstruction and normalization.
//
// Issuance will be added only after the Cvera SD-JWT VC employment vct and
// issuance claim profile are frozen.
func (p *Profile) Issue(
	_ context.Context,
	_ credential.CredentialIssuanceInput,
) (credential.IssuedCredential, error) {
	return credential.IssuedCredential{}, ErrIssuanceUnsupported
}

// VerifyAndNormalize verifies an SD-JWT VC issuer-signed JWT, processes
// disclosures and converts the resulting evidence into Cvera's
// format-neutral NormalizedCredentialEvidence.
//
// This method establishes evidence only.
//
// It MUST NOT:
//
//   - determine issuer authority;
//   - return AUTHORIZED / UNAUTHORIZED;
//   - evaluate verifier reliance policy;
//   - execute DP-1;
//   - return Cvera VERIFIED / REVIEW / REJECTED.
func (p *Profile) VerifyAndNormalize(
	ctx context.Context,
	raw []byte,
	opts credential.VerificationOptions,
) (credential.NormalizedCredentialEvidence, error) {
	out := newEvidence()

	encoded := strings.TrimSpace(string(raw))
	if encoded == "" {
		return out, ErrCredentialEmpty
	}

	parsed, err := ParseCompact(encoded)
	if err != nil {
		addEvidenceError(
			&out,
			"SD_JWT_FORMAT_INVALID",
			err.Error(),
			"",
		)
		return out, nil
	}

	header, unverifiedPayload, err := DecodeIssuerJWTUnverified(parsed.IssuerJWT)
	if err != nil {
		addEvidenceError(
			&out,
			"ISSUER_JWT_INVALID",
			err.Error(),
			"",
		)
		return out, nil
	}

	typ, _ := header["typ"].(string)

	// draft-18 requires dc+sd-jwt, while explicitly recommending temporary
	// acceptance of the historical vc+sd-jwt value during transition.
	if typ != "dc+sd-jwt" && typ != "vc+sd-jwt" {
		addEvidenceError(
			&out,
			"JOSE_TYP_INVALID",
			fmt.Sprintf("unexpected SD-JWT VC typ %q", typ),
			"header.typ",
		)
		return out, nil
	}

	alg, _ := header["alg"].(string)
	if !supportedAlgorithm(alg) {
		addEvidenceError(
			&out,
			"JOSE_ALG_UNSUPPORTED",
			fmt.Sprintf("unsupported SD-JWT VC signing algorithm %q", alg),
			"header.alg",
		)
		return out, nil
	}

	kid, _ := header["kid"].(string)
	kid = strings.TrimSpace(kid)

	// kid is RECOMMENDED rather than universally mandatory in SD-JWT VC,
	// but Cvera's default metadata/JWKS migration profile deliberately
	// requires it for deterministic key selection.
	if kid == "" {
		addEvidenceError(
			&out,
			"KID_MISSING",
			"Cvera SD-JWT VC metadata profile requires a kid header",
			"header.kid",
		)
		return out, nil
	}

	issuer, _ := unverifiedPayload["iss"].(string)
	issuer = strings.TrimSpace(issuer)

	if issuer == "" {
		addEvidenceError(
			&out,
			"ISSUER_MISSING",
			"SD-JWT VC iss claim is required",
			"iss",
		)
		return out, nil
	}

	out.Issuer.Identifier = issuer
	out.Issuer.KeyResolution.KeyID = kid

	if p == nil || p.keys == nil {
		out.Issuer.KeyResolution.Result = credential.EvidenceUnknown

		addEvidenceError(
			&out,
			"KEY_RESOLVER_UNCONFIGURED",
			"SD-JWT VC issuer key resolver is not configured",
			"",
		)
		return out, nil
	}

	key, method, err := p.keys.ResolveIssuerKey(
		ctx,
		issuer,
		kid,
		alg,
	)
	if err != nil {
		out.Issuer.KeyResolution.Result = credential.EvidenceInvalid

		addEvidenceError(
			&out,
			"KEY_RESOLUTION_FAILED",
			err.Error(),
			"iss",
		)
		return out, nil
	}

	out.Issuer.KeyResolution.Result = credential.EvidenceValid
	out.Issuer.KeyResolution.Method = method
	out.Cryptographic.Algorithm = alg

	verifiedPayload, err := verifyIssuerJWT(
		parsed.IssuerJWT,
		key,
		alg,
	)
	if err != nil {
		out.Cryptographic.Signature = credential.EvidenceInvalid

		addEvidenceError(
			&out,
			"BAD_SIGNATURE",
			err.Error(),
			"",
		)
		return out, nil
	}

	out.Cryptographic.Signature = credential.EvidenceValid

	processedPayload, _, err := ApplyDisclosures(
		verifiedPayload,
		parsed.Disclosures,
	)
	if err != nil {
		addEvidenceError(
			&out,
			"DISCLOSURE_INVALID",
			err.Error(),
			"",
		)
		return out, nil
	}

	// Re-read iss from the cryptographically verified payload rather than
	// trusting only the preliminary unverified decode.
	verifiedIssuer, _ := processedPayload["iss"].(string)
	if verifiedIssuer != issuer {
		addEvidenceError(
			&out,
			"ISSUER_MISMATCH",
			"verified issuer identifier differs from issuer used for key resolution",
			"iss",
		)
		return out, nil
	}

	claim, vct, err := ExtractEmploymentClaim(processedPayload)
	if err != nil {
		addEvidenceError(
			&out,
			"CLAIM_INVALID",
			err.Error(),
			"",
		)
		return out, nil
	}

	if p.expectedVCT != "" && vct != p.expectedVCT {
		addEvidenceError(
			&out,
			"VCT_UNSUPPORTED",
			fmt.Sprintf(
				"credential vct %q does not equal expected vct %q",
				vct,
				p.expectedVCT,
			),
			"vct",
		)
		return out, nil
	}

	out.ClaimType = credential.ClaimTypeEmploymentRole
	out.Claim = claim

	if jti, ok := processedPayload["jti"].(string); ok {
		out.CredentialID = strings.TrimSpace(jti)
	}

	normalizeTemporalEvidence(
		&out,
		processedPayload,
		effectiveNow(opts),
	)

	normalizeSubjectBindingEvidence(
		&out,
		processedPayload,
		parsed,
	)

	// Status processing deliberately belongs to the next implementation
	// slice. Presence is recorded without falsely claiming status validity.
	if _, ok := processedPayload["status"]; ok {
		out.Status = credential.StatusEvidence{
			Result: credential.EvidenceUnknown,
			Method: "sd-jwt-vc-status-unchecked",
		}
	} else {
		out.Status.Result = credential.EvidenceUnknown
	}

	out.Disclosure.DisclosedClaims =
		VisibleEmploymentClaimPaths(processedPayload)

	return out, nil
}

func verifyIssuerJWT(
	compact string,
	key any,
	alg string,
) (map[string]any, error) {
	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{alg}),
		jwt.WithoutClaimsValidation(),
	)

	token, err := parser.Parse(
		compact,
		func(token *jwt.Token) (any, error) {
			if token.Method.Alg() != alg {
				return nil, fmt.Errorf(
					"JWT algorithm %q does not match expected %q",
					token.Method.Alg(),
					alg,
				)
			}

			return key, nil
		},
	)
	if err != nil {
		return nil, fmt.Errorf("verify issuer JWT: %w", err)
	}

	if token == nil || !token.Valid {
		return nil, errors.New("issuer JWT signature is invalid")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, errors.New("issuer JWT payload is not a JSON object")
	}

	out := make(map[string]any, len(claims))
	for k, v := range claims {
		out[k] = v
	}

	return out, nil
}

func supportedAlgorithm(alg string) bool {
	switch alg {
	case "RS256", "ES256", "EdDSA":
		return true
	default:
		return false
	}
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

func addEvidenceError(
	out *credential.NormalizedCredentialEvidence,
	code string,
	message string,
	field string,
) {
	out.Errors = append(
		out.Errors,
		credential.EvidenceError{
			Code:    code,
			Message: message,
			Field:   field,
		},
	)
}

func effectiveNow(opts credential.VerificationOptions) time.Time {
	if !opts.Now.IsZero() {
		return opts.Now.UTC()
	}

	return time.Now().UTC()
}