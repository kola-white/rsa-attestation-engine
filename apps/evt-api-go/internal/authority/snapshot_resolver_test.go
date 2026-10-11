package authority

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSnapshotAuthorityResolver_RequiresCompleteSnapshot(t *testing.T) {
	resolver, err := NewSnapshotAuthorityResolver(
		AuthoritySnapshot{
			Complete: false,
		},
	)

	if err == nil {
		t.Fatal("incomplete snapshot must be rejected")
	}

	if resolver != nil {
		t.Fatal("rejected snapshot must not produce a resolver")
	}
}

func TestSnapshotAuthorityResolver_CompleteEmptySnapshot(t *testing.T) {
	resolver, err := NewSnapshotAuthorityResolver(
		AuthoritySnapshot{
			Complete:        true,
			RecordedThrough: time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC),
		},
	)
	if err != nil {
		t.Fatalf("construct resolver: %v", err)
	}

	grants, err := resolver.ResolveAuthorityGrants(
		context.Background(),
		"issuer-456",
		"employment.role",
	)
	if err != nil {
		t.Fatalf("resolve complete empty snapshot: %v", err)
	}

	if len(grants) != 0 {
		t.Fatalf("expected no grants, got %+v", grants)
	}
}

func TestSnapshotAuthorityResolver_ProjectsMatchingAuthorization(t *testing.T) {
	start := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)

	var journal AuthorizationJournal

	events := []AuthorizationJournalEvent{
		journalEvent("event-1", EventSubmit, start, start),
		journalEvent("event-2", EventApprove,
			start.Add(time.Hour), start.Add(time.Hour)),
		journalEvent("event-3", EventActivate,
			start.Add(2*time.Hour), start.Add(2*time.Hour)),
	}
	events[1].EvidenceRef = "authority-evidence-123"

	for _, event := range events {
		if err := journal.Append(event); err != nil {
			t.Fatalf("append event: %v", err)
		}
	}

	record := AuthorizationRecord{
		AuthorizationID:    "authorization-123",
		IssuerID:           "issuer-456",
		AuthoritySubjectID: "employer-789",
		ClaimType:          "employment.role",
		Adjudication: &AuthorizationAdjudication{
			DecisionID:         "decision-123",
			AuthorizationID:    "authorization-123",
			IssuerID:           "issuer-456",
			AuthoritySubjectID: "employer-789",
			ClaimType:          "employment.role",
			Outcome:            AdjudicationApproved,
			DecisionMakerID:    "cvera-adjudicator-1",
			AuthorityBasis:     "verified organizational delegation",
			EvidenceRef:        "authority-evidence-123",
			DecidedAt:          start.Add(time.Hour),
			RecordedAt:         start.Add(time.Hour),
		},
	}

	resolver, err := NewSnapshotAuthorityResolver(AuthoritySnapshot{
		Complete:        true,
		RecordedThrough: start.Add(3 * time.Hour),
		Entries: []AuthoritySnapshotEntry{
			{Record: record, Journal: journal},
		},
	})
	if err != nil {
		t.Fatalf("construct resolver: %v", err)
	}

	grants, err := resolver.ResolveAuthorityGrants(
		context.Background(),
		"issuer-456",
		"employment.role",
	)
	if err != nil {
		t.Fatalf("resolve grants: %v", err)
	}

	if len(grants) != 1 {
		t.Fatalf("expected one grant, got %+v", grants)
	}

	if grants[0].State != AuthorityGrantActive ||
		!grants[0].ValidFrom.Equal(start.Add(2*time.Hour)) ||
		grants[0].IssuerID != record.IssuerID ||
		grants[0].AuthoritySubjectID != record.AuthoritySubjectID {
		t.Fatalf("unexpected projected grant: %+v", grants[0])
	}
}
func TestSnapshotAuthorityResolver_RejectsEventsBeyondSnapshotBoundary(t *testing.T) {
	start := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	cutoff := start.Add(3 * time.Hour)

	var journal AuthorizationJournal

	events := []AuthorizationJournalEvent{
		journalEvent("event-1", EventSubmit, start, start),
		journalEvent("event-2", EventApprove,
			start.Add(time.Hour), start.Add(time.Hour)),
		journalEvent("event-3", EventActivate,
			start.Add(2*time.Hour), start.Add(2*time.Hour)),
		journalEvent("event-4", EventSuspend,
			start.Add(4*time.Hour), start.Add(4*time.Hour)),
	}
	events[1].EvidenceRef = "authority-evidence-123"

	for _, event := range events {
		if err := journal.Append(event); err != nil {
			t.Fatalf("append event: %v", err)
		}
	}

	record := AuthorizationRecord{
		AuthorizationID:    "authorization-123",
		IssuerID:           "issuer-456",
		AuthoritySubjectID: "employer-789",
		ClaimType:          "employment.role",
		Adjudication: &AuthorizationAdjudication{
			DecisionID:         "decision-123",
			AuthorizationID:    "authorization-123",
			IssuerID:           "issuer-456",
			AuthoritySubjectID: "employer-789",
			ClaimType:          "employment.role",
			Outcome:            AdjudicationApproved,
			DecisionMakerID:    "cvera-adjudicator-1",
			AuthorityBasis:     "verified organizational delegation",
			EvidenceRef:        "authority-evidence-123",
			DecidedAt:          start.Add(time.Hour),
			RecordedAt:         start.Add(time.Hour),
		},
	}

	// The history through cutoff is valid and produces an active grant.
	// The snapshot nevertheless contains a later recorded suspension,
	// so it must not be accepted as complete through this boundary.
	resolver, err := NewSnapshotAuthorityResolver(AuthoritySnapshot{
		Complete:        true,
		RecordedThrough: cutoff,
		Entries: []AuthoritySnapshotEntry{
			{Record: record, Journal: journal},
		},
	})

	if err == nil {
		t.Fatal("snapshot containing events beyond its recorded boundary must fail")
	}
	if resolver != nil {
		t.Fatal("invalid snapshot must not produce a resolver")
	}
}

