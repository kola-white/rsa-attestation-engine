package db

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/authority"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/credential"
	ec "github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/evidencecredential"
)

// IssuanceEngine holds no key when used as an independent verifier. Runtime
// authority access is bounded EXECUTE, never direct authority-table privileges.
type IssuanceEngine struct {
	DB     *DB
	Signer ec.Signer
}
type IssuanceResult struct {
	CredentialID string `json:"credentialId"`
	Credential   string `json:"credential"`
	MediaType    string `json:"mediaType"`
	Policy       string `json:"policy"`
	ScanPolicy   string `json:"scanPolicy"`
}
type wireKey struct {
	ec.KeyBinding
	PublicPEM string
}

func parseBoundKey(k wireKey) (ec.KeyBinding, error) {
	block, rest := pem.Decode([]byte(k.PublicPEM))
	if block == nil || block.Type != "PUBLIC KEY" || len(rest) != 0 {
		return ec.KeyBinding{}, ec.ErrUnavailable
	}
	value, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return ec.KeyBinding{}, err
	}
	key, ok := value.(*rsa.PublicKey)
	if !ok {
		return ec.KeyBinding{}, ec.ErrUnavailable
	}
	k.KeyBinding.PublicKey = key
	return k.KeyBinding, nil
}
func pinIssuanceSchema(ctx context.Context, tx pgx.Tx) error {
	var schemas []string
	if err := tx.QueryRow(ctx, `SELECT ARRAY(SELECT x FROM unnest(pg_catalog.current_schemas(false)) x WHERE x<>'pg_catalog' AND x NOT LIKE 'pg_temp%')`).Scan(&schemas); err != nil {
		return err
	}
	if len(schemas) != 1 {
		return ec.ErrUnavailable
	}
	_, err := tx.Exec(ctx, `SELECT pg_catalog.set_config('search_path',pg_catalog.format('pg_catalog,%I,pg_temp',$1::text),true)`, schemas[0])
	return err
}

// LoadBoundedAuthoritySnapshot preserves the accepted journal/projector and
// completeness semantics. SQL locks the global recording boundary and verifies
// gap-free positions; Go reconstructs every immutable journal and adjudication.
func LoadBoundedAuthoritySnapshot(ctx context.Context, tx pgx.Tx) (authority.AuthoritySnapshot, error) {
	var raw []byte
	if err := tx.QueryRow(ctx, `SELECT read_evidence_authority()`).Scan(&raw); err != nil {
		return authority.AuthoritySnapshot{}, errors.Join(ec.ErrUnavailable, err)
	}
	var wire struct {
		RecordingPosition int64
		RecordedThrough   time.Time
		Entries           []struct {
			Record authority.AuthorizationRecord
			Events []authority.AuthorizationJournalEvent
		}
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		return authority.AuthoritySnapshot{}, err
	}
	snapshot := authority.AuthoritySnapshot{Complete: true, RecordingPosition: wire.RecordingPosition, RecordedThrough: wire.RecordedThrough.UTC()}
	for _, entry := range wire.Entries {
		if entry.Record.Adjudication != nil {
			entry.Record.Adjudication.DecidedAt = entry.Record.Adjudication.DecidedAt.UTC()
			entry.Record.Adjudication.RecordedAt = entry.Record.Adjudication.RecordedAt.UTC()
		}
		var journal authority.AuthorizationJournal
		for _, event := range entry.Events {
			event.EffectiveAt = event.EffectiveAt.UTC()
			event.RecordedAt = event.RecordedAt.UTC()
			if err := journal.Append(event); err != nil {
				return authority.AuthoritySnapshot{}, err
			}
		}
		snapshot.Entries = append(snapshot.Entries, authority.AuthoritySnapshotEntry{Record: entry.Record, Journal: journal})
	}
	if _, err := authority.NewSnapshotAuthorityResolver(snapshot); err != nil {
		return authority.AuthoritySnapshot{}, err
	}
	return snapshot, nil
}

