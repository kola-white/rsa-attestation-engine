package authority

import (
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/credential"
	"reflect"
	"testing"
	"time"
)

func journalEvent(
	id string,
	eventType AuthorizationEvent,
	effectiveAt time.Time,
	recordedAt time.Time,
) AuthorizationJournalEvent {
	return AuthorizationJournalEvent{
		EventID:             id,
		AuthorizationID:     "authorization-123",
		EventType:           eventType,
		EffectiveAt:         effectiveAt,
		RecordedAt:          recordedAt,
		ActorID:             "cvera-adjudicator-1",
		EvidenceRef:         "evidence-123",
		ConditionsSatisfied: true,
	}
}

func TestAuthorizationJournal_AppendPreservesOriginalEvents(t *testing.T) {
	var journal AuthorizationJournal

	at := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	original := journalEvent("event-1", EventSubmit, at, at)

	if err := journal.Append(original); err != nil {
		t.Fatalf("append original: %v", err)
	}

	original.ActorID = "modified-after-append"

	stored := journal.Events()
	if len(stored) != 1 {
		t.Fatalf("stored events = %d, want 1", len(stored))
	}
	if stored[0].ActorID != "cvera-adjudicator-1" {
		t.Fatal("stored original was mutated")
	}

	stored[0].ActorID = "modified-returned-copy"

	again := journal.Events()
	if again[0].ActorID != "cvera-adjudicator-1" {
		t.Fatal("journal exposed mutable internal state")
	}
}

func TestAuthorizationJournal_CorrectionPreservesProvenance(t *testing.T) {
	var journal AuthorizationJournal

	originalTime := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	correctionTime := originalTime.Add(24 * time.Hour)

	original := journalEvent(
		"event-1",
		EventSubmit,
		originalTime,
		originalTime,
	)

	if err := journal.Append(original); err != nil {
		t.Fatalf("append original: %v", err)
	}

	correction := journalEvent(
		"event-2",
		EventSubmit,
		originalTime.Add(time.Hour),
		correctionTime,
	)
	correction.CorrectsEventID = "event-1"
	correction.CorrectionReason = "Original effective date was recorded incorrectly"
	correction.CorrectedField = "EffectiveAt"
	correction.EvidenceRef = "correction-evidence-456"

	if err := journal.Append(correction); err != nil {
		t.Fatalf("append correction: %v", err)
	}

	events := journal.Events()
	if len(events) != 2 {
		t.Fatalf("stored events = %d, want 2", len(events))
	}

	if events[0].EventID != "event-1" {
		t.Fatal("original event was replaced")
	}
	if events[1].CorrectsEventID != "event-1" {
		t.Fatal("correction reference was not preserved")
	}
	if events[1].RecordedAt != correctionTime {
		t.Fatal("correction recorded timestamp was not preserved")
	}
	if events[1].EvidenceRef != "correction-evidence-456" {
		t.Fatal("correction evidence provenance was not preserved")
	}
}

func TestAuthorizationJournal_DeterministicReplay(t *testing.T) {
	var journal AuthorizationJournal

	start := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)

	events := []AuthorizationJournalEvent{
		journalEvent("event-1", EventSubmit, start, start),
		journalEvent("event-2", EventApprove, start.Add(time.Hour), start.Add(time.Hour)),
	}

	for _, event := range events {
		if err := journal.Append(event); err != nil {
			t.Fatalf("append %s: %v", event.EventID, err)
		}
	}

	cutoff := start.Add(2 * time.Hour)

	first, err := journal.Reconstruct("authorization-123", cutoff)
	if err != nil {
		t.Fatalf("first reconstruction: %v", err)
	}

	second, err := journal.Reconstruct("authorization-123", cutoff)
	if err != nil {
		t.Fatalf("second reconstruction: %v", err)
	}

	if !reflect.DeepEqual(first, second) {
		t.Fatalf("reconstruction not deterministic: %v vs %v", first, second)
	}

	if first != AuthorizationApproved {
		t.Fatalf(
			"reconstructed state = %q, want %q",
			first,
			AuthorizationApproved,
		)
	}
}

