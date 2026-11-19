package persistence_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gdiop88/RL_trader_infrastructure/internal/persistence"
)

const (
	resourceReservationTestProjectID  = "11111111-1111-4111-8111-111111111111"
	resourceReservationTestAgentID    = "22222222-2222-4222-8222-222222222222"
	resourceReservationTestInstanceID = "33333333-3333-4333-8333-333333333333"
)

func resourceReservationTestString(s string) *string {
	return &s
}

func resourceReservationTestInt(n int) *int {
	return &n
}

func resourceReservationTestTime(t time.Time) *time.Time {
	return &t
}

func TestCreateResourceReservationValidation(t *testing.T) {
	now := time.Now().UTC()

	valid := func() persistence.CreateResourceReservationInput {
		return persistence.CreateResourceReservationInput{
			ProjectID:       resourceReservationTestProjectID,
			ReservationType: "compute",
			Status:          "pending",
		}
	}

	tests := []struct {
		name    string
		modify  func(*persistence.CreateResourceReservationInput)
		wantErr bool
	}{
		{
			name: "minimum valid reservation",
		},
		{
			name: "complete reservation",
			modify: func(in *persistence.CreateResourceReservationInput) {
				in.AgentID = resourceReservationTestString(resourceReservationTestAgentID)
				in.InstanceID = resourceReservationTestString(resourceReservationTestInstanceID)
				in.VCPUReserved = resourceReservationTestInt(4)
				in.MemoryMBReserved = resourceReservationTestInt(8192)
				in.GPUReserved = resourceReservationTestInt(1)
				in.StartsAt = resourceReservationTestTime(now)
				in.ExpiresAt = resourceReservationTestTime(now.Add(time.Hour))
			},
		},
		{
			name: "zero resources allowed",
			modify: func(in *persistence.CreateResourceReservationInput) {
				in.VCPUReserved = resourceReservationTestInt(0)
				in.MemoryMBReserved = resourceReservationTestInt(0)
				in.GPUReserved = resourceReservationTestInt(0)
			},
		},
		{
			name: "maximum string lengths",
			modify: func(in *persistence.CreateResourceReservationInput) {
				in.ReservationType = strings.Repeat("r", 50)
				in.Status = strings.Repeat("s", 50)
			},
		},
		{
			name: "reverse timestamp order permitted",
			modify: func(in *persistence.CreateResourceReservationInput) {
				in.StartsAt = resourceReservationTestTime(now)
				in.ExpiresAt = resourceReservationTestTime(now.Add(-time.Hour))
			},
		},
		{
			name: "invalid project UUID",
			modify: func(in *persistence.CreateResourceReservationInput) {
				in.ProjectID = "invalid"
			},
			wantErr: true,
		},
		{
			name: "missing project ID",
			modify: func(in *persistence.CreateResourceReservationInput) {
				in.ProjectID = ""
			},
			wantErr: true,
		},
		{
			name: "invalid agent UUID",
			modify: func(in *persistence.CreateResourceReservationInput) {
				in.AgentID = resourceReservationTestString("invalid")
			},
			wantErr: true,
		},
		{
			name: "invalid instance UUID",
			modify: func(in *persistence.CreateResourceReservationInput) {
				in.InstanceID = resourceReservationTestString("invalid")
			},
			wantErr: true,
		},
		{
			name: "missing reservation type",
			modify: func(in *persistence.CreateResourceReservationInput) {
				in.ReservationType = ""
			},
			wantErr: true,
		},
		{
			name: "reservation type too long",
			modify: func(in *persistence.CreateResourceReservationInput) {
				in.ReservationType = strings.Repeat("r", 51)
			},
			wantErr: true,
		},
		{
			name: "missing status",
			modify: func(in *persistence.CreateResourceReservationInput) {
				in.Status = ""
			},
			wantErr: true,
		},
		{
			name: "status too long",
			modify: func(in *persistence.CreateResourceReservationInput) {
				in.Status = strings.Repeat("s", 51)
			},
			wantErr: true,
		},
		{
			name: "negative vCPU",
			modify: func(in *persistence.CreateResourceReservationInput) {
				in.VCPUReserved = resourceReservationTestInt(-1)
			},
			wantErr: true,
		},
		{
			name: "negative memory",
			modify: func(in *persistence.CreateResourceReservationInput) {
				in.MemoryMBReserved = resourceReservationTestInt(-1)
			},
			wantErr: true,
		},
		{
			name: "negative GPU",
			modify: func(in *persistence.CreateResourceReservationInput) {
				in.GPUReserved = resourceReservationTestInt(-1)
			},
			wantErr: true,
		},
		{
			name: "zero starts_at",
			modify: func(in *persistence.CreateResourceReservationInput) {
				in.StartsAt = resourceReservationTestTime(time.Time{})
			},
			wantErr: true,
		},
		{
			name: "zero expires_at",
			modify: func(in *persistence.CreateResourceReservationInput) {
				in.ExpiresAt = resourceReservationTestTime(time.Time{})
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
				t.Fatalf(
					"Validate() error = %v, wantErr = %v",
					err,
					tt.wantErr,
				)
			}
		})
	}
}

