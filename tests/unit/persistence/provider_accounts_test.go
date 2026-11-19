package persistence_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/gdiop88/RL_trader_infrastructure/internal/persistence"
)

func TestCreateProviderAccountInputValidation(t *testing.T) {
	const providerID = "11111111-1111-4111-8111-111111111111"

	tests := []struct {
		name    string
		input   persistence.CreateProviderAccountInput
		wantErr bool
	}{
		{
			name: "valid minimal",
			input: persistence.CreateProviderAccountInput{
				ProviderID: providerID,
				Name:       "DigitalOcean Production",
			},
		},
		{
			name: "valid complete",
			input: persistence.CreateProviderAccountInput{
				ProviderID:          providerID,
				Name:                "DigitalOcean Production",
				ExternalAccountID:   accountString("external-123"),
				CredentialReference: accountString("secret://digitalocean/prod"),
				Metadata:            []byte(`{"environment":"production"}`),
			},
		},
		{
			name: "missing provider ID",
			input: persistence.CreateProviderAccountInput{
				Name: "Production",
			},
			wantErr: true,
		},
		{
			name: "invalid provider ID",
			input: persistence.CreateProviderAccountInput{
				ProviderID: "not-a-uuid",
				Name:       "Production",
			},
			wantErr: true,
		},
		{
			name: "missing name",
			input: persistence.CreateProviderAccountInput{
				ProviderID: providerID,
			},
			wantErr: true,
		},
		{
			name: "whitespace name",
			input: persistence.CreateProviderAccountInput{
				ProviderID: providerID,
				Name:       "   ",
			},
			wantErr: true,
		},
		{
			name: "name too long",
			input: persistence.CreateProviderAccountInput{
				ProviderID: providerID,
				Name:       strings.Repeat("x", 151),
			},
			wantErr: true,
		},
		{
			name: "external account ID too long",
			input: persistence.CreateProviderAccountInput{
				ProviderID:        providerID,
				Name:              "Production",
				ExternalAccountID: accountString(strings.Repeat("x", 256)),
			},
			wantErr: true,
		},
		{
			name: "credential reference too long",
			input: persistence.CreateProviderAccountInput{
				ProviderID:          providerID,
				Name:                "Production",
				CredentialReference: accountString(strings.Repeat("x", 501)),
			},
			wantErr: true,
		},
		{
			name: "invalid JSON",
			input: persistence.CreateProviderAccountInput{
				ProviderID: providerID,
				Name:       "Production",
				Metadata:   []byte(`{"invalid":`),
			},
			wantErr: true,
		},
		{
			name: "metadata array",
			input: persistence.CreateProviderAccountInput{
				ProviderID: providerID,
				Name:       "Production",
				Metadata:   []byte(`[]`),
			},
			wantErr: true,
		},
		{
			name: "metadata null",
			input: persistence.CreateProviderAccountInput{
				ProviderID: providerID,
				Name:       "Production",
				Metadata:   []byte(`null`),
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
					err, tt.wantErr,
				)
			}
		})
	}
}

func TestUpdateProviderAccountInputValidation(t *testing.T) {
	tests := []struct {
		name    string
		input   persistence.UpdateProviderAccountInput
		wantErr bool
	}{
		{
			name: "valid",
			input: persistence.UpdateProviderAccountInput{
				Name:                "Updated Account",
				CredentialReference: accountString("secret://updated"),
				Metadata:            []byte(`{"updated":true}`),
			},
		},
		{
			name: "missing name",
			input: persistence.UpdateProviderAccountInput{
				Metadata: []byte(`{}`),
			},
			wantErr: true,
		},
		{
			name: "name too long",
			input: persistence.UpdateProviderAccountInput{
				Name: strings.Repeat("x", 151),
			},
			wantErr: true,
		},
		{
			name: "invalid metadata",
			input: persistence.UpdateProviderAccountInput{
				Name:     "Updated Account",
				Metadata: []byte(`invalid`),
			},
			wantErr: true,
		},
		{
			name: "metadata must be object",
			input: persistence.UpdateProviderAccountInput{
				Name:     "Updated Account",
				Metadata: []byte(`["invalid"]`),
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
					err, tt.wantErr,
				)
			}
		})
	}
}

func TestProviderAccountRepositoryConstructor(t *testing.T) {
	_, err := persistence.NewProviderAccountRepository(nil)
	if err == nil {
		t.Fatal("expected error for nil database")
	}
}

func TestProviderAccountErrorSentinels(t *testing.T) {
	if errors.Is(
		persistence.ErrProviderAccountNotFound,
		persistence.ErrProviderAccountConflict,
	) {
		t.Fatal("not-found and conflict errors must be distinct")
	}
}

func accountString(value string) *string {
	return &value
}
