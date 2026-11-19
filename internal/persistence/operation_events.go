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
	ErrOperationEventNotFound = errors.New("operation event not found")
	ErrOperationEventConflict = errors.New("operation event conflict")
)

type OperationEvent struct {
	ID             string
	OperationID    string
	EventType      string
	Status         *string
	Message        *string
	EventData      json.RawMessage
	CreatedAt      time.Time
	SequenceNumber *int32
}

type CreateOperationEventInput struct {
	OperationID    string
	EventType      string
	Status         *string
	Message        *string
	EventData      json.RawMessage
	SequenceNumber *int32
}

type ListOperationEventsOptions struct {
	OperationID string
	EventType   string
	Status      *string
	Limit       int
	Offset      int
}

type OperationEventRepository struct {
	pool *pgxpool.Pool
}

func NewOperationEventRepository(
	db *database.Database,
) (*OperationEventRepository, error) {
	if db == nil || db.Pool() == nil {
		return nil, errors.New("database connection is required")
	}

	return &OperationEventRepository{
		pool: db.Pool(),
	}, nil
}

func (in CreateOperationEventInput) Validate() error {
	if !validUUID(in.OperationID) {
		return errors.New("operation_id must be a valid UUID")
	}

	if strings.TrimSpace(in.EventType) == "" ||
		len(in.EventType) > 100 ||
		strings.ContainsRune(in.EventType, '\x00') {
		return errors.New(
			"event_type must contain 1 to 100 valid characters",
		)
	}

	if err := validateOptionalString(
		"status",
		in.Status,
		50,
	); err != nil {
		return err
	}

	// PostgreSQL TEXT has no VARCHAR-style length limit.
	// Reject only null characters, which PostgreSQL cannot store.
	if in.Message != nil &&
		strings.ContainsRune(*in.Message, '\x00') {
		return errors.New("message contains a null character")
	}

	if len(in.EventData) > 0 {
		var value any

		if err := json.Unmarshal(in.EventData, &value); err != nil {
			return fmt.Errorf(
				"event_data contains invalid JSON: %w",
				err,
			)
		}

		if _, ok := value.(map[string]any); !ok {
			return errors.New("event_data must be a JSON object")
		}
	}

	if in.SequenceNumber != nil && *in.SequenceNumber < 0 {
		return errors.New("sequence_number cannot be negative")
	}

	return nil
}

const operationEventColumns = `
	id,
	operation_id,
	event_type,
	status,
	message,
	event_data,
	created_at,
	sequence_number
`

func scanOperationEvent(
	row pgx.Row,
) (*OperationEvent, error) {
	var event OperationEvent

	err := row.Scan(
		&event.ID,
		&event.OperationID,
		&event.EventType,
		&event.Status,
		&event.Message,
		&event.EventData,
		&event.CreatedAt,
		&event.SequenceNumber,
	)

	if err != nil {
		return nil, mapOperationEventError(err)
	}

	return &event, nil
}

func (r *OperationEventRepository) Create(
	ctx context.Context,
	input CreateOperationEventInput,
) (*OperationEvent, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}

	data := input.EventData
	if len(data) == 0 {
		data = json.RawMessage(`{}`)
	}

	query := `
		INSERT INTO operation_events (
			operation_id,
			event_type,
			status,
			message,
			event_data,
			sequence_number
		)
		VALUES (
			$1,
			$2,
			$3,
			$4,
			$5::jsonb,
			$6
		)
		RETURNING ` + operationEventColumns

	return scanOperationEvent(
		r.pool.QueryRow(
			ctx,
			query,
			input.OperationID,
			input.EventType,
			input.Status,
			input.Message,
			string(data),
			input.SequenceNumber,
		),
	)
}

func (r *OperationEventRepository) GetByID(
	ctx context.Context,
	id string,
) (*OperationEvent, error) {
	if !validUUID(id) {
		return nil, errors.New("id must be a valid UUID")
	}

	query := `
		SELECT ` + operationEventColumns + `
		FROM operation_events
		WHERE id = $1
	`

	return scanOperationEvent(
		r.pool.QueryRow(ctx, query, id),
	)
}

func (r *OperationEventRepository) GetBySequence(
	ctx context.Context,
	operationID string,
	sequenceNumber int32,
) (*OperationEvent, error) {
	if !validUUID(operationID) {
		return nil, errors.New("operation_id must be a valid UUID")
	}

	if sequenceNumber < 0 {
		return nil, errors.New("sequence_number cannot be negative")
	}

	query := `
		SELECT ` + operationEventColumns + `
		FROM operation_events
		WHERE operation_id = $1
		  AND sequence_number = $2
	`

	return scanOperationEvent(
		r.pool.QueryRow(
			ctx,
			query,
			operationID,
			sequenceNumber,
		),
	)
}

func (r *OperationEventRepository) List(
	ctx context.Context,
	options ListOperationEventsOptions,
) ([]OperationEvent, error) {
	if options.OperationID != "" &&
		!validUUID(options.OperationID) {
		return nil, errors.New("operation_id must be a valid UUID")
	}

	if options.EventType != "" &&
		(len(options.EventType) > 100 ||
			strings.ContainsRune(options.EventType, '\x00')) {
		return nil, errors.New("invalid event_type")
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

	var operationID any
	var eventType any

	if options.OperationID != "" {
		operationID = options.OperationID
	}

	if options.EventType != "" {
		eventType = options.EventType
	}

	query := `
		SELECT ` + operationEventColumns + `
		FROM operation_events
		WHERE
			($1::uuid IS NULL OR operation_id = $1::uuid)
			AND ($2::text IS NULL OR event_type = $2::text)
			AND ($3::text IS NULL OR status = $3::text)
		ORDER BY
			created_at ASC,
			sequence_number ASC NULLS LAST,
			id ASC
		LIMIT $4
		OFFSET $5
	`

	rows, err := r.pool.Query(
		ctx,
		query,
		operationID,
		eventType,
		options.Status,
		limit,
		options.Offset,
	)

	if err != nil {
		return nil, mapOperationEventError(err)
	}
	defer rows.Close()

	events := make([]OperationEvent, 0)

	for rows.Next() {
		event, err := scanOperationEvent(rows)
		if err != nil {
			return nil, err
		}

		events = append(events, *event)
	}

	if err := rows.Err(); err != nil {
		return nil, mapOperationEventError(err)
	}

	return events, nil
}

func (r *OperationEventRepository) Delete(
	ctx context.Context,
	id string,
) error {
	if !validUUID(id) {
		return errors.New("id must be a valid UUID")
	}

	tag, err := r.pool.Exec(
		ctx,
		`DELETE FROM operation_events WHERE id = $1`,
		id,
	)

	if err != nil {
		return mapOperationEventError(err)
	}

	if tag.RowsAffected() == 0 {
		return ErrOperationEventNotFound
	}

	return nil
}

func mapOperationEventError(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, pgx.ErrNoRows) {
		return ErrOperationEventNotFound
	}

	var pgErr *pgconn.PgError

	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return fmt.Errorf(
				"%w: %v",
				ErrOperationEventConflict,
				err,
			)

		case "23503":
			return fmt.Errorf(
				"operation event foreign-key constraint: %w",
				err,
			)

		case "23514":
			return fmt.Errorf(
				"operation event check constraint: %w",
				err,
			)

		case "23502":
			return fmt.Errorf(
				"operation event required field: %w",
				err,
			)
		}
	}

	return err
}
