package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/gdiop88/RL_trader_infrastructure/internal/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrResourceUsageSampleNotFound = errors.New("resource usage sample not found")
	ErrResourceUsageSampleConflict = errors.New("resource usage sample conflict")
)

type ResourceUsageSample struct {
	ID             string
	InstanceID     string
	CollectedAt    time.Time
	CPUPercent     *string
	MemoryUsedMB   *int64
	DiskUsedGB     *string
	NetworkRXBytes *int64
	NetworkTXBytes *int64
	GPUPercent     *string
	Metrics        json.RawMessage
}

type CreateResourceUsageSampleInput struct {
	InstanceID     string
	CollectedAt    time.Time
	CPUPercent     *string
	MemoryUsedMB   *int64
	DiskUsedGB     *string
	NetworkRXBytes *int64
	NetworkTXBytes *int64
	GPUPercent     *string
	Metrics        json.RawMessage
}

type UpdateResourceUsageSampleInput struct {
	CollectedAt    time.Time
	CPUPercent     *string
	MemoryUsedMB   *int64
	DiskUsedGB     *string
	NetworkRXBytes *int64
	NetworkTXBytes *int64
	GPUPercent     *string
	Metrics        json.RawMessage
}

type ListResourceUsageSamplesOptions struct {
	InstanceID *string
	From       *time.Time
	To         *time.Time
	Limit      int
	Offset     int
}

type ResourceUsageSampleRepository struct {
	pool *pgxpool.Pool
}

const resourceUsageSampleColumns = `
	id,
	instance_id,
	collected_at,
	cpu_percent::text,
	memory_used_mb,
	disk_used_gb::text,
	network_rx_bytes,
	network_tx_bytes,
	gpu_percent::text,
	metrics
`

func NewResourceUsageSampleRepository(
	db *database.Database,
) (*ResourceUsageSampleRepository, error) {
	if db == nil || db.Pool() == nil {
		return nil, errors.New("resource usage sample repository: nil database")
	}

	return &ResourceUsageSampleRepository{
		pool: db.Pool(),
	}, nil
}

func validateUsageDecimal(
	name string,
	value *string,
	precision int,
	scale int,
) error {
	if value == nil {
		return nil
	}

	s := *value
	if s == "" || strings.TrimSpace(s) != s {
		return fmt.Errorf("%s must be a valid decimal", name)
	}

	if strings.ContainsAny(s, "eE/") {
		return fmt.Errorf("%s must use plain decimal notation", name)
	}

	r, ok := new(big.Rat).SetString(s)
	if !ok {
		return fmt.Errorf("%s must be a valid decimal", name)
	}

	// PostgreSQL NUMERIC(p,s) rounds to s decimal places.
	// Check the rounded value against the permitted integer digits.
	abs := new(big.Rat).Abs(r)
	multiplier := new(big.Int).Exp(
		big.NewInt(10),
		big.NewInt(int64(scale)),
		nil,
	)

	scaled := new(big.Rat).Mul(
		abs,
		new(big.Rat).SetInt(multiplier),
	)

	numerator := scaled.Num()
	denominator := scaled.Denom()

	quotient, remainder := new(big.Int).QuoRem(
		numerator,
		denominator,
		new(big.Int),
	)

	twiceRemainder := new(big.Int).Mul(
		remainder,
		big.NewInt(2),
	)

	if twiceRemainder.Cmp(denominator) >= 0 {
		quotient.Add(quotient, big.NewInt(1))
	}

	maxDigits := new(big.Int).Exp(
		big.NewInt(10),
		big.NewInt(int64(precision)),
		nil,
	)

	if quotient.Cmp(maxDigits) >= 0 {
		return fmt.Errorf(
			"%s exceeds NUMERIC(%d,%d)",
			name,
			precision,
			scale,
		)
	}

	return nil
}

func validateUsageMetrics(metrics json.RawMessage) error {
	if len(metrics) == 0 {
		return nil
	}
	if !json.Valid(metrics) {
		return errors.New("metrics must contain valid JSON")
	}
	return nil
}

