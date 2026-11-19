package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/gdiop88/RL_trader_infrastructure/internal/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrInfrastructureProjectNotFound = errors.New(
		"infrastructure project not found",
	)
	ErrInfrastructureProjectConflict = errors.New(
		"infrastructure project conflict",
	)
)

type InfrastructureProject struct {
	ID             string
	Name           string
	Environment    string
	Description    *string
	OwnerReference *string
	BudgetLimit    *string
	Currency       *string
	Metadata       json.RawMessage
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type CreateInfrastructureProjectInput struct {
	Name           string
	Environment    string
	Description    *string
	OwnerReference *string
	BudgetLimit    *string
	Currency       *string
	Metadata       json.RawMessage
}

type UpdateInfrastructureProjectInput struct {
	Name           string
	Environment    string
	Description    *string
	OwnerReference *string
	BudgetLimit    *string
	Currency       *string
	Metadata       json.RawMessage
}

type ListInfrastructureProjectsOptions struct {
	Environment    string
	OwnerReference string
	Limit          int
	Offset         int
}

var projectBudgetPattern = regexp.MustCompile(
	`^[0-9]+(?:\.[0-9]{1,2})?$`,
)

func validateInfrastructureProjectFields(
	name string,
	environment string,
	description *string,
	ownerReference *string,
	budgetLimit *string,
	currency *string,
	metadata json.RawMessage,
) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("name is required")
	}
	if len(name) > 150 {
		return errors.New("name exceeds 150 characters")
	}

	if strings.TrimSpace(environment) == "" {
		return errors.New("environment is required")
	}
	if len(environment) > 50 {
		return errors.New("environment exceeds 50 characters")
	}

	if err := validateOptionalString(
		"owner_reference", ownerReference, 255,
	); err != nil {
		return err
	}

	if err := validateOptionalString(
		"currency", currency, 10,
	); err != nil {
		return err
	}

	if description != nil && strings.ContainsRune(*description, '\x00') {
		return errors.New("description contains a null character")
	}

	if budgetLimit != nil {
		budget := strings.TrimSpace(*budgetLimit)

		if !projectBudgetPattern.MatchString(budget) {
			return errors.New(
				"budget_limit must be a nonnegative decimal with at most 2 fractional digits",
			)
		}

		parts := strings.SplitN(budget, ".", 2)
		if len(parts[0]) > 16 {
			return errors.New(
				"budget_limit exceeds NUMERIC(18,2) precision",
			)
		}
	}

	if len(metadata) > 0 {
		var value any

		if err := json.Unmarshal(metadata, &value); err != nil {
			return fmt.Errorf("invalid metadata JSON: %w", err)
		}

		if _, ok := value.(map[string]any); !ok {
			return errors.New("metadata must be a JSON object")
		}
	}

	return nil
}

func (in CreateInfrastructureProjectInput) Validate() error {
	return validateInfrastructureProjectFields(
		in.Name,
		in.Environment,
		in.Description,
		in.OwnerReference,
		in.BudgetLimit,
		in.Currency,
		in.Metadata,
	)
}

func (in UpdateInfrastructureProjectInput) Validate() error {
	return validateInfrastructureProjectFields(
		in.Name,
		in.Environment,
		in.Description,
		in.OwnerReference,
		in.BudgetLimit,
		in.Currency,
		in.Metadata,
	)
}

type InfrastructureProjectRepository struct {
	pool *pgxpool.Pool
}

func NewInfrastructureProjectRepository(
	db *database.Database,
) (*InfrastructureProjectRepository, error) {
	if db == nil || db.Pool() == nil {
		return nil, errors.New("database connection is required")
	}

	return &InfrastructureProjectRepository{
		pool: db.Pool(),
	}, nil
}

const infrastructureProjectColumns = `
	id,
	name,
	environment,
	description,
	owner_reference,
	budget_limit::text,
	currency,
	metadata,
	created_at,
	updated_at
`

func scanInfrastructureProject(
	row pgx.Row,
) (*InfrastructureProject, error) {
	var project InfrastructureProject

	err := row.Scan(
		&project.ID,
		&project.Name,
		&project.Environment,
		&project.Description,
		&project.OwnerReference,
		&project.BudgetLimit,
		&project.Currency,
		&project.Metadata,
		&project.CreatedAt,
		&project.UpdatedAt,
	)
	if err != nil {
		return nil, mapInfrastructureProjectError(err)
	}

	return &project, nil
}

func projectMetadataArgument(metadata json.RawMessage) any {
	if len(metadata) == 0 {
		return nil
	}
	return string(metadata)
}

func (r *InfrastructureProjectRepository) Create(
	ctx context.Context,
	input CreateInfrastructureProjectInput,
) (*InfrastructureProject, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}

	query := `
		INSERT INTO infrastructure_projects (
			name,
			environment,
			description,
			owner_reference,
			budget_limit,
			currency,
			metadata
		)
		VALUES (
			$1, $2, $3, $4,
			$5::numeric, $6, $7::jsonb
		)
		RETURNING ` + infrastructureProjectColumns

	return scanInfrastructureProject(
		r.pool.QueryRow(
			ctx,
			query,
			input.Name,
			input.Environment,
			input.Description,
			input.OwnerReference,
			input.BudgetLimit,
			input.Currency,
			projectMetadataArgument(input.Metadata),
		),
	)
}

