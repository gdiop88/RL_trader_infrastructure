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
	testStorageProjectID = "11111111-1111-4111-8111-111111111111"
	testStorageAccountID = "22222222-2222-4222-8222-222222222222"
)

func storageString(value string) *string {
	return &value
}

func storageBool(value bool) *bool {
	return &value
}

func storageTime(value time.Time) *time.Time {
	return &value
}

func TestCreateStorageVolumeValidation(t *testing.T) {
	tests := []struct {
		name    string
		input   persistence.CreateStorageVolumeInput
		wantErr bool
	}{
		{
			name: "minimum valid volume",
			input: persistence.CreateStorageVolumeInput{
				SizeGB: 1,
			},
		},
		{
			name: "complete volume",
			input: persistence.CreateStorageVolumeInput{
				ProjectID:         storageString(testStorageProjectID),
				ProviderAccountID: storageString(testStorageAccountID),
				ProviderVolumeID:  storageString("vol-123"),
				Name:              storageString("production-volume"),
				Region:            storageString("nyc1"),
				SizeGB:            100,
				VolumeType:        storageString("ssd"),
				Status:            storageString("available"),
				Encrypted:         storageBool(true),
				Metadata:          json.RawMessage(`{"tier":"production"}`),
			},
		},
		{
			name: "maximum string lengths",
			input: persistence.CreateStorageVolumeInput{
				SizeGB:           1,
				ProviderVolumeID: storageString(strings.Repeat("a", 255)),
				Name:             storageString(strings.Repeat("b", 150)),
				Region:           storageString(strings.Repeat("c", 100)),
				VolumeType:       storageString(strings.Repeat("d", 100)),
				Status:           storageString(strings.Repeat("e", 50)),
			},
		},
		{
			name: "JSON array allowed",
			input: persistence.CreateStorageVolumeInput{
				SizeGB:   1,
				Metadata: json.RawMessage(`[1,2,3]`),
			},
		},
		{
			name: "JSON scalar allowed",
			input: persistence.CreateStorageVolumeInput{
				SizeGB:   1,
				Metadata: json.RawMessage(`42`),
			},
		},
		{
			name: "JSON null allowed",
			input: persistence.CreateStorageVolumeInput{
				SizeGB:   1,
				Metadata: json.RawMessage(`null`),
			},
		},
		{
			name: "zero size",
			input: persistence.CreateStorageVolumeInput{
				SizeGB: 0,
			},
			wantErr: true,
		},
		{
			name: "negative size",
			input: persistence.CreateStorageVolumeInput{
				SizeGB: -10,
			},
			wantErr: true,
		},
		{
			name: "invalid project UUID",
			input: persistence.CreateStorageVolumeInput{
				SizeGB:    10,
				ProjectID: storageString("invalid"),
			},
			wantErr: true,
		},
		{
			name: "invalid provider account UUID",
			input: persistence.CreateStorageVolumeInput{
				SizeGB:            10,
				ProviderAccountID: storageString("invalid"),
			},
			wantErr: true,
		},
		{
			name: "provider volume ID too long",
			input: persistence.CreateStorageVolumeInput{
				SizeGB:           10,
				ProviderVolumeID: storageString(strings.Repeat("a", 256)),
			},
			wantErr: true,
		},
		{
			name: "name too long",
			input: persistence.CreateStorageVolumeInput{
				SizeGB: 10,
				Name:   storageString(strings.Repeat("a", 151)),
			},
			wantErr: true,
		},
		{
			name: "region too long",
			input: persistence.CreateStorageVolumeInput{
				SizeGB: 10,
				Region: storageString(strings.Repeat("a", 101)),
			},
			wantErr: true,
		},
		{
			name: "volume type too long",
			input: persistence.CreateStorageVolumeInput{
				SizeGB:     10,
				VolumeType: storageString(strings.Repeat("a", 101)),
			},
			wantErr: true,
		},
		{
			name: "status too long",
			input: persistence.CreateStorageVolumeInput{
				SizeGB: 10,
				Status: storageString(strings.Repeat("a", 51)),
			},
			wantErr: true,
		},
		{
			name: "invalid JSON",
			input: persistence.CreateStorageVolumeInput{
				SizeGB:   10,
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
}

func TestUpdateStorageVolumeValidation(t *testing.T) {
	now := time.Now().UTC()

	tests := []struct {
		name    string
		input   persistence.UpdateStorageVolumeInput
		wantErr bool
	}{
		{
			name: "valid update",
			input: persistence.UpdateStorageVolumeInput{
				SizeGB:    200,
				Name:      storageString("updated-volume"),
				Status:    storageString("attached"),
				Encrypted: storageBool(false),
				Metadata:  json.RawMessage(`{"updated":true}`),
			},
		},
		{
			name: "valid soft deleted update",
			input: persistence.UpdateStorageVolumeInput{
				SizeGB:    100,
				DeletedAt: storageTime(now),
			},
		},
		{
			name: "clear nullable fields",
			input: persistence.UpdateStorageVolumeInput{
				SizeGB: 1,
			},
		},
		{
			name: "zero size",
			input: persistence.UpdateStorageVolumeInput{
				SizeGB: 0,
			},
			wantErr: true,
		},
		{
			name: "negative size",
			input: persistence.UpdateStorageVolumeInput{
				SizeGB: -1,
			},
			wantErr: true,
		},
		{
			name: "invalid project UUID",
			input: persistence.UpdateStorageVolumeInput{
				SizeGB:    10,
				ProjectID: storageString("not-a-uuid"),
			},
			wantErr: true,
		},
		{
			name: "invalid metadata",
			input: persistence.UpdateStorageVolumeInput{
				SizeGB:   10,
				Metadata: json.RawMessage(`{broken}`),
			},
			wantErr: true,
		},
		{
			name: "zero deleted timestamp",
			input: persistence.UpdateStorageVolumeInput{
				SizeGB:    10,
				DeletedAt: storageTime(time.Time{}),
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

func TestStorageVolumeErrorSentinels(t *testing.T) {
	if persistence.ErrStorageVolumeNotFound == nil {
		t.Fatal("not-found sentinel is nil")
	}

	if persistence.ErrStorageVolumeConflict == nil {
		t.Fatal("conflict sentinel is nil")
	}

	if errors.Is(
		persistence.ErrStorageVolumeNotFound,
		persistence.ErrStorageVolumeConflict,
	) {
		t.Fatal("error sentinels must be distinct")
	}
}

func TestStorageVolumeRepositoryNilDatabase(t *testing.T) {
	repo, err := persistence.NewStorageVolumeRepository(nil)

	if err == nil {
		t.Fatal("expected error for nil database")
	}

	if repo != nil {
		t.Fatal("expected nil repository")
	}
}