func TestAuthorizationJournal_RejectsIncompleteAndContradictoryHistory(t *testing.T) {
	t.Run("missing event identity", func(t *testing.T) {
		var journal AuthorizationJournal
		at := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)

		event := journalEvent("", EventSubmit, at, at)
		if err := journal.Append(event); err == nil {
			t.Fatal("expected missing event identity rejection")
		}
	})

	t.Run("duplicate event identity", func(t *testing.T) {
		var journal AuthorizationJournal
		at := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)

		event := journalEvent("event-1", EventSubmit, at, at)

		if err := journal.Append(event); err != nil {
			t.Fatalf("first append: %v", err)
		}
		if err := journal.Append(event); err == nil {
			t.Fatal("expected duplicate event rejection")
		}
	})

	t.Run("correction target missing", func(t *testing.T) {
		var journal AuthorizationJournal
		at := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)

		event := journalEvent("event-2", EventSubmit, at, at)
		event.CorrectsEventID = "missing-event"

		if err := journal.Append(event); err == nil {
			t.Fatal("expected missing correction target rejection")
		}
	})

	t.Run("invalid lifecycle sequence", func(t *testing.T) {
		var journal AuthorizationJournal
		at := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)

		event := journalEvent("event-1", EventActivate, at, at)

		if err := journal.Append(event); err == nil {
			t.Fatal("expected invalid lifecycle transition rejection")
		}
	})
}
func TestAuthorizationJournal_RejectsUnestablishedTransition(t *testing.T) {
	var journal AuthorizationJournal

	start := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)

	events := []AuthorizationJournalEvent{
		journalEvent("event-1", EventSubmit, start, start),
		journalEvent(
			"event-2",
			EventApprove,
			start.Add(time.Hour),
			start.Add(time.Hour),
		),
	}

	for _, event := range events {
		event.ConditionsSatisfied = true

		if err := journal.Append(event); err != nil {
			t.Fatalf("append %s: %v", event.EventID, err)
		}
	}

	activation := journalEvent(
		"event-3",
		EventActivate,
		start.Add(2*time.Hour),
		start.Add(2*time.Hour),
	)

	// No established authorization conditions accompany activation.
	activation.ConditionsSatisfied = false

	if err := journal.Append(activation); err == nil {
		t.Fatal("activation accepted without established conditions")
	}

	state, err := journal.Reconstruct(
		"authorization-123",
		start.Add(3*time.Hour),
	)
	if err != nil {
		t.Fatalf("reconstruct: %v", err)
	}

	if state != AuthorizationApproved {
		t.Fatalf(
			"state = %q, want %q",
			state,
			AuthorizationApproved,
		)
	}
}
func TestAuthorizationJournal_RequiresCorrectionProvenance(t *testing.T) {
	start := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name   string
		reason string
		field  string
		wantOK bool
	}{
		{
			name:  "missing reason",
			field: "EffectiveAt",
		},
		{
			name:   "missing corrected field",
			reason: "Original effective date was recorded incorrectly",
		},
		{
			name:   "complete correction provenance",
			reason: "Original effective date was recorded incorrectly",
			field:  "EffectiveAt",
			wantOK: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var journal AuthorizationJournal

			original := journalEvent("event-1", EventSubmit, start, start)
			if err := journal.Append(original); err != nil {
				t.Fatalf("append original: %v", err)
			}

			correction := journalEvent(
				"event-2",
				EventSubmit,
				start.Add(time.Hour),
				start.Add(24*time.Hour),
			)
			correction.CorrectsEventID = "event-1"
			correction.CorrectionReason = tt.reason
			correction.CorrectedField = tt.field
			correction.EvidenceRef = "correction-evidence-456"

			err := journal.Append(correction)
			if tt.wantOK && err != nil {
				t.Fatalf("expected valid correction, got %v", err)
			}
			if !tt.wantOK && err == nil {
				t.Fatal("accepted correction with incomplete provenance")
			}
		})
	}
}
func TestAuthorizationJournal_RejectsRecordedTimeRegression(t *testing.T) {
	var journal AuthorizationJournal

	start := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)

	first := journalEvent(
		"event-1",
		EventSubmit,
		start,
		start.Add(2*time.Hour),
	)
	if err := journal.Append(first); err != nil {
		t.Fatalf("append first event: %v", err)
	}

	// A different authorization has its own valid initial transition.
	// Lifecycle validation cannot explain rejection of this event.
	late := journalEvent(
		"event-2",
		EventSubmit,
		start,
		start.Add(time.Hour),
	)
	late.AuthorizationID = "authorization-456"

	if err := journal.Append(late); err == nil {
		t.Fatal("accepted event with regressed recorded timestamp")
	}

	events := journal.Events()
	if len(events) != 1 || events[0].EventID != "event-1" {
		t.Fatalf("rejected append changed journal: %+v", events)
	}

	state, err := journal.Reconstruct(
		"authorization-123",
		start.Add(3*time.Hour),
	)
	if err != nil {
		t.Fatalf("reconstruct after rejected append: %v", err)
	}
	if state != AuthorizationPending {
		t.Fatalf("state = %q, want %q", state, AuthorizationPending)
	}
}
func TestAuthorizationJournal_CorrectionPreservesRecordedTimeHistory(t *testing.T) {
	var journal AuthorizationJournal

	start := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)

	original := journalEvent(
		"event-1",
		EventSubmit,
		start,
		start,
	)

	if err := journal.Append(original); err != nil {
		t.Fatalf("append original: %v", err)
	}

	beforeCorrection := start.Add(12 * time.Hour)

	stateBefore, err := journal.Reconstruct(
		"authorization-123",
		beforeCorrection,
	)
	if err != nil {
		t.Fatalf("reconstruct before correction: %v", err)
	}

	correction := journalEvent(
		"event-2",
		EventSubmit,
		start.Add(time.Hour),
		start.Add(24*time.Hour),
	)
	correction.CorrectsEventID = "event-1"
	correction.CorrectionReason = "Original effective date was incorrect"
	correction.CorrectedField = "EffectiveAt"
	correction.EvidenceRef = "corrected-evidence-456"

	if err := journal.Append(correction); err != nil {
		t.Fatalf("append correction: %v", err)
	}

	stateAfter, err := journal.Reconstruct(
		"authorization-123",
		beforeCorrection,
	)
	if err != nil {
		t.Fatalf("reconstruct historical cutoff: %v", err)
	}

	if stateBefore != AuthorizationPending {
		t.Fatalf("original state = %q, want %q",
			stateBefore, AuthorizationPending)
	}

	if stateAfter != stateBefore {
		t.Fatalf(
			"correction rewrote recorded-time history: before=%q after=%q",
			stateBefore,
			stateAfter,
		)
	}
}
func TestAuthorizationJournal_CorrectionChangesEffectiveViewOnlyAfterRecording(t *testing.T) {
	var journal AuthorizationJournal

	start := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	correctedStart := start.Add(time.Hour)
	correctionRecorded := start.Add(24 * time.Hour)

	original := journalEvent("event-1", EventSubmit, start, start)
	if err := journal.Append(original); err != nil {
		t.Fatalf("append original: %v", err)
	}

	correction := journalEvent(
		"event-2",
		EventSubmit,
		correctedStart,
		correctionRecorded,
	)
	correction.CorrectsEventID = "event-1"
	correction.CorrectionReason = "Original effective date was incorrect"
	correction.CorrectedField = "EffectiveAt"
	correction.EvidenceRef = "corrected-evidence-456"

	if err := journal.Append(correction); err != nil {
		t.Fatalf("append correction: %v", err)
	}

	before, err := journal.EffectiveEvents(
		"authorization-123",
		correctionRecorded.Add(-time.Microsecond),
	)
	if err != nil {
		t.Fatalf("effective events before correction: %v", err)
	}
	if len(before) != 1 || !before[0].EffectiveAt.Equal(start) {
		t.Fatalf("incorrect pre-correction view: %+v", before)
	}

	after, err := journal.EffectiveEvents(
		"authorization-123",
		correctionRecorded,
	)
	if err != nil {
		t.Fatalf("effective events after correction: %v", err)
	}
	if len(after) != 1 || !after[0].EffectiveAt.Equal(correctedStart) {
		t.Fatalf("correction not applied to effective view: %+v", after)
	}

	// The stored original must remain unchanged.
	stored := journal.Events()
	if len(stored) != 2 ||
		!stored[0].EffectiveAt.Equal(start) ||
		!stored[1].EffectiveAt.Equal(correctedStart) {
		t.Fatalf("journal history was altered: %+v", stored)
	}
}
func TestAuthorizationJournal_RejectsContradictoryEffectiveHistory(t *testing.T) {
	var journal AuthorizationJournal

	start := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)

	events := []AuthorizationJournalEvent{
		journalEvent("event-1", EventSubmit, start, start),
		journalEvent(
			"event-2",
			EventApprove,
			start.Add(time.Hour),
			start.Add(time.Hour),
		),
		journalEvent(
			"event-3",
			EventActivate,
			start.Add(2*time.Hour),
			start.Add(2*time.Hour),
		),
	}

	for _, event := range events {
		if err := journal.Append(event); err != nil {
			t.Fatalf("append %s: %v", event.EventID, err)
		}
	}

	// Move ACTIVATE to before APPROVE, without modifying the
	// original activation event.
	correction := journalEvent(
		"event-4",
		EventActivate,
		start.Add(30*time.Minute),
		start.Add(24*time.Hour),
	)
	correction.CorrectsEventID = "event-3"
	correction.CorrectionReason = "Correcting activation effective time"
	correction.CorrectedField = "EffectiveAt"
	correction.EvidenceRef = "correction-evidence-789"

	// The journal may preserve the attributable correction, but
	// it must not expose a contradictory effective history.
	if err := journal.Append(correction); err != nil {
		t.Fatalf("append correction: %v", err)
	}

	_, err := journal.EffectiveEvents(
		"authorization-123",
		start.Add(25*time.Hour),
	)
	if err == nil {
		t.Fatal("accepted contradictory effective-time history")
	}

	// The same journal must still reconstruct the earlier
	// recorded-time view, before the correction existed.
	before, err := journal.EffectiveEvents(
		"authorization-123",
		start.Add(3*time.Hour),
	)
	if err != nil {
		t.Fatalf("historical reconstruction: %v", err)
	}
	if len(before) != 3 {
		t.Fatalf("historical event count = %d, want 3", len(before))
	}
}

func TestAuthorizationJournal_RejectsZeroDurationAuthorityInterval(t *testing.T) {
	var journal AuthorizationJournal

	start := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	activation := start.Add(2 * time.Hour)

	events := []AuthorizationJournalEvent{
		journalEvent("event-1", EventSubmit, start, start),
		journalEvent(
			"event-2",
			EventApprove,
			start.Add(time.Hour),
			start.Add(time.Hour),
		),
		journalEvent(
			"event-3",
			EventActivate,
			activation,
			activation,
		),
		journalEvent(
			"event-4",
			EventSuspend,
			activation,
			activation,
		),
	}

	for _, event := range events {
		if err := journal.Append(event); err != nil {
			t.Fatalf("append %s: %v", event.EventID, err)
		}
	}

	// Architecture §5: ACTIVE authority cannot be represented by
	// a zero-duration interval. A coherent resolver history must
	// reject contradictory authority boundaries, not silently
	// discard or manufacture an interval.
	_, err := journal.EffectiveEvents(
		"authorization-123",
		activation,
	)
	if err == nil {
		t.Fatal("accepted zero-duration ACTIVE authority interval")
	}
}

func TestAuthorizationJournal_AcceptsOneMicrosecondAuthorityInterval(t *testing.T) {
	var journal AuthorizationJournal

	start := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	activation := start.Add(2 * time.Hour)
	suspension := activation.Add(time.Microsecond)

	events := []AuthorizationJournalEvent{
		journalEvent("event-1", EventSubmit, start, start),
		journalEvent(
			"event-2",
			EventApprove,
			start.Add(time.Hour),
			start.Add(time.Hour),
		),
		journalEvent(
			"event-3",
			EventActivate,
			activation,
			activation,
		),
		journalEvent(
			"event-4",
			EventSuspend,
			suspension,
			suspension,
		),
	}

	for _, event := range events {
		if err := journal.Append(event); err != nil {
			t.Fatalf("append %s: %v", event.EventID, err)
		}
	}

	effective, err := journal.EffectiveEvents(
		"authorization-123",
		suspension,
	)
	if err != nil {
		t.Fatalf("rejected valid one-microsecond authority interval: %v", err)
	}

	if len(effective) != 4 {
		t.Fatalf("effective event count = %d, want 4", len(effective))
	}

	from, until, err := projectGrantInterval(
		effective[2].EffectiveAt,
		&effective[3].EffectiveAt,
	)
	if err != nil {
		t.Fatalf("failed to project one-microsecond interval: %v", err)
	}

	if !from.Equal(activation) {
		t.Fatalf("interval start = %s, want %s", from, activation)
	}

	if until == nil || !until.Equal(activation) {
		t.Fatalf("inclusive interval end = %v, want %s", until, activation)
	}
}

