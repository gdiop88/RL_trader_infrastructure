package persistence_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gdiop88/RL_trader_infrastructure/internal/persistence"
)

const (
	testAssignmentAgentID    = "11111111-1111-4111-8111-111111111111"
	testAssignmentInstanceID = "22222222-2222-4222-8222-222222222222"
)

func assignmentTestString(value string) *string {
	return &value
}

func assignmentTestTime(value time.Time) *time.Time {
	return &value
}

func TestCreateAgentInstanceAssignmentValidation(t *testing.T) {
	now := time.Now().UTC()

	base := func() persistence.CreateAgentInstanceAssignmentInput {
		return persistence.CreateAgentInstanceAssignmentInput{
			AgentID:    testAssignmentAgentID,
			InstanceID: testAssignmentInstanceID,
			Status:     "assigned",
		}
	}

	tests := []struct {
		name    string
		modify  func(*persistence.CreateAgentInstanceAssignmentInput)
		wantErr bool
	}{
		{
			name:   "valid minimal assignment",
			modify: func(in *persistence.CreateAgentInstanceAssignmentInput) {},
		},
		{
			name: "valid complete assignment",
			modify: func(in *persistence.CreateAgentInstanceAssignmentInput) {
				in.AssignmentRole = assignmentTestString("primary")
				in.AssignedAt = &now
				in.ReleasedAt = assignmentTestTime(now.Add(time.Hour))
				in.Metadata = json.RawMessage(`{"priority":1,"gpu":false}`)
			},
		},
		{
			name: "custom status allowed",
			modify: func(in *persistence.CreateAgentInstanceAssignmentInput) {
				in.Status = "awaiting_provisioning"
			},
		},
		{
			name: "empty optional role allowed",
			modify: func(in *persistence.CreateAgentInstanceAssignmentInput) {
				in.AssignmentRole = assignmentTestString("")
			},
		},
		{
			name: "maximum role length",
			modify: func(in *persistence.CreateAgentInstanceAssignmentInput) {
				in.AssignmentRole = assignmentTestString(strings.Repeat("r", 100))
			},
		},
		{
			name: "missing agent ID",
			modify: func(in *persistence.CreateAgentInstanceAssignmentInput) {
				in.AgentID = ""
			},
			wantErr: true,
		},
		{
			name: "invalid agent ID",
			modify: func(in *persistence.CreateAgentInstanceAssignmentInput) {
				in.AgentID = "invalid"
			},
			wantErr: true,
		},
		{
			name: "missing instance ID",
			modify: func(in *persistence.CreateAgentInstanceAssignmentInput) {
				in.InstanceID = ""
			},
			wantErr: true,
		},
		{
			name: "invalid instance ID",
			modify: func(in *persistence.CreateAgentInstanceAssignmentInput) {
				in.InstanceID = "invalid"
			},
			wantErr: true,
		},
		{
			name: "missing status",
			modify: func(in *persistence.CreateAgentInstanceAssignmentInput) {
				in.Status = ""
			},
			wantErr: true,
		},
		{
			name: "blank status",
			modify: func(in *persistence.CreateAgentInstanceAssignmentInput) {
				in.Status = "   "
			},
			wantErr: true,
		},
		{
			name: "status too long",
			modify: func(in *persistence.CreateAgentInstanceAssignmentInput) {
				in.Status = strings.Repeat("s", 51)
			},
			wantErr: true,
		},
		{
			name: "status contains null",
			modify: func(in *persistence.CreateAgentInstanceAssignmentInput) {
				in.Status = "assigned\x00"
			},
			wantErr: true,
		},
		{
			name: "role too long",
			modify: func(in *persistence.CreateAgentInstanceAssignmentInput) {
				in.AssignmentRole = assignmentTestString(strings.Repeat("r", 101))
			},
			wantErr: true,
		},
		{
			name: "role contains null",
			modify: func(in *persistence.CreateAgentInstanceAssignmentInput) {
				in.AssignmentRole = assignmentTestString("primary\x00")
			},
			wantErr: true,
		},
		{
			name: "zero assigned timestamp",
			modify: func(in *persistence.CreateAgentInstanceAssignmentInput) {
				in.AssignedAt = assignmentTestTime(time.Time{})
			},
			wantErr: true,
		},
		{
			name: "zero released timestamp",
			modify: func(in *persistence.CreateAgentInstanceAssignmentInput) {
				in.ReleasedAt = assignmentTestTime(time.Time{})
			},
			wantErr: true,
		},
		{
			name: "invalid metadata",
			modify: func(in *persistence.CreateAgentInstanceAssignmentInput) {
				in.Metadata = json.RawMessage(`{"invalid":`)
			},
			wantErr: true,
		},
		{
			name: "released before assigned allowed by schema",
			modify: func(in *persistence.CreateAgentInstanceAssignmentInput) {
				in.AssignedAt = assignmentTestTime(now)
				in.ReleasedAt = assignmentTestTime(now.Add(-time.Hour))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := base()
			tt.modify(&input)

			err := input.Validate()

			if tt.wantErr && err == nil {
				t.Fatal("expected validation error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected validation error: %v", err)
			}
		})
	}

	t.Run("all JSONB value types", func(t *testing.T) {
		values := []string{
			`{}`,
			`[]`,
			`null`,
			`"worker"`,
			`42`,
			`true`,
		}

		for _, value := range values {
			t.Run(value, func(t *testing.T) {
				input := base()
				input.Metadata = json.RawMessage(value)

				if err := input.Validate(); err != nil {
					t.Fatalf("valid JSON rejected: %v", err)
				}
			})
		}
	})
}

