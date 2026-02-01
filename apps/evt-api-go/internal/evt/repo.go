package evt

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/db"
)
type AttestResult struct {
	Status            string
	AttestationPresent bool
	KID               string
	JWSBytes          int
}

type Repo struct {
	DB *db.DB
	Signer *AttestationSigner
}

func ptr(s string) *string { return &s }

func insertEvent(
	ctx context.Context,
	tx pgx.Tx,
	requestID string,
	actorRole string,
	actorPersonID *string,
	eventType string,
	fromStatus *string,
	toStatus *string,
	payload any,
) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO evt_request_events
		  (request_id, actor_role, actor_personid, event_type, from_status, to_status, payload)
		VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb)
	`, requestID, actorRole, actorPersonID, eventType, fromStatus, toStatus, string(b))
	return err
}

func ensureEmployerConfig(ctx context.Context, tx pgx.Tx, employerID string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO evt_employer_config (employer_id, dispatch_mode)
		VALUES ($1, 'AUTO')
		ON CONFLICT (employer_id) DO NOTHING
	`, employerID)
	return err
}

/* ---------------- Candidate writes ---------------- */

func (r *Repo) CandidateCreateDraft(ctx context.Context, tx pgx.Tx, candidatePersonID, employerID string, claim json.RawMessage) (string, error) {
	if err := ensureEmployerConfig(ctx, tx, employerID); err != nil {
		return "", err
	}

	var requestID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO evt_requests (candidate_personid, employer_id, status, claim_snapshot)
		VALUES ($1, $2, 'DRAFT', COALESCE($3::jsonb, '{}'::jsonb))
		RETURNING request_id
	`, candidatePersonID, employerID, string(claim)).Scan(&requestID); err != nil {
		return "", err
	}

	actor := candidatePersonID
	_ = insertEvent(ctx, tx, requestID, "candidate", &actor, "DRAFT_CREATED", nil, ptr("DRAFT"), map[string]any{
		"employer_id": employerID,
	})
	return requestID, nil
}

func (r *Repo) CandidateUpdateDraft(ctx context.Context, tx pgx.Tx, requestID, candidatePersonID string, expectedVersion int, claim json.RawMessage) error {
	ct, err := tx.Exec(ctx, `
		UPDATE evt_requests
		SET claim_snapshot = $1::jsonb,
		    version = version + 1
		WHERE request_id = $2
		  AND candidate_personid = $3
		  AND status = 'DRAFT'
		  AND version = $4
	`, string(claim), requestID, candidatePersonID, expectedVersion)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return db.ErrConflict
	}
	actor := candidatePersonID
	return insertEvent(ctx, tx, requestID, "candidate", &actor, "DRAFT_UPDATED", ptr("DRAFT"), ptr("DRAFT"), map[string]any{
		"expected_version": expectedVersion,
	})
}

	// CandidateList returns the requestor's requests in descending updated order.
	// Minimal list shape only (matches RequestorListRow JSON contract).
	func (r *Repo) CandidateList(ctx context.Context, tx pgx.Tx, candidatePersonID string, limit int) ([]RequestorListRow, error) {
		if limit <= 0 || limit > 200 {
			limit = 100
		}

		const q = `
	SELECT
	request_id,
	status,
	COALESCE(claim_snapshot, '{}'::jsonb) AS claim_snapshot,
	created_at,
	updated_at
	FROM evt_requests
	WHERE candidate_personid = $1
	ORDER BY updated_at DESC
	LIMIT $2
	`

	rows, err := tx.Query(ctx, q, candidatePersonID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]RequestorListRow, 0, 16)

	for rows.Next() {
		var (
			requestID string
			status    string
			snap      []byte
			createdAt time.Time
			updatedAt time.Time
		)

		if err := rows.Scan(&requestID, &status, &snap, &createdAt, &updatedAt); err != nil {
			return nil, err
		}

		out = append(out, RequestorListRow{
			RequestID:     requestID,
			Status:        status,
			ClaimSnapshot: json.RawMessage(snap),
			CreatedAt:     createdAt.UTC().Format(time.RFC3339Nano),
			UpdatedAt:     updatedAt.UTC().Format(time.RFC3339Nano),
		})
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return out, nil
}

	// CandidateGet returns a single request owned by the candidate.
	// Shape matches RequestorGetResp (adds version).
	func (r *Repo) CandidateGet(
		ctx context.Context,
		tx pgx.Tx,
		requestID string,
		candidatePersonID string,
	) (RequestorGetResp, error) {

		const q = `
	SELECT
	request_id,
	status,
	COALESCE(claim_snapshot, '{}'::jsonb) AS claim_snapshot,
	created_at,
	updated_at,
	version
	FROM evt_requests
	WHERE request_id = $1
	AND candidate_personid = $2
	LIMIT 1
	`

		var (
			id        string
			status    string
			snap      []byte
			createdAt time.Time
			updatedAt time.Time
			version   int
		)

		err := tx.QueryRow(ctx, q, requestID, candidatePersonID).Scan(
			&id,
			&status,
			&snap,
			&createdAt,
			&updatedAt,
			&version,
		)
		if err != nil {
			if err == pgx.ErrNoRows {
				return RequestorGetResp{}, db.ErrNotFound
			}
			return RequestorGetResp{}, err
		}

		return RequestorGetResp{
			RequestID:     id,
			Status:        status,
			ClaimSnapshot: json.RawMessage(snap),
			CreatedAt:     createdAt.UTC().Format(time.RFC3339Nano),
			UpdatedAt:     updatedAt.UTC().Format(time.RFC3339Nano),
			Version:       version,
		}, nil
	}



func (r *Repo) CandidateSubmit(ctx context.Context, tx pgx.Tx, requestID, candidatePersonID string) (finalStatus string, err error) {
	var curStatus string
	var employerID string

	err = tx.QueryRow(ctx, `
		SELECT status, employer_id
		FROM evt_requests
		WHERE request_id=$1 AND candidate_personid=$2
		FOR UPDATE
	`, requestID, candidatePersonID).Scan(&curStatus, &employerID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", db.ErrNotFound
		}
		return "", err
	}
	if curStatus != "DRAFT" {
		return "", db.ErrConflict
	}

	_, err = tx.Exec(ctx, `
		UPDATE evt_requests
		SET status='SUBMITTED', submitted_at=now(), version=version+1
		WHERE request_id=$1
	`, requestID)
	if err != nil {
		return "", err
	}

	actor := candidatePersonID
	_ = insertEvent(ctx, tx, requestID, "candidate", &actor, "REQUEST_SUBMITTED", ptr("DRAFT"), ptr("SUBMITTED"), map[string]any{})

	var dispatchMode string
	if err := tx.QueryRow(ctx, `
		SELECT dispatch_mode FROM evt_employer_config WHERE employer_id=$1
	`, employerID).Scan(&dispatchMode); err != nil {
		return "", err
	}

	if dispatchMode == "AUTO" {
		_, err = tx.Exec(ctx, `
			UPDATE evt_requests
			SET status='ATTESTATION_PENDING', attestation_dispatched_at=now(), version=version+1
			WHERE request_id=$1 AND status='SUBMITTED'
		`, requestID)
		if err != nil {
			return "", err
		}
		_ = insertEvent(ctx, tx, requestID, "cvera", nil, "ATTESTATION_DISPATCHED", ptr("SUBMITTED"), ptr("ATTESTATION_PENDING"), map[string]any{
			"dispatch_mode": "AUTO",
		})
		return "ATTESTATION_PENDING", nil
	}

	_ = insertEvent(ctx, tx, requestID, "cvera", nil, "DISPATCH_GUARD_BLOCKED", ptr("SUBMITTED"), ptr("SUBMITTED"), map[string]any{
		"dispatch_mode": "MANUAL",
	})
	return "SUBMITTED", nil
}

func (r *Repo) CandidateCancel(ctx context.Context, tx pgx.Tx, requestID, candidatePersonID string) error {
	var curStatus string
	err := tx.QueryRow(ctx, `
		SELECT status
		FROM evt_requests
		WHERE request_id=$1 AND candidate_personid=$2
		FOR UPDATE
	`, requestID, candidatePersonID).Scan(&curStatus)
	if err != nil {
		if err == pgx.ErrNoRows {
			return db.ErrNotFound
		}
		return err
	}
	if curStatus != "DRAFT" && curStatus != "SUBMITTED" {
		return db.ErrConflict
	}

	_, err = tx.Exec(ctx, `
		UPDATE evt_requests
		SET status='CANCELED', canceled_at=now(), version=version+1
		WHERE request_id=$1
	`, requestID)
	if err != nil {
		return err
	}

	actor := candidatePersonID
	return insertEvent(ctx, tx, requestID, "candidate", &actor, "REQUEST_CANCELED", &curStatus, ptr("CANCELED"), map[string]any{})
}

/* ---------------- Employer HR writes ---------------- */

func (r *Repo) EmployerAttestServerSigned(
	ctx context.Context,
	tx pgx.Tx,
	requestID, employerID, employerHRPersonID, responseType string,
	responseBody json.RawMessage,
) (AttestResult, error) {

	var curStatus string
	err := tx.QueryRow(ctx, `
		SELECT status
		FROM evt_requests
		WHERE request_id=$1 AND employer_id=$2
		FOR UPDATE
	`, requestID, employerID).Scan(&curStatus)
	if err != nil {
		if err == pgx.ErrNoRows {
			return AttestResult{}, db.ErrNotFound
		}
		return AttestResult{}, err
	}

	if curStatus != "ATTESTATION_PENDING" {
		return AttestResult{}, db.ErrConflict
	}

	// Decide new status based on enum (matches DB employer_response_type)
	toStatus := "ATTESTED"
	if responseType == "REJECTED_NO_RECORD" || responseType == "REJECTED_POLICY" {
		toStatus = "REJECTED"
	}

	// Normalize body
	if len(responseBody) == 0 {
		responseBody = json.RawMessage(`{}`)
	}

	// Only generate JWS for ATTESTED (FULL_MATCH, PARTIAL_MATCH)
	var (
		jws     string
		kid     string
		jwsBytes int
		present bool
	)

	if toStatus == "ATTESTED" {
		if responseType != "FULL_MATCH" && responseType != "PARTIAL_MATCH" {
			// Anything else trying to become ATTESTED is invalid
			return AttestResult{}, db.ErrConflict
		}
		if r.Signer == nil {
			return AttestResult{}, errSignerNotConfigured()
		}

		// IMPORTANT: The signer sets header kid that matches trust/jwks.json
		jws, kid, err = r.Signer.SignAttestationJWS(AttestationClaims{
			RequestID:      requestID,
			EmployerID:     employerID,
			HRPersonID:     employerHRPersonID,
			ResponseType:   responseType,
			ResponseBody:   responseBody,
			IssuedAtUnix:   time.Now().UTC().Unix(),
		})
		if err != nil {
			return AttestResult{}, err
		}
		jwsBytes = len(jws)
		present = true

		_, err = tx.Exec(ctx, `
			UPDATE evt_requests
			SET status='ATTESTED',
			    employer_response_type=$1::employer_response_type,
			    employer_response=COALESCE($2::jsonb, '{}'::jsonb),
			    attestation_jws=$3,
			    attested_at=now(),
			    version=version+1
			WHERE request_id=$4
		`, responseType, string(responseBody), jws, requestID)
		if err != nil {
			return AttestResult{}, err
		}

	} else {
		// REJECTED path: no JWS stored
		_, err = tx.Exec(ctx, `
			UPDATE evt_requests
			SET status='REJECTED',
			    employer_response_type=$1::employer_response_type,
			    employer_response=COALESCE($2::jsonb, '{}'::jsonb),
			    attested_at=now(),
			    version=version+1
			WHERE request_id=$3
		`, responseType, string(responseBody), requestID)
		if err != nil {
			return AttestResult{}, err
		}
	}

	actor := employerHRPersonID
	_ = insertEvent(ctx, tx, requestID, "employer_hr", &actor, "HR_ATTESTED", &curStatus, &toStatus, map[string]any{
		"response_type": responseType,
		"attestation_present": present,
		"kid": kid,
	})

	return AttestResult{
		Status:             toStatus,
		AttestationPresent: present,
		KID:                kid,
		JWSBytes:           jwsBytes,
	}, nil
}

func (r *Repo) EmployerList(ctx context.Context, tx pgx.Tx, employerID string, limit int) ([]HRQueueRow, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}

	const q = `
SELECT
  request_id,
  status,
  COALESCE(claim_snapshot, '{}'::jsonb) AS claim_snapshot,
  created_at,
  updated_at
FROM evt_requests
WHERE employer_id = $1
  AND status = 'ATTESTATION_PENDING'
ORDER BY updated_at DESC
LIMIT $2
`
	rows, err := tx.Query(ctx, q, employerID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]HRQueueRow, 0, 16)
	for rows.Next() {
		var (
			requestID string
			status    string
			snap      []byte
			createdAt time.Time
			updatedAt time.Time
		)

		if err := rows.Scan(&requestID, &status, &snap, &createdAt, &updatedAt); err != nil {
			return nil, err
		}

		out = append(out, HRQueueRow{
			RequestID:     requestID,
			Status:        status,
			ClaimSnapshot: json.RawMessage(snap),
			CreatedAt:     createdAt.UTC().Format(time.RFC3339Nano),
			UpdatedAt:     updatedAt.UTC().Format(time.RFC3339Nano),
		})
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

type EmployerGetRow struct {
	RequestID     string
	Status        string
	ClaimSnapshot json.RawMessage
	CreatedAt     string
	UpdatedAt     string
	Version       int
	EmployerResponseType *string
}

func (r *Repo) EmployerGet(ctx context.Context, tx pgx.Tx, requestID string, employerID string) (EmployerGetRow, error) {
	const q = `
SELECT
  request_id,
  status,
  COALESCE(claim_snapshot, '{}'::jsonb) AS claim_snapshot,
  created_at,
  updated_at,
  version,
employer_response_type
FROM evt_requests
WHERE request_id = $1
  AND employer_id = $2
LIMIT 1
`
	var (
		id        string
		status    string
		snap      []byte
		createdAt time.Time
		updatedAt time.Time
		version   int
	)

	if err := tx.QueryRow(ctx, q, requestID, employerID).Scan(&id, &status, &snap, &createdAt, &updatedAt, &version); err != nil {
		if err == pgx.ErrNoRows {
			return EmployerGetRow{}, db.ErrNotFound
		}
		return EmployerGetRow{}, err
	}

	return EmployerGetRow{
		RequestID:     id,
		Status:        status,
		ClaimSnapshot: json.RawMessage(snap),
		CreatedAt:     createdAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:     updatedAt.UTC().Format(time.RFC3339Nano),
		Version:       version,
	}, nil
}

/* ---------------- Internal Cvera writes ---------------- */

func (r *Repo) InternalVerify(ctx context.Context, tx pgx.Tx, requestID string, trustResult string, trustFlags json.RawMessage, trustSummary string, evtTokenJWS string) error {
	var curStatus string
	err := tx.QueryRow(ctx, `
		SELECT status FROM evt_requests WHERE request_id=$1 FOR UPDATE
	`, requestID).Scan(&curStatus)
	if err != nil {
		if err == pgx.ErrNoRows {
			return db.ErrNotFound
		}
		return err
	}
	if curStatus != "ATTESTED" {
		return db.ErrConflict
	}

	toStatus := "VERIFIED"
	if trustResult == "UNVERIFIED" {
		toStatus = "UNVERIFIED"
	}
	if len(trustFlags) == 0 {
		trustFlags = json.RawMessage(`[]`)
	}

	_, err = tx.Exec(ctx, `
		UPDATE evt_requests
		SET status=$1::evt_request_status,
		    trust_result=$2::trust_result_type,
		    trust_flags=$3::jsonb,
		    trust_summary=$4,
		    evt_token_jws=COALESCE($5, evt_token_jws),
		    verified_at=now(),
		    version=version+1
		WHERE request_id=$6
	`, toStatus, trustResult, string(trustFlags), trustSummary, evtTokenJWS, requestID)
	if err != nil {
		return err
	}

	return insertEvent(ctx, tx, requestID, "cvera", nil, "TRUST_EVALUATED", &curStatus, &toStatus, map[string]any{
		"trust_result": trustResult,
	})
}

func (r *Repo) InternalClose(ctx context.Context, tx pgx.Tx, requestID string) error {
	var curStatus string
	err := tx.QueryRow(ctx, `
		SELECT status FROM evt_requests WHERE request_id=$1 FOR UPDATE
	`, requestID).Scan(&curStatus)
	if err != nil {
		if err == pgx.ErrNoRows {
			return db.ErrNotFound
		}
		return err
	}
	if curStatus != "CONSUMED" {
		return db.ErrConflict
	}

	_, err = tx.Exec(ctx, `
		UPDATE evt_requests
		SET status='CLOSED', closed_at=now(), version=version+1
		WHERE request_id=$1
	`, requestID)
	if err != nil {
		return err
	}

	return insertEvent(ctx, tx, requestID, "cvera", nil, "REQUEST_CLOSED", &curStatus, ptr("CLOSED"), map[string]any{})
}

/* ---------------- Recruiter writes ---------------- */

func (r *Repo) RecruiterConsume(ctx context.Context, tx pgx.Tx, requestID, recruiterPersonID, userAgent, ipHash string) error {
	var curStatus string
	err := tx.QueryRow(ctx, `
		SELECT status FROM evt_requests WHERE request_id=$1 FOR UPDATE
	`, requestID).Scan(&curStatus)
	if err != nil {
		if err == pgx.ErrNoRows {
			return db.ErrNotFound
		}
		return err
	}
	if curStatus != "VERIFIED" && curStatus != "UNVERIFIED" && curStatus != "CONSUMED" {
		return db.ErrConflict
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO evt_token_consumptions (request_id, recruiter_personid, user_agent, ip_hash)
		VALUES ($1,$2,$3,$4)
	`, requestID, recruiterPersonID, userAgent, ipHash)
	if err != nil {
		return err
	}

	if curStatus != "CONSUMED" {
		_, err = tx.Exec(ctx, `
			UPDATE evt_requests
			SET status='CONSUMED', consumed_at=now(), version=version+1
			WHERE request_id=$1
		`, requestID)
		if err != nil {
			return err
		}
		actor := recruiterPersonID
		_ = insertEvent(ctx, tx, requestID, "recruiter", &actor, "TOKEN_CONSUMED", &curStatus, ptr("CONSUMED"), map[string]any{})
	}

	return nil
}
