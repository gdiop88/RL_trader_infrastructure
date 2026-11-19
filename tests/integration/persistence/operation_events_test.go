package persistence_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gdiop88/RL_trader_infrastructure/internal/config"
	"github.com/gdiop88/RL_trader_infrastructure/internal/database"
	"github.com/gdiop88/RL_trader_infrastructure/internal/persistence"
)

func operationEventString(value string) *string {
	return &value
}

func operationEventInt32(value int32) *int32 {
	return &value
}

func TestOperationEventCRUD(t *testing.T) {
	cfg, err := config.Load("../../../configs/server.yaml")
	if err != nil {
		t.Fatalf("load configuration: %v", err)
	}

	connectCtx, connectCancel := context.WithTimeout(
		context.Background(), 15*time.Second,
	)
	defer connectCancel()

	db, err := database.ConnectWithConfig(connectCtx, cfg)
	if err != nil {
		t.Fatalf("connect to PostgreSQL: %v", err)
	}
	t.Cleanup(db.Close)

	ctx, cancel := context.WithTimeout(
		context.Background(), 120*time.Second,
	)
	defer cancel()

	eventRepo, err := persistence.NewOperationEventRepository(db)
	if err != nil {
		t.Fatalf("event repository: %v", err)
	}

	operationRepo, err := persistence.NewProvisioningOperationRepository(db)
	if err != nil {
		t.Fatalf("operation repository: %v", err)
	}

	var operationIDs []string
	var eventIDs []string

	t.Cleanup(func() {
		cleanupCtx, done := context.WithTimeout(
			context.Background(), 30*time.Second,
		)
		defer done()

		for _, id := range eventIDs {
			_, err := db.Pool().Exec(
				cleanupCtx,
				`DELETE FROM operation_events WHERE id = $1`,
				id,
			)
			if err != nil {
				t.Errorf("cleanup event %s: %v", id, err)
			}
		}

		for _, id := range operationIDs {
			_, err := db.Pool().Exec(
				cleanupCtx,
				`DELETE FROM provisioning_operations WHERE id = $1`,
				id,
			)
			if err != nil {
				t.Errorf("cleanup operation %s: %v", id, err)
			}
		}
	})

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())

	operation, err := operationRepo.Create(
		ctx,
		persistence.CreateProvisioningOperationInput{
			OperationType: "create",
			Provider:      "digitalocean",
			IdempotencyKey: operationEventString(
				"event_integration_" + suffix,
			),
		},
	)
	if err != nil {
		t.Fatalf("create parent operation: %v", err)
	}
	operationIDs = append(operationIDs, operation.ID)

	t.Run("Create", func(t *testing.T) {
		event, err := eventRepo.Create(
			ctx,
			persistence.CreateOperationEventInput{
				OperationID: operation.ID,
				EventType:   "provisioning.started",
				Status:      operationEventString("running"),
				Message: operationEventString(
					"Provisioning started",
				),
				EventData: json.RawMessage(
					`{"provider":"digitalocean","region":"nyc3"}`,
				),
				SequenceNumber: operationEventInt32(1),
			},
		)
		if err != nil {
			t.Fatalf("create event: %v", err)
		}
		eventIDs = append(eventIDs, event.ID)

		if event.ID == "" {
			t.Fatal("event ID is empty")
		}
		if event.OperationID != operation.ID {
			t.Fatal("operation ID mismatch")
		}
		if event.EventType != "provisioning.started" {
			t.Fatal("event type mismatch")
		}
		if event.CreatedAt.IsZero() {
			t.Fatal("created_at is missing")
		}
		if event.SequenceNumber == nil ||
			*event.SequenceNumber != 1 {
			t.Fatal("sequence number mismatch")
		}
	})

	t.Run("GetBySequence", func(t *testing.T) {
		event, err := eventRepo.GetBySequence(
			ctx, operation.ID, 1,
		)
		if err != nil {
			t.Fatalf("GetBySequence: %v", err)
		}

		if event.EventType != "provisioning.started" {
			t.Fatal("incorrect event returned")
		}
	})

	t.Run("GetByID", func(t *testing.T) {
		event, err := eventRepo.GetBySequence(
			ctx, operation.ID, 1,
		)
		if err != nil {
			t.Fatalf("find event: %v", err)
		}

		found, err := eventRepo.GetByID(ctx, event.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if found.ID != event.ID {
			t.Fatal("event ID mismatch")
		}
	})

	t.Run("JSONBPersistence", func(t *testing.T) {
		event, err := eventRepo.GetBySequence(
			ctx, operation.ID, 1,
		)
		if err != nil {
			t.Fatalf("find event: %v", err)
		}

		var payload map[string]any
		if err := json.Unmarshal(
			event.EventData, &payload,
		); err != nil {
			t.Fatalf("decode event data: %v", err)
		}

		if payload["provider"] != "digitalocean" {
			t.Fatal("provider not persisted")
		}
		if payload["region"] != "nyc3" {
			t.Fatal("region not persisted")
		}
	})

	t.Run("DuplicateSequence", func(t *testing.T) {
		_, err := eventRepo.Create(
			ctx,
			persistence.CreateOperationEventInput{
				OperationID:    operation.ID,
				EventType:      "duplicate",
				SequenceNumber: operationEventInt32(1),
			},
		)
		if !errors.Is(
			err, persistence.ErrOperationEventConflict,
		) {
			t.Fatalf("expected sequence conflict, got %v", err)
		}
	})

	t.Run("NullableSequence", func(t *testing.T) {
		for i := 0; i < 2; i++ {
			event, err := eventRepo.Create(
				ctx,
				persistence.CreateOperationEventInput{
					OperationID: operation.ID,
					EventType:   "unsequenced",
				},
			)
			if err != nil {
				t.Fatalf("create unsequenced event: %v", err)
			}
			eventIDs = append(eventIDs, event.ID)

			if event.SequenceNumber != nil {
				t.Fatal("expected NULL sequence number")
			}

			var data map[string]any
			if err := json.Unmarshal(
				event.EventData, &data,
			); err != nil {
				t.Fatalf("decode default event data: %v", err)
			}
			if len(data) != 0 {
				t.Fatal("default event data should be empty object")
			}
		}
	})

	t.Run("ZeroSequence", func(t *testing.T) {
		event, err := eventRepo.Create(
			ctx,
			persistence.CreateOperationEventInput{
				OperationID:    operation.ID,
				EventType:      "created",
				SequenceNumber: operationEventInt32(0),
			},
		)
		if err != nil {
			t.Fatalf("create zero-sequence event: %v", err)
		}
		eventIDs = append(eventIDs, event.ID)

		if event.SequenceNumber == nil ||
			*event.SequenceNumber != 0 {
			t.Fatal("zero sequence was not preserved")
		}
	})

	t.Run("LongMessage", func(t *testing.T) {
		message := strings.Repeat("event message ", 1000)

		event, err := eventRepo.Create(
			ctx,
			persistence.CreateOperationEventInput{
				OperationID: operation.ID,
				EventType:   "diagnostic",
				Message:     &message,
			},
		)
		if err != nil {
			t.Fatalf("create long-message event: %v", err)
		}
		eventIDs = append(eventIDs, event.ID)

		if event.Message == nil ||
			*event.Message != message {
			t.Fatal("long TEXT message was not preserved")
		}
	})

	t.Run("ListByOperation", func(t *testing.T) {
		events, err := eventRepo.List(
			ctx,
			persistence.ListOperationEventsOptions{
				OperationID: operation.ID,
			},
		)
		if err != nil {
			t.Fatalf("List: %v", err)
		}

		if len(events) != 5 {
			t.Fatalf(
				"expected 5 events, got %d",
				len(events),
			)
		}

		for _, event := range events {
			if event.OperationID != operation.ID {
				t.Fatal("list returned unrelated operation")
			}
		}
	})

	t.Run("ListByEventType", func(t *testing.T) {
		events, err := eventRepo.List(
			ctx,
			persistence.ListOperationEventsOptions{
				OperationID: operation.ID,
				EventType:   "unsequenced",
			},
		)
		if err != nil {
			t.Fatalf("filtered List: %v", err)
		}

		if len(events) != 2 {
			t.Fatalf(
				"expected 2 events, got %d",
				len(events),
			)
		}
	})

	t.Run("ListByStatus", func(t *testing.T) {
		events, err := eventRepo.List(
			ctx,
			persistence.ListOperationEventsOptions{
				OperationID: operation.ID,
				Status:      operationEventString("running"),
			},
		)
		if err != nil {
			t.Fatalf("status List: %v", err)
		}

		if len(events) != 1 {
			t.Fatalf(
				"expected 1 running event, got %d",
				len(events),
			)
		}
	})

	t.Run("Pagination", func(t *testing.T) {
		events, err := eventRepo.List(
			ctx,
			persistence.ListOperationEventsOptions{
				OperationID: operation.ID,
				Limit:       2,
				Offset:      1,
			},
		)
		if err != nil {
			t.Fatalf("paginated List: %v", err)
		}

		if len(events) != 2 {
			t.Fatalf(
				"expected 2 events, got %d",
				len(events),
			)
		}
	})

	t.Run("InvalidForeignKey", func(t *testing.T) {
		missingID := "22222222-2222-4222-8222-222222222222"

		_, err := eventRepo.Create(
			ctx,
			persistence.CreateOperationEventInput{
				OperationID: missingID,
				EventType:   "invalid.parent",
			},
		)
		if err == nil {
			t.Fatal("expected foreign-key error")
		}
	})

	t.Run("GetMissing", func(t *testing.T) {
		_, err := eventRepo.GetByID(
			ctx, "22222222-2222-4222-8222-222222222222",
		)
		if !errors.Is(
			err, persistence.ErrOperationEventNotFound,
		) {
			t.Fatalf("expected not found, got %v", err)
		}
	})

	t.Run("GetMissingSequence", func(t *testing.T) {
		_, err := eventRepo.GetBySequence(
			ctx, operation.ID, 999,
		)
		if !errors.Is(
			err, persistence.ErrOperationEventNotFound,
		) {
			t.Fatalf("expected not found, got %v", err)
		}
	})

	t.Run("Delete", func(t *testing.T) {
		event, err := eventRepo.GetBySequence(
			ctx, operation.ID, 0,
		)
		if err != nil {
			t.Fatalf("find event: %v", err)
		}

		if err := eventRepo.Delete(ctx, event.ID); err != nil {
			t.Fatalf("delete event: %v", err)
		}

		_, err = eventRepo.GetByID(ctx, event.ID)
		if !errors.Is(
			err, persistence.ErrOperationEventNotFound,
		) {
			t.Fatalf("expected not found, got %v", err)
		}
	})

	t.Run("DeleteMissing", func(t *testing.T) {
		err := eventRepo.Delete(
			ctx, "22222222-2222-4222-8222-222222222222",
		)
		if !errors.Is(
			err, persistence.ErrOperationEventNotFound,
		) {
			t.Fatalf("expected not found, got %v", err)
		}
	})

	t.Run("CascadeDelete", func(t *testing.T) {
		parent, err := operationRepo.Create(
			ctx,
			persistence.CreateProvisioningOperationInput{
				OperationType: "stop",
				Provider:      "digitalocean",
			},
		)
		if err != nil {
			t.Fatalf("create cascade parent: %v", err)
		}
		operationIDs = append(operationIDs, parent.ID)

		child, err := eventRepo.Create(
			ctx,
			persistence.CreateOperationEventInput{
				OperationID: parent.ID,
				EventType:   "cascade.test",
			},
		)
		if err != nil {
			t.Fatalf("create cascade child: %v", err)
		}
		eventIDs = append(eventIDs, child.ID)

		if err := operationRepo.Delete(ctx, parent.ID); err != nil {
			t.Fatalf("delete parent operation: %v", err)
		}

		_, err = eventRepo.GetByID(ctx, child.ID)
		if !errors.Is(
			err, persistence.ErrOperationEventNotFound,
		) {
			t.Fatalf(
				"expected cascading event deletion, got %v",
				err,
			)
		}
	})
}
