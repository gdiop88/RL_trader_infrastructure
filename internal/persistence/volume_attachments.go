package persistence

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gdiop88/RL_trader_infrastructure/internal/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrVolumeAttachmentNotFound = errors.New(
		"volume attachment not found",
	)
	ErrVolumeAttachmentConflict = errors.New(
		"volume attachment conflict",
	)
)

type VolumeAttachment struct {
	ID         string
	VolumeID   string
	InstanceID string
	MountPoint *string
	Status     *string
	AttachedAt *time.Time
	DetachedAt *time.Time
}

type CreateVolumeAttachmentInput struct {
	VolumeID   string
	InstanceID string
	MountPoint *string
	Status     *string
	AttachedAt *time.Time
	DetachedAt *time.Time
}

type UpdateVolumeAttachmentInput struct {
	MountPoint *string
	Status     *string
	AttachedAt *time.Time
	DetachedAt *time.Time
}

type ListVolumeAttachmentsOptions struct {
	VolumeID        *string
	InstanceID      *string
	Status          *string
	IncludeDetached bool
	Limit           int
	Offset          int
}

type VolumeAttachmentRepository struct {
	pool *pgxpool.Pool
}

func NewVolumeAttachmentRepository(
	db *database.Database,
) (*VolumeAttachmentRepository, error) {
	if db == nil || db.Pool() == nil {
		return nil, errors.New("database connection is required")
	}

	return &VolumeAttachmentRepository{
		pool: db.Pool(),
	}, nil
}

func validateVolumeAttachmentFields(
	mountPoint *string,
	status *string,
	attachedAt *time.Time,
	detachedAt *time.Time,
) error {
	if err := validateOptionalString(
		"mount_point",
		mountPoint,
		255,
	); err != nil {
		return err
	}

	if err := validateOptionalString(
		"status",
		status,
		50,
	); err != nil {
		return err
	}

	if attachedAt != nil && attachedAt.IsZero() {
		return errors.New("attached_at cannot be a zero timestamp")
	}

	if detachedAt != nil && detachedAt.IsZero() {
		return errors.New("detached_at cannot be a zero timestamp")
	}

	return nil
}

func (in CreateVolumeAttachmentInput) Validate() error {
	if !validUUID(in.VolumeID) {
		return errors.New("volume_id must be a valid UUID")
	}

	if !validUUID(in.InstanceID) {
		return errors.New("instance_id must be a valid UUID")
	}

	return validateVolumeAttachmentFields(
		in.MountPoint,
		in.Status,
		in.AttachedAt,
		in.DetachedAt,
	)
}

func (in UpdateVolumeAttachmentInput) Validate() error {
	return validateVolumeAttachmentFields(
		in.MountPoint,
		in.Status,
		in.AttachedAt,
		in.DetachedAt,
	)
}

const volumeAttachmentColumns = `
	id,
	volume_id,
	instance_id,
	mount_point,
	status,
	attached_at,
	detached_at
`

func scanVolumeAttachment(
	row pgx.Row,
) (*VolumeAttachment, error) {
	var attachment VolumeAttachment

	err := row.Scan(
		&attachment.ID,
		&attachment.VolumeID,
		&attachment.InstanceID,
		&attachment.MountPoint,
		&attachment.Status,
		&attachment.AttachedAt,
		&attachment.DetachedAt,
	)
	if err != nil {
		return nil, mapVolumeAttachmentError(err)
	}

	return &attachment, nil
}

func (r *VolumeAttachmentRepository) Create(
	ctx context.Context,
	input CreateVolumeAttachmentInput,
) (*VolumeAttachment, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}

	query := `
		INSERT INTO volume_attachments (
			volume_id,
			instance_id,
			mount_point,
			status,
			attached_at,
			detached_at
		)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING ` + volumeAttachmentColumns

	return scanVolumeAttachment(
		r.pool.QueryRow(
			ctx,
			query,
			input.VolumeID,
			input.InstanceID,
			input.MountPoint,
			input.Status,
			input.AttachedAt,
			input.DetachedAt,
		),
	)
}

func (r *VolumeAttachmentRepository) GetByID(
	ctx context.Context,
	id string,
) (*VolumeAttachment, error) {
	if !validUUID(id) {
		return nil, errors.New("id must be a valid UUID")
	}

	query := `
		SELECT ` + volumeAttachmentColumns + `
		FROM volume_attachments
		WHERE id = $1
	`

	return scanVolumeAttachment(
		r.pool.QueryRow(ctx, query, id),
	)
}