// This is the established request snapshot mapping, with canonical employer
// and subject supplied only by the persisted request, never descriptive fields.
func employmentFromSnapshot(snapshot, employer, subject string) (credential.EmploymentRelationship, error) {
	var wire struct {
		Subject struct {
			ID string `json:"id"`
		} `json:"subject"`
		ClaimType string `json:"claim_type"`
		Primary   struct {
			EmployerID   string  `json:"employer_id"`
			Title        string  `json:"title"`
			EmployerName string  `json:"issuer_name"`
			Level        *string `json:"level"`
			Skill        *string `json:"skill"`
			StartDate    *string `json:"start_date"`
			EndDate      *string `json:"end_date"`
		} `json:"primary_employment"`
	}
	if err := json.Unmarshal([]byte(snapshot), &wire); err != nil {
		return credential.EmploymentRelationship{}, err
	}
	if (wire.Subject.ID != "" && wire.Subject.ID != subject) || (wire.ClaimType != "" && wire.ClaimType != credential.ClaimTypeEmploymentRole) || (wire.Primary.EmployerID != "" && wire.Primary.EmployerID != employer) || wire.Primary.Title == "" {
		return credential.EmploymentRelationship{}, ec.ErrIneligible
	}
	for _, value := range []*string{wire.Primary.StartDate, wire.Primary.EndDate} {
		if value != nil && *value != "" {
			if _, err := time.Parse("2006-01-02", *value); err != nil {
				return credential.EmploymentRelationship{}, ec.ErrIneligible
			}
		}
	}
	return credential.EmploymentRelationship{EmployerID: employer, EmployerName: wire.Primary.EmployerName, Title: wire.Primary.Title, Level: wire.Primary.Level, Skill: wire.Primary.Skill, StartDate: wire.Primary.StartDate, EndDate: wire.Primary.EndDate}, nil
}

func (e *IssuanceEngine) Issue(ctx context.Context, principal, request, employer string) (IssuanceResult, error) {
	if e == nil || e.DB == nil || e.DB.Pool == nil || e.Signer == nil {
		return IssuanceResult{}, ec.ErrUnavailable
	}
	var result IssuanceResult
	err := e.DB.WithTx(ctx, func(tx pgx.Tx) error {
		if err := pinIssuanceSchema(ctx, tx); err != nil {
			return err
		}
		var raw []byte
		if err := tx.QueryRow(ctx, `SELECT prepare_bound_issuance($1,$2,$3,$4)`, principal, request, employer, e.Signer.KeyID()).Scan(&raw); err != nil {
			return err
		}
		var wire struct {
			Context            ec.Commitment
			Snapshot, Manifest string
			Now, ExpiresAt     time.Time
			Key                wireKey
		}
		if err := json.Unmarshal(raw, &wire); err != nil {
			return err
		}
		key, err := parseBoundKey(wire.Key)
		if err != nil {
			return err
		}
		snapshot, err := LoadBoundedAuthoritySnapshot(ctx, tx)
		if err != nil {
			return err
		}
		employment, err := employmentFromSnapshot(wire.Snapshot, wire.Context.EmployerID, wire.Context.SubjectID)
		if err != nil {
			return err
		}
		// Authority resolution may take time; evaluate at the current DB clock rather
		// than the earlier preparation timestamp, while all locks remain held.
		if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&wire.Now); err != nil {
			return err
		}
		claims, rawCredential, err := ec.IssueLocked(ctx, ec.Prepared{Context: wire.Context, Snapshot: wire.Snapshot, Manifest: wire.Manifest, Employment: employment, Key: key, Authority: snapshot, Now: wire.Now, ExpiresAt: wire.ExpiresAt}, e.Signer)
		if err != nil {
			return err
		}
		// Locks serialize revocation, but wall-clock expiry can still pass during
		// the signing call. Recheck at a fresh database time before persistence.
		var after time.Time
		if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&after); err != nil {
			return err
		}
		decision, err := ec.AuthorityAt(ctx, snapshot, wire.Context.EmployerID, after)
		if err != nil {
			return err
		}
		if decision.Decision != authority.AuthorityAuthorized || !claims.ExpiresAt.After(after) {
			return ec.ErrIneligible
		}
		encoded, err := json.Marshal(claims)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `SELECT commit_bound_issuance($1,$2,$3,$4,$5,$6::jsonb)`, principal, request, employer, e.Signer.KeyID(), rawCredential, string(encoded)); err != nil {
			return err
		}
		result = IssuanceResult{CredentialID: claims.ID, Credential: rawCredential, MediaType: "application/dc+sd-jwt", Policy: ec.Policy, ScanPolicy: ec.ManualPolicy}
		return nil
	})
	if err != nil {
		return IssuanceResult{}, err
	}
	return result, nil
}

