package persistence_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gdiop88/RL_trader_infrastructure/internal/persistence"
)

const usageSampleTestInstanceID = "11111111-1111-4111-8111-111111111111"

func usageSampleTestString(s string) *string {
	return &s
}

func usageSampleTestInt64(n int64) *int64 {
	return &n
}

func TestCreateResourceUsageSampleValidation(t *testing.T) {
	now := time.Now().UTC()

	valid := func() persistence.CreateResourceUsageSampleInput {
		return persistence.CreateResourceUsageSampleInput{
			InstanceID:  usageSampleTestInstanceID,
			CollectedAt: now,
		}
	}

	tests := []struct {
		name    string
		modify  func(*persistence.CreateResourceUsageSampleInput)
		wantErr bool
	}{
		{name: "minimum valid"},
		{
			name: "all metrics",
			modify: func(in *persistence.CreateResourceUsageSampleInput) {
				in.CPUPercent = usageSampleTestString("85.125")
				in.MemoryUsedMB = usageSampleTestInt64(8192)
				in.DiskUsedGB = usageSampleTestString("125.500")
				in.NetworkRXBytes = usageSampleTestInt64(100000)
				in.NetworkTXBytes = usageSampleTestInt64(200000)
				in.GPUPercent = usageSampleTestString("99.999")
				in.Metrics = json.RawMessage(`{"temperature":65}`)
			},
		},
		{
			name: "zero metrics",
			modify: func(in *persistence.CreateResourceUsageSampleInput) {
				in.CPUPercent = usageSampleTestString("0")
				in.MemoryUsedMB = usageSampleTestInt64(0)
				in.DiskUsedGB = usageSampleTestString("0")
				in.NetworkRXBytes = usageSampleTestInt64(0)
				in.NetworkTXBytes = usageSampleTestInt64(0)
				in.GPUPercent = usageSampleTestString("0")
			},
		},
		{
			name: "negative metrics permitted",
			modify: func(in *persistence.CreateResourceUsageSampleInput) {
				in.CPUPercent = usageSampleTestString("-1")
				in.MemoryUsedMB = usageSampleTestInt64(-1)
				in.DiskUsedGB = usageSampleTestString("-1")
				in.NetworkRXBytes = usageSampleTestInt64(-1)
				in.NetworkTXBytes = usageSampleTestInt64(-1)
				in.GPUPercent = usageSampleTestString("-1")
			},
		},
		{
			name: "numeric maximum",
			modify: func(in *persistence.CreateResourceUsageSampleInput) {
				in.CPUPercent = usageSampleTestString("9999.999")
				in.DiskUsedGB = usageSampleTestString("999999999999999.999")
				in.GPUPercent = usageSampleTestString("9999.999")
			},
		},
		{
			name: "numeric rounding allowed",
			modify: func(in *persistence.CreateResourceUsageSampleInput) {
				in.CPUPercent = usageSampleTestString("12.12345")
				in.DiskUsedGB = usageSampleTestString("1.23456")
			},
		},
		{
			name: "valid JSON array",
			modify: func(in *persistence.CreateResourceUsageSampleInput) {
				in.Metrics = json.RawMessage(`[1,2,3]`)
			},
		},
		{
			name: "missing instance",
			modify: func(in *persistence.CreateResourceUsageSampleInput) {
				in.InstanceID = ""
			},
			wantErr: true,
		},
		{
			name: "invalid instance UUID",
			modify: func(in *persistence.CreateResourceUsageSampleInput) {
				in.InstanceID = "invalid"
			},
			wantErr: true,
		},
		{
			name: "zero collected_at",
			modify: func(in *persistence.CreateResourceUsageSampleInput) {
				in.CollectedAt = time.Time{}
			},
			wantErr: true,
		},
		{
			name: "invalid CPU decimal",
			modify: func(in *persistence.CreateResourceUsageSampleInput) {
				in.CPUPercent = usageSampleTestString("invalid")
			},
			wantErr: true,
		},
		{
			name: "CPU overflow",
			modify: func(in *persistence.CreateResourceUsageSampleInput) {
				in.CPUPercent = usageSampleTestString("10000.000")
			},
			wantErr: true,
		},
		{
			name: "CPU rounding overflow",
			modify: func(in *persistence.CreateResourceUsageSampleInput) {
				in.CPUPercent = usageSampleTestString("9999.9995")
			},
			wantErr: true,
		},
		{
			name: "disk overflow",
			modify: func(in *persistence.CreateResourceUsageSampleInput) {
				in.DiskUsedGB = usageSampleTestString("1000000000000000")
			},
			wantErr: true,
		},
		{
			name: "GPU overflow",
			modify: func(in *persistence.CreateResourceUsageSampleInput) {
				in.GPUPercent = usageSampleTestString("10000")
			},
			wantErr: true,
		},
		{
			name: "scientific notation rejected",
			modify: func(in *persistence.CreateResourceUsageSampleInput) {
				in.CPUPercent = usageSampleTestString("1e2")
			},
			wantErr: true,
		},
		{
			name: "decimal whitespace rejected",
			modify: func(in *persistence.CreateResourceUsageSampleInput) {
				in.CPUPercent = usageSampleTestString(" 12.5 ")
			},
			wantErr: true,
		},
		{
			name: "empty decimal rejected",
			modify: func(in *persistence.CreateResourceUsageSampleInput) {
				in.CPUPercent = usageSampleTestString("")
			},
			wantErr: true,
		},
		{
			name: "invalid JSON",
			modify: func(in *persistence.CreateResourceUsageSampleInput) {
				in.Metrics = json.RawMessage(`{"invalid":`)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := valid()
			if tt.modify != nil {
				tt.modify(&input)
			}

			err := input.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestUpdateResourceUsageSampleValidation(t *testing.T) {
	now := time.Now().UTC()

	valid := func() persistence.UpdateResourceUsageSampleInput {
		return persistence.UpdateResourceUsageSampleInput{
			CollectedAt: now,
		}
	}

	tests := []struct {
		name    string
		modify  func(*persistence.UpdateResourceUsageSampleInput)
		wantErr bool
	}{
		{name: "minimum valid"},
		{
			name: "complete update",
			modify: func(in *persistence.UpdateResourceUsageSampleInput) {
				in.CPUPercent = usageSampleTestString("75.250")
				in.MemoryUsedMB = usageSampleTestInt64(4096)
				in.DiskUsedGB = usageSampleTestString("20.125")
				in.NetworkRXBytes = usageSampleTestInt64(12345)
				in.NetworkTXBytes = usageSampleTestInt64(67890)
				in.GPUPercent = usageSampleTestString("50.500")
				in.Metrics = json.RawMessage(`{"updated":true}`)
			},
		},
		{
			name: "null metrics allowed",
			modify: func(in *persistence.UpdateResourceUsageSampleInput) {
				in.CPUPercent = nil
				in.DiskUsedGB = nil
				in.GPUPercent = nil
				in.Metrics = nil
			},
		},
		{
			name: "zero timestamp",
			modify: func(in *persistence.UpdateResourceUsageSampleInput) {
				in.CollectedAt = time.Time{}
			},
			wantErr: true,
		},
		{
			name: "invalid CPU",
			modify: func(in *persistence.UpdateResourceUsageSampleInput) {
				in.CPUPercent = usageSampleTestString("abc")
			},
			wantErr: true,
		},
		{
			name: "CPU overflow",
			modify: func(in *persistence.UpdateResourceUsageSampleInput) {
				in.CPUPercent = usageSampleTestString("10000")
			},
			wantErr: true,
		},
		{
			name: "disk overflow",
			modify: func(in *persistence.UpdateResourceUsageSampleInput) {
				in.DiskUsedGB = usageSampleTestString("1000000000000000")
			},
			wantErr: true,
		},
		{
			name: "GPU overflow",
			modify: func(in *persistence.UpdateResourceUsageSampleInput) {
				in.GPUPercent = usageSampleTestString("10000")
			},
			wantErr: true,
		},
		{
			name: "invalid JSON",
			modify: func(in *persistence.UpdateResourceUsageSampleInput) {
				in.Metrics = json.RawMessage(`{invalid}`)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := valid()
			if tt.modify != nil {
				tt.modify(&input)
			}

			err := input.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestResourceUsageSampleErrorSentinels(t *testing.T) {
	if persistence.ErrResourceUsageSampleNotFound == nil {
		t.Fatal("not-found sentinel is nil")
	}
	if persistence.ErrResourceUsageSampleConflict == nil {
		t.Fatal("conflict sentinel is nil")
	}
	if errors.Is(
		persistence.ErrResourceUsageSampleNotFound,
		persistence.ErrResourceUsageSampleConflict,
	) {
		t.Fatal("sentinels must be distinct")
	}
}

func TestResourceUsageSampleRepositoryNilDatabase(t *testing.T) {
	repo, err := persistence.NewResourceUsageSampleRepository(nil)
	if err == nil || repo != nil {
		t.Fatalf("expected nil repository and error, got %v, %v", repo, err)
	}
}

func TestResourceUsageSampleLongDecimal(t *testing.T) {
	input := persistence.CreateResourceUsageSampleInput{
		InstanceID:  usageSampleTestInstanceID,
		CollectedAt: time.Now(),
		CPUPercent:  usageSampleTestString(strings.Repeat("9", 100)),
	}

	if err := input.Validate(); err == nil {
		t.Fatal("expected precision overflow")
	}
}