func TestSnapshotAuthorityResolver_RejectsMissingSnapshotBoundary(t *testing.T) {
	resolver, err := NewSnapshotAuthorityResolver(AuthoritySnapshot{
		Complete: true,
	})

	if err == nil {
		t.Fatal("complete snapshot without a recorded-time boundary must fail")
	}
	if resolver != nil {
		t.Fatal("invalid snapshot must not produce a resolver")
	}
}

func TestSnapshotAuthorityResolver_ReturnsDefensiveCopies(t *testing.T) {
	start := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)

	resolver := &SnapshotAuthorityResolver{
		grants: []AuthorityGrant{
			{
				GrantID:            "grant-123",
				IssuerID:           "issuer-456",
				ClaimType:          "employment.role",
				AuthoritySubjectID: "employer-789",
				ValidFrom:          start,
				ValidUntil:         &end,
				State:              AuthorityGrantActive,
			},
		},
	}

	first, err := resolver.ResolveAuthorityGrants(
		context.Background(),
		"issuer-456",
		"employment.role",
	)
	if err != nil {
		t.Fatalf("first resolution: %v", err)
	}
	if len(first) != 1 || first[0].ValidUntil == nil {
		t.Fatalf("unexpected first resolution: %+v", first)
	}

	first[0].State = AuthorityGrantInactive
	*first[0].ValidUntil = start

	second, err := resolver.ResolveAuthorityGrants(
		context.Background(),
		"issuer-456",
		"employment.role",
	)
	if err != nil {
		t.Fatalf("second resolution: %v", err)
	}
	if len(second) != 1 ||
		second[0].State != AuthorityGrantActive ||
		second[0].ValidUntil == nil ||
		!second[0].ValidUntil.Equal(end) {
		t.Fatalf("resolver state was mutated: %+v", second)
	}
}

func TestSnapshotAuthorityResolver_RejectsDuplicateAuthorizationIDs(t *testing.T) {
	start := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)

	record := AuthorizationRecord{
		AuthorizationID:    "authorization-123",
		IssuerID:           "issuer-456",
		AuthoritySubjectID: "employer-789",
		ClaimType:          "employment.role",
	}

	resolver, err := NewSnapshotAuthorityResolver(AuthoritySnapshot{
		Complete:        true,
		RecordedThrough: start,
		Entries: []AuthoritySnapshotEntry{
			{Record: record},
			{Record: record},
		},
	})

	if err == nil {
		t.Fatal("duplicate authorization IDs must be rejected")
	}
	if resolver != nil {
		t.Fatal("invalid snapshot must not produce a resolver")
	}
}

