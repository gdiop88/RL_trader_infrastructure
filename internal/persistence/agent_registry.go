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
	ErrAgentRegistryNotFound = errors.New("agent registry entry not found")
	ErrAgentRegistryConflict = errors.New("agent registry entry conflict")
)

type AgentRegistry struct {
	ID             string
	AgentReference string
	AgentType      *string
	OwnerReference *string
	Status         *string
	Capabilities   json.RawMessage
	RegisteredAt   *time.Time
	LastSeenAt     *time.Time
}

type CreateAgentRegistryInput struct {
	AgentReference string
	AgentType      *string
	OwnerReference *string
	Status         *string
	Capabilities   json.RawMessage
	RegisteredAt   *time.Time
	LastSeenAt     *time.Time
}

type UpdateAgentRegistryInput struct {
	AgentType      *string
	OwnerReference *string
	Status         *string
	Capabilities   json.RawMessage
	RegisteredAt   *time.Time
	LastSeenAt     *time.Time
}

type ListAgentRegistryOptions struct {
	AgentType      string
	OwnerReference string
	Status         string
	Limit          int
	Offset         int
}

type AgentRegistryRepository struct {
	pool *pgxpool.Pool
}

func NewAgentRegistryRepository(
	db *database.Database,
) (*AgentRegistryRepository, error) {
	if db == nil || db.Pool() == nil {
		return nil, errors.New("database connection is required")
	}

	return &AgentRegistryRepository{pool: db.Pool()}, nil
}

func validateAgentRegistryReference(value string) error {
	if strings.TrimSpace(value) == "" ||
		len(value) > 255 ||
		strings.ContainsRune(value, '\x00') {
		return errors.New(
			"agent_reference must contain 1 to 255 valid characters",
		)
	}
	return nil
}

func validateAgentRegistryOptional(
	name string,
	value *string,
	maxLength int,
) error {
	if err := validateOptionalString(
		name, value, maxLength,
	); err != nil {
		return err
	}

	if value != nil && strings.ContainsRune(*value, '\x00') {
		return fmt.Errorf("%s contains a null character", name)
	}

	return nil
}

func validateAgentRegistryFields(
	agentType *string,
	ownerReference *string,
	status *string,
	capabilities json.RawMessage,
	registeredAt *time.Time,
	lastSeenAt *time.Time,
) error {
	if err := validateAgentRegistryOptional(
		"agent_type", agentType, 100,
	); err != nil {
		return err
	}

	if err := validateAgentRegistryOptional(
		"owner_reference", ownerReference, 255,
	); err != nil {
		return err
	}

	if err := validateAgentRegistryOptional(
		"status", status, 50,
	); err != nil {
		return err
	}

	if len(capabilities) > 0 && !json.Valid(capabilities) {
		return errors.New("capabilities contains invalid JSON")
	}

	if registeredAt != nil && registeredAt.IsZero() {
		return errors.New("registered_at cannot be zero")
	}

	if lastSeenAt != nil && lastSeenAt.IsZero() {
		return errors.New("last_seen_at cannot be zero")
	}

	return nil
}

func (in CreateAgentRegistryInput) Validate() error {
	if err := validateAgentRegistryReference(
		in.AgentReference,
	); err != nil {
		return err
	}

	return validateAgentRegistryFields(
		in.AgentType,
		in.OwnerReference,
		in.Status,
		in.Capabilities,
		in.RegisteredAt,
		in.LastSeenAt,
	)
}

func (in UpdateAgentRegistryInput) Validate() error {
	return validateAgentRegistryFields(
		in.AgentType,
		in.OwnerReference,
		in.Status,
		in.Capabilities,
		in.RegisteredAt,
		in.LastSeenAt,
	)
}

const agentRegistryColumns = `
	id,
	agent_reference,
	agent_type,
	owner_reference,
	status,
	capabilities,
	registered_at,
	last_seen_at
`

func scanAgentRegistry(row pgx.Row) (*AgentRegistry, error) {
	var agent AgentRegistry

	err := row.Scan(
		&agent.ID,
		&agent.AgentReference,
		&agent.AgentType,
		&agent.OwnerReference,
		&agent.Status,
		&agent.Capabilities,
		&agent.RegisteredAt,
		&agent.LastSeenAt,
	)
	if err != nil {
		return nil, mapAgentRegistryError(err)
	}

	return &agent, nil
}

func nullableAgentCapabilities(data json.RawMessage) any {
	if len(data) == 0 {
		return nil
	}
	return string(data)
}

func (r *AgentRegistryRepository) Create(
	ctx context.Context,
	input CreateAgentRegistryInput,
) (*AgentRegistry, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}

	query := `
		INSERT INTO agent_registry (
			agent_reference,
			agent_type,
			owner_reference,
			status,
			capabilities,
			registered_at,
			last_seen_at
		)
		VALUES (
			$1, $2, $3, $4, $5::jsonb, $6, $7
		)
		RETURNING ` + agentRegistryColumns

	return scanAgentRegistry(
		r.pool.QueryRow(
			ctx,
			query,
			input.AgentReference,
			input.AgentType,
			input.OwnerReference,
			input.Status,
			nullableAgentCapabilities(input.Capabilities),
			input.RegisteredAt,
			input.LastSeenAt,
		),
	)
}

func (r *AgentRegistryRepository) GetByID(
	ctx context.Context,
	id string,
) (*AgentRegistry, error) {
	if !validUUID(id) {
		return nil, errors.New("id must be a valid UUID")
	}

	query := `
		SELECT ` + agentRegistryColumns + `
		FROM agent_registry
		WHERE id = $1
	`

	return scanAgentRegistry(
		r.pool.QueryRow(ctx, query, id),
	)
}

