package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/gdiop88/RL_trader_infrastructure/internal/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrProviderRegionNotFound = errors.New("provider region not found")
	ErrProviderRegionConflict = errors.New("provider region conflict")
)

type ProviderRegion struct {
	ID           string
	ProviderID   string
	Code         string
	DisplayName  *string
	Enabled      bool
	Capabilities json.RawMessage
}

type CreateProviderRegionInput struct {
	ProviderID   string
	Code         string
	DisplayName  *string
	Capabilities json.RawMessage
}

type UpdateProviderRegionInput struct {
	DisplayName  *string
	Capabilities json.RawMessage
}

type ListProviderRegionsOptions struct {
	ProviderID string
	Enabled    *bool
	Limit      int
	Offset     int
}

func (in CreateProviderRegionInput) Validate() error {
	if !validUUID(in.ProviderID) {
		return errors.New("provider_id must be a valid UUID")
	}

	if strings.TrimSpace(in.Code) == "" {
		return errors.New("code is required")
	}

	if len(in.Code) > 100 {
		return errors.New("code exceeds 100 characters")
	}

	if err := validateOptionalString(
		"display_name", in.DisplayName, 150,
	); err != nil {
		return err
	}

	return validateRegionCapabilities(in.Capabilities)
}

func (in UpdateProviderRegionInput) Validate() error {
	if err := validateOptionalString(
		"display_name", in.DisplayName, 150,
	); err != nil {
		return err
	}

	return validateRegionCapabilities(in.Capabilities)
}

func validateRegionCapabilities(value json.RawMessage) error {
	if len(value) == 0 {
		return nil
	}

	if !json.Valid(value) {
		return errors.New("capabilities must contain valid JSON")
	}

	var object map[string]json.RawMessage

	if err := json.Unmarshal(value, &object); err != nil ||
		object == nil {
		return errors.New("capabilities must be a JSON object")
	}

	return nil
}

type ProviderRegionRepository struct {
	pool *pgxpool.Pool
}

func NewProviderRegionRepository(
	db *database.Database,
) (*ProviderRegionRepository, error) {
	if db == nil || db.Pool() == nil {
		return nil, errors.New("database connection is required")
	}

	return &ProviderRegionRepository{
		pool: db.Pool(),
	}, nil
}

const providerRegionColumns = `
	id,
	provider_id,
	code,
	display_name,
	enabled,
	capabilities
`

func scanProviderRegion(row pgx.Row) (*ProviderRegion, error) {
	var region ProviderRegion

	err := row.Scan(
		&region.ID,
		&region.ProviderID,
		&region.Code,
		&region.DisplayName,
		&region.Enabled,
		&region.Capabilities,
	)
	if err != nil {
		return nil, mapProviderRegionError(err)
	}

	return &region, nil
}

func (r *ProviderRegionRepository) Create(
	ctx context.Context,
	input CreateProviderRegionInput,
) (*ProviderRegion, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}

	query := `
		INSERT INTO provider_regions (
			provider_id,
			code,
			display_name,
			capabilities
		)
		VALUES ($1, $2, $3, $4::jsonb)
		RETURNING ` + providerRegionColumns

	return scanProviderRegion(r.pool.QueryRow(
		ctx,
		query,
		input.ProviderID,
		input.Code,
		input.DisplayName,
		jsonOrDefault(input.Capabilities, "{}"),
	))
}

func (r *ProviderRegionRepository) GetByID(
	ctx context.Context,
	id string,
) (*ProviderRegion, error) {
	if !validUUID(id) {
		return nil, errors.New("id must be a valid UUID")
	}

	query := `
		SELECT ` + providerRegionColumns + `
		FROM provider_regions
		WHERE id = $1
	`

	return scanProviderRegion(
		r.pool.QueryRow(ctx, query, id),
	)
}

