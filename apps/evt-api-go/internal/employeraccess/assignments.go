// Package employeraccess enforces explicit application-principal assignments.
// It does not establish an issuer's credential assertion authority.
package employeraccess

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/db"
)

// Authorize retains a shared principal lock until the caller commits. Operator
// grant/revoke transactions take the exclusive version of the same lock first.
// Callers must use READ COMMITTED so a waiter reads grants after revocation.
func Authorize(ctx context.Context, tx pgx.Tx, principal, employer string) error {
	if err := lockReviewer(ctx, tx, principal, false); err != nil {
		return err
	}
	var assignment uuid.UUID
	err := tx.QueryRow(ctx, `SELECT assignment_id FROM employer_hr_assignments
 WHERE principal_id=$1 AND employer_id=$2 AND revoked_at IS NULL`, principal, employer).Scan(&assignment)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.ErrNotFound
	}
	return err
}

func lockReviewer(ctx context.Context, tx pgx.Tx, principal string, exclusive bool) error {
	if _, err := uuid.Parse(principal); err != nil {
		return db.ErrNotFound
	}
	var status, role string
	query := `SELECT status, role FROM domain_users WHERE id=$1 FOR SHARE`
	if exclusive {
		query = `SELECT status, role FROM domain_users WHERE id=$1 FOR UPDATE`
	}
	err := tx.QueryRow(ctx, query, principal).Scan(&status, &role)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.ErrNotFound
	}
	if err != nil {
		return err
	}
	if status != "active" || role != "hr_reviewer" {
		return db.ErrNotFound
	}
	return nil
}

// Grant is operator-only. Database privileges, not an API role or this exported
// function, must prevent runtime credentials from writing the assignment table.
// No config row or user is created implicitly.
func Grant(ctx context.Context, tx pgx.Tx, principal, employer, actor, basis string) (uuid.UUID, error) {
	if strings.TrimSpace(actor) == "" || strings.TrimSpace(basis) == "" || strings.TrimSpace(employer) == "" {
		return uuid.Nil, errors.New("actor, approval basis and employer are required")
	}
	if err := lockReviewer(ctx, tx, principal, true); err != nil {
		return uuid.Nil, err
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM evt_employer_config WHERE employer_id=$1)`, employer).Scan(&exists); err != nil {
		return uuid.Nil, err
	}
	if !exists {
		return uuid.Nil, db.ErrNotFound
	}
	id := uuid.New()
	_, err := tx.Exec(ctx, `INSERT INTO employer_hr_assignments
 (assignment_id,principal_id,employer_id,granted_at,granted_by,grant_basis_ref)
 VALUES($1,$2,$3,clock_timestamp(),$4,$5)`, id, principal, employer, actor, basis)
	return id, err
}

// Revoke works even after an account is disabled or its role changes. Revoked
// grant instances are retained and cannot be reactivated by this operation.
func Revoke(ctx context.Context, tx pgx.Tx, id uuid.UUID, actor, reason string) error {
	if strings.TrimSpace(actor) == "" || strings.TrimSpace(reason) == "" {
		return errors.New("actor and revocation reason are required")
	}
	var principal uuid.UUID
	err := tx.QueryRow(ctx, `SELECT principal_id FROM employer_hr_assignments WHERE assignment_id=$1`, id).Scan(&principal)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.ErrNotFound
	}
	if err != nil {
		return err
	}
	// Lock in the same order as authorization, before modifying an assignment.
	var locked uuid.UUID
	if err = tx.QueryRow(ctx, `SELECT id FROM domain_users WHERE id=$1 FOR UPDATE`, principal).Scan(&locked); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE employer_hr_assignments
 SET revoked_at=clock_timestamp(),revoked_by=$2,revocation_reason=$3
 WHERE assignment_id=$1 AND revoked_at IS NULL`, id, actor, reason)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return db.ErrConflict
	}
	return nil
}
