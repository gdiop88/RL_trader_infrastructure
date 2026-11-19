package persistence

import (
	"context"
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
	ErrProviderNotFound = errors.New("cloud provider not found")
	ErrProviderConflict = errors.New("cloud provider conflict")
)

type CloudProvider struct {
	ID           string    `json:"id"`
	Code         string    `json:"code"`
	Name         string    `json:"name"`
	ProviderType string    `json:"provider_type"`
	Enabled      bool      `json:"enabled"`
	APIEndpoint  *string   `json:"api_endpoint,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type CreateProviderInput struct {
	Code         string
	Name         string
	ProviderType string
	APIEndpoint  *string
}

type UpdateProviderInput struct {
	Name         string
	ProviderType string
	APIEndpoint  *string
}

type ListProvidersOptions struct {
	Enabled *bool
	Limit   int
	Offset  int
}

func validateProviderFields(
	name, providerType string,
	endpoint *string,
) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("provider name is required")
	}
	if len(name) > 100 {
		return errors.New("provider name exceeds 100 characters")
	}
	if strings.TrimSpace(providerType) == "" {
		return errors.New("provider type is required")
	}
	if len(providerType) > 50 {
		return errors.New("provider type exceeds 50 characters")
	}
	return validateOptionalString("API endpoint", endpoint, 500)
}

func (input CreateProviderInput) Validate() error {
	if strings.TrimSpace(input.Code) == "" {
		return errors.New("provider code is required")
	}
	if len(input.Code) > 50 {
		return errors.New("provider code exceeds 50 characters")
	}
	return validateProviderFields(
		input.Name, input.ProviderType, input.APIEndpoint,
	)
}

func (input UpdateProviderInput) Validate() error {
	return validateProviderFields(
		input.Name, input.ProviderType, input.APIEndpoint,
	)
}

type ProviderRepository struct {
	pool *pgxpool.Pool
}

func NewProviderRepository(
	db *database.Database,
) (*ProviderRepository, error) {
	if db == nil || db.Pool() == nil {
		return nil, errors.New("database pool is not initialized")
	}
	return &ProviderRepository{pool: db.Pool()}, nil
}

const providerColumns = `
	id,
	code,
	name,
	provider_type,
	enabled,
	api_endpoint,
	created_at,
	updated_at
`

func scanProvider(row pgx.Row) (*CloudProvider, error) {
	var provider CloudProvider
	err := row.Scan(
		&provider.ID,
		&provider.Code,
		&provider.Name,
		&provider.ProviderType,
		&provider.Enabled,
		&provider.APIEndpoint,
		&provider.CreatedAt,
		&provider.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &provider, nil
}

func mapProviderError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrProviderNotFound
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return fmt.Errorf("%w: %v", ErrProviderConflict, err)
		case "23503":
			return fmt.Errorf(
				"provider is referenced by other resources: %w",
				err,
			)
		}
	}
	return err
}

func validateProviderID(id string) error {
	if !validUUID(id) {
		return errors.New("provider ID must be a valid UUID")
	}
	return nil
}

func (r *ProviderRepository) Create(
	ctx context.Context,
	input CreateProviderInput,
) (*CloudProvider, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}

	query := `
		INSERT INTO cloud_providers (
			code, name, provider_type, api_endpoint
		)
		VALUES ($1, $2, $3, $4)
		RETURNING ` + providerColumns

	provider, err := scanProvider(r.pool.QueryRow(
		ctx,
		query,
		strings.TrimSpace(input.Code),
		strings.TrimSpace(input.Name),
		strings.TrimSpace(input.ProviderType),
		input.APIEndpoint,
	))
	if err != nil {
		return nil, mapProviderError(err)
	}
	return provider, nil
}

func (r *ProviderRepository) GetByID(
	ctx context.Context,
	id string,
) (*CloudProvider, error) {
	if err := validateProviderID(id); err != nil {
		return nil, err
	}

	query := `
		SELECT ` + providerColumns + `
		FROM cloud_providers
		WHERE id = $1::uuid
	`

	provider, err := scanProvider(r.pool.QueryRow(ctx, query, id))
	if err != nil {
		return nil, mapProviderError(err)
	}
	return provider, nil
}

func (r *ProviderRepository) GetByCode(
	ctx context.Context,
	code string,
) (*CloudProvider, error) {
	if strings.TrimSpace(code) == "" {
		return nil, errors.New("provider code is required")
	}

	query := `
		SELECT ` + providerColumns + `
		FROM cloud_providers
		WHERE code = $1
	`

	provider, err := scanProvider(
		r.pool.QueryRow(ctx, query, strings.TrimSpace(code)),
	)
	if err != nil {
		return nil, mapProviderError(err)
	}
	return provider, nil
}

func (r *ProviderRepository) List(
	ctx context.Context,
	options ListProvidersOptions,
) ([]CloudProvider, error) {
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

	query := `
		SELECT ` + providerColumns + `
		FROM cloud_providers
		WHERE ($1::boolean IS NULL OR enabled = $1::boolean)
		ORDER BY code ASC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.pool.Query(
		ctx, query, options.Enabled, limit, options.Offset,
	)
	if err != nil {
		return nil, mapProviderError(err)
	}
	defer rows.Close()

	providers := make([]CloudProvider, 0)
	for rows.Next() {
		provider, err := scanProvider(rows)
		if err != nil {
			return nil, err
		}
		providers = append(providers, *provider)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return providers, nil
}

func (r *ProviderRepository) Update(
	ctx context.Context,
	id string,
	input UpdateProviderInput,
) (*CloudProvider, error) {
	if err := validateProviderID(id); err != nil {
		return nil, err
	}
	if err := input.Validate(); err != nil {
		return nil, err
	}

	query := `
		UPDATE cloud_providers
		SET name = $2,
		    provider_type = $3,
		    api_endpoint = $4,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = $1::uuid
		RETURNING ` + providerColumns

	provider, err := scanProvider(r.pool.QueryRow(
		ctx, query, id,
		strings.TrimSpace(input.Name),
		strings.TrimSpace(input.ProviderType),
		input.APIEndpoint,
	))
	if err != nil {
		return nil, mapProviderError(err)
	}
	return provider, nil
}

func (r *ProviderRepository) SetEnabled(
	ctx context.Context,
	id string,
	enabled bool,
) (*CloudProvider, error) {
	if err := validateProviderID(id); err != nil {
		return nil, err
	}

	query := `
		UPDATE cloud_providers
		SET enabled = $2,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = $1::uuid
		RETURNING ` + providerColumns

	provider, err := scanProvider(
		r.pool.QueryRow(ctx, query, id, enabled),
	)
	if err != nil {
		return nil, mapProviderError(err)
	}
	return provider, nil
}

func (r *ProviderRepository) Delete(
	ctx context.Context,
	id string,
) error {
	if err := validateProviderID(id); err != nil {
		return err
	}

	tag, err := r.pool.Exec(
		ctx,
		`DELETE FROM cloud_providers WHERE id = $1::uuid`,
		id,
	)
	if err != nil {
		return mapProviderError(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrProviderNotFound
	}
	return nil
}