func (r *ProviderRegionRepository) GetByCode(
	ctx context.Context,
	providerID string,
	code string,
) (*ProviderRegion, error) {
	if !validUUID(providerID) {
		return nil, errors.New("provider_id must be a valid UUID")
	}

	if strings.TrimSpace(code) == "" {
		return nil, errors.New("code is required")
	}

	if len(code) > 100 {
		return nil, errors.New("code exceeds 100 characters")
	}

	query := `
		SELECT ` + providerRegionColumns + `
		FROM provider_regions
		WHERE provider_id = $1
		  AND code = $2
	`

	return scanProviderRegion(
		r.pool.QueryRow(ctx, query, providerID, code),
	)
}

func (r *ProviderRegionRepository) List(
	ctx context.Context,
	options ListProviderRegionsOptions,
) ([]ProviderRegion, error) {
	if options.ProviderID != "" &&
		!validUUID(options.ProviderID) {
		return nil, errors.New("provider_id must be a valid UUID")
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

	var providerID any
	if options.ProviderID != "" {
		providerID = options.ProviderID
	}

	query := `
		SELECT ` + providerRegionColumns + `
		FROM provider_regions
		WHERE (
			$1::uuid IS NULL
			OR provider_id = $1::uuid
		)
		AND (
			$2::boolean IS NULL
			OR enabled = $2::boolean
		)
		ORDER BY provider_id, code
		LIMIT $3 OFFSET $4
	`

	rows, err := r.pool.Query(
		ctx,
		query,
		providerID,
		options.Enabled,
		limit,
		options.Offset,
	)
	if err != nil {
		return nil, mapProviderRegionError(err)
	}
	defer rows.Close()

	regions := make([]ProviderRegion, 0)

	for rows.Next() {
		var region ProviderRegion

		if err := rows.Scan(
			&region.ID,
			&region.ProviderID,
			&region.Code,
			&region.DisplayName,
			&region.Enabled,
			&region.Capabilities,
		); err != nil {
			return nil, mapProviderRegionError(err)
		}

		regions = append(regions, region)
	}

	if err := rows.Err(); err != nil {
		return nil, mapProviderRegionError(err)
	}

	return regions, nil
}

func (r *ProviderRegionRepository) Update(
	ctx context.Context,
	id string,
	input UpdateProviderRegionInput,
) (*ProviderRegion, error) {
	if !validUUID(id) {
		return nil, errors.New("id must be a valid UUID")
	}

	if err := input.Validate(); err != nil {
		return nil, err
	}

	query := `
		UPDATE provider_regions
		SET
			display_name = $2,
			capabilities = $3::jsonb
		WHERE id = $1
		RETURNING ` + providerRegionColumns

	return scanProviderRegion(r.pool.QueryRow(
		ctx,
		query,
		id,
		input.DisplayName,
		jsonOrDefault(input.Capabilities, "{}"),
	))
}

func (r *ProviderRegionRepository) SetEnabled(
	ctx context.Context,
	id string,
	enabled bool,
) (*ProviderRegion, error) {
	if !validUUID(id) {
		return nil, errors.New("id must be a valid UUID")
	}

	query := `
		UPDATE provider_regions
		SET enabled = $2
		WHERE id = $1
		RETURNING ` + providerRegionColumns

	return scanProviderRegion(
		r.pool.QueryRow(ctx, query, id, enabled),
	)
}

func (r *ProviderRegionRepository) Delete(
	ctx context.Context,
	id string,
) error {
	if !validUUID(id) {
		return errors.New("id must be a valid UUID")
	}

	tag, err := r.pool.Exec(
		ctx,
		`DELETE FROM provider_regions WHERE id = $1`,
		id,
	)
	if err != nil {
		return mapProviderRegionError(err)
	}

	if tag.RowsAffected() == 0 {
		return ErrProviderRegionNotFound
	}

	return nil
}

func mapProviderRegionError(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, pgx.ErrNoRows) {
		return ErrProviderRegionNotFound
	}

	var pgErr *pgconn.PgError

	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return fmt.Errorf(
				"%w: %v",
				ErrProviderRegionConflict,
				err,
			)

		case "23503":
			return fmt.Errorf(
				"provider region foreign-key constraint: %w",
				err,
			)
		}
	}

	return err
}
