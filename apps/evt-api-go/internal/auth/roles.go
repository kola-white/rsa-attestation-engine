package auth

import (
	"fmt"
	"strings"
)

type Role string

const (
	RoleRequestor  Role = "requestor"
	RoleHRReviewer Role = "hr_reviewer"
	RoleRecruiter  Role = "recruiter"
	RoleCvera      Role = "cvera"
)

func (r Role) String() string { return string(r) }

// ParseRole normalizes + validates role strings coming from DB or computed rules.
func ParseRole(s string) (Role, error) {
	v := Role(strings.ToLower(strings.TrimSpace(s)))
	switch v {
	case RoleRequestor, RoleHRReviewer, RoleRecruiter, RoleCvera:
		return v, nil
	default:
		return "", fmt.Errorf("invalid role: %q", s)
	}
}
