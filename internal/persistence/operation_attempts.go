package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gdiop88/RL_trader_infrastructure/internal/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrOperationAttemptNotFound = errors.New("operation attempt not found")
	ErrOperationAttemptConflict = errors.New("operation attempt conflict")
)

type OperationAttempt struct {
	ID               string
	OperationID      string
	AttemptNumber    int32
	Status           string
	WorkerReference  *string
	StartedAt        *time.Time
	FinishedAt       *time.Time
	ErrorCode        *string
	ErrorMessage     *string
	ResponseMetadata json.RawMessage
}

type CreateOperationAttemptInput struct {
	OperationID      string
	AttemptNumber    int32
	Status           string
	WorkerReference  *string
	StartedAt        *time.Time
	FinishedAt       *time.Time
	ErrorCode        *string
	ErrorMessage     *string
	ResponseMetadata json.RawMessage
}

type UpdateOperationAttemptInput struct {
	Status           string
	WorkerReference  *string
	StartedAt        *time.Time
	FinishedAt       *time.Time
	ErrorCode        *string
	ErrorMessage     *string
	ResponseMetadata json.RawMessage
}

type ListOperationAttemptsOptions struct {
	OperationID string
	Status      string
	Limit       int
	Offset      int
}

type OperationAttemptRepository struct {
	pool *pgxpool.Pool
}

func NewOperationAttemptRepository(
	db *database.Database,
) (*OperationAttemptRepository, error) {
	if db == nil || db.Pool() == nil {
		return nil, errors.New("database connection is required")
	}

	return &OperationAttemptRepository{
		pool: db.Pool(),
	}, nil
}

func validateAttemptStatus(status string) error {
	if strings.TrimSpace(status) == "" ||
		len(status) > 50 ||
		strings.ContainsRune(status, '\x00') {
		return errors.New("status must contain 1 to 50 valid characters")
	}

	return nil
}

func validateAttemptMetadata(data json.RawMessage) error {
	if len(data) == 0 {
		return nil
	}

	if !json.Valid(data) {
		return errors.New("response_metadata contains invalid JSON")
	}

	// The PostgreSQL JSONB column accepts any valid JSON value.
	return nil
}

func validateAttemptFields(
	status string,
	workerReference *string,
	startedAt *time.Time,
	finishedAt *time.Time,
	errorCode *string,
	errorMessage *string,
	metadata json.RawMessage,
) error {
	if err := validateAttemptStatus(status); err != nil {
		return err
	}

	if err := validateOptionalString(
		"worker_reference", workerReference, 255,
	); err != nil {
		return err
	}

	if workerReference != nil &&
		strings.ContainsRune(*workerReference, '\x00') {
		return errors.New("worker_reference contains a null character")
	}

	if err := validateOptionalString(
		"error_code", errorCode, 100,
	); err != nil {
		return err
	}

	if errorCode != nil &&
		strings.ContainsRune(*errorCode, '\x00') {
		return errors.New("error_code contains a null character")
	}

	// PostgreSQL TEXT has no VARCHAR length limit.
	if errorMessage != nil &&
		strings.ContainsRune(*errorMessage, '\x00') {
		return errors.New("error_message contains a null character")
	}

	if startedAt != nil && startedAt.IsZero() {
		return errors.New("started_at cannot be zero")
	}

	if finishedAt != nil && finishedAt.IsZero() {
		return errors.New("finished_at cannot be zero")
	}

	if err := validateAttemptMetadata(metadata); err != nil {
		return err
	}

	return nil
}

func (in CreateOperationAttemptInput) Validate() error {
	if !validUUID(in.OperationID) {
		return errors.New("operation_id must be a valid UUID")
	}

	if in.AttemptNumber <= 0 {
		return errors.New("attempt_number must be greater than zero")
	}

	return validateAttemptFields(
		in.Status,
		in.WorkerReference,
		in.StartedAt,
		in.FinishedAt,
		in.ErrorCode,
		in.ErrorMessage,
		in.ResponseMetadata,
	)
}

func (in UpdateOperationAttemptInput) Validate() error {
	return validateAttemptFields(
		in.Status,
		in.WorkerReference,
		in.StartedAt,
		in.FinishedAt,
		in.ErrorCode,
		in.ErrorMessage,
		in.ResponseMetadata,
	)
}

const operationAttemptColumns = `
	id,
	operation_id,
	attempt_number,
	status,
	worker_reference,
	started_at,
	finished_at,
	error_code,
	error_message,
	response_metadata
`

func scanOperationAttempt(
	row pgx.Row,
) (*OperationAttempt, error) {
	var attempt OperationAttempt

	err := row.Scan(
		&attempt.ID,
		&attempt.OperationID,
		&attempt.AttemptNumber,
		&attempt.Status,
		&attempt.WorkerReference,
		&attempt.StartedAt,
		&attempt.FinishedAt,
		&attempt.ErrorCode,
		&attempt.ErrorMessage,
		&attempt.ResponseMetadata,
	)

	if err != nil {
		return nil, mapOperationAttemptError(err)
	}

	return &attempt, nil
}

func nullableAttemptMetadata(data json.RawMessage) any {
	if len(data) == 0 {
		return nil
	}

	return string(data)
}

