package persistence

import (
	"context"
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
	ErrInstanceNetworkAttachmentNotFound = errors.New(
		"instance network attachment not found",
	)
	ErrInstanceNetworkAttachmentConflict = errors.New(
		"instance network attachment conflict",
	)
)

type InstanceNetworkAttachment struct {
	ID            string
	InstanceID    string
	NetworkID     string
	PrivateIP     *string
	PublicIP      *string
	InterfaceName *string
	IsPrimary     *bool
	AttachedAt    *time.Time
	DetachedAt    *time.Time
}

type CreateInstanceNetworkAttachmentInput struct {
	InstanceID    string
	NetworkID     string
	PrivateIP     *string
	PublicIP      *string
	InterfaceName *string
	IsPrimary     *bool
	AttachedAt    *time.Time
	DetachedAt    *time.Time
}

type UpdateInstanceNetworkAttachmentInput struct {
	PrivateIP     *string
	PublicIP      *string
	InterfaceName *string
	IsPrimary     *bool
	AttachedAt    *time.Time
	DetachedAt    *time.Time
}

type ListInstanceNetworkAttachmentsOptions struct {
	InstanceID *string
	NetworkID  *string
	IsPrimary  *bool
	Limit      int
	Offset     int
}

type InstanceNetworkAttachmentRepository struct {
	pool *pgxpool.Pool
}

func NewInstanceNetworkAttachmentRepository(
	db *database.Database,
) (*InstanceNetworkAttachmentRepository, error) {
	if db == nil || db.Pool() == nil {
		return nil, errors.New("database connection is required")
	}

	return &InstanceNetworkAttachmentRepository{
		pool: db.Pool(),
	}, nil
}

func validateAttachmentIP(name string, value *string) error {
	if value == nil {
		return nil
	}

	if strings.ContainsRune(*value, '\x00') {
		return fmt.Errorf("%s contains a null character", name)
	}

	address, err := netip.ParseAddr(*value)
	if err != nil || !address.IsValid() {
		return fmt.Errorf(
			"%s must be a valid IPv4 or IPv6 address",
			name,
		)
	}

	return nil
}

func validateAttachmentFields(
	privateIP *string,
	publicIP *string,
	interfaceName *string,
	attachedAt *time.Time,
	detachedAt *time.Time,
) error {
	if err := validateAttachmentIP("private_ip", privateIP); err != nil {
		return err
	}

	if err := validateAttachmentIP("public_ip", publicIP); err != nil {
		return err
	}

	if err := validateOptionalString(
		"interface_name",
		interfaceName,
		100,
	); err != nil {
		return err
	}

	if interfaceName != nil &&
		strings.ContainsRune(*interfaceName, '\x00') {
		return errors.New(
			"interface_name contains a null character",
		)
	}

	if attachedAt != nil && attachedAt.IsZero() {
		return errors.New(
			"attached_at cannot be a zero timestamp",
		)
	}

	if detachedAt != nil && detachedAt.IsZero() {
		return errors.New(
			"detached_at cannot be a zero timestamp",
		)
	}

	return nil
}

func (in CreateInstanceNetworkAttachmentInput) Validate() error {
	if !validUUID(in.InstanceID) {
		return errors.New(
			"instance_id must be a valid UUID",
		)
	}

	if !validUUID(in.NetworkID) {
		return errors.New(
			"network_id must be a valid UUID",
		)
	}

	return validateAttachmentFields(
		in.PrivateIP,
		in.PublicIP,
		in.InterfaceName,
		in.AttachedAt,
		in.DetachedAt,
	)
}

func (in UpdateInstanceNetworkAttachmentInput) Validate() error {
	return validateAttachmentFields(
		in.PrivateIP,
		in.PublicIP,
		in.InterfaceName,
		in.AttachedAt,
		in.DetachedAt,
	)
}

const instanceNetworkAttachmentColumns = `
	id,
	instance_id,
	network_id,
	private_ip::text,
	public_ip::text,
	interface_name,
	is_primary,
	attached_at,
	detached_at
`

