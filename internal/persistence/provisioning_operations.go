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
	ErrProvisioningOperationNotFound = errors.New(
		"provisioning operation not found",
	)
	ErrProvisioningOperationConflict = errors.New(
		"provisioning operation conflict",
	)
)

type ProvisioningOperation struct {
	ID                string
	InstanceID        *string
	OperationType     string
	Status            string
	RequestedBy       *string
	Provider          string
	Region            *string
	RequestPayload    json.RawMessage
	ResponsePayload   json.RawMessage
	ErrorMessage      *string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	StartedAt         *time.Time
	CompletedAt       *time.Time
	RetryCount        int32
	MaxRetries        int32
	NextRetryAt       *time.Time
	IdempotencyKey    *string
	CorrelationID     *string
	TimeoutAt         *time.Time
	ProjectID         *string
	TemplateID        *string
	ProviderAccountID *string
}

type CreateProvisioningOperationInput struct {
	InstanceID        *string
	OperationType     string
	RequestedBy       *string
	Provider          string
	Region            *string
	RequestPayload    json.RawMessage
	MaxRetries        *int32
	IdempotencyKey    *string
	CorrelationID     *string
	TimeoutAt         *time.Time
	ProjectID         *string
	TemplateID        *string
	ProviderAccountID *string
}

type ListProvisioningOperationsOptions struct {
	InstanceID        *string
	ProjectID         *string
	TemplateID        *string
	ProviderAccountID *string
	CorrelationID     *string
	OperationType     string
	Status            string
	Provider          string
	Limit             int
	Offset            int
}

type UpdateProvisioningOperationStatusInput struct {
	Status          string
	ResponsePayload json.RawMessage
	ErrorMessage    *string
	StartedAt       *time.Time
	CompletedAt     *time.Time
	NextRetryAt     *time.Time
}

type ProvisioningOperationRepository struct {
	pool *pgxpool.Pool
}

func NewProvisioningOperationRepository(
	db *database.Database,
) (*ProvisioningOperationRepository, error) {
	if db == nil || db.Pool() == nil {
		return nil, errors.New("database connection is required")
	}

	return &ProvisioningOperationRepository{
		pool: db.Pool(),
	}, nil
}

func validProvisioningOperationType(value string) bool {
	switch value {
	case "create", "update", "delete", "start",
		"stop", "restart", "resize":
		return true
	default:
		return false
	}
}

func validProvisioningOperationStatus(value string) bool {
	switch value {
	case "pending", "running", "completed",
		"failed", "cancelled":
		return true
	default:
		return false
	}
}

func validateOperationUUID(name string, value *string) error {
	if value != nil && !validUUID(*value) {
		return fmt.Errorf("%s must be a valid UUID", name)
	}
	return nil
}

func validateOperationText(
	name string,
	value *string,
	max int,
) error {
	if value == nil {
		return nil
	}

	if strings.ContainsRune(*value, '\x00') {
		return fmt.Errorf("%s contains a null character", name)
	}

	if max > 0 && len(*value) > max {
		return fmt.Errorf("%s exceeds %d characters", name, max)
	}

	return nil
}

func validateOperationJSON(
	name string,
	value json.RawMessage,
	required bool,
) error {
	if len(value) == 0 {
		if required {
			return fmt.Errorf("%s is required", name)
		}
		return nil
	}

	var decoded any
	if err := json.Unmarshal(value, &decoded); err != nil {
		return fmt.Errorf("%s contains invalid JSON: %w", name, err)
	}

	if _, ok := decoded.(map[string]any); !ok {
		return fmt.Errorf("%s must be a JSON object", name)
	}

	return nil
}

