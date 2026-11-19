package persistence_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/gdiop88/RL_trader_infrastructure/internal/config"
	"github.com/gdiop88/RL_trader_infrastructure/internal/database"
	"github.com/gdiop88/RL_trader_infrastructure/internal/persistence"
)

func TestCloudProviderCRUD(t *testing.T) {
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
	// Close the database after all record cleanup callbacks.
	t.Cleanup(func() {
		db.Close()
	})

	ctx, cancel := context.WithTimeout(
		context.Background(), 90*time.Second,
	)
	defer cancel()

	repo, err := persistence.NewProviderRepository(db)
	if err != nil {
		t.Fatalf("initialize provider repository: %v", err)
	}

	unique := fmt.Sprintf("%d", time.Now().UnixNano())
	code := "integration_test_" + unique
	endpoint := "https://api.example.com/v1"

	provider, err := repo.Create(
		ctx,
		persistence.CreateProviderInput{
			Code:         code,
			Name:         "Integration Test Provider",
			ProviderType: "cloud",
			APIEndpoint:  &endpoint,
		},
	)
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}

	// Remove only the provider created by this test.
	// The cleanup remains safe if the Delete subtest succeeds.
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(), 15*time.Second,
		)
		defer cleanupCancel()

		_, cleanupErr := db.Pool().Exec(
			cleanupCtx,
			`DELETE FROM cloud_providers
			 WHERE id = $1::uuid AND code = $2`,
			provider.ID,
			code,
		)
		if cleanupErr != nil {
			t.Errorf("cleanup provider: %v", cleanupErr)
		}
	})

	if provider.ID == "" {
		t.Fatal("created provider has no UUID")
	}
	if provider.Code != code {
		t.Fatalf("expected code %q, got %q", code, provider.Code)
	}
	if !provider.Enabled {
		t.Fatal("new provider should be enabled by default")
	}
	if provider.APIEndpoint == nil ||
		*provider.APIEndpoint != endpoint {
		t.Fatal("API endpoint was not stored")
	}

	t.Logf("Created test provider: %s", provider.ID)

	t.Run("GetByID", func(t *testing.T) {
		found, err := repo.GetByID(ctx, provider.ID)
		if err != nil {
			t.Fatalf("get provider by ID: %v", err)
		}
		if found.ID != provider.ID {
			t.Fatalf("unexpected ID: %s", found.ID)
		}
		if found.Code != code {
			t.Fatalf("unexpected code: %s", found.Code)
		}
	})

	t.Run("GetByCode", func(t *testing.T) {
		found, err := repo.GetByCode(ctx, code)
		if err != nil {
			t.Fatalf("get provider by code: %v", err)
		}
		if found.ID != provider.ID {
			t.Fatalf("unexpected provider ID: %s", found.ID)
		}
	})

	t.Run("List", func(t *testing.T) {
		items, err := repo.List(
			ctx,
			persistence.ListProvidersOptions{
				Limit: 1000,
			},
		)
		if err != nil {
			t.Fatalf("list providers: %v", err)
		}

		found := false
		for _, item := range items {
			if item.ID == provider.ID {
				found = true
				break
			}
		}
		if !found {
			t.Fatal("created provider missing from list")
		}
	})

	t.Run("DuplicateCode", func(t *testing.T) {
		_, err := repo.Create(
			ctx,
			persistence.CreateProviderInput{
				Code:         code,
				Name:         "Duplicate Provider",
				ProviderType: "cloud",
			},
		)
		if !errors.Is(err, persistence.ErrProviderConflict) {
			t.Fatalf(
				"expected ErrProviderConflict, got %v",
				err,
			)
		}
	})

	t.Run("Update", func(t *testing.T) {
		newEndpoint := "https://api.example.com/v2"

		updated, err := repo.Update(
			ctx,
			provider.ID,
			persistence.UpdateProviderInput{
				Name:         "Updated Test Provider",
				ProviderType: "cloud",
				APIEndpoint:  &newEndpoint,
			},
		)
		if err != nil {
			t.Fatalf("update provider: %v", err)
		}
		if updated.Name != "Updated Test Provider" {
			t.Fatalf("unexpected name: %s", updated.Name)
		}
		if updated.APIEndpoint == nil ||
			*updated.APIEndpoint != newEndpoint {
			t.Fatal("API endpoint was not updated")
		}
		if updated.Code != code {
			t.Fatal("provider code changed unexpectedly")
		}
	})

	t.Run("Disable", func(t *testing.T) {
		updated, err := repo.SetEnabled(
			ctx, provider.ID, false,
		)
		if err != nil {
			t.Fatalf("disable provider: %v", err)
		}
		if updated.Enabled {
			t.Fatal("provider should be disabled")
		}
	})

	t.Run("ListDisabled", func(t *testing.T) {
		disabled := false
		items, err := repo.List(
			ctx,
			persistence.ListProvidersOptions{
				Enabled: &disabled,
				Limit:   1000,
			},
		)
		if err != nil {
			t.Fatalf("list disabled providers: %v", err)
		}

		found := false
		for _, item := range items {
			if item.ID == provider.ID {
				found = true
				break
			}
		}
		if !found {
			t.Fatal("disabled provider missing from filtered list")
		}

		enabled := true
		enabledItems, err := repo.List(
			ctx,
			persistence.ListProvidersOptions{
				Enabled: &enabled,
				Limit:   1000,
			},
		)
		if err != nil {
			t.Fatalf("list enabled providers: %v", err)
		}
		for _, item := range enabledItems {
			if item.ID == provider.ID {
				t.Fatal("disabled provider appears in enabled list")
			}
		}
	})

	t.Run("Enable", func(t *testing.T) {
		updated, err := repo.SetEnabled(
			ctx, provider.ID, true,
		)
		if err != nil {
			t.Fatalf("enable provider: %v", err)
		}
		if !updated.Enabled {
			t.Fatal("provider should be enabled")
		}
	})

	t.Run("Delete", func(t *testing.T) {
		if err := repo.Delete(ctx, provider.ID); err != nil {
			t.Fatalf("delete provider: %v", err)
		}

		_, err := repo.GetByID(ctx, provider.ID)
		if !errors.Is(err, persistence.ErrProviderNotFound) {
			t.Fatalf(
				"expected ErrProviderNotFound, got %v",
				err,
			)
		}
	})

	t.Run("DeleteMissing", func(t *testing.T) {
		err := repo.Delete(ctx, provider.ID)
		if !errors.Is(err, persistence.ErrProviderNotFound) {
			t.Fatalf(
				"expected ErrProviderNotFound, got %v",
				err,
			)
		}
	})
}
