package persistence

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gdiop88/RL_trader_infrastructure/internal/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrResourceReservationNotFound = errors.New(
		"resource reservation not found",
	)
	ErrResourceReservationConflict = errors.New(
		"resource reservation conflict",
	)
)

type ResourceReservation struct {
	ID               string
	ProjectID        string
	AgentID          *string
	InstanceID       *string
	ReservationType  string
	VCPUReserved     *int
	MemoryMBReserved *int
	GPUReserved      *int
	Status           string
	StartsAt         *time.Time
	ExpiresAt        *time.Time
	CreatedAt        time.Time
}

type CreateResourceReservationInput struct {
	ProjectID        string
	AgentID          *string
	InstanceID       *string
	ReservationType  string
	VCPUReserved     *int
	MemoryMBReserved *int
	GPUReserved      *int
	Status           string
	StartsAt         *time.Time
	ExpiresAt        *time.Time
}

type UpdateResourceReservationInput struct {
	AgentID          *string
	InstanceID       *string
	ReservationType  string
	VCPUReserved     *int
	MemoryMBReserved *int
	GPUReserved      *int
	Status           string
	StartsAt         *time.Time
	ExpiresAt        *time.Time
}

type ListResourceReservationsOptions struct {
	ProjectID       *string
	AgentID         *string
	InstanceID      *string
	ReservationType *string
	Status          *string
	Limit           int
	Offset          int
}

type ResourceReservationRepository struct {
	pool *pgxpool.Pool
}

func NewResourceReservationRepository(
	db *database.Database,
) (*ResourceReservationRepository, error) {
	if db == nil || db.Pool() == nil {
		return nil, errors.New("database connection is required")
	}

	return &ResourceReservationRepository{
		pool: db.Pool(),
	}, nil
}

func validateResourceReservationFields(
	agentID *string,
	instanceID *string,
	reservationType string,
	vcpuReserved *int,
	memoryMBReserved *int,
	gpuReserved *int,
	status string,
	startsAt *time.Time,
	expiresAt *time.Time,
) error {
	if agentID != nil && !validUUID(*agentID) {
		return errors.New("agent_id must be a valid UUID")
	}

	if instanceID != nil && !validUUID(*instanceID) {
		return errors.New("instance_id must be a valid UUID")
	}

	if len(reservationType) == 0 || len(reservationType) > 50 {
		return errors.New(
			"reservation_type must contain 1 to 50 characters",
		)
	}

	if len(status) == 0 || len(status) > 50 {
		return errors.New("status must contain 1 to 50 characters")
	}

	if vcpuReserved != nil && *vcpuReserved < 0 {
		return errors.New("vcpu_reserved cannot be negative")
	}

	if memoryMBReserved != nil && *memoryMBReserved < 0 {
		return errors.New("memory_mb_reserved cannot be negative")
	}

	if gpuReserved != nil && *gpuReserved < 0 {
		return errors.New("gpu_reserved cannot be negative")
	}

	if startsAt != nil && startsAt.IsZero() {
		return errors.New("starts_at cannot be a zero timestamp")
	}

	if expiresAt != nil && expiresAt.IsZero() {
		return errors.New("expires_at cannot be a zero timestamp")
	}

	return nil
}

func (in CreateResourceReservationInput) Validate() error {
	if !validUUID(in.ProjectID) {
		return errors.New("project_id must be a valid UUID")
	}

	return validateResourceReservationFields(
		in.AgentID,
		in.InstanceID,
		in.ReservationType,
		in.VCPUReserved,
		in.MemoryMBReserved,
		in.GPUReserved,
		in.Status,
		in.StartsAt,
		in.ExpiresAt,
	)
}

func (in UpdateResourceReservationInput) Validate() error {
	return validateResourceReservationFields(
		in.AgentID,
		in.InstanceID,
		in.ReservationType,
		in.VCPUReserved,
		in.MemoryMBReserved,
		in.GPUReserved,
		in.Status,
		in.StartsAt,
		in.ExpiresAt,
	)
}

const resourceReservationColumns = `
	id,
	project_id,
	agent_id,
	instance_id,
	reservation_type,
	vcpu_reserved,
	memory_mb_reserved,
	gpu_reserved,
	status,
	starts_at,
	expires_at,
	created_at
`

func scanResourceReservation(
	row pgx.Row,
) (*ResourceReservation, error) {
	var reservation ResourceReservation

	err := row.Scan(
		&reservation.ID,
		&reservation.ProjectID,
		&reservation.AgentID,
		&reservation.InstanceID,
		&reservation.ReservationType,
		&reservation.VCPUReserved,
		&reservation.MemoryMBReserved,
		&reservation.GPUReserved,
		&reservation.Status,
		&reservation.StartsAt,
		&reservation.ExpiresAt,
		&reservation.CreatedAt,
	)
	if err != nil {
		return nil, mapResourceReservationError(err)
	}

	return &reservation, nil
}

func (r *ResourceReservationRepository) Create(
	ctx context.Context,
	input CreateResourceReservationInput,
) (*ResourceReservation, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}

	query := `
		INSERT INTO resource_reservations (
			project_id,
			agent_id,
			instance_id,
			reservation_type,
			vcpu_reserved,
			memory_mb_reserved,
			gpu_reserved,
			status,
			starts_at,
			expires_at
		)
		VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8, $9, $10
		)
		RETURNING ` + resourceReservationColumns

	return scanResourceReservation(
		r.pool.QueryRow(
			ctx,
			query,
			input.ProjectID,
			input.AgentID,
			input.InstanceID,
			input.ReservationType,
			input.VCPUReserved,
			input.MemoryMBReserved,
			input.GPUReserved,
			input.Status,
			input.StartsAt,
			input.ExpiresAt,
		),
	)
}

