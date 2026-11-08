package db

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/authority"
)

// LoadLockedAuthoritySnapshot reuses the accepted completeness/projection checks
// in a caller-owned READ COMMITTED transaction. The shared recording-boundary
// lock blocks every participating authority write until that transaction ends.
// Caller must pin its trusted schema with pg_temp last. This function grants no
// privileges; it is NOT wired into the production eligibility handler yet.
func LoadLockedAuthoritySnapshot(ctx context.Context, tx pgx.Tx) (authority.AuthoritySnapshot, error) {
	var isolation string
	if err := tx.QueryRow(ctx, `SHOW transaction_isolation`).Scan(&isolation); err != nil {
		return authority.AuthoritySnapshot{}, err
	}
	if isolation != "read committed" {
		return authority.AuthoritySnapshot{}, errors.New("locked authority requires READ COMMITTED")
	}
	// Resolve exactly one trusted application schema and explicitly keep temp
	// relations last; missing/multiple application schemas fail closed.
	var schemas []string
	if err := tx.QueryRow(ctx, `SELECT ARRAY(SELECT x FROM unnest(pg_catalog.current_schemas(false)) x WHERE x<>'pg_catalog' AND x NOT LIKE 'pg_temp%')`).Scan(&schemas); err != nil {
		return authority.AuthoritySnapshot{}, err
	}
	if len(schemas) != 1 {
		return authority.AuthoritySnapshot{}, errors.New("trusted authority schema required")
	}
	if _, err := tx.Exec(ctx, `SELECT pg_catalog.set_config('search_path',pg_catalog.format('pg_catalog,%I,pg_temp',$1::text),true)`, schemas[0]); err != nil {
		return authority.AuthoritySnapshot{}, err
	}
	var unsafe bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_catalog.pg_class WHERE relnamespace=$1::regnamespace AND relname IN ('authority_authorizations','authority_adjudications','authority_journal_events','authority_recording_boundary') AND (relrowsecurity OR relforcerowsecurity))`, schemas[0]).Scan(&unsafe); err != nil {
		return authority.AuthoritySnapshot{}, err
	}
	if unsafe {
		return authority.AuthoritySnapshot{}, errors.New("authority RLS requires review")
	}
	var position int64
	if err := tx.QueryRow(ctx, `SELECT committed_position FROM authority_recording_boundary WHERE singleton=TRUE FOR SHARE`).Scan(&position); err != nil {
		return authority.AuthoritySnapshot{}, err
	}
	observed, err := loadAuthoritySnapshotObservedAt(ctx, tx)
	if err != nil {
		return authority.AuthoritySnapshot{}, err
	}
	if err = validateAuthorityRecordingCompleteness(ctx, tx, position); err != nil {
		return authority.AuthoritySnapshot{}, err
	}
	entries, err := loadAuthoritySnapshotEntriesAtPosition(ctx, tx, position)
	if err != nil {
		return authority.AuthoritySnapshot{}, err
	}
	snapshot := authority.AuthoritySnapshot{Complete: true, RecordingPosition: position, RecordedThrough: observed, Entries: entries}
	if _, err = authority.NewSnapshotAuthorityResolver(snapshot); err != nil {
		return authority.AuthoritySnapshot{}, err
	}
	return snapshot, nil
}
