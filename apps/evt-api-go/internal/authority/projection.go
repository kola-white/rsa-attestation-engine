package authority

import (
	"errors"
	"time"
)

var errInvalidGrantInterval = errors.New("invalid authority grant interval")

// projectGrantInterval converts a canonical half-open authority interval
// [start, end) into the inclusive interval used by AuthorityGrant.
//
// Both boundaries must be UTC and aligned to microsecond precision.
// A nil end represents an unbounded interval.
func projectGrantInterval(
	start time.Time,
	end *time.Time,
) (time.Time, *time.Time, error) {
	if !canonicalAuthorityTimestamp(start) {
		return time.Time{}, nil, errInvalidGrantInterval
	}

	if end == nil {
		return start, nil, nil
	}

	if !canonicalAuthorityTimestamp(*end) || !end.After(start) {
		return time.Time{}, nil, errInvalidGrantInterval
	}

	inclusiveEnd := end.Add(-time.Microsecond)

	return start, &inclusiveEnd, nil
}

// canonicalAuthorityTimestamp accepts only explicit UTC timestamps
// aligned to whole microseconds.
func canonicalAuthorityTimestamp(value time.Time) bool {
	return !value.IsZero() &&
		value.Location() == time.UTC &&
		value.Nanosecond()%1000 == 0
}

// AuthorizationRecord binds an authorization lifecycle to its
// independently adjudicated organizational authority scope.
// Adjudication provenance is required before grants can be projected.
type AuthorizationRecord struct {
	AuthorizationID    string
	IssuerID           string
	AuthoritySubjectID string
	ClaimType          string
	Adjudication       *AuthorizationAdjudication
}

type AdjudicationOutcome string

const (
	AdjudicationApproved AdjudicationOutcome = "APPROVED"
	AdjudicationRejected AdjudicationOutcome = "REJECTED"
)

// AuthorizationAdjudication records Cvera's attributable
// determination of organizational authorization.
type AuthorizationAdjudication struct {
	DecisionID         string
	AuthorizationID    string
	IssuerID           string
	AuthoritySubjectID string
	ClaimType          string
	Outcome            AdjudicationOutcome
	DecisionMakerID    string
	AuthorityBasis     string
	EvidenceRef        string
	DecidedAt          time.Time
	RecordedAt         time.Time
}

