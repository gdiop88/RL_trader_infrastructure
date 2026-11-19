package persistence_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gdiop88/RL_trader_infrastructure/internal/persistence"
)

func agentRegistryString(value string) *string {
	return &value
}

func agentRegistryTime(value time.Time) *time.Time {
	return &value
}

func TestCreateAgentRegistryValidation(t *testing.T) {
	now := time.Now().UTC()

	tests := []struct {
		name    string
		input   persistence.CreateAgentRegistryInput
		wantErr bool
	}{
		{
			name: "valid agent",
			input: persistence.CreateAgentRegistryInput{
				AgentReference: "agent-001",
				AgentType:      agentRegistryString("trading"),
				OwnerReference: agentRegistryString("project-001"),
				Status:         agentRegistryString("active"),
				Capabilities: json.RawMessage(
					`{"strategies":["dqn","ppo"],"gpu":false}`,
				),
				RegisteredAt: &now,
				LastSeenAt:   &now,
			},
		},
		{
			name: "minimal agent",
			input: persistence.CreateAgentRegistryInput{
				AgentReference: "agent-minimal",
			},
		},
		{
			name: "custom status allowed",
			input: persistence.CreateAgentRegistryInput{
				AgentReference: "agent-custom",
				Status:         agentRegistryString("awaiting_provisioning"),
			},
		},
		{
			name: "empty optional strings allowed",
			input: persistence.CreateAgentRegistryInput{
				AgentReference: "agent-empty",
				AgentType:      agentRegistryString(""),
				OwnerReference: agentRegistryString(""),
				Status:         agentRegistryString(""),
			},
		},
		{
			name: "maximum reference length",
			input: persistence.CreateAgentRegistryInput{
				AgentReference: strings.Repeat("a", 255),
			},
		},
		{
			name:    "missing reference",
			input:   persistence.CreateAgentRegistryInput{},
			wantErr: true,
		},
		{
			name: "blank reference",
			input: persistence.CreateAgentRegistryInput{
				AgentReference: "   ",
			},
			wantErr: true,
		},
		{
			name: "reference too long",
			input: persistence.CreateAgentRegistryInput{
				AgentReference: strings.Repeat("a", 256),
			},
			wantErr: true,
		},
		{
			name: "reference contains null",
			input: persistence.CreateAgentRegistryInput{
				AgentReference: "agent\x00001",
			},
			wantErr: true,
		},
		{
			name: "agent type too long",
			input: persistence.CreateAgentRegistryInput{
				AgentReference: "agent-001",
				AgentType:      agentRegistryString(strings.Repeat("a", 101)),
			},
			wantErr: true,
		},
		{
			name: "agent type contains null",
			input: persistence.CreateAgentRegistryInput{
				AgentReference: "agent-001",
				AgentType:      agentRegistryString("trading\x00agent"),
			},
			wantErr: true,
		},
		{
			name: "owner reference too long",
			input: persistence.CreateAgentRegistryInput{
				AgentReference: "agent-001",
				OwnerReference: agentRegistryString(strings.Repeat("a", 256)),
			},
			wantErr: true,
		},
		{
			name: "owner reference contains null",
			input: persistence.CreateAgentRegistryInput{
				AgentReference: "agent-001",
				OwnerReference: agentRegistryString("owner\x00001"),
			},
			wantErr: true,
		},
		{
			name: "status too long",
			input: persistence.CreateAgentRegistryInput{
				AgentReference: "agent-001",
				Status:         agentRegistryString(strings.Repeat("a", 51)),
			},
			wantErr: true,
		},
		{
			name: "status contains null",
			input: persistence.CreateAgentRegistryInput{
				AgentReference: "agent-001",
				Status:         agentRegistryString("active\x00"),
			},
			wantErr: true,
		},
		{
			name: "invalid JSON",
			input: persistence.CreateAgentRegistryInput{
				AgentReference: "agent-001",
				Capabilities:   json.RawMessage(`{"invalid":`),
			},
			wantErr: true,
		},
		{
			name: "zero registered at",
			input: persistence.CreateAgentRegistryInput{
				AgentReference: "agent-001",
				RegisteredAt:   agentRegistryTime(time.Time{}),
			},
			wantErr: true,
		},
		{
			name: "zero last seen at",
			input: persistence.CreateAgentRegistryInput{
				AgentReference: "agent-001",
				LastSeenAt:     agentRegistryTime(time.Time{}),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.input.Validate()

			if tt.wantErr && err == nil {
				t.Fatal("expected validation error")
			}

			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected validation error: %v", err)
			}
		})
	}

	t.Run("JSON values allowed", func(t *testing.T) {
		values := []string{
			`{}`,
			`[]`,
			`null`,
			`"gpu"`,
			`42`,
			`true`,
		}

		for _, value := range values {
			t.Run(value, func(t *testing.T) {
				input := persistence.CreateAgentRegistryInput{
					AgentReference: "agent-json",
					Capabilities:   json.RawMessage(value),
				}

				if err := input.Validate(); err != nil {
					t.Fatalf("valid JSON rejected: %v", err)
				}
			})
		}
	})
}