func (r *VolumeAttachmentRepository) List(
	ctx context.Context,
	options ListVolumeAttachmentsOptions,
) ([]VolumeAttachment, error) {
	if options.VolumeID != nil &&
		!validUUID(*options.VolumeID) {
		return nil, errors.New("volume_id must be a valid UUID")
	}

	if options.InstanceID != nil &&
		!validUUID(*options.InstanceID) {
		return nil, errors.New("instance_id must be a valid UUID")
	}

	if err := validateOptionalString(
		"status",
		options.Status,
		50,
	); err != nil {
		return nil, err
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

	var volumeID, instanceID, status any

	if options.VolumeID != nil {
		volumeID = *options.VolumeID
	}
	if options.InstanceID != nil {
		instanceID = *options.InstanceID
	}
	if options.Status != nil {
		status = *options.Status
	}

	query := `
		SELECT ` + volumeAttachmentColumns + `
		FROM volume_attachments
		WHERE
			($1::uuid IS NULL OR volume_id = $1::uuid)
			AND ($2::uuid IS NULL OR instance_id = $2::uuid)
			AND ($3::text IS NULL OR status = $3::text)
			AND ($4::boolean OR detached_at IS NULL)
		ORDER BY id ASC
		LIMIT $5 OFFSET $6
	`

	rows, err := r.pool.Query(
		ctx,
		query,
		volumeID,
		instanceID,
		status,
		options.IncludeDetached,
		limit,
		options.Offset,
	)
	if err != nil {
		return nil, mapVolumeAttachmentError(err)
	}
	defer rows.Close()

	attachments := make([]VolumeAttachment, 0)

	for rows.Next() {
		attachment, err := scanVolumeAttachment(rows)
		if err != nil {
			return nil, err
		}

		attachments = append(attachments, *attachment)
	}

	if err := rows.Err(); err != nil {
		return nil, mapVolumeAttachmentError(err)
	}

	return attachments, nil
}

func (r *VolumeAttachmentRepository) Update(
	ctx context.Context,
	id string,
	input UpdateVolumeAttachmentInput,
) (*VolumeAttachment, error) {
	if !validUUID(id) {
		return nil, errors.New("id must be a valid UUID")
	}

	if err := input.Validate(); err != nil {
		return nil, err
	}

	query := `
		UPDATE volume_attachments
		SET
			mount_point = $2,
			status = $3,
			attached_at = $4,
			detached_at = $5
		WHERE id = $1
		RETURNING ` + volumeAttachmentColumns

	return scanVolumeAttachment(
		r.pool.QueryRow(
			ctx,
			query,
			id,
			input.MountPoint,
			input.Status,
			input.AttachedAt,
			input.DetachedAt,
		),
	)
}

func (r *VolumeAttachmentRepository) UpdateStatus(
	ctx context.Context,
	id string,
	status *string,
) (*VolumeAttachment, error) {
	if !validUUID(id) {
		return nil, errors.New("id must be a valid UUID")
	}

	if err := validateOptionalString(
		"status",
		status,
		50,
	); err != nil {
		return nil, err
	}

	query := `
		UPDATE volume_attachments
		SET status = $2
		WHERE id = $1
		RETURNING ` + volumeAttachmentColumns

	return scanVolumeAttachment(
		r.pool.QueryRow(ctx, query, id, status),
	)
}

func (r *VolumeAttachmentRepository) Detach(
	ctx context.Context,
	id string,
	detachedAt time.Time,
) (*VolumeAttachment, error) {
	if !validUUID(id) {
		return nil, errors.New("id must be a valid UUID")
	}

	if detachedAt.IsZero() {
		return nil, errors.New(
			"detached_at cannot be a zero timestamp",
		)
	}

	query := `
		UPDATE volume_attachments
		SET detached_at = $2
		WHERE id = $1
		RETURNING ` + volumeAttachmentColumns

	return scanVolumeAttachment(
		r.pool.QueryRow(ctx, query, id, detachedAt),
	)
}

func (r *VolumeAttachmentRepository) Delete(
	ctx context.Context,
	id string,
) error {
	if !validUUID(id) {
		return errors.New("id must be a valid UUID")
	}

	tag, err := r.pool.Exec(
		ctx,
		`DELETE FROM volume_attachments WHERE id = $1`,
		id,
	)
	if err != nil {
		return mapVolumeAttachmentError(err)
	}

	if tag.RowsAffected() == 0 {
		return ErrVolumeAttachmentNotFound
	}

	return nil
}

func mapVolumeAttachmentError(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, pgx.ErrNoRows) {
		return ErrVolumeAttachmentNotFound
	}

	var pgErr *pgconn.PgError

	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return fmt.Errorf(
				"%w: %v",
				ErrVolumeAttachmentConflict,
				err,
			)

		case "23503":
			return fmt.Errorf(
				"volume attachment foreign-key constraint: %w",
				err,
			)

		case "23514":
			return fmt.Errorf(
				"volume attachment check constraint: %w",
				err,
			)

		case "23502":
			return fmt.Errorf(
				"volume attachment required field: %w",
				err,
			)
		}
	}

	return err
}
