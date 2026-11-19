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

func provisioningTestString(s string) *string {
	return &s
}

func TestProvisioningTemplateCRUD(t *testing.T) {
	cfg, err := config.Load("../../../configs/server.yaml")
	if err != nil {
		t.Fatalf("load config: %v", err)
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

	projectRepo, err := persistence.NewInfrastructureProjectRepository(db)
	if err != nil {
		t.Fatalf("project repository: %v", err)
	}

	repo, err := persistence.NewProvisioningTemplateRepository(db)
	if err != nil {
		t.Fatalf("template repository: %v", err)
	}

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	templateName := "template_test_" + suffix
	projectName := "template_project_" + suffix

	// Register cleanup before creating test resources.
	var templateIDs []string
	var projectIDs []string

	t.Cleanup(func() {
		cleanupCtx, done := context.WithTimeout(
			context.Background(), 30*time.Second,
		)
		defer done()

		// Delete templates before their parent projects.
		for _, id := range templateIDs {
			_, err := db.Pool().Exec(
				cleanupCtx,
				`DELETE FROM provisioning_templates WHERE id = $1`,
				id,
			)
			if err != nil {
				t.Errorf("cleanup template %s: %v", id, err)
			}
		}

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

	project, err := projectRepo.Create(
		ctx,
		persistence.CreateInfrastructureProjectInput{
			Name:        projectName,
			Environment: "testing",
		},
	)
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	projectIDs = append(projectIDs, project.ID)

	template, err := repo.Create(
		ctx,
		persistence.CreateProvisioningTemplateInput{
			ProjectID:      &project.ID,
			Name:           templateName,
			Version:        1,
			Description:    provisioningTestString("RL compute template"),
			Provider:       provisioningTestString("digitalocean"),
			ImageReference: provisioningTestString("centos-stream-9"),
			Configuration: json.RawMessage(
				`{"region":"nyc3","size":"s-2vcpu-4gb","agents":10}`,
			),
		},
	)
	if err != nil {
		t.Fatalf("create template: %v", err)
	}
	templateIDs = append(templateIDs, template.ID)

	if template.ID == "" {
		t.Fatal("template ID is empty")
	}
	if template.ProjectID == nil || *template.ProjectID != project.ID {
		t.Fatal("project association mismatch")
	}
	if template.Version != 1 {
		t.Fatalf("expected version 1, got %d", template.Version)
	}
	if !template.Enabled {
		t.Fatal("new template should be enabled")
	}
	if template.CreatedAt.IsZero() || template.UpdatedAt.IsZero() {
		t.Fatal("timestamps are missing")
	}

	t.Run("GetByID", func(t *testing.T) {
		found, err := repo.GetByID(ctx, template.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if found.ID != template.ID {
			t.Fatal("template ID mismatch")
		}
	})

	t.Run("ConfigurationJSONB", func(t *testing.T) {
		found, err := repo.GetByID(ctx, template.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}

		var config map[string]any
		if err := json.Unmarshal(found.Configuration, &config); err != nil {
			t.Fatalf("decode configuration: %v", err)
		}

		if config["region"] != "nyc3" {
			t.Fatal("region was not persisted")
		}
		if config["size"] != "s-2vcpu-4gb" {
			t.Fatal("compute size was not persisted")
		}
		if config["agents"] != float64(10) {
			t.Fatal("agent count was not persisted")
		}
	})

	t.Run("DefaultVersion", func(t *testing.T) {
		created, err := repo.Create(
			ctx,
			persistence.CreateProvisioningTemplateInput{
				Name:          "default_version_" + suffix,
				Configuration: json.RawMessage(`{}`),
			},
		)
		if err != nil {
			t.Fatalf("create default-version template: %v", err)
		}
		templateIDs = append(templateIDs, created.ID)

		if created.Version != 1 {
			t.Fatalf("expected version 1, got %d", created.Version)
		}
		if created.ProjectID != nil {
			t.Fatal("global template should have no project")
		}
	})

	t.Run("MultipleVersions", func(t *testing.T) {
		created, err := repo.Create(
			ctx,
			persistence.CreateProvisioningTemplateInput{
				ProjectID:     &project.ID,
				Name:          templateName,
				Version:       2,
				Provider:      provisioningTestString("digitalocean"),
				Configuration: json.RawMessage(`{"revision":2}`),
			},
		)
		if err != nil {
			t.Fatalf("create second version: %v", err)
		}
		templateIDs = append(templateIDs, created.ID)

		if created.Version != 2 {
			t.Fatal("second version mismatch")
		}
		if created.ID == template.ID {
			t.Fatal("versions must have different IDs")
		}
	})

	t.Run("ListByProject", func(t *testing.T) {
		items, err := repo.List(
			ctx,
			persistence.ListProvisioningTemplatesOptions{
				ProjectID: &project.ID,
			},
		)
		if err != nil {
			t.Fatalf("List: %v", err)
		}

		count := 0
		for _, item := range items {
			if item.ProjectID == nil ||
				*item.ProjectID != project.ID {
				t.Fatal("incorrect project in results")
			}
			if item.Name == templateName {
				count++
			}
		}
		if count != 2 {
			t.Fatalf("expected 2 template versions, got %d", count)
		}
	})

	t.Run("ListByProvider", func(t *testing.T) {
		items, err := repo.List(
			ctx,
			persistence.ListProvisioningTemplatesOptions{
				Provider: "digitalocean",
			},
		)
		if err != nil {
			t.Fatalf("List: %v", err)
		}

		found := false
		for _, item := range items {
			if item.Provider == nil ||
				*item.Provider != "digitalocean" {
				t.Fatal("incorrect provider in results")
			}
			if item.ID == template.ID {
				found = true
			}
		}
		if !found {
			t.Fatal("template missing from provider list")
		}
	})

	t.Run("Update", func(t *testing.T) {
		time.Sleep(10 * time.Millisecond)

		updated, err := repo.Update(
			ctx,
			template.ID,
			persistence.UpdateProvisioningTemplateInput{
				ProjectID:      &project.ID,
				Name:           templateName,
				Version:        3,
				Description:    provisioningTestString("Updated template"),
				Provider:       provisioningTestString("digitalocean"),
				ImageReference: provisioningTestString("ubuntu-24-04"),
				Configuration: json.RawMessage(
					`{"region":"sfo3","size":"s-4vcpu-8gb"}`,
				),
			},
		)
		if err != nil {
			t.Fatalf("Update: %v", err)
		}

		if updated.Version != 3 {
			t.Fatal("version not updated")
		}
		if updated.ImageReference == nil ||
			*updated.ImageReference != "ubuntu-24-04" {
			t.Fatal("image reference not updated")
		}
		if updated.UpdatedAt.Before(template.UpdatedAt) {
			t.Fatal("updated_at moved backwards")
		}
		if !updated.CreatedAt.Equal(template.CreatedAt) {
			t.Fatal("created_at changed")
		}
	})

	t.Run("Disable", func(t *testing.T) {
		updated, err := repo.SetEnabled(ctx, template.ID, false)
		if err != nil {
			t.Fatalf("disable template: %v", err)
		}
		if updated.Enabled {
			t.Fatal("template is still enabled")
		}
	})

	t.Run("ListDisabled", func(t *testing.T) {
		disabled := false

		items, err := repo.List(
			ctx,
			persistence.ListProvisioningTemplatesOptions{
				ProjectID: &project.ID,
				Enabled:   &disabled,
			},
		)
		if err != nil {
			t.Fatalf("List disabled: %v", err)
		}

		found := false
		for _, item := range items {
			if item.Enabled {
				t.Fatal("enabled template in disabled results")
			}
			if item.ID == template.ID {
				found = true
			}
		}
		if !found {
			t.Fatal("disabled template missing from results")
		}
	})

	t.Run("Enable", func(t *testing.T) {
		updated, err := repo.SetEnabled(ctx, template.ID, true)
		if err != nil {
			t.Fatalf("enable template: %v", err)
		}
		if !updated.Enabled {
			t.Fatal("template is still disabled")
		}
	})

	t.Run("ClearProjectAssociation", func(t *testing.T) {
		updated, err := repo.Update(
			ctx,
			template.ID,
			persistence.UpdateProvisioningTemplateInput{
				Name:          templateName,
				Version:       3,
				Configuration: json.RawMessage(`{}`),
			},
		)
		if err != nil {
			t.Fatalf("clear project association: %v", err)
		}
		if updated.ProjectID != nil {
			t.Fatal("project association was not cleared")
		}
		if updated.Description != nil ||
			updated.Provider != nil ||
			updated.ImageReference != nil {
			t.Fatal("optional fields were not cleared")
		}
	})

	t.Run("InvalidProjectReference", func(t *testing.T) {
		missingProject := "22222222-2222-4222-8222-222222222222"

		_, err := repo.Create(
			ctx,
			persistence.CreateProvisioningTemplateInput{
				ProjectID:     &missingProject,
				Name:          "invalid_project_" + suffix,
				Configuration: json.RawMessage(`{}`),
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
			err, persistence.ErrProvisioningTemplateNotFound,
		) {
			t.Fatalf("expected not found, got %v", err)
		}
	})

	t.Run("Delete", func(t *testing.T) {
		if err := repo.Delete(ctx, template.ID); err != nil {
			t.Fatalf("Delete: %v", err)
		}

		_, err := repo.GetByID(ctx, template.ID)
		if !errors.Is(
			err, persistence.ErrProvisioningTemplateNotFound,
		) {
			t.Fatalf("expected not found, got %v", err)
		}
	})

	t.Run("DeleteMissing", func(t *testing.T) {
		err := repo.Delete(ctx, template.ID)
		if !errors.Is(
			err, persistence.ErrProvisioningTemplateNotFound,
		) {
			t.Fatalf("expected not found, got %v", err)
		}
	})
}
