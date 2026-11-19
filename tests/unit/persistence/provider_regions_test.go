package persistence_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/gdiop88/RL_trader_infrastructure/internal/persistence"
)

const testRegionProviderID = "11111111-1111-4111-8111-111111111111"

func regionString(value string) *string {
	return &value
}

func TestCreateProviderRegionInputValidation(t *testing.T) {
	tests := []struct {
		name    string
		input   persistence.CreateProviderRegionInput
		wantErr bool
	}{
		{
			name: "valid minimal",
			input: persistence.CreateProviderRegionInput{
				ProviderID: testRegionProviderID,
				Code:       "nyc3",
			},
		},
		{
			name: "valid complete",
			input: persistence.CreateProviderRegionInput{
				ProviderID:   testRegionProviderID,
				Code:         "nyc3",
				DisplayName:  regionString("New York 3"),
				Capabilities: []byte(`{"compute":true,"gpu":false}`),
			},
		},
		{
			name: "missing provider ID",
			input: persistence.CreateProviderRegionInput{
				Code: "nyc3",
			},
			wantErr: true,
		},
		{
			name: "invalid provider ID",
			input: persistence.CreateProviderRegionInput{
				ProviderID: "invalid",
				Code:       "nyc3",
			},
			wantErr: true,
		},
		{
			name: "missing code",
			input: persistence.CreateProviderRegionInput{
				ProviderID: testRegionProviderID,
			},
			wantErr: true,
		},
		{
			name: "whitespace code",
			input: persistence.CreateProviderRegionInput{
				ProviderID: testRegionProviderID,
				Code:       "   ",
			},
			wantErr: true,
		},
		{
			name: "code too long",
			input: persistence.CreateProviderRegionInput{
				ProviderID: testRegionProviderID,
				Code:       strings.Repeat("x", 101),
			},
			wantErr: true,
		},
		{
			name: "display name too long",
			input: persistence.CreateProviderRegionInput{
				ProviderID:  testRegionProviderID,
				Code:        "nyc3",
				DisplayName: regionString(strings.Repeat("x", 151)),
			},
			wantErr: true,
		},
		{
			name: "invalid capabilities JSON",
			input: persistence.CreateProviderRegionInput{
				ProviderID:   testRegionProviderID,
				Code:         "nyc3",
				Capabilities: []byte(`{"gpu":`),
			},
			wantErr: true,
		},
		{
			name: "capabilities array",
			input: persistence.CreateProviderRegionInput{
				ProviderID:   testRegionProviderID,
				Code:         "nyc3",
				Capabilities: []byte(`["gpu"]`),
			},
			wantErr: true,
		},
		{
			name: "capabilities null",
			input: persistence.CreateProviderRegionInput{
				ProviderID:   testRegionProviderID,
				Code:         "nyc3",
				Capabilities: []byte(`null`),
			},
			wantErr: true,
		},
		{
			name: "valid empty capabilities object",
			input: persistence.CreateProviderRegionInput{
				ProviderID:   testRegionProviderID,
				Code:         "nyc3",
				Capabilities: []byte(`{}`),
			},
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

func TestUpdateProviderRegionInputValidation(t *testing.T) {
	tests := []struct {
		name    string
		input   persistence.UpdateProviderRegionInput
		wantErr bool
	}{
		{
			name:  "valid empty update",
			input: persistence.UpdateProviderRegionInput{},
		},
		{
			name: "valid update",
			input: persistence.UpdateProviderRegionInput{
				DisplayName:  regionString("New York"),
				Capabilities: []byte(`{"gpu":true}`),
			},
		},
		{
			name: "display name too long",
			input: persistence.UpdateProviderRegionInput{
				DisplayName: regionString(strings.Repeat("x", 151)),
			},
			wantErr: true,
		},
		{
			name: "invalid JSON",
			input: persistence.UpdateProviderRegionInput{
				Capabilities: []byte(`invalid`),
			},
			wantErr: true,
		},
		{
			name: "JSON array",
			input: persistence.UpdateProviderRegionInput{
				Capabilities: []byte(`[1,2,3]`),
			},
			wantErr: true,
		},
		{
			name: "JSON null",
			input: persistence.UpdateProviderRegionInput{
				Capabilities: []byte(`null`),
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

func TestProviderRegionRepositoryConstructor(t *testing.T) {
	_, err := persistence.NewProviderRegionRepository(nil)

	if err == nil {
		t.Fatal("expected error for nil database")
	}
}

func TestProviderRegionErrorSentinels(t *testing.T) {
	if errors.Is(
		persistence.ErrProviderRegionNotFound,
		persistence.ErrProviderRegionConflict,
	) {
		t.Fatal("not-found and conflict errors must be distinct")
	}
}
