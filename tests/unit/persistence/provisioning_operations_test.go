package persistence_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gdiop88/RL_trader_infrastructure/internal/persistence"
)

func operationTestString(value string) *string {
	return &value
}

func operationTestInt32(value int32) *int32 {
	return &value
}

func validProvisioningOperationInput() persistence.CreateProvisioningOperationInput {
	return persistence.CreateProvisioningOperationInput{
		OperationType: "create",
		Provider:      "digitalocean",
		Region:        operationTestString("nyc3"),
		RequestedBy:   operationTestString("rl-trader"),
		RequestPayload: json.RawMessage(
			`{"size":"s-2vcpu-4gb","image":"centos-stream-9"}`,
		),
		MaxRetries:     operationTestInt32(3),
		IdempotencyKey: operationTestString("operation-test-key"),
	}
}

func TestCreateProvisioningOperationValidation(t *testing.T) {
	t.Run("valid operation", func(t *testing.T) {
		input := validProvisioningOperationInput()
		if err := input.Validate(); err != nil {
			t.Fatalf("expected valid operation: %v", err)
		}
	})

	t.Run("minimal operation", func(t *testing.T) {
		input := persistence.CreateProvisioningOperationInput{
			OperationType: "create",
			Provider:      "digitalocean",
		}
		if err := input.Validate(); err != nil {
			t.Fatalf("expected valid minimal operation: %v", err)
		}
	})

	t.Run("all supported operation types", func(t *testing.T) {
		types := []string{
			"create", "update", "delete",
			"start", "stop", "restart", "resize",
		}

		for _, operationType := range types {
			t.Run(operationType, func(t *testing.T) {
				input := validProvisioningOperationInput()
				input.OperationType = operationType

				if err := input.Validate(); err != nil {
					t.Fatalf(
						"operation type %q rejected: %v",
						operationType,
						err,
					)
				}
			})
		}
	})

	tests := []struct {
		name   string
		mutate func(*persistence.CreateProvisioningOperationInput)
	}{
		{
			name: "invalid operation type",
			mutate: func(in *persistence.CreateProvisioningOperationInput) {
				in.OperationType = "deploy"
			},
		},
		{
			name: "missing provider",
			mutate: func(in *persistence.CreateProvisioningOperationInput) {
				in.Provider = ""
			},
		},
		{
			name: "provider too long",
			mutate: func(in *persistence.CreateProvisioningOperationInput) {
				in.Provider = strings.Repeat("p", 51)
			},
		},
		{
			name: "provider contains null",
			mutate: func(in *persistence.CreateProvisioningOperationInput) {
				in.Provider = "digital\x00ocean"
			},
		},
		{
			name: "invalid instance ID",
			mutate: func(in *persistence.CreateProvisioningOperationInput) {
				in.InstanceID = operationTestString("invalid")
			},
		},
		{
			name: "invalid project ID",
			mutate: func(in *persistence.CreateProvisioningOperationInput) {
				in.ProjectID = operationTestString("invalid")
			},
		},
		{
			name: "invalid template ID",
			mutate: func(in *persistence.CreateProvisioningOperationInput) {
				in.TemplateID = operationTestString("invalid")
			},
		},
		{
			name: "invalid provider account ID",
			mutate: func(in *persistence.CreateProvisioningOperationInput) {
				in.ProviderAccountID = operationTestString("invalid")
			},
		},
		{
			name: "invalid correlation ID",
			mutate: func(in *persistence.CreateProvisioningOperationInput) {
				in.CorrelationID = operationTestString("invalid")
			},
		},
		{
			name: "requested by too long",
			mutate: func(in *persistence.CreateProvisioningOperationInput) {
				in.RequestedBy = operationTestString(strings.Repeat("r", 256))
			},
		},
		{
			name: "region too long",
			mutate: func(in *persistence.CreateProvisioningOperationInput) {
				in.Region = operationTestString(strings.Repeat("r", 101))
			},
		},
		{
			name: "idempotency key too long",
			mutate: func(in *persistence.CreateProvisioningOperationInput) {
				in.IdempotencyKey = operationTestString(strings.Repeat("k", 256))
			},
		},
		{
			name: "negative max retries",
			mutate: func(in *persistence.CreateProvisioningOperationInput) {
				in.MaxRetries = operationTestInt32(-1)
			},
		},
		{
			name: "invalid request JSON",
			mutate: func(in *persistence.CreateProvisioningOperationInput) {
				in.RequestPayload = json.RawMessage(`{"invalid":`)
			},
		},
		{
			name: "request array",
			mutate: func(in *persistence.CreateProvisioningOperationInput) {
				in.RequestPayload = json.RawMessage(`["invalid"]`)
			},
		},
		{
			name: "request null",
			mutate: func(in *persistence.CreateProvisioningOperationInput) {
				in.RequestPayload = json.RawMessage(`null`)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := validProvisioningOperationInput()
			tt.mutate(&input)

			if err := input.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestUpdateProvisioningOperationStatusValidation(t *testing.T) {
	validStatuses := []string{
		"pending",
		"running",
		"completed",
		"failed",
		"cancelled",
	}

	for _, status := range validStatuses {
		t.Run("valid "+status, func(t *testing.T) {
			input := persistence.UpdateProvisioningOperationStatusInput{
				Status: status,
			}
			if err := input.Validate(); err != nil {
				t.Fatalf("status %q rejected: %v", status, err)
			}
		})
	}

	t.Run("invalid status", func(t *testing.T) {
		input := persistence.UpdateProvisioningOperationStatusInput{
			Status: "unknown",
		}
		if err := input.Validate(); err == nil {
			t.Fatal("expected invalid status error")
		}
	})

	t.Run("valid response JSON", func(t *testing.T) {
		input := persistence.UpdateProvisioningOperationStatusInput{
			Status:          "completed",
			ResponsePayload: json.RawMessage(`{"instance_id":"123"}`),
		}
		if err := input.Validate(); err != nil {
			t.Fatalf("valid response rejected: %v", err)
		}
	})

	t.Run("response array rejected", func(t *testing.T) {
		input := persistence.UpdateProvisioningOperationStatusInput{
			Status:          "completed",
			ResponsePayload: json.RawMessage(`[1,2,3]`),
		}
		if err := input.Validate(); err == nil {
			t.Fatal("expected JSON object validation error")
		}
	})

	t.Run("response null rejected", func(t *testing.T) {
		input := persistence.UpdateProvisioningOperationStatusInput{
			Status:          "completed",
			ResponsePayload: json.RawMessage(`null`),
		}
		if err := input.Validate(); err == nil {
			t.Fatal("expected JSON object validation error")
		}
	})

	t.Run("error message contains null", func(t *testing.T) {
		input := persistence.UpdateProvisioningOperationStatusInput{
			Status:       "failed",
			ErrorMessage: operationTestString("error\x00message"),
		}
		if err := input.Validate(); err == nil {
			t.Fatal("expected error message validation failure")
		}
	})

	t.Run("valid timestamps", func(t *testing.T) {
		started := time.Now().UTC()
		completed := started.Add(time.Minute)

		input := persistence.UpdateProvisioningOperationStatusInput{
			Status:      "completed",
			StartedAt:   &started,
			CompletedAt: &completed,
		}

		if err := input.Validate(); err != nil {
			t.Fatalf("valid timestamps rejected: %v", err)
		}
	})

	t.Run("completed before started", func(t *testing.T) {
		started := time.Now().UTC()
		completed := started.Add(-time.Minute)

		input := persistence.UpdateProvisioningOperationStatusInput{
			Status:      "completed",
			StartedAt:   &started,
			CompletedAt: &completed,
		}

		if err := input.Validate(); err == nil {
			t.Fatal("expected timestamp validation error")
		}
	})
}

func TestProvisioningOperationErrorSentinels(t *testing.T) {
	if persistence.ErrProvisioningOperationNotFound == nil {
		t.Fatal("not-found error must be defined")
	}
	if persistence.ErrProvisioningOperationConflict == nil {
		t.Fatal("conflict error must be defined")
	}

	if errors.Is(
		persistence.ErrProvisioningOperationNotFound,
		persistence.ErrProvisioningOperationConflict,
	) {
		t.Fatal("error sentinels must be distinct")
	}
}

func TestProvisioningOperationRepositoryNilDatabase(t *testing.T) {
	repo, err := persistence.NewProvisioningOperationRepository(nil)
	if err == nil {
		t.Fatal("expected constructor error")
	}
	if repo != nil {
		t.Fatal("expected nil repository")
	}
}
