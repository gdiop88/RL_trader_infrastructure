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
	ErrInstanceNotFound = errors.New("infrastructure instance not found")
	ErrInstanceConflict = errors.New("infrastructure instance conflict")
)

type InstanceRepository struct {
	pool *pgxpool.Pool
}

func NewInstanceRepository(db *database.Database) (*InstanceRepository, error) {
	if db == nil || db.Pool() == nil {
		return nil, errors.New("database pool is not initialized")
	}
	return &InstanceRepository{pool: db.Pool()}, nil
}

const instanceColumns = `
	id,
	provider,
	provider_instance_id,
	name,
	status,
	region,
	instance_type,
	host(public_ip),
	host(private_ip),
	created_at,
	updated_at,
	deleted_at,
	metadata,
	tags,
	last_error,
	provisioned_at,
	terminated_at,
	provider_metadata_id,
	project_id,
	provider_account_id,
	compute_profile_id,
	desired_state,
	last_observed_at
`

func scanInstance(row pgx.Row) (*InfrastructureInstance, error) {
	var instance InfrastructureInstance

	err := row.Scan(
		&instance.ID,
		&instance.Provider,
		&instance.ProviderInstanceID,
		&instance.Name,
		&instance.Status,
		&instance.Region,
		&instance.InstanceType,
		&instance.PublicIP,
		&instance.PrivateIP,
		&instance.CreatedAt,
		&instance.UpdatedAt,
		&instance.DeletedAt,
		&instance.Metadata,
		&instance.Tags,
		&instance.LastError,
		&instance.ProvisionedAt,
		&instance.TerminatedAt,
		&instance.ProviderMetadataID,
		&instance.ProjectID,
		&instance.ProviderAccountID,
		&instance.ComputeProfileID,
		&instance.DesiredState,
		&instance.LastObservedAt,
	)
	if err != nil {
		return nil, err
	}
	return &instance, nil
}

func mapInstanceError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrInstanceNotFound
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return fmt.Errorf("%w: %v", ErrInstanceConflict, err)
		case "23503":
			return fmt.Errorf("invalid infrastructure reference: %w", err)
		}
	}
	return err
}

func jsonOrDefault(value json.RawMessage, fallback string) json.RawMessage {
	if len(value) == 0 {
		return json.RawMessage(fallback)
	}
	return value
}

func nullableJSON(value json.RawMessage) any {
	if len(value) == 0 {
		return nil
	}
	return value
}

func validateInstanceID(id string) error {
	if !validUUID(id) {
		return errors.New("instance ID must be a valid UUID")
	}
	return nil
}

func (r *InstanceRepository) Create(
	ctx context.Context,
	input CreateInstanceInput,
) (*InfrastructureInstance, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}

	var id any
	if strings.TrimSpace(input.ID) != "" {
		id = input.ID
	}

	query := `
		INSERT INTO infrastructure_instances (
			id, provider, provider_instance_id,
			name, status, region, instance_type,
			public_ip, private_ip, metadata, tags,
			provider_metadata_id, project_id,
			provider_account_id, compute_profile_id,
			desired_state
		)
		VALUES (
			COALESCE($1::uuid, gen_random_uuid()),
			$2, $3, $4, 'pending',
			$5, $6, $7::inet, $8::inet,
			$9::jsonb, $10::jsonb,
			$11, $12::uuid, $13::uuid,
			$14::uuid, $15
		)
		RETURNING ` + instanceColumns

	instance, err := scanInstance(r.pool.QueryRow(
		ctx, query,
		id,
		input.Provider,
		input.ProviderInstanceID,
		input.Name,
		input.Region,
		input.InstanceType,
		input.PublicIP,
		input.PrivateIP,
		jsonOrDefault(input.Metadata, `{}`),
		jsonOrDefault(input.Tags, `[]`),
		input.ProviderMetadataID,
		input.ProjectID,
		input.ProviderAccountID,
		input.ComputeProfileID,
		input.DesiredState,
	))
	if err != nil {
		return nil, mapInstanceError(err)
	}
	return instance, nil
}

func (r *InstanceRepository) GetByID(
	ctx context.Context,
	id string,
) (*InfrastructureInstance, error) {
	if err := validateInstanceID(id); err != nil {
		return nil, err
	}

	query := `
		SELECT ` + instanceColumns + `
		FROM infrastructure_instances
		WHERE id = $1::uuid
		  AND deleted_at IS NULL
	`

	instance, err := scanInstance(r.pool.QueryRow(ctx, query, id))
	if err != nil {
		return nil, mapInstanceError(err)
	}
	return instance, nil
}

func (r *InstanceRepository) List(
	ctx context.Context,
	options ListInstancesOptions,
) ([]InfrastructureInstance, error) {
	if options.Status != "" && !options.Status.Valid() {
		return nil, errors.New("invalid instance status")
	}
	if options.Offset < 0 {
		return nil, errors.New("offset cannot be negative")
	}
	if options.Limit < 0 {
		return nil, errors.New("limit cannot be negative")
	}

	limit := options.Limit
	if limit == 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}

	query := `
		SELECT ` + instanceColumns + `
		FROM infrastructure_instances
		WHERE deleted_at IS NULL
		  AND ($1::text = '' OR provider = $1::text)
		  AND ($2::text = '' OR status = $2::text)
		ORDER BY created_at DESC, id DESC
		LIMIT $3 OFFSET $4
	`

	rows, err := r.pool.Query(
		ctx, query,
		options.Provider,
		string(options.Status),
		limit,
		options.Offset,
	)
	if err != nil {
		return nil, mapInstanceError(err)
	}
	defer rows.Close()

	instances := make([]InfrastructureInstance, 0)
	for rows.Next() {
		instance, err := scanInstance(rows)
		if err != nil {
			return nil, err
		}
		instances = append(instances, *instance)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return instances, nil
}