func (r *OperationAttemptRepository) Create(
	ctx context.Context,
	input CreateOperationAttemptInput,
) (*OperationAttempt, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}

	query := `
		INSERT INTO operation_attempts (
			operation_id,
			attempt_number,
			status,
			worker_reference,
			started_at,
			finished_at,
			error_code,
			error_message,
			response_metadata
		)
		VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb
		)
		RETURNING ` + operationAttemptColumns

	return scanOperationAttempt(
		r.pool.QueryRow(
			ctx,
			query,
			input.OperationID,
			input.AttemptNumber,
			input.Status,
			input.WorkerReference,
			input.StartedAt,
			input.FinishedAt,
			input.ErrorCode,
			input.ErrorMessage,
			nullableAttemptMetadata(input.ResponseMetadata),
		),
	)
}

func (r *OperationAttemptRepository) GetByID(
	ctx context.Context,
	id string,
) (*OperationAttempt, error) {
	if !validUUID(id) {
		return nil, errors.New("id must be a valid UUID")
	}

	query := `
		SELECT ` + operationAttemptColumns + `
		FROM operation_attempts
		WHERE id = $1
	`

	return scanOperationAttempt(
		r.pool.QueryRow(ctx, query, id),
	)
}

func (r *OperationAttemptRepository) GetByAttemptNumber(
	ctx context.Context,
	operationID string,
	attemptNumber int32,
) (*OperationAttempt, error) {
	if !validUUID(operationID) {
		return nil, errors.New("operation_id must be a valid UUID")
	}

	if attemptNumber <= 0 {
		return nil, errors.New("attempt_number must be greater than zero")
	}

	query := `
		SELECT ` + operationAttemptColumns + `
		FROM operation_attempts
		WHERE operation_id = $1
		  AND attempt_number = $2
	`

	return scanOperationAttempt(
		r.pool.QueryRow(
			ctx,
			query,
			operationID,
			attemptNumber,
		),
	)
}

func (r *OperationAttemptRepository) List(
	ctx context.Context,
	options ListOperationAttemptsOptions,
) ([]OperationAttempt, error) {
	if options.OperationID != "" &&
		!validUUID(options.OperationID) {
		return nil, errors.New("operation_id must be a valid UUID")
	}

	if options.Status != "" {
		if err := validateAttemptStatus(options.Status); err != nil {
			return nil, err
		}
	}

	if options.Limit < 0 || options.Offset < 0 {
		return nil, errors.New(
			"limit and offset cannot be negative",
		)
	}

	limit := options.Limit
	if limit == 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}

	var operationID any
	var status any

	if options.OperationID != "" {
		operationID = options.OperationID
	}
	if options.Status != "" {
		status = options.Status
	}

	query := `
		SELECT ` + operationAttemptColumns + `
		FROM operation_attempts
		WHERE
			($1::uuid IS NULL OR operation_id = $1::uuid)
			AND ($2::text IS NULL OR status = $2::text)
		ORDER BY
			operation_id ASC,
			attempt_number ASC,
			id ASC
		LIMIT $3
		OFFSET $4
	`

	rows, err := r.pool.Query(
		ctx,
		query,
		operationID,
		status,
		limit,
		options.Offset,
	)
	if err != nil {
		return nil, mapOperationAttemptError(err)
	}
	defer rows.Close()

	attempts := make([]OperationAttempt, 0)

	for rows.Next() {
		attempt, err := scanOperationAttempt(rows)
		if err != nil {
			return nil, err
		}

		attempts = append(attempts, *attempt)
	}

	if err := rows.Err(); err != nil {
		return nil, mapOperationAttemptError(err)
	}

	return attempts, nil
}

func (r *OperationAttemptRepository) Update(
	ctx context.Context,
	id string,
	input UpdateOperationAttemptInput,
) (*OperationAttempt, error) {
	if !validUUID(id) {
		return nil, errors.New("id must be a valid UUID")
	}

	if err := input.Validate(); err != nil {
		return nil, err
	}

	query := `
		UPDATE operation_attempts
		SET
			status = $2,
			worker_reference = $3,
			started_at = $4,
			finished_at = $5,
			error_code = $6,
			error_message = $7,
			response_metadata = $8::jsonb
		WHERE id = $1
		RETURNING ` + operationAttemptColumns

	return scanOperationAttempt(
		r.pool.QueryRow(
			ctx,
			query,
			id,
			input.Status,
			input.WorkerReference,
			input.StartedAt,
			input.FinishedAt,
			input.ErrorCode,
			input.ErrorMessage,
			nullableAttemptMetadata(input.ResponseMetadata),
		),
	)
}

func (r *OperationAttemptRepository) Delete(
	ctx context.Context,
	id string,
) error {
	if !validUUID(id) {
		return errors.New("id must be a valid UUID")
	}

	tag, err := r.pool.Exec(
		ctx,
		`DELETE FROM operation_attempts WHERE id = $1`,
		id,
	)
	if err != nil {
		return mapOperationAttemptError(err)
	}

	if tag.RowsAffected() == 0 {
		return ErrOperationAttemptNotFound
	}

	return nil
}

func mapOperationAttemptError(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, pgx.ErrNoRows) {
		return ErrOperationAttemptNotFound
	}

	var pgErr *pgconn.PgError

	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return fmt.Errorf(
				"%w: %v",
				ErrOperationAttemptConflict,
				err,
			)

		case "23503":
			return fmt.Errorf(
				"operation attempt foreign-key constraint: %w",
				err,
			)

		case "23514":
			return fmt.Errorf(
				"operation attempt check constraint: %w",
				err,
			)

		case "23502":
			return fmt.Errorf(
				"operation attempt required field: %w",
				err,
			)
		}
	}

	return err
}
