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
	ErrProvisioningTemplateNotFound = errors.New(
		"provisioning template not found",
	)
	ErrProvisioningTemplateConflict = errors.New(
		"provisioning template conflict",
	)
)

type ProvisioningTemplate struct {
	ID             string
	ProjectID      *string
	Name           string
	Version        int32
	Description    *string
	Provider       *string
	ImageReference *string
	Configuration  json.RawMessage
	Enabled        bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type CreateProvisioningTemplateInput struct {
	ProjectID      *string
	Name           string
	Version        int32
	Description    *string
	Provider       *string
	ImageReference *string
	Configuration  json.RawMessage
}

type UpdateProvisioningTemplateInput struct {
	ProjectID      *string
	Name           string
	Version        int32
	Description    *string
	Provider       *string
	ImageReference *string
	Configuration  json.RawMessage
}

type ListProvisioningTemplatesOptions struct {
	ProjectID *string
	Provider  string
	Enabled   *bool
	Limit     int
	Offset    int
}

func validateProvisioningTemplateFields(
	projectID *string,
	name string,
	version int32,
	description *string,
	provider *string,
	imageReference *string,
	configuration json.RawMessage,
) error {
	if projectID != nil && !validUUID(*projectID) {
		return errors.New("project_id must be a valid UUID")
	}

	if strings.TrimSpace(name) == "" {
		return errors.New("name is required")
	}
	if len(name) > 150 {
		return errors.New("name exceeds 150 characters")
	}

	if version < 1 {
		return errors.New("version must be greater than zero")
	}

	if description != nil &&
		strings.ContainsRune(*description, '\x00') {
		return errors.New(
			"description contains a null character",
		)
	}

	if err := validateOptionalString(
		"provider", provider, 50,
	); err != nil {
		return err
	}

	if err := validateOptionalString(
		"image_reference", imageReference, 255,
	); err != nil {
		return err
	}

	if len(configuration) == 0 {
		return errors.New("configuration is required")
	}

	var value any
	if err := json.Unmarshal(configuration, &value); err != nil {
		return fmt.Errorf(
			"invalid configuration JSON: %w", err,
		)
	}

	if _, ok := value.(map[string]any); !ok {
		return errors.New(
			"configuration must be a JSON object",
		)
	}

	return nil
}

func (in CreateProvisioningTemplateInput) Validate() error {
	if in.Version < 0 {
		return errors.New("version must be greater than zero")
	}

	version := in.Version
	if version == 0 {
		version = 1
	}

	return validateProvisioningTemplateFields(
		in.ProjectID,
		in.Name,
		version,
		in.Description,
		in.Provider,
		in.ImageReference,
		in.Configuration,
	)
}

func (in UpdateProvisioningTemplateInput) Validate() error {
	return validateProvisioningTemplateFields(
		in.ProjectID,
		in.Name,
		in.Version,
		in.Description,
		in.Provider,
		in.ImageReference,
		in.Configuration,
	)
}

type ProvisioningTemplateRepository struct {
	pool *pgxpool.Pool
}

func NewProvisioningTemplateRepository(
	db *database.Database,
) (*ProvisioningTemplateRepository, error) {
	if db == nil || db.Pool() == nil {
		return nil, errors.New(
			"database connection is required",
		)
	}

	return &ProvisioningTemplateRepository{
		pool: db.Pool(),
	}, nil
}

const provisioningTemplateColumns = `
	id,
	project_id,
	name,
	version,
	description,
	provider,
	image_reference,
	configuration,
	enabled,
	created_at,
	updated_at
`

func scanProvisioningTemplate(
	row pgx.Row,
) (*ProvisioningTemplate, error) {
	var template ProvisioningTemplate

	err := row.Scan(
		&template.ID,
		&template.ProjectID,
		&template.Name,
		&template.Version,
		&template.Description,
		&template.Provider,
		&template.ImageReference,
		&template.Configuration,
		&template.Enabled,
		&template.CreatedAt,
		&template.UpdatedAt,
	)
	if err != nil {
		return nil, mapProvisioningTemplateError(err)
	}

	return &template, nil
}

func (r *ProvisioningTemplateRepository) Create(
	ctx context.Context,
	input CreateProvisioningTemplateInput,
) (*ProvisioningTemplate, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}

	version := input.Version
	if version == 0 {
		version = 1
	}

	query := `
		INSERT INTO provisioning_templates (
			project_id,
			name,
			version,
			description,
			provider,
			image_reference,
			configuration
		)
		VALUES (
			$1::uuid, $2, $3, $4, $5, $6, $7::jsonb
		)
		RETURNING ` + provisioningTemplateColumns

	return scanProvisioningTemplate(
		r.pool.QueryRow(
			ctx,
			query,
			input.ProjectID,
			input.Name,
			version,
			input.Description,
			input.Provider,
			input.ImageReference,
			string(input.Configuration),
		),
	)
}

