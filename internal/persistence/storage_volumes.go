package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/gdiop88/RL_trader_infrastructure/internal/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrStorageVolumeNotFound = errors.New("storage volume not found")
	ErrStorageVolumeConflict = errors.New("storage volume conflict")
)

type StorageVolume struct {
	ID                string
	ProjectID         *string
	ProviderAccountID *string
	ProviderVolumeID  *string
	Name              *string
	Region            *string
	SizeGB            int
	VolumeType        *string
	Status            *string
	Encrypted         *bool
	Metadata          json.RawMessage
	CreatedAt         time.Time
	DeletedAt         *time.Time
}

type CreateStorageVolumeInput struct {
	ProjectID         *string
	ProviderAccountID *string
	ProviderVolumeID  *string
	Name              *string
	Region            *string
	SizeGB            int
	VolumeType        *string
	Status            *string
	Encrypted         *bool
	Metadata          json.RawMessage
}

type UpdateStorageVolumeInput struct {
	ProjectID         *string
	ProviderAccountID *string
	ProviderVolumeID  *string
	Name              *string
	Region            *string
	SizeGB            int
	VolumeType        *string
	Status            *string
	Encrypted         *bool
	Metadata          json.RawMessage
	DeletedAt         *time.Time
}

type ListStorageVolumesOptions struct {
	ProjectID         *string
	ProviderAccountID *string
	Region            *string
	VolumeType        *string
	Status            *string
	Encrypted         *bool
	IncludeDeleted    bool
	Limit             int
	Offset            int
}

type StorageVolumeRepository struct {
	pool *pgxpool.Pool
}

func NewStorageVolumeRepository(
	db *database.Database,
) (*StorageVolumeRepository, error) {
	if db == nil || db.Pool() == nil {
		return nil, errors.New("database connection is required")
	}

	return &StorageVolumeRepository{
		pool: db.Pool(),
	}, nil
}

func validateStorageVolumeFields(
	projectID *string,
	providerAccountID *string,
	providerVolumeID *string,
	name *string,
	region *string,
	sizeGB int,
	volumeType *string,
	status *string,
	metadata json.RawMessage,
) error {
	if projectID != nil && !validUUID(*projectID) {
		return errors.New("project_id must be a valid UUID")
	}

	if providerAccountID != nil && !validUUID(*providerAccountID) {
		return errors.New("provider_account_id must be a valid UUID")
	}

	fields := []struct {
		name  string
		value *string
		max   int
	}{
		{"provider_volume_id", providerVolumeID, 255},
		{"name", name, 150},
		{"region", region, 100},
		{"volume_type", volumeType, 100},
		{"status", status, 50},
	}

	for _, field := range fields {
		if err := validateOptionalString(
			field.name,
			field.value,
			field.max,
		); err != nil {
			return err
		}
	}

	if sizeGB <= 0 {
		return errors.New("size_gb must be greater than zero")
	}

	if len(metadata) > 0 && !json.Valid(metadata) {
		return errors.New("metadata must contain valid JSON")
	}

	return nil
}

func (in CreateStorageVolumeInput) Validate() error {
	return validateStorageVolumeFields(
		in.ProjectID,
		in.ProviderAccountID,
		in.ProviderVolumeID,
		in.Name,
		in.Region,
		in.SizeGB,
		in.VolumeType,
		in.Status,
		in.Metadata,
	)
}

func (in UpdateStorageVolumeInput) Validate() error {
	if in.DeletedAt != nil && in.DeletedAt.IsZero() {
		return errors.New("deleted_at cannot be a zero timestamp")
	}

	return validateStorageVolumeFields(
		in.ProjectID,
		in.ProviderAccountID,
		in.ProviderVolumeID,
		in.Name,
		in.Region,
		in.SizeGB,
		in.VolumeType,
		in.Status,
		in.Metadata,
	)
}

