package db

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/authority"
)

func openAuthorityTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	raw := os.Getenv("CVERA_AUTHORITY_TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("CVERA_AUTHORITY_TEST_DATABASE_URL is not set")
	}

	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("invalid test database URL: %v", err)
	}

	if u.Scheme != "postgres" ||
		net.ParseIP(u.Hostname()) == nil ||
		u.Hostname() != "127.0.0.1" ||
		u.Port() == "" ||
		u.Path != "/cvera_authority_test" ||
		u.User.Username() != "cvera_test" ||
		u.Query().Get("sslmode") != "disable" {
		t.Fatal("refusing connection to unexpected authority test database")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, raw)
	if err != nil {
		t.Fatalf("create test database pool: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("connect to authority test database: %v", err)
	}

	return pool
}

func TestAuthorityRepositoryPostgresConnection(t *testing.T) {
	pool := openAuthorityTestPool(t)

	var database, username string
	err := pool.QueryRow(
		context.Background(),
		"SELECT current_database(), current_user",
	).Scan(&database, &username)
	if err != nil {
		t.Fatalf("query database identity: %v", err)
	}

	if database != "cvera_authority_test" || username != "cvera_test" {
		t.Fatalf("unexpected database identity: %s / %s", database, username)
	}
}

func TestAuthorityRepositoryCreateAuthorization(t *testing.T) {
	pool := openIsolatedAuthoritySnapshotPool(t)
	ctx := context.Background()

	id := fmt.Sprintf("integration-auth-%d", time.Now().UnixNano())
	record := authority.AuthorizationRecord{
		AuthorizationID:    id,
		IssuerID:           "integration-issuer",
		AuthoritySubjectID: "integration-subject",
		ClaimType:          "employment",
	}

	repository := NewAuthorityRepository(pool)

	if err := repository.CreateAuthorization(ctx, record); err != nil {
		t.Fatalf("create authorization: %v", err)
	}

	var issuerID, subjectID, claimType string
	err := pool.QueryRow(ctx, `
		SELECT issuer_id, authority_subject_id, claim_type
		FROM authority_authorizations
		WHERE authorization_id = $1
	`, id).Scan(&issuerID, &subjectID, &claimType)
	if err != nil {
		t.Fatalf("read persisted authorization: %v", err)
	}

	if issuerID != record.IssuerID ||
		subjectID != record.AuthoritySubjectID ||
		claimType != record.ClaimType {
		t.Fatalf("persisted authorization scope does not match input")
	}
}

func TestAuthorityRepositoryRejectsDuplicateAuthorization(t *testing.T) {
	pool := openIsolatedAuthoritySnapshotPool(t)
	ctx := context.Background()

	record := authority.AuthorizationRecord{
		AuthorizationID:    fmt.Sprintf("integration-duplicate-%d", time.Now().UnixNano()),
		IssuerID:           "integration-issuer",
		AuthoritySubjectID: "integration-subject",
		ClaimType:          "employment",
	}

	repository := NewAuthorityRepository(pool)

	if err := repository.CreateAuthorization(ctx, record); err != nil {
		t.Fatalf("create initial authorization: %v", err)
	}

	err := repository.CreateAuthorization(ctx, record)
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict for duplicate authorization, got: %v", err)
	}
}

func TestAuthorityRepositoryCreateApprovedAdjudication(t *testing.T) {
	pool := openIsolatedAuthoritySnapshotPool(t)
	ctx := context.Background()
	repository := NewAuthorityRepository(pool)

	id := fmt.Sprintf("integration-adjudication-%d", time.Now().UnixNano())
	record := authority.AuthorizationRecord{
		AuthorizationID:    id,
		IssuerID:           "integration-issuer",
		AuthoritySubjectID: "integration-subject",
		ClaimType:          "employment",
	}

	if err := repository.CreateAuthorization(ctx, record); err != nil {
		t.Fatalf("create authorization: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	decision := authority.AuthorizationAdjudication{
		DecisionID:         id + "-decision",
		AuthorizationID:    record.AuthorizationID,
		IssuerID:           record.IssuerID,
		AuthoritySubjectID: record.AuthoritySubjectID,
		ClaimType:          record.ClaimType,
		Outcome:            authority.AdjudicationApproved,
		DecisionMakerID:    "integration-reviewer",
		AuthorityBasis:     "verified organizational authority",
		EvidenceRef:        "integration-evidence",
		DecidedAt:          now,
		RecordedAt:         now,
	}

	if err := repository.CreateAdjudication(ctx, decision); err != nil {
		t.Fatalf("create adjudication: %v", err)
	}

	var outcome, decisionMaker, authorityBasis, evidenceRef string
	err := pool.QueryRow(ctx, `
		SELECT outcome, decision_maker_id, authority_basis, evidence_ref
		FROM authority_adjudications
		WHERE decision_id = $1
	`, decision.DecisionID).Scan(
		&outcome, &decisionMaker, &authorityBasis, &evidenceRef,
	)
	if err != nil {
		t.Fatalf("read persisted adjudication: %v", err)
	}

	if outcome != string(decision.Outcome) ||
		decisionMaker != decision.DecisionMakerID ||
		authorityBasis != decision.AuthorityBasis ||
		evidenceRef != decision.EvidenceRef {
		t.Fatal("persisted adjudication evidence does not match input")
	}
}

func TestAuthorityRepositoryAppendSubmitEvent(t *testing.T) {
	pool := openIsolatedAuthoritySnapshotPool(t)
	ctx := context.Background()
	repository := NewAuthorityRepository(pool)

	id := fmt.Sprintf("integration-journal-%d", time.Now().UnixNano())
	record := authority.AuthorizationRecord{
		AuthorizationID:    id,
		IssuerID:           "integration-issuer",
		AuthoritySubjectID: "integration-subject",
		ClaimType:          "employment",
	}

	if err := repository.CreateAuthorization(ctx, record); err != nil {
		t.Fatalf("create authorization: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	event := authority.AuthorizationJournalEvent{
		EventID:             id + "-submit",
		AuthorizationID:     id,
		EventType:           authority.EventSubmit,
		EffectiveAt:         now,
		RecordedAt:          now,
		ActorID:             "integration-actor",
		EvidenceRef:         "integration-submit-evidence",
		ConditionsSatisfied: true,
	}

	if err := repository.AppendJournalEvent(ctx, event); err != nil {
		t.Fatalf("append SUBMIT event: %v", err)
	}

	var eventType, actorID, evidenceRef string
	var effectiveAt, recordedAt time.Time
	var conditionsSatisfied bool

	err := pool.QueryRow(ctx, `
		SELECT event_type, actor_id, evidence_ref,
		       effective_at, recorded_at, conditions_satisfied
		FROM authority_journal_events
		WHERE event_id = $1 AND authorization_id = $2
	`, event.EventID, id).Scan(
		&eventType, &actorID, &evidenceRef,
		&effectiveAt, &recordedAt, &conditionsSatisfied,
	)
	if err != nil {
		t.Fatalf("read persisted journal event: %v", err)
	}

	if eventType != string(event.EventType) ||
		actorID != event.ActorID ||
		evidenceRef != event.EvidenceRef ||
		!effectiveAt.Equal(event.EffectiveAt) ||
		!recordedAt.Equal(event.RecordedAt) ||
		conditionsSatisfied != event.ConditionsSatisfied {
		t.Fatal("persisted journal event does not match input")
	}
}

func TestAuthorityRepositoryRejectsInvalidTransitionWithoutWriting(t *testing.T) {
	pool := openIsolatedAuthoritySnapshotPool(t)
	ctx := context.Background()
	repository := NewAuthorityRepository(pool)

	id := fmt.Sprintf("integration-invalid-transition-%d", time.Now().UnixNano())
	record := authority.AuthorizationRecord{
		AuthorizationID:    id,
		IssuerID:           "integration-issuer",
		AuthoritySubjectID: "integration-subject",
		ClaimType:          "employment",
	}

	if err := repository.CreateAuthorization(ctx, record); err != nil {
		t.Fatalf("create authorization: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	event := authority.AuthorizationJournalEvent{
		EventID:             id + "-invalid-activate",
		AuthorizationID:     id,
		EventType:           authority.EventActivate,
		EffectiveAt:         now,
		RecordedAt:          now,
		ActorID:             "integration-actor",
		EvidenceRef:         "integration-invalid-evidence",
		ConditionsSatisfied: true,
	}

	if err := repository.AppendJournalEvent(ctx, event); err == nil {
		t.Fatal("expected invalid ACTIVATE transition to be rejected")
	}

	var count int
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM authority_journal_events
		WHERE authorization_id = $1
	`, id).Scan(&count)
	if err != nil {
		t.Fatalf("count persisted journal events: %v", err)
	}

	if count != 0 {
		t.Fatalf("invalid transition persisted %d journal events, expected zero", count)
	}
}

func TestAuthorityRepositoryRollsBackAfterInsert(t *testing.T) {
	pool := openIsolatedAuthoritySnapshotPool(t)
	ctx := context.Background()
	repository := NewAuthorityRepository(pool)

	id := fmt.Sprintf("integration-rollback-%d", time.Now().UnixNano())
	record := authority.AuthorizationRecord{
		AuthorizationID:    id,
		IssuerID:           "integration-issuer",
		AuthoritySubjectID: "integration-subject",
		ClaimType:          "employment",
	}

	if err := repository.CreateAuthorization(ctx, record); err != nil {
		t.Fatalf("create authorization: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	deliberateFailure := errors.New("deliberate rollback test failure")

	err := repository.withLockedAuthorization(ctx, id, func(tx pgx.Tx, _ int64) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO authority_journal_events (
				event_id, authorization_id, event_type,
				effective_at, recorded_at, actor_id,
				evidence_ref, conditions_satisfied
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`,
			id+"-submit",
			id,
			string(authority.EventSubmit),
			now,
			now,
			"integration-actor",
			"integration-rollback-evidence",
			true,
		)
		if err != nil {
			return err
		}

		return deliberateFailure
	})

	if !errors.Is(err, deliberateFailure) {
		t.Fatalf("expected deliberate failure, got: %v", err)
	}

	var count int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM authority_journal_events
		WHERE authorization_id = $1
	`, id).Scan(&count); err != nil {
		t.Fatalf("count persisted journal events: %v", err)
	}

	if count != 0 {
		t.Fatalf("rollback left %d journal events, expected zero", count)
	}
}

func TestAuthorityRepositoryAuthorizationImmutability(t *testing.T) {
	pool := openIsolatedAuthoritySnapshotPool(t)
	ctx := context.Background()
	repository := NewAuthorityRepository(pool)

	id := fmt.Sprintf("integration-immutable-%d", time.Now().UnixNano())
	record := authority.AuthorizationRecord{
		AuthorizationID:    id,
		IssuerID:           "integration-issuer",
		AuthoritySubjectID: "integration-subject",
		ClaimType:          "employment",
	}

	if err := repository.CreateAuthorization(ctx, record); err != nil {
		t.Fatalf("create authorization: %v", err)
	}

	_, updateErr := pool.Exec(ctx, `
		UPDATE authority_authorizations
		SET issuer_id = 'unauthorized-change'
		WHERE authorization_id = $1
	`, id)
	if updateErr == nil {
		t.Fatal("expected UPDATE to be rejected by immutability trigger")
	}
	if !strings.Contains(updateErr.Error(), "append-only: UPDATE on authority_authorizations is prohibited") {
		t.Fatalf("unexpected UPDATE rejection: %v", updateErr)
	}

	_, deleteErr := pool.Exec(ctx, `
		DELETE FROM authority_authorizations
		WHERE authorization_id = $1
	`, id)
	if deleteErr == nil {
		t.Fatal("expected DELETE to be rejected by immutability trigger")
	}
	if !strings.Contains(deleteErr.Error(), "append-only: DELETE on authority_authorizations is prohibited") {
		t.Fatalf("unexpected DELETE rejection: %v", deleteErr)
	}

	var issuerID string
	if err := pool.QueryRow(ctx, `
		SELECT issuer_id
		FROM authority_authorizations
		WHERE authorization_id = $1
	`, id).Scan(&issuerID); err != nil {
		t.Fatalf("read preserved authorization: %v", err)
	}

	if issuerID != record.IssuerID {
		t.Fatalf("authorization changed: got issuer %q, want %q", issuerID, record.IssuerID)
	}
}

func TestAuthorityRepositoryAdjudicationImmutability(t *testing.T) {
	pool := openIsolatedAuthoritySnapshotPool(t)
	ctx := context.Background()
	repository := NewAuthorityRepository(pool)

	id := fmt.Sprintf("integration-immutable-decision-%d", time.Now().UnixNano())
	record := authority.AuthorizationRecord{
		AuthorizationID:    id,
		IssuerID:           "integration-issuer",
		AuthoritySubjectID: "integration-subject",
		ClaimType:          "employment",
	}
	if err := repository.CreateAuthorization(ctx, record); err != nil {
		t.Fatalf("create authorization: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	decision := authority.AuthorizationAdjudication{
		DecisionID:         id + "-decision",
		AuthorizationID:    id,
		IssuerID:           record.IssuerID,
		AuthoritySubjectID: record.AuthoritySubjectID,
		ClaimType:          record.ClaimType,
		Outcome:            authority.AdjudicationApproved,
		DecisionMakerID:    "integration-decision-maker",
		AuthorityBasis:     "integration-authority-basis",
		EvidenceRef:        "integration-decision-evidence",
		DecidedAt:          now,
		RecordedAt:         now,
	}
	if err := repository.CreateAdjudication(ctx, decision); err != nil {
		t.Fatalf("create adjudication: %v", err)
	}

	_, updateErr := pool.Exec(ctx, `
		UPDATE authority_adjudications
		SET authority_basis = 'unauthorized-change'
		WHERE decision_id = $1
	`, decision.DecisionID)
	if updateErr == nil || !strings.Contains(updateErr.Error(), "append-only: UPDATE on authority_adjudications is prohibited") {
		t.Fatalf("expected adjudication immutability UPDATE error, got: %v", updateErr)
	}

	_, deleteErr := pool.Exec(ctx, `
		DELETE FROM authority_adjudications
		WHERE decision_id = $1
	`, decision.DecisionID)
	if deleteErr == nil || !strings.Contains(deleteErr.Error(), "append-only: DELETE on authority_adjudications is prohibited") {
		t.Fatalf("expected adjudication immutability DELETE error, got: %v", deleteErr)
	}

	var basis string
	if err := pool.QueryRow(ctx, `
		SELECT authority_basis
		FROM authority_adjudications
		WHERE decision_id = $1
	`, decision.DecisionID).Scan(&basis); err != nil {
		t.Fatalf("read preserved adjudication: %v", err)
	}
	if basis != decision.AuthorityBasis {
		t.Fatalf("adjudication changed: got basis %q, want %q", basis, decision.AuthorityBasis)
	}
}

func TestAuthorityRepositoryJournalImmutability(t *testing.T) {
	pool := openIsolatedAuthoritySnapshotPool(t)
	ctx := context.Background()
	repository := NewAuthorityRepository(pool)

	id := fmt.Sprintf("integration-immutable-journal-%d", time.Now().UnixNano())
	record := authority.AuthorizationRecord{
		AuthorizationID:    id,
		IssuerID:           "integration-issuer",
		AuthoritySubjectID: "integration-subject",
		ClaimType:          "employment",
	}
	if err := repository.CreateAuthorization(ctx, record); err != nil {
		t.Fatalf("create authorization: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	event := authority.AuthorizationJournalEvent{
		EventID:             id + "-submit",
		AuthorizationID:     id,
		EventType:           authority.EventSubmit,
		EffectiveAt:         now,
		RecordedAt:          now,
		ActorID:             "integration-actor",
		EvidenceRef:         "integration-journal-evidence",
		ConditionsSatisfied: true,
	}
	if err := repository.AppendJournalEvent(ctx, event); err != nil {
		t.Fatalf("append journal event: %v", err)
	}

	_, updateErr := pool.Exec(ctx, `
		UPDATE authority_journal_events
		SET evidence_ref = 'unauthorized-change'
		WHERE event_id = $1
	`, event.EventID)
	if updateErr == nil || !strings.Contains(updateErr.Error(), "append-only: UPDATE on authority_journal_events is prohibited") {
		t.Fatalf("expected journal immutability UPDATE error, got: %v", updateErr)
	}

	_, deleteErr := pool.Exec(ctx, `
		DELETE FROM authority_journal_events
		WHERE event_id = $1
	`, event.EventID)
	if deleteErr == nil || !strings.Contains(deleteErr.Error(), "append-only: DELETE on authority_journal_events is prohibited") {
		t.Fatalf("expected journal immutability DELETE error, got: %v", deleteErr)
	}

	var evidenceRef string
	if err := pool.QueryRow(ctx, `
		SELECT evidence_ref
		FROM authority_journal_events
		WHERE event_id = $1
	`, event.EventID).Scan(&evidenceRef); err != nil {
		t.Fatalf("read preserved journal event: %v", err)
	}
	if evidenceRef != event.EvidenceRef {
		t.Fatalf("journal event changed: got evidence %q, want %q", evidenceRef, event.EvidenceRef)
	}
}

func TestAuthorityRepositoryRejectsMismatchedAdjudicationScope(t *testing.T) {
	pool := openIsolatedAuthoritySnapshotPool(t)
	ctx := context.Background()
	repository := NewAuthorityRepository(pool)

	id := fmt.Sprintf("integration-scope-%d", time.Now().UnixNano())
	record := authority.AuthorizationRecord{
		AuthorizationID:    id,
		IssuerID:           "integration-issuer",
		AuthoritySubjectID: "integration-subject",
		ClaimType:          "employment",
	}
	if err := repository.CreateAuthorization(ctx, record); err != nil {
		t.Fatalf("create authorization: %v", err)
	}

	cases := []struct {
		name      string
		issuer    string
		subject   string
		claimType string
	}{
		{"issuer", "wrong-issuer", record.AuthoritySubjectID, record.ClaimType},
		{"subject", record.IssuerID, "wrong-subject", record.ClaimType},
		{"claim-type", record.IssuerID, record.AuthoritySubjectID, "education"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Now().UTC().Truncate(time.Microsecond)
			decision := authority.AuthorizationAdjudication{
				DecisionID:         id + "-" + tc.name,
				AuthorizationID:    id,
				IssuerID:           tc.issuer,
				AuthoritySubjectID: tc.subject,
				ClaimType:          tc.claimType,
				Outcome:            authority.AdjudicationApproved,
				DecisionMakerID:    "integration-decision-maker",
				AuthorityBasis:     "integration-authority-basis",
				EvidenceRef:        "integration-scope-evidence",
				DecidedAt:          now,
				RecordedAt:         now,
			}

			err := repository.CreateAdjudication(ctx, decision)
			if err == nil {
				t.Fatal("expected mismatched adjudication scope to be rejected")
			}

			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) {
				t.Fatalf("expected PostgreSQL constraint error, got: %v", err)
			}
			if pgErr.Code != "23503" || pgErr.ConstraintName != "authority_adjudications_scope_fk" {
				t.Fatalf("unexpected constraint error: code=%s constraint=%s",
					pgErr.Code, pgErr.ConstraintName)
			}

			var count int
			if err := pool.QueryRow(ctx, `
				SELECT COUNT(*)
				FROM authority_adjudications
				WHERE decision_id = $1
			`, decision.DecisionID).Scan(&count); err != nil {
				t.Fatalf("count adjudications: %v", err)
			}
			if count != 0 {
				t.Fatalf("mismatched adjudication was persisted: count=%d", count)
			}
		})
	}
}

func TestAuthorityRepositoryRejectsCrossAuthorizationCorrection(t *testing.T) {
	pool := openIsolatedAuthoritySnapshotPool(t)
	ctx := context.Background()
	repository := NewAuthorityRepository(pool)

	base := fmt.Sprintf("integration-cross-correction-%d", time.Now().UnixNano())
	now := time.Now().UTC().Truncate(time.Microsecond)

	for _, suffix := range []string{"a", "b"} {
		id := base + "-" + suffix
		record := authority.AuthorizationRecord{
			AuthorizationID:    id,
			IssuerID:           "integration-issuer",
			AuthoritySubjectID: "integration-subject-" + suffix,
			ClaimType:          "employment",
		}
		if err := repository.CreateAuthorization(ctx, record); err != nil {
			t.Fatalf("create authorization %s: %v", suffix, err)
		}

		event := authority.AuthorizationJournalEvent{
			EventID:             id + "-submit",
			AuthorizationID:     id,
			EventType:           authority.EventSubmit,
			EffectiveAt:         now,
			RecordedAt:          now,
			ActorID:             "integration-actor",
			EvidenceRef:         "integration-evidence-" + suffix,
			ConditionsSatisfied: true,
		}
		if err := repository.AppendJournalEvent(ctx, event); err != nil {
			t.Fatalf("append SUBMIT for %s: %v", suffix, err)
		}
	}

	_, err := pool.Exec(ctx, `
		INSERT INTO authority_journal_events (
			event_id, authorization_id, event_type,
			effective_at, recorded_at, actor_id,
			evidence_ref, conditions_satisfied,
			corrects_event_id, correction_reason, corrected_field
		)
		VALUES ($1, $2, 'SUBMIT', $3, $4, $5, $6, TRUE, $7, $8, 'EffectiveAt')
	`,
		base+"-invalid-correction",
		base+"-b",
		now,
		now,
		"integration-actor",
		"integration-correction-evidence",
		base+"-a-submit",
		"integration-test-correction",
	)

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("expected PostgreSQL foreign key violation, got: %v", err)
	}
	if pgErr.Code != "23503" ||
		pgErr.ConstraintName != "authority_journal_correction_reference_fk" {
		t.Fatalf("unexpected constraint error: code=%s constraint=%s",
			pgErr.Code, pgErr.ConstraintName)
	}

	var count int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM authority_journal_events
		WHERE event_id = $1
	`, base+"-invalid-correction").Scan(&count); err != nil {
		t.Fatalf("count invalid corrections: %v", err)
	}
	if count != 0 {
		t.Fatalf("cross-authorization correction was persisted: count=%d", count)
	}
}

