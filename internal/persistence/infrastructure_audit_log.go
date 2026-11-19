package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gdiop88/RL_trader_infrastructure/internal/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrInfrastructureAuditLogNotFound = errors.New(
		"infrastructure audit log not found",
	)
	ErrInfrastructureAuditLogConflict = errors.New(
		"infrastructure audit log conflict",
	)
)

type InfrastructureAuditLog struct {
	ID                string
	ProjectID         *string
	OperationID       *string
	InstanceID        *string
	ActorReference    *string
	Action            string
	ResourceType      *string
	ResourceReference *string
	Outcome           *string
	Details           json.RawMessage
	OccurredAt        time.Time
}

type CreateInfrastructureAuditLogInput struct {
	ProjectID         *string
	OperationID       *string
	InstanceID        *string
	ActorReference    *string
	Action            string
	ResourceType      *string
	ResourceReference *string
	Outcome           *string
	Details           json.RawMessage
	OccurredAt        time.Time
}

type ListInfrastructureAuditLogsOptions struct {
	ProjectID         *string
	OperationID       *string
	InstanceID        *string
	ActorReference    *string
	Action            *string
	ResourceType      *string
	ResourceReference *string
	Outcome           *string
	From              *time.Time
	To                *time.Time
	Limit             int
	Offset            int
}

type InfrastructureAuditLogRepository struct {
	pool *pgxpool.Pool
}

const infrastructureAuditLogColumns = `
	id,
	project_id,
	operation_id,
	instance_id,
	actor_reference,
	action,
	resource_type,
	resource_reference,
	outcome,
	details,
	occurred_at
`

func NewInfrastructureAuditLogRepository(
	db *database.Database,
) (*InfrastructureAuditLogRepository, error) {
	if db == nil || db.Pool() == nil {
		return nil, errors.New("database pool is required")
	}

	return &InfrastructureAuditLogRepository{
		pool: db.Pool(),
	}, nil
}

func validateAuditOptionalUUID(name string, value *string) error {
	if value != nil && !validUUID(*value) {
		return fmt.Errorf("%s must be a valid UUID", name)
	}
	return nil
}

func validateAuditOptionalText(
	name string,
	value *string,
	maxLength int,
) error {
	if value != nil && utf8.RuneCountInString(*value) > maxLength {
		return fmt.Errorf("%s exceeds %d characters", name, maxLength)
	}
	return nil
}

func validateAuditTextFields(
	actorReference *string,
	action string,
	resourceType *string,
	resourceReference *string,
	outcome *string,
) error {
	if utf8.RuneCountInString(action) > 100 {
		return errors.New("action exceeds 100 characters")
	}

	for _, field := range []struct {
		name   string
		value  *string
		maxLen int
	}{
		{"actor_reference", actorReference, 255},
		{"resource_type", resourceType, 100},
		{"resource_reference", resourceReference, 255},
		{"outcome", outcome, 50},
	} {
		if err := validateAuditOptionalText(
			field.name,
			field.value,
			field.maxLen,
		); err != nil {
			return err
		}
	}

	return nil
}

func (in CreateInfrastructureAuditLogInput) Validate() error {
	for _, field := range []struct {
		name  string
		value *string
	}{
		{"project_id", in.ProjectID},
		{"operation_id", in.OperationID},
		{"instance_id", in.InstanceID},
	} {
		if err := validateAuditOptionalUUID(
			field.name,
			field.value,
		); err != nil {
			return err
		}
	}

	if in.OccurredAt.IsZero() {
		return errors.New("occurred_at must not be zero")
	}

	if err := validateAuditTextFields(
		in.ActorReference,
		in.Action,
		in.ResourceType,
		in.ResourceReference,
		in.Outcome,
	); err != nil {
		return err
	}

	if len(in.Details) > 0 && !json.Valid(in.Details) {
		return errors.New("details must be valid JSON")
	}

	return nil
}

func (opts ListInfrastructureAuditLogsOptions) Validate() error {
	for _, field := range []struct {
		name  string
		value *string
	}{
		{"project_id", opts.ProjectID},
		{"operation_id", opts.OperationID},
		{"instance_id", opts.InstanceID},
	} {
		if err := validateAuditOptionalUUID(
			field.name,
			field.value,
		); err != nil {
			return err
		}
	}

	if opts.Action != nil &&
		utf8.RuneCountInString(*opts.Action) > 100 {
		return errors.New("action exceeds 100 characters")
	}

	for _, field := range []struct {
		name   string
		value  *string
		maxLen int
	}{
		{"actor_reference", opts.ActorReference, 255},
		{"resource_type", opts.ResourceType, 100},
		{"resource_reference", opts.ResourceReference, 255},
		{"outcome", opts.Outcome, 50},
	} {
		if err := validateAuditOptionalText(
			field.name,
			field.value,
			field.maxLen,
		); err != nil {
			return err
		}
	}

	if opts.From != nil && opts.From.IsZero() {
		return errors.New("from must not be zero")
	}
	if opts.To != nil && opts.To.IsZero() {
		return errors.New("to must not be zero")
	}
	if opts.From != nil && opts.To != nil &&
		opts.From.After(*opts.To) {
		return errors.New("from must not be after to")
	}
	if opts.Limit < 0 || opts.Limit > 1000 {
		return errors.New("limit must be between 0 and 1000")
	}
	if opts.Offset < 0 {
		return errors.New("offset must not be negative")
	}

	return nil
}