func (r *InfrastructureProjectRepository) GetByID(
	ctx context.Context,
	id string,
) (*InfrastructureProject, error) {
	if !validUUID(id) {
		return nil, errors.New("id must be a valid UUID")
	}

	query := `
		SELECT ` + infrastructureProjectColumns + `
		FROM infrastructure_projects
		WHERE id = $1
	`

	return scanInfrastructureProject(
		r.pool.QueryRow(ctx, query, id),
	)
}

func (r *InfrastructureProjectRepository) GetByName(
	ctx context.Context,
	name string,
) (*InfrastructureProject, error) {
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("name is required")
	}
	if len(name) > 150 {
		return nil, errors.New("name exceeds 150 characters")
	}

	query := `
		SELECT ` + infrastructureProjectColumns + `
		FROM infrastructure_projects
		WHERE name = $1
	`

	return scanInfrastructureProject(
		r.pool.QueryRow(ctx, query, name),
	)
}

func (r *InfrastructureProjectRepository) List(
	ctx context.Context,
	options ListInfrastructureProjectsOptions,
) ([]InfrastructureProject, error) {
	if options.Limit < 0 || options.Offset < 0 {
		return nil, errors.New(
			"limit and offset cannot be negative",
		)
	}
	if len(options.Environment) > 50 {
		return nil, errors.New(
			"environment exceeds 50 characters",
		)
	}
	if len(options.OwnerReference) > 255 {
		return nil, errors.New(
			"owner_reference exceeds 255 characters",
		)
	}

	limit := options.Limit
	if limit == 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}

	var environment any
	if options.Environment != "" {
		environment = options.Environment
	}

	var ownerReference any
	if options.OwnerReference != "" {
		ownerReference = options.OwnerReference
	}

	query := `
		SELECT ` + infrastructureProjectColumns + `
		FROM infrastructure_projects
		WHERE (
			$1::text IS NULL
			OR environment = $1::text
		)
		AND (
			$2::text IS NULL
			OR owner_reference = $2::text
		)
		ORDER BY created_at DESC, id
		LIMIT $3 OFFSET $4
	`

	rows, err := r.pool.Query(
		ctx,
		query,
		environment,
		ownerReference,
		limit,
		options.Offset,
	)
	if err != nil {
		return nil, mapInfrastructureProjectError(err)
	}
	defer rows.Close()

	projects := make([]InfrastructureProject, 0)

	for rows.Next() {
		var project InfrastructureProject

		err := rows.Scan(
			&project.ID,
			&project.Name,
			&project.Environment,
			&project.Description,
			&project.OwnerReference,
			&project.BudgetLimit,
			&project.Currency,
			&project.Metadata,
			&project.CreatedAt,
			&project.UpdatedAt,
		)
		if err != nil {
			return nil, mapInfrastructureProjectError(err)
		}

		projects = append(projects, project)
	}

	if err := rows.Err(); err != nil {
		return nil, mapInfrastructureProjectError(err)
	}

	return projects, nil
}

func (r *InfrastructureProjectRepository) Update(
	ctx context.Context,
	id string,
	input UpdateInfrastructureProjectInput,
) (*InfrastructureProject, error) {
	if !validUUID(id) {
		return nil, errors.New("id must be a valid UUID")
	}
	if err := input.Validate(); err != nil {
		return nil, err
	}

	query := `
		UPDATE infrastructure_projects
		SET
			name = $2,
			environment = $3,
			description = $4,
			owner_reference = $5,
			budget_limit = $6::numeric,
			currency = $7,
			metadata = $8::jsonb,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
		RETURNING ` + infrastructureProjectColumns

	return scanInfrastructureProject(
		r.pool.QueryRow(
			ctx,
			query,
			id,
			input.Name,
			input.Environment,
			input.Description,
			input.OwnerReference,
			input.BudgetLimit,
			input.Currency,
			projectMetadataArgument(input.Metadata),
		),
	)
}

func (r *InfrastructureProjectRepository) Delete(
	ctx context.Context,
	id string,
) error {
	if !validUUID(id) {
		return errors.New("id must be a valid UUID")
	}

	tag, err := r.pool.Exec(
		ctx,
		`DELETE FROM infrastructure_projects WHERE id = $1`,
		id,
	)
	if err != nil {
		return mapInfrastructureProjectError(err)
	}

	if tag.RowsAffected() == 0 {
		return ErrInfrastructureProjectNotFound
	}

	return nil
}

func mapInfrastructureProjectError(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, pgx.ErrNoRows) {
		return ErrInfrastructureProjectNotFound
	}

	var pgErr *pgconn.PgError

	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return fmt.Errorf(
				"%w: %v",
				ErrInfrastructureProjectConflict,
				err,
			)

		case "23503":
			return fmt.Errorf(
				"infrastructure project foreign-key constraint: %w",
				err,
			)

		case "23514":
			return fmt.Errorf(
				"infrastructure project check constraint: %w",
				err,
			)

		case "22003":
			return fmt.Errorf(
				"infrastructure project numeric value out of range: %w",
				err,
			)
		}
	}

	return err
}