func (e *IssuanceEngine) WithFacts(ctx context.Context, tokenDigest string, fn func(ec.Facts) error) error {
	if e == nil || e.DB == nil || e.DB.Pool == nil {
		return ec.ErrUnavailable
	}
	return e.DB.WithTx(ctx, func(tx pgx.Tx) error {
		if err := pinIssuanceSchema(ctx, tx); err != nil {
			return err
		}
		var raw []byte
		if err := tx.QueryRow(ctx, `SELECT resolve_bound_credential($1)`, tokenDigest).Scan(&raw); err != nil {
			return err
		}
		var wire struct {
			ec.Facts
			Key wireKey
		}
		if err := json.Unmarshal(raw, &wire); err != nil {
			return err
		}
		key, err := parseBoundKey(wire.Key)
		if err != nil {
			return err
		}
		wire.Facts.Key = key
		wire.Facts.Authority, err = LoadBoundedAuthoritySnapshot(ctx, tx)
		if err != nil {
			return err
		}
		actualEmployment, err := employmentFromSnapshot(wire.Snapshot, wire.Claims.Evidence.EmployerID, wire.Claims.Subject)
		if err != nil {
			return err
		}
		expected, _ := json.Marshal(actualEmployment)
		actual, _ := json.Marshal(wire.Claims.Employment)
		if string(actual) != string(expected) {
			return errors.New("issued claim differs from canonical request snapshot")
		}
		if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&wire.Facts.EvaluatedAt); err != nil {
			return err
		}
		return fn(wire.Facts)
	})
}

// OptionalAuthoritySource preserves pre-000019 deployments and missing authority
// as explicitly unconfigured. Once present, source failures remain failures.
func OptionalAuthoritySource(ctx context.Context, tx pgx.Tx) (func(context.Context, pgx.Tx) (authority.AuthoritySnapshot, error), error) {
	if err := pinIssuanceSchema(ctx, tx); err != nil {
		return nil, err
	}
	var configured bool
	if err := tx.QueryRow(ctx, `SELECT to_regprocedure('read_evidence_authority()') IS NOT NULL AND (SELECT count(*) FROM pg_catalog.pg_class WHERE relnamespace=(SELECT pronamespace FROM pg_catalog.pg_proc WHERE oid=to_regprocedure('read_evidence_authority()')) AND relname IN ('authority_authorizations','authority_adjudications','authority_journal_events','authority_recording_boundary'))=4`).Scan(&configured); err != nil {
		return nil, err
	}
	if !configured {
		return nil, nil
	}
	return LoadBoundedAuthoritySnapshot, nil
}
func ActiveEvidenceSigningCredential(ctx context.Context, tx pgx.Tx, signer ec.Signer) (bool, error) {
	if signer == nil {
		return false, nil
	}
	var raw []byte
	if err := tx.QueryRow(ctx, `SELECT active_evidence_signing_credential($1)`, signer.KeyID()).Scan(&raw); err != nil {
		return false, err
	}
	if len(raw) == 0 {
		return false, nil
	}
	var wire wireKey
	if err := json.Unmarshal(raw, &wire); err != nil {
		return false, err
	}
	key, err := parseBoundKey(wire)
	if err != nil {
		return false, err
	}
	return key.Fingerprint == ec.Fingerprint(signer.PublicKey()) && key.KID == signer.KeyID() && key.Issuer == ec.Issuer, nil
}

// SubjectCredential uses only existing legacy-table SELECT permissions, with
// trusted schema pinning and a current active canonical requestor check.
func (e *IssuanceEngine) SubjectCredential(ctx context.Context, principal, request string) (string, error) {
	if e == nil || e.DB == nil || e.DB.Pool == nil {
		return "", ec.ErrUnavailable
	}
	var artifact string
	err := e.DB.WithTx(ctx, func(tx pgx.Tx) error {
		if err := pinIssuanceSchema(ctx, tx); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT r.attestation_jws FROM evt_requests r JOIN domain_users p ON p.id=r.candidate_personid WHERE r.request_id=$1 AND r.candidate_personid=$2 AND p.status='active' AND p.role='requestor' AND right(coalesce(r.attestation_jws,''),1)='~'`, request, principal).Scan(&artifact)
	})
	if err != nil {
		return "", err
	}
	return artifact, nil
}