func normalizeAttachmentINET(
	value *string,
) (*string, error) {
	if value == nil {
		return nil, nil
	}

	if address, err := netip.ParseAddr(*value); err == nil {
		normalized := address.String()
		return &normalized, nil
	}

	prefix, err := netip.ParsePrefix(*value)
	if err != nil {
		return nil, fmt.Errorf(
			"invalid PostgreSQL INET address: %q",
			*value,
		)
	}

	normalized := prefix.Addr().String()
	return &normalized, nil
}

func scanInstanceNetworkAttachment(
	row pgx.Row,
) (*InstanceNetworkAttachment, error) {
	var attachment InstanceNetworkAttachment

	err := row.Scan(
		&attachment.ID,
		&attachment.InstanceID,
		&attachment.NetworkID,
		&attachment.PrivateIP,
		&attachment.PublicIP,
		&attachment.InterfaceName,
		&attachment.IsPrimary,
		&attachment.AttachedAt,
		&attachment.DetachedAt,
	)
	if err != nil {
		return nil, mapInstanceNetworkAttachmentError(err)
	}

	attachment.PrivateIP, err = normalizeAttachmentINET(
		attachment.PrivateIP,
	)
	if err != nil {
		return nil, err
	}

	attachment.PublicIP, err = normalizeAttachmentINET(
		attachment.PublicIP,
	)
	if err != nil {
		return nil, err
	}

	return &attachment, nil
}

func (r *InstanceNetworkAttachmentRepository) Create(
	ctx context.Context,
	input CreateInstanceNetworkAttachmentInput,
) (*InstanceNetworkAttachment, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}

	query := `
		INSERT INTO instance_network_attachments (
			instance_id,
			network_id,
			private_ip,
			public_ip,
			interface_name,
			is_primary,
			attached_at,
			detached_at
		)
		VALUES (
			$1,
			$2,
			$3::inet,
			$4::inet,
			$5,
			COALESCE($6::boolean, false),
			$7,
			$8
		)
		RETURNING ` + instanceNetworkAttachmentColumns

	return scanInstanceNetworkAttachment(
		r.pool.QueryRow(
			ctx,
			query,
			input.InstanceID,
			input.NetworkID,
			input.PrivateIP,
			input.PublicIP,
			input.InterfaceName,
			input.IsPrimary,
			input.AttachedAt,
			input.DetachedAt,
		),
	)
}

func (r *InstanceNetworkAttachmentRepository) GetByID(
	ctx context.Context,
	id string,
) (*InstanceNetworkAttachment, error) {
	if !validUUID(id) {
		return nil, errors.New(
			"id must be a valid UUID",
		)
	}

	query := `
		SELECT ` + instanceNetworkAttachmentColumns + `
		FROM instance_network_attachments
		WHERE id = $1
	`

	return scanInstanceNetworkAttachment(
		r.pool.QueryRow(ctx, query, id),
	)
}

