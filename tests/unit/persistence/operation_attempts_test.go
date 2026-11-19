package persistence_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gdiop88/RL_trader_infrastructure/internal/persistence"
)

func attemptString(value string) *string {
	return &value
}

func attemptTime(value time.Time) *time.Time {
	return &value
}

func validOperationAttemptInput() persistence.CreateOperationAttemptInput {
	return persistence.CreateOperationAttemptInput{
		OperationID:   "11111111-1111-4111-8111-111111111111",
		AttemptNumber: 1,
		Status:        "running",
		WorkerReference: attemptString(
			"worker-001",
		),
		StartedAt: attemptTime(time.Now().UTC()),
		ResponseMetadata: json.RawMessage(
			`{"worker":"worker-001","retry":false}`,
		),
	}
}

func TestCreateOperationAttemptValidation(t *testing.T) {
	t.Run("valid attempt", func(t *testing.T) {
		input := validOperationAttemptInput()
		if err := input.Validate(); err != nil {
			t.Fatalf("expected valid attempt: %v", err)
		}
	})

	t.Run("minimal attempt", func(t *testing.T) {
		input := persistence.CreateOperationAttemptInput{
			OperationID:   "11111111-1111-4111-8111-111111111111",
			AttemptNumber: 1,
			Status:        "pending",
		}

		if err := input.Validate(); err != nil {
			t.Fatalf("expected valid minimal attempt: %v", err)
		}
	})

	t.Run("custom status allowed", func(t *testing.T) {
		input := validOperationAttemptInput()
		input.Status = "waiting_for_worker"

		if err := input.Validate(); err != nil {
			t.Fatalf("custom status rejected: %v", err)
		}
	})

	t.Run("JSON values allowed", func(t *testing.T) {
		values := []string{
			`{}`,
			`[]`,
			`null`,
			`"completed"`,
			`42`,
			`true`,
		}

		for _, value := range values {
			t.Run(value, func(t *testing.T) {
				input := validOperationAttemptInput()
				input.ResponseMetadata = json.RawMessage(value)

				if err := input.Validate(); err != nil {
					t.Fatalf("valid JSON rejected: %v", err)
				}
			})
		}
	})

	t.Run("long error message allowed", func(t *testing.T) {
		input := validOperationAttemptInput()
		input.ErrorMessage = attemptString(
			strings.Repeat("failure detail ", 1000),
		)

		if err := input.Validate(); err != nil {
			t.Fatalf("long TEXT message rejected: %v", err)
		}
	})

	tests := []struct {
		name   string
		mutate func(*persistence.CreateOperationAttemptInput)
	}{
		{
			name: "missing operation ID",
			mutate: func(in *persistence.CreateOperationAttemptInput) {
				in.OperationID = ""
			},
		},
		{
			name: "invalid operation ID",
			mutate: func(in *persistence.CreateOperationAttemptInput) {
				in.OperationID = "invalid-uuid"
			},
		},
		{
			name: "zero attempt number",
			mutate: func(in *persistence.CreateOperationAttemptInput) {
				in.AttemptNumber = 0
			},
		},
		{
			name: "negative attempt number",
			mutate: func(in *persistence.CreateOperationAttemptInput) {
				in.AttemptNumber = -1
			},
		},
		{
			name: "missing status",
			mutate: func(in *persistence.CreateOperationAttemptInput) {
				in.Status = ""
			},
		},
		{
			name: "blank status",
			mutate: func(in *persistence.CreateOperationAttemptInput) {
				in.Status = "   "
			},
		},
		{
			name: "status too long",
			mutate: func(in *persistence.CreateOperationAttemptInput) {
				in.Status = strings.Repeat("s", 51)
			},
		},
		{
			name: "status contains null",
			mutate: func(in *persistence.CreateOperationAttemptInput) {
				in.Status = "running\x00"
			},
		},
		{
			name: "worker reference too long",
			mutate: func(in *persistence.CreateOperationAttemptInput) {
				in.WorkerReference = attemptString(
					strings.Repeat("w", 256),
				)
			},
		},
		{
			name: "worker reference contains null",
			mutate: func(in *persistence.CreateOperationAttemptInput) {
				in.WorkerReference = attemptString("worker\x00")
			},
		},
		{
			name: "error code too long",
			mutate: func(in *persistence.CreateOperationAttemptInput) {
				in.ErrorCode = attemptString(
					strings.Repeat("e", 101),
				)
			},
		},
		{
			name: "error code contains null",
			mutate: func(in *persistence.CreateOperationAttemptInput) {
				in.ErrorCode = attemptString("ERR\x00")
			},
		},
		{
			name: "error message contains null",
			mutate: func(in *persistence.CreateOperationAttemptInput) {
				in.ErrorMessage = attemptString("failed\x00")
			},
		},
		{
			name: "zero started at",
			mutate: func(in *persistence.CreateOperationAttemptInput) {
				in.StartedAt = attemptTime(time.Time{})
			},
		},
		{
			name: "zero finished at",
			mutate: func(in *persistence.CreateOperationAttemptInput) {
				in.FinishedAt = attemptTime(time.Time{})
			},
		},
		{
			name: "invalid JSON",
			mutate: func(in *persistence.CreateOperationAttemptInput) {
				in.ResponseMetadata = json.RawMessage(`{"broken":`)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := validOperationAttemptInput()
			tt.mutate(&input)

			if err := input.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestUpdateOperationAttemptValidation(t *testing.T) {
	t.Run("valid update", func(t *testing.T) {
		started := time.Now().UTC()
		finished := started.Add(2 * time.Second)

		input := persistence.UpdateOperationAttemptInput{
			Status:       "completed",
			StartedAt:    &started,
			FinishedAt:   &finished,
			ErrorCode:    nil,
			ErrorMessage: nil,
			ResponseMetadata: json.RawMessage(
				`{"instance_id":"droplet-123"}`,
			),
		}

		if err := input.Validate(); err != nil {
			t.Fatalf("valid update rejected: %v", err)
		}
	})

	t.Run("missing status", func(t *testing.T) {
		input := persistence.UpdateOperationAttemptInput{}

		if err := input.Validate(); err == nil {
			t.Fatal("expected status validation error")
		}
	})

	t.Run("invalid metadata", func(t *testing.T) {
		input := persistence.UpdateOperationAttemptInput{
			Status:           "failed",
			ResponseMetadata: json.RawMessage(`{invalid}`),
		}

		if err := input.Validate(); err == nil {
			t.Fatal("expected JSON validation error")
		}
	})
}

func TestOperationAttemptErrorSentinels(t *testing.T) {
	if persistence.ErrOperationAttemptNotFound == nil {
		t.Fatal("ErrOperationAttemptNotFound is nil")
	}

	if persistence.ErrOperationAttemptConflict == nil {
		t.Fatal("ErrOperationAttemptConflict is nil")
	}

	if errors.Is(
		persistence.ErrOperationAttemptNotFound,
		persistence.ErrOperationAttemptConflict,
	) {
		t.Fatal("not-found and conflict errors must differ")
	}
}

func TestOperationAttemptRepositoryNilDatabase(t *testing.T) {
	repo, err := persistence.NewOperationAttemptRepository(nil)

	if err == nil {
		t.Fatal("expected constructor error")
	}

	if repo != nil {
		t.Fatal("expected nil repository")
	}
}