func (in CreateProvisioningOperationInput) Validate() error {
	if !validProvisioningOperationType(in.OperationType) {
		return errors.New("invalid operation_type")
	}

	if strings.TrimSpace(in.Provider) == "" ||
		len(in.Provider) > 50 ||
		strings.ContainsRune(in.Provider, '\x00') {
		return errors.New("provider must contain 1 to 50 valid characters")
	}

	for _, field := range []struct {
		name  string
		value *string
	}{
		{"instance_id", in.InstanceID},
		{"project_id", in.ProjectID},
		{"template_id", in.TemplateID},
		{"provider_account_id", in.ProviderAccountID},
		{"correlation_id", in.CorrelationID},
	} {
		if err := validateOperationUUID(
			field.name, field.value,
		); err != nil {
			return err
		}
	}

	for _, field := range []struct {
		name  string
		value *string
		max   int
	}{
		{"requested_by", in.RequestedBy, 255},
		{"region", in.Region, 100},
		{"idempotency_key", in.IdempotencyKey, 255},
	} {
		if err := validateOperationText(
			field.name, field.value, field.max,
		); err != nil {
			return err
		}
	}

	if err := validateOperationJSON(
		"request_payload", in.RequestPayload, false,
	); err != nil {
		return err
	}

	if in.MaxRetries != nil && *in.MaxRetries < 0 {
		return errors.New("max_retries cannot be negative")
	}

	return nil
}

func (in UpdateProvisioningOperationStatusInput) Validate() error {
	if !validProvisioningOperationStatus(in.Status) {
		return errors.New("invalid status")
	}

	if err := validateOperationJSON(
		"response_payload", in.ResponsePayload, false,
	); err != nil {
		return err
	}

	if err := validateOperationText(
		"error_message", in.ErrorMessage, 0,
	); err != nil {
		return err
	}

	if in.StartedAt != nil && in.CompletedAt != nil &&
		in.CompletedAt.Before(*in.StartedAt) {
		return errors.New(
			"completed_at cannot precede started_at",
		)
	}

	return nil
}

const provisioningOperationColumns = `
	id,
	instance_id,
	operation_type,
	status,
	requested_by,
	provider,
	region,
	request_payload,
	response_payload,
	error_message,
	created_at,
	updated_at,
	started_at,
	completed_at,
	retry_count,
	max_retries,
	next_retry_at,
	idempotency_key,
	correlation_id,
	timeout_at,
	project_id,
	template_id,
	provider_account_id
`

func scanProvisioningOperation(
	row pgx.Row,
) (*ProvisioningOperation, error) {
	var operation ProvisioningOperation

	err := row.Scan(
		&operation.ID,
		&operation.InstanceID,
		&operation.OperationType,
		&operation.Status,
		&operation.RequestedBy,
		&operation.Provider,
		&operation.Region,
		&operation.RequestPayload,
		&operation.ResponsePayload,
		&operation.ErrorMessage,
		&operation.CreatedAt,
		&operation.UpdatedAt,
		&operation.StartedAt,
		&operation.CompletedAt,
		&operation.RetryCount,
		&operation.MaxRetries,
		&operation.NextRetryAt,
		&operation.IdempotencyKey,
		&operation.CorrelationID,
		&operation.TimeoutAt,
		&operation.ProjectID,
		&operation.TemplateID,
		&operation.ProviderAccountID,
	)
	if err != nil {
		return nil, mapProvisioningOperationError(err)
	}

	return &operation, nil
}

func (r *ProvisioningOperationRepository) Create(
	ctx context.Context,
	input CreateProvisioningOperationInput,
) (*ProvisioningOperation, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}

	request := input.RequestPayload
	if len(request) == 0 {
		request = json.RawMessage(`{}`)
	}

	maxRetries := int32(3)
	if input.MaxRetries != nil {
		maxRetries = *input.MaxRetries
	}

	query := `
		INSERT INTO provisioning_operations (
			instance_id,
			operation_type,
			requested_by,
			provider,
			region,
			request_payload,
			max_retries,
			idempotency_key,
			correlation_id,
			timeout_at,
			project_id,
			template_id,
			provider_account_id
		)
		VALUES (
			$1::uuid, $2, $3, $4, $5, $6::jsonb,
			$7, $8, $9::uuid, $10,
			$11::uuid, $12::uuid, $13::uuid
		)
		RETURNING ` + provisioningOperationColumns

	return scanProvisioningOperation(
		r.pool.QueryRow(
			ctx,
			query,
			input.InstanceID,
			input.OperationType,
			input.RequestedBy,
			input.Provider,
			input.Region,
			string(request),
			maxRetries,
			input.IdempotencyKey,
			input.CorrelationID,
			input.TimeoutAt,
			input.ProjectID,
			input.TemplateID,
			input.ProviderAccountID,
		),
	)
}