func TestUpdateResourceReservationValidation(t *testing.T) {
	now := time.Now().UTC()

	valid := func() persistence.UpdateResourceReservationInput {
		return persistence.UpdateResourceReservationInput{
			ReservationType: "compute",
			Status:          "active",
		}
	}

	tests := []struct {
		name    string
		modify  func(*persistence.UpdateResourceReservationInput)
		wantErr bool
	}{
		{
			name: "valid update",
		},
		{
			name: "nullable fields omitted",
			modify: func(in *persistence.UpdateResourceReservationInput) {
				in.AgentID = nil
				in.InstanceID = nil
				in.VCPUReserved = nil
				in.MemoryMBReserved = nil
				in.GPUReserved = nil
			},
		},
		{
			name: "zero quantities",
			modify: func(in *persistence.UpdateResourceReservationInput) {
				in.VCPUReserved = resourceReservationTestInt(0)
				in.MemoryMBReserved = resourceReservationTestInt(0)
				in.GPUReserved = resourceReservationTestInt(0)
			},
		},
		{
			name: "reverse timestamps permitted",
			modify: func(in *persistence.UpdateResourceReservationInput) {
				in.StartsAt = resourceReservationTestTime(now)
				in.ExpiresAt = resourceReservationTestTime(now.Add(-time.Hour))
			},
		},
		{
			name: "invalid agent ID",
			modify: func(in *persistence.UpdateResourceReservationInput) {
				in.AgentID = resourceReservationTestString("bad")
			},
			wantErr: true,
		},
		{
			name: "invalid instance ID",
			modify: func(in *persistence.UpdateResourceReservationInput) {
				in.InstanceID = resourceReservationTestString("bad")
			},
			wantErr: true,
		},
		{
			name: "empty reservation type",
			modify: func(in *persistence.UpdateResourceReservationInput) {
				in.ReservationType = ""
			},
			wantErr: true,
		},
		{
			name: "reservation type too long",
			modify: func(in *persistence.UpdateResourceReservationInput) {
				in.ReservationType = strings.Repeat("r", 51)
			},
			wantErr: true,
		},
		{
			name: "empty status",
			modify: func(in *persistence.UpdateResourceReservationInput) {
				in.Status = ""
			},
			wantErr: true,
		},
		{
			name: "status too long",
			modify: func(in *persistence.UpdateResourceReservationInput) {
				in.Status = strings.Repeat("s", 51)
			},
			wantErr: true,
		},
		{
			name: "negative vCPU",
			modify: func(in *persistence.UpdateResourceReservationInput) {
				in.VCPUReserved = resourceReservationTestInt(-1)
			},
			wantErr: true,
		},
		{
			name: "negative memory",
			modify: func(in *persistence.UpdateResourceReservationInput) {
				in.MemoryMBReserved = resourceReservationTestInt(-1)
			},
			wantErr: true,
		},
		{
			name: "negative GPU",
			modify: func(in *persistence.UpdateResourceReservationInput) {
				in.GPUReserved = resourceReservationTestInt(-1)
			},
			wantErr: true,
		},
		{
			name: "zero starts_at",
			modify: func(in *persistence.UpdateResourceReservationInput) {
				in.StartsAt = resourceReservationTestTime(time.Time{})
			},
			wantErr: true,
		},
		{
			name: "zero expires_at",
			modify: func(in *persistence.UpdateResourceReservationInput) {
				in.ExpiresAt = resourceReservationTestTime(time.Time{})
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
				t.Fatalf(
					"Validate() error = %v, wantErr = %v",
					err,
					tt.wantErr,
				)
			}
		})
	}
}

func TestResourceReservationErrorSentinels(t *testing.T) {
	if persistence.ErrResourceReservationNotFound == nil {
		t.Fatal("not-found sentinel is nil")
	}

	if persistence.ErrResourceReservationConflict == nil {
		t.Fatal("conflict sentinel is nil")
	}

	if errors.Is(
		persistence.ErrResourceReservationNotFound,
		persistence.ErrResourceReservationConflict,
	) {
		t.Fatal("error sentinels must be distinct")
	}
}

func TestResourceReservationRepositoryNilDatabase(t *testing.T) {
	repo, err := persistence.NewResourceReservationRepository(nil)
	if err == nil {
		t.Fatal("expected error for nil database")
	}
	if repo != nil {
		t.Fatal("expected nil repository")
	}
}
