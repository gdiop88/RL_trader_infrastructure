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

func networkIntegrationString(value string) *string {
	return &value
}

func TestNetworkResourceCRUD(t *testing.T) {
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
		t.Fatalf("connect PostgreSQL: %v", err)
	}
	t.Cleanup(db.Close)

	ctx, cancel := context.WithTimeout(
		context.Background(), 120*time.Second,
	)
	defer cancel()

	repo, err := persistence.NewNetworkResourceRepository(db)
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())

	var projectID, providerID, accountID string
	var resourceIDs []string

	t.Cleanup(func() {
		cleanupCtx, done := context.WithTimeout(
			context.Background(), 30*time.Second,
		)
		defer done()

		for _, id := range resourceIDs {
			_, err := db.Pool().Exec(
				cleanupCtx,
				`DELETE FROM network_resources WHERE id = $1`,
				id,
			)
			if err != nil {
				t.Errorf("cleanup network %s: %v", id, err)
			}
		}

		if accountID != "" {
			_, err := db.Pool().Exec(
				cleanupCtx,
				`DELETE FROM provider_accounts WHERE id = $1`,
				accountID,
			)
			if err != nil {
				t.Errorf("cleanup provider account: %v", err)
			}
		}

		if providerID != "" {
			_, err := db.Pool().Exec(
				cleanupCtx,
				`DELETE FROM cloud_providers WHERE id = $1`,
				providerID,
			)
			if err != nil {
				t.Errorf("cleanup provider: %v", err)
			}
		}

		if projectID != "" {
			_, err := db.Pool().Exec(
				cleanupCtx,
				`DELETE FROM infrastructure_projects WHERE id = $1`,
				projectID,
			)
			if err != nil {
				t.Errorf("cleanup project: %v", err)
			}
		}
	})

	// Create isolated foreign-key fixtures.

	err = db.Pool().QueryRow(
		ctx,
		`INSERT INTO infrastructure_projects (name, environment)
		 VALUES ($1, $2) RETURNING id`,
		"network-project-"+suffix,
		"integration",
	).Scan(&projectID)
	if err != nil {
		t.Fatalf("create project fixture: %v", err)
	}

	err = db.Pool().QueryRow(
		ctx,
		`INSERT INTO cloud_providers (code, name, provider_type)
		 VALUES ($1, $2, $3) RETURNING id`,
		"network-"+suffix,
		"Network Integration Provider",
		"cloud",
	).Scan(&providerID)
	if err != nil {
		t.Fatalf("create provider fixture: %v", err)
	}

	err = db.Pool().QueryRow(
		ctx,
		`INSERT INTO provider_accounts (provider_id, name)
		 VALUES ($1, $2) RETURNING id`,
		providerID,
		"network-account-"+suffix,
	).Scan(&accountID)
	if err != nil {
		t.Fatalf("create account fixture: %v", err)
	}

	first, err := repo.Create(
		ctx,
		persistence.CreateNetworkResourceInput{
			ProjectID:         &projectID,
			ProviderAccountID: &accountID,
			ProviderNetworkID: networkIntegrationString("vpc-" + suffix),
			Name:              networkIntegrationString("trading-vpc"),
			Region:            networkIntegrationString("nyc1"),
			NetworkType:       networkIntegrationString("vpc"),
			CIDR:              networkIntegrationString("10.20.0.0/16"),
			Status:            networkIntegrationString("creating"),
			Metadata:          json.RawMessage(`{"private":true,"version":1}`),
		},
	)
	if err != nil {
		t.Fatalf("create first network: %v", err)
	}
	resourceIDs = append(resourceIDs, first.ID)

	t.Run("Create", func(t *testing.T) {
		if first.ID == "" {
			t.Fatal("expected generated ID")
		}
		if first.ProjectID == nil || *first.ProjectID != projectID {
			t.Fatal("project ID mismatch")
		}
		if first.ProviderAccountID == nil ||
			*first.ProviderAccountID != accountID {
			t.Fatal("provider account ID mismatch")
		}
		if first.CreatedAt.IsZero() {
			t.Fatal("created_at was not generated")
		}
		if first.CIDR == nil || *first.CIDR != "10.20.0.0/16" {
			t.Fatalf("CIDR mismatch: %v", first.CIDR)
		}
	})

	t.Run("GetByID", func(t *testing.T) {
		got, err := repo.GetByID(ctx, first.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got.ID != first.ID {
			t.Fatal("incorrect resource returned")
		}
	})

	t.Run("JSONBPersistence", func(t *testing.T) {
		got, err := repo.GetByID(ctx, first.ID)
		if err != nil {
			t.Fatal(err)
		}

		var metadata map[string]any
		if err := json.Unmarshal(got.Metadata, &metadata); err != nil {
			t.Fatal(err)
		}
		if metadata["private"] != true ||
			metadata["version"] != float64(1) {
			t.Fatal("metadata mismatch")
		}
	})

	t.Run("InvalidForeignKeys", func(t *testing.T) {
		missing := "33333333-3333-4333-8333-333333333333"

		_, err := repo.Create(
			ctx,
			persistence.CreateNetworkResourceInput{
				ProjectID: &missing,
			},
		)
		if err == nil {
			t.Fatal("expected project foreign-key error")
		}

		_, err = repo.Create(
			ctx,
			persistence.CreateNetworkResourceInput{
				ProviderAccountID: &missing,
			},
		)
		if err == nil {
			t.Fatal("expected provider account foreign-key error")
		}
	})

	second, err := repo.Create(
		ctx,
		persistence.CreateNetworkResourceInput{
			ProjectID:         &projectID,
			ProviderAccountID: &accountID,
			Name:              networkIntegrationString("private-ipv6"),
			Region:            networkIntegrationString("sfo3"),
			NetworkType:       networkIntegrationString("private"),
			CIDR:              networkIntegrationString("2001:db8::/32"),
			Status:            networkIntegrationString("active"),
		},
	)
	if err != nil {
		t.Fatalf("create second network: %v", err)
	}
	resourceIDs = append(resourceIDs, second.ID)

	t.Run("IPv6Persistence", func(t *testing.T) {
		got, err := repo.GetByID(ctx, second.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.CIDR == nil || *got.CIDR != "2001:db8::/32" {
			t.Fatalf("IPv6 CIDR mismatch: %v", got.CIDR)
		}
	})

	t.Run("NullableFields", func(t *testing.T) {
		resource, err := repo.Create(
			ctx,
			persistence.CreateNetworkResourceInput{},
		)
		if err != nil {
			t.Fatal(err)
		}
		resourceIDs = append(resourceIDs, resource.ID)

		got, err := repo.GetByID(ctx, resource.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.ProjectID != nil ||
			got.ProviderAccountID != nil ||
			got.ProviderNetworkID != nil ||
			got.Name != nil ||
			got.Region != nil ||
			got.NetworkType != nil ||
			got.CIDR != nil ||
			got.Status != nil ||
			len(got.Metadata) != 0 {
			t.Fatal("expected all optional fields to be NULL")
		}
	})

	t.Run("ListByProject", func(t *testing.T) {
		rows, err := repo.List(
			ctx,
			persistence.ListNetworkResourcesOptions{
				ProjectID: &projectID,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 2 {
			t.Fatalf("expected 2 networks, got %d", len(rows))
		}
	})

	t.Run("ListByAccount", func(t *testing.T) {
		rows, err := repo.List(
			ctx,
			persistence.ListNetworkResourcesOptions{
				ProviderAccountID: &accountID,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 2 {
			t.Fatalf("expected 2 networks, got %d", len(rows))
		}
	})

	t.Run("ListByRegionTypeStatus", func(t *testing.T) {
		rows, err := repo.List(
			ctx,
			persistence.ListNetworkResourcesOptions{
				ProjectID:   &projectID,
				Region:      "sfo3",
				NetworkType: "private",
				Status:      "active",
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 || rows[0].ID != second.ID {
			t.Fatal("incorrect filtered results")
		}
	})

	t.Run("Pagination", func(t *testing.T) {
		rows, err := repo.List(
			ctx,
			persistence.ListNetworkResourcesOptions{
				ProjectID: &projectID,
				Limit:     1,
				Offset:    1,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 {
			t.Fatalf("expected one row, got %d", len(rows))
		}
	})

	t.Run("Update", func(t *testing.T) {
		updated, err := repo.Update(
			ctx,
			first.ID,
			persistence.UpdateNetworkResourceInput{
				ProjectID:         &projectID,
				ProviderAccountID: &accountID,
				ProviderNetworkID: networkIntegrationString("vpc-updated-" + suffix),
				Name:              networkIntegrationString("updated-vpc"),
				Region:            networkIntegrationString("nyc3"),
				NetworkType:       networkIntegrationString("vpc"),
				CIDR:              networkIntegrationString("172.16.0.0/16"),
				Status:            networkIntegrationString("ready"),
				Metadata:          json.RawMessage(`{"version":2}`),
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if updated.Name == nil || *updated.Name != "updated-vpc" {
			t.Fatal("name not updated")
		}
		if updated.CIDR == nil || *updated.CIDR != "172.16.0.0/16" {
			t.Fatal("CIDR not updated")
		}
		if updated.Status == nil || *updated.Status != "ready" {
			t.Fatal("status not updated")
		}
		if updated.ID != first.ID {
			t.Fatal("primary key changed")
		}
	})

	t.Run("UpdateStatus", func(t *testing.T) {
		got, err := repo.UpdateStatus(
			ctx, second.ID, networkIntegrationString("provisioning"),
		)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status == nil || *got.Status != "provisioning" {
			t.Fatal("status update mismatch")
		}
	})

	t.Run("ClearNullableFields", func(t *testing.T) {
		got, err := repo.Update(
			ctx,
			second.ID,
			persistence.UpdateNetworkResourceInput{},
		)
		if err != nil {
			t.Fatal(err)
		}
		if got.ProjectID != nil ||
			got.ProviderAccountID != nil ||
			got.Name != nil ||
			got.CIDR != nil ||
			got.Status != nil ||
			len(got.Metadata) != 0 {
			t.Fatal("nullable fields not cleared")
		}
	})

	t.Run("SetNullStatus", func(t *testing.T) {
		_, err := repo.UpdateStatus(
			ctx, first.ID, networkIntegrationString("active"),
		)
		if err != nil {
			t.Fatal(err)
		}

		got, err := repo.UpdateStatus(ctx, first.ID, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status != nil {
			t.Fatal("expected NULL status")
		}
	})

	t.Run("JSONBArray", func(t *testing.T) {
		got, err := repo.Update(
			ctx,
			first.ID,
			persistence.UpdateNetworkResourceInput{
				Metadata: json.RawMessage(`["private","encrypted"]`),
			},
		)
		if err != nil {
			t.Fatal(err)
		}

		var values []string
		if err := json.Unmarshal(got.Metadata, &values); err != nil {
			t.Fatal(err)
		}
		if len(values) != 2 {
			t.Fatal("JSONB array mismatch")
		}
	})

	t.Run("JSONBScalar", func(t *testing.T) {
		got, err := repo.Update(
			ctx,
			first.ID,
			persistence.UpdateNetworkResourceInput{
				Metadata: json.RawMessage(`42`),
			},
		)
		if err != nil {
			t.Fatal(err)
		}

		var value int
		if err := json.Unmarshal(got.Metadata, &value); err != nil {
			t.Fatal(err)
		}
		if value != 42 {
			t.Fatal("JSONB scalar mismatch")
		}
	})

	t.Run("DuplicateProviderNetworkIDAllowed", func(t *testing.T) {
		shared := "shared-network-" + suffix

		for i := 0; i < 2; i++ {
			resource, err := repo.Create(
				ctx,
				persistence.CreateNetworkResourceInput{
					ProviderNetworkID: &shared,
				},
			)
			if err != nil {
				t.Fatalf("create duplicate provider ID: %v", err)
			}
			resourceIDs = append(resourceIDs, resource.ID)
		}
	})

	missingID := "44444444-4444-4444-8444-444444444444"

	t.Run("GetMissing", func(t *testing.T) {
		_, err := repo.GetByID(ctx, missingID)
		if !errors.Is(err, persistence.ErrNetworkResourceNotFound) {
			t.Fatalf("expected not found, got %v", err)
		}
	})

	t.Run("UpdateMissing", func(t *testing.T) {
		_, err := repo.Update(
			ctx, missingID,
			persistence.UpdateNetworkResourceInput{},
		)
		if !errors.Is(err, persistence.ErrNetworkResourceNotFound) {
			t.Fatalf("expected not found, got %v", err)
		}
	})

	t.Run("UpdateStatusMissing", func(t *testing.T) {
		_, err := repo.UpdateStatus(
			ctx, missingID, networkIntegrationString("active"),
		)
		if !errors.Is(err, persistence.ErrNetworkResourceNotFound) {
			t.Fatalf("expected not found, got %v", err)
		}
	})

	t.Run("Delete", func(t *testing.T) {
		if err := repo.Delete(ctx, second.ID); err != nil {
			t.Fatal(err)
		}

		_, err := repo.GetByID(ctx, second.ID)
		if !errors.Is(err, persistence.ErrNetworkResourceNotFound) {
			t.Fatalf("expected deleted resource missing, got %v", err)
		}
	})

	t.Run("DeleteMissing", func(t *testing.T) {
		err := repo.Delete(ctx, missingID)
		if !errors.Is(err, persistence.ErrNetworkResourceNotFound) {
			t.Fatalf("expected not found, got %v", err)
		}
	})
}
