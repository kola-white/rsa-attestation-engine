package httpapi

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrUserNotFound = errors.New("user_not_found")

// ResolveUserID resolves the canonical UUID used in evt_requests.candidate_user_id / hr_assigned_user_id.
//
// Assumes domain_users has:
// - user_id UUID (PK or unique)
// - kratos_identity_id TEXT (unique)
func ResolveUserID(ctx context.Context, db *pgxpool.Pool, subject string) (string, error) {
	// subject is claims.Sub (either a domain user UUID or a Kratos identity id).
	var userID string

	// Try: subject is already the domain user UUID
	err := db.QueryRow(ctx, `
		SELECT user_id::text
		FROM domain_users
		WHERE user_id::text = $1
	`, subject).Scan(&userID)
	if err == nil {
		return userID, nil
	}

	// Fallback: subject is the Kratos identity id
	err = db.QueryRow(ctx, `
		SELECT user_id::text
		FROM domain_users
		WHERE kratos_identity_id = $1
	`, subject).Scan(&userID)
	if err == nil {
		return userID, nil
	}

	return "", ErrUserNotFound
}
