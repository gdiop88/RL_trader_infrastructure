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
	"github.com/jackc/pgx/v5/pgconn"
)

func TestProviderRegionCRUD(t *testing.T) {
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

	providerRepo, err := persistence.NewProviderRepository(db)
	if err != nil {
		t.Fatalf("provider repository: %v", err)
	}

	regionRepo, err := persistence.NewProviderRegionRepository(db)
	if err != nil {
		t.Fatalf("region repository: %v", err)
	}

	unique := fmt.Sprintf("%d", time.Now().UnixNano())

	// Create two independent cloud providers.
	providerA, err := providerRepo.Create(
		ctx,
		persistence.CreateProviderInput{
			Code:         "region_test_a_" + unique,
			Name:         "Region Test Provider A",
			ProviderType: "cloud",
		},
	)
	if err != nil {
		t.Fatalf("create provider A: %v", err)
	}

	providerB, err := providerRepo.Create(
		ctx,
		persistence.CreateProviderInput{
			Code:         "region_test_b_" + unique,
			Name:         "Region Test Provider B",
			ProviderType: "cloud",
		},
	)
	if err != nil {
		cleanupCtx, done := context.WithTimeout(
			context.Background(), 15*time.Second,
		)
		defer done()

		if cleanupErr := providerRepo.Delete(
			cleanupCtx, providerA.ID,
		); cleanupErr != nil {
			t.Errorf("cleanup provider A: %v", cleanupErr)
		}
		t.Fatalf("create provider B: %v", err)
	}

	// Cleanup must delete regions before providers.
	t.Cleanup(func() {
		cleanupCtx, done := context.WithTimeout(
			context.Background(), 30*time.Second,
		)
		defer done()

		for _, providerID := range []string{
			providerA.ID,
			providerB.ID,
		} {
			_, err := db.Pool().Exec(
				cleanupCtx,
				`DELETE FROM provider_regions
				 WHERE provider_id = $1::uuid`,
				providerID,
			)
			if err != nil {
				t.Errorf("cleanup regions: %v", err)
			}

			if err := providerRepo.Delete(
				cleanupCtx, providerID,
			); err != nil {
				t.Errorf("cleanup provider %s: %v", providerID, err)
			}
		}
	})

	displayName := "New York 3"
	regionCode := "nyc3"

	region, err := regionRepo.Create(
		ctx,
		persistence.CreateProviderRegionInput{
			ProviderID:   providerA.ID,
			Code:         regionCode,
			DisplayName:  &displayName,
			Capabilities: json.RawMessage(`{"compute":true,"gpu":false}`),
		},
	)
	if err != nil {
		t.Fatalf("create region: %v", err)
	}

	if region.ID == "" {
		t.Fatal("region ID is empty")
	}
	if region.ProviderID != providerA.ID {
		t.Fatal("provider ID mismatch")
	}
	if !region.Enabled {
		t.Fatal("new region should be enabled")
	}

	t.Logf("Created provider A: %s", providerA.ID)
	t.Logf("Created provider B: %s", providerB.ID)
	t.Logf("Created region: %s", region.ID)

	t.Run("GetByID", func(t *testing.T) {
		found, err := regionRepo.GetByID(ctx, region.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if found.ID != region.ID {
			t.Fatal("retrieved region ID mismatch")
		}
	})

	t.Run("GetByCode", func(t *testing.T) {
		found, err := regionRepo.GetByCode(
			ctx, providerA.ID, regionCode,
		)
		if err != nil {
			t.Fatalf("GetByCode: %v", err)
		}
		if found.ID != region.ID {
			t.Fatal("retrieved region mismatch")
		}
	})

	t.Run("ListByProvider", func(t *testing.T) {
		items, err := regionRepo.List(
			ctx,
			persistence.ListProviderRegionsOptions{
				ProviderID: providerA.ID,
				Limit:      100,
			},
		)
		if err != nil {
			t.Fatalf("List: %v", err)
		}

		found := false
		for _, item := range items {
			if item.ID == region.ID {
				found = true
			}
			if item.ProviderID != providerA.ID {
				t.Fatal("list returned another provider's region")
			}
		}
		if !found {
			t.Fatal("region missing from list")
		}
	})

	t.Run("DuplicateCodeSameProvider", func(t *testing.T) {
		_, err := regionRepo.Create(
			ctx,
			persistence.CreateProviderRegionInput{
				ProviderID: providerA.ID,
				Code:       regionCode,
			},
		)
		if !errors.Is(
			err, persistence.ErrProviderRegionConflict,
		) {
			t.Fatalf("expected conflict, got %v", err)
		}
	})

	t.Run("SameCodeDifferentProvider", func(t *testing.T) {
		second, err := regionRepo.Create(
			ctx,
			persistence.CreateProviderRegionInput{
				ProviderID: providerB.ID,
				Code:       regionCode,
			},
		)
		if err != nil {
			t.Fatalf("same code across providers: %v", err)
		}

		if second.ID == region.ID {
			t.Fatal("regions should have different IDs")
		}
		if second.ProviderID != providerB.ID {
			t.Fatal("second region provider mismatch")
		}
	})

	t.Run("ForeignKeyProtection", func(t *testing.T) {
		err := providerRepo.Delete(ctx, providerA.ID)
		if err == nil {
			t.Fatal("expected provider deletion to fail")
		}

		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) ||
			pgErr.Code != "23503" {
			t.Fatalf("expected SQLSTATE 23503, got %v", err)
		}
	})

	t.Run("InvalidProviderReference", func(t *testing.T) {
		_, err := regionRepo.Create(
			ctx,
			persistence.CreateProviderRegionInput{
				ProviderID: "11111111-1111-4111-8111-111111111111",
				Code:       "invalid-provider",
			},
		)
		if err == nil {
			t.Fatal("expected foreign-key failure")
		}

		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) ||
			pgErr.Code != "23503" {
			t.Fatalf("expected SQLSTATE 23503, got %v", err)
		}
	})

	t.Run("Update", func(t *testing.T) {
		newName := "New York Updated"

		updated, err := regionRepo.Update(
			ctx,
			region.ID,
			persistence.UpdateProviderRegionInput{
				DisplayName: &newName,
				Capabilities: json.RawMessage(
					`{"compute":true,"gpu":true}`,
				),
			},
		)
		if err != nil {
			t.Fatalf("Update: %v", err)
		}

		if updated.DisplayName == nil ||
			*updated.DisplayName != newName {
			t.Fatal("display name not updated")
		}

		var capabilities map[string]any
		if err := json.Unmarshal(
			updated.Capabilities, &capabilities,
		); err != nil {
			t.Fatalf("decode capabilities: %v", err)
		}
		if capabilities["gpu"] != true {
			t.Fatal("GPU capability not updated")
		}
		if updated.Code != regionCode {
			t.Fatal("region code changed unexpectedly")
		}
	})

	t.Run("Disable", func(t *testing.T) {
		updated, err := regionRepo.SetEnabled(
			ctx, region.ID, false,
		)
		if err != nil {
			t.Fatalf("Disable: %v", err)
		}
		if updated.Enabled {
			t.Fatal("region should be disabled")
		}
	})

	t.Run("ListDisabled", func(t *testing.T) {
		disabled := false

		items, err := regionRepo.List(
			ctx,
			persistence.ListProviderRegionsOptions{
				ProviderID: providerA.ID,
				Enabled:    &disabled,
			},
		)
		if err != nil {
			t.Fatalf("list disabled: %v", err)
		}

		found := false
		for _, item := range items {
			if item.ID == region.ID {
				found = true
			}
		}
		if !found {
			t.Fatal("disabled region missing")
		}

		enabled := true
		items, err = regionRepo.List(
			ctx,
			persistence.ListProviderRegionsOptions{
				ProviderID: providerA.ID,
				Enabled:    &enabled,
			},
		)
		if err != nil {
			t.Fatalf("list enabled: %v", err)
		}
		for _, item := range items {
			if item.ID == region.ID {
				t.Fatal("disabled region in enabled list")
			}
		}
	})

	t.Run("Enable", func(t *testing.T) {
		updated, err := regionRepo.SetEnabled(
			ctx, region.ID, true,
		)
		if err != nil {
			t.Fatalf("Enable: %v", err)
		}
		if !updated.Enabled {
			t.Fatal("region should be enabled")
		}
	})

	t.Run("Delete", func(t *testing.T) {
		if err := regionRepo.Delete(
			ctx, region.ID,
		); err != nil {
			t.Fatalf("Delete: %v", err)
		}

		_, err := regionRepo.GetByID(ctx, region.ID)
		if !errors.Is(
			err, persistence.ErrProviderRegionNotFound,
		) {
			t.Fatalf("expected not found, got %v", err)
		}
	})

	t.Run("DeleteMissing", func(t *testing.T) {
		err := regionRepo.Delete(ctx, region.ID)
		if !errors.Is(
			err, persistence.ErrProviderRegionNotFound,
		) {
			t.Fatalf("expected not found, got %v", err)
		}
	})
}
