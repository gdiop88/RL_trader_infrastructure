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
	ErrAgentInstanceAssignmentNotFound = errors.New("agent instance assignment not found")
	ErrAgentInstanceAssignmentConflict = errors.New("agent instance assignment conflict")
)

type AgentInstanceAssignment struct {
	ID             string
	AgentID        string
	InstanceID     string
	AssignmentRole *string
	Status         string
	AssignedAt     *time.Time
	ReleasedAt     *time.Time
	Metadata       json.RawMessage
}

type CreateAgentInstanceAssignmentInput struct {
	AgentID        string
	InstanceID     string
	AssignmentRole *string
	Status         string
	AssignedAt     *time.Time
	ReleasedAt     *time.Time
	Metadata       json.RawMessage
}

type UpdateAgentInstanceAssignmentInput struct {
	AssignmentRole *string
	Status         string
	AssignedAt     *time.Time
	ReleasedAt     *time.Time
	Metadata       json.RawMessage
}

type ListAgentInstanceAssignmentsOptions struct {
	AgentID    string
	InstanceID string
	Status     string
	Limit      int
	Offset     int
}

type AgentInstanceAssignmentRepository struct {
	pool *pgxpool.Pool
}

func NewAgentInstanceAssignmentRepository(
	db *database.Database,
) (*AgentInstanceAssignmentRepository, error) {
	if db == nil || db.Pool() == nil {
		return nil, errors.New("database connection is required")
	}

	return &AgentInstanceAssignmentRepository{
		pool: db.Pool(),
	}, nil
}

func validateAssignmentStatus(status string) error {
	if strings.TrimSpace(status) == "" ||
		len(status) > 50 ||
		strings.ContainsRune(status, '\x00') {
		return errors.New("status must contain 1 to 50 valid characters")
	}
	return nil
}

func validateAssignmentFields(
	role *string,
	status string,
	assignedAt *time.Time,
	releasedAt *time.Time,
	metadata json.RawMessage,
) error {
	if err := validateOptionalString(
		"assignment_role", role, 100,
	); err != nil {
		return err
	}

	if role != nil && strings.ContainsRune(*role, '\x00') {
		return errors.New("assignment_role contains a null character")
	}

	if err := validateAssignmentStatus(status); err != nil {
		return err
	}

	if assignedAt != nil && assignedAt.IsZero() {
		return errors.New("assigned_at cannot be zero")
	}

	if releasedAt != nil && releasedAt.IsZero() {
		return errors.New("released_at cannot be zero")
	}

	if len(metadata) > 0 && !json.Valid(metadata) {
		return errors.New("metadata contains invalid JSON")
	}

	return nil
}

func (in CreateAgentInstanceAssignmentInput) Validate() error {
	if !validUUID(in.AgentID) {
		return errors.New("agent_id must be a valid UUID")
	}
	if !validUUID(in.InstanceID) {
		return errors.New("instance_id must be a valid UUID")
	}

	return validateAssignmentFields(
		in.AssignmentRole,
		in.Status,
		in.AssignedAt,
		in.ReleasedAt,
		in.Metadata,
	)
}

func (in UpdateAgentInstanceAssignmentInput) Validate() error {
	return validateAssignmentFields(
		in.AssignmentRole,
		in.Status,
		in.AssignedAt,
		in.ReleasedAt,
		in.Metadata,
	)
}

const agentInstanceAssignmentColumns = `
	id,
	agent_id,
	instance_id,
	assignment_role,
	status,
	assigned_at,
	released_at,
	metadata
`

func scanAgentInstanceAssignment(
	row pgx.Row,
) (*AgentInstanceAssignment, error) {
	var assignment AgentInstanceAssignment

	err := row.Scan(
		&assignment.ID,
		&assignment.AgentID,
		&assignment.InstanceID,
		&assignment.AssignmentRole,
		&assignment.Status,
		&assignment.AssignedAt,
		&assignment.ReleasedAt,
		&assignment.Metadata,
	)
	if err != nil {
		return nil, mapAgentInstanceAssignmentError(err)
	}

	return &assignment, nil
}

func nullableAssignmentMetadata(data json.RawMessage) any {
	if len(data) == 0 {
		return nil
	}
	return string(data)
}

func (r *AgentInstanceAssignmentRepository) Create(
	ctx context.Context,
	input CreateAgentInstanceAssignmentInput,
) (*AgentInstanceAssignment, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}

	query := `
		INSERT INTO agent_instance_assignments (
			agent_id,
			instance_id,
			assignment_role,
			status,
			assigned_at,
			released_at,
			metadata
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb)
		RETURNING ` + agentInstanceAssignmentColumns

	return scanAgentInstanceAssignment(
		r.pool.QueryRow(
			ctx,
			query,
			input.AgentID,
			input.InstanceID,
			input.AssignmentRole,
			input.Status,
			input.AssignedAt,
			input.ReleasedAt,
			nullableAssignmentMetadata(input.Metadata),
		),
	)
}

func (r *AgentInstanceAssignmentRepository) GetByID(
	ctx context.Context,
	id string,
) (*AgentInstanceAssignment, error) {
	if !validUUID(id) {
		return nil, errors.New("id must be a valid UUID")
	}

	query := `
		SELECT ` + agentInstanceAssignmentColumns + `
		FROM agent_instance_assignments
		WHERE id = $1
	`

	return scanAgentInstanceAssignment(
		r.pool.QueryRow(ctx, query, id),
	)
}

