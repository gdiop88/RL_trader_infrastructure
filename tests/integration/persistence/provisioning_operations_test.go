package persistence_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/gdiop88/RL_trader_infrastructure/internal/config"
	"github.com/gdiop88/RL_trader_infrastructure/internal/database"
	"github.com/gdiop88/RL_trader_infrastructure/internal/persistence"
)

func operationIntegrationString(s string) *string {
	return &s
}

func operationIntegrationInt32(n int32) *int32 {
	return &n
}

func TestProvisioningOperationCRUD(t *testing.T) {
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

	repo, err := persistence.NewProvisioningOperationRepository(db)
	if err != nil {
		t.Fatalf("operation repository: %v", err)
	}

	projectRepo, err := persistence.NewInfrastructureProjectRepository(db)
	if err != nil {
		t.Fatalf("project repository: %v", err)
	}

	templateRepo, err := persistence.NewProvisioningTemplateRepository(db)
	if err != nil {
		t.Fatalf("template repository: %v", err)
	}

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	key := "operation_test_" + suffix

	var operationIDs []string
	var templateIDs []string
	var projectIDs []string

	t.Cleanup(func() {
		cleanupCtx, done := context.WithTimeout(
			context.Background(), 30*time.Second,
		)
		defer done()

		for _, id := range operationIDs {
			if _, err := db.Pool().Exec(
				cleanupCtx,
				`DELETE FROM provisioning_operations WHERE id = $1`,
				id,
			); err != nil {
				t.Errorf("cleanup operation %s: %v", id, err)
			}
		}

		for _, id := range templateIDs {
			if _, err := db.Pool().Exec(
				cleanupCtx,
				`DELETE FROM provisioning_templates WHERE id = $1`,
				id,
			); err != nil {
				t.Errorf("cleanup template %s: %v", id, err)
			}
		}

		for _, id := range projectIDs {
			if _, err := db.Pool().Exec(
				cleanupCtx,
				`DELETE FROM infrastructure_projects WHERE id = $1`,
				id,
			); err != nil {
				t.Errorf("cleanup project %s: %v", id, err)
			}
		}
	})

	project, err := projectRepo.Create(
		ctx,
		persistence.CreateInfrastructureProjectInput{
			Name:        "operation_project_" + suffix,
			Environment: "testing",
		},
	)
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	projectIDs = append(projectIDs, project.ID)

	template, err := templateRepo.Create(
		ctx,
		persistence.CreateProvisioningTemplateInput{
			ProjectID:     &project.ID,
			Name:          "operation_template_" + suffix,
			Configuration: json.RawMessage(`{"test":true}`),
		},
	)
	if err != nil {
		t.Fatalf("create template: %v", err)
	}
	templateIDs = append(templateIDs, template.ID)

	operation, err := repo.Create(
		ctx,
		persistence.CreateProvisioningOperationInput{
			OperationType: "create",
			Provider:      "digitalocean",
			Region:        operationIntegrationString("nyc3"),
			RequestedBy:   operationIntegrationString("integration-test"),
			RequestPayload: json.RawMessage(
				`{"size":"s-2vcpu-4gb","agents":10}`,
			),
			MaxRetries:     operationIntegrationInt32(2),
			IdempotencyKey: &key,
			ProjectID:      &project.ID,
			TemplateID:     &template.ID,
		},
	)
	if err != nil {
		t.Fatalf("create operation: %v", err)
	}
	operationIDs = append(operationIDs, operation.ID)

	if operation.ID == "" {
		t.Fatal("operation ID is empty")
	}
	if operation.Status != "pending" {
		t.Fatalf("expected pending, got %s", operation.Status)
	}
	if operation.RetryCount != 0 || operation.MaxRetries != 2 {
		t.Fatal("incorrect initial retry configuration")
	}
	if operation.CreatedAt.IsZero() || operation.UpdatedAt.IsZero() {
		t.Fatal("missing timestamps")
	}
	if operation.ProjectID == nil ||
		*operation.ProjectID != project.ID {
		t.Fatal("project association mismatch")
	}
	if operation.TemplateID == nil ||
		*operation.TemplateID != template.ID {
		t.Fatal("template association mismatch")
	}

	t.Run("GetByID", func(t *testing.T) {
		found, err := repo.GetByID(ctx, operation.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if found.ID != operation.ID {
			t.Fatal("operation ID mismatch")
		}
	})

	t.Run("GetByIdempotencyKey", func(t *testing.T) {
		found, err := repo.GetByIdempotencyKey(ctx, key)
		if err != nil {
			t.Fatalf("GetByIdempotencyKey: %v", err)
		}
		if found.ID != operation.ID {
			t.Fatal("idempotency lookup mismatch")
		}
	})

	t.Run("DuplicateIdempotencyKey", func(t *testing.T) {
		_, err := repo.Create(
			ctx,
			persistence.CreateProvisioningOperationInput{
				OperationType:  "create",
				Provider:       "digitalocean",
				IdempotencyKey: &key,
			},
		)
		if !errors.Is(
			err, persistence.ErrProvisioningOperationConflict,
		) {
			t.Fatalf("expected idempotency conflict, got %v", err)
		}
	})

	t.Run("RequestJSONB", func(t *testing.T) {
		found, err := repo.GetByID(ctx, operation.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}

		var payload map[string]any
		if err := json.Unmarshal(
			found.RequestPayload, &payload,
		); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		if payload["size"] != "s-2vcpu-4gb" {
			t.Fatal("request payload was not persisted")
		}
		if payload["agents"] != float64(10) {
			t.Fatal("agent count mismatch")
		}
	})

	t.Run("DefaultValues", func(t *testing.T) {
		created, err := repo.Create(
			ctx,
			persistence.CreateProvisioningOperationInput{
				OperationType: "stop",
				Provider:      "digitalocean",
			},
		)
		if err != nil {
			t.Fatalf("create minimal operation: %v", err)
		}
		operationIDs = append(operationIDs, created.ID)

		if created.Status != "pending" {
			t.Fatal("default status should be pending")
		}
		if created.RetryCount != 0 || created.MaxRetries != 3 {
			t.Fatal("incorrect default retry values")
		}

		var payload map[string]any
		if err := json.Unmarshal(
			created.RequestPayload, &payload,
		); err != nil {
			t.Fatalf("decode default payload: %v", err)
		}
		if len(payload) != 0 {
			t.Fatal("default request payload should be empty object")
		}
	})

	t.Run("ListByProject", func(t *testing.T) {
		items, err := repo.List(
			ctx,
			persistence.ListProvisioningOperationsOptions{
				ProjectID: &project.ID,
			},
		)
		if err != nil {
			t.Fatalf("List by project: %v", err)
		}

		found := false
		for _, item := range items {
			if item.ProjectID == nil ||
				*item.ProjectID != project.ID {
				t.Fatal("incorrect project in results")
			}
			if item.ID == operation.ID {
				found = true
			}
		}
		if !found {
			t.Fatal("operation missing from project list")
		}
	})

	t.Run("ListByTemplate", func(t *testing.T) {
		items, err := repo.List(
			ctx,
			persistence.ListProvisioningOperationsOptions{
				TemplateID: &template.ID,
			},
		)
		if err != nil {
			t.Fatalf("List by template: %v", err)
		}

		found := false
		for _, item := range items {
			if item.ID == operation.ID {
				found = true
			}
		}
		if !found {
			t.Fatal("operation missing from template list")
		}
	})

	t.Run("ListByStatusAndProvider", func(t *testing.T) {
		items, err := repo.List(
			ctx,
			persistence.ListProvisioningOperationsOptions{
				ProjectID:     &project.ID,
				Status:        "pending",
				Provider:      "digitalocean",
				OperationType: "create",
			},
		)
		if err != nil {
			t.Fatalf("filtered List: %v", err)
		}

		if len(items) != 1 || items[0].ID != operation.ID {
			t.Fatal("filtered listing mismatch")
		}
	})

	var startedAt time.Time

	t.Run("StartOperation", func(t *testing.T) {
		startedAt = time.Now().UTC()

		updated, err := repo.UpdateStatus(
			ctx,
			operation.ID,
			persistence.UpdateProvisioningOperationStatusInput{
				Status:    "running",
				StartedAt: &startedAt,
			},
		)
		if err != nil {
			t.Fatalf("start operation: %v", err)
		}

		if updated.Status != "running" {
			t.Fatal("operation not running")
		}
		if updated.StartedAt == nil {
			t.Fatal("started_at was not persisted")
		}
	})

	t.Run("IncrementRetry", func(t *testing.T) {
		next := time.Now().UTC().Add(time.Minute)

		updated, err := repo.IncrementRetry(
			ctx, operation.ID, &next,
		)
		if err != nil {
			t.Fatalf("increment retry: %v", err)
		}
		if updated.RetryCount != 1 {
			t.Fatalf("expected retry count 1, got %d", updated.RetryCount)
		}
		if updated.NextRetryAt == nil {
			t.Fatal("next_retry_at missing")
		}

		updated, err = repo.IncrementRetry(
			ctx, operation.ID, &next,
		)
		if err != nil {
			t.Fatalf("second retry: %v", err)
		}
		if updated.RetryCount != 2 {
			t.Fatalf("expected retry count 2, got %d", updated.RetryCount)
		}
	})

	t.Run("RetryLimit", func(t *testing.T) {
		_, err := repo.IncrementRetry(ctx, operation.ID, nil)
		if !errors.Is(
			err, persistence.ErrProvisioningOperationConflict,
		) {
			t.Fatalf("expected retry limit conflict, got %v", err)
		}

		found, err := repo.GetByID(ctx, operation.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if found.RetryCount != 2 {
			t.Fatal("retry count exceeded maximum")
		}
	})

	t.Run("CompleteOperation", func(t *testing.T) {
		completedAt := time.Now().UTC().Add(time.Second)

		updated, err := repo.UpdateStatus(
			ctx,
			operation.ID,
			persistence.UpdateProvisioningOperationStatusInput{
				Status: "completed",
				ResponsePayload: json.RawMessage(
					`{"provider_instance_id":"droplet-123"}`,
				),
				CompletedAt: &completedAt,
			},
		)
		if err != nil {
			t.Fatalf("complete operation: %v", err)
		}

		if updated.Status != "completed" {
			t.Fatal("operation not completed")
		}
		if updated.CompletedAt == nil {
			t.Fatal("completed_at missing")
		}
		if updated.StartedAt == nil {
			t.Fatal("started_at was not preserved")
		}
		if updated.CompletedAt.Before(*updated.StartedAt) {
			t.Fatal("completion precedes start")
		}

		var response map[string]any
		if err := json.Unmarshal(
			updated.ResponsePayload, &response,
		); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if response["provider_instance_id"] != "droplet-123" {
			t.Fatal("response payload mismatch")
		}
	})

	t.Run("RetryCompletedOperation", func(t *testing.T) {
		_, err := repo.IncrementRetry(ctx, operation.ID, nil)
		if !errors.Is(
			err, persistence.ErrProvisioningOperationConflict,
		) {
			t.Fatalf("expected conflict, got %v", err)
		}
	})

	t.Run("InvalidForeignKey", func(t *testing.T) {
		missingProject := "22222222-2222-4222-8222-222222222222"

		_, err := repo.Create(
			ctx,
			persistence.CreateProvisioningOperationInput{
				OperationType: "create",
				Provider:      "digitalocean",
				ProjectID:     &missingProject,
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
			err, persistence.ErrProvisioningOperationNotFound,
		) {
			t.Fatalf("expected not found, got %v", err)
		}
	})

	t.Run("Delete", func(t *testing.T) {
		if err := repo.Delete(ctx, operation.ID); err != nil {
			t.Fatalf("delete operation: %v", err)
		}

		_, err := repo.GetByID(ctx, operation.ID)
		if !errors.Is(
			err, persistence.ErrProvisioningOperationNotFound,
		) {
			t.Fatalf("expected not found, got %v", err)
		}
	})

	t.Run("DeleteMissing", func(t *testing.T) {
		err := repo.Delete(ctx, operation.ID)
		if !errors.Is(
			err, persistence.ErrProvisioningOperationNotFound,
		) {
			t.Fatalf("expected not found, got %v", err)
		}
	})
}
