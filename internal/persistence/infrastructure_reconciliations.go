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
	ErrInfrastructureReconciliationNotFound = errors.New(
		"infrastructure reconciliation not found",
	)
	ErrInfrastructureReconciliationConflict = errors.New(
		"infrastructure reconciliation conflict",
	)
)

type InfrastructureReconciliation struct {
	ID            string
	InstanceID    string
	DesiredState  *string
	ObservedState *string
	DriftDetected *bool
	DriftDetails  json.RawMessage
	Status        *string
	CheckedAt     *time.Time
	ResolvedAt    *time.Time
}

type CreateInfrastructureReconciliationInput struct {
	InstanceID    string
	DesiredState  *string
	ObservedState *string
	DriftDetected *bool
	DriftDetails  json.RawMessage
	Status        *string
	CheckedAt     *time.Time
	ResolvedAt    *time.Time
}

type UpdateInfrastructureReconciliationInput struct {
	DesiredState  *string
	ObservedState *string
	DriftDetected *bool
	DriftDetails  json.RawMessage
	Status        *string
	CheckedAt     *time.Time
	ResolvedAt    *time.Time
}

type ListInfrastructureReconciliationsOptions struct {
	InstanceID    *string
	Status        *string
	DriftDetected *bool
	From          *time.Time
	To            *time.Time
	Limit         int
	Offset        int
}

type InfrastructureReconciliationRepository struct {
	pool *pgxpool.Pool
}

const infrastructureReconciliationColumns = `
	id,
	instance_id,
	desired_state,
	observed_state,
	drift_detected,
	drift_details,
	status,
	checked_at,
	resolved_at
`

func NewInfrastructureReconciliationRepository(
	db *database.Database,
) (*InfrastructureReconciliationRepository, error) {
	if db == nil || db.Pool() == nil {
		return nil, errors.New("database pool is required")
	}
	return &InfrastructureReconciliationRepository{
		pool: db.Pool(),
	}, nil
}

func validateReconciliationFields(
	desiredState *string,
	observedState *string,
	driftDetails json.RawMessage,
	status *string,
) error {
	for _, field := range []struct {
		name  string
		value *string
	}{
		{"desired_state", desiredState},
		{"observed_state", observedState},
		{"status", status},
	} {
		if field.value != nil && len(*field.value) > 50 {
			return fmt.Errorf("%s exceeds 50 characters", field.name)
		}
	}

	if len(driftDetails) > 0 && !json.Valid(driftDetails) {
		return errors.New("drift_details must be valid JSON")
	}
	return nil
}

func (in CreateInfrastructureReconciliationInput) Validate() error {
	if !validUUID(in.InstanceID) {
		return errors.New("instance_id must be a valid UUID")
	}
	return validateReconciliationFields(
		in.DesiredState,
		in.ObservedState,
		in.DriftDetails,
		in.Status,
	)
}

func (in UpdateInfrastructureReconciliationInput) Validate() error {
	return validateReconciliationFields(
		in.DesiredState,
		in.ObservedState,
		in.DriftDetails,
		in.Status,
	)
}