func TestAuthorityRepositorySerializesConcurrentSubmit(t *testing.T) {
	pool := openIsolatedAuthoritySnapshotPool(t)
	ctx := context.Background()
	repository := NewAuthorityRepository(pool)

	id := fmt.Sprintf("integration-concurrent-%d", time.Now().UnixNano())
	record := authority.AuthorizationRecord{
		AuthorizationID:    id,
		IssuerID:           "integration-issuer",
		AuthoritySubjectID: "integration-subject",
		ClaimType:          "employment",
	}
	if err := repository.CreateAuthorization(ctx, record); err != nil {
		t.Fatalf("create authorization: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	start := make(chan struct{})
	results := make(chan error, 2)

	for i := 0; i < 2; i++ {
		go func(index int) {
			event := authority.AuthorizationJournalEvent{
				EventID:             fmt.Sprintf("%s-submit-%d", id, index),
				AuthorizationID:     id,
				EventType:           authority.EventSubmit,
				EffectiveAt:         now,
				RecordedAt:          now,
				ActorID:             "integration-actor",
				EvidenceRef:         fmt.Sprintf("integration-evidence-%d", index),
				ConditionsSatisfied: true,
			}
			<-start
			results <- repository.AppendJournalEvent(ctx, event)
		}(i)
	}

	close(start)

	successes := 0
	rejections := 0
	for i := 0; i < 2; i++ {
		err := <-results
		if err == nil {
			successes++
		} else {
			rejections++
		}
	}

	if successes != 1 || rejections != 1 {
		t.Fatalf("expected one successful append and one rejection; got successes=%d rejections=%d",
			successes, rejections)
	}

	var count int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM authority_journal_events
		WHERE authorization_id = $1
	`, id).Scan(&count); err != nil {
		t.Fatalf("count journal events: %v", err)
	}

	if count != 1 {
		t.Fatalf("expected exactly one persisted SUBMIT event, got %d", count)
	}
}

func TestAuthorityRepositoryAuthorizationRowLockContention(t *testing.T) {
	pool := openIsolatedAuthoritySnapshotPool(t)
	ctx := context.Background()
	repository := NewAuthorityRepository(pool)

	id := fmt.Sprintf("integration-row-lock-%d", time.Now().UnixNano())
	record := authority.AuthorizationRecord{
		AuthorizationID:    id,
		IssuerID:           "integration-issuer",
		AuthoritySubjectID: "integration-subject",
		ClaimType:          "employment",
	}
	if err := repository.CreateAuthorization(ctx, record); err != nil {
		t.Fatalf("create authorization: %v", err)
	}

	firstTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin first transaction: %v", err)
	}
	defer func() { _ = firstTx.Rollback(ctx) }()

	var lockedID string
	if err := firstTx.QueryRow(ctx, `
		SELECT authorization_id
		FROM authority_authorizations
		WHERE authorization_id = $1
		FOR UPDATE
	`, id).Scan(&lockedID); err != nil {
		t.Fatalf("acquire first row lock: %v", err)
	}

	secondTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin second transaction: %v", err)
	}
	defer func() { _ = secondTx.Rollback(ctx) }()

	var secondID string
	err = secondTx.QueryRow(ctx, `
		SELECT authorization_id
		FROM authority_authorizations
		WHERE authorization_id = $1
		FOR UPDATE NOWAIT
	`, id).Scan(&secondID)

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("expected PostgreSQL lock contention error, got: %v", err)
	}
	if pgErr.Code != "55P03" {
		t.Fatalf("expected SQLSTATE 55P03, got %s: %v", pgErr.Code, err)
	}
}

func TestAuthorityRepositoryAppendRespectsHeldAuthorizationLock(t *testing.T) {
	pool := openIsolatedAuthoritySnapshotPool(t)
	ctx := context.Background()
	repository := NewAuthorityRepository(pool)

	id := fmt.Sprintf("integration-append-lock-%d", time.Now().UnixNano())
	record := authority.AuthorizationRecord{
		AuthorizationID:    id,
		IssuerID:           "integration-issuer",
		AuthoritySubjectID: "integration-subject",
		ClaimType:          "employment",
	}
	if err := repository.CreateAuthorization(ctx, record); err != nil {
		t.Fatalf("create authorization: %v", err)
	}

	lockTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin locking transaction: %v", err)
	}
	defer func() { _ = lockTx.Rollback(ctx) }()

	var lockedID string
	if err := lockTx.QueryRow(ctx, `
		SELECT authorization_id
		FROM authority_authorizations
		WHERE authorization_id = $1
		FOR UPDATE
	`, id).Scan(&lockedID); err != nil {
		t.Fatalf("lock authorization: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	event := authority.AuthorizationJournalEvent{
		EventID:             id + "-submit",
		AuthorizationID:     id,
		EventType:           authority.EventSubmit,
		EffectiveAt:         now,
		RecordedAt:          now,
		ActorID:             "integration-actor",
		EvidenceRef:         "integration-lock-evidence",
		ConditionsSatisfied: true,
	}

	// Use a separate connection so the append cannot inherit the lock.
	appendCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()

	err = repository.AppendJournalEvent(appendCtx, event)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context deadline while authorization lock was held, got: %v", err)
	}

	var count int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM authority_journal_events
		WHERE authorization_id = $1
	`, id).Scan(&count); err != nil {
		t.Fatalf("count events after blocked append: %v", err)
	}
	if count != 0 {
		t.Fatalf("blocked append persisted %d events", count)
	}

	if err := lockTx.Commit(ctx); err != nil {
		t.Fatalf("release authorization lock: %v", err)
	}

	if err := repository.AppendJournalEvent(ctx, event); err != nil {
		t.Fatalf("append after releasing lock: %v", err)
	}

	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM authority_journal_events
		WHERE authorization_id = $1
	`, id).Scan(&count); err != nil {
		t.Fatalf("count events after successful append: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one event after releasing lock, got %d", count)
	}
}

func TestAuthorityRepositorySnapshotTransactionProperties(t *testing.T) {
	pool := openIsolatedAuthoritySnapshotPool(t)
	repository := NewAuthorityRepository(pool)
	ctx := context.Background()

	err := repository.withAuthoritySnapshotTx(ctx, func(tx pgx.Tx) error {
		var isolationLevel string
		var readOnly string

		if err := tx.QueryRow(ctx, `
			SELECT current_setting('transaction_isolation'),
			       current_setting('transaction_read_only')
		`).Scan(&isolationLevel, &readOnly); err != nil {
			return err
		}

		if isolationLevel != "repeatable read" {
			t.Errorf("isolation level = %q, want repeatable read", isolationLevel)
		}
		if readOnly != "on" {
			t.Errorf("transaction_read_only = %q, want on", readOnly)
		}

		_, err := tx.Exec(ctx, `
			INSERT INTO authority_authorizations (
				authorization_id, issuer_id,
				authority_subject_id, claim_type
			)
			VALUES (
				'readonly-probe', 'integration-issuer',
				'integration-subject', 'employment'
			)
		`)
		if err == nil {
			t.Error("read-only transaction unexpectedly permitted an INSERT")
		} else {
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != "25006" {
				t.Errorf("expected SQLSTATE 25006 (read-only violation), got: %v", err)
			}
		}

		// The rejected INSERT aborts the transaction. Return its error
		// so the helper rolls back rather than attempting to commit.
		return err
	})

	if err == nil {
		t.Fatal("expected read-only transaction to reject the INSERT")
	}
}

func TestAuthorityRepositorySnapshotRemainsStableAcrossConcurrentInsert(t *testing.T) {
	pool := openIsolatedAuthoritySnapshotPool(t)
	repository := NewAuthorityRepository(pool)
	ctx := context.Background()

	id := fmt.Sprintf("integration-snapshot-%d", time.Now().UnixNano())

	err := repository.withAuthoritySnapshotTx(ctx, func(tx pgx.Tx) error {
		var before int
		if err := tx.QueryRow(ctx, `
			SELECT COUNT(*)
			FROM authority_authorizations
			WHERE authorization_id = $1
		`, id).Scan(&before); err != nil {
			return err
		}
		if before != 0 {
			t.Fatalf("expected authorization to be absent initially, got %d", before)
		}

		if err := repository.CreateAuthorization(ctx, authority.AuthorizationRecord{
			AuthorizationID:    id,
			IssuerID:           "integration-issuer",
			AuthoritySubjectID: "integration-subject",
			ClaimType:          "employment",
		}); err != nil {
			return fmt.Errorf("concurrent authorization insert: %w", err)
		}

		var during int
		if err := tx.QueryRow(ctx, `
			SELECT COUNT(*)
			FROM authority_authorizations
			WHERE authorization_id = $1
		`, id).Scan(&during); err != nil {
			return err
		}
		if during != 0 {
			t.Fatalf("snapshot observed concurrent committed insert: count=%d", during)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("snapshot transaction: %v", err)
	}

	var after int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM authority_authorizations
		WHERE authorization_id = $1
	`, id).Scan(&after); err != nil {
		t.Fatalf("query after snapshot: %v", err)
	}
	if after != 1 {
		t.Fatalf("expected committed authorization after snapshot, got %d", after)
	}
}

