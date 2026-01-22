package evt

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
)

// HREmployerAttest is a handler-facing adapter that maps to Repo.EmployerAttest.
// It returns the new status string so handlers can respond consistently.
func (r *Repo) HREmployerAttest(
	ctx context.Context,
	tx pgx.Tx,
	requestID string,
	employerID string,
	employerHRPersonID string,
	responseType string,
	responseBody json.RawMessage,
	attestationJWS string,
) (string, error) {
	if err := r.EmployerAttest(ctx, tx, requestID, employerID, employerHRPersonID, responseType, responseBody, attestationJWS); err != nil {
		return "", err
	}
	// Your repo method returns error only; the new status is deterministically "ATTESTED".
	// If you later make status dynamic, change Repo.EmployerAttest to return status.
	return "ATTESTED", nil
}