func (r *ProvisioningOperationRepository) GetByID(
	ctx context.Context,
	id string,
) (*ProvisioningOperation, error) {
	if !validUUID(id) {
		return nil, errors.New("id must be a valid UUID")
	}

	query := `
		SELECT ` + provisioningOperationColumns + `
		FROM provisioning_operations
		WHERE id = $1
	`

	return scanProvisioningOperation(
		r.pool.QueryRow(ctx, query, id),
	)
}

func (r *ProvisioningOperationRepository) GetByIdempotencyKey(
	ctx context.Context,
	key string,
) (*ProvisioningOperation, error) {
	if strings.TrimSpace(key) == "" || len(key) > 255 {
		return nil, errors.New("invalid idempotency_key")
	}

	query := `
		SELECT ` + provisioningOperationColumns + `
		FROM provisioning_operations
		WHERE idempotency_key = $1
	`

	return scanProvisioningOperation(
		r.pool.QueryRow(ctx, query, key),
	)
}

func (r *ProvisioningOperationRepository) List(
	ctx context.Context,
	options ListProvisioningOperationsOptions,
) ([]ProvisioningOperation, error) {
	for _, field := range []struct {
		name  string
		value *string
	}{
		{"instance_id", options.InstanceID},
		{"project_id", options.ProjectID},
		{"template_id", options.TemplateID},
		{"provider_account_id", options.ProviderAccountID},
		{"correlation_id", options.CorrelationID},
	} {
		if err := validateOperationUUID(
			field.name, field.value,
		); err != nil {
			return nil, err
		}
	}

	if options.OperationType != "" &&
		!validProvisioningOperationType(options.OperationType) {
		return nil, errors.New("invalid operation_type")
	}

	if options.Status != "" &&
		!validProvisioningOperationStatus(options.Status) {
		return nil, errors.New("invalid status")
	}

	if len(options.Provider) > 50 {
		return nil, errors.New("provider exceeds 50 characters")
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

	var operationType, status, provider any
	if options.OperationType != "" {
		operationType = options.OperationType
	}
	if options.Status != "" {
		status = options.Status
	}
	if options.Provider != "" {
		provider = options.Provider
	}

	query := `
		SELECT ` + provisioningOperationColumns + `
		FROM provisioning_operations
		WHERE ($1::uuid IS NULL OR instance_id = $1::uuid)
		  AND ($2::uuid IS NULL OR project_id = $2::uuid)
		  AND ($3::uuid IS NULL OR template_id = $3::uuid)
		  AND ($4::uuid IS NULL OR provider_account_id = $4::uuid)
		  AND ($5::uuid IS NULL OR correlation_id = $5::uuid)
		  AND ($6::text IS NULL OR operation_type = $6::text)
		  AND ($7::text IS NULL OR status = $7::text)
		  AND ($8::text IS NULL OR provider = $8::text)
		ORDER BY created_at DESC, id DESC
		LIMIT $9 OFFSET $10
	`

	rows, err := r.pool.Query(
		ctx,
		query,
		options.InstanceID,
		options.ProjectID,
		options.TemplateID,
		options.ProviderAccountID,
		options.CorrelationID,
		operationType,
		status,
		provider,
		limit,
		options.Offset,
	)
	if err != nil {
		return nil, mapProvisioningOperationError(err)
	}
	defer rows.Close()

	operations := make([]ProvisioningOperation, 0)

	for rows.Next() {
		operation, err := scanProvisioningOperation(rows)
		if err != nil {
			return nil, err
		}
		operations = append(operations, *operation)
	}

	if err := rows.Err(); err != nil {
		return nil, mapProvisioningOperationError(err)
	}

	return operations, nil
}

func (r *ProvisioningOperationRepository) UpdateStatus(
	ctx context.Context,
	id string,
	input UpdateProvisioningOperationStatusInput,
) (*ProvisioningOperation, error) {
	if !validUUID(id) {
		return nil, errors.New("id must be a valid UUID")
	}
	if err := input.Validate(); err != nil {
		return nil, err
	}

	var response any
	if len(input.ResponsePayload) > 0 {
		response = string(input.ResponsePayload)
	}

	// Status updates preserve timestamps not explicitly supplied.
	// Lifecycle transition policies belong in the provisioning engine.
	query := `
		UPDATE provisioning_operations
		SET
			status = $2,
			response_payload = COALESCE($3::jsonb, response_payload),
			error_message = $4,
			started_at = COALESCE($5, started_at),
			completed_at = COALESCE($6, completed_at),
			next_retry_at = $7,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
		RETURNING ` + provisioningOperationColumns

	return scanProvisioningOperation(
		r.pool.QueryRow(
			ctx,
			query,
			id,
			input.Status,
			response,
			input.ErrorMessage,
			input.StartedAt,
			input.CompletedAt,
			input.NextRetryAt,
		),
	)
}

func (r *ProvisioningOperationRepository) IncrementRetry(
	ctx context.Context,
	id string,
	nextRetryAt *time.Time,
) (*ProvisioningOperation, error) {
	if !validUUID(id) {
		return nil, errors.New("id must be a valid UUID")
	}

	query := `
		UPDATE provisioning_operations
		SET
			retry_count = retry_count + 1,
			next_retry_at = $2,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
		  AND retry_count < max_retries
		  AND status IN ('pending', 'running', 'failed')
		RETURNING ` + provisioningOperationColumns

	operation, err := scanProvisioningOperation(
		r.pool.QueryRow(ctx, query, id, nextRetryAt),
	)
	if errors.Is(err, ErrProvisioningOperationNotFound) {
		// Distinguish a missing operation from an operation
		// that exists but cannot accept another retry.
		var exists bool
		checkErr := r.pool.QueryRow(
			ctx,
			`SELECT EXISTS(
				SELECT 1 FROM provisioning_operations WHERE id = $1
			)`,
			id,
		).Scan(&exists)
		if checkErr != nil {
			return nil, mapProvisioningOperationError(checkErr)
		}
		if exists {
			return nil, ErrProvisioningOperationConflict
		}
	}
	return operation, err
}

func (r *ProvisioningOperationRepository) Delete(
	ctx context.Context,
	id string,
) error {
	if !validUUID(id) {
		return errors.New("id must be a valid UUID")
	}

	tag, err := r.pool.Exec(
		ctx,
		`DELETE FROM provisioning_operations WHERE id = $1`,
		id,
	)
	if err != nil {
		return mapProvisioningOperationError(err)
	}

	if tag.RowsAffected() == 0 {
		return ErrProvisioningOperationNotFound
	}

	return nil
}

func mapProvisioningOperationError(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, pgx.ErrNoRows) {
		return ErrProvisioningOperationNotFound
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return fmt.Errorf(
				"%w: %v",
				ErrProvisioningOperationConflict,
				err,
			)
		case "23503":
			return fmt.Errorf(
				"provisioning operation foreign-key constraint: %w",
				err,
			)
		case "23514":
			return fmt.Errorf(
				"provisioning operation check constraint: %w",
				err,
			)
		case "23502":
			return fmt.Errorf(
				"provisioning operation required field: %w",
				err,
			)
		}
	}

	return err
}