// ProjectAuthorityGrants derives time-bounded authority grants from
// independently adjudicated authorization evidence and accepted
// lifecycle history, as known at the recorded-time cutoff.
//
// Missing or inconsistent adjudication, lifecycle, or temporal
// evidence causes projection to fail closed.
func ProjectAuthorityGrants(
	record AuthorizationRecord,
	journal *AuthorizationJournal,
	recordedThrough time.Time,
) ([]AuthorityGrant, error) {
	if record.AuthorizationID == "" ||
		record.IssuerID == "" ||
		record.AuthoritySubjectID == "" ||
		record.ClaimType == "" ||
		journal == nil ||
		!canonicalAuthorityTimestamp(recordedThrough) {
		return nil, errInvalidJournalEvent
	}

	adjudication := record.Adjudication
	if adjudication == nil ||
		adjudication.DecisionID == "" ||
		adjudication.AuthorizationID != record.AuthorizationID ||
		adjudication.IssuerID != record.IssuerID ||
		adjudication.AuthoritySubjectID != record.AuthoritySubjectID ||
		adjudication.ClaimType != record.ClaimType ||
		adjudication.Outcome != AdjudicationApproved ||
		adjudication.DecisionMakerID == "" ||
		adjudication.AuthorityBasis == "" ||
		adjudication.EvidenceRef == "" ||
		!canonicalAuthorityTimestamp(adjudication.DecidedAt) ||
		!canonicalAuthorityTimestamp(adjudication.RecordedAt) ||
		adjudication.DecidedAt.After(adjudication.RecordedAt) ||
		adjudication.RecordedAt.After(recordedThrough) {
		return nil, errInvalidJournalEvent
	}

	events, err := journal.EffectiveEvents(
		record.AuthorizationID,
		recordedThrough,
	)
	if err != nil {
		return nil, err
	}

	// The lifecycle must contain an attributable approval and activation.
	// Each interval retains the authority state that applied during it.
	// Later suspension or revocation must not rewrite earlier ACTIVE time.
	var approved bool
	var intervalFrom time.Time
	var intervalGrantID string
	var intervalState AuthorityGrantState
	var lifecycleState AuthorizationState = AuthorizationRequested
	var grants []AuthorityGrant

	openInterval := func(event AuthorizationJournalEvent, state AuthorityGrantState) error {
		if !intervalFrom.IsZero() {
			return errInvalidJournalEvent
		}

		intervalFrom = event.EffectiveAt
		intervalGrantID = record.AuthorizationID + ":" + event.EventID
		intervalState = state
		return nil
	}

	closeInterval := func(end time.Time) error {
		if intervalFrom.IsZero() {
			return errInvalidJournalEvent
		}

		from, until, err := projectGrantInterval(intervalFrom, &end)
		if err != nil {
			return err
		}

		grants = append(grants, AuthorityGrant{
			GrantID:            intervalGrantID,
			IssuerID:           record.IssuerID,
			ClaimType:          record.ClaimType,
			AuthoritySubjectID: record.AuthoritySubjectID,
			ValidFrom:          from,
			ValidUntil:         until,
			State:              intervalState,
		})

		intervalFrom = time.Time{}
		intervalGrantID = ""
		return nil
	}

	for _, event := range events {
		transition := (StateMachine{}).Transition(
			lifecycleState,
			event.EventType,
			TransitionConditions{Satisfied: event.ConditionsSatisfied},
		)
		if !transition.Applied {
			return nil, errInvalidJournalEvent
		}
		lifecycleState = transition.State

		switch event.EventType {
		case EventApprove:
			if event.EvidenceRef != adjudication.EvidenceRef ||
				event.ActorID != adjudication.DecisionMakerID ||
				adjudication.RecordedAt.After(event.RecordedAt) {
				return nil, errInvalidJournalEvent
			}
			approved = true

		case EventActivate:
			if !approved {
				return nil, errInvalidJournalEvent
			}
			if err := openInterval(event, AuthorityGrantActive); err != nil {
				return nil, err
			}

		case EventSuspend:
			if intervalState != AuthorityGrantActive {
				return nil, errInvalidJournalEvent
			}
			if err := closeInterval(event.EffectiveAt); err != nil {
				return nil, err
			}
			if err := openInterval(event, AuthorityGrantInactive); err != nil {
				return nil, err
			}

		case EventReinstate:
			if intervalState != AuthorityGrantInactive {
				return nil, errInvalidJournalEvent
			}
			if err := closeInterval(event.EffectiveAt); err != nil {
				return nil, err
			}
			if err := openInterval(event, AuthorityGrantActive); err != nil {
				return nil, err
			}

		case EventExpire:
			if !intervalFrom.IsZero() {
				if err := closeInterval(event.EffectiveAt); err != nil {
					return nil, err
				}
			}

		case EventRevoke:
			if !intervalFrom.IsZero() {
				if err := closeInterval(event.EffectiveAt); err != nil {
					return nil, err
				}
			}
			if err := openInterval(event, AuthorityGrantInactive); err != nil {
				return nil, err
			}
		}
	}

	if !approved {
		return nil, errInvalidJournalEvent
	}

	if lifecycleState == AuthorizationActive &&
		(intervalFrom.IsZero() || intervalState != AuthorityGrantActive) {
		return nil, errInvalidJournalEvent
	}

	// Architecture §5: revocation establishes explicit negative
	// authority from the effective withdrawal time onward.
	if lifecycleState == AuthorizationRevoked &&
		(intervalFrom.IsZero() || intervalState != AuthorityGrantInactive) {
		return nil, errInvalidJournalEvent
	}

	if lifecycleState == AuthorizationSuspended &&
		(intervalFrom.IsZero() || intervalState != AuthorityGrantInactive) {
		return nil, errInvalidJournalEvent
	}

	if lifecycleState != AuthorizationActive &&
		lifecycleState != AuthorizationSuspended &&
		lifecycleState != AuthorizationRevoked &&
		!intervalFrom.IsZero() {
		return nil, errInvalidJournalEvent
	}

	if !intervalFrom.IsZero() {
		grants = append(grants, AuthorityGrant{
			GrantID:            intervalGrantID,
			IssuerID:           record.IssuerID,
			ClaimType:          record.ClaimType,
			AuthoritySubjectID: record.AuthoritySubjectID,
			ValidFrom:          intervalFrom,
			State:              intervalState,
		})
	}

	return grants, nil
}
