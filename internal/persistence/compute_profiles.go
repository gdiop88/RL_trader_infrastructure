package persistence

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/gdiop88/RL_trader_infrastructure/internal/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrComputeProfileNotFound = errors.New("compute profile not found")
	ErrComputeProfileConflict = errors.New("compute profile conflict")
)

type ComputeProfile struct {
	ID           string
	ProviderID   string
	Name         string
	ProviderSize string
	VCPUCount    *int32
	MemoryMB     *int32
	DiskGB       *int32
	GPUCount     *int32
	GPUModel     *string
	Architecture *string
	HourlyPrice  *string
	Currency     *string
	Enabled      bool
}

type CreateComputeProfileInput struct {
	ProviderID   string
	Name         string
	ProviderSize string
	VCPUCount    *int32
	MemoryMB     *int32
	DiskGB       *int32
	GPUCount     *int32
	GPUModel     *string
	Architecture *string
	HourlyPrice  *string
	Currency     *string
}

type UpdateComputeProfileInput struct {
	Name         string
	VCPUCount    *int32
	MemoryMB     *int32
	DiskGB       *int32
	GPUCount     *int32
	GPUModel     *string
	Architecture *string
	HourlyPrice  *string
	Currency     *string
}

type ListComputeProfilesOptions struct {
	ProviderID string
	Enabled    *bool
	Limit      int
	Offset     int
}

var computePricePattern = regexp.MustCompile(
	`^(?:0|[0-9]+)(?:\.[0-9]{1,6})?$`,
)

func validateComputeProfileFields(
	name string,
	vcpuCount *int32,
	memoryMB *int32,
	diskGB *int32,
	gpuCount *int32,
	gpuModel *string,
	architecture *string,
	hourlyPrice *string,
	currency *string,
) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("name is required")
	}
	if len(name) > 150 {
		return errors.New("name exceeds 150 characters")
	}

	for _, field := range []struct {
		name  string
		value *int32
	}{
		{"vcpu_count", vcpuCount},
		{"memory_mb", memoryMB},
		{"disk_gb", diskGB},
		{"gpu_count", gpuCount},
	} {
		if field.value != nil && *field.value < 0 {
			return fmt.Errorf("%s cannot be negative", field.name)
		}
	}

	for _, field := range []struct {
		name   string
		value  *string
		length int
	}{
		{"gpu_model", gpuModel, 150},
		{"architecture", architecture, 50},
		{"currency", currency, 10},
	} {
		if err := validateOptionalString(
			field.name,
			field.value,
			field.length,
		); err != nil {
			return err
		}
	}

	if hourlyPrice != nil {
		price := strings.TrimSpace(*hourlyPrice)
		if !computePricePattern.MatchString(price) {
			return errors.New(
				"hourly_price must be a nonnegative decimal with at most 6 fractional digits",
			)
		}

		parts := strings.SplitN(price, ".", 2)
		if len(parts[0]) > 12 {
			return errors.New(
				"hourly_price exceeds NUMERIC(18,6) precision",
			)
		}
	}

	return nil
}

func (in CreateComputeProfileInput) Validate() error {
	if !validUUID(in.ProviderID) {
		return errors.New("provider_id must be a valid UUID")
	}

	if strings.TrimSpace(in.ProviderSize) == "" {
		return errors.New("provider_size is required")
	}
	if len(in.ProviderSize) > 150 {
		return errors.New("provider_size exceeds 150 characters")
	}

	return validateComputeProfileFields(
		in.Name,
		in.VCPUCount,
		in.MemoryMB,
		in.DiskGB,
		in.GPUCount,
		in.GPUModel,
		in.Architecture,
		in.HourlyPrice,
		in.Currency,
	)
}

func (in UpdateComputeProfileInput) Validate() error {
	return validateComputeProfileFields(
		in.Name,
		in.VCPUCount,
		in.MemoryMB,
		in.DiskGB,
		in.GPUCount,
		in.GPUModel,
		in.Architecture,
		in.HourlyPrice,
		in.Currency,
	)
}

type ComputeProfileRepository struct {
	pool *pgxpool.Pool
}

func NewComputeProfileRepository(
	db *database.Database,
) (*ComputeProfileRepository, error) {
	if db == nil || db.Pool() == nil {
		return nil, errors.New("database connection is required")
	}
	return &ComputeProfileRepository{pool: db.Pool()}, nil
}

const computeProfileColumns = `
	id,
	provider_id,
	name,
	provider_size,
	vcpu_count,
	memory_mb,
	disk_gb,
	gpu_count,
	gpu_model,
	architecture,
	hourly_price::text,
	currency,
	enabled
`

func scanComputeProfile(row pgx.Row) (*ComputeProfile, error) {
	var profile ComputeProfile

	err := row.Scan(
		&profile.ID,
		&profile.ProviderID,
		&profile.Name,
		&profile.ProviderSize,
		&profile.VCPUCount,
		&profile.MemoryMB,
		&profile.DiskGB,
		&profile.GPUCount,
		&profile.GPUModel,
		&profile.Architecture,
		&profile.HourlyPrice,
		&profile.Currency,
		&profile.Enabled,
	)
	if err != nil {
		return nil, mapComputeProfileError(err)
	}

	return &profile, nil
}

func (r *ComputeProfileRepository) Create(
	ctx context.Context,
	input CreateComputeProfileInput,
) (*ComputeProfile, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}

	query := `
		INSERT INTO compute_profiles (
			provider_id, name, provider_size,
			vcpu_count, memory_mb, disk_gb, gpu_count,
			gpu_model, architecture, hourly_price, currency
		)
		VALUES (
			$1, $2, $3, $4, $5, $6, $7,
			$8, $9, $10::numeric, $11
		)
		RETURNING ` + computeProfileColumns

	return scanComputeProfile(r.pool.QueryRow(
		ctx, query,
		input.ProviderID,
		input.Name,
		input.ProviderSize,
		input.VCPUCount,
		input.MemoryMB,
		input.DiskGB,
		input.GPUCount,
		input.GPUModel,
		input.Architecture,
		input.HourlyPrice,
		input.Currency,
	))
}