func TestUpdateAgentRegistryValidation(t *testing.T) {
	now := time.Now().UTC()

	tests := []struct {
		name    string
		input   persistence.UpdateAgentRegistryInput
		wantErr bool
	}{
		{
			name: "valid update",
			input: persistence.UpdateAgentRegistryInput{
				AgentType:      agentRegistryString("prediction"),
				OwnerReference: agentRegistryString("owner-001"),
				Status:         agentRegistryString("running"),
				Capabilities:   json.RawMessage(`{"cpu":4}`),
				RegisteredAt:   &now,
				LastSeenAt:     &now,
			},
		},
		{
			name:  "clear nullable fields",
			input: persistence.UpdateAgentRegistryInput{},
		},
		{
			name: "custom status",
			input: persistence.UpdateAgentRegistryInput{
				Status: agentRegistryString("provisioning"),
			},
		},
		{
			name: "invalid status length",
			input: persistence.UpdateAgentRegistryInput{
				Status: agentRegistryString(strings.Repeat("s", 51)),
			},
			wantErr: true,
		},
		{
			name: "invalid status null character",
			input: persistence.UpdateAgentRegistryInput{
				Status: agentRegistryString("running\x00"),
			},
			wantErr: true,
		},
		{
			name: "invalid capabilities",
			input: persistence.UpdateAgentRegistryInput{
				Capabilities: json.RawMessage(`not-json`),
			},
			wantErr: true,
		},
		{
			name: "invalid registered timestamp",
			input: persistence.UpdateAgentRegistryInput{
				RegisteredAt: agentRegistryTime(time.Time{}),
			},
			wantErr: true,
		},
		{
			name: "invalid last seen timestamp",
			input: persistence.UpdateAgentRegistryInput{
				LastSeenAt: agentRegistryTime(time.Time{}),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.input.Validate()

			if tt.wantErr && err == nil {
				t.Fatal("expected validation error")
			}

			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected validation error: %v", err)
			}
		})
	}
}

func TestAgentRegistryErrorSentinels(t *testing.T) {
	if persistence.ErrAgentRegistryNotFound == nil {
		t.Fatal("not-found sentinel is nil")
	}

	if persistence.ErrAgentRegistryConflict == nil {
		t.Fatal("conflict sentinel is nil")
	}

	if errors.Is(
		persistence.ErrAgentRegistryNotFound,
		persistence.ErrAgentRegistryConflict,
	) {
		t.Fatal("error sentinels must be distinct")
	}
}

func TestAgentRegistryRepositoryNilDatabase(t *testing.T) {
	repo, err := persistence.NewAgentRegistryRepository(nil)

	if err == nil {
		t.Fatal("expected error for nil database")
	}

	if repo != nil {
		t.Fatal("expected nil repository")
	}
}
