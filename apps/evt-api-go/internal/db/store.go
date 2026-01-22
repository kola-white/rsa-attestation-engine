package db 

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type EnsureDomainUserInput struct {
	KratosIdentityID string // REQUIRED (claims.sub)
	Email            *string
	Name             *string
	Role             string // e.g. "hr_reviewer" / "recruiter" / "candidate"
	Status           string // e.g. "active"
}

func EnsureDomainUser(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, in EnsureDomainUserInput) (uuid.UUID, error) {

	if in.KratosIdentityID == "" {
		return uuid.Nil, errors.New("kratos identity id is empty")
	}
	if in.Role == "" {
		in.Role = "hr_reviewer"
	}
	if in.Status == "" {
		in.Status = "active"
	}

	var personID uuid.UUID
	err := q.QueryRow(ctx, `
		INSERT INTO domain_users (kratos_identity_id, email, name, role, status)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (kratos_identity_id)
		DO UPDATE SET
			email = COALESCE(EXCLUDED.email, domain_users.email),
			name  = COALESCE(EXCLUDED.name,  domain_users.name),
			role  = COALESCE(EXCLUDED.role,  domain_users.role),
			status= COALESCE(EXCLUDED.status,domain_users.status),
			updated_at = now()
		RETURNING id
	`, in.KratosIdentityID, in.Email, in.Name, in.Role, in.Status).Scan(&personID)

	return personID, err
}
