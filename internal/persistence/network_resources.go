package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/gdiop88/RL_trader_infrastructure/internal/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNetworkResourceNotFound = errors.New("network resource not found")
	ErrNetworkResourceConflict = errors.New("network resource conflict")
)

type NetworkResource struct {
	ID                string
	ProjectID         *string
	ProviderAccountID *string
	ProviderNetworkID *string
	Name              *string
	Region            *string
	NetworkType       *string
	CIDR              *string
	Status            *string
	Metadata          json.RawMessage
	CreatedAt         time.Time
}

type CreateNetworkResourceInput struct {
	ProjectID         *string
	ProviderAccountID *string
	ProviderNetworkID *string
	Name              *string
	Region            *string
	NetworkType       *string
	CIDR              *string
	Status            *string
	Metadata          json.RawMessage
}

type UpdateNetworkResourceInput struct {
	ProjectID         *string
	ProviderAccountID *string
	ProviderNetworkID *string
	Name              *string
	Region            *string
	NetworkType       *string
	CIDR              *string
	Status            *string
	Metadata          json.RawMessage
}

type ListNetworkResourcesOptions struct {
	ProjectID         *string
	ProviderAccountID *string
	Region            string
	NetworkType       string
	Status            string
	Limit             int
	Offset            int
}

type NetworkResourceRepository struct {
	pool *pgxpool.Pool
}

func NewNetworkResourceRepository(
	db *database.Database,
) (*NetworkResourceRepository, error) {
	if db == nil || db.Pool() == nil {
		return nil, errors.New("database connection is required")
	}

	return &NetworkResourceRepository{pool: db.Pool()}, nil
}

func validateNetworkResourceOptional(
	name string,
	value *string,
	max int,
) error {
	if err := validateOptionalString(name, value, max); err != nil {
		return err
	}
	if value != nil && strings.ContainsRune(*value, '\x00') {
		return fmt.Errorf("%s contains a null character", name)
	}
	return nil
}

func validateNetworkResourceCIDR(value *string) error {
	if value == nil {
		return nil
	}

	if strings.ContainsRune(*value, '\x00') {
		return errors.New("cidr contains a null character")
	}

	prefix, err := netip.ParsePrefix(*value)
	if err != nil || !prefix.IsValid() {
		return errors.New("cidr must be a valid IPv4 or IPv6 prefix")
	}

	return nil
}

func validateNetworkResourceFields(
	projectID *string,
	providerAccountID *string,
	providerNetworkID *string,
	name *string,
	region *string,
	networkType *string,
	cidr *string,
	status *string,
	metadata json.RawMessage,
) error {
	if projectID != nil && !validUUID(*projectID) {
		return errors.New("project_id must be a valid UUID")
	}
	if providerAccountID != nil && !validUUID(*providerAccountID) {
		return errors.New("provider_account_id must be a valid UUID")
	}

	for _, field := range []struct {
		name  string
		value *string
		max   int
	}{
		{"provider_network_id", providerNetworkID, 255},
		{"name", name, 150},
		{"region", region, 100},
		{"network_type", networkType, 50},
		{"status", status, 50},
	} {
		if err := validateNetworkResourceOptional(
			field.name, field.value, field.max,
		); err != nil {
			return err
		}
	}

	if err := validateNetworkResourceCIDR(cidr); err != nil {
		return err
	}

	if len(metadata) > 0 && !json.Valid(metadata) {
		return errors.New("metadata contains invalid JSON")
	}

	return nil
}

func (in CreateNetworkResourceInput) Validate() error {
	return validateNetworkResourceFields(
		in.ProjectID,
		in.ProviderAccountID,
		in.ProviderNetworkID,
		in.Name,
		in.Region,
		in.NetworkType,
		in.CIDR,
		in.Status,
		in.Metadata,
	)
}

func (in UpdateNetworkResourceInput) Validate() error {
	return validateNetworkResourceFields(
		in.ProjectID,
		in.ProviderAccountID,
		in.ProviderNetworkID,
		in.Name,
		in.Region,
		in.NetworkType,
		in.CIDR,
		in.Status,
		in.Metadata,
	)
}

const networkResourceColumns = `
	id,
	project_id,
	provider_account_id,
	provider_network_id,
	name,
	region,
	network_type,
	cidr::text,
	status,
	metadata,
	created_at
`

func scanNetworkResource(row pgx.Row) (*NetworkResource, error) {
	var resource NetworkResource

	err := row.Scan(
		&resource.ID,
		&resource.ProjectID,
		&resource.ProviderAccountID,
		&resource.ProviderNetworkID,
		&resource.Name,
		&resource.Region,
		&resource.NetworkType,
		&resource.CIDR,
		&resource.Status,
		&resource.Metadata,
		&resource.CreatedAt,
	)
	if err != nil {
		return nil, mapNetworkResourceError(err)
	}

	return &resource, nil
}

func nullableNetworkResourceMetadata(data json.RawMessage) any {
	if len(data) == 0 {
		return nil
	}
	return string(data)
}

func (r *NetworkResourceRepository) Create(
	ctx context.Context,
	input CreateNetworkResourceInput,
) (*NetworkResource, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}

	query := `
		INSERT INTO network_resources (
			project_id,
			provider_account_id,
			provider_network_id,
			name,
			region,
			network_type,
			cidr,
			status,
			metadata
		)
		VALUES (
			$1, $2, $3, $4, $5, $6,
			$7::cidr, $8, $9::jsonb
		)
		RETURNING ` + networkResourceColumns

	return scanNetworkResource(
		r.pool.QueryRow(
			ctx,
			query,
			input.ProjectID,
			input.ProviderAccountID,
			input.ProviderNetworkID,
			input.Name,
			input.Region,
			input.NetworkType,
			input.CIDR,
			input.Status,
			nullableNetworkResourceMetadata(input.Metadata),
		),
	)
}

