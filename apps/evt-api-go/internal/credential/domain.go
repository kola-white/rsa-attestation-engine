package credential

// ClaimTypeEmploymentRole is the canonical Cvera claim-type identifier
// for an employment-role assertion.
//
// This is a Cvera domain identifier. It is intentionally independent of
// any credential serialization or transport profile such as legacy JWS,
// SD-JWT VC, OpenID4VCI, or OpenID4VP.
const ClaimTypeEmploymentRole = "employment.role"

// EmploymentClaim is Cvera's canonical domain representation of an
// authoritative employment assertion.
//
// It describes the real-world assertion being made. It MUST NOT contain
// credential-format mechanics, request/workflow state, verifier policy,
// trust decisions, billing state, or presentation state.
//
// Examples of fields that do NOT belong here:
//
//   - request_id
//   - workflow status
//   - recruiter/verifier identity
//   - JWS/JWT headers
//   - kid / alg
//   - SD-JWT disclosures
//   - revocation/status-list mechanics
//   - Cvera trust_result
//   - VERIFIED / REJECTED state
//
// Those concerns belong to their respective workflow, credential-profile,
// evidence, or Cvera trust-evaluation layers.
type EmploymentClaim struct {
	ClaimType string                 `json:"claim_type"`
	Subject   EmploymentSubject      `json:"subject"`
	Employment EmploymentRelationship `json:"employment"`
}

// EmploymentSubject identifies the subject of the employment assertion
// within Cvera's canonical domain model.
//
// ID is intentionally format-neutral. It is not necessarily the identifier
// disclosed to a verifier and does not prescribe how holder/subject binding
// is represented in any credential profile.
type EmploymentSubject struct {
	ID string `json:"id"`
}

// EmploymentRelationship contains the substantive employment assertion
// made by the authoritative issuer.
//
// EmployerID identifies the organization asserting the relationship.
// EmployerName is descriptive and is not, by itself, evidence of issuer
// authority.
//
// Optional fields are represented as pointers so that "not asserted" can be
// distinguished from an asserted empty value.
type EmploymentRelationship struct {
	EmployerID   string `json:"employer_id"`
	EmployerName string `json:"employer_name,omitempty"`

	Title string  `json:"title"`
	Level *string `json:"level,omitempty"`
	Skill *string `json:"skill,omitempty"`

	StartDate *string `json:"start_date,omitempty"`
	EndDate   *string `json:"end_date,omitempty"`
}