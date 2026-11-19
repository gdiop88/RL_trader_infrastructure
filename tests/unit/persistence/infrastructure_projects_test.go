package persistence_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/gdiop88/RL_trader_infrastructure/internal/persistence"
)

func projectString(value string) *string {
	return &value
}

func validInfrastructureProjectInput() persistence.CreateInfrastructureProjectInput {
	return persistence.CreateInfrastructureProjectInput{
		Name:           "RL Trading Production",
		Environment:    "production",
		Description:    projectString("Production infrastructure"),
		OwnerReference: projectString("rl-trader"),
		BudgetLimit:    projectString("100000.00"),
		Currency:       projectString("USD"),
		Metadata:       []byte(`{"team":"infrastructure","managed":true}`),
	}
}

func TestCreateInfrastructureProjectValidation(t *testing.T) {
	t.Run("valid project", func(t *testing.T) {
		input := validInfrastructureProjectInput()

		if err := input.Validate(); err != nil {
			t.Fatalf("expected valid project: %v", err)
		}
	})

	t.Run("minimal project", func(t *testing.T) {
		input := persistence.CreateInfrastructureProjectInput{
			Name:        "Development",
			Environment: "development",
		}

		if err := input.Validate(); err != nil {
			t.Fatalf("expected valid minimal project: %v", err)
		}
	})

	tests := []struct {
		name   string
		mutate func(*persistence.CreateInfrastructureProjectInput)
	}{
		{
			name: "missing name",
			mutate: func(in *persistence.CreateInfrastructureProjectInput) {
				in.Name = " "
			},
		},
		{
			name: "name too long",
			mutate: func(in *persistence.CreateInfrastructureProjectInput) {
				in.Name = strings.Repeat("n", 151)
			},
		},
		{
			name: "missing environment",
			mutate: func(in *persistence.CreateInfrastructureProjectInput) {
				in.Environment = ""
			},
		},
		{
			name: "environment too long",
			mutate: func(in *persistence.CreateInfrastructureProjectInput) {
				in.Environment = strings.Repeat("e", 51)
			},
		},
		{
			name: "owner reference too long",
			mutate: func(in *persistence.CreateInfrastructureProjectInput) {
				in.OwnerReference = projectString(strings.Repeat("o", 256))
			},
		},
		{
			name: "currency too long",
			mutate: func(in *persistence.CreateInfrastructureProjectInput) {
				in.Currency = projectString(strings.Repeat("c", 11))
			},
		},
		{
			name: "negative budget",
			mutate: func(in *persistence.CreateInfrastructureProjectInput) {
				in.BudgetLimit = projectString("-100.00")
			},
		},
		{
			name: "budget scale exceeded",
			mutate: func(in *persistence.CreateInfrastructureProjectInput) {
				in.BudgetLimit = projectString("100.123")
			},
		},
		{
			name: "budget precision exceeded",
			mutate: func(in *persistence.CreateInfrastructureProjectInput) {
				in.BudgetLimit = projectString("12345678901234567.00")
			},
		},
		{
			name: "invalid budget",
			mutate: func(in *persistence.CreateInfrastructureProjectInput) {
				in.BudgetLimit = projectString("invalid")
			},
		},
		{
			name: "invalid metadata",
			mutate: func(in *persistence.CreateInfrastructureProjectInput) {
				in.Metadata = []byte(`{"invalid":`)
			},
		},
		{
			name: "metadata array",
			mutate: func(in *persistence.CreateInfrastructureProjectInput) {
				in.Metadata = []byte(`["not","an","object"]`)
			},
		},
		{
			name: "metadata null",
			mutate: func(in *persistence.CreateInfrastructureProjectInput) {
				in.Metadata = []byte(`null`)
			},
		},
		{
			name: "description contains null character",
			mutate: func(in *persistence.CreateInfrastructureProjectInput) {
				in.Description = projectString("invalid\x00description")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := validInfrastructureProjectInput()
			tt.mutate(&input)

			if err := input.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestUpdateInfrastructureProjectValidation(t *testing.T) {
	t.Run("valid update", func(t *testing.T) {
		input := persistence.UpdateInfrastructureProjectInput{
			Name:        "RL Trading Staging",
			Environment: "staging",
			BudgetLimit: projectString("50000.50"),
			Currency:    projectString("USD"),
			Metadata:    []byte(`{"updated":true}`),
		}

		if err := input.Validate(); err != nil {
			t.Fatalf("expected valid update: %v", err)
		}
	})

	t.Run("minimal update", func(t *testing.T) {
		input := persistence.UpdateInfrastructureProjectInput{
			Name:        "Updated Project",
			Environment: "development",
		}

		if err := input.Validate(); err != nil {
			t.Fatalf("expected valid minimal update: %v", err)
		}
	})

	t.Run("missing name", func(t *testing.T) {
		input := persistence.UpdateInfrastructureProjectInput{
			Environment: "production",
		}

		if err := input.Validate(); err == nil {
			t.Fatal("expected validation error")
		}
	})

	t.Run("missing environment", func(t *testing.T) {
		input := persistence.UpdateInfrastructureProjectInput{
			Name: "Updated Project",
		}

		if err := input.Validate(); err == nil {
			t.Fatal("expected validation error")
		}
	})

	t.Run("negative budget", func(t *testing.T) {
		input := persistence.UpdateInfrastructureProjectInput{
			Name:        "Updated Project",
			Environment: "production",
			BudgetLimit: projectString("-1.00"),
		}

		if err := input.Validate(); err == nil {
			t.Fatal("expected validation error")
		}
	})
}

func TestInfrastructureProjectErrorSentinels(t *testing.T) {
	if persistence.ErrInfrastructureProjectNotFound == nil {
		t.Fatal("ErrInfrastructureProjectNotFound must be defined")
	}

	if persistence.ErrInfrastructureProjectConflict == nil {
		t.Fatal("ErrInfrastructureProjectConflict must be defined")
	}

	if errors.Is(
		persistence.ErrInfrastructureProjectNotFound,
		persistence.ErrInfrastructureProjectConflict,
	) {
		t.Fatal("not-found and conflict errors must be distinct")
	}
}

func TestInfrastructureProjectRepositoryNilDatabase(t *testing.T) {
	repo, err := persistence.NewInfrastructureProjectRepository(nil)

	if err == nil {
		t.Fatal("expected error for nil database")
	}

	if repo != nil {
		t.Fatal("expected nil repository")
	}
}