func (r *InstanceRepository) UpdateStatus(
	ctx context.Context,
	id string,
	status InstanceStatus,
) (*InfrastructureInstance, error) {
	if err := validateInstanceID(id); err != nil {
		return nil, err
	}
	if !status.Valid() {
		return nil, errors.New("invalid instance status")
	}

	query := `
		UPDATE infrastructure_instances
		SET status = $2::text,
		    updated_at = CURRENT_TIMESTAMP,
		    provisioned_at = CASE
		        WHEN $2::text = 'running'
		        THEN COALESCE(provisioned_at, CURRENT_TIMESTAMP)
		        ELSE provisioned_at
		    END,
		    terminated_at = CASE
		        WHEN $2::text = 'deleted'
		        THEN COALESCE(terminated_at, CURRENT_TIMESTAMP)
		        ELSE terminated_at
		    END
		WHERE id = $1::uuid
		  AND deleted_at IS NULL
		RETURNING ` + instanceColumns

	instance, err := scanInstance(
		r.pool.QueryRow(ctx, query, id, string(status)),
	)
	if err != nil {
		return nil, mapInstanceError(err)
	}
	return instance, nil
}

func (r *InstanceRepository) UpdateProviderDetails(
	ctx context.Context,
	id string,
	input UpdateProviderDetailsInput,
) (*InfrastructureInstance, error) {
	if err := validateInstanceID(id); err != nil {
		return nil, err
	}
	if err := input.Validate(); err != nil {
		return nil, err
	}

	query := `
		UPDATE infrastructure_instances
		SET provider_instance_id =
		        COALESCE($2::text, provider_instance_id),
		    provider_metadata_id =
		        COALESCE($3::text, provider_metadata_id),
		    region = COALESCE($4::text, region),
		    instance_type = COALESCE($5::text, instance_type),
		    public_ip = COALESCE($6::inet, public_ip),
		    private_ip = COALESCE($7::inet, private_ip),
		    metadata = COALESCE($8::jsonb, metadata),
		    tags = COALESCE($9::jsonb, tags),
		    last_observed_at = CURRENT_TIMESTAMP,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = $1::uuid
		  AND deleted_at IS NULL
		RETURNING ` + instanceColumns

	instance, err := scanInstance(r.pool.QueryRow(
		ctx, query,
		id,
		input.ProviderInstanceID,
		input.ProviderMetadataID,
		input.Region,
		input.InstanceType,
		input.PublicIP,
		input.PrivateIP,
		nullableJSON(input.Metadata),
		nullableJSON(input.Tags),
	))
	if err != nil {
		return nil, mapInstanceError(err)
	}
	return instance, nil
}

func (r *InstanceRepository) UpdateMetadata(
	ctx context.Context,
	id string,
	input UpdateInstanceMetadataInput,
) (*InfrastructureInstance, error) {
	if err := validateInstanceID(id); err != nil {
		return nil, err
	}
	if err := input.Validate(); err != nil {
		return nil, err
	}

	query := `
		UPDATE infrastructure_instances
		SET metadata = COALESCE($2::jsonb, metadata),
		    tags = COALESCE($3::jsonb, tags),
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = $1::uuid
		  AND deleted_at IS NULL
		RETURNING ` + instanceColumns

	instance, err := scanInstance(r.pool.QueryRow(
		ctx, query,
		id,
		nullableJSON(input.Metadata),
		nullableJSON(input.Tags),
	))
	if err != nil {
		return nil, mapInstanceError(err)
	}
	return instance, nil
}

func (r *InstanceRepository) SetDesiredState(
	ctx context.Context,
	id string,
	desiredState InstanceStatus,
) (*InfrastructureInstance, error) {
	if err := validateInstanceID(id); err != nil {
		return nil, err
	}
	if !desiredState.Valid() {
		return nil, errors.New("invalid desired state")
	}

	query := `
		UPDATE infrastructure_instances
		SET desired_state = $2::text,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = $1::uuid
		  AND deleted_at IS NULL
		RETURNING ` + instanceColumns

	instance, err := scanInstance(
		r.pool.QueryRow(ctx, query, id, string(desiredState)),
	)
	if err != nil {
		return nil, mapInstanceError(err)
	}
	return instance, nil
}

func (r *InstanceRepository) SetLastError(
	ctx context.Context,
	id string,
	message string,
) (*InfrastructureInstance, error) {
	if err := validateInstanceID(id); err != nil {
		return nil, err
	}

	query := `
		UPDATE infrastructure_instances
		SET last_error = NULLIF($2::text, ''),
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = $1::uuid
		  AND deleted_at IS NULL
		RETURNING ` + instanceColumns

	instance, err := scanInstance(
		r.pool.QueryRow(ctx, query, id, message),
	)
	if err != nil {
		return nil, mapInstanceError(err)
	}
	return instance, nil
}

func (r *InstanceRepository) SoftDelete(
	ctx context.Context,
	id string,
) error {
	if err := validateInstanceID(id); err != nil {
		return err
	}

	query := `
		UPDATE infrastructure_instances
		SET status = 'deleted',
		    deleted_at = CURRENT_TIMESTAMP,
		    terminated_at = COALESCE(
		        terminated_at, CURRENT_TIMESTAMP
		    ),
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = $1::uuid
		  AND deleted_at IS NULL
	`

	tag, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return mapInstanceError(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrInstanceNotFound
	}
	return nil
}