func (r *ComputeProfileRepository) GetByID(
	ctx context.Context,
	id string,
) (*ComputeProfile, error) {
	if !validUUID(id) {
		return nil, errors.New("id must be a valid UUID")
	}

	query := `
		SELECT ` + computeProfileColumns + `
		FROM compute_profiles
		WHERE id = $1
	`

	return scanComputeProfile(r.pool.QueryRow(ctx, query, id))
}

func (r *ComputeProfileRepository) GetByProviderSize(
	ctx context.Context,
	providerID string,
	providerSize string,
) (*ComputeProfile, error) {
	if !validUUID(providerID) {
		return nil, errors.New("provider_id must be a valid UUID")
	}
	if strings.TrimSpace(providerSize) == "" {
		return nil, errors.New("provider_size is required")
	}

	query := `
		SELECT ` + computeProfileColumns + `
		FROM compute_profiles
		WHERE provider_id = $1
		  AND provider_size = $2
		ORDER BY id
		LIMIT 1
	`

	return scanComputeProfile(
		r.pool.QueryRow(ctx, query, providerID, providerSize),
	)
}

func (r *ComputeProfileRepository) List(
	ctx context.Context,
	options ListComputeProfilesOptions,
) ([]ComputeProfile, error) {
	if options.ProviderID != "" &&
		!validUUID(options.ProviderID) {
		return nil, errors.New("provider_id must be a valid UUID")
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

	var providerID any
	if options.ProviderID != "" {
		providerID = options.ProviderID
	}

	query := `
		SELECT ` + computeProfileColumns + `
		FROM compute_profiles
		WHERE ($1::uuid IS NULL OR provider_id = $1::uuid)
		  AND ($2::boolean IS NULL OR enabled = $2::boolean)
		ORDER BY provider_id, provider_size, id
		LIMIT $3 OFFSET $4
	`

	rows, err := r.pool.Query(
		ctx, query,
		providerID,
		options.Enabled,
		limit,
		options.Offset,
	)
	if err != nil {
		return nil, mapComputeProfileError(err)
	}
	defer rows.Close()

	profiles := make([]ComputeProfile, 0)

	for rows.Next() {
		var profile ComputeProfile

		err := rows.Scan(
			&profile.ID,
			&profile.ProviderID,
			&profile.Name,
			&profile.ProviderSize,
			&profile.VCPUCount,
			&profile.MemoryMB,
			&profile.DiskGB,
			&profile.GPUCount,
			&profile.GPUModel,
			&profile.Architecture,
			&profile.HourlyPrice,
			&profile.Currency,
			&profile.Enabled,
		)
		if err != nil {
			return nil, mapComputeProfileError(err)
		}

		profiles = append(profiles, profile)
	}

	if err := rows.Err(); err != nil {
		return nil, mapComputeProfileError(err)
	}

	return profiles, nil
}

func (r *ComputeProfileRepository) Update(
	ctx context.Context,
	id string,
	input UpdateComputeProfileInput,
) (*ComputeProfile, error) {
	if !validUUID(id) {
		return nil, errors.New("id must be a valid UUID")
	}
	if err := input.Validate(); err != nil {
		return nil, err
	}

	query := `
		UPDATE compute_profiles
		SET
			name = $2,
			vcpu_count = $3,
			memory_mb = $4,
			disk_gb = $5,
			gpu_count = $6,
			gpu_model = $7,
			architecture = $8,
			hourly_price = $9::numeric,
			currency = $10
		WHERE id = $1
		RETURNING ` + computeProfileColumns

	return scanComputeProfile(r.pool.QueryRow(
		ctx, query,
		id,
		input.Name,
		input.VCPUCount,
		input.MemoryMB,
		input.DiskGB,
		input.GPUCount,
		input.GPUModel,
		input.Architecture,
		input.HourlyPrice,
		input.Currency,
	))
}

func (r *ComputeProfileRepository) SetEnabled(
	ctx context.Context,
	id string,
	enabled bool,
) (*ComputeProfile, error) {
	if !validUUID(id) {
		return nil, errors.New("id must be a valid UUID")
	}

	query := `
		UPDATE compute_profiles
		SET enabled = $2
		WHERE id = $1
		RETURNING ` + computeProfileColumns

	return scanComputeProfile(
		r.pool.QueryRow(ctx, query, id, enabled),
	)
}

func (r *ComputeProfileRepository) Delete(
	ctx context.Context,
	id string,
) error {
	if !validUUID(id) {
		return errors.New("id must be a valid UUID")
	}

	tag, err := r.pool.Exec(
		ctx,
		`DELETE FROM compute_profiles WHERE id = $1`,
		id,
	)
	if err != nil {
		return mapComputeProfileError(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrComputeProfileNotFound
	}

	return nil
}

func mapComputeProfileError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrComputeProfileNotFound
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return fmt.Errorf(
				"%w: %v", ErrComputeProfileConflict, err,
			)
		case "23503":
			return fmt.Errorf(
				"compute profile foreign-key constraint: %w", err,
			)
		case "23514":
			return fmt.Errorf(
				"compute profile check constraint: %w", err,
			)
		case "22003":
			return fmt.Errorf(
				"compute profile numeric value out of range: %w", err,
			)
		}
	}

	return err
}