const storageVolumeColumns = `
	id,
	project_id,
	provider_account_id,
	provider_volume_id,
	name,
	region,
	size_gb,
	volume_type,
	status,
	encrypted,
	metadata,
	created_at,
	deleted_at
`

func scanStorageVolume(row pgx.Row) (*StorageVolume, error) {
	var volume StorageVolume

	err := row.Scan(
		&volume.ID,
		&volume.ProjectID,
		&volume.ProviderAccountID,
		&volume.ProviderVolumeID,
		&volume.Name,
		&volume.Region,
		&volume.SizeGB,
		&volume.VolumeType,
		&volume.Status,
		&volume.Encrypted,
		&volume.Metadata,
		&volume.CreatedAt,
		&volume.DeletedAt,
	)
	if err != nil {
		return nil, mapStorageVolumeError(err)
	}

	return &volume, nil
}

func (r *StorageVolumeRepository) Create(
	ctx context.Context,
	input CreateStorageVolumeInput,
) (*StorageVolume, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}

	query := `
		INSERT INTO storage_volumes (
			project_id,
			provider_account_id,
			provider_volume_id,
			name,
			region,
			size_gb,
			volume_type,
			status,
			encrypted,
			metadata
		)
		VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8, $9, $10::jsonb
		)
		RETURNING ` + storageVolumeColumns

	return scanStorageVolume(
		r.pool.QueryRow(
			ctx,
			query,
			input.ProjectID,
			input.ProviderAccountID,
			input.ProviderVolumeID,
			input.Name,
			input.Region,
			input.SizeGB,
			input.VolumeType,
			input.Status,
			input.Encrypted,
			storageVolumeJSON(input.Metadata),
		),
	)
}

func (r *StorageVolumeRepository) GetByID(
	ctx context.Context,
	id string,
) (*StorageVolume, error) {
	if !validUUID(id) {
		return nil, errors.New("id must be a valid UUID")
	}

	query := `
		SELECT ` + storageVolumeColumns + `
		FROM storage_volumes
		WHERE id = $1
	`

	return scanStorageVolume(r.pool.QueryRow(ctx, query, id))
}