func (r *AgentInstanceAssignmentRepository) List(
	ctx context.Context,
	options ListAgentInstanceAssignmentsOptions,
) ([]AgentInstanceAssignment, error) {
	if options.AgentID != "" && !validUUID(options.AgentID) {
		return nil, errors.New("agent_id must be a valid UUID")
	}
	if options.InstanceID != "" && !validUUID(options.InstanceID) {
		return nil, errors.New("instance_id must be a valid UUID")
	}
	if options.Status != "" {
		if err := validateAssignmentStatus(options.Status); err != nil {
			return nil, err
		}
	}
	if options.Limit < 0 || options.Offset < 0 {
		return nil, errors.New("limit and offset cannot be negative")
	}

	limit := options.Limit
	if limit == 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}

	var agentID any
	var instanceID any
	var status any

	if options.AgentID != "" {
		agentID = options.AgentID
	}
	if options.InstanceID != "" {
		instanceID = options.InstanceID
	}
	if options.Status != "" {
		status = options.Status
	}

	query := `
		SELECT ` + agentInstanceAssignmentColumns + `
		FROM agent_instance_assignments
		WHERE
			($1::uuid IS NULL OR agent_id = $1::uuid)
			AND ($2::uuid IS NULL OR instance_id = $2::uuid)
			AND ($3::text IS NULL OR status = $3::text)
		ORDER BY agent_id ASC, instance_id ASC, id ASC
		LIMIT $4 OFFSET $5
	`

	rows, err := r.pool.Query(
		ctx,
		query,
		agentID,
		instanceID,
		status,
		limit,
		options.Offset,
	)
	if err != nil {
		return nil, mapAgentInstanceAssignmentError(err)
	}
	defer rows.Close()

	assignments := make([]AgentInstanceAssignment, 0)

	for rows.Next() {
		assignment, err := scanAgentInstanceAssignment(rows)
		if err != nil {
			return nil, err
		}
		assignments = append(assignments, *assignment)
	}

	if err := rows.Err(); err != nil {
		return nil, mapAgentInstanceAssignmentError(err)
	}

	return assignments, nil
}

func (r *AgentInstanceAssignmentRepository) Update(
	ctx context.Context,
	id string,
	input UpdateAgentInstanceAssignmentInput,
) (*AgentInstanceAssignment, error) {
	if !validUUID(id) {
		return nil, errors.New("id must be a valid UUID")
	}
	if err := input.Validate(); err != nil {
		return nil, err
	}

	query := `
		UPDATE agent_instance_assignments
		SET
			assignment_role = $2,
			status = $3,
			assigned_at = $4,
			released_at = $5,
			metadata = $6::jsonb
		WHERE id = $1
		RETURNING ` + agentInstanceAssignmentColumns

	return scanAgentInstanceAssignment(
		r.pool.QueryRow(
			ctx,
			query,
			id,
			input.AssignmentRole,
			input.Status,
			input.AssignedAt,
			input.ReleasedAt,
			nullableAssignmentMetadata(input.Metadata),
		),
	)
}

func (r *AgentInstanceAssignmentRepository) UpdateStatus(
	ctx context.Context,
	id string,
	status string,
) (*AgentInstanceAssignment, error) {
	if !validUUID(id) {
		return nil, errors.New("id must be a valid UUID")
	}
	if err := validateAssignmentStatus(status); err != nil {
		return nil, err
	}

	query := `
		UPDATE agent_instance_assignments
		SET status = $2
		WHERE id = $1
		RETURNING ` + agentInstanceAssignmentColumns

	return scanAgentInstanceAssignment(
		r.pool.QueryRow(ctx, query, id, status),
	)
}

func (r *AgentInstanceAssignmentRepository) Release(
	ctx context.Context,
	id string,
	releasedAt time.Time,
) (*AgentInstanceAssignment, error) {
	if !validUUID(id) {
		return nil, errors.New("id must be a valid UUID")
	}
	if releasedAt.IsZero() {
		return nil, errors.New("released_at cannot be zero")
	}

	query := `
		UPDATE agent_instance_assignments
		SET
			status = 'released',
			released_at = $2
		WHERE id = $1
		RETURNING ` + agentInstanceAssignmentColumns

	return scanAgentInstanceAssignment(
		r.pool.QueryRow(ctx, query, id, releasedAt),
	)
}

func (r *AgentInstanceAssignmentRepository) Delete(
	ctx context.Context,
	id string,
) error {
	if !validUUID(id) {
		return errors.New("id must be a valid UUID")
	}

	tag, err := r.pool.Exec(
		ctx,
		`DELETE FROM agent_instance_assignments WHERE id = $1`,
		id,
	)
	if err != nil {
		return mapAgentInstanceAssignmentError(err)
	}

	if tag.RowsAffected() == 0 {
		return ErrAgentInstanceAssignmentNotFound
	}

	return nil
}

func mapAgentInstanceAssignmentError(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, pgx.ErrNoRows) {
		return ErrAgentInstanceAssignmentNotFound
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return fmt.Errorf(
				"%w: %v",
				ErrAgentInstanceAssignmentConflict,
				err,
			)
		case "23503":
			return fmt.Errorf(
				"agent instance assignment foreign-key constraint: %w",
				err,
			)
		case "23514":
			return fmt.Errorf(
				"agent instance assignment check constraint: %w",
				err,
			)
		case "23502":
			return fmt.Errorf(
				"agent instance assignment required field: %w",
				err,
			)
		}
	}

	return err
}