func (r *ResourceReservationRepository) GetByID(
	ctx context.Context,
	id string,
) (*ResourceReservation, error) {
	if !validUUID(id) {
		return nil, errors.New("id must be a valid UUID")
	}

	query := `
		SELECT ` + resourceReservationColumns + `
		FROM resource_reservations
		WHERE id = $1
	`

	return scanResourceReservation(
		r.pool.QueryRow(ctx, query, id),
	)
}

func (r *ResourceReservationRepository) List(
	ctx context.Context,
	options ListResourceReservationsOptions,
) ([]ResourceReservation, error) {
	if options.ProjectID != nil && !validUUID(*options.ProjectID) {
		return nil, errors.New("project_id must be a valid UUID")
	}

	if options.AgentID != nil && !validUUID(*options.AgentID) {
		return nil, errors.New("agent_id must be a valid UUID")
	}

	if options.InstanceID != nil && !validUUID(*options.InstanceID) {
		return nil, errors.New("instance_id must be a valid UUID")
	}

	if err := validateOptionalString(
		"reservation_type",
		options.ReservationType,
		50,
	); err != nil {
		return nil, err
	}

	if err := validateOptionalString(
		"status",
		options.Status,
		50,
	); err != nil {
		return nil, err
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

	var projectID, agentID, instanceID any
	var reservationType, status any

	if options.ProjectID != nil {
		projectID = *options.ProjectID
	}
	if options.AgentID != nil {
		agentID = *options.AgentID
	}
	if options.InstanceID != nil {
		instanceID = *options.InstanceID
	}
	if options.ReservationType != nil {
		reservationType = *options.ReservationType
	}
	if options.Status != nil {
		status = *options.Status
	}

	query := `
		SELECT ` + resourceReservationColumns + `
		FROM resource_reservations
		WHERE
			($1::uuid IS NULL OR project_id = $1::uuid)
			AND ($2::uuid IS NULL OR agent_id = $2::uuid)
			AND ($3::uuid IS NULL OR instance_id = $3::uuid)
			AND (
				$4::text IS NULL
				OR reservation_type = $4::text
			)
			AND ($5::text IS NULL OR status = $5::text)
		ORDER BY created_at DESC, id ASC
		LIMIT $6 OFFSET $7
	`

	rows, err := r.pool.Query(
		ctx,
		query,
		projectID,
		agentID,
		instanceID,
		reservationType,
		status,
		limit,
		options.Offset,
	)
	if err != nil {
		return nil, mapResourceReservationError(err)
	}
	defer rows.Close()

	reservations := make([]ResourceReservation, 0)

	for rows.Next() {
		reservation, err := scanResourceReservation(rows)
		if err != nil {
			return nil, err
		}
		reservations = append(reservations, *reservation)
	}

	if err := rows.Err(); err != nil {
		return nil, mapResourceReservationError(err)
	}

	return reservations, nil
}

func (r *ResourceReservationRepository) Update(
	ctx context.Context,
	id string,
	input UpdateResourceReservationInput,
) (*ResourceReservation, error) {
	if !validUUID(id) {
		return nil, errors.New("id must be a valid UUID")
	}

	if err := input.Validate(); err != nil {
		return nil, err
	}

	query := `
		UPDATE resource_reservations
		SET
			agent_id = $2,
			instance_id = $3,
			reservation_type = $4,
			vcpu_reserved = $5,
			memory_mb_reserved = $6,
			gpu_reserved = $7,
			status = $8,
			starts_at = $9,
			expires_at = $10
		WHERE id = $1
		RETURNING ` + resourceReservationColumns

	return scanResourceReservation(
		r.pool.QueryRow(
			ctx,
			query,
			id,
			input.AgentID,
			input.InstanceID,
			input.ReservationType,
			input.VCPUReserved,
			input.MemoryMBReserved,
			input.GPUReserved,
			input.Status,
			input.StartsAt,
			input.ExpiresAt,
		),
	)
}

func (r *ResourceReservationRepository) UpdateStatus(
	ctx context.Context,
	id string,
	status string,
) (*ResourceReservation, error) {
	if !validUUID(id) {
		return nil, errors.New("id must be a valid UUID")
	}

	if len(status) == 0 || len(status) > 50 {
		return nil, errors.New(
			"status must contain 1 to 50 characters",
		)
	}

	query := `
		UPDATE resource_reservations
		SET status = $2
		WHERE id = $1
		RETURNING ` + resourceReservationColumns

	return scanResourceReservation(
		r.pool.QueryRow(ctx, query, id, status),
	)
}

func (r *ResourceReservationRepository) Delete(
	ctx context.Context,
	id string,
) error {
	if !validUUID(id) {
		return errors.New("id must be a valid UUID")
	}

	tag, err := r.pool.Exec(
		ctx,
		`DELETE FROM resource_reservations WHERE id = $1`,
		id,
	)
	if err != nil {
		return mapResourceReservationError(err)
	}

	if tag.RowsAffected() == 0 {
		return ErrResourceReservationNotFound
	}

	return nil
}

func mapResourceReservationError(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, pgx.ErrNoRows) {
		return ErrResourceReservationNotFound
	}

	var pgErr *pgconn.PgError

	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return fmt.Errorf(
				"%w: %v",
				ErrResourceReservationConflict,
				err,
			)

		case "23503":
			return fmt.Errorf(
				"resource reservation foreign-key constraint: %w",
				err,
			)

		case "23514":
			return fmt.Errorf(
				"resource reservation check constraint: %w",
				err,
			)

		case "23502":
			return fmt.Errorf(
				"resource reservation required field: %w",
				err,
			)
		}
	}

	return err
}
