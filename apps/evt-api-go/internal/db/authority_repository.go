package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/authority"
)

// AuthorityRepository provides PostgreSQL-backed persistence for
// authorization records, adjudications, and lifecycle journals.
//
// Authority evidence is append-only. Grant state is derived from
// that evidence rather than stored as mutable authority state.
type AuthorityRepository struct {
	pool *pgxpool.Pool
}

func NewAuthorityRepository(pool *pgxpool.Pool) *AuthorityRepository {
	return &AuthorityRepository{pool: pool}
}

// withAuthorityRecordingTx serializes participating authority writes.
// The recording counter advances in the same transaction as the evidence.
// A rollback also rolls back the counter advancement.
func (r *AuthorityRepository) withAuthorityRecordingTx(
	ctx context.Context,
	fn func(pgx.Tx, int64) error,
) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel: pgx.ReadCommitted,
	})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	var position int64
	if err := tx.QueryRow(ctx,
		`SELECT next_authority_recording_position()`,
	).Scan(&position); err != nil {
		return fmt.Errorf("allocate authority recording position: %w", err)
	}
	if position <= 0 {
		return errors.New("invalid authority recording position")
	}

	if err := fn(tx, position); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// CreateAuthorization inserts the immutable identity and scope of an
// authorization. Adjudication evidence is persisted separately.
// The evidence insert and recording position commit atomically.
func (r *AuthorityRepository) CreateAuthorization(
	ctx context.Context,
	record authority.AuthorizationRecord,
) error {
	if record.AuthorizationID == "" ||
		record.IssuerID == "" ||
		record.AuthoritySubjectID == "" ||
		record.ClaimType == "" {
		return errors.New("invalid authorization scope")
	}

	return r.withAuthorityRecordingTx(ctx, func(tx pgx.Tx, position int64) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO authority_authorizations (
				authorization_id,
				issuer_id,
				authority_subject_id,
				claim_type, recording_position
			)
			VALUES ($1, $2, $3, $4, $5)
		`,
			record.AuthorizationID,
			record.IssuerID,
			record.AuthoritySubjectID,
			record.ClaimType,
			position,
		)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				return fmt.Errorf("%w: authorization already exists", ErrConflict)
			}
		}
		return err
	})
}

// CreateAdjudication persists an immutable organizational authority decision.
// Both approved and rejected decisions are retained as evidence.
func (r *AuthorityRepository) CreateAdjudication(
	ctx context.Context,
	decision authority.AuthorizationAdjudication,
) error {
	if decision.DecisionID == "" ||
		decision.AuthorizationID == "" ||
		decision.IssuerID == "" ||
		decision.AuthoritySubjectID == "" ||
		decision.ClaimType == "" ||
		decision.DecisionMakerID == "" ||
		decision.AuthorityBasis == "" ||
		decision.EvidenceRef == "" ||
		(decision.Outcome != authority.AdjudicationApproved &&
			decision.Outcome != authority.AdjudicationRejected) ||
		!validAuthorityDBTimestamp(decision.DecidedAt) ||
		!validAuthorityDBTimestamp(decision.RecordedAt) ||
		decision.DecidedAt.After(decision.RecordedAt) {
		return errors.New("invalid authority adjudication")
	}

	return r.withAuthorityRecordingTx(ctx, func(tx pgx.Tx, position int64) error {
		_, err := tx.Exec(ctx, `
		INSERT INTO authority_adjudications (
			decision_id, authorization_id, issuer_id,
			authority_subject_id, claim_type, outcome,
			decision_maker_id, authority_basis, evidence_ref,
			decided_at, recorded_at, recording_position
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`,
			decision.DecisionID,
			decision.AuthorizationID,
			decision.IssuerID,
			decision.AuthoritySubjectID,
			decision.ClaimType,
			decision.Outcome,
			decision.DecisionMakerID,
			decision.AuthorityBasis,
			decision.EvidenceRef,
			decision.DecidedAt,
			decision.RecordedAt,
			position,
		)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				return fmt.Errorf("%w: adjudication decision already exists", ErrConflict)
			}
		}
		return err
	})
}

// validAuthorityDBTimestamp requires canonical UTC microsecond precision.
func validAuthorityDBTimestamp(value time.Time) bool {
	return !value.IsZero() &&
		value.Location() == time.UTC &&
		value.Nanosecond()%1000 == 0
}

// withLockedAuthorization serializes journal operations for one authorization.
// The caller's work and the row lock share a single database transaction.
func (r *AuthorityRepository) withLockedAuthorization(
	ctx context.Context,
	authorizationID string,
	fn func(pgx.Tx, int64) error,
) error {
	if authorizationID == "" {
		return errors.New("authorization ID is empty")
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel: pgx.ReadCommitted,
	})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	// Acquire the global recording lock before the authorization row lock.
	// All authority write paths now follow the same lock ordering.
	var recordingPosition int64
	if err := tx.QueryRow(ctx,
		`SELECT next_authority_recording_position()`,
	).Scan(&recordingPosition); err != nil {
		return fmt.Errorf("allocate journal recording position: %w", err)
	}
	if recordingPosition <= 0 {
		return errors.New("invalid journal recording position")
	}

	var lockedID string
	err = tx.QueryRow(ctx, `
		SELECT authorization_id
		FROM authority_authorizations
		WHERE authorization_id = $1
		FOR UPDATE
	`, authorizationID).Scan(&lockedID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}

	if err := fn(tx, recordingPosition); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// loadAuthorizationJournal reconstructs an authorization's accepted
// journal from rows read within the caller's transaction.
func loadAuthorizationJournal(
	ctx context.Context,
	tx pgx.Tx,
	authorizationID string,
) (authority.AuthorizationJournal, error) {
	return loadAuthorizationJournalThrough(ctx, tx, authorizationID, nil)
}

// loadAuthorizationJournalThrough loads journal evidence through an optional
// recorded-time cutoff while preserving append order.
func loadAuthorizationJournalThrough(
	ctx context.Context,
	tx pgx.Tx,
	authorizationID string,
	recordedThrough *time.Time,
) (authority.AuthorizationJournal, error) {
	var journal authority.AuthorizationJournal

	rows, err := tx.Query(ctx, `
		SELECT
			event_id,
			authorization_id,
			event_type,
			effective_at,
			recorded_at,
			actor_id,
			evidence_ref,
			conditions_satisfied,
			COALESCE(corrects_event_id, ''),
			COALESCE(correction_reason, ''),
			COALESCE(corrected_field, '')
		FROM authority_journal_events
		WHERE authorization_id = $1
				AND ($2::timestamptz IS NULL OR recorded_at <= $2)
			ORDER BY journal_sequence ASC
		`, authorizationID, recordedThrough)
	if err != nil {
		return journal, err
	}
	defer rows.Close()

	for rows.Next() {
		var event authority.AuthorizationJournalEvent
		var eventType string

		if err := rows.Scan(
			&event.EventID,
			&event.AuthorizationID,
			&eventType,
			&event.EffectiveAt,
			&event.RecordedAt,
			&event.ActorID,
			&event.EvidenceRef,
			&event.ConditionsSatisfied,
			&event.CorrectsEventID,
			&event.CorrectionReason,
			&event.CorrectedField,
		); err != nil {
			return journal, err
		}

		event.EventType = authority.AuthorizationEvent(eventType)
		event.EffectiveAt = event.EffectiveAt.UTC()
		event.RecordedAt = event.RecordedAt.UTC()

		if err := journal.Append(event); err != nil {
			return journal, fmt.Errorf(
				"invalid persisted authority journal event %s: %w",
				event.EventID,
				err,
			)
		}
	}

	if err := rows.Err(); err != nil {
		return journal, err
	}

	return journal, nil
}

// loadAuthorizationJournalAtPosition reconstructs lifecycle evidence at a
// captured recording boundary while preserving journal sequence validation.
func loadAuthorizationJournalAtPosition(
	ctx context.Context,
	tx pgx.Tx,
	authorizationID string,
	position int64,
) (authority.AuthorizationJournal, error) {
	var journal authority.AuthorizationJournal
	if authorizationID == "" || position < 0 {
		return journal, errors.New("invalid journal recording boundary")
	}

	rows, err := tx.Query(ctx, `
        SELECT
            event_id,
            authorization_id,
            event_type,
            effective_at,
            recorded_at,
            actor_id,
            evidence_ref,
            conditions_satisfied,
            COALESCE(corrects_event_id, ''),
            COALESCE(correction_reason, ''),
            COALESCE(corrected_field, ''),
            recording_position
        FROM authority_journal_events
        WHERE authorization_id = $1
        ORDER BY journal_sequence ASC
    `, authorizationID)
	if err != nil {
		return journal, err
	}
	defer rows.Close()

	for rows.Next() {
		var event authority.AuthorizationJournalEvent
		var eventType string
		var recordedPosition *int64

		if err := rows.Scan(
			&event.EventID,
			&event.AuthorizationID,
			&eventType,
			&event.EffectiveAt,
			&event.RecordedAt,
			&event.ActorID,
			&event.EvidenceRef,
			&event.ConditionsSatisfied,
			&event.CorrectsEventID,
			&event.CorrectionReason,
			&event.CorrectedField,
			&recordedPosition,
		); err != nil {
			return journal, err
		}

		if recordedPosition == nil {
			return journal, fmt.Errorf(
				"journal event %s has no trustworthy recording position",
				event.EventID,
			)
		}
		if *recordedPosition > position {
			continue
		}

		event.EventType = authority.AuthorizationEvent(eventType)
		event.EffectiveAt = event.EffectiveAt.UTC()
		event.RecordedAt = event.RecordedAt.UTC()

		if err := journal.Append(event); err != nil {
			return journal, fmt.Errorf(
				"invalid persisted authority journal event %s: %w",
				event.EventID,
				err,
			)
		}
	}

	if err := rows.Err(); err != nil {
		return journal, err
	}
	return journal, nil
}

// AppendJournalEvent validates and persists an immutable journal event.
// The authorization row lock serializes concurrent writers.
func (r *AuthorityRepository) AppendJournalEvent(
	ctx context.Context,
	event authority.AuthorizationJournalEvent,
) error {
	return r.withLockedAuthorization(
		ctx,
		event.AuthorizationID,
		func(tx pgx.Tx, position int64) error {
			journal, err := loadAuthorizationJournal(
				ctx,
				tx,
				event.AuthorizationID,
			)
			if err != nil {
				return err
			}

			if err := journal.Append(event); err != nil {
				return fmt.Errorf("invalid authority journal event: %w", err)
			}

			_, err = tx.Exec(ctx, `
				INSERT INTO authority_journal_events (
					event_id,
					authorization_id,
					event_type,
					effective_at,
					recorded_at,
					actor_id,
					evidence_ref,
					conditions_satisfied,
					corrects_event_id,
					correction_reason,
					corrected_field, recording_position
				)
				VALUES (
					$1, $2, $3, $4, $5, $6, $7, $8,
					NULLIF($9, ''),
					NULLIF($10, ''),
					NULLIF($11, ''), $12
				)
			`,
				event.EventID,
				event.AuthorizationID,
				string(event.EventType),
				event.EffectiveAt,
				event.RecordedAt,
				event.ActorID,
				event.EvidenceRef,
				event.ConditionsSatisfied,
				event.CorrectsEventID,
				event.CorrectionReason,
				event.CorrectedField,
				position,
			)
			if err != nil {
				var pgErr *pgconn.PgError
				if errors.As(err, &pgErr) && pgErr.Code == "23505" {
					return fmt.Errorf(
						"%w: journal event already exists",
						ErrConflict,
					)
				}
				return err
			}

			return nil
		},
	)
}

// withAuthoritySnapshotTx executes a read-only, repeatable-read
// transaction so every authority evidence query observes one
// coherent PostgreSQL snapshot.
func (r *AuthorityRepository) withAuthoritySnapshotTx(
	ctx context.Context,
	fn func(pgx.Tx) error,
) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return fmt.Errorf("begin authority snapshot transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if err := fn(tx); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit authority snapshot transaction: %w", err)
	}
	return nil
}

// loadAuthorityRecordingBoundary captures the committed evidence position
// visible to the caller's consistent PostgreSQL snapshot.
func loadAuthorityRecordingBoundary(
	ctx context.Context,
	tx pgx.Tx,
) (int64, error) {
	var position int64
	err := tx.QueryRow(ctx, `
        SELECT committed_position
        FROM authority_recording_boundary
        WHERE singleton = TRUE
    `).Scan(&position)
	if err != nil {
		return 0, fmt.Errorf("read authority recording boundary: %w", err)
	}
	if position < 0 {
		return 0, errors.New("invalid authority recording boundary")
	}
	return position, nil
}

// loadAuthoritySnapshotObservedAt captures the database observation time.
// This is a wall-clock cutoff, not a translation of recording position.
func loadAuthoritySnapshotObservedAt(
	ctx context.Context,
	tx pgx.Tx,
) (time.Time, error) {
	var observedAt time.Time
	err := tx.QueryRow(ctx, `
        SELECT clock_timestamp()
    `).Scan(&observedAt)
	if err != nil {
		return time.Time{}, fmt.Errorf(
			"read authority snapshot observation time: %w", err,
		)
	}

	observedAt = observedAt.UTC().Truncate(time.Microsecond)
	if !validAuthorityDBTimestamp(observedAt) {
		return time.Time{}, errors.New(
			"invalid authority snapshot observation timestamp",
		)
	}
	return observedAt, nil
}

// loadAuthorityAdjudicationThrough retrieves the sole adjudication visible
// at the recorded-time cutoff. Missing or competing decisions fail closed.
func loadAuthorityAdjudicationThrough(
	ctx context.Context,
	tx pgx.Tx,
	authorizationID string,
	recordedThrough time.Time,
) (*authority.AuthorizationAdjudication, error) {
	if authorizationID == "" || !validAuthorityDBTimestamp(recordedThrough) {
		return nil, errors.New("invalid adjudication snapshot parameters")
	}

	rows, err := tx.Query(ctx, `
		SELECT
			decision_id, authorization_id, issuer_id,
			authority_subject_id, claim_type, outcome,
			decision_maker_id, authority_basis, evidence_ref,
			decided_at, recorded_at
		FROM authority_adjudications
		WHERE authorization_id = $1
		  AND recorded_at <= $2
		ORDER BY recorded_at ASC, decision_id ASC
		LIMIT 2
	`, authorizationID, recordedThrough)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var decision *authority.AuthorizationAdjudication
	for rows.Next() {
		if decision != nil {
			return nil, errors.New("ambiguous authority adjudication: multiple decisions")
		}

		var loaded authority.AuthorizationAdjudication
		var outcome string
		if err := rows.Scan(
			&loaded.DecisionID,
			&loaded.AuthorizationID,
			&loaded.IssuerID,
			&loaded.AuthoritySubjectID,
			&loaded.ClaimType,
			&outcome,
			&loaded.DecisionMakerID,
			&loaded.AuthorityBasis,
			&loaded.EvidenceRef,
			&loaded.DecidedAt,
			&loaded.RecordedAt,
		); err != nil {
			return nil, err
		}

		loaded.Outcome = authority.AdjudicationOutcome(outcome)
		loaded.DecidedAt = loaded.DecidedAt.UTC()
		loaded.RecordedAt = loaded.RecordedAt.UTC()
		decision = &loaded
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	if decision == nil {
		return nil, errors.New("missing authority adjudication")
	}
	return decision, nil
}

// loadAuthorityAdjudicationAtPosition retrieves the adjudication visible at
// a captured recording boundary. Legacy evidence without a position fails closed.
func loadAuthorityAdjudicationAtPosition(
	ctx context.Context,
	tx pgx.Tx,
	authorizationID string,
	position int64,
) (*authority.AuthorizationAdjudication, error) {
	if authorizationID == "" || position < 0 {
		return nil, errors.New("invalid adjudication recording boundary")
	}

	rows, err := tx.Query(ctx, `
        SELECT decision_id, authorization_id, issuer_id,
               authority_subject_id, claim_type, outcome,
               decision_maker_id, authority_basis, evidence_ref,
               decided_at, recorded_at, recording_position
        FROM authority_adjudications
        WHERE authorization_id = $1
        ORDER BY decision_id ASC
    `, authorizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var decision *authority.AuthorizationAdjudication
	for rows.Next() {
		var loaded authority.AuthorizationAdjudication
		var outcome string
		var recordedPosition *int64
		if err := rows.Scan(
			&loaded.DecisionID,
			&loaded.AuthorizationID,
			&loaded.IssuerID,
			&loaded.AuthoritySubjectID,
			&loaded.ClaimType,
			&outcome,
			&loaded.DecisionMakerID,
			&loaded.AuthorityBasis,
			&loaded.EvidenceRef,
			&loaded.DecidedAt,
			&loaded.RecordedAt,
			&recordedPosition,
		); err != nil {
			return nil, err
		}

		if recordedPosition == nil {
			return nil, fmt.Errorf(
				"adjudication %s has no trustworthy recording position",
				loaded.DecisionID,
			)
		}
		if *recordedPosition > position {
			continue
		}
		if decision != nil {
			return nil, errors.New("ambiguous authority adjudication: multiple decisions")
		}

		loaded.Outcome = authority.AdjudicationOutcome(outcome)
		loaded.DecidedAt = loaded.DecidedAt.UTC()
		loaded.RecordedAt = loaded.RecordedAt.UTC()
		decision = &loaded
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if decision == nil {
		return nil, errors.New("missing authority adjudication at recording boundary")
	}
	return decision, nil
}

// loadAuthorityRecords retrieves authorization scopes within the caller's
// transaction. Adjudication and lifecycle evidence are loaded separately.
func loadAuthorityRecords(
	ctx context.Context,
	tx pgx.Tx,
) ([]authority.AuthorizationRecord, error) {
	rows, err := tx.Query(ctx, `
		SELECT
			authorization_id,
			issuer_id,
			authority_subject_id,
			claim_type
		FROM authority_authorizations
		ORDER BY authorization_id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []authority.AuthorizationRecord

	for rows.Next() {
		var record authority.AuthorizationRecord

		if err := rows.Scan(
			&record.AuthorizationID,
			&record.IssuerID,
			&record.AuthoritySubjectID,
			&record.ClaimType,
		); err != nil {
			return nil, err
		}

		records = append(records, record)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return records, nil
}

// loadAuthorityRecordsAtPosition retrieves authorization scopes visible at
// a captured recording boundary. Unpositioned legacy evidence fails closed.
func loadAuthorityRecordsAtPosition(
	ctx context.Context,
	tx pgx.Tx,
	position int64,
) ([]authority.AuthorizationRecord, error) {
	if position < 0 {
		return nil, errors.New("invalid authority recording position")
	}

	rows, err := tx.Query(ctx, `
        SELECT authorization_id, issuer_id, authority_subject_id, claim_type,
               recording_position
        FROM authority_authorizations
        ORDER BY authorization_id ASC
    `)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []authority.AuthorizationRecord
	for rows.Next() {
		var record authority.AuthorizationRecord
		var recordedPosition *int64
		if err := rows.Scan(
			&record.AuthorizationID,
			&record.IssuerID,
			&record.AuthoritySubjectID,
			&record.ClaimType,
			&recordedPosition,
		); err != nil {
			return nil, err
		}
		if recordedPosition == nil {
			return nil, fmt.Errorf(
				"authorization %s has no trustworthy recording position",
				record.AuthorizationID,
			)
		}
		if *recordedPosition <= position {
			records = append(records, record)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

// loadAuthoritySnapshotEntries assembles persisted authorization evidence
// within a single caller-owned, consistent PostgreSQL transaction.
// It does not assert that the resulting registry snapshot is complete.
func loadAuthoritySnapshotEntries(
	ctx context.Context,
	tx pgx.Tx,
	recordedThrough time.Time,
) ([]authority.AuthoritySnapshotEntry, error) {
	if !validAuthorityDBTimestamp(recordedThrough) {
		return nil, errors.New("invalid authority snapshot cutoff")
	}

	records, err := loadAuthorityRecords(ctx, tx)
	if err != nil {
		return nil, fmt.Errorf("load authority records: %w", err)
	}

	entries := make([]authority.AuthoritySnapshotEntry, 0, len(records))

	for _, record := range records {
		decision, err := loadAuthorityAdjudicationThrough(
			ctx, tx, record.AuthorizationID, recordedThrough,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"load adjudication for authorization %s: %w",
				record.AuthorizationID, err,
			)
		}

		journal, err := loadAuthorizationJournalThrough(
			ctx, tx, record.AuthorizationID, &recordedThrough,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"load journal for authorization %s: %w",
				record.AuthorizationID, err,
			)
		}

		record.Adjudication = decision
		entries = append(entries, authority.AuthoritySnapshotEntry{
			Record:  record,
			Journal: journal,
		})
	}

	return entries, nil
}

// loadAuthoritySnapshotEntriesAtPosition assembles evidence visible at a
// captured recording boundary. The caller must use a consistent read snapshot.
// This function does not assign authoritative completeness or a wall-clock cutoff.
func loadAuthoritySnapshotEntriesAtPosition(
	ctx context.Context,
	tx pgx.Tx,
	position int64,
) ([]authority.AuthoritySnapshotEntry, error) {
	if position < 0 {
		return nil, errors.New("invalid authority snapshot recording boundary")
	}

	records, err := loadAuthorityRecordsAtPosition(ctx, tx, position)
	if err != nil {
		return nil, fmt.Errorf("load positioned authority records: %w", err)
	}

	entries := make([]authority.AuthoritySnapshotEntry, 0, len(records))

	for _, record := range records {
		decision, err := loadAuthorityAdjudicationAtPosition(
			ctx, tx, record.AuthorizationID, position,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"load positioned adjudication for authorization %s: %w",
				record.AuthorizationID, err,
			)
		}

		journal, err := loadAuthorizationJournalAtPosition(
			ctx, tx, record.AuthorizationID, position,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"load positioned journal for authorization %s: %w",
				record.AuthorizationID, err,
			)
		}

		record.Adjudication = decision
		entries = append(entries, authority.AuthoritySnapshotEntry{
			Record:  record,
			Journal: journal,
		})
	}

	return entries, nil
}

// validateAuthorityRecordingCompleteness verifies that every authority
// evidence row in the consistent database view has a trustworthy position
// within the captured committed boundary.
func validateAuthorityRecordingCompleteness(
	ctx context.Context,
	tx pgx.Tx,
	position int64,
) error {
	if position < 0 {
		return errors.New("invalid authority recording boundary")
	}

	var invalidCount int64
	err := tx.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(*) FROM authority_authorizations
			 WHERE recording_position IS NULL OR recording_position <= 0
			    OR recording_position > $1)
			+
			(SELECT COUNT(*) FROM authority_adjudications
			 WHERE recording_position IS NULL OR recording_position <= 0
			    OR recording_position > $1)
			+
			(SELECT COUNT(*) FROM authority_journal_events
			 WHERE recording_position IS NULL OR recording_position <= 0
			    OR recording_position > $1)
	`, position).Scan(&invalidCount)
	if err != nil {
		return fmt.Errorf("validate authority recording completeness: %w", err)
	}
	if invalidCount != 0 {
		return fmt.Errorf(
			"authority recording completeness cannot be established: %d untrusted evidence rows",
			invalidCount,
		)
	}

	var count, distinctCount, minPosition, maxPosition int64
	err = tx.QueryRow(ctx, `
		SELECT COUNT(*), COUNT(DISTINCT recording_position),
		       COALESCE(MIN(recording_position), 0),
		       COALESCE(MAX(recording_position), 0)
		FROM (
			SELECT recording_position FROM authority_authorizations
			UNION ALL
			SELECT recording_position FROM authority_adjudications
			UNION ALL
			SELECT recording_position FROM authority_journal_events
		) AS evidence
	`).Scan(&count, &distinctCount, &minPosition, &maxPosition)
	if err != nil {
		return fmt.Errorf("validate authority recording sequence: %w", err)
	}

	if count != position || distinctCount != position ||
		(position == 0 && (minPosition != 0 || maxPosition != 0)) ||
		(position > 0 && (minPosition != 1 || maxPosition != position)) {
		return fmt.Errorf(
			"authority recording sequence incomplete: boundary=%d count=%d distinct=%d min=%d max=%d",
			position, count, distinctCount, minPosition, maxPosition,
		)
	}
	return nil
}

// LoadAuthoritativeSnapshot constructs a complete authority evidence snapshot
// from one consistent PostgreSQL read view. Recording position controls
// evidence visibility; RecordedThrough controls domain evidence-time validity.
func (r *AuthorityRepository) LoadAuthoritativeSnapshot(
	ctx context.Context,
) (authority.AuthoritySnapshot, error) {
	var snapshot authority.AuthoritySnapshot

	err := r.withAuthoritySnapshotTx(ctx, func(tx pgx.Tx) error {
		// This first read establishes the repeatable-read snapshot.
		position, err := loadAuthorityRecordingBoundary(ctx, tx)
		if err != nil {
			return err
		}

		observedAt, err := loadAuthoritySnapshotObservedAt(ctx, tx)
		if err != nil {
			return err
		}

		if err := validateAuthorityRecordingCompleteness(
			ctx, tx, position,
		); err != nil {
			return err
		}

		entries, err := loadAuthoritySnapshotEntriesAtPosition(
			ctx, tx, position,
		)
		if err != nil {
			return err
		}

		candidate := authority.AuthoritySnapshot{
			Complete:          true,
			RecordingPosition: position,
			RecordedThrough:   observedAt,
			Entries:           entries,
		}

		// A complete registry view must also be valid for grant projection.
		// This rejects future-recorded or otherwise invalid evidence.
		if _, err := authority.NewSnapshotAuthorityResolver(candidate); err != nil {
			return fmt.Errorf("validate authoritative snapshot: %w", err)
		}

		snapshot = candidate
		return nil
	})
	if err != nil {
		return authority.AuthoritySnapshot{}, fmt.Errorf(
			"load authoritative authority snapshot: %w", err,
		)
	}

	return snapshot, nil
}