func TestSnapshotAuthorityResolver_RejectsCanceledContext(t *testing.T) {
	resolver, err := NewSnapshotAuthorityResolver(AuthoritySnapshot{
		Complete:        true,
		RecordedThrough: time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("construct resolver: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	grants, err := resolver.ResolveAuthorityGrants(
		ctx,
		"issuer-456",
		"employment.role",
	)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if len(grants) != 0 {
		t.Fatalf("canceled resolution returned grants: %+v", grants)
	}
}

func snapshotTestEntry(
	t *testing.T,
	id, issuer, employer string,
	start time.Time,
	suspended bool,
) AuthoritySnapshotEntry {
	t.Helper()

	var journal AuthorizationJournal

	events := []AuthorizationJournalEvent{
		{
			EventID: "submit-" + id, AuthorizationID: id,
			EventType:   EventSubmit,
			EffectiveAt: start, RecordedAt: start,
			ActorID: "adjudicator", EvidenceRef: "evidence-" + id,
			ConditionsSatisfied: true,
		},
		{
			EventID: "approve-" + id, AuthorizationID: id,
			EventType:   EventApprove,
			EffectiveAt: start.Add(time.Hour),
			RecordedAt:  start.Add(time.Hour),
			ActorID:     "adjudicator", EvidenceRef: "evidence-" + id,
			ConditionsSatisfied: true,
		},
		{
			EventID: "activate-" + id, AuthorizationID: id,
			EventType:   EventActivate,
			EffectiveAt: start.Add(2 * time.Hour),
			RecordedAt:  start.Add(2 * time.Hour),
			ActorID:     "adjudicator", EvidenceRef: "evidence-" + id,
			ConditionsSatisfied: true,
		},
	}

	if suspended {
		events = append(events, AuthorizationJournalEvent{
			EventID: "suspend-" + id, AuthorizationID: id,
			EventType:   EventSuspend,
			EffectiveAt: start.Add(4 * time.Hour),
			RecordedAt:  start.Add(4 * time.Hour),
			ActorID:     "adjudicator", EvidenceRef: "evidence-" + id,
			ConditionsSatisfied: true,
		})
	}

	for _, event := range events {
		if err := journal.Append(event); err != nil {
			t.Fatalf("append %s: %v", event.EventID, err)
		}
	}

	return AuthoritySnapshotEntry{
		Record: AuthorizationRecord{
			AuthorizationID:    id,
			IssuerID:           issuer,
			AuthoritySubjectID: employer,
			ClaimType:          "employment.role",
			Adjudication: &AuthorizationAdjudication{
				DecisionID:         "decision-" + id,
				AuthorizationID:    id,
				IssuerID:           issuer,
				AuthoritySubjectID: employer,
				ClaimType:          "employment.role",
				Outcome:            AdjudicationApproved,
				DecisionMakerID:    "adjudicator",
				AuthorityBasis:     "verified organizational delegation",
				EvidenceRef:        "evidence-" + id,
				DecidedAt:          start.Add(time.Hour),
				RecordedAt:         start.Add(time.Hour),
			},
		},
		Journal: journal,
	}
}

func TestSnapshotAuthorityResolver_RetrievesAllMatchingEmployers(t *testing.T) {
	start := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)

	resolver, err := NewSnapshotAuthorityResolver(AuthoritySnapshot{
		Complete:        true,
		RecordedThrough: start.Add(5 * time.Hour),
		Entries: []AuthoritySnapshotEntry{
			snapshotTestEntry(t, "auth-1", "issuer-A", "employer-1", start, false),
			snapshotTestEntry(t, "auth-2", "issuer-A", "employer-2", start, false),
			snapshotTestEntry(t, "auth-3", "issuer-B", "employer-3", start, false),
		},
	})
	if err != nil {
		t.Fatalf("construct resolver: %v", err)
	}

	grants, err := resolver.ResolveAuthorityGrants(
		context.Background(), "issuer-A", "employment.role",
	)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(grants) != 2 {
		t.Fatalf("expected both matching employers, got %+v", grants)
	}

	subjects := map[string]bool{}
	for _, grant := range grants {
		subjects[grant.AuthoritySubjectID] = true
	}
	if !subjects["employer-1"] || !subjects["employer-2"] {
		t.Fatalf("missing matching employer: %+v", grants)
	}

	other, err := resolver.ResolveAuthorityGrants(
		context.Background(), "issuer-B", "employment.role",
	)
	if err != nil || len(other) != 1 || other[0].AuthoritySubjectID != "employer-3" {
		t.Fatalf("issuer filtering failed: grants=%+v err=%v", other, err)
	}

	none, err := resolver.ResolveAuthorityGrants(
		context.Background(), "issuer-A", "other.claim",
	)
	if err != nil || len(none) != 0 {
		t.Fatalf("claim-type filtering failed: grants=%+v err=%v", none, err)
	}
}

func TestSnapshotAuthorityResolver_PreservesHistoricalIntervals(t *testing.T) {
	start := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	suspension := start.Add(4 * time.Hour)

	resolver, err := NewSnapshotAuthorityResolver(AuthoritySnapshot{
		Complete:        true,
		RecordedThrough: start.Add(5 * time.Hour),
		Entries: []AuthoritySnapshotEntry{
			snapshotTestEntry(t, "auth-1", "issuer-A", "employer-1", start, true),
		},
	})
	if err != nil {
		t.Fatalf("construct resolver: %v", err)
	}

	grants, err := resolver.ResolveAuthorityGrants(
		context.Background(), "issuer-A", "employment.role",
	)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(grants) != 2 {
		t.Fatalf("expected active and inactive intervals, got %+v", grants)
	}

	active, inactive := grants[0], grants[1]

	if active.State != AuthorityGrantActive ||
		!active.ValidFrom.Equal(start.Add(2*time.Hour)) ||
		active.ValidUntil == nil ||
		!active.ValidUntil.Equal(suspension.Add(-time.Microsecond)) {
		t.Fatalf("incorrect historical active interval: %+v", active)
	}

	if inactive.State != AuthorityGrantInactive ||
		!inactive.ValidFrom.Equal(suspension) ||
		inactive.ValidUntil != nil {
		t.Fatalf("incorrect suspension interval: %+v", inactive)
	}
}