func TestUpdateAgentInstanceAssignmentValidation(t *testing.T) {
	now := time.Now().UTC()

	tests := []struct {
		name    string
		input   persistence.UpdateAgentInstanceAssignmentInput
		wantErr bool
	}{
		{
			name: "valid update",
			input: persistence.UpdateAgentInstanceAssignmentInput{
				AssignmentRole: assignmentTestString("secondary"),
				Status:         "active",
				AssignedAt:     &now,
				ReleasedAt:     assignmentTestTime(now.Add(time.Minute)),
				Metadata:       json.RawMessage(`{"slot":2}`),
			},
		},
		{
			name: "minimal update",
			input: persistence.UpdateAgentInstanceAssignmentInput{
				Status: "assigned",
			},
		},
		{
			name: "custom status",
			input: persistence.UpdateAgentInstanceAssignmentInput{
				Status: "migrating",
			},
		},
		{
			name:    "missing status",
			input:   persistence.UpdateAgentInstanceAssignmentInput{},
			wantErr: true,
		},
		{
			name: "invalid role",
			input: persistence.UpdateAgentInstanceAssignmentInput{
				Status:         "active",
				AssignmentRole: assignmentTestString("worker\x00"),
			},
			wantErr: true,
		},
		{
			name: "invalid status",
			input: persistence.UpdateAgentInstanceAssignmentInput{
				Status: strings.Repeat("s", 51),
			},
			wantErr: true,
		},
		{
			name: "invalid metadata",
			input: persistence.UpdateAgentInstanceAssignmentInput{
				Status:   "active",
				Metadata: json.RawMessage(`invalid-json`),
			},
			wantErr: true,
		},
		{
			name: "zero assigned timestamp",
			input: persistence.UpdateAgentInstanceAssignmentInput{
				Status:     "active",
				AssignedAt: assignmentTestTime(time.Time{}),
			},
			wantErr: true,
		},
		{
			name: "zero released timestamp",
			input: persistence.UpdateAgentInstanceAssignmentInput{
				Status:     "released",
				ReleasedAt: assignmentTestTime(time.Time{}),
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

func TestAgentInstanceAssignmentErrorSentinels(t *testing.T) {
	if persistence.ErrAgentInstanceAssignmentNotFound == nil {
		t.Fatal("not-found sentinel is nil")
	}

	if persistence.ErrAgentInstanceAssignmentConflict == nil {
		t.Fatal("conflict sentinel is nil")
	}

	if errors.Is(
		persistence.ErrAgentInstanceAssignmentNotFound,
		persistence.ErrAgentInstanceAssignmentConflict,
	) {
		t.Fatal("error sentinels must be distinct")
	}
}

func TestAgentInstanceAssignmentRepositoryNilDatabase(t *testing.T) {
	repo, err := persistence.NewAgentInstanceAssignmentRepository(nil)

	if err == nil {
		t.Fatal("expected error for nil database")
	}

	if repo != nil {
		t.Fatal("expected nil repository")
	}
}
