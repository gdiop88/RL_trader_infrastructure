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

func ptr(value string) *string {
	return &value
}

func TestInfrastructureInstanceCRUD(t *testing.T) {
	cfg, err := config.Load("../../../configs/server.yaml")
	if err != nil {
		t.Fatalf("load configuration: %v", err)
	}

	dbCtx, dbCancel := context.WithTimeout(
		context.Background(), 15*time.Second,
	)
	defer dbCancel()

	db, err := database.ConnectWithConfig(dbCtx, cfg)
	if err != nil {
		t.Fatalf("connect to Aiven PostgreSQL: %v", err)
	}

	// Cleanup callbacks execute in reverse registration order.
	// Register database closure first so record deletion runs first.
	t.Cleanup(func() {
		db.Close()
	})

	ctx, cancel := context.WithTimeout(
		context.Background(), 60*time.Second,
	)
	defer cancel()

	repo, err := persistence.NewInstanceRepository(db)
	if err != nil {
		t.Fatalf("initialize repository: %v", err)
	}

	unique := fmt.Sprintf("%d", time.Now().UnixNano())
	name := "integration-test-" + unique
	providerInstanceID := "test-provider-" + unique

	instance, err := repo.Create(
		ctx,
		persistence.CreateInstanceInput{
			Provider:     "integration_test",
			Name:         name,
			Region:       ptr("test-region"),
			InstanceType: ptr("test-small"),
			Metadata: json.RawMessage(
				`{"environment":"integration"}`,
			),
			Tags: json.RawMessage(
				`["integration","temporary"]`,
			),
		},
	)
	if err != nil {
		t.Fatalf("create instance: %v", err)
	}

	// Delete the exact record created by this test.
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(), 15*time.Second,
		)
		defer cleanupCancel()

		tag, cleanupErr := db.Pool().Exec(
			cleanupCtx,
			`DELETE FROM infrastructure_instances
			 WHERE id = $1::uuid
			   AND provider = 'integration_test'
			   AND name = $2`,
			instance.ID,
			name,
		)
		if cleanupErr != nil {
			t.Errorf("cleanup instance: %v", cleanupErr)
			return
		}
		if tag.RowsAffected() != 1 {
			t.Errorf(
				"cleanup expected 1 deleted row, got %d",
				tag.RowsAffected(),
			)
		}
	})

	if instance.ID == "" {
		t.Fatal("created instance has no UUID")
	}
	if instance.Status != persistence.InstancePending {
		t.Fatalf("expected pending, got %s", instance.Status)
	}
	if instance.Name != name {
		t.Fatalf("expected name %q, got %q", name, instance.Name)
	}

	t.Logf("Created test instance: %s", instance.ID)

	t.Run("GetByID", func(t *testing.T) {
		found, err := repo.GetByID(ctx, instance.ID)
		if err != nil {
			t.Fatalf("get instance: %v", err)
		}
		if found.ID != instance.ID {
			t.Fatalf("unexpected ID: %s", found.ID)
		}
		if found.Provider != "integration_test" {
			t.Fatalf("unexpected provider: %s", found.Provider)
		}
	})

	t.Run("List", func(t *testing.T) {
		items, err := repo.List(
			ctx,
			persistence.ListInstancesOptions{
				Provider: "integration_test",
				Status:   persistence.InstancePending,
				Limit:    100,
			},
		)
		if err != nil {
			t.Fatalf("list instances: %v", err)
		}

		found := false
		for _, item := range items {
			if item.ID == instance.ID {
				found = true
				break
			}
		}
		if !found {
			t.Fatal("created instance missing from list")
		}
	})

	t.Run("UpdateStatus", func(t *testing.T) {
		updated, err := repo.UpdateStatus(
			ctx, instance.ID, persistence.InstanceRunning,
		)
		if err != nil {
			t.Fatalf("update status: %v", err)
		}
		if updated.Status != persistence.InstanceRunning {
			t.Fatalf("expected running, got %s", updated.Status)
		}
		if updated.ProvisionedAt == nil {
			t.Fatal("provisioned_at was not set")
		}
	})

	t.Run("UpdateProviderDetails", func(t *testing.T) {
		updated, err := repo.UpdateProviderDetails(
			ctx,
			instance.ID,
			persistence.UpdateProviderDetailsInput{
				ProviderInstanceID: ptr(providerInstanceID),
				ProviderMetadataID: ptr("test-metadata-" + unique),
				Region:             ptr("test-region-2"),
				InstanceType:       ptr("test-medium"),
				PublicIP:           ptr("192.0.2.10"),
				PrivateIP:          ptr("10.0.0.10"),
				Metadata: json.RawMessage(
					`{"source":"provider"}`,
				),
				Tags: json.RawMessage(
					`["provider-synced"]`,
				),
			},
		)
		if err != nil {
			t.Fatalf("update provider details: %v", err)
		}
		if updated.ProviderInstanceID == nil ||
			*updated.ProviderInstanceID != providerInstanceID {
			t.Fatal("provider instance ID not updated")
		}
		if updated.PublicIP == nil ||
			*updated.PublicIP != "192.0.2.10" {
			t.Fatal("public IP not updated")
		}
		if updated.LastObservedAt == nil {
			t.Fatal("last_observed_at was not set")
		}
	})

	t.Run("UpdateMetadata", func(t *testing.T) {
		updated, err := repo.UpdateMetadata(
			ctx,
			instance.ID,
			persistence.UpdateInstanceMetadataInput{
				Metadata: json.RawMessage(
					`{"phase":"verified"}`,
				),
				Tags: json.RawMessage(
					`["crud-tested"]`,
				),
			},
		)
		if err != nil {
			t.Fatalf("update metadata: %v", err)
		}

		var metadata map[string]any
		if err := json.Unmarshal(
			updated.Metadata, &metadata,
		); err != nil {
			t.Fatalf("decode metadata: %v", err)
		}
		if metadata["phase"] != "verified" {
			t.Fatalf("unexpected metadata: %v", metadata)
		}
	})

	t.Run("SetDesiredState", func(t *testing.T) {
		updated, err := repo.SetDesiredState(
			ctx, instance.ID, persistence.InstanceStopped,
		)
		if err != nil {
			t.Fatalf("set desired state: %v", err)
		}
		if updated.DesiredState == nil ||
			*updated.DesiredState != "stopped" {
			t.Fatal("desired state was not updated")
		}
	})

	t.Run("SetLastError", func(t *testing.T) {
		updated, err := repo.SetLastError(
			ctx, instance.ID, "integration test error",
		)
		if err != nil {
			t.Fatalf("set last error: %v", err)
		}
		if updated.LastError == nil ||
			*updated.LastError != "integration test error" {
			t.Fatal("last error was not stored")
		}

		cleared, err := repo.SetLastError(
			ctx, instance.ID, "",
		)
		if err != nil {
			t.Fatalf("clear last error: %v", err)
		}
		if cleared.LastError != nil {
			t.Fatal("last error was not cleared")
		}
	})

	t.Run("SoftDelete", func(t *testing.T) {
		if err := repo.SoftDelete(ctx, instance.ID); err != nil {
			t.Fatalf("soft delete: %v", err)
		}

		_, err := repo.GetByID(ctx, instance.ID)
		if !errors.Is(err, persistence.ErrInstanceNotFound) {
			t.Fatalf("expected ErrInstanceNotFound, got %v", err)
		}

		var status string
		var deletedAt *time.Time

		err = db.Pool().QueryRow(
			ctx,
			`SELECT status, deleted_at
			 FROM infrastructure_instances
			 WHERE id = $1::uuid`,
			instance.ID,
		).Scan(&status, &deletedAt)
		if err != nil {
			t.Fatalf("verify soft delete: %v", err)
		}
		if status != "deleted" || deletedAt == nil {
			t.Fatalf("invalid soft delete state: %s", status)
		}

		items, err := repo.List(
			ctx,
			persistence.ListInstancesOptions{
				Provider: "integration_test",
			},
		)
		if err != nil {
			t.Fatalf("list after soft delete: %v", err)
		}
		for _, item := range items {
			if item.ID == instance.ID {
				t.Fatal("soft-deleted instance appears in list")
			}
		}
	})
}
