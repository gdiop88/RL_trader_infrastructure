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

func TestProviderAccountCRUD(t *testing.T) {
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
		t.Fatalf("connect to Aiven PostgreSQL: %v", err)
	}

	// Cleanup runs in reverse registration order.
	t.Cleanup(db.Close)

	ctx, cancel := context.WithTimeout(
		context.Background(), 90*time.Second,
	)
	defer cancel()

	providerRepo, err := persistence.NewProviderRepository(db)
	if err != nil {
		t.Fatalf("initialize provider repository: %v", err)
	}

	accountRepo, err := persistence.NewProviderAccountRepository(db)
	if err != nil {
		t.Fatalf("initialize account repository: %v", err)
	}

	unique := fmt.Sprintf("%d", time.Now().UnixNano())
	providerCode := "integration_account_" + unique

	provider, err := providerRepo.Create(
		ctx,
		persistence.CreateProviderInput{
			Code:         providerCode,
			Name:         "Integration Account Provider",
			ProviderType: "cloud",
		},
	)
	if err != nil {
		t.Fatalf("create test provider: %v", err)
	}

	// Register provider cleanup before account cleanup.
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
			providerCode,
		)
		if cleanupErr != nil {
			t.Errorf("cleanup provider: %v", cleanupErr)
		}
	})

	accountName := "Integration Account " + unique
	externalID := "external-" + unique
	credentialRef := "secret://integration/" + unique

	account, err := accountRepo.Create(
		ctx,
		persistence.CreateProviderAccountInput{
			ProviderID:          provider.ID,
			Name:                accountName,
			ExternalAccountID:   &externalID,
			CredentialReference: &credentialRef,
			Metadata: json.RawMessage(
				`{"environment":"integration"}`,
			),
		},
	)
	if err != nil {
		t.Fatalf("create test account: %v", err)
	}

	// Account cleanup executes before provider cleanup.
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(), 15*time.Second,
		)
		defer cleanupCancel()

		_, cleanupErr := db.Pool().Exec(
			cleanupCtx,
			`DELETE FROM provider_accounts
			 WHERE id = $1::uuid AND provider_id = $2::uuid`,
			account.ID,
			provider.ID,
		)
		if cleanupErr != nil {
			t.Errorf("cleanup account: %v", cleanupErr)
		}
	})

	if account.ID == "" {
		t.Fatal("created account has no UUID")
	}
	if account.ProviderID != provider.ID {
		t.Fatal("account provider ID mismatch")
	}
	if !account.Enabled {
		t.Fatal("new account should be enabled")
	}
	if account.CredentialReference == nil ||
		*account.CredentialReference != credentialRef {
		t.Fatal("credential reference not stored")
	}

	t.Logf("Created test provider: %s", provider.ID)
	t.Logf("Created test account: %s", account.ID)

	t.Run("GetByID", func(t *testing.T) {
		found, err := accountRepo.GetByID(ctx, account.ID)
		if err != nil {
			t.Fatalf("get account: %v", err)
		}
		if found.ID != account.ID ||
			found.ProviderID != provider.ID {
			t.Fatal("retrieved account does not match")
		}
	})

	t.Run("ListByProvider", func(t *testing.T) {
		items, err := accountRepo.List(
			ctx,
			persistence.ListProviderAccountsOptions{
				ProviderID: provider.ID,
				Limit:      100,
			},
		)
		if err != nil {
			t.Fatalf("list accounts: %v", err)
		}

		found := false
		for _, item := range items {
			if item.ID == account.ID {
				found = true
				break
			}
		}
		if !found {
			t.Fatal("account missing from provider list")
		}
	})

	t.Run("ForeignKeyProtection", func(t *testing.T) {
		err := providerRepo.Delete(ctx, provider.ID)
		if err == nil {
			t.Fatal("expected provider deletion to be blocked")
		}

		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) ||
			pgErr.Code != "23503" {
			t.Fatalf("expected SQLSTATE 23503, got %v", err)
		}
	})

	t.Run("InvalidProviderReference", func(t *testing.T) {
		_, err := accountRepo.Create(
			ctx,
			persistence.CreateProviderAccountInput{
				ProviderID: "11111111-1111-4111-8111-111111111111",
				Name:       "Invalid Provider Account",
			},
		)
		if err == nil {
			t.Fatal("expected foreign key error")
		}

		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) ||
			pgErr.Code != "23503" {
			t.Fatalf("expected SQLSTATE 23503, got %v", err)
		}
	})

	t.Run("Update", func(t *testing.T) {
		newName := "Updated Integration Account"
		newExternalID := "updated-" + unique
		newCredentialRef := "secret://updated/" + unique

		updated, err := accountRepo.Update(
			ctx,
			account.ID,
			persistence.UpdateProviderAccountInput{
				Name:                newName,
				ExternalAccountID:   &newExternalID,
				CredentialReference: &newCredentialRef,
				Metadata: json.RawMessage(
					`{"environment":"updated"}`,
				),
			},
		)
		if err != nil {
			t.Fatalf("update account: %v", err)
		}
		if updated.Name != newName {
			t.Fatal("account name was not updated")
		}
		if updated.ExternalAccountID == nil ||
			*updated.ExternalAccountID != newExternalID {
			t.Fatal("external account ID was not updated")
		}
		if updated.CredentialReference == nil ||
			*updated.CredentialReference != newCredentialRef {
			t.Fatal("credential reference was not updated")
		}

		var metadata map[string]any
		if err := json.Unmarshal(updated.Metadata, &metadata); err != nil {
			t.Fatalf("decode metadata: %v", err)
		}
		if metadata["environment"] != "updated" {
			t.Fatalf("unexpected metadata: %v", metadata)
		}
	})

	t.Run("Disable", func(t *testing.T) {
		updated, err := accountRepo.SetEnabled(
			ctx, account.ID, false,
		)
		if err != nil {
			t.Fatalf("disable account: %v", err)
		}
		if updated.Enabled {
			t.Fatal("account should be disabled")
		}
	})

	t.Run("ListDisabled", func(t *testing.T) {
		disabled := false

		items, err := accountRepo.List(
			ctx,
			persistence.ListProviderAccountsOptions{
				ProviderID: provider.ID,
				Enabled:    &disabled,
			},
		)
		if err != nil {
			t.Fatalf("list disabled accounts: %v", err)
		}

		found := false
		for _, item := range items {
			if item.ID == account.ID {
				found = true
			}
		}
		if !found {
			t.Fatal("disabled account missing from list")
		}

		enabled := true
		items, err = accountRepo.List(
			ctx,
			persistence.ListProviderAccountsOptions{
				ProviderID: provider.ID,
				Enabled:    &enabled,
			},
		)
		if err != nil {
			t.Fatalf("list enabled accounts: %v", err)
		}
		for _, item := range items {
			if item.ID == account.ID {
				t.Fatal("disabled account appears in enabled list")
			}
		}
	})

	t.Run("Enable", func(t *testing.T) {
		updated, err := accountRepo.SetEnabled(
			ctx, account.ID, true,
		)
		if err != nil {
			t.Fatalf("enable account: %v", err)
		}
		if !updated.Enabled {
			t.Fatal("account should be enabled")
		}
	})

	t.Run("Delete", func(t *testing.T) {
		if err := accountRepo.Delete(ctx, account.ID); err != nil {
			t.Fatalf("delete account: %v", err)
		}

		_, err := accountRepo.GetByID(ctx, account.ID)
		if !errors.Is(err, persistence.ErrProviderAccountNotFound) {
			t.Fatalf("expected not found, got %v", err)
		}
	})

	t.Run("DeleteMissing", func(t *testing.T) {
		err := accountRepo.Delete(ctx, account.ID)
		if !errors.Is(err, persistence.ErrProviderAccountNotFound) {
			t.Fatalf("expected not found, got %v", err)
		}
	})
}
