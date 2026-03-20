package evt

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
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

type RecruiterCompanyOption struct {
	CompanyID string
	Name      string
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
		ert 	*string
	)

	if err := tx.QueryRow(ctx, q, requestID, employerID).Scan(&id, &status, &snap, &createdAt, &updatedAt, &version, &ert); err != nil {
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
		EmployerResponseType: ert,
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

func (r *Repo) RecruiterCompanyOptions(
	ctx context.Context,
	tx pgx.Tx,
	recruiterPersonID string,
) ([]RecruiterCompanyOption, error) {

	// MVP: companies = employer_id values that have requests in states
	// a recruiter may reasonably see. Adjust later if you add a true company table.
	const q = `
SELECT DISTINCT
  employer_id
FROM evt_requests
WHERE employer_id IS NOT NULL
  AND employer_id <> ''
  AND status IN ('VERIFIED','UNVERIFIED','CONSUMED','CLOSED','ATTESTED','REJECTED')
ORDER BY employer_id ASC
`

	rows, err := tx.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]RecruiterCompanyOption, 0, 16)
	for rows.Next() {
		var companyID string
		if err := rows.Scan(&companyID); err != nil {
			return nil, err
		}
		out = append(out, RecruiterCompanyOption{
			CompanyID: companyID,
			Name:      companyID, // MVP placeholder
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	_ = recruiterPersonID // reserved for future scoping by recruiter/org
	return out, nil
}

func (r *Repo) RecruiterCandidateList(
	ctx context.Context,
	tx pgx.Tx,
	recruiterPersonID string,
	search string,
	trustMode RecruiterTrustMode,
	signatures []SignatureBadge,
	companyIDs []string,
	limit int,
	cursor *recruiterCursor,
) ([]CandidateRowSnapshot, *string, error) {

	if limit <= 0 || limit > 100 {
		limit = 25
	}

	// Normalize signature filter to []string for SQL ANY()
	sigVals := make([]string, 0, len(signatures))
	for _, s := range signatures {
		sigVals = append(sigVals, string(s))
	}

	// Build SQL dynamically (only add clauses that apply)
	// NOTE: We use issuer_id = employer_id for MVP.
	var sb strings.Builder
	args := make([]any, 0, 10)
	arg := func(v any) string {
		args = append(args, v)
		return "$" + strconv.Itoa(len(args))
	}

	sb.WriteString(`
	WITH base AS (
	SELECT
		er.request_id,
		er.candidate_personid,
		er.employer_id,
		COALESCE(er.claim_snapshot, '{}'::jsonb) AS claim_snapshot,
		er.attestation_jws,
		er.updated_at,
		er.status,
		er.trust_result,

		CASE
		WHEN er.attestation_jws IS NOT NULL AND btrim(er.attestation_jws) <> '' THEN 'verified'
		ELSE 'unknown'
		END AS signature_status,

		CASE
		WHEN rti.trust_level = 'trusted' THEN 'trusted'
		WHEN rti.trust_level = 'untrusted' THEN 'untrusted'
		ELSE 'unknown'
		END AS trust_status,

		CASE
		WHEN er.status = 'VERIFIED' THEN 'verified'
		WHEN er.status IN ('UNVERIFIED', 'REJECTED') THEN 'unverified'
		WHEN er.status IN ('DRAFT', 'SUBMITTED', 'ATTESTATION_PENDING', 'ATTESTED') THEN 'pending'
		WHEN er.status IN ('CONSUMED', 'CLOSED') AND er.trust_result IN ('VERIFIED', 'VERIFIED_WITH_FLAGS') THEN 'verified'
		WHEN er.status IN ('CONSUMED', 'CLOSED') AND er.trust_result = 'UNVERIFIED' THEN 'unverified'
		ELSE 'unknown'
		END AS verification_state

	FROM evt_requests er
	LEFT JOIN recruiter_trusted_issuers rti
		ON rti.recruiter_personid = $1
	AND rti.issuer_id = er.employer_id
	WHERE er.status IN (
		'DRAFT',
		'SUBMITTED',
		'ATTESTATION_PENDING',
		'ATTESTED',
		'VERIFIED',
		'UNVERIFIED',
		'REJECTED',
		'CONSUMED',
		'CLOSED'
	)
	),
	ranked AS (
	SELECT
		*,
		ROW_NUMBER() OVER (
		PARTITION BY candidate_personid
		ORDER BY updated_at DESC, request_id DESC
		) AS rn
	FROM base
	),
	picked AS (
	SELECT
		request_id,
		candidate_personid,
		employer_id,
		claim_snapshot,
		signature_status,
		trust_status,
		verification_state,
		trust_result,
		status,
		updated_at
	FROM ranked
	WHERE rn = 1
	)
	SELECT
	request_id,
	candidate_personid,
	employer_id,
	claim_snapshot,
	signature_status,
	trust_status,
	verification_state,
	trust_result,
	status,
	updated_at
	FROM picked
	WHERE 1=1
	`)

	// Cursor pagination: (updated_at, request_id) < (cursor.updated_at, cursor.request_id)
		if cursor != nil {
			sb.WriteString(`
		AND (updated_at, request_id) < (` + arg(cursor.UpdatedAt) + `::timestamptz, ` + arg(cursor.RequestID) + `)
		`)
	}

	// signature_status filter
	sb.WriteString(`
	AND signature_status = ANY(` + arg(sigVals) + `::text[])
	`)

	// company_ids filter (maps to employer_id)
	if len(companyIDs) > 0 {
		sb.WriteString(`
	AND employer_id = ANY(` + arg(companyIDs) + `::text[])
	`)
	}

	// trust_mode filter
	switch trustMode {
	case TrustTrustedOnly:
		sb.WriteString(`
	AND trust_level = 'trusted'
	`)
	case TrustIncludeUntrust:
		// include both trusted + untrusted, exclude unknown
		sb.WriteString(`
	AND trust_level IS NOT NULL
	`)
	case TrustAny:
		// no clause
	default:
		// safety: behave like "any"
	}

// search filter over claim_snapshot
// We keep this schema-light: look for common JSON paths but tolerate missing.
search = strings.TrimSpace(search)
if search != "" {
	like := "%" + search + "%"
	sb.WriteString(`
  AND (
    COALESCE(claim_snapshot #>> '{subject,full_name}', '') ILIKE ` + arg(like) + `
    OR COALESCE(claim_snapshot #>> '{subject,employee_id}', '') ILIKE ` + arg(like) + `
    OR COALESCE(claim_snapshot #>> '{primary_employment,issuer_name}', '') ILIKE ` + arg(like) + `
    OR COALESCE(claim_snapshot #>> '{primary_employment,title}', '') ILIKE ` + arg(like) + `
    OR COALESCE(employer_id, '') ILIKE ` + arg(like) + `
  )
`)
	}

// Stable sort
sb.WriteString(`
ORDER BY updated_at DESC, request_id DESC
LIMIT ` + arg(limit+1) + `
`)

	rows, err := tx.Query(ctx, sb.String(), args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	type dbRow struct {
		RequestID         string
		CandidatePerson   string
		EmployerID        string
		ClaimSnapshot     []byte
		SignatureStatus   string
		TrustStatus       string
		VerificationState string
		TrustResult       *string
		Status            string
		UpdatedAt         time.Time
	}

	dbRows := make([]dbRow, 0, limit+1)

	for rows.Next() {
		var r0 dbRow
		if err := rows.Scan(
		&r0.RequestID,
		&r0.CandidatePerson,
		&r0.EmployerID,
		&r0.ClaimSnapshot,
		&r0.SignatureStatus,
		&r0.TrustStatus,
		&r0.VerificationState,
		&r0.TrustResult,
		&r0.Status,
		&r0.UpdatedAt,
	); err != nil {
		return nil, nil, err
	}
		dbRows = append(dbRows, r0)
	}

	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	// Next cursor if we fetched limit+1
	var nextCursor *string
	if len(dbRows) > limit {
		last := dbRows[limit-1] 
		cur := recruiterCursor{UpdatedAt: last.UpdatedAt.UTC(), RequestID: last.RequestID}
		enc, err := encodeCursor(cur)
		if err != nil {
			return nil, nil, err
		}
		nextCursor = &enc

		dbRows = dbRows[:limit]
	}

	// Convert to CandidateRowSnapshot (your UI contract shape)
	items := make([]CandidateRowSnapshot, 0, len(dbRows))
	for _, r0 := range dbRows {
		var snap map[string]any
		_ = json.Unmarshal(r0.ClaimSnapshot, &snap) // tolerant

		// Extract known fields safely from JSONB (no panics)
		// We'll also use the SQL JSON paths in filters, but for response we parse once.
		fullName := ""
		var employeeID *string

		issuerName := r0.EmployerID
		title := ""
		startDate := ""
		var endDate *string

		// best-effort decode paths:
		// subject.full_name, subject.employee_id, primary_employment.*
		if subj, ok := snap["subject"].(map[string]any); ok {
			if v, ok := subj["full_name"].(string); ok {
				fullName = v
			}
			if v, ok := subj["employee_id"].(string); ok && strings.TrimSpace(v) != "" {
				tmp := v
				employeeID = &tmp
			}
		}
		if pe, ok := snap["primary_employment"].(map[string]any); ok {
			if v, ok := pe["issuer_name"].(string); ok && strings.TrimSpace(v) != "" {
				issuerName = v
			}
			if v, ok := pe["title"].(string); ok {
				title = v
			}
			if v, ok := pe["start_date"].(string); ok {
				startDate = v
			}
			if v, ok := pe["end_date"].(string); ok && strings.TrimSpace(v) != "" {
				tmp := v
				endDate = &tmp
			}
		}

		// badges
		sig := SignatureBadge(r0.SignatureStatus)
		if sig != SigVerified && sig != SigInvalid && sig != SigUnknown {
			sig = SigUnknown
		}

		trust := TrustUnknown
		switch strings.ToLower(strings.TrimSpace(r0.TrustStatus)) {
		case "trusted":
			trust = TrustTrusted
		case "untrusted":
			trust = TrustUntrusted
		default:
			trust = TrustUnknown
		}

		verification := &VerificationSummary{}
		switch r0.VerificationState {
		case "verified":
			verification.State = VerificationVerified
		case "unverified":
			verification.State = VerificationUnverified
		case "pending":
			verification.State = VerificationPending
		default:
			verification.State = VerificationUnknown
		}

		if r0.TrustResult != nil && strings.TrimSpace(*r0.TrustResult) != "" {
			tr := TrustResult(strings.TrimSpace(*r0.TrustResult))
			switch tr {
			case TrustResultVerified, TrustResultVerifiedWithFlags, TrustResultUnverified:
				verification.TrustResult = &tr
			}
		}

		if strings.TrimSpace(r0.Status) != "" {
			rs := RequestStatus(strings.TrimSpace(r0.Status))
			switch rs {
			case RequestStatusDraft,
				RequestStatusSubmitted,
				RequestStatusAttestationPending,
				RequestStatusAttested,
				RequestStatusVerified,
				RequestStatusUnverified,
				RequestStatusRejected,
				RequestStatusConsumed,
				RequestStatusClosed:
				verification.RequestStatus = &rs
			}
		}

		var out CandidateRowSnapshot
		out.CandidateID = r0.CandidatePerson
		out.Subject.FullName = fullName
		out.Subject.EmployeeID = employeeID
		out.PrimaryEmployment.IssuerName = issuerName
		out.PrimaryEmployment.Title = title
		out.PrimaryEmployment.StartDate = startDate
		out.PrimaryEmployment.EndDate = endDate
		out.PrimaryEVT.EVTID = r0.RequestID
		out.Badges.Signature = sig
		out.Badges.Trust = trust
		out.Verification = verification
		out.UpdatedAt = r0.UpdatedAt.UTC().Format(time.RFC3339Nano)

		items = append(items, out)
	}

	return items, nextCursor, nil
}