func (r *StorageVolumeRepository) List(
	ctx context.Context,
	options ListStorageVolumesOptions,
) ([]StorageVolume, error) {
	if options.ProjectID != nil && !validUUID(*options.ProjectID) {
		return nil, errors.New("project_id must be a valid UUID")
	}

	if options.ProviderAccountID != nil &&
		!validUUID(*options.ProviderAccountID) {
		return nil, errors.New("provider_account_id must be a valid UUID")
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

	var projectID, accountID, region, volumeType, status, encrypted any

	if options.ProjectID != nil {
		projectID = *options.ProjectID
	}
	if options.ProviderAccountID != nil {
		accountID = *options.ProviderAccountID
	}
	if options.Region != nil {
		region = *options.Region
	}
	if options.VolumeType != nil {
		volumeType = *options.VolumeType
	}
	if options.Status != nil {
		status = *options.Status
	}
	if options.Encrypted != nil {
		encrypted = *options.Encrypted
	}

	query := `
		SELECT ` + storageVolumeColumns + `
		FROM storage_volumes
		WHERE
			($1::uuid IS NULL OR project_id = $1::uuid)
			AND ($2::uuid IS NULL OR provider_account_id = $2::uuid)
			AND ($3::text IS NULL OR region = $3::text)
			AND ($4::text IS NULL OR volume_type = $4::text)
			AND ($5::text IS NULL OR status = $5::text)
			AND ($6::boolean IS NULL OR encrypted = $6::boolean)
			AND ($7::boolean OR deleted_at IS NULL)
		ORDER BY created_at DESC, id ASC
		LIMIT $8 OFFSET $9
	`

	rows, err := r.pool.Query(
		ctx,
		query,
		projectID,
		accountID,
		region,
		volumeType,
		status,
		encrypted,
		options.IncludeDeleted,
		limit,
		options.Offset,
	)
	if err != nil {
		return nil, mapStorageVolumeError(err)
	}
	defer rows.Close()

	volumes := make([]StorageVolume, 0)

	for rows.Next() {
		volume, err := scanStorageVolume(rows)
		if err != nil {
			return nil, err
		}
		volumes = append(volumes, *volume)
	}

	if err := rows.Err(); err != nil {
		return nil, mapStorageVolumeError(err)
	}

	return volumes, nil
}

func (r *StorageVolumeRepository) Update(
	ctx context.Context,
	id string,
	input UpdateStorageVolumeInput,
) (*StorageVolume, error) {
	if !validUUID(id) {
		return nil, errors.New("id must be a valid UUID")
	}

	if err := input.Validate(); err != nil {
		return nil, err
	}

	query := `
		UPDATE storage_volumes
		SET
			project_id = $2,
			provider_account_id = $3,
			provider_volume_id = $4,
			name = $5,
			region = $6,
			size_gb = $7,
			volume_type = $8,
			status = $9,
			encrypted = $10,
			metadata = $11::jsonb,
			deleted_at = $12
		WHERE id = $1
		RETURNING ` + storageVolumeColumns

	return scanStorageVolume(
		r.pool.QueryRow(
			ctx,
			query,
			id,
			input.ProjectID,
			input.ProviderAccountID,
			input.ProviderVolumeID,
			input.Name,
			input.Region,
			input.SizeGB,
			input.VolumeType,
			input.Status,
			input.Encrypted,
			storageVolumeJSON(input.Metadata),
			input.DeletedAt,
		),
	)
}

func (r *StorageVolumeRepository) UpdateStatus(
	ctx context.Context,
	id string,
	status *string,
) (*StorageVolume, error) {
	if !validUUID(id) {
		return nil, errors.New("id must be a valid UUID")
	}

	if err := validateOptionalString("status", status, 50); err != nil {
		return nil, err
	}

	query := `
		UPDATE storage_volumes
		SET status = $2
		WHERE id = $1
		RETURNING ` + storageVolumeColumns

	return scanStorageVolume(
		r.pool.QueryRow(ctx, query, id, status),
	)
}

func (r *StorageVolumeRepository) SoftDelete(
	ctx context.Context,
	id string,
	deletedAt time.Time,
) (*StorageVolume, error) {
	if !validUUID(id) {
		return nil, errors.New("id must be a valid UUID")
	}

	if deletedAt.IsZero() {
		return nil, errors.New("deleted_at cannot be a zero timestamp")
	}

	query := `
		UPDATE storage_volumes
		SET deleted_at = $2
		WHERE id = $1
		RETURNING ` + storageVolumeColumns

	return scanStorageVolume(
		r.pool.QueryRow(ctx, query, id, deletedAt),
	)
}

func (r *StorageVolumeRepository) Delete(
	ctx context.Context,
	id string,
) error {
	if !validUUID(id) {
		return errors.New("id must be a valid UUID")
	}

	tag, err := r.pool.Exec(
		ctx,
		`DELETE FROM storage_volumes WHERE id = $1`,
		id,
	)
	if err != nil {
		return mapStorageVolumeError(err)
	}

	if tag.RowsAffected() == 0 {
		return ErrStorageVolumeNotFound
	}

	return nil
}

func storageVolumeJSON(value json.RawMessage) any {
	if len(value) == 0 {
		return nil
	}
	return value
}

func mapStorageVolumeError(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, pgx.ErrNoRows) {
		return ErrStorageVolumeNotFound
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return fmt.Errorf("%w: %v", ErrStorageVolumeConflict, err)
		case "23503":
			return fmt.Errorf("storage volume foreign-key constraint: %w", err)
		case "23514":
			return fmt.Errorf("storage volume check constraint: %w", err)
		case "23502":
			return fmt.Errorf("storage volume required field: %w", err)
		}
	}

	return err
}
