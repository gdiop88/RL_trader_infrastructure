package persistence_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/gdiop88/RL_trader_infrastructure/internal/persistence"
)

func TestCreateProviderInputValidation(t *testing.T) {
	valid := persistence.CreateProviderInput{
		Code:         "digitalocean",
		Name:         "DigitalOcean",
		ProviderType: "cloud",
	}

	tests := []struct {
		name    string
		input   persistence.CreateProviderInput
		wantErr bool
	}{
		{"valid", valid, false},
		{"missing code", persistence.CreateProviderInput{
			Name: "DigitalOcean", ProviderType: "cloud",
		}, true},
		{"missing name", persistence.CreateProviderInput{
			Code: "digitalocean", ProviderType: "cloud",
		}, true},
		{"missing type", persistence.CreateProviderInput{
			Code: "digitalocean", Name: "DigitalOcean",
		}, true},
		{"code too long", persistence.CreateProviderInput{
			Code: strings.Repeat("x", 51),
			Name: "DigitalOcean", ProviderType: "cloud",
		}, true},
		{"name too long", persistence.CreateProviderInput{
			Code:         "digitalocean",
			Name:         strings.Repeat("x", 101),
			ProviderType: "cloud",
		}, true},
		{"type too long", persistence.CreateProviderInput{
			Code:         "digitalocean",
			Name:         "DigitalOcean",
			ProviderType: strings.Repeat("x", 51),
		}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.input.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v",
					err, tt.wantErr)
			}
		})
	}
}

func TestUpdateProviderInputValidation(t *testing.T) {
	endpoint := "https://api.digitalocean.com"

	tests := []struct {
		name    string
		input   persistence.UpdateProviderInput
		wantErr bool
	}{
		{"valid", persistence.UpdateProviderInput{
			Name: "DigitalOcean", ProviderType: "cloud",
			APIEndpoint: &endpoint,
		}, false},
		{"missing name", persistence.UpdateProviderInput{
			ProviderType: "cloud",
		}, true},
		{"missing type", persistence.UpdateProviderInput{
			Name: "DigitalOcean",
		}, true},
		{"endpoint too long", persistence.UpdateProviderInput{
			Name: "DigitalOcean", ProviderType: "cloud",
			APIEndpoint: func() *string {
				s := strings.Repeat("x", 501)
				return &s
			}(),
		}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.input.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v",
					err, tt.wantErr)
			}
		})
	}
}

func TestProviderRepositoryConstructor(t *testing.T) {
	_, err := persistence.NewProviderRepository(nil)
	if err == nil {
		t.Fatal("expected error for nil database")
	}
}

func TestProviderErrorSentinels(t *testing.T) {
	if errors.Is(
		persistence.ErrProviderNotFound,
		persistence.ErrProviderConflict,
	) {
		t.Fatal("not-found and conflict errors must be distinct")
	}
}