func (r *NetworkResourceRepository) GetByID(
	ctx context.Context,
	id string,
) (*NetworkResource, error) {
	if !validUUID(id) {
		return nil, errors.New("id must be a valid UUID")
	}

	query := `
		SELECT ` + networkResourceColumns + `
		FROM network_resources
		WHERE id = $1
	`

	return scanNetworkResource(r.pool.QueryRow(ctx, query, id))
}

func (r *NetworkResourceRepository) List(
	ctx context.Context,
	options ListNetworkResourcesOptions,
) ([]NetworkResource, error) {
	if options.ProjectID != nil && !validUUID(*options.ProjectID) {
		return nil, errors.New("project_id must be a valid UUID")
	}
	if options.ProviderAccountID != nil &&
		!validUUID(*options.ProviderAccountID) {
		return nil, errors.New("provider_account_id must be a valid UUID")
	}

	for _, field := range []struct {
		name  string
		value string
		max   int
	}{
		{"region", options.Region, 100},
		{"network_type", options.NetworkType, 50},
		{"status", options.Status, 50},
	} {
		if len(field.value) > field.max ||
			strings.ContainsRune(field.value, '\x00') {
			return nil, fmt.Errorf("invalid %s filter", field.name)
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

	var projectID, accountID, region, networkType, status any

	if options.ProjectID != nil {
		projectID = *options.ProjectID
	}
	if options.ProviderAccountID != nil {
		accountID = *options.ProviderAccountID
	}
	if options.Region != "" {
		region = options.Region
	}
	if options.NetworkType != "" {
		networkType = options.NetworkType
	}
	if options.Status != "" {
		status = options.Status
	}

	query := `
		SELECT ` + networkResourceColumns + `
		FROM network_resources
		WHERE
			($1::uuid IS NULL OR project_id = $1::uuid)
			AND ($2::uuid IS NULL OR provider_account_id = $2::uuid)
			AND ($3::text IS NULL OR region = $3::text)
			AND ($4::text IS NULL OR network_type = $4::text)
			AND ($5::text IS NULL OR status = $5::text)
		ORDER BY created_at ASC, id ASC
		LIMIT $6 OFFSET $7
	`

	rows, err := r.pool.Query(
		ctx,
		query,
		projectID,
		accountID,
		region,
		networkType,
		status,
		limit,
		options.Offset,
	)
	if err != nil {
		return nil, mapNetworkResourceError(err)
	}
	defer rows.Close()

	resources := make([]NetworkResource, 0)

	for rows.Next() {
		resource, err := scanNetworkResource(rows)
		if err != nil {
			return nil, err
		}
		resources = append(resources, *resource)
	}

	if err := rows.Err(); err != nil {
		return nil, mapNetworkResourceError(err)
	}

	return resources, nil
}

func (r *NetworkResourceRepository) Update(
	ctx context.Context,
	id string,
	input UpdateNetworkResourceInput,
) (*NetworkResource, error) {
	if !validUUID(id) {
		return nil, errors.New("id must be a valid UUID")
	}
	if err := input.Validate(); err != nil {
		return nil, err
	}

	query := `
		UPDATE network_resources
		SET
			project_id = $2,
			provider_account_id = $3,
			provider_network_id = $4,
			name = $5,
			region = $6,
			network_type = $7,
			cidr = $8::cidr,
			status = $9,
			metadata = $10::jsonb
		WHERE id = $1
		RETURNING ` + networkResourceColumns

	return scanNetworkResource(
		r.pool.QueryRow(
			ctx,
			query,
			id,
			input.ProjectID,
			input.ProviderAccountID,
			input.ProviderNetworkID,
			input.Name,
			input.Region,
			input.NetworkType,
			input.CIDR,
			input.Status,
			nullableNetworkResourceMetadata(input.Metadata),
		),
	)
}

func (r *NetworkResourceRepository) UpdateStatus(
	ctx context.Context,
	id string,
	status *string,
) (*NetworkResource, error) {
	if !validUUID(id) {
		return nil, errors.New("id must be a valid UUID")
	}
	if err := validateNetworkResourceOptional(
		"status", status, 50,
	); err != nil {
		return nil, err
	}

	query := `
		UPDATE network_resources
		SET status = $2
		WHERE id = $1
		RETURNING ` + networkResourceColumns

	return scanNetworkResource(
		r.pool.QueryRow(ctx, query, id, status),
	)
}

func (r *NetworkResourceRepository) Delete(
	ctx context.Context,
	id string,
) error {
	if !validUUID(id) {
		return errors.New("id must be a valid UUID")
	}

	tag, err := r.pool.Exec(
		ctx,
		`DELETE FROM network_resources WHERE id = $1`,
		id,
	)
	if err != nil {
		return mapNetworkResourceError(err)
	}

	if tag.RowsAffected() == 0 {
		return ErrNetworkResourceNotFound
	}

	return nil
}

func mapNetworkResourceError(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNetworkResourceNotFound
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return fmt.Errorf(
				"%w: %v", ErrNetworkResourceConflict, err,
			)
		case "23503":
			return fmt.Errorf(
				"network resource foreign-key constraint: %w", err,
			)
		case "23514":
			return fmt.Errorf(
				"network resource check constraint: %w", err,
			)
		case "23502":
			return fmt.Errorf(
				"network resource required field: %w", err,
			)
		}
	}

	return err
}
