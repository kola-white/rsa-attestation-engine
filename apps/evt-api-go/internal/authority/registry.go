package authority

import (
	"errors"
	"time"
)

var errInvalidJournalEvent = errors.New("invalid authority journal event")

// AuthorizationJournalEvent is an immutable-by-contract record of an
// accepted lifecycle event or an attributable correction to one.
type AuthorizationJournalEvent struct {
	EventID             string
	AuthorizationID     string
	EventType           AuthorizationEvent
	EffectiveAt         time.Time
	RecordedAt          time.Time
	ActorID             string
	EvidenceRef         string
	ConditionsSatisfied bool
	CorrectsEventID     string
	CorrectionReason    string
	CorrectedField      string
}

// AuthorizationJournal preserves accepted events in append order.
// The zero value is an empty journal.
type AuthorizationJournal struct {
	events []AuthorizationJournalEvent
}

// Append validates and records an event without altering prior events.
func (j *AuthorizationJournal) Append(event AuthorizationJournalEvent) error {
	if event.EventID == "" ||
		event.AuthorizationID == "" ||
		event.ActorID == "" ||
		event.EvidenceRef == "" ||
		!canonicalAuthorityTimestamp(event.EffectiveAt) ||
		!canonicalAuthorityTimestamp(event.RecordedAt) {
		return errInvalidJournalEvent
	}

	// Preserve the recorded-time ordering of the append-only journal.
	// Equal timestamps are permitted; append order breaks ties.
	if len(j.events) > 0 &&
		event.RecordedAt.Before(j.events[len(j.events)-1].RecordedAt) {
		return errInvalidJournalEvent
	}

	for _, existing := range j.events {
		if existing.EventID == event.EventID {
			return errInvalidJournalEvent
		}
	}

	if event.CorrectsEventID != "" {
		if event.CorrectionReason == "" || event.CorrectedField == "" {
			return errInvalidJournalEvent
		}

		found := false
		for _, existing := range j.events {
			if existing.EventID == event.CorrectsEventID &&
				existing.AuthorizationID == event.AuthorizationID {
				found = true
				break
			}
		}
		if !found {
			return errInvalidJournalEvent
		}
	} else {
		state, err := j.Reconstruct(event.AuthorizationID, event.RecordedAt)
		if err != nil {
			return err
		}

		result := StateMachine{}.Transition(
			state,
			event.EventType,
			TransitionConditions{Satisfied: event.ConditionsSatisfied},
		)
		if !result.Applied {
			return errInvalidJournalEvent
		}
	}

	j.events = append(j.events, event)
	return nil
}

// Events returns a defensive copy of the append-only journal.
func (j *AuthorizationJournal) Events() []AuthorizationJournalEvent {
	return append([]AuthorizationJournalEvent(nil), j.events...)
}

// Reconstruct returns the lifecycle state known at a recorded-time cutoff.
// Corrections remain visible in the journal but do not yet amend replay.
func (j *AuthorizationJournal) Reconstruct(
	authorizationID string,
	recordedThrough time.Time,
) (AuthorizationState, error) {
	if authorizationID == "" || !canonicalAuthorityTimestamp(recordedThrough) {
		return "", errInvalidJournalEvent
	}

	state := AuthorizationRequested

	for _, event := range j.events {
		if event.AuthorizationID != authorizationID ||
			event.RecordedAt.After(recordedThrough) ||
			event.CorrectsEventID != "" {
			continue
		}

		result := StateMachine{}.Transition(
			state,
			event.EventType,
			TransitionConditions{Satisfied: event.ConditionsSatisfied},
		)
		if !result.Applied {
			return "", errInvalidJournalEvent
		}
		state = result.State
	}

	return state, nil
}

// EffectiveEvents returns a derived lifecycle-event view as known at the
// specified recorded-time cutoff. It never modifies stored journal events.
func (j *AuthorizationJournal) EffectiveEvents(
	authorizationID string,
	recordedThrough time.Time,
) ([]AuthorizationJournalEvent, error) {
	if authorizationID == "" || !canonicalAuthorityTimestamp(recordedThrough) {
		return nil, errInvalidJournalEvent
	}

	var effective []AuthorizationJournalEvent
	positions := make(map[string]int)
	corrected := make(map[string]bool)

	for _, event := range j.events {
		if event.RecordedAt.After(recordedThrough) {
			break
		}

		if event.AuthorizationID != authorizationID {
			continue
		}

		if event.CorrectsEventID == "" {
			positions[event.EventID] = len(effective)
			effective = append(effective, event)
			continue
		}

		if event.CorrectedField != "EffectiveAt" {
			return nil, errInvalidJournalEvent
		}

		position, found := positions[event.CorrectsEventID]
		if !found || corrected[event.CorrectsEventID] {
			return nil, errInvalidJournalEvent
		}

		if !canonicalAuthorityTimestamp(event.EffectiveAt) {
			return nil, errInvalidJournalEvent
		}

		corrected[event.CorrectsEventID] = true

		// Apply the amended timestamp only to the derived view.
		// The original stored event remains unchanged.
		effective[position].EffectiveAt = event.EffectiveAt
	}

	// Validate the derived lifecycle history without changing journal order.
	// Equal effective timestamps are permitted; event order breaks ties.
	state := AuthorizationRequested

	for i, event := range effective {
		if i > 0 {
			previous := effective[i-1]

			if event.EffectiveAt.Before(previous.EffectiveAt) {
				return nil, errInvalidJournalEvent
			}

			// Architecture §5: every interval opened by ACTIVATE,
			// SUSPEND, REINSTATE, or REVOKE must have positive
			// duration before a subsequent lifecycle transition
			// closes it. Equal timestamps cannot represent that
			// interval in the authority grant projection.
			switch previous.EventType {
			case EventActivate, EventSuspend, EventReinstate, EventRevoke:
				if !event.EffectiveAt.After(previous.EffectiveAt) {
					return nil, errInvalidJournalEvent
				}
			}
		}

		result := StateMachine{}.Transition(
			state,
			event.EventType,
			TransitionConditions{Satisfied: event.ConditionsSatisfied},
		)
		if !result.Applied {
			return nil, errInvalidJournalEvent
		}

		state = result.State
	}

	return effective, nil
}