func (r *AgentRegistryRepository) GetByReference(
	ctx context.Context,
	reference string,
) (*AgentRegistry, error) {
	if err := validateAgentRegistryReference(
		reference,
	); err != nil {
		return nil, err
	}

	query := `
		SELECT ` + agentRegistryColumns + `
		FROM agent_registry
		WHERE agent_reference = $1
	`

	return scanAgentRegistry(
		r.pool.QueryRow(ctx, query, reference),
	)
}

func (r *AgentRegistryRepository) List(
	ctx context.Context,
	options ListAgentRegistryOptions,
) ([]AgentRegistry, error) {
	if options.Limit < 0 || options.Offset < 0 {
		return nil, errors.New(
			"limit and offset cannot be negative",
		)
	}

	for _, field := range []struct {
		name  string
		value string
		max   int
	}{
		{"agent_type", options.AgentType, 100},
		{"owner_reference", options.OwnerReference, 255},
		{"status", options.Status, 50},
	} {
		if field.value != "" {
			if err := validateAgentRegistryOptional(
				field.name, &field.value, field.max,
			); err != nil {
				return nil, err
			}
		}
	}

	limit := options.Limit
	if limit == 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}

	var agentType any
	var ownerReference any
	var status any

	if options.AgentType != "" {
		agentType = options.AgentType
	}
	if options.OwnerReference != "" {
		ownerReference = options.OwnerReference
	}
	if options.Status != "" {
		status = options.Status
	}

	query := `
		SELECT ` + agentRegistryColumns + `
		FROM agent_registry
		WHERE
			($1::text IS NULL OR agent_type = $1::text)
			AND ($2::text IS NULL OR owner_reference = $2::text)
			AND ($3::text IS NULL OR status = $3::text)
		ORDER BY agent_reference ASC, id ASC
		LIMIT $4 OFFSET $5
	`

	rows, err := r.pool.Query(
		ctx,
		query,
		agentType,
		ownerReference,
		status,
		limit,
		options.Offset,
	)
	if err != nil {
		return nil, mapAgentRegistryError(err)
	}
	defer rows.Close()

	agents := make([]AgentRegistry, 0)

	for rows.Next() {
		agent, err := scanAgentRegistry(rows)
		if err != nil {
			return nil, err
		}
		agents = append(agents, *agent)
	}

	if err := rows.Err(); err != nil {
		return nil, mapAgentRegistryError(err)
	}

	return agents, nil
}

func (r *AgentRegistryRepository) Update(
	ctx context.Context,
	id string,
	input UpdateAgentRegistryInput,
) (*AgentRegistry, error) {
	if !validUUID(id) {
		return nil, errors.New("id must be a valid UUID")
	}

	if err := input.Validate(); err != nil {
		return nil, err
	}

	query := `
		UPDATE agent_registry
		SET
			agent_type = $2,
			owner_reference = $3,
			status = $4,
			capabilities = $5::jsonb,
			registered_at = $6,
			last_seen_at = $7
		WHERE id = $1
		RETURNING ` + agentRegistryColumns

	return scanAgentRegistry(
		r.pool.QueryRow(
			ctx,
			query,
			id,
			input.AgentType,
			input.OwnerReference,
			input.Status,
			nullableAgentCapabilities(input.Capabilities),
			input.RegisteredAt,
			input.LastSeenAt,
		),
	)
}

func (r *AgentRegistryRepository) UpdateStatus(
	ctx context.Context,
	id string,
	status *string,
) (*AgentRegistry, error) {
	if !validUUID(id) {
		return nil, errors.New("id must be a valid UUID")
	}

	if err := validateAgentRegistryOptional(
		"status", status, 50,
	); err != nil {
		return nil, err
	}

	query := `
		UPDATE agent_registry
		SET status = $2
		WHERE id = $1
		RETURNING ` + agentRegistryColumns

	return scanAgentRegistry(
		r.pool.QueryRow(ctx, query, id, status),
	)
}

func (r *AgentRegistryRepository) SetLastSeen(
	ctx context.Context,
	id string,
	lastSeenAt *time.Time,
) (*AgentRegistry, error) {
	if !validUUID(id) {
		return nil, errors.New("id must be a valid UUID")
	}

	if lastSeenAt != nil && lastSeenAt.IsZero() {
		return nil, errors.New("last_seen_at cannot be zero")
	}

	query := `
		UPDATE agent_registry
		SET last_seen_at = $2
		WHERE id = $1
		RETURNING ` + agentRegistryColumns

	return scanAgentRegistry(
		r.pool.QueryRow(ctx, query, id, lastSeenAt),
	)
}

func (r *AgentRegistryRepository) Delete(
	ctx context.Context,
	id string,
) error {
	if !validUUID(id) {
		return errors.New("id must be a valid UUID")
	}

	tag, err := r.pool.Exec(
		ctx,
		`DELETE FROM agent_registry WHERE id = $1`,
		id,
	)
	if err != nil {
		return mapAgentRegistryError(err)
	}

	if tag.RowsAffected() == 0 {
		return ErrAgentRegistryNotFound
	}

	return nil
}

func mapAgentRegistryError(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, pgx.ErrNoRows) {
		return ErrAgentRegistryNotFound
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return fmt.Errorf(
				"%w: %v",
				ErrAgentRegistryConflict, err,
			)
		case "23503":
			return fmt.Errorf(
				"agent registry foreign-key constraint: %w",
				err,
			)
		case "23514":
			return fmt.Errorf(
				"agent registry check constraint: %w",
				err,
			)
		case "23502":
			return fmt.Errorf(
				"agent registry required field: %w",
				err,
			)
		}
	}

	return err
}
