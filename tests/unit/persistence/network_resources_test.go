package persistence_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/gdiop88/RL_trader_infrastructure/internal/persistence"
)

const (
	testNetworkProjectID = "11111111-1111-4111-8111-111111111111"
	testNetworkAccountID = "22222222-2222-4222-8222-222222222222"
)

func networkTestString(value string) *string {
	return &value
}

func TestCreateNetworkResourceValidation(t *testing.T) {
	tests := []struct {
		name    string
		input   persistence.CreateNetworkResourceInput
		wantErr bool
	}{
		{
			name:  "all nullable fields omitted",
			input: persistence.CreateNetworkResourceInput{},
		},
		{
			name: "complete IPv4 network",
			input: persistence.CreateNetworkResourceInput{
				ProjectID:         networkTestString(testNetworkProjectID),
				ProviderAccountID: networkTestString(testNetworkAccountID),
				ProviderNetworkID: networkTestString("vpc-001"),
				Name:              networkTestString("trading-network"),
				Region:            networkTestString("nyc1"),
				NetworkType:       networkTestString("vpc"),
				CIDR:              networkTestString("10.10.0.0/16"),
				Status:            networkTestString("active"),
				Metadata:          json.RawMessage(`{"private":true}`),
			},
		},
		{
			name: "valid IPv6 network",
			input: persistence.CreateNetworkResourceInput{
				CIDR: networkTestString("2001:db8::/32"),
			},
		},
		{
			name: "valid host prefix",
			input: persistence.CreateNetworkResourceInput{
				CIDR: networkTestString("192.168.1.10/32"),
			},
		},
		{
			name: "valid custom status",
			input: persistence.CreateNetworkResourceInput{
				Status: networkTestString("awaiting_provisioning"),
			},
		},
		{
			name: "empty optional strings",
			input: persistence.CreateNetworkResourceInput{
				Name:        networkTestString(""),
				Region:      networkTestString(""),
				NetworkType: networkTestString(""),
				Status:      networkTestString(""),
			},
		},
		{
			name: "maximum string lengths",
			input: persistence.CreateNetworkResourceInput{
				ProviderNetworkID: networkTestString(strings.Repeat("a", 255)),
				Name:              networkTestString(strings.Repeat("n", 150)),
				Region:            networkTestString(strings.Repeat("r", 100)),
				NetworkType:       networkTestString(strings.Repeat("t", 50)),
				Status:            networkTestString(strings.Repeat("s", 50)),
			},
		},
		{
			name: "invalid project UUID",
			input: persistence.CreateNetworkResourceInput{
				ProjectID: networkTestString("invalid"),
			},
			wantErr: true,
		},
		{
			name: "invalid account UUID",
			input: persistence.CreateNetworkResourceInput{
				ProviderAccountID: networkTestString("invalid"),
			},
			wantErr: true,
		},
		{
			name: "empty project UUID",
			input: persistence.CreateNetworkResourceInput{
				ProjectID: networkTestString(""),
			},
			wantErr: true,
		},
		{
			name: "provider network ID too long",
			input: persistence.CreateNetworkResourceInput{
				ProviderNetworkID: networkTestString(strings.Repeat("a", 256)),
			},
			wantErr: true,
		},
		{
			name: "name too long",
			input: persistence.CreateNetworkResourceInput{
				Name: networkTestString(strings.Repeat("n", 151)),
			},
			wantErr: true,
		},
		{
			name: "region too long",
			input: persistence.CreateNetworkResourceInput{
				Region: networkTestString(strings.Repeat("r", 101)),
			},
			wantErr: true,
		},
		{
			name: "network type too long",
			input: persistence.CreateNetworkResourceInput{
				NetworkType: networkTestString(strings.Repeat("t", 51)),
			},
			wantErr: true,
		},
		{
			name: "status too long",
			input: persistence.CreateNetworkResourceInput{
				Status: networkTestString(strings.Repeat("s", 51)),
			},
			wantErr: true,
		},
		{
			name: "null character in name",
			input: persistence.CreateNetworkResourceInput{
				Name: networkTestString("network\x00name"),
			},
			wantErr: true,
		},
		{
			name: "null character in status",
			input: persistence.CreateNetworkResourceInput{
				Status: networkTestString("active\x00"),
			},
			wantErr: true,
		},
		{
			name: "invalid IPv4 CIDR",
			input: persistence.CreateNetworkResourceInput{
				CIDR: networkTestString("10.0.0.0/33"),
			},
			wantErr: true,
		},
		{
			name: "invalid IPv6 CIDR",
			input: persistence.CreateNetworkResourceInput{
				CIDR: networkTestString("2001:db8::/129"),
			},
			wantErr: true,
		},
		{
			name: "invalid CIDR text",
			input: persistence.CreateNetworkResourceInput{
				CIDR: networkTestString("not-a-network"),
			},
			wantErr: true,
		},
		{
			name: "CIDR missing prefix",
			input: persistence.CreateNetworkResourceInput{
				CIDR: networkTestString("10.0.0.1"),
			},
			wantErr: true,
		},
		{
			name: "CIDR contains null",
			input: persistence.CreateNetworkResourceInput{
				CIDR: networkTestString("10.0.0.0/24\x00"),
			},
			wantErr: true,
		},
		{
			name: "invalid metadata",
			input: persistence.CreateNetworkResourceInput{
				Metadata: json.RawMessage(`{"invalid":`),
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

	t.Run("all valid JSONB types", func(t *testing.T) {
		values := []string{
			`{}`,
			`[]`,
			`null`,
			`"network"`,
			`42`,
			`true`,
		}

		for _, value := range values {
			t.Run(value, func(t *testing.T) {
				input := persistence.CreateNetworkResourceInput{
					Metadata: json.RawMessage(value),
				}

				if err := input.Validate(); err != nil {
					t.Fatalf("valid JSON rejected: %v", err)
				}
			})
		}
	})
}

func TestUpdateNetworkResourceValidation(t *testing.T) {
	tests := []struct {
		name    string
		input   persistence.UpdateNetworkResourceInput
		wantErr bool
	}{
		{
			name:  "clear all nullable fields",
			input: persistence.UpdateNetworkResourceInput{},
		},
		{
			name: "valid complete update",
			input: persistence.UpdateNetworkResourceInput{
				ProjectID:         networkTestString(testNetworkProjectID),
				ProviderAccountID: networkTestString(testNetworkAccountID),
				ProviderNetworkID: networkTestString("vpc-updated"),
				Name:              networkTestString("updated-network"),
				Region:            networkTestString("sfo3"),
				NetworkType:       networkTestString("private"),
				CIDR:              networkTestString("172.16.0.0/16"),
				Status:            networkTestString("ready"),
				Metadata:          json.RawMessage(`{"version":2}`),
			},
		},
		{
			name: "invalid project",
			input: persistence.UpdateNetworkResourceInput{
				ProjectID: networkTestString("bad-id"),
			},
			wantErr: true,
		},
		{
			name: "invalid provider account",
			input: persistence.UpdateNetworkResourceInput{
				ProviderAccountID: networkTestString("bad-id"),
			},
			wantErr: true,
		},
		{
			name: "invalid CIDR",
			input: persistence.UpdateNetworkResourceInput{
				CIDR: networkTestString("192.168.0.0/99"),
			},
			wantErr: true,
		},
		{
			name: "invalid status",
			input: persistence.UpdateNetworkResourceInput{
				Status: networkTestString(strings.Repeat("s", 51)),
			},
			wantErr: true,
		},
		{
			name: "invalid metadata",
			input: persistence.UpdateNetworkResourceInput{
				Metadata: json.RawMessage(`invalid-json`),
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

func TestNetworkResourceErrorSentinels(t *testing.T) {
	if persistence.ErrNetworkResourceNotFound == nil {
		t.Fatal("not-found sentinel is nil")
	}

	if persistence.ErrNetworkResourceConflict == nil {
		t.Fatal("conflict sentinel is nil")
	}

	if errors.Is(
		persistence.ErrNetworkResourceNotFound,
		persistence.ErrNetworkResourceConflict,
	) {
		t.Fatal("error sentinels must be distinct")
	}
}

func TestNetworkResourceRepositoryNilDatabase(t *testing.T) {
	repo, err := persistence.NewNetworkResourceRepository(nil)

	if err == nil {
		t.Fatal("expected error for nil database")
	}

	if repo != nil {
		t.Fatal("expected nil repository")
	}
}
