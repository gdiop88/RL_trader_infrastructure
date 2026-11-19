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

func operationAttemptString(value string) *string {
	return &value
}

func operationAttemptTime(value time.Time) *time.Time {
	return &value
}

func TestOperationAttemptCRUD(t *testing.T) {
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

	repo, err := persistence.NewOperationAttemptRepository(db)
	if err != nil {
		t.Fatalf("create attempt repository: %v", err)
	}

	operationRepo, err := persistence.NewProvisioningOperationRepository(db)
	if err != nil {
		t.Fatalf("create operation repository: %v", err)
	}

	var operationIDs []string

	t.Cleanup(func() {
		cleanupCtx, done := context.WithTimeout(
			context.Background(), 30*time.Second,
		)
		defer done()

		for _, id := range operationIDs {
			// ON DELETE CASCADE removes all associated attempts.
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
			IdempotencyKey: operationAttemptString(
				"attempt_integration_" + suffix,
			),
		},
	)
	if err != nil {
		t.Fatalf("create parent operation: %v", err)
	}
	operationIDs = append(operationIDs, operation.ID)

	started := time.Now().UTC().Truncate(time.Microsecond)

	first, err := repo.Create(
		ctx,
		persistence.CreateOperationAttemptInput{
			OperationID:     operation.ID,
			AttemptNumber:   1,
			Status:          "running",
			WorkerReference: operationAttemptString("worker-001"),
			StartedAt:       &started,
			ResponseMetadata: json.RawMessage(
				`{"provider":"digitalocean","retry":false}`,
			),
		},
	)
	if err != nil {
		t.Fatalf("create first attempt: %v", err)
	}

	t.Run("Create", func(t *testing.T) {
		if first.ID == "" {
			t.Fatal("attempt ID is empty")
		}
		if first.OperationID != operation.ID {
			t.Fatal("operation ID mismatch")
		}
		if first.AttemptNumber != 1 {
			t.Fatal("attempt number mismatch")
		}
		if first.Status != "running" {
			t.Fatal("status mismatch")
		}
		if first.WorkerReference == nil ||
			*first.WorkerReference != "worker-001" {
			t.Fatal("worker reference mismatch")
		}
		if first.StartedAt == nil {
			t.Fatal("started_at was not persisted")
		}
	})

	t.Run("GetByID", func(t *testing.T) {
		found, err := repo.GetByID(ctx, first.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if found.ID != first.ID {
			t.Fatal("incorrect attempt returned")
		}
	})

	t.Run("GetByAttemptNumber", func(t *testing.T) {
		found, err := repo.GetByAttemptNumber(
			ctx, operation.ID, 1,
		)
		if err != nil {
			t.Fatalf("GetByAttemptNumber: %v", err)
		}
		if found.ID != first.ID {
			t.Fatal("incorrect attempt number lookup")
		}
	})

	t.Run("JSONBPersistence", func(t *testing.T) {
		found, err := repo.GetByID(ctx, first.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}

		var metadata map[string]any
		if err := json.Unmarshal(
			found.ResponseMetadata, &metadata,
		); err != nil {
			t.Fatalf("decode metadata: %v", err)
		}
		if metadata["provider"] != "digitalocean" {
			t.Fatal("provider metadata mismatch")
		}
		if metadata["retry"] != false {
			t.Fatal("retry metadata mismatch")
		}
	})

	t.Run("DuplicateAttemptNumber", func(t *testing.T) {
		_, err := repo.Create(
			ctx,
			persistence.CreateOperationAttemptInput{
				OperationID:   operation.ID,
				AttemptNumber: 1,
				Status:        "pending",
			},
		)
		if !errors.Is(
			err, persistence.ErrOperationAttemptConflict,
		) {
			t.Fatalf("expected unique conflict, got %v", err)
		}
	})

	second, err := repo.Create(
		ctx,
		persistence.CreateOperationAttemptInput{
			OperationID:   operation.ID,
			AttemptNumber: 2,
			Status:        "pending",
		},
	)
	if err != nil {
		t.Fatalf("create second attempt: %v", err)
	}

	t.Run("NullableFields", func(t *testing.T) {
		if second.WorkerReference != nil ||
			second.StartedAt != nil ||
			second.FinishedAt != nil ||
			second.ErrorCode != nil ||
			second.ErrorMessage != nil {
			t.Fatal("expected nullable fields to be nil")
		}
		if len(second.ResponseMetadata) != 0 {
			t.Fatal("expected NULL response metadata")
		}
	})

	t.Run("Update", func(t *testing.T) {
		finished := started.Add(3 * time.Second)

		updated, err := repo.Update(
			ctx,
			first.ID,
			persistence.UpdateOperationAttemptInput{
				Status:          "failed",
				WorkerReference: operationAttemptString("worker-001"),
				StartedAt:       &started,
				FinishedAt:      &finished,
				ErrorCode:       operationAttemptString("PROVIDER_TIMEOUT"),
				ErrorMessage: operationAttemptString(
					"DigitalOcean API request timed out",
				),
				ResponseMetadata: json.RawMessage(
					`{"retryable":true,"http_status":504}`,
				),
			},
		)
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		if updated.Status != "failed" {
			t.Fatal("status was not updated")
		}
		if updated.FinishedAt == nil {
			t.Fatal("finished_at was not updated")
		}
		if updated.ErrorCode == nil ||
			*updated.ErrorCode != "PROVIDER_TIMEOUT" {
			t.Fatal("error code was not updated")
		}

		found, err := repo.GetByID(ctx, first.ID)
		if err != nil {
			t.Fatalf("reload updated attempt: %v", err)
		}
		if found.Status != "failed" {
			t.Fatal("updated status was not persisted")
		}
		if found.ErrorMessage == nil ||
			*found.ErrorMessage != "DigitalOcean API request timed out" {
			t.Fatal("error message was not persisted")
		}
	})

	t.Run("ListByOperation", func(t *testing.T) {
		attempts, err := repo.List(
			ctx,
			persistence.ListOperationAttemptsOptions{
				OperationID: operation.ID,
			},
		)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(attempts) != 2 {
			t.Fatalf("expected 2 attempts, got %d", len(attempts))
		}
		if attempts[0].AttemptNumber != 1 ||
			attempts[1].AttemptNumber != 2 {
			t.Fatal("attempts are not ordered by attempt number")
		}
	})

	t.Run("ListByStatus", func(t *testing.T) {
		attempts, err := repo.List(
			ctx,
			persistence.ListOperationAttemptsOptions{
				OperationID: operation.ID,
				Status:      "failed",
			},
		)
		if err != nil {
			t.Fatalf("List by status: %v", err)
		}
		if len(attempts) != 1 ||
			attempts[0].ID != first.ID {
			t.Fatal("incorrect status filter results")
		}
	})

	t.Run("Pagination", func(t *testing.T) {
		attempts, err := repo.List(
			ctx,
			persistence.ListOperationAttemptsOptions{
				OperationID: operation.ID,
				Limit:       1,
				Offset:      1,
			},
		)
		if err != nil {
			t.Fatalf("paginated List: %v", err)
		}
		if len(attempts) != 1 ||
			attempts[0].ID != second.ID {
			t.Fatal("incorrect pagination results")
		}
	})

	t.Run("CustomStatus", func(t *testing.T) {
		updated, err := repo.Update(
			ctx,
			second.ID,
			persistence.UpdateOperationAttemptInput{
				Status: "waiting_for_worker",
			},
		)
		if err != nil {
			t.Fatalf("custom status update: %v", err)
		}
		if updated.Status != "waiting_for_worker" {
			t.Fatal("custom status was not persisted")
		}
	})

	t.Run("JSONBArray", func(t *testing.T) {
		updated, err := repo.Update(
			ctx,
			second.ID,
			persistence.UpdateOperationAttemptInput{
				Status:           "completed",
				ResponseMetadata: json.RawMessage(`[1,2,3]`),
			},
		)
		if err != nil {
			t.Fatalf("JSONB array update: %v", err)
		}

		var values []int
		if err := json.Unmarshal(
			updated.ResponseMetadata, &values,
		); err != nil {
			t.Fatalf("decode JSONB array: %v", err)
		}
		if len(values) != 3 {
			t.Fatal("JSONB array was not persisted")
		}
	})

	t.Run("LongErrorMessage", func(t *testing.T) {
		message := strings.Repeat("failure detail ", 1000)

		updated, err := repo.Update(
			ctx,
			second.ID,
			persistence.UpdateOperationAttemptInput{
				Status:       "failed",
				ErrorMessage: &message,
			},
		)
		if err != nil {
			t.Fatalf("long message update: %v", err)
		}
		if updated.ErrorMessage == nil ||
			*updated.ErrorMessage != message {
			t.Fatal("long TEXT message was not preserved")
		}
	})

	t.Run("InvalidForeignKey", func(t *testing.T) {
		_, err := repo.Create(
			ctx,
			persistence.CreateOperationAttemptInput{
				OperationID:   "22222222-2222-4222-8222-222222222222",
				AttemptNumber: 1,
				Status:        "pending",
			},
		)
		if err == nil {
			t.Fatal("expected foreign-key error")
		}
	})

	t.Run("GetMissing", func(t *testing.T) {
		_, err := repo.GetByID(
			ctx, "22222222-2222-4222-8222-222222222222",
		)
		if !errors.Is(
			err, persistence.ErrOperationAttemptNotFound,
		) {
			t.Fatalf("expected not found, got %v", err)
		}
	})

	t.Run("GetMissingAttemptNumber", func(t *testing.T) {
		_, err := repo.GetByAttemptNumber(
			ctx, operation.ID, 999,
		)
		if !errors.Is(
			err, persistence.ErrOperationAttemptNotFound,
		) {
			t.Fatalf("expected not found, got %v", err)
		}
	})

	t.Run("UpdateMissing", func(t *testing.T) {
		_, err := repo.Update(
			ctx,
			"22222222-2222-4222-8222-222222222222",
			persistence.UpdateOperationAttemptInput{
				Status: "failed",
			},
		)
		if !errors.Is(
			err, persistence.ErrOperationAttemptNotFound,
		) {
			t.Fatalf("expected not found, got %v", err)
		}
	})

	t.Run("Delete", func(t *testing.T) {
		if err := repo.Delete(ctx, second.ID); err != nil {
			t.Fatalf("Delete: %v", err)
		}

		_, err := repo.GetByID(ctx, second.ID)
		if !errors.Is(
			err, persistence.ErrOperationAttemptNotFound,
		) {
			t.Fatalf("expected deleted attempt not found, got %v", err)
		}
	})

	t.Run("DeleteMissing", func(t *testing.T) {
		err := repo.Delete(
			ctx, "22222222-2222-4222-8222-222222222222",
		)
		if !errors.Is(
			err, persistence.ErrOperationAttemptNotFound,
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

		child, err := repo.Create(
			ctx,
			persistence.CreateOperationAttemptInput{
				OperationID:   parent.ID,
				AttemptNumber: 1,
				Status:        "running",
			},
		)
		if err != nil {
			t.Fatalf("create cascade child: %v", err)
		}

		if err := operationRepo.Delete(ctx, parent.ID); err != nil {
			t.Fatalf("delete parent: %v", err)
		}

		_, err = repo.GetByID(ctx, child.ID)
		if !errors.Is(
			err, persistence.ErrOperationAttemptNotFound,
		) {
			t.Fatalf("expected cascade deletion, got %v", err)
		}
	})
}