func (opts ListInfrastructureReconciliationsOptions) Validate() error {
	if opts.InstanceID != nil && !validUUID(*opts.InstanceID) {
		return errors.New("instance_id must be a valid UUID")
	}
	if opts.Status != nil && len(*opts.Status) > 50 {
		return errors.New("status exceeds 50 characters")
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

func reconciliationJSONArgument(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	return string(raw)
}

func scanInfrastructureReconciliation(
	row pgx.Row,
) (*InfrastructureReconciliation, error) {
	var item InfrastructureReconciliation

	err := row.Scan(
		&item.ID,
		&item.InstanceID,
		&item.DesiredState,
		&item.ObservedState,
		&item.DriftDetected,
		&item.DriftDetails,
		&item.Status,
		&item.CheckedAt,
		&item.ResolvedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrInfrastructureReconciliationNotFound
		}
		return nil, mapInfrastructureReconciliationError(err)
	}
	return &item, nil
}

func mapInfrastructureReconciliationError(err error) error {
	if err == nil {
		return nil
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return fmt.Errorf(
				"%w: %v",
				ErrInfrastructureReconciliationConflict,
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

func (r *InfrastructureReconciliationRepository) Create(
	ctx context.Context,
	in CreateInfrastructureReconciliationInput,
) (*InfrastructureReconciliation, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}

	query := `
		INSERT INTO infrastructure_reconciliations (
			instance_id,
			desired_state,
			observed_state,
			drift_detected,
			drift_details,
			status,
			checked_at,
			resolved_at
		)
		VALUES ($1,$2,$3,$4,$5::jsonb,$6,$7,$8)
		RETURNING ` + infrastructureReconciliationColumns

	return scanInfrastructureReconciliation(
		r.pool.QueryRow(
			ctx,
			query,
			in.InstanceID,
			in.DesiredState,
			in.ObservedState,
			in.DriftDetected,
			reconciliationJSONArgument(in.DriftDetails),
			in.Status,
			in.CheckedAt,
			in.ResolvedAt,
		),
	)
}

func (r *InfrastructureReconciliationRepository) GetByID(
	ctx context.Context,
	id string,
) (*InfrastructureReconciliation, error) {
	if !validUUID(id) {
		return nil, errors.New("id must be a valid UUID")
	}

	query := `
		SELECT ` + infrastructureReconciliationColumns + `
		FROM infrastructure_reconciliations
		WHERE id = $1
	`

	return scanInfrastructureReconciliation(
		r.pool.QueryRow(ctx, query, id),
	)
}

func (r *InfrastructureReconciliationRepository) List(
	ctx context.Context,
	opts ListInfrastructureReconciliationsOptions,
) ([]InfrastructureReconciliation, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	limit := opts.Limit
	if limit == 0 {
		limit = 100
	}

	var conditions []string
	var args []any

	addFilter := func(expression string, value any) {
		args = append(args, value)
		conditions = append(
			conditions,
			fmt.Sprintf(expression, len(args)),
		)
	}

	if opts.InstanceID != nil {
		addFilter("instance_id = $%d", *opts.InstanceID)
	}
	if opts.Status != nil {
		addFilter("status = $%d", *opts.Status)
	}
	if opts.DriftDetected != nil {
		addFilter("drift_detected = $%d", *opts.DriftDetected)
	}
	if opts.From != nil {
		addFilter("checked_at >= $%d", *opts.From)
	}
	if opts.To != nil {
		addFilter("checked_at <= $%d", *opts.To)
	}

	query := `
		SELECT ` + infrastructureReconciliationColumns + `
		FROM infrastructure_reconciliations
	`

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}

	args = append(args, limit)
	limitPosition := len(args)
	args = append(args, opts.Offset)
	offsetPosition := len(args)

	query += fmt.Sprintf(
		" ORDER BY checked_at DESC NULLS LAST, id ASC LIMIT $%d OFFSET $%d",
		limitPosition,
		offsetPosition,
	)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, mapInfrastructureReconciliationError(err)
	}
	defer rows.Close()

	items := make([]InfrastructureReconciliation, 0)

	for rows.Next() {
		item, err := scanInfrastructureReconciliation(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}

	if err := rows.Err(); err != nil {
		return nil, mapInfrastructureReconciliationError(err)
	}

	return items, nil
}

func (r *InfrastructureReconciliationRepository) Update(
	ctx context.Context,
	id string,
	in UpdateInfrastructureReconciliationInput,
) (*InfrastructureReconciliation, error) {
	if !validUUID(id) {
		return nil, errors.New("id must be a valid UUID")
	}
	if err := in.Validate(); err != nil {
		return nil, err
	}

	query := `
		UPDATE infrastructure_reconciliations
		SET desired_state = $2,
			observed_state = $3,
			drift_detected = $4,
			drift_details = $5::jsonb,
			status = $6,
			checked_at = $7,
			resolved_at = $8
		WHERE id = $1
		RETURNING ` + infrastructureReconciliationColumns

	return scanInfrastructureReconciliation(
		r.pool.QueryRow(
			ctx,
			query,
			id,
			in.DesiredState,
			in.ObservedState,
			in.DriftDetected,
			reconciliationJSONArgument(in.DriftDetails),
			in.Status,
			in.CheckedAt,
			in.ResolvedAt,
		),
	)
}

func (r *InfrastructureReconciliationRepository) Delete(
	ctx context.Context,
	id string,
) error {
	if !validUUID(id) {
		return errors.New("id must be a valid UUID")
	}

	tag, err := r.pool.Exec(
		ctx,
		`DELETE FROM infrastructure_reconciliations WHERE id = $1`,
		id,
	)
	if err != nil {
		return mapInfrastructureReconciliationError(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrInfrastructureReconciliationNotFound
	}
	return nil
}
