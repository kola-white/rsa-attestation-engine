package authority

import "testing"

func TestStateMachine_PermittedTransitions(t *testing.T) {
	tests := []struct {
		name    string
		current AuthorizationState
		event   AuthorizationEvent
		want    AuthorizationState
	}{
		{"requested submit pending", AuthorizationRequested, EventSubmit, AuthorizationPending},
		{"pending approve approved", AuthorizationPending, EventApprove, AuthorizationApproved},
		{"pending reject rejected", AuthorizationPending, EventReject, AuthorizationRejected},
		{"approved activate active", AuthorizationApproved, EventActivate, AuthorizationActive},
		{"active suspend suspended", AuthorizationActive, EventSuspend, AuthorizationSuspended},
		{"suspended reinstate active", AuthorizationSuspended, EventReinstate, AuthorizationActive},
		{"active expire expired", AuthorizationActive, EventExpire, AuthorizationExpired},
		{"suspended expire expired", AuthorizationSuspended, EventExpire, AuthorizationExpired},
		{"active revoke revoked", AuthorizationActive, EventRevoke, AuthorizationRevoked},
		{"suspended revoke revoked", AuthorizationSuspended, EventRevoke, AuthorizationRevoked},
	}

	machine := StateMachine{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := machine.Transition(
				tt.current,
				tt.event,
				TransitionConditions{Satisfied: true},
			)

			if !got.Applied {
				t.Fatalf("applied = false, want true; reason = %q", got.Reason)
			}
			if got.State != tt.want {
				t.Fatalf("state = %q, want %q", got.State, tt.want)
			}
			if got.Reason != ReasonTransitionApplied {
				t.Fatalf("reason = %q, want %q", got.Reason, ReasonTransitionApplied)
			}
		})
	}
}

func TestStateMachine_UnsatisfiedConditionsRejectEveryPermittedTransition(t *testing.T) {
	transitions := []struct {
		current AuthorizationState
		event   AuthorizationEvent
	}{
		{AuthorizationRequested, EventSubmit},
		{AuthorizationPending, EventApprove},
		{AuthorizationPending, EventReject},
		{AuthorizationApproved, EventActivate},
		{AuthorizationActive, EventSuspend},
		{AuthorizationSuspended, EventReinstate},
		{AuthorizationActive, EventExpire},
		{AuthorizationSuspended, EventExpire},
		{AuthorizationActive, EventRevoke},
		{AuthorizationSuspended, EventRevoke},
	}

	machine := StateMachine{}

	for _, tt := range transitions {
		got := machine.Transition(
			tt.current,
			tt.event,
			TransitionConditions{Satisfied: false},
		)

		if got.Applied {
			t.Fatalf(
				"transition %q + %q applied with unsatisfied conditions",
				tt.current,
				tt.event,
			)
		}
		if got.State != tt.current {
			t.Fatalf(
				"transition %q + %q mutated state to %q",
				tt.current,
				tt.event,
				got.State,
			)
		}
		if got.Reason != ReasonConditionsNotSatisfied {
			t.Fatalf(
				"transition %q + %q reason = %q, want %q",
				tt.current,
				tt.event,
				got.Reason,
				ReasonConditionsNotSatisfied,
			)
		}
	}
}

func TestStateMachine_CompleteNegativeSpace(t *testing.T) {
	states := []AuthorizationState{
		AuthorizationRequested,
		AuthorizationPending,
		AuthorizationApproved,
		AuthorizationActive,
		AuthorizationSuspended,
		AuthorizationRejected,
		AuthorizationExpired,
		AuthorizationRevoked,
	}

	events := []AuthorizationEvent{
		EventSubmit,
		EventApprove,
		EventReject,
		EventActivate,
		EventSuspend,
		EventReinstate,
		EventExpire,
		EventRevoke,
	}

	permitted := map[AuthorizationState]map[AuthorizationEvent]bool{
		AuthorizationRequested: {
			EventSubmit: true,
		},
		AuthorizationPending: {
			EventApprove: true,
			EventReject:  true,
		},
		AuthorizationApproved: {
			EventActivate: true,
		},
		AuthorizationActive: {
			EventSuspend: true,
			EventExpire:  true,
			EventRevoke:  true,
		},
		AuthorizationSuspended: {
			EventReinstate: true,
			EventExpire:    true,
			EventRevoke:    true,
		},
	}

	machine := StateMachine{}

	tested := 0
	for _, state := range states {
		for _, event := range events {
			if permitted[state][event] {
				continue
			}

			tested++

			got := machine.Transition(
				state,
				event,
				TransitionConditions{Satisfied: true},
			)

			if got.Applied {
				t.Fatalf(
					"invalid transition %q + %q unexpectedly applied",
					state,
					event,
				)
			}
			if got.State != state {
				t.Fatalf(
					"invalid transition %q + %q mutated state to %q",
					state,
					event,
					got.State,
				)
			}
			if got.Reason != ReasonInvalidTransition {
				t.Fatalf(
					"invalid transition %q + %q reason = %q, want %q",
					state,
					event,
					got.Reason,
					ReasonInvalidTransition,
				)
			}
		}
	}

	if tested != 54 {
		t.Fatalf("tested %d invalid transitions, want 54", tested)
	}
}

func TestStateMachine_TerminalStatesHaveNoOutgoingTransitions(t *testing.T) {
	terminalStates := []AuthorizationState{
		AuthorizationRejected,
		AuthorizationExpired,
		AuthorizationRevoked,
	}

	events := []AuthorizationEvent{
		EventSubmit,
		EventApprove,
		EventReject,
		EventActivate,
		EventSuspend,
		EventReinstate,
		EventExpire,
		EventRevoke,
	}

	machine := StateMachine{}

	for _, state := range terminalStates {
		for _, event := range events {
			got := machine.Transition(
				state,
				event,
				TransitionConditions{Satisfied: true},
			)

			if got.Applied {
				t.Fatalf(
					"terminal state %q accepted event %q",
					state,
					event,
				)
			}
			if got.State != state {
				t.Fatalf(
					"terminal state %q mutated to %q for event %q",
					state,
					got.State,
					event,
				)
			}
			if got.Reason != ReasonInvalidTransition {
				t.Fatalf(
					"terminal state %q event %q reason = %q, want %q",
					state,
					event,
					got.Reason,
					ReasonInvalidTransition,
				)
			}
		}
	}
}

func TestStateMachine_UnknownStateFailsClosed(t *testing.T) {
	machine := StateMachine{}
	current := AuthorizationState("UNKNOWN_STATE")

	got := machine.Transition(
		current,
		EventSubmit,
		TransitionConditions{Satisfied: true},
	)

	if got.Applied {
		t.Fatal("transition from unknown state unexpectedly applied")
	}
	if got.State != current {
		t.Fatalf("state = %q, want unchanged %q", got.State, current)
	}
	if got.Reason != ReasonInvalidTransition {
		t.Fatalf("reason = %q, want %q", got.Reason, ReasonInvalidTransition)
	}
}

func TestStateMachine_UnknownEventFailsClosed(t *testing.T) {
	machine := StateMachine{}
	current := AuthorizationActive

	got := machine.Transition(
		current,
		AuthorizationEvent("UNKNOWN_EVENT"),
		TransitionConditions{Satisfied: true},
	)

	if got.Applied {
		t.Fatal("unknown event unexpectedly applied")
	}
	if got.State != current {
		t.Fatalf("state = %q, want unchanged %q", got.State, current)
	}
	if got.Reason != ReasonInvalidTransition {
		t.Fatalf("reason = %q, want %q", got.Reason, ReasonInvalidTransition)
	}
}