func validateUsageFields(
	collectedAt time.Time,
	cpu *string,
	disk *string,
	gpu *string,
	metrics json.RawMessage,
) error {
	if collectedAt.IsZero() {
		return errors.New("collected_at is required")
	}

	for _, field := range []struct {
		name      string
		value     *string
		precision int
		scale     int
	}{
		{"cpu_percent", cpu, 7, 3},
		{"disk_used_gb", disk, 18, 3},
		{"gpu_percent", gpu, 7, 3},
	} {
		if err := validateUsageDecimal(
			field.name,
			field.value,
			field.precision,
			field.scale,
		); err != nil {
			return err
		}
	}

	return validateUsageMetrics(metrics)
}

func (in CreateResourceUsageSampleInput) Validate() error {
	if !validUUID(in.InstanceID) {
		return errors.New("instance_id must be a valid UUID")
	}

	return validateUsageFields(
		in.CollectedAt,
		in.CPUPercent,
		in.DiskUsedGB,
		in.GPUPercent,
		in.Metrics,
	)
}

func (in UpdateResourceUsageSampleInput) Validate() error {
	return validateUsageFields(
		in.CollectedAt,
		in.CPUPercent,
		in.DiskUsedGB,
		in.GPUPercent,
		in.Metrics,
	)
}

func scanResourceUsageSample(row pgx.Row) (
	*ResourceUsageSample,
	error,
) {
	var sample ResourceUsageSample

	err := row.Scan(
		&sample.ID,
		&sample.InstanceID,
		&sample.CollectedAt,
		&sample.CPUPercent,
		&sample.MemoryUsedMB,
		&sample.DiskUsedGB,
		&sample.NetworkRXBytes,
		&sample.NetworkTXBytes,
		&sample.GPUPercent,
		&sample.Metrics,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrResourceUsageSampleNotFound
	}
	if err != nil {
		return nil, err
	}

	return &sample, nil
}

func resourceUsageSampleDBError(err error) error {
	if err == nil {
		return nil
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return fmt.Errorf(
				"%w: %v",
				ErrResourceUsageSampleConflict,
				err,
			)
		case "23503":
			return fmt.Errorf("resource usage sample foreign key: %w", err)
		case "23502":
			return fmt.Errorf("resource usage sample required field: %w", err)
		case "23514":
			return fmt.Errorf("resource usage sample check constraint: %w", err)
		case "22003":
			return fmt.Errorf("resource usage sample numeric overflow: %w", err)
		case "22P02":
			return fmt.Errorf("resource usage sample invalid value: %w", err)
		}
	}

	return err
}

func usageMetricsArgument(metrics json.RawMessage) any {
	if len(metrics) == 0 {
		return nil
	}
	return string(metrics)
}

func (r *ResourceUsageSampleRepository) Create(
	ctx context.Context,
	in CreateResourceUsageSampleInput,
) (*ResourceUsageSample, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}

	query := `
		INSERT INTO resource_usage_samples (
			instance_id, collected_at, cpu_percent,
			memory_used_mb, disk_used_gb,
			network_rx_bytes, network_tx_bytes,
			gpu_percent, metrics
		)
		VALUES (
			$1, $2, $3::numeric,
			$4, $5::numeric,
			$6, $7,
			$8::numeric, $9::jsonb
		)
		RETURNING ` + resourceUsageSampleColumns

	sample, err := scanResourceUsageSample(
		r.pool.QueryRow(
			ctx,
			query,
			in.InstanceID,
			in.CollectedAt,
			in.CPUPercent,
			in.MemoryUsedMB,
			in.DiskUsedGB,
			in.NetworkRXBytes,
			in.NetworkTXBytes,
			in.GPUPercent,
			usageMetricsArgument(in.Metrics),
		),
	)

	return sample, resourceUsageSampleDBError(err)
}

func (r *ResourceUsageSampleRepository) GetByID(
	ctx context.Context,
	id string,
) (*ResourceUsageSample, error) {
	if !validUUID(id) {
		return nil, errors.New("id must be a valid UUID")
	}

	query := `
		SELECT ` + resourceUsageSampleColumns + `
		FROM resource_usage_samples
		WHERE id = $1
	`

	return scanResourceUsageSample(r.pool.QueryRow(ctx, query, id))
}

