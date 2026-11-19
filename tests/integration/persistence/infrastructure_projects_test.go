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

func integrationProjectString(value string) *string {
	return &value
}

func TestInfrastructureProjectCRUD(t *testing.T) {
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

	repo, err := persistence.NewInfrastructureProjectRepository(db)
	if err != nil {
		t.Fatalf("project repository: %v", err)
	}

	unique := fmt.Sprintf("%d", time.Now().UnixNano())
	name := "project_test_" + unique
	updatedName := "project_updated_" + unique
	owner := "integration-test-" + unique

	projectIDs := make([]string, 0)

	t.Cleanup(func() {
		cleanupCtx, done := context.WithTimeout(
			context.Background(), 30*time.Second,
		)
		defer done()

		for _, id := range projectIDs {
			_, err := db.Pool().Exec(
				cleanupCtx,
				`DELETE FROM infrastructure_projects WHERE id = $1`,
				id,
			)
			if err != nil {
				t.Errorf("cleanup project %s: %v", id, err)
			}
		}
	})

	project, err := repo.Create(
		ctx,
		persistence.CreateInfrastructureProjectInput{
			Name:           name,
			Environment:    "development",
			Description:    integrationProjectString("Integration test project"),
			OwnerReference: &owner,
			BudgetLimit:    integrationProjectString("100000.50"),
			Currency:       integrationProjectString("USD"),
			Metadata:       json.RawMessage(`{"managed":true,"team":"rl"}`),
		},
	)
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	projectIDs = append(projectIDs, project.ID)

	if project.ID == "" {
		t.Fatal("project ID is empty")
	}
	if project.Name != name {
		t.Fatal("project name mismatch")
	}
	if project.BudgetLimit == nil ||
		*project.BudgetLimit != "100000.50" {
		t.Fatalf("unexpected budget: %v", project.BudgetLimit)
	}
	if project.CreatedAt.IsZero() || project.UpdatedAt.IsZero() {
		t.Fatal("timestamps must be populated")
	}

	t.Logf("Created infrastructure project: %s", project.ID)

	t.Run("GetByID", func(t *testing.T) {
		found, err := repo.GetByID(ctx, project.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if found.ID != project.ID {
			t.Fatal("retrieved project ID mismatch")
		}
	})

	t.Run("GetByName", func(t *testing.T) {
		found, err := repo.GetByName(ctx, name)
		if err != nil {
			t.Fatalf("GetByName: %v", err)
		}
		if found.ID != project.ID {
			t.Fatal("retrieved project mismatch")
		}
	})

	t.Run("DuplicateName", func(t *testing.T) {
		_, err := repo.Create(
			ctx,
			persistence.CreateInfrastructureProjectInput{
				Name:        name,
				Environment: "staging",
			},
		)
		if !errors.Is(
			err, persistence.ErrInfrastructureProjectConflict,
		) {
			t.Fatalf("expected conflict, got %v", err)
		}
	})

	t.Run("MinimalProject", func(t *testing.T) {
		minimal, err := repo.Create(
			ctx,
			persistence.CreateInfrastructureProjectInput{
				Name:        "minimal_" + unique,
				Environment: "testing",
			},
		)
		if err != nil {
			t.Fatalf("create minimal project: %v", err)
		}
		projectIDs = append(projectIDs, minimal.ID)

		if minimal.Description != nil ||
			minimal.OwnerReference != nil ||
			minimal.BudgetLimit != nil ||
			minimal.Currency != nil {
			t.Fatal("expected optional fields to be NULL")
		}
		if len(minimal.Metadata) != 0 &&
			string(minimal.Metadata) != "null" {
			t.Fatalf("unexpected metadata: %s", minimal.Metadata)
		}
	})

	t.Run("ListByEnvironment", func(t *testing.T) {
		items, err := repo.List(
			ctx,
			persistence.ListInfrastructureProjectsOptions{
				Environment: "development",
				Limit:       100,
			},
		)
		if err != nil {
			t.Fatalf("List: %v", err)
		}

		found := false
		for _, item := range items {
			if item.Environment != "development" {
				t.Fatal("incorrect environment returned")
			}
			if item.ID == project.ID {
				found = true
			}
		}
		if !found {
			t.Fatal("project missing from environment list")
		}
	})

	t.Run("ListByOwner", func(t *testing.T) {
		items, err := repo.List(
			ctx,
			persistence.ListInfrastructureProjectsOptions{
				OwnerReference: owner,
			},
		)
		if err != nil {
			t.Fatalf("List by owner: %v", err)
		}

		found := false
		for _, item := range items {
			if item.OwnerReference == nil ||
				*item.OwnerReference != owner {
				t.Fatal("incorrect owner returned")
			}
			if item.ID == project.ID {
				found = true
			}
		}
		if !found {
			t.Fatal("project missing from owner list")
		}
	})

	t.Run("Update", func(t *testing.T) {
		// Ensure the timestamp advances even on systems
		// with relatively coarse timestamp resolution.
		time.Sleep(10 * time.Millisecond)

		updated, err := repo.Update(
			ctx,
			project.ID,
			persistence.UpdateInfrastructureProjectInput{
				Name:           updatedName,
				Environment:    "production",
				Description:    integrationProjectString("Updated project"),
				OwnerReference: &owner,
				BudgetLimit:    integrationProjectString("250000.75"),
				Currency:       integrationProjectString("USD"),
				Metadata:       json.RawMessage(`{"managed":true,"updated":true}`),
			},
		)
		if err != nil {
			t.Fatalf("Update: %v", err)
		}

		if updated.Name != updatedName {
			t.Fatal("name not updated")
		}
		if updated.Environment != "production" {
			t.Fatal("environment not updated")
		}
		if updated.BudgetLimit == nil ||
			*updated.BudgetLimit != "250000.75" {
			t.Fatal("budget not updated")
		}
		if updated.UpdatedAt.Before(project.UpdatedAt) {
			t.Fatal("updated_at moved backwards")
		}
		if !updated.CreatedAt.Equal(project.CreatedAt) {
			t.Fatal("created_at changed")
		}

		var metadata map[string]any
		if err := json.Unmarshal(updated.Metadata, &metadata); err != nil {
			t.Fatalf("decode metadata: %v", err)
		}
		if metadata["updated"] != true {
			t.Fatal("metadata not updated")
		}
	})

	t.Run("GetUpdatedName", func(t *testing.T) {
		found, err := repo.GetByName(ctx, updatedName)
		if err != nil {
			t.Fatalf("GetByName after update: %v", err)
		}
		if found.ID != project.ID {
			t.Fatal("updated name lookup mismatch")
		}

		_, err = repo.GetByName(ctx, name)
		if !errors.Is(
			err, persistence.ErrInfrastructureProjectNotFound,
		) {
			t.Fatalf("expected old name not found, got %v", err)
		}
	})

	t.Run("ClearOptionalFields", func(t *testing.T) {
		updated, err := repo.Update(
			ctx,
			project.ID,
			persistence.UpdateInfrastructureProjectInput{
				Name:        updatedName,
				Environment: "production",
			},
		)
		if err != nil {
			t.Fatalf("clear optional fields: %v", err)
		}
		if updated.Description != nil ||
			updated.OwnerReference != nil ||
			updated.BudgetLimit != nil ||
			updated.Currency != nil {
			t.Fatal("optional fields were not cleared")
		}
	})

	t.Run("GetMissing", func(t *testing.T) {
		_, err := repo.GetByID(
			ctx, "22222222-2222-4222-8222-222222222222",
		)
		if !errors.Is(
			err, persistence.ErrInfrastructureProjectNotFound,
		) {
			t.Fatalf("expected not found, got %v", err)
		}
	})

	t.Run("Delete", func(t *testing.T) {
		if err := repo.Delete(ctx, project.ID); err != nil {
			t.Fatalf("Delete: %v", err)
		}

		_, err := repo.GetByID(ctx, project.ID)
		if !errors.Is(
			err, persistence.ErrInfrastructureProjectNotFound,
		) {
			t.Fatalf("expected not found, got %v", err)
		}
	})

	t.Run("DeleteMissing", func(t *testing.T) {
		err := repo.Delete(ctx, project.ID)
		if !errors.Is(
			err, persistence.ErrInfrastructureProjectNotFound,
		) {
			t.Fatalf("expected not found, got %v", err)
		}
	})
}