func TestAuthorityRepositoryHistoricalJournalCutoff(t *testing.T) {
	pool := openIsolatedAuthoritySnapshotPool(t)
	repository := NewAuthorityRepository(pool)
	ctx := context.Background()

	id := fmt.Sprintf("integration-history-%d", time.Now().UnixNano())
	if err := repository.CreateAuthorization(ctx, authority.AuthorizationRecord{
		AuthorizationID:    id,
		IssuerID:           "integration-issuer",
		AuthoritySubjectID: "integration-subject",
		ClaimType:          "employment",
	}); err != nil {
		t.Fatalf("create authorization: %v", err)
	}

	start := time.Now().UTC().Truncate(time.Microsecond)

	events := []authority.AuthorizationJournalEvent{
		{
			EventID: id + "-submit", AuthorizationID: id,
			EventType:   authority.EventSubmit,
			EffectiveAt: start, RecordedAt: start,
			ActorID: "integration-actor", EvidenceRef: "submit-evidence",
			ConditionsSatisfied: true,
		},
		{
			EventID: id + "-approve", AuthorizationID: id,
			EventType:   authority.EventApprove,
			EffectiveAt: start.Add(time.Minute),
			RecordedAt:  start.Add(time.Minute),
			ActorID:     "integration-actor", EvidenceRef: "approval-evidence",
			ConditionsSatisfied: true,
		},
	}

	for _, event := range events {
		if err := repository.AppendJournalEvent(ctx, event); err != nil {
			t.Fatalf("append %s: %v", event.EventID, err)
		}
	}

	err := repository.withAuthoritySnapshotTx(ctx, func(tx pgx.Tx) error {
		historical, err := loadAuthorizationJournalThrough(ctx, tx, id, &start)
		if err != nil {
			return err
		}
		historicalEvents := historical.Events()
		if len(historicalEvents) != 1 ||
			historicalEvents[0].EventID != id+"-submit" {
			t.Errorf("historical events = %+v, want only SUBMIT", historicalEvents)
		}

		complete, err := loadAuthorizationJournal(ctx, tx, id)
		if err != nil {
			return err
		}
		if len(complete.Events()) != 2 {
			t.Errorf("complete journal contains %d events, want 2", len(complete.Events()))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("load historical journal: %v", err)
	}
}

func TestAuthorityRepositoryHistoricalAdjudicationMissingFailsClosed(t *testing.T) {
	pool := openIsolatedAuthoritySnapshotPool(t)
	repository := NewAuthorityRepository(pool)
	ctx := context.Background()

	id := fmt.Sprintf("integration-missing-decision-%d", time.Now().UnixNano())

	if err := repository.CreateAuthorization(ctx, authority.AuthorizationRecord{
		AuthorizationID:    id,
		IssuerID:           "integration-issuer",
		AuthoritySubjectID: "integration-subject",
		ClaimType:          "employment",
	}); err != nil {
		t.Fatalf("create authorization: %v", err)
	}

	cutoff := time.Now().UTC().Truncate(time.Microsecond)

	err := repository.withAuthoritySnapshotTx(ctx, func(tx pgx.Tx) error {
		decision, err := loadAuthorityAdjudicationThrough(ctx, tx, id, cutoff)

		if err == nil {
			t.Fatalf("expected missing adjudication to fail closed; decision=%+v", decision)
		}
		if decision != nil {
			t.Fatalf("failed adjudication lookup returned a decision: %+v", decision)
		}
		if !strings.Contains(err.Error(), "missing authority adjudication") {
			t.Fatalf("expected missing adjudication error, got: %v", err)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("snapshot transaction: %v", err)
	}
}

func TestAuthorityRepositoryHistoricalAdjudicationSingleDecision(t *testing.T) {
	pool := openIsolatedAuthoritySnapshotPool(t)
	repository := NewAuthorityRepository(pool)
	ctx := context.Background()

	id := fmt.Sprintf("integration-single-decision-%d", time.Now().UnixNano())
	record := authority.AuthorizationRecord{
		AuthorizationID:    id,
		IssuerID:           "integration-issuer",
		AuthoritySubjectID: "integration-subject",
		ClaimType:          "employment",
	}
	if err := repository.CreateAuthorization(ctx, record); err != nil {
		t.Fatalf("create authorization: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	expected := authority.AuthorizationAdjudication{
		DecisionID:         id + "-decision",
		AuthorizationID:    id,
		IssuerID:           record.IssuerID,
		AuthoritySubjectID: record.AuthoritySubjectID,
		ClaimType:          record.ClaimType,
		Outcome:            authority.AdjudicationApproved,
		DecisionMakerID:    "integration-adjudicator",
		AuthorityBasis:     "verified organizational delegation",
		EvidenceRef:        "integration-approval-evidence",
		DecidedAt:          now,
		RecordedAt:         now,
	}
	if err := repository.CreateAdjudication(ctx, expected); err != nil {
		t.Fatalf("create adjudication: %v", err)
	}

	err := repository.withAuthoritySnapshotTx(ctx, func(tx pgx.Tx) error {
		actual, err := loadAuthorityAdjudicationThrough(ctx, tx, id, now)
		if err != nil {
			return err
		}
		if actual == nil {
			t.Fatal("expected adjudication, got nil")
		}
		if *actual != expected {
			t.Errorf("loaded adjudication = %+v, want %+v", *actual, expected)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot transaction: %v", err)
	}
}

func TestAuthorityRepositoryHistoricalAdjudicationExcludesLaterDecision(t *testing.T) {
	pool := openIsolatedAuthoritySnapshotPool(t)
	repository := NewAuthorityRepository(pool)
	ctx := context.Background()

	id := fmt.Sprintf("integration-future-decision-%d", time.Now().UnixNano())
	record := authority.AuthorizationRecord{
		AuthorizationID:    id,
		IssuerID:           "integration-issuer",
		AuthoritySubjectID: "integration-subject",
		ClaimType:          "employment",
	}
	if err := repository.CreateAuthorization(ctx, record); err != nil {
		t.Fatalf("create authorization: %v", err)
	}

	cutoff := time.Now().UTC().Truncate(time.Microsecond)
	later := cutoff.Add(time.Minute)

	decision := authority.AuthorizationAdjudication{
		DecisionID:         id + "-decision",
		AuthorizationID:    id,
		IssuerID:           record.IssuerID,
		AuthoritySubjectID: record.AuthoritySubjectID,
		ClaimType:          record.ClaimType,
		Outcome:            authority.AdjudicationApproved,
		DecisionMakerID:    "integration-adjudicator",
		AuthorityBasis:     "verified organizational delegation",
		EvidenceRef:        "integration-future-evidence",
		DecidedAt:          later,
		RecordedAt:         later,
	}
	if err := repository.CreateAdjudication(ctx, decision); err != nil {
		t.Fatalf("create later adjudication: %v", err)
	}

	err := repository.withAuthoritySnapshotTx(ctx, func(tx pgx.Tx) error {
		actual, err := loadAuthorityAdjudicationThrough(ctx, tx, id, cutoff)
		if err == nil || actual != nil {
			t.Fatalf("later adjudication visible at earlier cutoff: decision=%+v, err=%v", actual, err)
		}
		if !strings.Contains(err.Error(), "missing authority adjudication") {
			t.Fatalf("expected missing adjudication at earlier cutoff, got: %v", err)
		}

		actual, err = loadAuthorityAdjudicationThrough(ctx, tx, id, later)
		if err != nil {
			return err
		}
		if actual == nil || actual.DecisionID != decision.DecisionID {
			t.Errorf("decision at later cutoff = %+v, want %s", actual, decision.DecisionID)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot transaction: %v", err)
	}
}

func TestAuthorityRepositoryHistoricalAdjudicationMultipleDecisionsFailClosed(t *testing.T) {
	pool := openIsolatedAuthoritySnapshotPool(t)
	repository := NewAuthorityRepository(pool)
	ctx := context.Background()

	id := fmt.Sprintf("integration-competing-decisions-%d", time.Now().UnixNano())
	record := authority.AuthorizationRecord{
		AuthorizationID:    id,
		IssuerID:           "integration-issuer",
		AuthoritySubjectID: "integration-subject",
		ClaimType:          "employment",
	}
	if err := repository.CreateAuthorization(ctx, record); err != nil {
		t.Fatalf("create authorization: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)

	first := authority.AuthorizationAdjudication{
		DecisionID:         id + "-first",
		AuthorizationID:    id,
		IssuerID:           record.IssuerID,
		AuthoritySubjectID: record.AuthoritySubjectID,
		ClaimType:          record.ClaimType,
		Outcome:            authority.AdjudicationApproved,
		DecisionMakerID:    "integration-adjudicator",
		AuthorityBasis:     "verified organizational delegation",
		EvidenceRef:        "integration-first-evidence",
		DecidedAt:          now,
		RecordedAt:         now,
	}
	if err := repository.CreateAdjudication(ctx, first); err != nil {
		t.Fatalf("create first adjudication: %v", err)
	}

	second := first
	second.DecisionID = id + "-second"
	second.Outcome = authority.AdjudicationRejected
	second.EvidenceRef = "integration-second-evidence"
	second.DecidedAt = now.Add(time.Minute)
	second.RecordedAt = now.Add(time.Minute)

	if err := repository.CreateAdjudication(ctx, second); err != nil {
		t.Fatalf("create second adjudication: %v", err)
	}

	err := repository.withAuthoritySnapshotTx(ctx, func(tx pgx.Tx) error {
		earlier, err := loadAuthorityAdjudicationThrough(ctx, tx, id, now)
		if err != nil {
			return fmt.Errorf("earlier cutoff: %w", err)
		}
		if earlier == nil || earlier.DecisionID != first.DecisionID {
			t.Fatalf("earlier cutoff decision = %+v, want %s", earlier, first.DecisionID)
		}

		ambiguous, err := loadAuthorityAdjudicationThrough(
			ctx, tx, id, second.RecordedAt,
		)
		if err == nil {
			t.Fatalf("expected competing decisions to fail closed; got %+v", ambiguous)
		}
		if ambiguous != nil {
			t.Fatalf("ambiguous lookup returned a decision: %+v", ambiguous)
		}
		if !strings.Contains(err.Error(), "ambiguous authority adjudication") {
			t.Fatalf("expected ambiguity error, got: %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot transaction: %v", err)
	}
}

func TestAuthorityRepositoryLoadAuthorizationRecords(t *testing.T) {
	pool := openIsolatedAuthoritySnapshotPool(t)
	repository := NewAuthorityRepository(pool)
	ctx := context.Background()

	id := fmt.Sprintf("integration-record-loader-%d", time.Now().UnixNano())
	expected := authority.AuthorizationRecord{
		AuthorizationID:    id,
		IssuerID:           "integration-record-issuer",
		AuthoritySubjectID: "integration-record-subject",
		ClaimType:          "employment",
	}

	if err := repository.CreateAuthorization(ctx, expected); err != nil {
		t.Fatalf("create authorization: %v", err)
	}

	err := repository.withAuthoritySnapshotTx(ctx, func(tx pgx.Tx) error {
		records, err := loadAuthorityRecords(ctx, tx)
		if err != nil {
			return err
		}

		matches := 0
		for _, record := range records {
			if record.AuthorizationID != id {
				continue
			}
			matches++
			if record != expected {
				t.Errorf("loaded record = %+v, want %+v", record, expected)
			}
		}

		if matches != 1 {
			t.Errorf("matching authorization records = %d, want 1", matches)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("snapshot transaction: %v", err)
	}
}

func TestAuthorityRepositorySnapshotAssemblyFailsOnMissingAdjudication(t *testing.T) {
	pool := openIsolatedAuthoritySnapshotPool(t)
	repository := NewAuthorityRepository(pool)
	ctx := context.Background()

	id := fmt.Sprintf("integration-incomplete-snapshot-%d", time.Now().UnixNano())

	if err := repository.CreateAuthorization(ctx, authority.AuthorizationRecord{
		AuthorizationID:    id,
		IssuerID:           "integration-issuer",
		AuthoritySubjectID: "integration-subject",
		ClaimType:          "employment",
	}); err != nil {
		t.Fatalf("create authorization: %v", err)
	}

	cutoff := time.Now().UTC().Truncate(time.Microsecond)

	err := repository.withAuthoritySnapshotTx(ctx, func(tx pgx.Tx) error {
		entries, err := loadAuthoritySnapshotEntries(ctx, tx, cutoff)

		if err == nil {
			t.Fatalf("expected incomplete snapshot to fail; entries=%d", len(entries))
		}
		if entries != nil {
			t.Fatalf("failed snapshot assembly returned %d entries", len(entries))
		}
		if !strings.Contains(err.Error(), "missing authority adjudication") {
			t.Fatalf("expected missing adjudication error, got: %v", err)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("snapshot transaction: %v", err)
	}
}

// openIsolatedAuthoritySnapshotPool uses the disposable test database,
// but resolves unqualified authority tables only within the isolated schema.
func openIsolatedAuthoritySnapshotPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	basePool := openAuthorityTestPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	schema := fmt.Sprintf("authority_snapshot_%d", time.Now().UnixNano())

	if _, err := basePool.Exec(ctx, `CREATE SCHEMA "`+schema+`"`); err != nil {
		t.Fatalf("create isolated snapshot schema: %v", err)
	}

	// Clean up only the uniquely named schema created by this test.
	// Cleanup callbacks run in reverse registration order, so the
	// isolated connection pool closes before this callback executes.
	t.Cleanup(func() {
		if !strings.HasPrefix(schema, "authority_snapshot_") {
			t.Errorf("refusing to drop unexpected test schema %q", schema)
			return
		}

		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if _, err := basePool.Exec(cleanupCtx, `DROP SCHEMA "`+schema+`" CASCADE`); err != nil {
			t.Errorf("drop disposable snapshot schema %q: %v", schema, err)
		}
	})

	files, err := filepath.Glob("../../migrations/[0-9]*.sql")
	if err != nil {
		t.Fatalf("locate authority migrations: %v", err)
	}
	sort.Strings(files)
	if len(files) < 9 {
		t.Fatalf("expected at least nine authority migrations, found %d", len(files))
	}
	for i, file := range files {
		expectedPrefix := fmt.Sprintf("%06d_", i+1)
		if !strings.HasPrefix(filepath.Base(file), expectedPrefix) {
			t.Fatalf("unexpected authority migration sequence at %d: %s", i+1, file)
		}
	}

	tx, err := basePool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin isolated schema migration: %v", err)
	}
	defer tx.Rollback(context.Background())

	if _, err := tx.Exec(ctx, `SET LOCAL search_path TO "`+schema+`"`); err != nil {
		t.Fatalf("set migration schema: %v", err)
	}
	for _, file := range files {
		sql, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read migration %s: %v", file, err)
		}
		if _, err := tx.Exec(ctx, string(sql)); err != nil {
			t.Fatalf("apply migration %s: %v", file, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit isolated schema migrations: %v", err)
	}

	config, err := pgxpool.ParseConfig(os.Getenv("CVERA_AUTHORITY_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatalf("parse isolated pool configuration: %v", err)
	}
	if config.ConnConfig.RuntimeParams == nil {
		config.ConnConfig.RuntimeParams = make(map[string]string)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatalf("create isolated snapshot pool: %v", err)
	}
	t.Cleanup(pool.Close)

	var actualSchema string
	if err := pool.QueryRow(ctx, "SELECT current_schema()").Scan(&actualSchema); err != nil {
		t.Fatalf("verify isolated schema: %v", err)
	}
	if actualSchema != schema {
		t.Fatalf("expected schema %q, got %q", schema, actualSchema)
	}

	return pool
}

func TestAuthorityRepositoryAuthorizationRecordingBoundaryCommitAndRollback(t *testing.T) {
	pool := openIsolatedAuthoritySnapshotPool(t)
	repository := NewAuthorityRepository(pool)
	ctx := context.Background()

	readPosition := func() int64 {
		t.Helper()
		var position int64
		if err := pool.QueryRow(ctx, `
			SELECT committed_position
			FROM authority_recording_boundary
			WHERE singleton = TRUE
		`).Scan(&position); err != nil {
			t.Fatalf("read recording position: %v", err)
		}
		return position
	}

	initial := readPosition()
	if initial != 0 {
		t.Fatalf("initial recording position = %d, want 0", initial)
	}

	record := authority.AuthorizationRecord{
		AuthorizationID:    fmt.Sprintf("recording-boundary-%d", time.Now().UnixNano()),
		IssuerID:           "integration-issuer",
		AuthoritySubjectID: "integration-subject",
		ClaimType:          "employment",
	}

	if err := repository.CreateAuthorization(ctx, record); err != nil {
		t.Fatalf("create authorization: %v", err)
	}

	if got := readPosition(); got != initial+1 {
		t.Fatalf("position after committed insert = %d, want %d", got, initial+1)
	}

	var storedPosition int64
	if err := pool.QueryRow(ctx, `
                SELECT recording_position
                FROM authority_authorizations
                WHERE authorization_id = $1
        `, record.AuthorizationID).Scan(&storedPosition); err != nil {
		t.Fatalf("read persisted recording position: %v", err)
	}
	if storedPosition != initial+1 {
		t.Fatalf("persisted recording position = %d, want %d", storedPosition, initial+1)
	}

	var count int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM authority_authorizations
		WHERE authorization_id = $1
	`, record.AuthorizationID).Scan(&count); err != nil {
		t.Fatalf("query committed authorization: %v", err)
	}
	if count != 1 {
		t.Fatalf("committed authorization count = %d, want 1", count)
	}

	if err := repository.CreateAuthorization(ctx, record); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate authorization error = %v, want ErrConflict", err)
	}

	if got := readPosition(); got != initial+1 {
		t.Fatalf("position after duplicate rollback = %d, want %d", got, initial+1)
	}

	invalid := record
	invalid.AuthorizationID = ""
	if err := repository.CreateAuthorization(ctx, invalid); err == nil {
		t.Fatal("expected invalid authorization to be rejected")
	}

	if got := readPosition(); got != initial+1 {
		t.Fatalf("position after invalid authorization = %d, want %d", got, initial+1)
	}

	if err := repository.withAuthorityRecordingTx(ctx, func(tx pgx.Tx, position int64) error {
		if position != initial+2 {
			t.Fatalf("rollback transaction position = %d, want %d", position, initial+2)
		}
		return errors.New("intentional rollback")
	}); err == nil || err.Error() != "intentional rollback" {
		t.Fatalf("intentional rollback error = %v", err)
	}

	if got := readPosition(); got != initial+1 {
		t.Fatalf("position after explicit rollback = %d, want %d", got, initial+1)
	}
}

func TestAuthorityRepositoryAdjudicationRecordingBoundaryCommitAndRollback(t *testing.T) {
	pool := openIsolatedAuthoritySnapshotPool(t)
	repository := NewAuthorityRepository(pool)
	ctx := context.Background()

	readPosition := func() int64 {
		t.Helper()
		var position int64
		if err := pool.QueryRow(ctx, `
			SELECT committed_position
			FROM authority_recording_boundary
			WHERE singleton = TRUE
		`).Scan(&position); err != nil {
			t.Fatalf("read recording position: %v", err)
		}
		return position
	}

	id := fmt.Sprintf("adjudication-boundary-%d", time.Now().UnixNano())
	record := authority.AuthorizationRecord{
		AuthorizationID:    id,
		IssuerID:           "integration-issuer",
		AuthoritySubjectID: "integration-subject",
		ClaimType:          "employment",
	}
	if err := repository.CreateAuthorization(ctx, record); err != nil {
		t.Fatalf("create authorization: %v", err)
	}

	before := readPosition()
	if before != 1 {
		t.Fatalf("position after authorization = %d, want 1", before)
	}

	recordedAt := time.Now().UTC().Truncate(time.Microsecond)
	decision := authority.AuthorizationAdjudication{
		DecisionID:         id + "-decision",
		AuthorizationID:    id,
		IssuerID:           record.IssuerID,
		AuthoritySubjectID: record.AuthoritySubjectID,
		ClaimType:          record.ClaimType,
		Outcome:            authority.AdjudicationApproved,
		DecisionMakerID:    "integration-adjudicator",
		AuthorityBasis:     "verified organizational delegation",
		EvidenceRef:        "integration-evidence",
		DecidedAt:          recordedAt,
		RecordedAt:         recordedAt,
	}

	if err := repository.CreateAdjudication(ctx, decision); err != nil {
		t.Fatalf("create adjudication: %v", err)
	}

	if got := readPosition(); got != before+1 {
		t.Fatalf("position after adjudication = %d, want %d", got, before+1)
	}

	var storedPosition int64
	if err := pool.QueryRow(ctx, `
                SELECT recording_position
                FROM authority_adjudications
                WHERE decision_id = $1
        `, decision.DecisionID).Scan(&storedPosition); err != nil {
		t.Fatalf("read persisted recording position: %v", err)
	}
	if storedPosition != before+1 {
		t.Fatalf("persisted recording position = %d, want %d", storedPosition, before+1)
	}

	var count int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM authority_adjudications
		WHERE decision_id = $1
	`, decision.DecisionID).Scan(&count); err != nil {
		t.Fatalf("query adjudication: %v", err)
	}
	if count != 1 {
		t.Fatalf("adjudication count = %d, want 1", count)
	}

	if err := repository.CreateAdjudication(ctx, decision); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate adjudication error = %v, want ErrConflict", err)
	}

	if got := readPosition(); got != before+1 {
		t.Fatalf("position after duplicate rollback = %d, want %d", got, before+1)
	}
}

func TestAuthorityRepositoryJournalRecordingBoundaryCommitAndRollback(t *testing.T) {
	pool := openIsolatedAuthoritySnapshotPool(t)
	repository := NewAuthorityRepository(pool)
	ctx := context.Background()

	readPosition := func() int64 {
		t.Helper()
		var position int64
		if err := pool.QueryRow(ctx, `
			SELECT committed_position
			FROM authority_recording_boundary
			WHERE singleton = TRUE
		`).Scan(&position); err != nil {
			t.Fatalf("read recording position: %v", err)
		}
		return position
	}

	id := fmt.Sprintf("journal-boundary-%d", time.Now().UnixNano())
	if err := repository.CreateAuthorization(ctx, authority.AuthorizationRecord{
		AuthorizationID:    id,
		IssuerID:           "integration-issuer",
		AuthoritySubjectID: "integration-subject",
		ClaimType:          "employment",
	}); err != nil {
		t.Fatalf("create authorization: %v", err)
	}

	before := readPosition()
	if before != 1 {
		t.Fatalf("position after authorization = %d, want 1", before)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	event := authority.AuthorizationJournalEvent{
		EventID:             id + "-submit",
		AuthorizationID:     id,
		EventType:           authority.EventSubmit,
		EffectiveAt:         now,
		RecordedAt:          now,
		ActorID:             "integration-actor",
		EvidenceRef:         "integration-evidence",
		ConditionsSatisfied: true,
	}

	if err := repository.AppendJournalEvent(ctx, event); err != nil {
		t.Fatalf("append journal event: %v", err)
	}
	if got := readPosition(); got != before+1 {
		t.Fatalf("position after journal commit = %d, want %d", got, before+1)
	}

	var storedPosition int64
	if err := pool.QueryRow(ctx, `
                SELECT recording_position
                FROM authority_journal_events
                WHERE event_id = $1
        `, event.EventID).Scan(&storedPosition); err != nil {
		t.Fatalf("read persisted recording position: %v", err)
	}
	if storedPosition != before+1 {
		t.Fatalf("persisted recording position = %d, want %d", storedPosition, before+1)
	}

	var count int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM authority_journal_events WHERE event_id = $1
	`, event.EventID).Scan(&count); err != nil {
		t.Fatalf("query journal event: %v", err)
	}
	if count != 1 {
		t.Fatalf("journal event count = %d, want 1", count)
	}

	if err := repository.AppendJournalEvent(ctx, event); err == nil {
		t.Fatal("expected duplicate journal event to fail")
	}
	if got := readPosition(); got != before+1 {
		t.Fatalf("position after duplicate event = %d, want %d", got, before+1)
	}

	invalid := event
	invalid.EventID = id + "-invalid"
	invalid.EventType = authority.EventActivate
	invalid.RecordedAt = now.Add(time.Second)
	invalid.EffectiveAt = invalid.RecordedAt

	if err := repository.AppendJournalEvent(ctx, invalid); err == nil {
		t.Fatal("expected invalid journal transition to fail")
	}
	if got := readPosition(); got != before+1 {
		t.Fatalf("position after invalid transition = %d, want %d", got, before+1)
	}
}

func TestAuthorityRepositoryConcurrentJournalAndAdjudicationRecording(t *testing.T) {
	pool := openIsolatedAuthoritySnapshotPool(t)
	repository := NewAuthorityRepository(pool)
	ctx := context.Background()

	id := fmt.Sprintf("concurrent-recording-%d", time.Now().UnixNano())
	record := authority.AuthorizationRecord{
		AuthorizationID:    id,
		IssuerID:           "integration-issuer",
		AuthoritySubjectID: "integration-subject",
		ClaimType:          "employment",
	}
	if err := repository.CreateAuthorization(ctx, record); err != nil {
		t.Fatalf("create authorization: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	event := authority.AuthorizationJournalEvent{
		EventID:             id + "-submit",
		AuthorizationID:     id,
		EventType:           authority.EventSubmit,
		EffectiveAt:         now,
		RecordedAt:          now,
		ActorID:             "integration-actor",
		EvidenceRef:         "submission-evidence",
		ConditionsSatisfied: true,
	}
	decision := authority.AuthorizationAdjudication{
		DecisionID:         id + "-decision",
		AuthorizationID:    id,
		IssuerID:           record.IssuerID,
		AuthoritySubjectID: record.AuthoritySubjectID,
		ClaimType:          record.ClaimType,
		Outcome:            authority.AdjudicationApproved,
		DecisionMakerID:    "integration-adjudicator",
		AuthorityBasis:     "verified organizational delegation",
		EvidenceRef:        "decision-evidence",
		DecidedAt:          now,
		RecordedAt:         now,
	}

	runCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	start := make(chan struct{})
	results := make(chan error, 2)

	go func() {
		<-start
		results <- repository.AppendJournalEvent(runCtx, event)
	}()
	go func() {
		<-start
		results <- repository.CreateAdjudication(runCtx, decision)
	}()
	close(start)

	for i := 0; i < 2; i++ {
		select {
		case err := <-results:
			if err != nil {
				t.Fatalf("concurrent authority write failed: %v", err)
			}
		case <-runCtx.Done():
			t.Fatalf("concurrent authority writes exceeded deadline: %v", runCtx.Err())
		}
	}

	var position int64
	if err := pool.QueryRow(ctx, `
		SELECT committed_position
		FROM authority_recording_boundary
		WHERE singleton = TRUE
	`).Scan(&position); err != nil {
		t.Fatalf("read recording position: %v", err)
	}
	if position != 3 {
		t.Fatalf("recording position = %d, want 3", position)
	}

	var eventPosition, decisionPosition int64
	if err := pool.QueryRow(ctx, `
		SELECT recording_position
		FROM authority_journal_events
		WHERE event_id = $1
	`, event.EventID).Scan(&eventPosition); err != nil {
		t.Fatalf("read journal recording position: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT recording_position
		FROM authority_adjudications
		WHERE decision_id = $1
	`, decision.DecisionID).Scan(&decisionPosition); err != nil {
		t.Fatalf("read adjudication recording position: %v", err)
	}
	if !((eventPosition == 2 && decisionPosition == 3) ||
		(eventPosition == 3 && decisionPosition == 2)) {
		t.Fatalf("concurrent evidence positions: journal=%d adjudication=%d, want distinct positions 2 and 3",
			eventPosition, decisionPosition)
	}

	var eventCount, decisionCount int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM authority_journal_events WHERE event_id = $1
	`, event.EventID).Scan(&eventCount); err != nil {
		t.Fatalf("count journal events: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM authority_adjudications WHERE decision_id = $1
	`, decision.DecisionID).Scan(&decisionCount); err != nil {
		t.Fatalf("count adjudications: %v", err)
	}
	if eventCount != 1 || decisionCount != 1 {
		t.Fatalf("persisted evidence: events=%d decisions=%d, want 1 each",
			eventCount, decisionCount)
	}
}

func TestAuthorityRepositoryAcceptsBackdatedAdjudicationRecordedAt(t *testing.T) {
	pool := openIsolatedAuthoritySnapshotPool(t)
	repository := NewAuthorityRepository(pool)
	ctx := context.Background()

	id := fmt.Sprintf("backdated-decision-%d", time.Now().UnixNano())
	record := authority.AuthorizationRecord{
		AuthorizationID:    id,
		IssuerID:           "integration-issuer",
		AuthoritySubjectID: "integration-subject",
		ClaimType:          "employment",
	}
	if err := repository.CreateAuthorization(ctx, record); err != nil {
		t.Fatalf("create authorization: %v", err)
	}

	recordedAt := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	decision := authority.AuthorizationAdjudication{
		DecisionID:         id + "-decision",
		AuthorizationID:    id,
		IssuerID:           record.IssuerID,
		AuthoritySubjectID: record.AuthoritySubjectID,
		ClaimType:          record.ClaimType,
		Outcome:            authority.AdjudicationApproved,
		DecisionMakerID:    "integration-adjudicator",
		AuthorityBasis:     "verified organizational delegation",
		EvidenceRef:        "integration-evidence",
		DecidedAt:          recordedAt,
		RecordedAt:         recordedAt,
	}
	if err := repository.CreateAdjudication(ctx, decision); err != nil {
		t.Fatalf("create backdated adjudication: %v", err)
	}

	err := repository.withAuthoritySnapshotTx(ctx, func(tx pgx.Tx) error {
		loaded, err := loadAuthorityAdjudicationThrough(ctx, tx, id, recordedAt)
		if err != nil {
			return err
		}
		if loaded.DecisionID != decision.DecisionID {
			t.Fatalf("unexpected adjudication: %s", loaded.DecisionID)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("load backdated adjudication: %v", err)
	}
}

func TestAuthorityRepositoryIsolatedSnapshotSchema(t *testing.T) {
	pool := openIsolatedAuthoritySnapshotPool(t)
	ctx := context.Background()

	var schema string
	err := pool.QueryRow(ctx, "SELECT current_schema()").Scan(&schema)
	if err != nil {
		t.Fatalf("identify isolated schema: %v", err)
	}
	if !strings.HasPrefix(schema, "authority_snapshot_") {
		t.Fatalf("unexpected isolated schema: %q", schema)
	}

	var tableSchema string
	err = pool.QueryRow(ctx, `
		SELECT table_schema
		FROM information_schema.tables
		WHERE table_schema = current_schema()
		  AND table_name = 'authority_authorizations'
	`).Scan(&tableSchema)
	if err != nil {
		t.Fatalf("locate isolated authority table: %v", err)
	}
	if tableSchema != schema {
		t.Fatalf("authority table belongs to %q, expected %q", tableSchema, schema)
	}

	var resolvedTable string
	err = pool.QueryRow(ctx, `
		SELECT 'authority_authorizations'::regclass::text
	`).Scan(&resolvedTable)
	if err != nil {
		t.Fatalf("resolve authority table through search_path: %v", err)
	}
	if resolvedTable != "authority_authorizations" {
		t.Fatalf("unexpected resolved table: %q", resolvedTable)
	}

	var count int
	err = pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM authority_authorizations
	`).Scan(&count)
	if err != nil {
		t.Fatalf("query isolated authorization table: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected empty isolated authorization table, found %d records", count)
	}
}

func TestAuthorityRepositoryAssemblesCompleteSnapshotEntries(t *testing.T) {
	pool := openIsolatedAuthoritySnapshotPool(t)
	repository := NewAuthorityRepository(pool)
	ctx := context.Background()

	id := fmt.Sprintf("integration-complete-snapshot-%d", time.Now().UnixNano())
	record := authority.AuthorizationRecord{
		AuthorizationID:    id,
		IssuerID:           "integration-issuer",
		AuthoritySubjectID: "integration-subject",
		ClaimType:          "employment",
	}
	if err := repository.CreateAuthorization(ctx, record); err != nil {
		t.Fatalf("create authorization: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	decision := authority.AuthorizationAdjudication{
		DecisionID:         id + "-decision",
		AuthorizationID:    id,
		IssuerID:           record.IssuerID,
		AuthoritySubjectID: record.AuthoritySubjectID,
		ClaimType:          record.ClaimType,
		Outcome:            authority.AdjudicationApproved,
		DecisionMakerID:    "integration-adjudicator",
		AuthorityBasis:     "verified organizational delegation",
		EvidenceRef:        id + "-approval-evidence",
		DecidedAt:          now,
		RecordedAt:         now,
	}
	if err := repository.CreateAdjudication(ctx, decision); err != nil {
		t.Fatalf("create adjudication: %v", err)
	}

	eventTypes := []authority.AuthorizationEvent{
		authority.EventSubmit,
		authority.EventApprove,
		authority.EventActivate,
	}
	for i, eventType := range eventTypes {
		event := authority.AuthorizationJournalEvent{
			EventID:             fmt.Sprintf("%s-event-%d", id, i),
			AuthorizationID:     id,
			EventType:           eventType,
			EffectiveAt:         now.Add(time.Duration(i) * time.Minute),
			RecordedAt:          now.Add(time.Duration(i) * time.Minute),
			ActorID:             "integration-adjudicator",
			EvidenceRef:         fmt.Sprintf("%s-evidence-%d", id, i),
			ConditionsSatisfied: true,
		}
		if eventType == authority.EventApprove {
			event.EvidenceRef = decision.EvidenceRef
		}
		if err := repository.AppendJournalEvent(ctx, event); err != nil {
			t.Fatalf("append %s event: %v", eventType, err)
		}
	}

	cutoff := now.Add(3 * time.Minute)
	err := repository.withAuthoritySnapshotTx(ctx, func(tx pgx.Tx) error {
		entries, err := loadAuthoritySnapshotEntries(ctx, tx, cutoff)
		if err != nil {
			return err
		}
		if len(entries) != 1 {
			t.Fatalf("snapshot entries = %d, want 1", len(entries))
		}

		entry := entries[0]
		if entry.Record.AuthorizationID != id {
			t.Errorf("authorization ID = %q, want %q", entry.Record.AuthorizationID, id)
		}
		if entry.Record.Adjudication == nil ||
			*entry.Record.Adjudication != decision {
			t.Errorf("loaded adjudication = %+v, want %+v",
				entry.Record.Adjudication, decision)
		}

		events := entry.Journal.Events()
		if len(events) != 3 {
			t.Fatalf("journal events = %d, want 3", len(events))
		}
		for i, event := range events {
			if event.EventType != eventTypes[i] {
				t.Errorf("event %d = %s, want %s",
					i, event.EventType, eventTypes[i])
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot transaction: %v", err)
	}
}

func TestAuthorityRecordingEnforcementRejectsDirectSQLBypass(t *testing.T) {
	pool := openIsolatedAuthoritySnapshotPool(t)
	ctx := context.Background()

	readBoundary := func() int64 {
		t.Helper()
		var position int64
		if err := pool.QueryRow(ctx, `
			SELECT committed_position
			FROM authority_recording_boundary
			WHERE singleton = TRUE
		`).Scan(&position); err != nil {
			t.Fatalf("read recording boundary: %v", err)
		}
		return position
	}

	// A new evidence row cannot bypass allocation by omitting its position.
	_, err := pool.Exec(ctx, `
		INSERT INTO authority_authorizations (
			authorization_id, issuer_id, authority_subject_id, claim_type
		) VALUES ('bypass-null', 'issuer', 'subject', 'employment')
	`)
	if err == nil {
		t.Fatal("expected insert without recording position to fail")
	}
	if got := readBoundary(); got != 0 {
		t.Fatalf("boundary after rejected insert = %d, want 0", got)
	}

	// Allocating a position without inserting evidence cannot commit.
	_, err = pool.Exec(ctx, `SELECT next_authority_recording_position()`)
	if err == nil {
		t.Fatal("expected counter-only commit to fail")
	}
	if got := readBoundary(); got != 0 {
		t.Fatalf("boundary after rejected allocation = %d, want 0", got)
	}

	repository := NewAuthorityRepository(pool)
	record := authority.AuthorizationRecord{
		AuthorizationID:    "recorded-original",
		IssuerID:           "issuer",
		AuthoritySubjectID: "subject",
		ClaimType:          "employment",
	}
	if err := repository.CreateAuthorization(ctx, record); err != nil {
		t.Fatalf("create legitimate authorization: %v", err)
	}
	if got := readBoundary(); got != 1 {
		t.Fatalf("boundary after legitimate insert = %d, want 1", got)
	}

	// An independent transaction cannot reuse the committed position.
	_, err = pool.Exec(ctx, `
		INSERT INTO authority_authorizations (
			authorization_id, issuer_id, authority_subject_id,
			claim_type, recording_position
		) VALUES ('bypass-reuse', 'issuer', 'subject', 'employment', 1)
	`)
	if err == nil {
		t.Fatal("expected reused recording position to fail")
	}
	if got := readBoundary(); got != 1 {
		t.Fatalf("boundary after rejected reuse = %d, want 1", got)
	}
}

func TestAuthorityRecordingEnforcementRejectsCrossTableDuplication(t *testing.T) {
	pool := openIsolatedAuthoritySnapshotPool(t)
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}
	defer tx.Rollback(context.Background())

	var position int64
	if err := tx.QueryRow(ctx,
		`SELECT next_authority_recording_position()`,
	).Scan(&position); err != nil {
		t.Fatalf("allocate recording position: %v", err)
	}
	if position != 1 {
		t.Fatalf("allocated position = %d, want 1", position)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO authority_authorizations (
			authorization_id, issuer_id, authority_subject_id,
			claim_type, recording_position
		) VALUES ('cross-table-auth', 'issuer', 'subject', 'employment', $1)
	`, position)
	if err != nil {
		t.Fatalf("insert authorization: %v", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO authority_adjudications (
			decision_id, authorization_id, issuer_id,
			authority_subject_id, claim_type, outcome,
			decision_maker_id, authority_basis, evidence_ref,
			decided_at, recorded_at, recording_position
		) VALUES (
			'cross-table-decision', 'cross-table-auth', 'issuer',
			'subject', 'employment', 'APPROVED',
			'adjudicator', 'verified delegation', 'evidence',
			CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, $1
		)
	`, position)
	if err != nil {
		t.Fatalf("insert adjudication: %v", err)
	}

	if err := tx.Commit(ctx); err == nil {
		t.Fatal("expected cross-table recording-position duplication to fail at commit")
	}

	var boundary, authorizationCount, adjudicationCount int64
	if err := pool.QueryRow(ctx, `
		SELECT committed_position FROM authority_recording_boundary
		WHERE singleton = TRUE
	`).Scan(&boundary); err != nil {
		t.Fatalf("read recording boundary: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM authority_authorizations
		WHERE authorization_id = 'cross-table-auth'
	`).Scan(&authorizationCount); err != nil {
		t.Fatalf("count rolled-back authorization: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM authority_adjudications
		WHERE decision_id = 'cross-table-decision'
	`).Scan(&adjudicationCount); err != nil {
		t.Fatalf("count rolled-back adjudication: %v", err)
	}

	if boundary != 0 || authorizationCount != 0 || adjudicationCount != 0 {
		t.Fatalf(
			"cross-table rollback incomplete: boundary=%d authorizations=%d adjudications=%d",
			boundary, authorizationCount, adjudicationCount,
		)
	}
}

func TestAuthorityRecordingEnforcementRejectsForeignTransactionPosition(t *testing.T) {
	pool := openIsolatedAuthoritySnapshotPool(t)
	ctx := context.Background()

	repository := NewAuthorityRepository(pool)
	if err := repository.CreateAuthorization(ctx, authority.AuthorizationRecord{
		AuthorizationID:    "ownership-original",
		IssuerID:           "issuer",
		AuthoritySubjectID: "subject",
		ClaimType:          "employment",
	}); err != nil {
		t.Fatalf("create original authorization: %v", err)
	}

	// Position 1 was allocated and committed by a different transaction.
	// A new transaction must not claim that position for another evidence row.
	_, err := pool.Exec(ctx, `
		INSERT INTO authority_authorizations (
			authorization_id, issuer_id, authority_subject_id,
			claim_type, recording_position
		) VALUES ('ownership-forged', 'issuer', 'subject', 'employment', 1)
	`)
	if err == nil {
		t.Fatal("expected foreign transaction recording position to be rejected")
	}

	var count int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM authority_authorizations
		WHERE authorization_id = 'ownership-forged'
	`).Scan(&count); err != nil {
		t.Fatalf("count forged authorization: %v", err)
	}
	if count != 0 {
		t.Fatalf("forged authorization count = %d, want 0", count)
	}
}

func TestAuthorityRecordingEnforcementRejectsUnallocatedPosition(t *testing.T) {
	pool := openIsolatedAuthoritySnapshotPool(t)
	ctx := context.Background()

	// Position 1 is unused. The insert must fail because this
	// transaction never allocated it through the recording counter.
	_, err := pool.Exec(ctx, `
		INSERT INTO authority_authorizations (
			authorization_id, issuer_id, authority_subject_id,
			claim_type, recording_position
		) VALUES (
			'unallocated-position', 'issuer', 'subject',
			'employment', 1
		)
	`)
	if err == nil {
		t.Fatal("expected unallocated recording position to be rejected")
	}

	var boundary int64
	if err := pool.QueryRow(ctx, `
		SELECT committed_position
		FROM authority_recording_boundary
		WHERE singleton = TRUE
	`).Scan(&boundary); err != nil {
		t.Fatalf("read recording boundary: %v", err)
	}

	var count int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM authority_authorizations
		WHERE authorization_id = 'unallocated-position'
	`).Scan(&count); err != nil {
		t.Fatalf("count rejected authorization: %v", err)
	}

	if boundary != 0 || count != 0 {
		t.Fatalf(
			"rejected insert changed database: boundary=%d rows=%d",
			boundary, count,
		)
	}
}

func TestAuthorityRepositorySnapshotRecordingBoundaryVisibility(t *testing.T) {
	pool := openIsolatedAuthoritySnapshotPool(t)
	repository := NewAuthorityRepository(pool)
	ctx := context.Background()

	readBoundary := func() int64 {
		t.Helper()
		var position int64
		err := repository.withAuthoritySnapshotTx(ctx, func(tx pgx.Tx) error {
			var err error
			position, err = loadAuthorityRecordingBoundary(ctx, tx)
			return err
		})
		if err != nil {
			t.Fatalf("read snapshot recording boundary: %v", err)
		}
		return position
	}

	if got := readBoundary(); got != 0 {
		t.Fatalf("initial recording boundary = %d, want 0", got)
	}

	if err := repository.CreateAuthorization(ctx, authority.AuthorizationRecord{
		AuthorizationID:    "boundary-committed",
		IssuerID:           "issuer",
		AuthoritySubjectID: "subject",
		ClaimType:          "employment",
	}); err != nil {
		t.Fatalf("create committed authorization: %v", err)
	}

	if got := readBoundary(); got != 1 {
		t.Fatalf("committed recording boundary = %d, want 1", got)
	}

	writer, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin uncommitted writer: %v", err)
	}
	defer writer.Rollback(context.Background())

	var allocated int64
	if err := writer.QueryRow(ctx,
		`SELECT next_authority_recording_position()`,
	).Scan(&allocated); err != nil {
		t.Fatalf("allocate uncommitted position: %v", err)
	}
	if allocated != 2 {
		t.Fatalf("allocated position = %d, want 2", allocated)
	}

	if got := readBoundary(); got != 1 {
		t.Fatalf("snapshot exposed uncommitted position %d, want 1", got)
	}

	if err := writer.Rollback(ctx); err != nil {
		t.Fatalf("rollback uncommitted writer: %v", err)
	}

	if got := readBoundary(); got != 1 {
		t.Fatalf("boundary after rollback = %d, want 1", got)
	}
}

func TestAuthorityRepositorySnapshotRecordingBoundaryRemainsStable(t *testing.T) {
	pool := openIsolatedAuthoritySnapshotPool(t)
	repository := NewAuthorityRepository(pool)
	ctx := context.Background()

	err := repository.withAuthoritySnapshotTx(ctx, func(tx pgx.Tx) error {
		before, err := loadAuthorityRecordingBoundary(ctx, tx)
		if err != nil {
			return err
		}
		if before != 0 {
			t.Fatalf("initial snapshot boundary = %d, want 0", before)
		}

		if err := repository.CreateAuthorization(ctx, authority.AuthorizationRecord{
			AuthorizationID:    "boundary-concurrent-commit",
			IssuerID:           "issuer",
			AuthoritySubjectID: "subject",
			ClaimType:          "employment",
		}); err != nil {
			return fmt.Errorf("commit concurrent authorization: %w", err)
		}

		after, err := loadAuthorityRecordingBoundary(ctx, tx)
		if err != nil {
			return err
		}
		if after != before {
			t.Fatalf("snapshot boundary changed from %d to %d", before, after)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot transaction: %v", err)
	}

	err = repository.withAuthoritySnapshotTx(ctx, func(tx pgx.Tx) error {
		position, err := loadAuthorityRecordingBoundary(ctx, tx)
		if err != nil {
			return err
		}
		if position != 1 {
			t.Fatalf("fresh snapshot boundary = %d, want 1", position)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("fresh snapshot transaction: %v", err)
	}
}

func TestAuthorityRepositoryAuthorizationRecordingPositionFiltering(t *testing.T) {
	pool := openIsolatedAuthoritySnapshotPool(t)
	repository := NewAuthorityRepository(pool)
	ctx := context.Background()

	create := func(id string) {
		t.Helper()
		if err := repository.CreateAuthorization(ctx, authority.AuthorizationRecord{
			AuthorizationID:    id,
			IssuerID:           "issuer",
			AuthoritySubjectID: "subject",
			ClaimType:          "employment",
		}); err != nil {
			t.Fatalf("create authorization %s: %v", id, err)
		}
	}

	load := func(position int64) ([]authority.AuthorizationRecord, error) {
		t.Helper()
		var records []authority.AuthorizationRecord
		err := repository.withAuthoritySnapshotTx(ctx, func(tx pgx.Tx) error {
			var err error
			records, err = loadAuthorityRecordsAtPosition(ctx, tx, position)
			return err
		})
		return records, err
	}

	create("position-first")
	create("position-second")

	records, err := load(1)
	if err != nil {
		t.Fatalf("load position 1: %v", err)
	}
	if len(records) != 1 || records[0].AuthorizationID != "position-first" {
		t.Fatalf("position 1 records = %+v, want first only", records)
	}

	records, err = load(2)
	if err != nil {
		t.Fatalf("load position 2: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("position 2 records = %+v, want both", records)
	}

	records, err = load(0)
	if err != nil {
		t.Fatalf("load position 0: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("position 0 records = %+v, want none", records)
	}

	_, err = load(-1)
	if err == nil {
		t.Fatal("expected negative recording boundary to fail")
	}

	// The immutable legacy record has no trustworthy recording position.
	// It must not be silently omitted from an authoritative reconstruction.
	if _, err := pool.Exec(ctx, `
		ALTER TABLE authority_authorizations
		DISABLE TRIGGER authority_authorizations_recording_guard
	`); err != nil {
		t.Fatalf("disable guard for legacy fixture: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO authority_authorizations (
			authorization_id, issuer_id, authority_subject_id, claim_type
		) VALUES ('legacy-unpositioned', 'issuer', 'subject', 'employment')
	`); err != nil {
		t.Fatalf("insert legacy fixture: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		ALTER TABLE authority_authorizations
		ENABLE TRIGGER authority_authorizations_recording_guard
	`); err != nil {
		t.Fatalf("restore recording guard: %v", err)
	}

	records, err = load(2)
	if err == nil || !strings.Contains(err.Error(), "no trustworthy recording position") {
		t.Fatalf("expected legacy evidence failure, got records=%+v, err=%v", records, err)
	}
}

func TestAuthorityRepositoryAdjudicationRecordingPositionFiltering(t *testing.T) {
	pool := openIsolatedAuthoritySnapshotPool(t)
	repository := NewAuthorityRepository(pool)
	ctx := context.Background()

	record := authority.AuthorizationRecord{
		AuthorizationID:    "position-adjudication",
		IssuerID:           "issuer",
		AuthoritySubjectID: "subject",
		ClaimType:          "employment",
	}
	if err := repository.CreateAuthorization(ctx, record); err != nil {
		t.Fatalf("create authorization: %v", err)
	}

	// Capture the boundary before adjudication exists.
	var earlierBoundary int64
	if err := repository.withAuthoritySnapshotTx(ctx, func(tx pgx.Tx) error {
		var err error
		earlierBoundary, err = loadAuthorityRecordingBoundary(ctx, tx)
		return err
	}); err != nil {
		t.Fatalf("capture earlier boundary: %v", err)
	}
	if earlierBoundary != 1 {
		t.Fatalf("earlier boundary = %d, want 1", earlierBoundary)
	}

	// The decision is deliberately backdated relative to its commit.
	recordedAt := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	decision := authority.AuthorizationAdjudication{
		DecisionID:         "position-backdated-decision",
		AuthorizationID:    record.AuthorizationID,
		IssuerID:           record.IssuerID,
		AuthoritySubjectID: record.AuthoritySubjectID,
		ClaimType:          record.ClaimType,
		Outcome:            authority.AdjudicationApproved,
		DecisionMakerID:    "adjudicator",
		AuthorityBasis:     "verified organizational delegation",
		EvidenceRef:        "backdated-evidence",
		DecidedAt:          recordedAt,
		RecordedAt:         recordedAt,
	}
	if err := repository.CreateAdjudication(ctx, decision); err != nil {
		t.Fatalf("create backdated adjudication: %v", err)
	}

	load := func(position int64) (*authority.AuthorizationAdjudication, error) {
		t.Helper()
		var result *authority.AuthorizationAdjudication
		err := repository.withAuthoritySnapshotTx(ctx, func(tx pgx.Tx) error {
			var err error
			result, err = loadAuthorityAdjudicationAtPosition(
				ctx, tx, record.AuthorizationID, position,
			)
			return err
		})
		return result, err
	}

	earlier, err := load(earlierBoundary)
	if err == nil || earlier != nil ||
		!strings.Contains(err.Error(), "missing authority adjudication") {
		t.Fatalf("earlier boundary exposed decision: result=%+v, err=%v", earlier, err)
	}

	current, err := load(2)
	if err != nil {
		t.Fatalf("load committed decision: %v", err)
	}
	if current == nil || *current != decision {
		t.Fatalf("committed decision = %+v, want %+v", current, decision)
	}

	if _, err := load(-1); err == nil {
		t.Fatal("expected negative recording boundary to fail")
	}
}

func TestAuthorityRepositoryJournalRecordingPositionFiltering(t *testing.T) {
	pool := openIsolatedAuthoritySnapshotPool(t)
	repository := NewAuthorityRepository(pool)
	ctx := context.Background()

	record := authority.AuthorizationRecord{
		AuthorizationID:    "position-journal",
		IssuerID:           "issuer",
		AuthoritySubjectID: "subject",
		ClaimType:          "employment",
	}
	if err := repository.CreateAuthorization(ctx, record); err != nil {
		t.Fatalf("create authorization: %v", err)
	}

	// Authorization creation occupies position 1.
	var earlierBoundary int64
	if err := repository.withAuthoritySnapshotTx(ctx, func(tx pgx.Tx) error {
		var err error
		earlierBoundary, err = loadAuthorityRecordingBoundary(ctx, tx)
		return err
	}); err != nil {
		t.Fatalf("capture earlier boundary: %v", err)
	}
	if earlierBoundary != 1 {
		t.Fatalf("earlier boundary = %d, want 1", earlierBoundary)
	}

	// This event is recorded with a timestamp earlier than its commit.
	backdated := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	event := authority.AuthorizationJournalEvent{
		EventID:             "position-backdated-submit",
		AuthorizationID:     record.AuthorizationID,
		EventType:           authority.EventSubmit,
		EffectiveAt:         backdated,
		RecordedAt:          backdated,
		ActorID:             "integration-actor",
		EvidenceRef:         "backdated-submit-evidence",
		ConditionsSatisfied: true,
	}
	if err := repository.AppendJournalEvent(ctx, event); err != nil {
		t.Fatalf("append backdated event: %v", err)
	}

	load := func(position int64) (authority.AuthorizationJournal, error) {
		t.Helper()
		var journal authority.AuthorizationJournal
		err := repository.withAuthoritySnapshotTx(ctx, func(tx pgx.Tx) error {
			var err error
			journal, err = loadAuthorizationJournalAtPosition(
				ctx, tx, record.AuthorizationID, position,
			)
			return err
		})
		return journal, err
	}

	// The event must not appear in a snapshot captured before its commit.
	earlier, err := load(earlierBoundary)
	if err != nil {
		t.Fatalf("load earlier journal: %v", err)
	}
	if len(earlier.Events()) != 0 {
		t.Fatalf("earlier journal contains %d events, want 0", len(earlier.Events()))
	}

	// The event becomes visible at its committed recording position.
	current, err := load(2)
	if err != nil {
		t.Fatalf("load committed journal: %v", err)
	}
	if len(current.Events()) != 1 || current.Events()[0] != event {
		t.Fatalf("committed journal = %+v, want one matching event", current.Events())
	}

	if _, err := load(-1); err == nil {
		t.Fatal("expected negative recording boundary to fail")
	}
}

func TestAuthorityRepositoryAssemblesSnapshotAtRecordingPosition(t *testing.T) {
	pool := openIsolatedAuthoritySnapshotPool(t)
	repository := NewAuthorityRepository(pool)
	ctx := context.Background()

	record := authority.AuthorizationRecord{
		AuthorizationID:    "position-assembled-snapshot",
		IssuerID:           "integration-issuer",
		AuthoritySubjectID: "integration-subject",
		ClaimType:          "employment",
	}
	if err := repository.CreateAuthorization(ctx, record); err != nil {
		t.Fatalf("create authorization: %v", err)
	}

	now := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	decision := authority.AuthorizationAdjudication{
		DecisionID:         record.AuthorizationID + "-decision",
		AuthorizationID:    record.AuthorizationID,
		IssuerID:           record.IssuerID,
		AuthoritySubjectID: record.AuthoritySubjectID,
		ClaimType:          record.ClaimType,
		Outcome:            authority.AdjudicationApproved,
		DecisionMakerID:    "integration-adjudicator",
		AuthorityBasis:     "verified organizational delegation",
		EvidenceRef:        "position-approval-evidence",
		DecidedAt:          now,
		RecordedAt:         now,
	}
	if err := repository.CreateAdjudication(ctx, decision); err != nil {
		t.Fatalf("create adjudication: %v", err)
	}

	eventTypes := []authority.AuthorizationEvent{
		authority.EventSubmit,
		authority.EventApprove,
		authority.EventActivate,
	}
	for i, eventType := range eventTypes {
		event := authority.AuthorizationJournalEvent{
			EventID:             fmt.Sprintf("position-event-%d", i),
			AuthorizationID:     record.AuthorizationID,
			EventType:           eventType,
			EffectiveAt:         now.Add(time.Duration(i) * time.Minute),
			RecordedAt:          now.Add(time.Duration(i) * time.Minute),
			ActorID:             "integration-adjudicator",
			EvidenceRef:         fmt.Sprintf("position-evidence-%d", i),
			ConditionsSatisfied: true,
		}
		if eventType == authority.EventApprove {
			event.EvidenceRef = decision.EvidenceRef
		}
		if err := repository.AppendJournalEvent(ctx, event); err != nil {
			t.Fatalf("append %s: %v", eventType, err)
		}
	}

	load := func(position int64) []authority.AuthoritySnapshotEntry {
		t.Helper()
		var entries []authority.AuthoritySnapshotEntry
		err := repository.withAuthoritySnapshotTx(ctx, func(tx pgx.Tx) error {
			var err error
			entries, err = loadAuthoritySnapshotEntriesAtPosition(ctx, tx, position)
			return err
		})
		if err != nil {
			t.Fatalf("assemble at position %d: %v", position, err)
		}
		return entries
	}

	// Positions: authorization=1, adjudication=2,
	// SUBMIT=3, APPROVE=4, ACTIVATE=5.
	for _, tc := range []struct {
		position   int64
		wantEvents int
	}{
		{3, 1},
		{4, 2},
		{5, 3},
	} {
		entries := load(tc.position)
		if len(entries) != 1 {
			t.Fatalf("position %d: entries=%d, want 1", tc.position, len(entries))
		}
		entry := entries[0]
		if entry.Record.AuthorizationID != record.AuthorizationID ||
			entry.Record.Adjudication == nil ||
			*entry.Record.Adjudication != decision {
			t.Fatalf("position %d: incorrect authorization or adjudication", tc.position)
		}

		events := entry.Journal.Events()
		if len(events) != tc.wantEvents {
			t.Fatalf("position %d: events=%d, want %d",
				tc.position, len(events), tc.wantEvents)
		}
		for i, event := range events {
			if event.EventType != eventTypes[i] {
				t.Fatalf("position %d: event %d=%s, want %s",
					tc.position, i, event.EventType, eventTypes[i])
			}
		}
	}

	// An authorization without its adjudication is not complete evidence.
	if err := repository.withAuthoritySnapshotTx(ctx, func(tx pgx.Tx) error {
		_, err := loadAuthoritySnapshotEntriesAtPosition(ctx, tx, 1)
		if err == nil {
			t.Fatal("expected missing adjudication to fail closed")
		}
		return nil
	}); err != nil {
		t.Fatalf("early snapshot transaction: %v", err)
	}
}

func TestAuthorityRepositoryRecordingCompleteness(t *testing.T) {
	pool := openIsolatedAuthoritySnapshotPool(t)
	repository := NewAuthorityRepository(pool)
	ctx := context.Background()

	validate := func(position int64) error {
		t.Helper()
		return repository.withAuthoritySnapshotTx(ctx, func(tx pgx.Tx) error {
			return validateAuthorityRecordingCompleteness(ctx, tx, position)
		})
	}

	// An empty registry is complete only at boundary zero.
	if err := validate(0); err != nil {
		t.Fatalf("empty registry at boundary zero: %v", err)
	}
	if err := validate(1); err == nil {
		t.Fatal("expected missing recording position to fail")
	}

	for _, id := range []string{"complete-first", "complete-second"} {
		if err := repository.CreateAuthorization(ctx, authority.AuthorizationRecord{
			AuthorizationID:    id,
			IssuerID:           "issuer",
			AuthoritySubjectID: "subject",
			ClaimType:          "employment",
		}); err != nil {
			t.Fatalf("create authorization %s: %v", id, err)
		}
	}

	if err := validate(2); err != nil {
		t.Fatalf("complete recording sequence: %v", err)
	}
	if err := validate(1); err == nil {
		t.Fatal("expected evidence beyond boundary to fail")
	}
	if err := validate(3); err == nil {
		t.Fatal("expected missing position three to fail")
	}
	if err := validate(-1); err == nil {
		t.Fatal("expected negative boundary to fail")
	}

	// Simulate pre-migration legacy evidence in this disposable schema only.
	if _, err := pool.Exec(ctx, `
		ALTER TABLE authority_authorizations
		DISABLE TRIGGER authority_authorizations_recording_guard
	`); err != nil {
		t.Fatalf("disable recording guard for fixture: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO authority_authorizations (
			authorization_id, issuer_id, authority_subject_id, claim_type
		) VALUES ('complete-legacy', 'issuer', 'subject', 'employment')
	`); err != nil {
		t.Fatalf("insert legacy fixture: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		ALTER TABLE authority_authorizations
		ENABLE TRIGGER authority_authorizations_recording_guard
	`); err != nil {
		t.Fatalf("restore recording guard: %v", err)
	}

	if err := validate(2); err == nil ||
		!strings.Contains(err.Error(), "untrusted evidence rows") {
		t.Fatalf("expected legacy completeness failure, got %v", err)
	}
}

func TestAuthorityRepositoryLoadAuthoritativeSnapshotEmpty(t *testing.T) {
	pool := openIsolatedAuthoritySnapshotPool(t)
	repository := NewAuthorityRepository(pool)
	ctx := context.Background()

	snapshot, err := repository.LoadAuthoritativeSnapshot(ctx)
	if err != nil {
		t.Fatalf("load empty authoritative snapshot: %v", err)
	}
	if snapshot.RecordingPosition != 0 {
		t.Fatalf("expected empty recording position 0, got %d", snapshot.RecordingPosition)
	}
	if !snapshot.Complete {
		t.Fatal("empty authoritative snapshot must be complete")
	}
	if snapshot.RecordedThrough.IsZero() ||
		snapshot.RecordedThrough.Location() != time.UTC ||
		snapshot.RecordedThrough.Nanosecond()%1000 != 0 {
		t.Fatalf("invalid snapshot observation timestamp: %v",
			snapshot.RecordedThrough)
	}
	if len(snapshot.Entries) != 0 {
		t.Fatalf("expected no entries, got %d", len(snapshot.Entries))
	}

	resolver, err := authority.NewSnapshotAuthorityResolver(snapshot)
	if err != nil {
		t.Fatalf("construct resolver from empty snapshot: %v", err)
	}
	grants, err := resolver.ResolveAuthorityGrants(
		ctx, "unknown-issuer", "employment",
	)
	if err != nil {
		t.Fatalf("resolve empty authoritative snapshot: %v", err)
	}
	if len(grants) != 0 {
		t.Fatalf("expected no grants, got %d", len(grants))
	}
}

func TestAuthorityRepositoryLoadAuthoritativeSnapshotApproved(t *testing.T) {
	pool := openIsolatedAuthoritySnapshotPool(t)
	repository := NewAuthorityRepository(pool)
	ctx := context.Background()

	record := authority.AuthorizationRecord{
		AuthorizationID:    "authoritative-approved",
		IssuerID:           "approved-issuer",
		AuthoritySubjectID: "approved-subject",
		ClaimType:          "employment",
	}
	if err := repository.CreateAuthorization(ctx, record); err != nil {
		t.Fatalf("create authorization: %v", err)
	}

	now := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	decision := authority.AuthorizationAdjudication{
		DecisionID:         "authoritative-approved-decision",
		AuthorizationID:    record.AuthorizationID,
		IssuerID:           record.IssuerID,
		AuthoritySubjectID: record.AuthoritySubjectID,
		ClaimType:          record.ClaimType,
		Outcome:            authority.AdjudicationApproved,
		DecisionMakerID:    "approved-adjudicator",
		AuthorityBasis:     "verified organizational delegation",
		EvidenceRef:        "approved-decision-evidence",
		DecidedAt:          now,
		RecordedAt:         now,
	}
	if err := repository.CreateAdjudication(ctx, decision); err != nil {
		t.Fatalf("create adjudication: %v", err)
	}

	for i, eventType := range []authority.AuthorizationEvent{
		authority.EventSubmit,
		authority.EventApprove,
		authority.EventActivate,
	} {
		event := authority.AuthorizationJournalEvent{
			EventID:             fmt.Sprintf("authoritative-event-%d", i),
			AuthorizationID:     record.AuthorizationID,
			EventType:           eventType,
			EffectiveAt:         now.Add(time.Duration(i) * time.Minute),
			RecordedAt:          now.Add(time.Duration(i) * time.Minute),
			ActorID:             "approved-adjudicator",
			EvidenceRef:         fmt.Sprintf("approved-event-evidence-%d", i),
			ConditionsSatisfied: true,
		}
		if eventType == authority.EventApprove {
			event.EvidenceRef = decision.EvidenceRef
		}
		if err := repository.AppendJournalEvent(ctx, event); err != nil {
			t.Fatalf("append %s: %v", eventType, err)
		}
	}

	snapshot, err := repository.LoadAuthoritativeSnapshot(ctx)
	if err != nil {
		t.Fatalf("load authoritative snapshot: %v", err)
	}
	if snapshot.RecordingPosition != 5 {
		t.Fatalf("expected recording position 5, got %d", snapshot.RecordingPosition)
	}
	if !snapshot.Complete || len(snapshot.Entries) != 1 {
		t.Fatalf("unexpected snapshot: complete=%t entries=%d",
			snapshot.Complete, len(snapshot.Entries))
	}

	resolver, err := authority.NewSnapshotAuthorityResolver(snapshot)
	if err != nil {
		t.Fatalf("construct resolver: %v", err)
	}
	grants, err := resolver.ResolveAuthorityGrants(
		ctx, record.IssuerID, record.ClaimType,
	)
	if err != nil {
		t.Fatalf("resolve approved authority: %v", err)
	}
	if len(grants) != 1 ||
		grants[0].State != authority.AuthorityGrantActive ||
		grants[0].AuthoritySubjectID != record.AuthoritySubjectID {
		t.Fatalf("expected one active grant, got %+v", grants)
	}
}

func TestAuthorityRepositoryLoadAuthoritativeSnapshotRejectsLegacyEvidence(t *testing.T) {
	pool := openIsolatedAuthoritySnapshotPool(t)
	repository := NewAuthorityRepository(pool)
	ctx := context.Background()

	// This disposable schema simulates an authorization created before
	// recording positions were introduced.
	if _, err := pool.Exec(ctx, `
		ALTER TABLE authority_authorizations
		DISABLE TRIGGER authority_authorizations_recording_guard
	`); err != nil {
		t.Fatalf("disable recording guard for legacy fixture: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO authority_authorizations (
			authorization_id, issuer_id, authority_subject_id, claim_type
		) VALUES (
			'legacy-unpositioned', 'legacy-issuer',
			'legacy-subject', 'employment'
		)
	`); err != nil {
		t.Fatalf("insert legacy authorization: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		ALTER TABLE authority_authorizations
		ENABLE TRIGGER authority_authorizations_recording_guard
	`); err != nil {
		t.Fatalf("restore recording guard: %v", err)
	}

	snapshot, err := repository.LoadAuthoritativeSnapshot(ctx)
	if err == nil {
		t.Fatal("legacy evidence must prevent authoritative snapshot production")
	}
	if !strings.Contains(err.Error(), "untrusted evidence rows") {
		t.Fatalf("unexpected legacy rejection error: %v", err)
	}
	if snapshot.Complete || len(snapshot.Entries) != 0 {
		t.Fatalf("failed snapshot must not expose trusted evidence: %+v", snapshot)
	}
}

func TestAuthorityRepositoryLoadAuthoritativeSnapshotRejectsFutureRecordedEvidence(t *testing.T) {
	pool := openIsolatedAuthoritySnapshotPool(t)
	repository := NewAuthorityRepository(pool)
	ctx := context.Background()

	record := authority.AuthorizationRecord{
		AuthorizationID:    "future-recorded-authorization",
		IssuerID:           "future-issuer",
		AuthoritySubjectID: "future-subject",
		ClaimType:          "employment",
	}
	if err := repository.CreateAuthorization(ctx, record); err != nil {
		t.Fatalf("create authorization: %v", err)
	}

	future := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Microsecond)
	decision := authority.AuthorizationAdjudication{
		DecisionID:         "future-recorded-decision",
		AuthorizationID:    record.AuthorizationID,
		IssuerID:           record.IssuerID,
		AuthoritySubjectID: record.AuthoritySubjectID,
		ClaimType:          record.ClaimType,
		Outcome:            authority.AdjudicationApproved,
		DecisionMakerID:    "future-adjudicator",
		AuthorityBasis:     "verified organizational delegation",
		EvidenceRef:        "future-evidence",
		DecidedAt:          future,
		RecordedAt:         future,
	}
	if err := repository.CreateAdjudication(ctx, decision); err != nil {
		t.Fatalf("create future-recorded adjudication: %v", err)
	}

	snapshot, err := repository.LoadAuthoritativeSnapshot(ctx)
	if err == nil {
		t.Fatal("future-recorded evidence must fail closed")
	}
	if !strings.Contains(err.Error(), "validate authoritative snapshot") {
		t.Fatalf("expected resolver validation failure, got %v", err)
	}
	if snapshot.Complete || len(snapshot.Entries) != 0 {
		t.Fatalf("failed snapshot exposed trusted evidence: %+v", snapshot)
	}
}

func TestAuthorityRepositorySnapshotAssemblyStableAcrossConcurrentCommit(t *testing.T) {
	pool := openIsolatedAuthoritySnapshotPool(t)
	repository := NewAuthorityRepository(pool)
	ctx := context.Background()

	// Establish a valid, empty authority registry at recording position zero.
	err := repository.withAuthoritySnapshotTx(ctx, func(tx pgx.Tx) error {
		position, err := loadAuthorityRecordingBoundary(ctx, tx)
		if err != nil {
			return err
		}
		if position != 0 {
			t.Fatalf("initial recording boundary = %d, want 0", position)
		}

		// This commits through a different pooled connection after the
		// repeatable-read snapshot has already been established.
		if err := repository.CreateAuthorization(ctx, authority.AuthorizationRecord{
			AuthorizationID:    "concurrent-snapshot-authorization",
			IssuerID:           "concurrent-issuer",
			AuthoritySubjectID: "concurrent-subject",
			ClaimType:          "employment",
		}); err != nil {
			return fmt.Errorf("concurrent authorization commit: %w", err)
		}

		observedAt, err := loadAuthoritySnapshotObservedAt(ctx, tx)
		if err != nil {
			return err
		}
		if !observedAt.After(time.Time{}) {
			t.Fatal("invalid observation timestamp")
		}

		if err := validateAuthorityRecordingCompleteness(ctx, tx, position); err != nil {
			return fmt.Errorf("snapshot completeness changed after commit: %w", err)
		}

		entries, err := loadAuthoritySnapshotEntriesAtPosition(ctx, tx, position)
		if err != nil {
			return err
		}
		if len(entries) != 0 {
			t.Fatalf("snapshot included concurrent evidence: %+v", entries)
		}

		after, err := loadAuthorityRecordingBoundary(ctx, tx)
		if err != nil {
			return err
		}
		if after != position {
			t.Fatalf("recording boundary changed from %d to %d", position, after)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("consistent snapshot assembly: %v", err)
	}

	// A new transaction must see the committed authorization.
	err = repository.withAuthoritySnapshotTx(ctx, func(tx pgx.Tx) error {
		position, err := loadAuthorityRecordingBoundary(ctx, tx)
		if err != nil {
			return err
		}
		if position != 1 {
			t.Fatalf("fresh recording boundary = %d, want 1", position)
		}

		records, err := loadAuthorityRecordsAtPosition(ctx, tx, position)
		if err != nil {
			return err
		}
		if len(records) != 1 ||
			records[0].AuthorizationID != "concurrent-snapshot-authorization" {
			t.Fatalf("fresh snapshot did not see committed evidence: %+v", records)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("fresh snapshot: %v", err)
	}
}

func TestAuthorityRepositoryAllocatorRestrictedRole(t *testing.T) {
	pool := openIsolatedAuthoritySnapshotPool(t)
	ctx := context.Background()

	var schema string
	if err := pool.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema); err != nil {
		t.Fatalf("read isolated schema: %v", err)
	}
	if !strings.HasPrefix(schema, "authority_snapshot_") {
		t.Fatalf("refusing unexpected schema %q", schema)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin privilege test: %v", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	const role = "cvera_allocator_probe"
	if _, err := tx.Exec(ctx, `CREATE ROLE `+role+` NOLOGIN`); err != nil {
		t.Fatalf("create transactional probe role: %v", err)
	}
	if _, err := tx.Exec(ctx, `GRANT USAGE ON SCHEMA "`+schema+`" TO `+role); err != nil {
		t.Fatalf("grant schema usage: %v", err)
	}

	if _, err := tx.Exec(ctx, `SET LOCAL ROLE `+role); err != nil {
		t.Fatalf("assume restricted role: %v", err)
	}

	var allowed bool
	if err := tx.QueryRow(ctx,
		`SELECT has_function_privilege(current_user, $1, 'EXECUTE')`,
		schema+".next_authority_recording_position()",
	).Scan(&allowed); err != nil {
		t.Fatalf("inspect allocator permission: %v", err)
	}
	if allowed {
		t.Fatal("allocator must not be executable without an explicit grant")
	}

	if err := tx.QueryRow(ctx,
		`SELECT has_table_privilege(current_user, $1, 'UPDATE')`,
		schema+".authority_recording_boundary",
	).Scan(&allowed); err != nil {
		t.Fatalf("inspect boundary permission: %v", err)
	}
	if allowed {
		t.Fatal("restricted role must not update recording boundary directly")
	}

	if _, err := tx.Exec(ctx, `RESET ROLE`); err != nil {
		t.Fatalf("restore migration role: %v", err)
	}
	if _, err := tx.Exec(ctx,
		`GRANT EXECUTE ON FUNCTION "`+schema+`".next_authority_recording_position() TO `+role,
	); err != nil {
		t.Fatalf("grant allocator execution: %v", err)
	}
	if _, err := tx.Exec(ctx, `SET LOCAL ROLE `+role); err != nil {
		t.Fatalf("reassume restricted role: %v", err)
	}

	// The caller controls this schema, but must not redirect privileged SQL.
	if _, err := tx.Exec(ctx, `RESET ROLE`); err != nil {
		t.Fatalf("restore migration role for decoy setup: %v", err)
	}
	if _, err := tx.Exec(ctx, `CREATE SCHEMA cvera_allocator_decoy AUTHORIZATION `+role); err != nil {
		t.Fatalf("create caller-owned decoy schema: %v", err)
	}
	if _, err := tx.Exec(ctx, `SET LOCAL ROLE `+role); err != nil {
		t.Fatalf("reassume restricted role for decoy setup: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		CREATE TABLE cvera_allocator_decoy.authority_recording_boundary (
			singleton BOOLEAN PRIMARY KEY,
			committed_position BIGINT NOT NULL,
			allocating_xid xid8
		)
	`); err != nil {
		t.Fatalf("create decoy boundary: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO cvera_allocator_decoy.authority_recording_boundary
		(singleton, committed_position) VALUES (TRUE, 900)
	`); err != nil {
		t.Fatalf("initialize decoy boundary: %v", err)
	}
	if _, err := tx.Exec(ctx, `SET LOCAL search_path TO cvera_allocator_decoy`); err != nil {
		t.Fatalf("set attacker-controlled search path: %v", err)
	}

	var position int64
	if err := tx.QueryRow(ctx,
		`SELECT "`+schema+`".next_authority_recording_position()`,
	).Scan(&position); err != nil {
		t.Fatalf("restricted allocator execution: %v", err)
	}
	if position != 1 {
		t.Fatalf("allocated position = %d, want 1", position)
	}

	var decoyPosition int64
	if err := tx.QueryRow(ctx, `
		SELECT committed_position
		FROM cvera_allocator_decoy.authority_recording_boundary
	`).Scan(&decoyPosition); err != nil {
		t.Fatalf("read decoy boundary: %v", err)
	}
	if decoyPosition != 900 {
		t.Fatalf("allocator modified caller-controlled boundary: %d", decoyPosition)
	}

	// The allocation is intentionally rolled back. No evidence is inserted.
}

func TestAuthorityRepositoryRestrictedRoleJournalInsert(t *testing.T) {
	pool := openIsolatedAuthoritySnapshotPool(t)
	ctx := context.Background()

	var schema string
	if err := pool.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema); err != nil {
		t.Fatalf("read isolated schema: %v", err)
	}
	if !strings.HasPrefix(schema, "authority_snapshot_") {
		t.Fatalf("unexpected isolated schema %q", schema)
	}

	id := fmt.Sprintf("restricted-journal-%d", time.Now().UnixNano())
	repository := NewAuthorityRepository(pool)
	if err := repository.CreateAuthorization(ctx, authority.AuthorizationRecord{
		AuthorizationID:    id,
		IssuerID:           "integration-issuer",
		AuthoritySubjectID: "integration-subject",
		ClaimType:          "employment",
	}); err != nil {
		t.Fatalf("create prerequisite authorization: %v", err)
	}

	const role = "cvera_journal_probe"
	// Registered after the fixture cleanup, so this runs first.
	// Revoke schema dependencies before dropping the cluster-wide role.
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if _, err := pool.Exec(cleanupCtx,
			`REVOKE ALL PRIVILEGES ON ALL TABLES IN SCHEMA "`+schema+`" FROM `+role,
		); err != nil {
			t.Errorf("revoke probe table privileges: %v", err)
		}
		if _, err := pool.Exec(cleanupCtx,
			`REVOKE ALL PRIVILEGES ON ALL FUNCTIONS IN SCHEMA "`+schema+`" FROM `+role,
		); err != nil {
			t.Errorf("revoke probe function privileges: %v", err)
		}
		if _, err := pool.Exec(cleanupCtx,
			`REVOKE USAGE ON SCHEMA "`+schema+`" FROM `+role,
		); err != nil {
			t.Errorf("revoke probe schema usage: %v", err)
		}
		if _, err := pool.Exec(cleanupCtx, `DROP ROLE `+role); err != nil {
			t.Errorf("drop probe role: %v", err)
		}
	})

	adminTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin role setup: %v", err)
	}
	defer func() { _ = adminTx.Rollback(context.Background()) }()

	grants := []string{
		`CREATE ROLE ` + role + ` NOLOGIN`,
		`GRANT USAGE ON SCHEMA "` + schema + `" TO ` + role,
		`GRANT SELECT ON "` + schema + `".authority_recording_boundary TO ` + role,
		`GRANT SELECT ON "` + schema + `".authority_authorizations TO ` + role,
		`GRANT SELECT ON "` + schema + `".authority_adjudications TO ` + role,
		`GRANT SELECT, INSERT ON "` + schema + `".authority_journal_events TO ` + role,
		`GRANT UPDATE (authorization_id) ON "` + schema + `".authority_authorizations TO ` + role,
		`GRANT EXECUTE ON FUNCTION "` + schema + `".next_authority_recording_position() TO ` + role,
	}
	for _, statement := range grants {
		if _, err := adminTx.Exec(ctx, statement); err != nil {
			t.Fatalf("role setup %q: %v", statement, err)
		}
	}
	if err := adminTx.Commit(ctx); err != nil {
		t.Fatalf("commit restricted-role grants: %v", err)
	}

	// The NOLOGIN role is assumed only within this transaction.
	// Explicit cleanup removes the cluster-wide role after the test.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin restricted journal transaction: %v", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	if _, err := tx.Exec(ctx, `SET LOCAL ROLE `+role); err != nil {
		t.Fatalf("assume restricted role: %v", err)
	}

	var sequencePrivilege bool
	if err := tx.QueryRow(ctx, `
		SELECT has_sequence_privilege(
			current_user,
			pg_get_serial_sequence($1, 'journal_sequence'),
			'USAGE'
		)
	`, schema+".authority_journal_events").Scan(&sequencePrivilege); err != nil {
		t.Fatalf("inspect journal sequence privilege: %v", err)
	}
	if sequencePrivilege {
		t.Fatal("restricted role unexpectedly has sequence USAGE")
	}

	var position int64
	if err := tx.QueryRow(ctx, `SELECT next_authority_recording_position()`).Scan(&position); err != nil {
		t.Fatalf("allocate journal position: %v", err)
	}
	if position != 2 {
		t.Fatalf("journal position = %d, want 2", position)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	_, err = tx.Exec(ctx, `
		INSERT INTO authority_journal_events (
			event_id, authorization_id, event_type, effective_at,
			recorded_at, actor_id, evidence_ref,
			conditions_satisfied, recording_position
		) VALUES ($1, $2, 'SUBMIT', $3, $3, $4, $5, TRUE, $6)
	`, id+"-submit", id, now, "integration-actor", "integration-evidence", position)
	if err != nil {
		t.Fatalf("restricted journal insert without sequence USAGE: %v", err)
	}

	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit restricted journal insert and deferred guards: %v", err)
	}

	var storedPosition int64
	if err := pool.QueryRow(ctx, `
		SELECT recording_position FROM authority_journal_events WHERE event_id = $1
	`, id+"-submit").Scan(&storedPosition); err != nil {
		t.Fatalf("read committed journal event: %v", err)
	}
	if storedPosition != 2 {
		t.Fatalf("stored journal position = %d, want 2", storedPosition)
	}
}