func (r *ResourceUsageSampleRepository) List(
	ctx context.Context,
	opts ListResourceUsageSamplesOptions,
) ([]ResourceUsageSample, error) {
	if opts.Limit < 0 || opts.Limit > 1000 {
		return nil, errors.New("limit must be between 0 and 1000")
	}
	if opts.Offset < 0 {
		return nil, errors.New("offset cannot be negative")
	}
	if opts.InstanceID != nil && !validUUID(*opts.InstanceID) {
		return nil, errors.New("instance_id must be a valid UUID")
	}
	if opts.From != nil && opts.From.IsZero() {
		return nil, errors.New("from cannot be a zero timestamp")
	}
	if opts.To != nil && opts.To.IsZero() {
		return nil, errors.New("to cannot be a zero timestamp")
	}
	if opts.From != nil && opts.To != nil &&
		opts.From.After(*opts.To) {
		return nil, errors.New("from cannot be after to")
	}

	limit := opts.Limit
	if limit == 0 {
		limit = 100
	}

	query := `
		SELECT ` + resourceUsageSampleColumns + `
		FROM resource_usage_samples
		WHERE 1=1
	`

	args := make([]any, 0, 5)

	addFilter := func(condition string, value any) {
		args = append(args, value)
		query += fmt.Sprintf(
			" AND %s $%d",
			condition,
			len(args),
		)
	}

	if opts.InstanceID != nil {
		addFilter("instance_id =", *opts.InstanceID)
	}
	if opts.From != nil {
		addFilter("collected_at >=", *opts.From)
	}
	if opts.To != nil {
		addFilter("collected_at <=", *opts.To)
	}

	args = append(args, limit, opts.Offset)
	query += fmt.Sprintf(
		" ORDER BY collected_at DESC, id ASC LIMIT $%d OFFSET $%d",
		len(args)-1,
		len(args),
	)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, resourceUsageSampleDBError(err)
	}
	defer rows.Close()

	samples := make([]ResourceUsageSample, 0)

	for rows.Next() {
		sample, err := scanResourceUsageSample(rows)
		if err != nil {
			return nil, err
		}
		samples = append(samples, *sample)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return samples, nil
}

func (r *ResourceUsageSampleRepository) Update(
	ctx context.Context,
	id string,
	in UpdateResourceUsageSampleInput,
) (*ResourceUsageSample, error) {
	if !validUUID(id) {
		return nil, errors.New("id must be a valid UUID")
	}
	if err := in.Validate(); err != nil {
		return nil, err
	}

	query := `
		UPDATE resource_usage_samples
		SET collected_at = $2,
		    cpu_percent = $3::numeric,
		    memory_used_mb = $4,
		    disk_used_gb = $5::numeric,
		    network_rx_bytes = $6,
		    network_tx_bytes = $7,
		    gpu_percent = $8::numeric,
		    metrics = $9::jsonb
		WHERE id = $1
		RETURNING ` + resourceUsageSampleColumns

	sample, err := scanResourceUsageSample(
		r.pool.QueryRow(
			ctx,
			query,
			id,
			in.CollectedAt,
			in.CPUPercent,
			in.MemoryUsedMB,
			in.DiskUsedGB,
			in.NetworkRXBytes,
			in.NetworkTXBytes,
			in.GPUPercent,
			usageMetricsArgument(in.Metrics),
		),
	)

	return sample, resourceUsageSampleDBError(err)
}

func (r *ResourceUsageSampleRepository) Delete(
	ctx context.Context,
	id string,
) error {
	if !validUUID(id) {
		return errors.New("id must be a valid UUID")
	}

	result, err := r.pool.Exec(
		ctx,
		`DELETE FROM resource_usage_samples WHERE id = $1`,
		id,
	)
	if err != nil {
		return resourceUsageSampleDBError(err)
	}
	if result.RowsAffected() == 0 {
		return ErrResourceUsageSampleNotFound
	}

	return nil
}
