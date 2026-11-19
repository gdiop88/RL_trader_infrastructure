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
	"github.com/jackc/pgx/v5/pgconn"
)

func profileInt32(value int32) *int32 {
	return &value
}

func profileString(value string) *string {
	return &value
}

func TestComputeProfileCRUD(t *testing.T) {
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

	profileRepo, err := persistence.NewComputeProfileRepository(db)
	if err != nil {
		t.Fatalf("compute profile repository: %v", err)
	}

	unique := fmt.Sprintf("%d", time.Now().UnixNano())

	providerA, err := providerRepo.Create(
		ctx,
		persistence.CreateProviderInput{
			Code:         "compute_test_a_" + unique,
			Name:         "Compute Test Provider A",
			ProviderType: "cloud",
		},
	)
	if err != nil {
		t.Fatalf("create provider A: %v", err)
	}

	// Register cleanup immediately after creating the first provider.
	// Delete child profiles before deleting parent providers.
	providerIDs := []string{providerA.ID}

	t.Cleanup(func() {
		cleanupCtx, done := context.WithTimeout(
			context.Background(), 30*time.Second,
		)
		defer done()

		for _, providerID := range providerIDs {
			_, err := db.Pool().Exec(
				cleanupCtx,
				`DELETE FROM compute_profiles
				 WHERE provider_id = $1::uuid`,
				providerID,
			)
			if err != nil {
				t.Errorf("cleanup compute profiles: %v", err)
				continue
			}

			if err := providerRepo.Delete(
				cleanupCtx, providerID,
			); err != nil {
				t.Errorf(
					"cleanup provider %s: %v",
					providerID, err,
				)
			}
		}
	})

	providerB, err := providerRepo.Create(
		ctx,
		persistence.CreateProviderInput{
			Code:         "compute_test_b_" + unique,
			Name:         "Compute Test Provider B",
			ProviderType: "cloud",
		},
	)
	if err != nil {
		t.Fatalf("create provider B: %v", err)
	}
	providerIDs = append(providerIDs, providerB.ID)

	profile, err := profileRepo.Create(
		ctx,
		persistence.CreateComputeProfileInput{
			ProviderID:   providerA.ID,
			Name:         "General Purpose",
			ProviderSize: "s-2vcpu-4gb",
			VCPUCount:    profileInt32(2),
			MemoryMB:     profileInt32(4096),
			DiskGB:       profileInt32(80),
			GPUCount:     profileInt32(0),
			Architecture: profileString("x86_64"),
			HourlyPrice:  profileString("0.029760"),
			Currency:     profileString("USD"),
		},
	)
	if err != nil {
		t.Fatalf("create compute profile: %v", err)
	}

	if profile.ID == "" {
		t.Fatal("compute profile ID is empty")
	}
	if profile.ProviderID != providerA.ID {
		t.Fatal("provider ID mismatch")
	}
	if !profile.Enabled {
		t.Fatal("new compute profile should be enabled")
	}
	if profile.HourlyPrice == nil ||
		*profile.HourlyPrice != "0.029760" {
		t.Fatalf(
			"unexpected hourly price: %v",
			profile.HourlyPrice,
		)
	}

	t.Logf("Created provider A: %s", providerA.ID)
	t.Logf("Created provider B: %s", providerB.ID)
	t.Logf("Created compute profile: %s", profile.ID)

	t.Run("GetByID", func(t *testing.T) {
		found, err := profileRepo.GetByID(ctx, profile.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if found.ID != profile.ID {
			t.Fatal("retrieved profile ID mismatch")
		}
		if found.VCPUCount == nil || *found.VCPUCount != 2 {
			t.Fatal("retrieved CPU count mismatch")
		}
	})

	t.Run("GetByProviderSize", func(t *testing.T) {
		found, err := profileRepo.GetByProviderSize(
			ctx, providerA.ID, "s-2vcpu-4gb",
		)
		if err != nil {
			t.Fatalf("GetByProviderSize: %v", err)
		}
		if found.ID != profile.ID {
			t.Fatal("retrieved profile mismatch")
		}
	})

	t.Run("ListByProvider", func(t *testing.T) {
		items, err := profileRepo.List(
			ctx,
			persistence.ListComputeProfilesOptions{
				ProviderID: providerA.ID,
				Limit:      100,
			},
		)
		if err != nil {
			t.Fatalf("List: %v", err)
		}

		found := false
		for _, item := range items {
			if item.ProviderID != providerA.ID {
				t.Fatal("list returned another provider's profile")
			}
			if item.ID == profile.ID {
				found = true
			}
		}
		if !found {
			t.Fatal("compute profile missing from list")
		}
	})

	t.Run("SameSizeDifferentProvider", func(t *testing.T) {
		second, err := profileRepo.Create(
			ctx,
			persistence.CreateComputeProfileInput{
				ProviderID:   providerB.ID,
				Name:         "Other Provider General Purpose",
				ProviderSize: "s-2vcpu-4gb",
				VCPUCount:    profileInt32(2),
			},
		)
		if err != nil {
			t.Fatalf("create second provider profile: %v", err)
		}
		if second.ProviderID != providerB.ID {
			t.Fatal("second profile provider mismatch")
		}

		found, err := profileRepo.GetByProviderSize(
			ctx, providerB.ID, "s-2vcpu-4gb",
		)
		if err != nil {
			t.Fatalf("retrieve second provider profile: %v", err)
		}
		if found.ID != second.ID {
			t.Fatal("cross-provider lookup mismatch")
		}
	})

	t.Run("MinimalNullableProfile", func(t *testing.T) {
		minimal, err := profileRepo.Create(
			ctx,
			persistence.CreateComputeProfileInput{
				ProviderID:   providerA.ID,
				Name:         "Minimal Profile",
				ProviderSize: "minimal",
			},
		)
		if err != nil {
			t.Fatalf("create minimal profile: %v", err)
		}

		if minimal.VCPUCount != nil ||
			minimal.MemoryMB != nil ||
			minimal.DiskGB != nil ||
			minimal.GPUCount != nil ||
			minimal.HourlyPrice != nil {
			t.Fatal("expected nullable specifications to remain NULL")
		}
	})

	t.Run("InvalidProviderReference", func(t *testing.T) {
		_, err := profileRepo.Create(
			ctx,
			persistence.CreateComputeProfileInput{
				ProviderID:   "11111111-1111-4111-8111-111111111111",
				Name:         "Invalid Provider Profile",
				ProviderSize: "invalid-provider",
			},
		)
		if err == nil {
			t.Fatal("expected foreign-key failure")
		}

		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) ||
			pgErr.Code != "23503" {
			t.Fatalf(
				"expected SQLSTATE 23503, got %v",
				err,
			)
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
			t.Fatalf(
				"expected SQLSTATE 23503, got %v",
				err,
			)
		}
	})

	t.Run("Update", func(t *testing.T) {
		updated, err := profileRepo.Update(
			ctx,
			profile.ID,
			persistence.UpdateComputeProfileInput{
				Name:         "GPU Optimized",
				VCPUCount:    profileInt32(16),
				MemoryMB:     profileInt32(65536),
				DiskGB:       profileInt32(500),
				GPUCount:     profileInt32(2),
				GPUModel:     profileString("NVIDIA H100"),
				Architecture: profileString("x86_64"),
				HourlyPrice:  profileString("3.500000"),
				Currency:     profileString("USD"),
			},
		)
		if err != nil {
			t.Fatalf("Update: %v", err)
		}

		if updated.Name != "GPU Optimized" {
			t.Fatal("profile name not updated")
		}
		if updated.GPUCount == nil || *updated.GPUCount != 2 {
			t.Fatal("GPU count not updated")
		}
		if updated.GPUModel == nil ||
			*updated.GPUModel != "NVIDIA H100" {
			t.Fatal("GPU model not updated")
		}
		if updated.HourlyPrice == nil ||
			*updated.HourlyPrice != "3.500000" {
			t.Fatal("hourly price not updated")
		}
		if updated.ProviderSize != "s-2vcpu-4gb" {
			t.Fatal("provider size changed unexpectedly")
		}
		if updated.ProviderID != providerA.ID {
			t.Fatal("provider ID changed unexpectedly")
		}
	})

	t.Run("Disable", func(t *testing.T) {
		updated, err := profileRepo.SetEnabled(
			ctx, profile.ID, false,
		)
		if err != nil {
			t.Fatalf("Disable: %v", err)
		}
		if updated.Enabled {
			t.Fatal("profile should be disabled")
		}
	})

	t.Run("ListDisabled", func(t *testing.T) {
		disabled := false

		items, err := profileRepo.List(
			ctx,
			persistence.ListComputeProfilesOptions{
				ProviderID: providerA.ID,
				Enabled:    &disabled,
			},
		)
		if err != nil {
			t.Fatalf("list disabled: %v", err)
		}

		found := false
		for _, item := range items {
			if item.ID == profile.ID {
				found = true
			}
		}
		if !found {
			t.Fatal("disabled profile missing")
		}

		enabled := true
		items, err = profileRepo.List(
			ctx,
			persistence.ListComputeProfilesOptions{
				ProviderID: providerA.ID,
				Enabled:    &enabled,
			},
		)
		if err != nil {
			t.Fatalf("list enabled: %v", err)
		}
		for _, item := range items {
			if item.ID == profile.ID {
				t.Fatal("disabled profile in enabled list")
			}
		}
	})

	t.Run("Enable", func(t *testing.T) {
		updated, err := profileRepo.SetEnabled(
			ctx, profile.ID, true,
		)
		if err != nil {
			t.Fatalf("Enable: %v", err)
		}
		if !updated.Enabled {
			t.Fatal("profile should be enabled")
		}
	})

	t.Run("GetMissing", func(t *testing.T) {
		_, err := profileRepo.GetByID(
			ctx, "22222222-2222-4222-8222-222222222222",
		)
		if !errors.Is(
			err, persistence.ErrComputeProfileNotFound,
		) {
			t.Fatalf("expected not found, got %v", err)
		}
	})

	t.Run("Delete", func(t *testing.T) {
		if err := profileRepo.Delete(ctx, profile.ID); err != nil {
			t.Fatalf("Delete: %v", err)
		}

		_, err := profileRepo.GetByID(ctx, profile.ID)
		if !errors.Is(
			err, persistence.ErrComputeProfileNotFound,
		) {
			t.Fatalf("expected not found, got %v", err)
		}
	})

	t.Run("DeleteMissing", func(t *testing.T) {
		err := profileRepo.Delete(ctx, profile.ID)
		if !errors.Is(
			err, persistence.ErrComputeProfileNotFound,
		) {
			t.Fatalf("expected not found, got %v", err)
		}
	})
}
