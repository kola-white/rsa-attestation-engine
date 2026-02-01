package evt

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
)

// HREmployerAttest is a backward-compatible alias that now delegates to the
// server-signed implementation and returns the resulting status.
func (r *Repo) HREmployerAttest(
	ctx context.Context,
	tx pgx.Tx,
	requestID string,
	employerID string,
	employerHRPersonID string,
	responseType string,
	responseBody json.RawMessage,
	attestationJWS string, // kept to avoid changing callers; ignored now
) (string, error) {
	out, err := r.EmployerAttestServerSigned(
		ctx,
		tx,
		requestID,
		employerID,
		employerHRPersonID,
		responseType,
		responseBody,
	)
	if err != nil {
		return "", err
	}

	return out.Status, nil
}
