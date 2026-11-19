package persistence_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/gdiop88/RL_trader_infrastructure/internal/persistence"
)

func eventTestString(value string) *string {
	return &value
}

func eventTestInt32(value int32) *int32 {
	return &value
}

func validOperationEventInput() persistence.CreateOperationEventInput {
	return persistence.CreateOperationEventInput{
		OperationID: "123e4567-e89b-42d3-a456-426614174000",
		EventType:   "provisioning.started",
		Status:      eventTestString("running"),
		Message:     eventTestString("Provisioning started"),
		EventData: json.RawMessage(
			`{"provider":"digitalocean","region":"nyc3"}`,
		),
		SequenceNumber: eventTestInt32(1),
	}
}

func TestCreateOperationEventValidation(t *testing.T) {
	t.Run("valid event", func(t *testing.T) {
		input := validOperationEventInput()

		if err := input.Validate(); err != nil {
			t.Fatalf("expected valid event: %v", err)
		}
	})

	t.Run("minimal event", func(t *testing.T) {
		input := persistence.CreateOperationEventInput{
			OperationID: "123e4567-e89b-42d3-a456-426614174000",
			EventType:   "created",
		}

		if err := input.Validate(); err != nil {
			t.Fatalf("expected valid minimal event: %v", err)
		}
	})

	t.Run("zero sequence number", func(t *testing.T) {
		input := validOperationEventInput()
		input.SequenceNumber = eventTestInt32(0)

		if err := input.Validate(); err != nil {
			t.Fatalf("zero sequence number rejected: %v", err)
		}
	})

	t.Run("nullable sequence number", func(t *testing.T) {
		input := validOperationEventInput()
		input.SequenceNumber = nil

		if err := input.Validate(); err != nil {
			t.Fatalf("nil sequence number rejected: %v", err)
		}
	})

	t.Run("custom status allowed", func(t *testing.T) {
		input := validOperationEventInput()
		input.Status = eventTestString("queued_for_provider")

		if err := input.Validate(); err != nil {
			t.Fatalf("custom status rejected: %v", err)
		}
	})

	tests := []struct {
		name   string
		mutate func(*persistence.CreateOperationEventInput)
	}{
		{
			name: "missing operation ID",
			mutate: func(in *persistence.CreateOperationEventInput) {
				in.OperationID = ""
			},
		},
		{
			name: "invalid operation ID",
			mutate: func(in *persistence.CreateOperationEventInput) {
				in.OperationID = "invalid"
			},
		},
		{
			name: "missing event type",
			mutate: func(in *persistence.CreateOperationEventInput) {
				in.EventType = ""
			},
		},
		{
			name: "blank event type",
			mutate: func(in *persistence.CreateOperationEventInput) {
				in.EventType = "   "
			},
		},
		{
			name: "event type too long",
			mutate: func(in *persistence.CreateOperationEventInput) {
				in.EventType = strings.Repeat("e", 101)
			},
		},
		{
			name: "event type contains null",
			mutate: func(in *persistence.CreateOperationEventInput) {
				in.EventType = "event\x00type"
			},
		},
		{
			name: "status too long",
			mutate: func(in *persistence.CreateOperationEventInput) {
				in.Status = eventTestString(strings.Repeat("s", 51))
			},
		},
		{
			name: "message contains null",
			mutate: func(in *persistence.CreateOperationEventInput) {
				in.Message = eventTestString("message\x00invalid")
			},
		},
		{
			name: "invalid JSON",
			mutate: func(in *persistence.CreateOperationEventInput) {
				in.EventData = json.RawMessage(`{"invalid":`)
			},
		},
		{
			name: "JSON array",
			mutate: func(in *persistence.CreateOperationEventInput) {
				in.EventData = json.RawMessage(`[1,2,3]`)
			},
		},
		{
			name: "JSON null",
			mutate: func(in *persistence.CreateOperationEventInput) {
				in.EventData = json.RawMessage(`null`)
			},
		},
		{
			name: "JSON string",
			mutate: func(in *persistence.CreateOperationEventInput) {
				in.EventData = json.RawMessage(`"event"`)
			},
		},
		{
			name: "negative sequence number",
			mutate: func(in *persistence.CreateOperationEventInput) {
				in.SequenceNumber = eventTestInt32(-1)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := validOperationEventInput()
			tt.mutate(&input)

			if err := input.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestOperationEventErrorSentinels(t *testing.T) {
	if persistence.ErrOperationEventNotFound == nil {
		t.Fatal("not-found error must be defined")
	}

	if persistence.ErrOperationEventConflict == nil {
		t.Fatal("conflict error must be defined")
	}

	if errors.Is(
		persistence.ErrOperationEventNotFound,
		persistence.ErrOperationEventConflict,
	) {
		t.Fatal("error sentinels must be distinct")
	}
}

func TestOperationEventRepositoryNilDatabase(t *testing.T) {
	repo, err := persistence.NewOperationEventRepository(nil)

	if err == nil {
		t.Fatal("expected constructor error")
	}

	if repo != nil {
		t.Fatal("expected nil repository")
	}
}
