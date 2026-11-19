package persistence_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/gdiop88/RL_trader_infrastructure/internal/persistence"
)

const validComputeProviderID = "550e8400-e29b-41d4-a716-446655440000"

func computeInt32(value int32) *int32 {
	return &value
}

func computeString(value string) *string {
	return &value
}

func validComputeProfileInput() persistence.CreateComputeProfileInput {
	return persistence.CreateComputeProfileInput{
		ProviderID:   validComputeProviderID,
		Name:         "General Purpose",
		ProviderSize: "s-2vcpu-4gb",
		VCPUCount:    computeInt32(2),
		MemoryMB:     computeInt32(4096),
		DiskGB:       computeInt32(80),
		GPUCount:     computeInt32(0),
		Architecture: computeString("x86_64"),
		HourlyPrice:  computeString("0.029760"),
		Currency:     computeString("USD"),
	}
}

func TestCreateComputeProfileValidation(t *testing.T) {
	t.Run("valid profile", func(t *testing.T) {
		input := validComputeProfileInput()
		if err := input.Validate(); err != nil {
			t.Fatalf("expected valid profile: %v", err)
		}
	})

	t.Run("minimal profile", func(t *testing.T) {
		input := persistence.CreateComputeProfileInput{
			ProviderID:   validComputeProviderID,
			Name:         "Minimal",
			ProviderSize: "small",
		}
		if err := input.Validate(); err != nil {
			t.Fatalf("expected valid minimal profile: %v", err)
		}
	})

	tests := []struct {
		name   string
		mutate func(*persistence.CreateComputeProfileInput)
	}{
		{
			name: "invalid provider ID",
			mutate: func(in *persistence.CreateComputeProfileInput) {
				in.ProviderID = "invalid"
			},
		},
		{
			name: "missing name",
			mutate: func(in *persistence.CreateComputeProfileInput) {
				in.Name = " "
			},
		},
		{
			name: "name too long",
			mutate: func(in *persistence.CreateComputeProfileInput) {
				in.Name = strings.Repeat("a", 151)
			},
		},
		{
			name: "missing provider size",
			mutate: func(in *persistence.CreateComputeProfileInput) {
				in.ProviderSize = ""
			},
		},
		{
			name: "provider size too long",
			mutate: func(in *persistence.CreateComputeProfileInput) {
				in.ProviderSize = strings.Repeat("x", 151)
			},
		},
		{
			name: "negative CPUs",
			mutate: func(in *persistence.CreateComputeProfileInput) {
				in.VCPUCount = computeInt32(-1)
			},
		},
		{
			name: "negative memory",
			mutate: func(in *persistence.CreateComputeProfileInput) {
				in.MemoryMB = computeInt32(-1)
			},
		},
		{
			name: "negative disk",
			mutate: func(in *persistence.CreateComputeProfileInput) {
				in.DiskGB = computeInt32(-1)
			},
		},
		{
			name: "negative GPU count",
			mutate: func(in *persistence.CreateComputeProfileInput) {
				in.GPUCount = computeInt32(-1)
			},
		},
		{
			name: "GPU model too long",
			mutate: func(in *persistence.CreateComputeProfileInput) {
				in.GPUModel = computeString(strings.Repeat("g", 151))
			},
		},
		{
			name: "architecture too long",
			mutate: func(in *persistence.CreateComputeProfileInput) {
				in.Architecture = computeString(strings.Repeat("a", 51))
			},
		},
		{
			name: "currency too long",
			mutate: func(in *persistence.CreateComputeProfileInput) {
				in.Currency = computeString(strings.Repeat("c", 11))
			},
		},
		{
			name: "negative price",
			mutate: func(in *persistence.CreateComputeProfileInput) {
				in.HourlyPrice = computeString("-0.10")
			},
		},
		{
			name: "price exceeds decimal scale",
			mutate: func(in *persistence.CreateComputeProfileInput) {
				in.HourlyPrice = computeString("0.1234567")
			},
		},
		{
			name: "price exceeds precision",
			mutate: func(in *persistence.CreateComputeProfileInput) {
				in.HourlyPrice = computeString("1234567890123.00")
			},
		},
		{
			name: "invalid price format",
			mutate: func(in *persistence.CreateComputeProfileInput) {
				in.HourlyPrice = computeString("not-a-price")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := validComputeProfileInput()
			tt.mutate(&input)

			if err := input.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestUpdateComputeProfileValidation(t *testing.T) {
	t.Run("valid update", func(t *testing.T) {
		input := persistence.UpdateComputeProfileInput{
			Name:        "GPU Optimized",
			VCPUCount:   computeInt32(16),
			MemoryMB:    computeInt32(65536),
			GPUCount:    computeInt32(2),
			GPUModel:    computeString("NVIDIA H100"),
			HourlyPrice: computeString("3.500000"),
			Currency:    computeString("USD"),
		}

		if err := input.Validate(); err != nil {
			t.Fatalf("expected valid update: %v", err)
		}
	})

	t.Run("missing name", func(t *testing.T) {
		input := persistence.UpdateComputeProfileInput{
			Name: " ",
		}
		if err := input.Validate(); err == nil {
			t.Fatal("expected validation error")
		}
	})

	t.Run("negative GPU count", func(t *testing.T) {
		input := persistence.UpdateComputeProfileInput{
			Name:     "Invalid GPU Profile",
			GPUCount: computeInt32(-1),
		}
		if err := input.Validate(); err == nil {
			t.Fatal("expected validation error")
		}
	})
}

func TestComputeProfileErrors(t *testing.T) {
	if persistence.ErrComputeProfileNotFound == nil {
		t.Fatal("ErrComputeProfileNotFound must be defined")
	}
	if persistence.ErrComputeProfileConflict == nil {
		t.Fatal("ErrComputeProfileConflict must be defined")
	}
	if errors.Is(
		persistence.ErrComputeProfileNotFound,
		persistence.ErrComputeProfileConflict,
	) {
		t.Fatal("not-found and conflict errors must be distinct")
	}
}

func TestComputeProfileRepositoryNilDatabase(t *testing.T) {
	repo, err := persistence.NewComputeProfileRepository(nil)
	if err == nil {
		t.Fatal("expected error for nil database")
	}
	if repo != nil {
		t.Fatal("expected nil repository")
	}
}
