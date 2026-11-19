package persistence_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gdiop88/RL_trader_infrastructure/internal/persistence"
)

const reconciliationTestInstanceID = "11111111-1111-4111-8111-111111111111"

func reconciliationTestString(s string) *string {
	return &s
}

func reconciliationTestBool(b bool) *bool {
	return &b
}

func reconciliationTestTime(tm time.Time) *time.Time {
	return &tm
}

func TestCreateInfrastructureReconciliationValidation(t *testing.T) {
	now := time.Now().UTC()

	valid := func() persistence.CreateInfrastructureReconciliationInput {
		return persistence.CreateInfrastructureReconciliationInput{
			InstanceID: reconciliationTestInstanceID,
		}
	}

	tests := []struct {
		name    string
		modify  func(*persistence.CreateInfrastructureReconciliationInput)
		wantErr bool
	}{
		{name: "minimum valid"},
		{
			name: "all fields",
			modify: func(in *persistence.CreateInfrastructureReconciliationInput) {
				in.DesiredState = reconciliationTestString("running")
				in.ObservedState = reconciliationTestString("stopped")
				in.DriftDetected = reconciliationTestBool(true)
				in.DriftDetails = json.RawMessage(`{"reason":"state mismatch"}`)
				in.Status = reconciliationTestString("pending")
				in.CheckedAt = reconciliationTestTime(now)
				in.ResolvedAt = reconciliationTestTime(now.Add(time.Minute))
			},
		},
		{
			name: "false drift",
			modify: func(in *persistence.CreateInfrastructureReconciliationInput) {
				in.DriftDetected = reconciliationTestBool(false)
			},
		},
		{
			name: "nullable fields",
			modify: func(in *persistence.CreateInfrastructureReconciliationInput) {
				in.DesiredState = nil
				in.ObservedState = nil
				in.DriftDetected = nil
				in.DriftDetails = nil
				in.Status = nil
				in.CheckedAt = nil
				in.ResolvedAt = nil
			},
		},
		{
			name: "maximum state lengths",
			modify: func(in *persistence.CreateInfrastructureReconciliationInput) {
				in.DesiredState = reconciliationTestString(strings.Repeat("a", 50))
				in.ObservedState = reconciliationTestString(strings.Repeat("b", 50))
				in.Status = reconciliationTestString(strings.Repeat("c", 50))
			},
		},
		{
			name: "empty strings permitted",
			modify: func(in *persistence.CreateInfrastructureReconciliationInput) {
				in.DesiredState = reconciliationTestString("")
				in.ObservedState = reconciliationTestString("")
				in.Status = reconciliationTestString("")
			},
		},
		{
			name: "arbitrary status permitted",
			modify: func(in *persistence.CreateInfrastructureReconciliationInput) {
				in.Status = reconciliationTestString("custom_status")
			},
		},
		{
			name: "valid JSON array",
			modify: func(in *persistence.CreateInfrastructureReconciliationInput) {
				in.DriftDetails = json.RawMessage(`[1,2,3]`)
			},
		},
		{
			name: "valid JSON scalar",
			modify: func(in *persistence.CreateInfrastructureReconciliationInput) {
				in.DriftDetails = json.RawMessage(`true`)
			},
		},
		{
			name: "missing instance",
			modify: func(in *persistence.CreateInfrastructureReconciliationInput) {
				in.InstanceID = ""
			},
			wantErr: true,
		},
		{
			name: "invalid instance UUID",
			modify: func(in *persistence.CreateInfrastructureReconciliationInput) {
				in.InstanceID = "invalid"
			},
			wantErr: true,
		},
		{
			name: "desired state too long",
			modify: func(in *persistence.CreateInfrastructureReconciliationInput) {
				in.DesiredState = reconciliationTestString(strings.Repeat("a", 51))
			},
			wantErr: true,
		},
		{
			name: "observed state too long",
			modify: func(in *persistence.CreateInfrastructureReconciliationInput) {
				in.ObservedState = reconciliationTestString(strings.Repeat("b", 51))
			},
			wantErr: true,
		},
		{
			name: "status too long",
			modify: func(in *persistence.CreateInfrastructureReconciliationInput) {
				in.Status = reconciliationTestString(strings.Repeat("c", 51))
			},
			wantErr: true,
		},
		{
			name: "invalid JSON",
			modify: func(in *persistence.CreateInfrastructureReconciliationInput) {
				in.DriftDetails = json.RawMessage(`{"broken":`)
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

func TestUpdateInfrastructureReconciliationValidation(t *testing.T) {
	tests := []struct {
		name    string
		input   persistence.UpdateInfrastructureReconciliationInput
		wantErr bool
	}{
		{
			name:  "minimum valid",
			input: persistence.UpdateInfrastructureReconciliationInput{},
		},
		{
			name: "complete update",
			input: persistence.UpdateInfrastructureReconciliationInput{
				DesiredState:  reconciliationTestString("running"),
				ObservedState: reconciliationTestString("running"),
				DriftDetected: reconciliationTestBool(false),
				DriftDetails:  json.RawMessage(`{"resolved":true}`),
				Status:        reconciliationTestString("resolved"),
			},
		},
		{
			name: "desired state too long",
			input: persistence.UpdateInfrastructureReconciliationInput{
				DesiredState: reconciliationTestString(strings.Repeat("a", 51)),
			},
			wantErr: true,
		},
		{
			name: "observed state too long",
			input: persistence.UpdateInfrastructureReconciliationInput{
				ObservedState: reconciliationTestString(strings.Repeat("b", 51)),
			},
			wantErr: true,
		},
		{
			name: "status too long",
			input: persistence.UpdateInfrastructureReconciliationInput{
				Status: reconciliationTestString(strings.Repeat("c", 51)),
			},
			wantErr: true,
		},
		{
			name: "invalid JSON",
			input: persistence.UpdateInfrastructureReconciliationInput{
				DriftDetails: json.RawMessage(`{invalid}`),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.input.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestListInfrastructureReconciliationsValidation(t *testing.T) {
	now := time.Now().UTC()
	earlier := now.Add(-time.Hour)
	later := now.Add(time.Hour)
	zero := time.Time{}

	tests := []struct {
		name    string
		options persistence.ListInfrastructureReconciliationsOptions
		wantErr bool
	}{
		{
			name: "defaults",
		},
		{
			name: "all filters",
			options: persistence.ListInfrastructureReconciliationsOptions{
				InstanceID:    reconciliationTestString(reconciliationTestInstanceID),
				Status:        reconciliationTestString("pending"),
				DriftDetected: reconciliationTestBool(true),
				From:          &earlier,
				To:            &later,
				Limit:         100,
				Offset:        10,
			},
		},
		{
			name: "maximum limit",
			options: persistence.ListInfrastructureReconciliationsOptions{
				Limit: 1000,
			},
		},
		{
			name: "equal timestamps",
			options: persistence.ListInfrastructureReconciliationsOptions{
				From: &now,
				To:   &now,
			},
		},
		{
			name: "invalid instance",
			options: persistence.ListInfrastructureReconciliationsOptions{
				InstanceID: reconciliationTestString("invalid"),
			},
			wantErr: true,
		},
		{
			name: "status too long",
			options: persistence.ListInfrastructureReconciliationsOptions{
				Status: reconciliationTestString(strings.Repeat("x", 51)),
			},
			wantErr: true,
		},
		{
			name: "zero from",
			options: persistence.ListInfrastructureReconciliationsOptions{
				From: &zero,
			},
			wantErr: true,
		},
		{
			name: "zero to",
			options: persistence.ListInfrastructureReconciliationsOptions{
				To: &zero,
			},
			wantErr: true,
		},
		{
			name: "reversed range",
			options: persistence.ListInfrastructureReconciliationsOptions{
				From: &later,
				To:   &earlier,
			},
			wantErr: true,
		},
		{
			name: "negative limit",
			options: persistence.ListInfrastructureReconciliationsOptions{
				Limit: -1,
			},
			wantErr: true,
		},
		{
			name: "limit above maximum",
			options: persistence.ListInfrastructureReconciliationsOptions{
				Limit: 1001,
			},
			wantErr: true,
		},
		{
			name: "negative offset",
			options: persistence.ListInfrastructureReconciliationsOptions{
				Offset: -1,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.options.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestInfrastructureReconciliationErrorSentinels(t *testing.T) {
	if persistence.ErrInfrastructureReconciliationNotFound == nil {
		t.Fatal("not-found sentinel is nil")
	}
	if persistence.ErrInfrastructureReconciliationConflict == nil {
		t.Fatal("conflict sentinel is nil")
	}
	if errors.Is(
		persistence.ErrInfrastructureReconciliationNotFound,
		persistence.ErrInfrastructureReconciliationConflict,
	) {
		t.Fatal("error sentinels must be distinct")
	}
}

func TestInfrastructureReconciliationRepositoryNilDatabase(t *testing.T) {
	repo, err := persistence.NewInfrastructureReconciliationRepository(nil)
	if err == nil || repo != nil {
		t.Fatalf("expected nil repository and error, got %v, %v", repo, err)
	}
}