func (r *InstanceNetworkAttachmentRepository) List(
	ctx context.Context,
	options ListInstanceNetworkAttachmentsOptions,
) ([]InstanceNetworkAttachment, error) {
	if options.InstanceID != nil &&
		!validUUID(*options.InstanceID) {
		return nil, errors.New(
			"instance_id must be a valid UUID",
		)
	}

	if options.NetworkID != nil &&
		!validUUID(*options.NetworkID) {
		return nil, errors.New(
			"network_id must be a valid UUID",
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

	var instanceID, networkID, isPrimary any

	if options.InstanceID != nil {
		instanceID = *options.InstanceID
	}

	if options.NetworkID != nil {
		networkID = *options.NetworkID
	}

	if options.IsPrimary != nil {
		isPrimary = *options.IsPrimary
	}

	query := `
		SELECT ` + instanceNetworkAttachmentColumns + `
		FROM instance_network_attachments
		WHERE
			($1::uuid IS NULL OR instance_id = $1::uuid)
			AND ($2::uuid IS NULL OR network_id = $2::uuid)
			AND ($3::boolean IS NULL OR is_primary = $3::boolean)
		ORDER BY id ASC
		LIMIT $4 OFFSET $5
	`

	rows, err := r.pool.Query(
		ctx,
		query,
		instanceID,
		networkID,
		isPrimary,
		limit,
		options.Offset,
	)
	if err != nil {
		return nil, mapInstanceNetworkAttachmentError(err)
	}
	defer rows.Close()

	attachments := make([]InstanceNetworkAttachment, 0)

	for rows.Next() {
		attachment, err := scanInstanceNetworkAttachment(rows)
		if err != nil {
			return nil, err
		}

		attachments = append(
			attachments,
			*attachment,
		)
	}

	if err := rows.Err(); err != nil {
		return nil, mapInstanceNetworkAttachmentError(err)
	}

	return attachments, nil
}

func (r *InstanceNetworkAttachmentRepository) Update(
	ctx context.Context,
	id string,
	input UpdateInstanceNetworkAttachmentInput,
) (*InstanceNetworkAttachment, error) {
	if !validUUID(id) {
		return nil, errors.New(
			"id must be a valid UUID",
		)
	}

	if err := input.Validate(); err != nil {
		return nil, err
	}

	query := `
		UPDATE instance_network_attachments
		SET
			private_ip = $2::inet,
			public_ip = $3::inet,
			interface_name = $4,
			is_primary = $5::boolean,
			attached_at = $6,
			detached_at = $7
		WHERE id = $1
		RETURNING ` + instanceNetworkAttachmentColumns

	return scanInstanceNetworkAttachment(
		r.pool.QueryRow(
			ctx,
			query,
			id,
			input.PrivateIP,
			input.PublicIP,
			input.InterfaceName,
			input.IsPrimary,
			input.AttachedAt,
			input.DetachedAt,
		),
	)
}

func (r *InstanceNetworkAttachmentRepository) SetPrimary(
	ctx context.Context,
	id string,
	isPrimary *bool,
) (*InstanceNetworkAttachment, error) {
	if !validUUID(id) {
		return nil, errors.New(
			"id must be a valid UUID",
		)
	}

	query := `
		UPDATE instance_network_attachments
		SET is_primary = $2::boolean
		WHERE id = $1
		RETURNING ` + instanceNetworkAttachmentColumns

	return scanInstanceNetworkAttachment(
		r.pool.QueryRow(
			ctx,
			query,
			id,
			isPrimary,
		),
	)
}

func (r *InstanceNetworkAttachmentRepository) Detach(
	ctx context.Context,
	id string,
	detachedAt time.Time,
) (*InstanceNetworkAttachment, error) {
	if !validUUID(id) {
		return nil, errors.New(
			"id must be a valid UUID",
		)
	}

	if detachedAt.IsZero() {
		return nil, errors.New(
			"detached_at cannot be a zero timestamp",
		)
	}

	query := `
		UPDATE instance_network_attachments
		SET detached_at = $2
		WHERE id = $1
		RETURNING ` + instanceNetworkAttachmentColumns

	return scanInstanceNetworkAttachment(
		r.pool.QueryRow(
			ctx,
			query,
			id,
			detachedAt,
		),
	)
}

func (r *InstanceNetworkAttachmentRepository) Delete(
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
		`DELETE FROM instance_network_attachments WHERE id = $1`,
		id,
	)
	if err != nil {
		return mapInstanceNetworkAttachmentError(err)
	}

	if tag.RowsAffected() == 0 {
		return ErrInstanceNetworkAttachmentNotFound
	}

	return nil
}

func mapInstanceNetworkAttachmentError(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, pgx.ErrNoRows) {
		return ErrInstanceNetworkAttachmentNotFound
	}

	var pgErr *pgconn.PgError

	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return fmt.Errorf(
				"%w: %v",
				ErrInstanceNetworkAttachmentConflict,
				err,
			)

		case "23503":
			return fmt.Errorf(
				"instance network attachment foreign-key constraint: %w",
				err,
			)

		case "23514":
			return fmt.Errorf(
				"instance network attachment check constraint: %w",
				err,
			)

		case "23502":
			return fmt.Errorf(
				"instance network attachment required field: %w",
				err,
			)
		}
	}

	return err
}