func (r *ProvisioningTemplateRepository) GetByID(
	ctx context.Context,
	id string,
) (*ProvisioningTemplate, error) {
	if !validUUID(id) {
		return nil, errors.New(
			"id must be a valid UUID",
		)
	}

	query := `
		SELECT ` + provisioningTemplateColumns + `
		FROM provisioning_templates
		WHERE id = $1
	`

	return scanProvisioningTemplate(
		r.pool.QueryRow(ctx, query, id),
	)
}

func (r *ProvisioningTemplateRepository) List(
	ctx context.Context,
	options ListProvisioningTemplatesOptions,
) ([]ProvisioningTemplate, error) {
	if options.ProjectID != nil &&
		!validUUID(*options.ProjectID) {
		return nil, errors.New(
			"project_id must be a valid UUID",
		)
	}

	if len(options.Provider) > 50 {
		return nil, errors.New(
			"provider exceeds 50 characters",
		)
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

	var provider any
	if options.Provider != "" {
		provider = options.Provider
	}

	query := `
		SELECT ` + provisioningTemplateColumns + `
		FROM provisioning_templates
		WHERE (
			$1::uuid IS NULL
			OR project_id = $1::uuid
		)
		AND (
			$2::text IS NULL
			OR provider = $2::text
		)
		AND (
			$3::boolean IS NULL
			OR enabled = $3::boolean
		)
		ORDER BY name, version DESC, id
		LIMIT $4 OFFSET $5
	`

	rows, err := r.pool.Query(
		ctx,
		query,
		options.ProjectID,
		provider,
		options.Enabled,
		limit,
		options.Offset,
	)
	if err != nil {
		return nil, mapProvisioningTemplateError(err)
	}
	defer rows.Close()

	templates := make([]ProvisioningTemplate, 0)

	for rows.Next() {
		var template ProvisioningTemplate

		err := rows.Scan(
			&template.ID,
			&template.ProjectID,
			&template.Name,
			&template.Version,
			&template.Description,
			&template.Provider,
			&template.ImageReference,
			&template.Configuration,
			&template.Enabled,
			&template.CreatedAt,
			&template.UpdatedAt,
		)
		if err != nil {
			return nil, mapProvisioningTemplateError(err)
		}

		templates = append(templates, template)
	}

	if err := rows.Err(); err != nil {
		return nil, mapProvisioningTemplateError(err)
	}

	return templates, nil
}

func (r *ProvisioningTemplateRepository) Update(
	ctx context.Context,
	id string,
	input UpdateProvisioningTemplateInput,
) (*ProvisioningTemplate, error) {
	if !validUUID(id) {
		return nil, errors.New(
			"id must be a valid UUID",
		)
	}

	if err := input.Validate(); err != nil {
		return nil, err
	}

	query := `
		UPDATE provisioning_templates
		SET
			project_id = $2::uuid,
			name = $3,
			version = $4,
			description = $5,
			provider = $6,
			image_reference = $7,
			configuration = $8::jsonb,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
		RETURNING ` + provisioningTemplateColumns

	return scanProvisioningTemplate(
		r.pool.QueryRow(
			ctx,
			query,
			id,
			input.ProjectID,
			input.Name,
			input.Version,
			input.Description,
			input.Provider,
			input.ImageReference,
			string(input.Configuration),
		),
	)
}

func (r *ProvisioningTemplateRepository) SetEnabled(
	ctx context.Context,
	id string,
	enabled bool,
) (*ProvisioningTemplate, error) {
	if !validUUID(id) {
		return nil, errors.New(
			"id must be a valid UUID",
		)
	}

	query := `
		UPDATE provisioning_templates
		SET
			enabled = $2,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
		RETURNING ` + provisioningTemplateColumns

	return scanProvisioningTemplate(
		r.pool.QueryRow(ctx, query, id, enabled),
	)
}

func (r *ProvisioningTemplateRepository) Delete(
	ctx context.Context,
	id string,
) error {
	if !validUUID(id) {
		return errors.New(
			"id must be a valid UUID",
		)
	}

	tag, err := r.pool.Exec(
		ctx,
		`DELETE FROM provisioning_templates WHERE id = $1`,
		id,
	)
	if err != nil {
		return mapProvisioningTemplateError(err)
	}

	if tag.RowsAffected() == 0 {
		return ErrProvisioningTemplateNotFound
	}

	return nil
}

func mapProvisioningTemplateError(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, pgx.ErrNoRows) {
		return ErrProvisioningTemplateNotFound
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return fmt.Errorf(
				"%w: %v",
				ErrProvisioningTemplateConflict,
				err,
			)

		case "23503":
			return fmt.Errorf(
				"provisioning template foreign-key constraint: %w",
				err,
			)

		case "23514":
			return fmt.Errorf(
				"provisioning template check constraint: %w",
				err,
			)

		case "23502":
			return fmt.Errorf(
				"provisioning template required field: %w",
				err,
			)
		}
	}

	return err
}
