package persistence_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/gdiop88/RL_trader_infrastructure/internal/persistence"
)

const validTemplateProjectID = "550e8400-e29b-41d4-a716-446655440000"

func templateString(value string) *string {
	return &value
}

func validProvisioningTemplateInput() persistence.CreateProvisioningTemplateInput {
	return persistence.CreateProvisioningTemplateInput{
		ProjectID:      templateString(validTemplateProjectID),
		Name:           "RL Agent Compute",
		Version:        1,
		Description:    templateString("Compute template for RL agents"),
		Provider:       templateString("digitalocean"),
		ImageReference: templateString("centos-stream-9"),
		Configuration: json.RawMessage(
			`{"size":"s-2vcpu-4gb","region":"nyc3"}`,
		),
	}
}

func TestCreateProvisioningTemplateValidation(t *testing.T) {
	t.Run("valid template", func(t *testing.T) {
		input := validProvisioningTemplateInput()
		if err := input.Validate(); err != nil {
			t.Fatalf("expected valid template: %v", err)
		}
	})

	t.Run("minimal global template", func(t *testing.T) {
		input := persistence.CreateProvisioningTemplateInput{
			Name:          "Global Template",
			Configuration: json.RawMessage(`{}`),
		}
		if err := input.Validate(); err != nil {
			t.Fatalf("expected valid minimal template: %v", err)
		}
	})

	t.Run("version zero uses default", func(t *testing.T) {
		input := validProvisioningTemplateInput()
		input.Version = 0
		if err := input.Validate(); err != nil {
			t.Fatalf("expected default version to be valid: %v", err)
		}
	})

	tests := []struct {
		name   string
		mutate func(*persistence.CreateProvisioningTemplateInput)
	}{
		{
			name: "invalid project ID",
			mutate: func(in *persistence.CreateProvisioningTemplateInput) {
				in.ProjectID = templateString("invalid")
			},
		},
		{
			name: "missing name",
			mutate: func(in *persistence.CreateProvisioningTemplateInput) {
				in.Name = " "
			},
		},
		{
			name: "name too long",
			mutate: func(in *persistence.CreateProvisioningTemplateInput) {
				in.Name = strings.Repeat("n", 151)
			},
		},
		{
			name: "negative version",
			mutate: func(in *persistence.CreateProvisioningTemplateInput) {
				in.Version = -1
			},
		},
		{
			name: "provider too long",
			mutate: func(in *persistence.CreateProvisioningTemplateInput) {
				in.Provider = templateString(strings.Repeat("p", 51))
			},
		},
		{
			name: "image reference too long",
			mutate: func(in *persistence.CreateProvisioningTemplateInput) {
				in.ImageReference = templateString(strings.Repeat("i", 256))
			},
		},
		{
			name: "description contains null",
			mutate: func(in *persistence.CreateProvisioningTemplateInput) {
				in.Description = templateString("invalid\x00description")
			},
		},
		{
			name: "missing configuration",
			mutate: func(in *persistence.CreateProvisioningTemplateInput) {
				in.Configuration = nil
			},
		},
		{
			name: "invalid JSON",
			mutate: func(in *persistence.CreateProvisioningTemplateInput) {
				in.Configuration = json.RawMessage(`{"invalid":`)
			},
		},
		{
			name: "configuration array",
			mutate: func(in *persistence.CreateProvisioningTemplateInput) {
				in.Configuration = json.RawMessage(`["not","an","object"]`)
			},
		},
		{
			name: "configuration null",
			mutate: func(in *persistence.CreateProvisioningTemplateInput) {
				in.Configuration = json.RawMessage(`null`)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := validProvisioningTemplateInput()
			tt.mutate(&input)

			if err := input.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestUpdateProvisioningTemplateValidation(t *testing.T) {
	t.Run("valid update", func(t *testing.T) {
		input := persistence.UpdateProvisioningTemplateInput{
			Name:          "Updated Template",
			Version:       2,
			Configuration: json.RawMessage(`{"gpu":true}`),
		}
		if err := input.Validate(); err != nil {
			t.Fatalf("expected valid update: %v", err)
		}
	})

	t.Run("zero version rejected", func(t *testing.T) {
		input := persistence.UpdateProvisioningTemplateInput{
			Name:          "Updated Template",
			Version:       0,
			Configuration: json.RawMessage(`{}`),
		}
		if err := input.Validate(); err == nil {
			t.Fatal("expected validation error")
		}
	})

	t.Run("missing configuration", func(t *testing.T) {
		input := persistence.UpdateProvisioningTemplateInput{
			Name:    "Updated Template",
			Version: 2,
		}
		if err := input.Validate(); err == nil {
			t.Fatal("expected validation error")
		}
	})

	t.Run("invalid project ID", func(t *testing.T) {
		input := persistence.UpdateProvisioningTemplateInput{
			ProjectID:     templateString("invalid"),
			Name:          "Updated Template",
			Version:       2,
			Configuration: json.RawMessage(`{}`),
		}
		if err := input.Validate(); err == nil {
			t.Fatal("expected validation error")
		}
	})
}

func TestProvisioningTemplateErrorSentinels(t *testing.T) {
	if persistence.ErrProvisioningTemplateNotFound == nil {
		t.Fatal("ErrProvisioningTemplateNotFound must be defined")
	}
	if persistence.ErrProvisioningTemplateConflict == nil {
		t.Fatal("ErrProvisioningTemplateConflict must be defined")
	}
	if errors.Is(
		persistence.ErrProvisioningTemplateNotFound,
		persistence.ErrProvisioningTemplateConflict,
	) {
		t.Fatal("not-found and conflict errors must be distinct")
	}
}

func TestProvisioningTemplateRepositoryNilDatabase(t *testing.T) {
	repo, err := persistence.NewProvisioningTemplateRepository(nil)
	if err == nil {
		t.Fatal("expected error for nil database")
	}
	if repo != nil {
		t.Fatal("expected nil repository")
	}
}
