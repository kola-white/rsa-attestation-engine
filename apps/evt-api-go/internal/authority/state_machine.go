package authority

// AuthorizationState is the lifecycle state of a specific issuer-authority
// authorization.
//
// These states are deliberately separate from AuthorityDecision and
// AuthorityGrantState. Lifecycle state does not itself represent a verifier
// decision or final Cvera verification outcome.
type AuthorizationState string

const (
	AuthorizationRequested AuthorizationState = "REQUESTED"
	AuthorizationPending   AuthorizationState = "PENDING"
	AuthorizationApproved  AuthorizationState = "APPROVED"
	AuthorizationActive    AuthorizationState = "ACTIVE"
	AuthorizationSuspended AuthorizationState = "SUSPENDED"
	AuthorizationRejected  AuthorizationState = "REJECTED"
	AuthorizationExpired   AuthorizationState = "EXPIRED"
	AuthorizationRevoked   AuthorizationState = "REVOKED"
)

// AuthorizationEvent is an event that may request a lifecycle transition.
type AuthorizationEvent string

const (
	EventSubmit    AuthorizationEvent = "SUBMIT"
	EventApprove   AuthorizationEvent = "APPROVE"
	EventReject    AuthorizationEvent = "REJECT"
	EventActivate  AuthorizationEvent = "ACTIVATE"
	EventSuspend   AuthorizationEvent = "SUSPEND"
	EventReinstate AuthorizationEvent = "REINSTATE"
	EventExpire    AuthorizationEvent = "EXPIRE"
	EventRevoke    AuthorizationEvent = "REVOKE"
)

// TransitionConditions represents whether the conditions required by the
// canonical authority-establishment specification have already been
// established by the responsible orchestration/evidence layer.
//
// The state machine deliberately does not establish those facts itself.
type TransitionConditions struct {
	Satisfied bool
}

// TransitionResult is the deterministic result of evaluating a requested
// authority lifecycle transition.
type TransitionResult struct {
	State   AuthorizationState
	Applied bool
	Reason  string
}

const (
	ReasonTransitionApplied      = "TRANSITION_APPLIED"
	ReasonConditionsNotSatisfied = "CONDITIONS_NOT_SATISFIED"
	ReasonInvalidTransition      = "INVALID_TRANSITION"
)

// StateMachine applies the canonical authority-establishment lifecycle.
type StateMachine struct{}

// Transition evaluates a requested lifecycle transition.
//
// Invalid transitions fail closed and preserve the current state. A permitted
// transition is applied only when its required conditions have been
// established.
func (StateMachine) Transition(
	current AuthorizationState,
	event AuthorizationEvent,
	conditions TransitionConditions,
) TransitionResult {
	next, permitted := nextAuthorizationState(current, event)
	if !permitted {
		return TransitionResult{
			State:   current,
			Applied: false,
			Reason:  ReasonInvalidTransition,
		}
	}

	if !conditions.Satisfied {
		return TransitionResult{
			State:   current,
			Applied: false,
			Reason:  ReasonConditionsNotSatisfied,
		}
	}

	return TransitionResult{
		State:   next,
		Applied: true,
		Reason:  ReasonTransitionApplied,
	}
}

func nextAuthorizationState(
	current AuthorizationState,
	event AuthorizationEvent,
) (AuthorizationState, bool) {
	switch current {
	case AuthorizationRequested:
		if event == EventSubmit {
			return AuthorizationPending, true
		}

	case AuthorizationPending:
		switch event {
		case EventApprove:
			return AuthorizationApproved, true
		case EventReject:
			return AuthorizationRejected, true
		}

	case AuthorizationApproved:
		if event == EventActivate {
			return AuthorizationActive, true
		}

	case AuthorizationActive:
		switch event {
		case EventSuspend:
			return AuthorizationSuspended, true
		case EventExpire:
			return AuthorizationExpired, true
		case EventRevoke:
			return AuthorizationRevoked, true
		}

	case AuthorizationSuspended:
		switch event {
		case EventReinstate:
			return AuthorizationActive, true
		case EventExpire:
			return AuthorizationExpired, true
		case EventRevoke:
			return AuthorizationRevoked, true
		}
	}

	return current, false
}
