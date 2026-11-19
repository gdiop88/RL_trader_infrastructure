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
	ErrProviderAccountNotFound = errors.New("provider account not found")
	ErrProviderAccountConflict = errors.New("provider account conflict")
)

type ProviderAccount struct {
	ID                  string          `json:"id"`
	ProviderID          string          `json:"provider_id"`
	Name                string          `json:"name"`
	ExternalAccountID   *string         `json:"external_account_id,omitempty"`
	CredentialReference *string         `json:"credential_reference,omitempty"`
	Enabled             bool            `json:"enabled"`
	Metadata            json.RawMessage `json:"metadata"`
	CreatedAt           time.Time       `json:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at"`
}

type CreateProviderAccountInput struct {
	ProviderID          string
	Name                string
	ExternalAccountID   *string
	CredentialReference *string
	Metadata            json.RawMessage
}

type UpdateProviderAccountInput struct {
	Name                string
	ExternalAccountID   *string
	CredentialReference *string
	Metadata            json.RawMessage
}

type ListProviderAccountsOptions struct {
	ProviderID string
	Enabled    *bool
	Limit      int
	Offset     int
}

func validateProviderAccountFields(
	name string,
	externalID *string,
	credentialReference *string,
	metadata json.RawMessage,
) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("account name is required")
	}
	if len(name) > 150 {
		return errors.New("account name exceeds 150 characters")
	}
	if err := validateOptionalString(
		"external account ID", externalID, 255,
	); err != nil {
		return err
	}
	if err := validateOptionalString(
		"credential reference", credentialReference, 500,
	); err != nil {
		return err
	}
	return validateJSONType("metadata", metadata, "object")
}

func (input CreateProviderAccountInput) Validate() error {
	if !validUUID(input.ProviderID) {
		return errors.New("provider ID must be a valid UUID")
	}
	return validateProviderAccountFields(
		input.Name,
		input.ExternalAccountID,
		input.CredentialReference,
		input.Metadata,
	)
}

func (input UpdateProviderAccountInput) Validate() error {
	return validateProviderAccountFields(
		input.Name,
		input.ExternalAccountID,
		input.CredentialReference,
		input.Metadata,
	)
}

type ProviderAccountRepository struct {
	pool *pgxpool.Pool
}

func NewProviderAccountRepository(
	db *database.Database,
) (*ProviderAccountRepository, error) {
	if db == nil || db.Pool() == nil {
		return nil, errors.New("database pool is not initialized")
	}
	return &ProviderAccountRepository{pool: db.Pool()}, nil
}

const providerAccountColumns = `
	id,
	provider_id,
	name,
	external_account_id,
	credential_reference,
	enabled,
	metadata,
	created_at,
	updated_at
`

func scanProviderAccount(row pgx.Row) (*ProviderAccount, error) {
	var account ProviderAccount
	err := row.Scan(
		&account.ID,
		&account.ProviderID,
		&account.Name,
		&account.ExternalAccountID,
		&account.CredentialReference,
		&account.Enabled,
		&account.Metadata,
		&account.CreatedAt,
		&account.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &account, nil
}

func mapProviderAccountError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrProviderAccountNotFound
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return fmt.Errorf("%w: %v", ErrProviderAccountConflict, err)
		case "23503":
			return fmt.Errorf(
				"invalid or referenced provider account resource: %w",
				err,
			)
		}
	}
	return err
}

func validateProviderAccountID(id string) error {
	if !validUUID(id) {
		return errors.New("provider account ID must be a valid UUID")
	}
	return nil
}

func (r *ProviderAccountRepository) Create(
	ctx context.Context,
	input CreateProviderAccountInput,
) (*ProviderAccount, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}

	query := `
		INSERT INTO provider_accounts (
			provider_id,
			name,
			external_account_id,
			credential_reference,
			metadata
		)
		VALUES ($1::uuid, $2, $3, $4, $5::jsonb)
		RETURNING ` + providerAccountColumns

	account, err := scanProviderAccount(r.pool.QueryRow(
		ctx,
		query,
		input.ProviderID,
		strings.TrimSpace(input.Name),
		input.ExternalAccountID,
		input.CredentialReference,
		jsonOrDefault(input.Metadata, `{}`),
	))
	if err != nil {
		return nil, mapProviderAccountError(err)
	}
	return account, nil
}

func (r *ProviderAccountRepository) GetByID(
	ctx context.Context,
	id string,
) (*ProviderAccount, error) {
	if err := validateProviderAccountID(id); err != nil {
		return nil, err
	}

	query := `
		SELECT ` + providerAccountColumns + `
		FROM provider_accounts
		WHERE id = $1::uuid
	`

	account, err := scanProviderAccount(
		r.pool.QueryRow(ctx, query, id),
	)
	if err != nil {
		return nil, mapProviderAccountError(err)
	}
	return account, nil
}

func (r *ProviderAccountRepository) List(
	ctx context.Context,
	options ListProviderAccountsOptions,
) ([]ProviderAccount, error) {
	if options.ProviderID != "" && !validUUID(options.ProviderID) {
		return nil, errors.New("provider ID must be a valid UUID")
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
		SELECT ` + providerAccountColumns + `
		FROM provider_accounts
		WHERE ($1::uuid IS NULL OR provider_id = $1::uuid)
		  AND ($2::boolean IS NULL OR enabled = $2::boolean)
		ORDER BY created_at DESC, id DESC
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
		return nil, mapProviderAccountError(err)
	}
	defer rows.Close()

	accounts := make([]ProviderAccount, 0)
	for rows.Next() {
		account, err := scanProviderAccount(rows)
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, *account)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return accounts, nil
}

func (r *ProviderAccountRepository) Update(
	ctx context.Context,
	id string,
	input UpdateProviderAccountInput,
) (*ProviderAccount, error) {
	if err := validateProviderAccountID(id); err != nil {
		return nil, err
	}
	if err := input.Validate(); err != nil {
		return nil, err
	}

	query := `
		UPDATE provider_accounts
		SET name = $2,
		    external_account_id = $3,
		    credential_reference = $4,
		    metadata = $5::jsonb,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = $1::uuid
		RETURNING ` + providerAccountColumns

	account, err := scanProviderAccount(r.pool.QueryRow(
		ctx,
		query,
		id,
		strings.TrimSpace(input.Name),
		input.ExternalAccountID,
		input.CredentialReference,
		jsonOrDefault(input.Metadata, `{}`),
	))
	if err != nil {
		return nil, mapProviderAccountError(err)
	}
	return account, nil
}

func (r *ProviderAccountRepository) SetEnabled(
	ctx context.Context,
	id string,
	enabled bool,
) (*ProviderAccount, error) {
	if err := validateProviderAccountID(id); err != nil {
		return nil, err
	}

	query := `
		UPDATE provider_accounts
		SET enabled = $2,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = $1::uuid
		RETURNING ` + providerAccountColumns

	account, err := scanProviderAccount(
		r.pool.QueryRow(ctx, query, id, enabled),
	)
	if err != nil {
		return nil, mapProviderAccountError(err)
	}
	return account, nil
}

func (r *ProviderAccountRepository) Delete(
	ctx context.Context,
	id string,
) error {
	if err := validateProviderAccountID(id); err != nil {
		return err
	}

	tag, err := r.pool.Exec(
		ctx,
		`DELETE FROM provider_accounts WHERE id = $1::uuid`,
		id,
	)
	if err != nil {
		return mapProviderAccountError(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrProviderAccountNotFound
	}
	return nil
}
