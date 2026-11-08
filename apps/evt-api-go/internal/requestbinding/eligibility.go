// Package requestbinding prepares evidence-backed requests without signing.
package requestbinding

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/authority"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/credential"
)

const IssuerID = "https://cvera.app"

// LockedAuthoritySource must load a complete authoritative snapshot while holding
// its recording-boundary lock in this SAME transaction until caller COMMIT.
// It is a trusted server dependency, never supplied by an HTTP caller. A missing
// source is intentionally ineligible; production authority privileges are unchanged.
type LockedAuthoritySource func(context.Context, pgx.Tx) (authority.AuthoritySnapshot, error)

type Result struct {
	ScanStatuses               []string                    `json:"scanStatuses,omitempty"`
	PendingScanCount           int                         `json:"pendingScanCount"`
	Eligible                   bool                        `json:"eligible"`
	EvidenceReady              bool                        `json:"evidenceReady"`
	Reason                     string                      `json:"reason"`
	SigningEnabled             bool                        `json:"signingEnabled"`
	ManifestID                 string                      `json:"manifestId,omitempty"`
	ManifestDigest             string                      `json:"manifestDigest,omitempty"`
	ManualPolicy               string                      `json:"manualPolicy,omitempty"`
	AuthorityDecision          authority.AuthorityDecision `json:"authorityDecision,omitempty"`
	AuthorityGrantID           string                      `json:"authorityGrantId,omitempty"`
	AuthorityRecordingPosition int64                       `json:"authorityRecordingPosition,omitempty"`
}

// Prepare's result is a preparation snapshot, NOT an issuance capability. A
// future issuance transaction must reauthorize and commit the exact manifest,
// subject/issuer/claim context and actual credential reference atomically.
func Prepare(ctx context.Context, tx pgx.Tx, principal, request, employer string, source LockedAuthoritySource) (Result, error) {
	var raw []byte
	if err := tx.QueryRow(ctx, `SELECT prepare_request_evidence($1,$2,$3)`, principal, request, employer).Scan(&raw); err != nil {
		return Result{}, err
	}
	var evidence struct {
		ScanStatuses     []string `json:"scanStatuses"`
		PendingScanCount int      `json:"pendingScanCount"`
		EvidenceReady    bool     `json:"evidenceReady"`
		Reason           string   `json:"reason"`
		ManifestID       string   `json:"manifestId"`
		ManifestDigest   string   `json:"manifestDigest"`
		IssuerID         string   `json:"issuerId"`
		EmployerID       string   `json:"employerId"`
		ClaimType        string   `json:"claimType"`
		ManualPolicy     string   `json:"manualPolicy"`
	}
	if err := json.Unmarshal(raw, &evidence); err != nil {
		return Result{}, err
	}
	result := Result{ScanStatuses: evidence.ScanStatuses, PendingScanCount: evidence.PendingScanCount, EvidenceReady: evidence.EvidenceReady, Reason: evidence.Reason, ManifestID: evidence.ManifestID,
		ManifestDigest: evidence.ManifestDigest, ManualPolicy: evidence.ManualPolicy}
	if !evidence.EvidenceReady {
		return result, nil
	}
	if evidence.IssuerID != IssuerID || evidence.ClaimType != credential.ClaimTypeEmploymentRole || evidence.ManualPolicy != "MANUAL_REVIEW_PILOT_V1" || evidence.ManifestID == "" || len(evidence.ManifestDigest) != 64 {
		return Result{}, errors.New("invalid binding preparation contract")
	}
	result.Reason = "AUTHORITY_SOURCE_UNAVAILABLE"
	result.AuthorityDecision = authority.AuthorityReview
	if source == nil {
		return result, nil
	}
	snapshot, err := source(ctx, tx)
	if err != nil {
		return result, err
	}
	resolver, err := authority.NewSnapshotAuthorityResolver(snapshot)
	if err != nil {
		return result, err
	}
	var now time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return result, err
	}
	grants, err := resolver.ResolveAuthorityGrants(ctx, evidence.IssuerID, evidence.ClaimType)
	if err != nil {
		return result, err
	}
	evaluation := authority.Evaluator{}.Evaluate(credential.NormalizedCredentialEvidence{
		Issuer: credential.IssuerEvidence{Identifier: evidence.IssuerID}, ClaimType: evidence.ClaimType,
		Claim: credential.EmploymentClaim{Employment: credential.EmploymentRelationship{EmployerID: evidence.EmployerID}},
	}, grants, now)
	result.AuthorityDecision = evaluation.Decision
	result.AuthorityGrantID = evaluation.GrantID
	result.AuthorityRecordingPosition = snapshot.RecordingPosition
	result.Reason = evaluation.Reason
	result.Eligible = evaluation.Decision == authority.AuthorityAuthorized
	return result, nil
}

// ErrSigningDeferred prevents the legacy signer from bypassing this milestone's
// binding/authority gate. It applies to bound requests, including revoked ones.
var ErrSigningDeferred = errors.New("evidence_backed_signing_not_enabled")

func IsBound(ctx context.Context, tx pgx.Tx, principal, request, employer string) (bool, error) {
	var schemas []string
	if err := tx.QueryRow(ctx, `SELECT ARRAY(SELECT x FROM unnest(pg_catalog.current_schemas(false)) x WHERE x<>'pg_catalog' AND x NOT LIKE 'pg_temp%')`).Scan(&schemas); err != nil {
		return false, err
	}
	if len(schemas) != 1 {
		return false, errors.New("trusted request schema required")
	}
	var enabled bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_catalog.pg_class WHERE relnamespace=$1::regnamespace AND relname='request_case_bindings') OR EXISTS(SELECT 1 FROM pg_catalog.pg_proc WHERE pronamespace=$1::regnamespace AND proname='request_requires_evidence_issuance')`, schemas[0]).Scan(&enabled); err != nil {
		return false, err
	}
	if !enabled {
		return false, nil
	} // Pre-000018 schemas have no adoptable bindings.
	var bound bool
	err := tx.QueryRow(ctx, `SELECT `+pgx.Identifier{schemas[0], "request_requires_evidence_issuance"}.Sanitize()+`($1,$2,$3)`, principal, request, employer).Scan(&bound)
	return bound, err
}