func TestAuthorizationJournal_CorrectionCannotCollapseAuthorityInterval(t *testing.T) {
	var journal AuthorizationJournal

	start := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	activation := start.Add(2 * time.Hour)
	suspension := start.Add(3 * time.Hour)
	correctionRecordedAt := start.Add(24 * time.Hour)

	events := []AuthorizationJournalEvent{
		journalEvent("event-1", EventSubmit, start, start),
		journalEvent(
			"event-2",
			EventApprove,
			start.Add(time.Hour),
			start.Add(time.Hour),
		),
		journalEvent(
			"event-3",
			EventActivate,
			activation,
			activation,
		),
		journalEvent(
			"event-4",
			EventSuspend,
			suspension,
			suspension,
		),
	}

	for _, event := range events {
		if err := journal.Append(event); err != nil {
			t.Fatalf("append %s: %v", event.EventID, err)
		}
	}

	// A later correction moves SUSPEND to the exact instant
	// of ACTIVATE, collapsing the ACTIVE interval to zero.
	correction := journalEvent(
		"event-5",
		EventSuspend,
		activation,
		correctionRecordedAt,
	)
	correction.CorrectsEventID = "event-4"
	correction.CorrectionReason = "Corrected suspension effective timestamp"
	correction.CorrectedField = "EffectiveAt"
	correction.EvidenceRef = "correction-evidence-789"

	if err := journal.Append(correction); err != nil {
		t.Fatalf("append attributable correction: %v", err)
	}

	// As known before the correction, the original ACTIVE
	// interval remains coherent and historically reconstructable.
	before, err := journal.EffectiveEvents(
		"authorization-123",
		suspension,
	)
	if err != nil {
		t.Fatalf("historical view before correction: %v", err)
	}

	if len(before) != 4 {
		t.Fatalf("historical event count = %d, want 4", len(before))
	}

	if !before[3].EffectiveAt.Equal(suspension) {
		t.Fatalf(
			"historical suspension changed: got %s, want %s",
			before[3].EffectiveAt,
			suspension,
		)
	}

	// Once the correction is recorded, the derived history
	// must fail closed rather than expose a zero-duration grant.
	_, err = journal.EffectiveEvents(
		"authorization-123",
		correctionRecordedAt,
	)
	if err == nil {
		t.Fatal("accepted corrected history with zero-duration ACTIVE interval")
	}

	// The append-only source journal must preserve both the
	// original event and the attributable correction.
	stored := journal.Events()
	if len(stored) != 5 {
		t.Fatalf("stored event count = %d, want 5", len(stored))
	}

	if !stored[3].EffectiveAt.Equal(suspension) {
		t.Fatal("original suspension event was modified")
	}

	if stored[4].CorrectsEventID != "event-4" ||
		!stored[4].EffectiveAt.Equal(activation) {
		t.Fatal("correction provenance was not preserved")
	}
}
func TestAuthorizationJournal_RejectsCrossAuthorizationCorrection(t *testing.T) {
	var journal AuthorizationJournal

	start := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)

	original := journalEvent("event-1", EventSubmit, start, start)

	if err := journal.Append(original); err != nil {
		t.Fatalf("append original: %v", err)
	}

	correction := journalEvent(
		"event-2",
		EventSubmit,
		start.Add(time.Hour),
		start.Add(24*time.Hour),
	)

	correction.AuthorizationID = "authorization-456"
	correction.CorrectsEventID = "event-1"
	correction.CorrectionReason = "Correcting effective date"
	correction.CorrectedField = "EffectiveAt"
	correction.EvidenceRef = "correction-evidence-456"

	if err := journal.Append(correction); err == nil {
		t.Fatal("accepted correction targeting another authorization")
	}

	events := journal.Events()
	if len(events) != 1 || events[0].EventID != "event-1" {
		t.Fatalf("rejected correction changed journal: %+v", events)
	}
}
func TestAuthorizationJournal_ActiveWithoutAdjudicationCannotProduceGrant(t *testing.T) {
	var journal AuthorizationJournal

	start := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)

	events := []AuthorizationJournalEvent{
		journalEvent(
			"event-1",
			EventSubmit,
			start,
			start,
		),
		journalEvent(
			"event-2",
			EventApprove,
			start.Add(time.Hour),
			start.Add(time.Hour),
		),
		journalEvent(
			"event-3",
			EventActivate,
			start.Add(2*time.Hour),
			start.Add(2*time.Hour),
		),
	}

	for _, event := range events {
		if err := journal.Append(event); err != nil {
			t.Fatalf("append %s: %v", event.EventID, err)
		}
	}

	record := AuthorizationRecord{
		AuthorizationID:    "authorization-123",
		IssuerID:           "issuer-456",
		AuthoritySubjectID: "employer-789",
		ClaimType:          "employment.role",

		// Deliberately no affirmative adjudication.
	}

	grants, err := ProjectAuthorityGrants(
		record,
		&journal,
		start.Add(3*time.Hour),
	)

	// Missing authoritative adjudication must fail closed.
	// An empty grant list without an error would conceal an
	// incomplete authority record from the resolver.
	if err == nil {
		t.Fatalf(
			"expected authority projection failure without adjudication; grants=%+v",
			grants,
		)
	}

	if len(grants) != 0 {
		t.Fatalf(
			"failed projection returned grants: %+v",
			grants,
		)
	}
}
func TestAuthorizationJournal_AffirmativeAdjudicationProducesActiveGrant(t *testing.T) {
	var journal AuthorizationJournal

	start := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)

	events := []AuthorizationJournalEvent{
		journalEvent("event-1", EventSubmit, start, start),
		journalEvent(
			"event-2",
			EventApprove,
			start.Add(time.Hour),
			start.Add(time.Hour),
		),
		journalEvent(
			"event-3",
			EventActivate,
			start.Add(2*time.Hour),
			start.Add(2*time.Hour),
		),
	}

	for _, event := range events {
		if event.EventType == EventApprove {
			event.EvidenceRef = "authority-evidence-123"
		}

		if err := journal.Append(event); err != nil {
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

	grants, err := ProjectAuthorityGrants(
		record,
		&journal,
		start.Add(3*time.Hour),
	)
	if err != nil {
		t.Fatalf("project valid authorization: %v", err)
	}

	if len(grants) != 1 {
		t.Fatalf("grant count = %d, want 1", len(grants))
	}

	grant := grants[0]

	if grant.IssuerID != record.IssuerID ||
		grant.AuthoritySubjectID != record.AuthoritySubjectID ||
		grant.ClaimType != record.ClaimType {
		t.Fatalf("projected grant scope mismatch: %+v", grant)
	}

	if grant.State != AuthorityGrantActive {
		t.Fatalf("grant state = %q, want active", grant.State)
	}

	if !grant.ValidFrom.Equal(start.Add(2 * time.Hour)) {
		t.Fatalf(
			"grant valid from = %s, want %s",
			grant.ValidFrom,
			start.Add(2*time.Hour),
		)
	}

	if grant.ValidUntil != nil {
		t.Fatalf("active grant unexpectedly bounded: %+v", grant)
	}
}
func TestAuthorizationJournal_RejectsAdjudicationRecordedAfterApproval(t *testing.T) {
	var journal AuthorizationJournal

	start := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)

	submit := journalEvent(
		"event-1",
		EventSubmit,
		start,
		start,
	)

	approve := journalEvent(
		"event-2",
		EventApprove,
		start.Add(time.Hour),
		start.Add(time.Hour),
	)
	approve.EvidenceRef = "authority-evidence-123"

	activate := journalEvent(
		"event-3",
		EventActivate,
		start.Add(2*time.Hour),
		start.Add(2*time.Hour),
	)

	for _, event := range []AuthorizationJournalEvent{
		submit,
		approve,
		activate,
	} {
		if err := journal.Append(event); err != nil {
			t.Fatalf("append %s: %v", event.EventID, err)
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

			// Decision was recorded AFTER the approval event.
			DecidedAt:  start.Add(3 * time.Hour),
			RecordedAt: start.Add(3 * time.Hour),
		},
	}

	grants, err := ProjectAuthorityGrants(
		record,
		&journal,
		start.Add(4*time.Hour),
	)

	if err == nil {
		t.Fatalf(
			"accepted approval backed by later-recorded adjudication: %+v",
			grants,
		)
	}

	if len(grants) != 0 {
		t.Fatalf("failed projection returned grants: %+v", grants)
	}
}
func TestAuthorizationJournal_RejectsAdjudicationDecisionAfterRecording(t *testing.T) {
	var journal AuthorizationJournal

	start := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)

	submit := journalEvent(
		"event-1",
		EventSubmit,
		start,
		start,
	)

	approve := journalEvent(
		"event-2",
		EventApprove,
		start.Add(2*time.Hour),
		start.Add(2*time.Hour),
	)
	approve.EvidenceRef = "authority-evidence-123"

	activate := journalEvent(
		"event-3",
		EventActivate,
		start.Add(3*time.Hour),
		start.Add(3*time.Hour),
	)

	for _, event := range []AuthorizationJournalEvent{
		submit,
		approve,
		activate,
	} {
		if err := journal.Append(event); err != nil {
			t.Fatalf("append %s: %v", event.EventID, err)
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

			// Impossible chronology: recorded before decided.
			DecidedAt:  start.Add(90 * time.Minute),
			RecordedAt: start.Add(time.Hour),
		},
	}

	grants, err := ProjectAuthorityGrants(
		record,
		&journal,
		start.Add(4*time.Hour),
	)

	if err == nil {
		t.Fatalf(
			"accepted adjudication decided after recording: %+v",
			grants,
		)
	}

	if len(grants) != 0 {
		t.Fatalf("failed projection returned grants: %+v", grants)
	}
}
func TestAuthorizationJournal_SuspensionPreservesHistoricalGrant(t *testing.T) {
	var journal AuthorizationJournal

	start := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	activation := start.Add(2 * time.Hour)
	suspension := start.Add(4 * time.Hour)

	submit := journalEvent("event-1", EventSubmit, start, start)

	approve := journalEvent(
		"event-2",
		EventApprove,
		start.Add(time.Hour),
		start.Add(time.Hour),
	)
	approve.EvidenceRef = "authority-evidence-123"

	activate := journalEvent(
		"event-3",
		EventActivate,
		activation,
		activation,
	)

	suspend := journalEvent(
		"event-4",
		EventSuspend,
		suspension,
		suspension,
	)

	for _, event := range []AuthorizationJournalEvent{
		submit, approve, activate, suspend,
	} {
		if err := journal.Append(event); err != nil {
			t.Fatalf("append %s: %v", event.EventID, err)
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

	grants, err := ProjectAuthorityGrants(
		record,
		&journal,
		start.Add(5*time.Hour),
	)
	if err != nil {
		t.Fatalf("project suspended authorization: %v", err)
	}

	if len(grants) != 2 {
		t.Fatalf("grant count = %d, want 2; grants=%+v", len(grants), grants)
	}

	active := grants[0]
	inactive := grants[1]

	// Architecture §5: suspension cannot retroactively erase ACTIVE authority.
	if active.State != AuthorityGrantActive {
		t.Fatalf("historical state = %q, want active", active.State)
	}
	if active.GrantID != "authorization-123:event-3" {
		t.Fatalf("active GrantID = %q", active.GrantID)
	}
	if !active.ValidFrom.Equal(activation) {
		t.Fatalf("active start = %s, want %s", active.ValidFrom, activation)
	}

	wantActiveEnd := suspension.Add(-time.Microsecond)
	if active.ValidUntil == nil || !active.ValidUntil.Equal(wantActiveEnd) {
		t.Fatalf("active end = %v, want %s", active.ValidUntil, wantActiveEnd)
	}

	// Suspension creates a separate, explicitly INACTIVE interval.
	if inactive.State != AuthorityGrantInactive {
		t.Fatalf("suspended state = %q, want inactive", inactive.State)
	}
	if inactive.GrantID != "authorization-123:event-4" {
		t.Fatalf("inactive GrantID = %q", inactive.GrantID)
	}
	if !inactive.ValidFrom.Equal(suspension) {
		t.Fatalf("inactive start = %s, want %s", inactive.ValidFrom, suspension)
	}
	if inactive.ValidUntil != nil {
		t.Fatalf("suspended interval unexpectedly bounded: %+v", inactive)
	}

	// Inclusive microsecond boundaries must have no gap or overlap.
	if !active.ValidUntil.Add(time.Microsecond).Equal(inactive.ValidFrom) {
		t.Fatal("gap or overlap at ACTIVE to SUSPENDED boundary")
	}
}
func TestAuthorizationJournal_ReinstatementPreservesSeparateGrantIntervals(t *testing.T) {
	var journal AuthorizationJournal

	start := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	activation := start.Add(2 * time.Hour)
	suspension := start.Add(4 * time.Hour)
	reinstatement := start.Add(6 * time.Hour)

	submit := journalEvent("event-1", EventSubmit, start, start)

	approve := journalEvent(
		"event-2",
		EventApprove,
		start.Add(time.Hour),
		start.Add(time.Hour),
	)
	approve.EvidenceRef = "authority-evidence-123"

	activate := journalEvent(
		"event-3",
		EventActivate,
		activation,
		activation,
	)

	suspend := journalEvent(
		"event-4",
		EventSuspend,
		suspension,
		suspension,
	)

	reinstate := journalEvent(
		"event-5",
		EventReinstate,
		reinstatement,
		reinstatement,
	)

	for _, event := range []AuthorizationJournalEvent{
		submit, approve, activate, suspend, reinstate,
	} {
		if err := journal.Append(event); err != nil {
			t.Fatalf("append %s: %v", event.EventID, err)
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
			AuthorityBasis:     "verified organizational delegation",
			ClaimType:          "employment.role",
			Outcome:            AdjudicationApproved,
			DecisionMakerID:    "cvera-adjudicator-1",
			EvidenceRef:        "authority-evidence-123",
			DecidedAt:          start.Add(time.Hour),
			RecordedAt:         start.Add(time.Hour),
		},
	}

	grants, err := ProjectAuthorityGrants(
		record,
		&journal,
		start.Add(7*time.Hour),
	)
	if err != nil {
		t.Fatalf("project reinstated authorization: %v", err)
	}

	if len(grants) != 3 {
		t.Fatalf("grant count = %d, want 3; grants=%+v", len(grants), grants)
	}

	wantStates := []AuthorityGrantState{
		AuthorityGrantActive,
		AuthorityGrantInactive,
		AuthorityGrantActive,
	}
	wantIDs := []string{
		"authorization-123:event-3",
		"authorization-123:event-4",
		"authorization-123:event-5",
	}
	wantStarts := []time.Time{
		activation,
		suspension,
		reinstatement,
	}

	for i, grant := range grants {
		if grant.State != wantStates[i] {
			t.Fatalf("grant %d state = %q, want %q", i, grant.State, wantStates[i])
		}
		if grant.GrantID != wantIDs[i] {
			t.Fatalf("grant %d ID = %q, want %q", i, grant.GrantID, wantIDs[i])
		}
		if !grant.ValidFrom.Equal(wantStarts[i]) {
			t.Fatalf("grant %d start = %s, want %s", i, grant.ValidFrom, wantStarts[i])
		}
		if grant.IssuerID != record.IssuerID ||
			grant.AuthoritySubjectID != record.AuthoritySubjectID ||
			grant.ClaimType != record.ClaimType {
			t.Fatalf("grant %d scope mismatch: %+v", i, grant)
		}
	}

	// Architecture §5: each lifecycle interval retains its historical state.
	wantFirstEnd := suspension.Add(-time.Microsecond)
	if grants[0].ValidUntil == nil ||
		!grants[0].ValidUntil.Equal(wantFirstEnd) {
		t.Fatalf("first interval end = %v, want %s", grants[0].ValidUntil, wantFirstEnd)
	}

	wantSecondEnd := reinstatement.Add(-time.Microsecond)
	if grants[1].ValidUntil == nil ||
		!grants[1].ValidUntil.Equal(wantSecondEnd) {
		t.Fatalf("suspension interval end = %v, want %s", grants[1].ValidUntil, wantSecondEnd)
	}

	if grants[2].ValidUntil != nil {
		t.Fatalf("reinstated interval unexpectedly bounded: %+v", grants[2])
	}

	for i := range 2 {
		if !grants[i].ValidUntil.Add(time.Microsecond).Equal(grants[i+1].ValidFrom) {
			t.Fatalf("gap or overlap between intervals %d and %d", i, i+1)
		}
	}
}
func TestAuthorizationJournal_RevocationPreservesHistoricalGrant(t *testing.T) {
	var journal AuthorizationJournal

	start := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	activation := start.Add(2 * time.Hour)
	revocation := start.Add(4 * time.Hour)

	submit := journalEvent("event-1", EventSubmit, start, start)

	approve := journalEvent(
		"event-2",
		EventApprove,
		start.Add(time.Hour),
		start.Add(time.Hour),
	)
	approve.EvidenceRef = "authority-evidence-123"

	activate := journalEvent(
		"event-3",
		EventActivate,
		activation,
		activation,
	)

	revoke := journalEvent(
		"event-4",
		EventRevoke,
		revocation,
		revocation,
	)

	for _, event := range []AuthorizationJournalEvent{
		submit, approve, activate, revoke,
	} {
		if err := journal.Append(event); err != nil {
			t.Fatalf("append %s: %v", event.EventID, err)
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

	grants, err := ProjectAuthorityGrants(
		record,
		&journal,
		start.Add(5*time.Hour),
	)
	if err != nil {
		t.Fatalf("project revoked authorization: %v", err)
	}

	if len(grants) != 2 {
		t.Fatalf("grant count = %d, want 2; grants=%+v", len(grants), grants)
	}

	active := grants[0]
	inactive := grants[1]

	// Architecture §5: revocation does not retroactively erase authority.
	if active.State != AuthorityGrantActive {
		t.Fatalf("historical state = %q, want active", active.State)
	}
	if active.GrantID != "authorization-123:event-3" {
		t.Fatalf("active GrantID = %q", active.GrantID)
	}
	if !active.ValidFrom.Equal(activation) {
		t.Fatalf("active start = %s, want %s", active.ValidFrom, activation)
	}

	wantActiveEnd := revocation.Add(-time.Microsecond)
	if active.ValidUntil == nil || !active.ValidUntil.Equal(wantActiveEnd) {
		t.Fatalf("active end = %v, want %s", active.ValidUntil, wantActiveEnd)
	}

	// Revocation establishes negative authority from withdrawal onward.
	if inactive.State != AuthorityGrantInactive {
		t.Fatalf("revoked state = %q, want inactive", inactive.State)
	}
	if inactive.GrantID != "authorization-123:event-4" {
		t.Fatalf("inactive GrantID = %q", inactive.GrantID)
	}
	if !inactive.ValidFrom.Equal(revocation) {
		t.Fatalf("inactive start = %s, want %s", inactive.ValidFrom, revocation)
	}
	if inactive.ValidUntil != nil {
		t.Fatalf("revocation interval unexpectedly bounded: %+v", inactive)
	}

	for i, grant := range grants {
		if grant.IssuerID != record.IssuerID ||
			grant.AuthoritySubjectID != record.AuthoritySubjectID ||
			grant.ClaimType != record.ClaimType {
			t.Fatalf("grant %d scope mismatch: %+v", i, grant)
		}
	}

	if !active.ValidUntil.Add(time.Microsecond).Equal(inactive.ValidFrom) {
		t.Fatal("gap or overlap at ACTIVE to REVOKED boundary")
	}

	// Architecture §5: withdrawal changes authority prospectively.
	// The frozen evaluator must preserve decisions made during the
	// earlier ACTIVE interval, including its final microsecond.
	evidence := credential.NormalizedCredentialEvidence{
		ClaimType: credential.ClaimTypeEmploymentRole,
		Claim: credential.EmploymentClaim{
			ClaimType: credential.ClaimTypeEmploymentRole,
			Employment: credential.EmploymentRelationship{
				EmployerID: record.AuthoritySubjectID,
			},
		},
		Issuer: credential.IssuerEvidence{
			Identifier: record.IssuerID,
		},
	}

	tests := []struct {
		name string
		at   time.Time
		want AuthorityDecision
	}{
		{
			name: "historical active authority",
			at:   activation.Add(time.Hour),
			want: AuthorityAuthorized,
		},
		{
			name: "last microsecond before revocation",
			at:   revocation.Add(-time.Microsecond),
			want: AuthorityAuthorized,
		},
		{
			name: "exact revocation boundary",
			at:   revocation,
			want: AuthorityUnauthorized,
		},
		{
			name: "after revocation",
			at:   revocation.Add(time.Hour),
			want: AuthorityUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := (Evaluator{}).Evaluate(evidence, grants, tt.at)
			if result.Decision != tt.want {
				t.Fatalf("authority decision at %s = %+v, want %s", tt.at, result, tt.want)
			}
		})
	}
}
func TestAuthorizationJournal_ExpirationPreservesHistoricalGrant(t *testing.T) {
	var journal AuthorizationJournal

	start := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	activation := start.Add(2 * time.Hour)
	expiration := start.Add(4 * time.Hour)

	submit := journalEvent("event-1", EventSubmit, start, start)

	approve := journalEvent(
		"event-2",
		EventApprove,
		start.Add(time.Hour),
		start.Add(time.Hour),
	)
	approve.EvidenceRef = "authority-evidence-123"

	activate := journalEvent(
		"event-3",
		EventActivate,
		activation,
		activation,
	)

	expire := journalEvent(
		"event-4",
		EventExpire,
		expiration,
		expiration,
	)

	for _, event := range []AuthorizationJournalEvent{
		submit, approve, activate, expire,
	} {
		if err := journal.Append(event); err != nil {
			t.Fatalf("append %s: %v", event.EventID, err)
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

	grants, err := ProjectAuthorityGrants(
		record,
		&journal,
		start.Add(5*time.Hour),
	)
	if err != nil {
		t.Fatalf("project expired authorization: %v", err)
	}

	if len(grants) != 1 {
		t.Fatalf("grant count = %d, want 1; grants=%+v", len(grants), grants)
	}

	grant := grants[0]

	// Architecture §5: expiration preserves historical ACTIVE authority.
	// It does not manufacture an INACTIVE interval after expiration.
	if grant.State != AuthorityGrantActive {
		t.Fatalf("historical state = %q, want active", grant.State)
	}

	if grant.GrantID != "authorization-123:event-3" {
		t.Fatalf("grant ID = %q, want authorization-123:event-3", grant.GrantID)
	}

	if !grant.ValidFrom.Equal(activation) {
		t.Fatalf("grant start = %s, want %s", grant.ValidFrom, activation)
	}

	wantEnd := expiration.Add(-time.Microsecond)
	if grant.ValidUntil == nil || !grant.ValidUntil.Equal(wantEnd) {
		t.Fatalf("grant end = %v, want %s", grant.ValidUntil, wantEnd)
	}

	if grant.IssuerID != record.IssuerID ||
		grant.AuthoritySubjectID != record.AuthoritySubjectID ||
		grant.ClaimType != record.ClaimType {
		t.Fatalf("grant scope mismatch: %+v", grant)
	}

	// Architecture §5: expiration preserves prior authority but does
	// not manufacture explicit negative authority after expiration.
	evidence := credential.NormalizedCredentialEvidence{
		ClaimType: credential.ClaimTypeEmploymentRole,
		Claim: credential.EmploymentClaim{
			ClaimType: credential.ClaimTypeEmploymentRole,
			Employment: credential.EmploymentRelationship{
				EmployerID: record.AuthoritySubjectID,
			},
		},
		Issuer: credential.IssuerEvidence{
			Identifier: record.IssuerID,
		},
	}

	tests := []struct {
		name string
		at   time.Time
		want AuthorityDecision
	}{
		{
			name: "historical active authority",
			at:   activation.Add(time.Hour),
			want: AuthorityAuthorized,
		},
		{
			name: "last microsecond before expiration",
			at:   expiration.Add(-time.Microsecond),
			want: AuthorityAuthorized,
		},
		{
			name: "exact expiration boundary",
			at:   expiration,
			want: AuthorityReview,
		},
		{
			name: "after expiration",
			at:   expiration.Add(time.Hour),
			want: AuthorityReview,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := (Evaluator{}).Evaluate(evidence, grants, tt.at)

			if result.Decision != tt.want {
				t.Fatalf(
					"authority decision at %s = %+v, want %s",
					tt.at,
					result,
					tt.want,
				)
			}
		})
	}
}
func TestAuthorizationJournal_RevocationFromSuspendedPreservesHistory(t *testing.T) {
	var journal AuthorizationJournal

	start := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	activation := start.Add(2 * time.Hour)
	suspension := start.Add(4 * time.Hour)
	revocation := start.Add(6 * time.Hour)

	submit := journalEvent("event-1", EventSubmit, start, start)

	approve := journalEvent(
		"event-2",
		EventApprove,
		start.Add(time.Hour),
		start.Add(time.Hour),
	)
	approve.EvidenceRef = "authority-evidence-123"

	activate := journalEvent(
		"event-3",
		EventActivate,
		activation,
		activation,
	)

	suspend := journalEvent(
		"event-4",
		EventSuspend,
		suspension,
		suspension,
	)

	revoke := journalEvent(
		"event-5",
		EventRevoke,
		revocation,
		revocation,
	)

	for _, event := range []AuthorizationJournalEvent{
		submit, approve, activate, suspend, revoke,
	} {
		if err := journal.Append(event); err != nil {
			t.Fatalf("append %s: %v", event.EventID, err)
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

	grants, err := ProjectAuthorityGrants(
		record,
		&journal,
		start.Add(7*time.Hour),
	)
	if err != nil {
		t.Fatalf("project revoked suspended authorization: %v", err)
	}

	if len(grants) != 3 {
		t.Fatalf("grant count = %d, want 3; grants=%+v", len(grants), grants)
	}

	wantStates := []AuthorityGrantState{
		AuthorityGrantActive,
		AuthorityGrantInactive,
		AuthorityGrantInactive,
	}
	wantIDs := []string{
		"authorization-123:event-3",
		"authorization-123:event-4",
		"authorization-123:event-5",
	}
	wantStarts := []time.Time{
		activation,
		suspension,
		revocation,
	}

	for i, grant := range grants {
		if grant.State != wantStates[i] {
			t.Fatalf("grant %d state = %q, want %q", i, grant.State, wantStates[i])
		}
		if grant.GrantID != wantIDs[i] {
			t.Fatalf("grant %d ID = %q, want %q", i, grant.GrantID, wantIDs[i])
		}
		if !grant.ValidFrom.Equal(wantStarts[i]) {
			t.Fatalf("grant %d start = %s, want %s", i, grant.ValidFrom, wantStarts[i])
		}
		if grant.IssuerID != record.IssuerID ||
			grant.AuthoritySubjectID != record.AuthoritySubjectID ||
			grant.ClaimType != record.ClaimType {
			t.Fatalf("grant %d scope mismatch: %+v", i, grant)
		}
	}

	// Architecture §5: neither suspension nor revocation may
	// retroactively rewrite the earlier ACTIVE interval.
	wantActiveEnd := suspension.Add(-time.Microsecond)
	if grants[0].ValidUntil == nil ||
		!grants[0].ValidUntil.Equal(wantActiveEnd) {
		t.Fatalf("active interval end = %v, want %s", grants[0].ValidUntil, wantActiveEnd)
	}

	// The suspension interval ends when revocation takes effect.
	wantSuspensionEnd := revocation.Add(-time.Microsecond)
	if grants[1].ValidUntil == nil ||
		!grants[1].ValidUntil.Equal(wantSuspensionEnd) {
		t.Fatalf("suspension interval end = %v, want %s", grants[1].ValidUntil, wantSuspensionEnd)
	}

	// Revocation establishes an unbounded negative-authority interval.
	if grants[2].ValidUntil != nil {
		t.Fatalf("revocation interval unexpectedly bounded: %+v", grants[2])
	}

	for i := range 2 {
		if !grants[i].ValidUntil.Add(time.Microsecond).Equal(grants[i+1].ValidFrom) {
			t.Fatalf("gap or overlap between intervals %d and %d", i, i+1)
		}
	}
}
func TestAuthorizationJournal_ExpirationFromSuspendedPreservesHistory(t *testing.T) {
	var journal AuthorizationJournal

	start := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	activation := start.Add(2 * time.Hour)
	suspension := start.Add(4 * time.Hour)
	expiration := start.Add(6 * time.Hour)

	submit := journalEvent("event-1", EventSubmit, start, start)

	approve := journalEvent(
		"event-2",
		EventApprove,
		start.Add(time.Hour),
		start.Add(time.Hour),
	)
	approve.EvidenceRef = "authority-evidence-123"

	activate := journalEvent(
		"event-3",
		EventActivate,
		activation,
		activation,
	)

	suspend := journalEvent(
		"event-4",
		EventSuspend,
		suspension,
		suspension,
	)

	expire := journalEvent(
		"event-5",
		EventExpire,
		expiration,
		expiration,
	)

	for _, event := range []AuthorizationJournalEvent{
		submit, approve, activate, suspend, expire,
	} {
		if err := journal.Append(event); err != nil {
			t.Fatalf("append %s: %v", event.EventID, err)
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

	grants, err := ProjectAuthorityGrants(
		record,
		&journal,
		start.Add(7*time.Hour),
	)
	if err != nil {
		t.Fatalf("project expired suspended authorization: %v", err)
	}

	if len(grants) != 2 {
		t.Fatalf("grant count = %d, want 2; grants=%+v", len(grants), grants)
	}

	active := grants[0]
	inactive := grants[1]

	// Architecture §5: expiration preserves the historical
	// ACTIVE and SUSPENDED intervals without manufacturing
	// a new INACTIVE interval.
	if active.State != AuthorityGrantActive {
		t.Fatalf("historical state = %q, want active", active.State)
	}
	if active.GrantID != "authorization-123:event-3" {
		t.Fatalf("active GrantID = %q", active.GrantID)
	}
	if !active.ValidFrom.Equal(activation) {
		t.Fatalf("active start = %s, want %s", active.ValidFrom, activation)
	}

	wantActiveEnd := suspension.Add(-time.Microsecond)
	if active.ValidUntil == nil || !active.ValidUntil.Equal(wantActiveEnd) {
		t.Fatalf("active end = %v, want %s", active.ValidUntil, wantActiveEnd)
	}

	if inactive.State != AuthorityGrantInactive {
		t.Fatalf("suspension state = %q, want inactive", inactive.State)
	}
	if inactive.GrantID != "authorization-123:event-4" {
		t.Fatalf("suspension GrantID = %q", inactive.GrantID)
	}
	if !inactive.ValidFrom.Equal(suspension) {
		t.Fatalf("suspension start = %s, want %s", inactive.ValidFrom, suspension)
	}

	wantSuspensionEnd := expiration.Add(-time.Microsecond)
	if inactive.ValidUntil == nil ||
		!inactive.ValidUntil.Equal(wantSuspensionEnd) {
		t.Fatalf("suspension end = %v, want %s", inactive.ValidUntil, wantSuspensionEnd)
	}

	for i, grant := range grants {
		if grant.IssuerID != record.IssuerID ||
			grant.AuthoritySubjectID != record.AuthoritySubjectID ||
			grant.ClaimType != record.ClaimType {
			t.Fatalf("grant %d scope mismatch: %+v", i, grant)
		}
	}

	if !active.ValidUntil.Add(time.Microsecond).Equal(inactive.ValidFrom) {
		t.Fatal("gap or overlap at ACTIVE to SUSPENDED boundary")
	}
}
func TestAuthorizationJournal_CorrectionUpdatesHistoricalGrantBoundary(t *testing.T) {
	var journal AuthorizationJournal

	start := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	originalActivation := start.Add(2 * time.Hour)
	correctedActivation := start.Add(3 * time.Hour)
	correctionRecorded := start.Add(4 * time.Hour)

	submit := journalEvent("event-1", EventSubmit, start, start)

	approve := journalEvent(
		"event-2",
		EventApprove,
		start.Add(time.Hour),
		start.Add(time.Hour),
	)
	approve.EvidenceRef = "authority-evidence-123"

	activate := journalEvent(
		"event-3",
		EventActivate,
		originalActivation,
		originalActivation,
	)

	for _, event := range []AuthorizationJournalEvent{
		submit, approve, activate,
	} {
		if err := journal.Append(event); err != nil {
			t.Fatalf("append %s: %v", event.EventID, err)
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

	before, err := ProjectAuthorityGrants(
		record,
		&journal,
		start.Add(3*time.Hour),
	)
	if err != nil {
		t.Fatalf("project before correction: %v", err)
	}
	if len(before) != 1 || !before[0].ValidFrom.Equal(originalActivation) {
		t.Fatalf("unexpected pre-correction grants: %+v", before)
	}
	originalGrantID := before[0].GrantID

	if originalGrantID == "" {
		t.Fatal("pre-correction GrantID is empty")
	}
	correction := journalEvent(
		"correction-1",
		EventActivate,
		correctedActivation,
		correctionRecorded,
	)
	correction.CorrectsEventID = "event-3"
	correction.CorrectedField = "EffectiveAt"
	correction.CorrectionReason = "corrected activation effective time"

	if err := journal.Append(correction); err != nil {
		t.Fatalf("append correction: %v", err)
	}

	after, err := ProjectAuthorityGrants(
		record,
		&journal,
		start.Add(5*time.Hour),
	)
	if err != nil {
		t.Fatalf("project after correction: %v", err)
	}
	if len(after) != 1 || !after[0].ValidFrom.Equal(correctedActivation) {
		t.Fatalf("unexpected post-correction grants: %+v", after)
	}
	if after[0].GrantID != originalGrantID {
		t.Fatalf(
			"GrantID changed after effective-time correction: before=%q after=%q",
			originalGrantID,
			after[0].GrantID,
		)
	}
	originalEvents := journal.Events()
	if !originalEvents[2].EffectiveAt.Equal(originalActivation) {
		t.Fatalf("original activation was mutated: %+v", originalEvents[2])
	}
}
func TestAuthorizationJournal_GrantIDsAreDistinctAndStableAcrossReinstatement(t *testing.T) {
	var journal AuthorizationJournal

	start := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)

	submit := journalEvent("event-1", EventSubmit, start, start)

	approve := journalEvent(
		"event-2",
		EventApprove,
		start.Add(time.Hour),
		start.Add(time.Hour),
	)
	approve.EvidenceRef = "authority-evidence-123"

	activate := journalEvent(
		"event-3",
		EventActivate,
		start.Add(2*time.Hour),
		start.Add(2*time.Hour),
	)

	suspend := journalEvent(
		"event-4",
		EventSuspend,
		start.Add(3*time.Hour),
		start.Add(3*time.Hour),
	)

	reinstate := journalEvent(
		"event-5",
		EventReinstate,
		start.Add(4*time.Hour),
		start.Add(4*time.Hour),
	)

	for _, event := range []AuthorizationJournalEvent{
		submit, approve, activate, suspend, reinstate,
	} {
		if err := journal.Append(event); err != nil {
			t.Fatalf("append %s: %v", event.EventID, err)
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

	cutoff := start.Add(5 * time.Hour)

	firstProjection, err := ProjectAuthorityGrants(record, &journal, cutoff)
	if err != nil {
		t.Fatalf("first projection: %v", err)
	}

	secondProjection, err := ProjectAuthorityGrants(record, &journal, cutoff)
	if err != nil {
		t.Fatalf("second projection: %v", err)
	}

	if len(firstProjection) != 3 || len(secondProjection) != 3 {
		t.Fatalf(
			"expected three intervals in both projections: first=%+v second=%+v",
			firstProjection,
			secondProjection,
		)
	}

	// Architecture §5: ACTIVATE, SUSPEND, and REINSTATE each
	// establish a distinct interval with stable event provenance.
	wantIDs := []string{
		"authorization-123:event-3",
		"authorization-123:event-4",
		"authorization-123:event-5",
	}
	wantStates := []AuthorityGrantState{
		AuthorityGrantActive,
		AuthorityGrantInactive,
		AuthorityGrantActive,
	}

	seen := make(map[string]bool)

	for i, grant := range firstProjection {
		if grant.GrantID == "" {
			t.Fatalf("interval %d has empty GrantID", i)
		}

		if grant.GrantID != wantIDs[i] {
			t.Fatalf(
				"interval %d GrantID = %q, want %q",
				i,
				grant.GrantID,
				wantIDs[i],
			)
		}

		if grant.State != wantStates[i] {
			t.Fatalf(
				"interval %d state = %q, want %q",
				i,
				grant.State,
				wantStates[i],
			)
		}

		if seen[grant.GrantID] {
			t.Fatalf("distinct intervals share GrantID %q", grant.GrantID)
		}
		seen[grant.GrantID] = true

		if grant.GrantID != secondProjection[i].GrantID {
			t.Fatalf(
				"interval %d GrantID changed across replay: first=%q second=%q",
				i,
				grant.GrantID,
				secondProjection[i].GrantID,
			)
		}
	}

	if !reflect.DeepEqual(firstProjection, secondProjection) {
		t.Fatalf(
			"grant projections differ across deterministic replay: first=%+v second=%+v",
			firstProjection,
			secondProjection,
		)
	}
}
func TestAuthorizationJournal_FutureEffectiveActivationDoesNotAuthorizeEarly(t *testing.T) {
	var journal AuthorizationJournal

	start := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	activation := start.Add(4 * time.Hour)
	recordedThrough := start.Add(3 * time.Hour)

	submit := journalEvent("event-1", EventSubmit, start, start)

	approve := journalEvent(
		"event-2",
		EventApprove,
		start.Add(time.Hour),
		start.Add(time.Hour),
	)
	approve.EvidenceRef = "authority-evidence-123"

	activate := journalEvent(
		"event-3",
		EventActivate,
		activation,
		start.Add(2*time.Hour),
	)

	for _, event := range []AuthorizationJournalEvent{
		submit, approve, activate,
	} {
		if err := journal.Append(event); err != nil {
			t.Fatalf("append %s: %v", event.EventID, err)
		}
	}

	record := AuthorizationRecord{
		AuthorizationID:    "authorization-123",
		IssuerID:           "https://issuer.example",
		AuthoritySubjectID: "employer-123",
		ClaimType:          credential.ClaimTypeEmploymentRole,
		Adjudication: &AuthorizationAdjudication{
			DecisionID:         "decision-123",
			AuthorizationID:    "authorization-123",
			IssuerID:           "https://issuer.example",
			AuthoritySubjectID: "employer-123",
			ClaimType:          credential.ClaimTypeEmploymentRole,
			Outcome:            AdjudicationApproved,
			DecisionMakerID:    "cvera-adjudicator-1",
			AuthorityBasis:     "verified organizational delegation",
			EvidenceRef:        "authority-evidence-123",
			DecidedAt:          start.Add(time.Hour),
			RecordedAt:         start.Add(time.Hour),
		},
	}

	grants, err := ProjectAuthorityGrants(
		record,
		&journal,
		recordedThrough,
	)
	if err != nil {
		t.Fatalf("project future-effective activation: %v", err)
	}

	if len(grants) != 1 {
		t.Fatalf("grant count = %d, want 1; grants=%+v", len(grants), grants)
	}

	if grants[0].State != AuthorityGrantActive {
		t.Fatalf("grant state = %q, want active", grants[0].State)
	}

	if grants[0].GrantID != "authorization-123:event-3" {
		t.Fatalf("grant ID = %q, want authorization-123:event-3", grants[0].GrantID)
	}

	if grants[0].ValidUntil != nil {
		t.Fatalf("future-effective active grant unexpectedly bounded: %+v", grants[0])
	}

	if !grants[0].ValidFrom.Equal(activation) {
		t.Fatalf(
			"grant start = %s, want %s",
			grants[0].ValidFrom,
			activation,
		)
	}

	evidence := credential.NormalizedCredentialEvidence{
		ClaimType: credential.ClaimTypeEmploymentRole,
		Claim: credential.EmploymentClaim{
			ClaimType: credential.ClaimTypeEmploymentRole,
			Employment: credential.EmploymentRelationship{
				EmployerID: "employer-123",
			},
		},
		Issuer: credential.IssuerEvidence{
			Identifier: "https://issuer.example",
		},
	}

	tests := []struct {
		name       string
		at         time.Time
		decision   AuthorityDecision
		reason     string
		authorized bool
	}{
		{
			name:     "before activation",
			at:       recordedThrough,
			decision: AuthorityReview,
			reason:   "NO_CURRENT_GRANT",
		},
		{
			name:       "at activation",
			at:         activation,
			decision:   AuthorityAuthorized,
			reason:     "MATCHING_ACTIVE_GRANT",
			authorized: true,
		},
		{
			name:       "after activation",
			at:         activation.Add(time.Hour),
			decision:   AuthorityAuthorized,
			reason:     "MATCHING_ACTIVE_GRANT",
			authorized: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := (Evaluator{}).Evaluate(evidence, grants, tt.at)

			if result.Decision != tt.decision || result.Reason != tt.reason {
				t.Fatalf(
					"evaluation = %+v, want decision=%q reason=%q",
					result,
					tt.decision,
					tt.reason,
				)
			}

			if tt.authorized && result.GrantID != grants[0].GrantID {
				t.Fatalf(
					"authorized GrantID = %q, want %q",
					result.GrantID,
					grants[0].GrantID,
				)
			}
		})
	}
}
func TestAuthorizationJournal_SuspensionDoesNotRetroactivelyRemoveAuthority(t *testing.T) {
	var journal AuthorizationJournal

	start := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	activation := start.Add(2 * time.Hour)
	suspension := start.Add(4 * time.Hour)

	submit := journalEvent("event-1", EventSubmit, start, start)

	approve := journalEvent(
		"event-2",
		EventApprove,
		start.Add(time.Hour),
		start.Add(time.Hour),
	)
	approve.EvidenceRef = "authority-evidence-123"

	activate := journalEvent(
		"event-3",
		EventActivate,
		activation,
		activation,
	)

	suspend := journalEvent(
		"event-4",
		EventSuspend,
		suspension,
		suspension,
	)

	for _, event := range []AuthorizationJournalEvent{
		submit, approve, activate, suspend,
	} {
		if err := journal.Append(event); err != nil {
			t.Fatalf("append %s: %v", event.EventID, err)
		}
	}

	record := AuthorizationRecord{
		AuthorizationID:    "authorization-123",
		IssuerID:           "https://issuer.example",
		AuthoritySubjectID: "employer-123",
		ClaimType:          credential.ClaimTypeEmploymentRole,
		Adjudication: &AuthorizationAdjudication{
			DecisionID:         "decision-123",
			AuthorizationID:    "authorization-123",
			IssuerID:           "https://issuer.example",
			AuthoritySubjectID: "employer-123",
			ClaimType:          credential.ClaimTypeEmploymentRole,
			Outcome:            AdjudicationApproved,
			DecisionMakerID:    "cvera-adjudicator-1",
			AuthorityBasis:     "verified organizational delegation",
			EvidenceRef:        "authority-evidence-123",
			DecidedAt:          start.Add(time.Hour),
			RecordedAt:         start.Add(time.Hour),
		},
	}

	// Reconstruct after suspension has been recorded.
	grants, err := ProjectAuthorityGrants(
		record,
		&journal,
		start.Add(5*time.Hour),
	)
	if err != nil {
		t.Fatalf("project suspended authorization: %v", err)
	}

	evidence := credential.NormalizedCredentialEvidence{
		ClaimType: credential.ClaimTypeEmploymentRole,
		Claim: credential.EmploymentClaim{
			ClaimType: credential.ClaimTypeEmploymentRole,
			Employment: credential.EmploymentRelationship{
				EmployerID: "employer-123",
			},
		},
		Issuer: credential.IssuerEvidence{
			Identifier: "https://issuer.example",
		},
	}

	// The historical decision must reflect authority at 15:00,
	// not the subsequent suspension at 16:00.
	historical := (Evaluator{}).Evaluate(
		evidence,
		grants,
		start.Add(3*time.Hour),
	)

	if historical.Decision != AuthorityAuthorized {
		t.Fatalf(
			"historical decision = %+v, want AUTHORIZED",
			historical,
		)
	}

	// At suspension, the same authorization must no longer authorize.
	current := (Evaluator{}).Evaluate(
		evidence,
		grants,
		suspension,
	)

	if current.Decision != AuthorityUnauthorized {
		t.Fatalf(
			"suspension decision = %+v, want UNAUTHORIZED",
			current,
		)
	}
}

func TestAuthorizationJournal_ReinstatementRestoresAuthorityWithoutRewritingHistory(t *testing.T) {
	var journal AuthorizationJournal

	start := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	activation := start.Add(2 * time.Hour)
	suspension := start.Add(4 * time.Hour)
	reinstatement := start.Add(6 * time.Hour)

	submit := journalEvent("event-1", EventSubmit, start, start)

	approve := journalEvent(
		"event-2",
		EventApprove,
		start.Add(time.Hour),
		start.Add(time.Hour),
	)
	approve.EvidenceRef = "authority-evidence-123"

	events := []AuthorizationJournalEvent{
		submit,
		approve,
		journalEvent("event-3", EventActivate, activation, activation),
		journalEvent("event-4", EventSuspend, suspension, suspension),
		journalEvent("event-5", EventReinstate, reinstatement, reinstatement),
	}

	for _, event := range events {
		if err := journal.Append(event); err != nil {
			t.Fatalf("append %s: %v", event.EventID, err)
		}
	}

	record := AuthorizationRecord{
		AuthorizationID:    "authorization-123",
		IssuerID:           "https://issuer.example",
		AuthoritySubjectID: "employer-123",
		ClaimType:          credential.ClaimTypeEmploymentRole,
		Adjudication: &AuthorizationAdjudication{
			DecisionID:         "decision-123",
			AuthorizationID:    "authorization-123",
			IssuerID:           "https://issuer.example",
			AuthoritySubjectID: "employer-123",
			ClaimType:          credential.ClaimTypeEmploymentRole,
			Outcome:            AdjudicationApproved,
			DecisionMakerID:    "cvera-adjudicator-1",
			AuthorityBasis:     "verified organizational delegation",
			EvidenceRef:        "authority-evidence-123",
			DecidedAt:          start.Add(time.Hour),
			RecordedAt:         start.Add(time.Hour),
		},
	}

	// Project the complete history after reinstatement is recorded.
	grants, err := ProjectAuthorityGrants(
		record,
		&journal,
		start.Add(7*time.Hour),
	)
	if err != nil {
		t.Fatalf("project reinstated authorization: %v", err)
	}

	evidence := credential.NormalizedCredentialEvidence{
		ClaimType: credential.ClaimTypeEmploymentRole,
		Claim: credential.EmploymentClaim{
			ClaimType: credential.ClaimTypeEmploymentRole,
			Employment: credential.EmploymentRelationship{
				EmployerID: "employer-123",
			},
		},
		Issuer: credential.IssuerEvidence{
			Identifier: "https://issuer.example",
		},
	}

	tests := []struct {
		name string
		at   time.Time
		want AuthorityDecision
	}{
		{
			name: "historical active authority remains authorized",
			at:   start.Add(3 * time.Hour),
			want: AuthorityAuthorized,
		},
		{
			name: "suspended authority remains unauthorized",
			at:   start.Add(5 * time.Hour),
			want: AuthorityUnauthorized,
		},
		{
			name: "reinstatement boundary restores authority",
			at:   reinstatement,
			want: AuthorityAuthorized,
		},
		{
			name: "authority remains active after reinstatement",
			at:   start.Add(7 * time.Hour),
			want: AuthorityAuthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := (Evaluator{}).Evaluate(evidence, grants, tt.at)

			if result.Decision != tt.want {
				t.Fatalf(
					"decision at %s = %+v, want %s",
					tt.at,
					result,
					tt.want,
				)
			}
		})
	}
}