func auditLogJSONArgument(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	return string(raw)
}

func scanInfrastructureAuditLog(
	row pgx.Row,
) (*InfrastructureAuditLog, error) {
	var item InfrastructureAuditLog

	err := row.Scan(
		&item.ID,
		&item.ProjectID,
		&item.OperationID,
		&item.InstanceID,
		&item.ActorReference,
		&item.Action,
		&item.ResourceType,
		&item.ResourceReference,
		&item.Outcome,
		&item.Details,
		&item.OccurredAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrInfrastructureAuditLogNotFound
		}
		return nil, mapInfrastructureAuditLogError(err)
	}

	return &item, nil
}

func mapInfrastructureAuditLogError(err error) error {
	if err == nil {
		return nil
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return fmt.Errorf(
				"%w: %v",
				ErrInfrastructureAuditLogConflict,
				err,
			)
		case "23503":
			return fmt.Errorf("foreign key violation: %w", err)
		case "23502":
			return fmt.Errorf("required field violation: %w", err)
		case "23514":
			return fmt.Errorf("check constraint violation: %w", err)
		case "22001":
			return fmt.Errorf("field length violation: %w", err)
		case "22P02":
			return fmt.Errorf("invalid database value: %w", err)
		}
	}

	return err
}

func (r *InfrastructureAuditLogRepository) Create(
	ctx context.Context,
	in CreateInfrastructureAuditLogInput,
) (*InfrastructureAuditLog, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}

	query := `
		INSERT INTO infrastructure_audit_log (
			project_id,
			operation_id,
			instance_id,
			actor_reference,
			action,
			resource_type,
			resource_reference,
			outcome,
			details,
			occurred_at
		)
		VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10
		)
		RETURNING ` + infrastructureAuditLogColumns

	return scanInfrastructureAuditLog(
		r.pool.QueryRow(
			ctx,
			query,
			in.ProjectID,
			in.OperationID,
			in.InstanceID,
			in.ActorReference,
			in.Action,
			in.ResourceType,
			in.ResourceReference,
			in.Outcome,
			auditLogJSONArgument(in.Details),
			in.OccurredAt,
		),
	)
}

func (r *InfrastructureAuditLogRepository) GetByID(
	ctx context.Context,
	id string,
) (*InfrastructureAuditLog, error) {
	if !validUUID(id) {
		return nil, errors.New("id must be a valid UUID")
	}

	query := `
		SELECT ` + infrastructureAuditLogColumns + `
		FROM infrastructure_audit_log
		WHERE id = $1
	`

	return scanInfrastructureAuditLog(
		r.pool.QueryRow(ctx, query, id),
	)
}

func (r *InfrastructureAuditLogRepository) List(
	ctx context.Context,
	opts ListInfrastructureAuditLogsOptions,
) ([]InfrastructureAuditLog, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	limit := opts.Limit
	if limit == 0 {
		limit = 100
	}

	var conditions []string
	var args []any

	addFilter := func(column, operator string, value any) {
		args = append(args, value)
		conditions = append(
			conditions,
			fmt.Sprintf(
				"%s %s $%d",
				column,
				operator,
				len(args),
			),
		)
	}

	if opts.ProjectID != nil {
		addFilter("project_id", "=", *opts.ProjectID)
	}
	if opts.OperationID != nil {
		addFilter("operation_id", "=", *opts.OperationID)
	}
	if opts.InstanceID != nil {
		addFilter("instance_id", "=", *opts.InstanceID)
	}
	if opts.ActorReference != nil {
		addFilter("actor_reference", "=", *opts.ActorReference)
	}
	if opts.Action != nil {
		addFilter("action", "=", *opts.Action)
	}
	if opts.ResourceType != nil {
		addFilter("resource_type", "=", *opts.ResourceType)
	}
	if opts.ResourceReference != nil {
		addFilter("resource_reference", "=", *opts.ResourceReference)
	}
	if opts.Outcome != nil {
		addFilter("outcome", "=", *opts.Outcome)
	}
	if opts.From != nil {
		addFilter("occurred_at", ">=", *opts.From)
	}
	if opts.To != nil {
		addFilter("occurred_at", "<=", *opts.To)
	}

	query := `
		SELECT ` + infrastructureAuditLogColumns + `
		FROM infrastructure_audit_log
	`

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}

	args = append(args, limit)
	limitPosition := len(args)

	args = append(args, opts.Offset)
	offsetPosition := len(args)

	query += fmt.Sprintf(
		" ORDER BY occurred_at DESC, id ASC LIMIT $%d OFFSET $%d",
		limitPosition,
		offsetPosition,
	)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, mapInfrastructureAuditLogError(err)
	}
	defer rows.Close()

	items := make([]InfrastructureAuditLog, 0)

	for rows.Next() {
		item, err := scanInfrastructureAuditLog(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}

	if err := rows.Err(); err != nil {
		return nil, mapInfrastructureAuditLogError(err)
	}

	return items, nil
}
