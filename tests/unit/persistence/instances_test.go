package persistence_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/gdiop88/RL_trader_infrastructure/internal/persistence"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestInstanceStatusValid(t *testing.T) {
	valid := []persistence.InstanceStatus{
		persistence.InstancePending,
		persistence.InstanceProvisioning,
		persistence.InstanceRunning,
		persistence.InstanceStopped,
		persistence.InstanceFailed,
		persistence.InstanceDeleting,
		persistence.InstanceDeleted,
	}

	for _, status := range valid {
		if !status.Valid() {
			t.Errorf("expected %q to be valid", status)
		}
	}

	invalid := []persistence.InstanceStatus{
		"",
		"unknown",
		"terminated",
		"RUNNING",
	}

	for _, status := range invalid {
		if status.Valid() {
			t.Errorf("expected %q to be invalid", status)
		}
	}
}

func TestCreateInstanceInputValidation(t *testing.T) {
	tests := []struct {
		name    string
		input   persistence.CreateInstanceInput
		wantErr bool
	}{
		{
			name: "valid input",
			input: persistence.CreateInstanceInput{
				Provider: "digitalocean",
				Name:     "agent-node-001",
			},
		},
		{
			name: "valid metadata",
			input: persistence.CreateInstanceInput{
				Provider: "digitalocean",
				Name:     "agent-node-001",
				Metadata: json.RawMessage(`{"role":"worker"}`),
			},
		},
		{
			name: "missing provider",
			input: persistence.CreateInstanceInput{
				Name: "agent-node-001",
			},
			wantErr: true,
		},
		{
			name: "missing name",
			input: persistence.CreateInstanceInput{
				Provider: "digitalocean",
			},
			wantErr: true,
		},
		{
			name: "invalid JSON",
			input: persistence.CreateInstanceInput{
				Provider: "digitalocean",
				Name:     "agent-node-001",
				Metadata: json.RawMessage(`{invalid}`),
			},
			wantErr: true,
		},
		{
			name: "metadata array",
			input: persistence.CreateInstanceInput{
				Provider: "digitalocean",
				Name:     "agent-node-001",
				Metadata: json.RawMessage(`[]`),
			},
			wantErr: true,
		},
		{
			name: "metadata null",
			input: persistence.CreateInstanceInput{
				Provider: "digitalocean",
				Name:     "agent-node-001",
				Metadata: json.RawMessage(`null`),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.input.Validate()

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

func TestInstanceRepositoryConstructor(t *testing.T) {
	_, err := persistence.NewInstanceRepository(nil)
	if err == nil {
		t.Fatal("expected error for nil database")
	}
}

func TestInstanceErrorTypes(t *testing.T) {
	if !errors.Is(
		persistence.ErrInstanceNotFound,
		persistence.ErrInstanceNotFound,
	) {
		t.Fatal("not-found sentinel should match itself")
	}

	if errors.Is(
		persistence.ErrInstanceConflict,
		persistence.ErrInstanceNotFound,
	) {
		t.Fatal("conflict and not-found must be distinct")
	}
}

func TestPgxDependenciesAvailable(t *testing.T) {
	var row pgx.Row
	var pool *pgxpool.Pool

	if row != nil || pool != nil {
		t.Fatal("unexpected initialized database dependency")
	}
}
