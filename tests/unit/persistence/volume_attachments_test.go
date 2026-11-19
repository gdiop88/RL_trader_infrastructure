package persistence_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gdiop88/RL_trader_infrastructure/internal/persistence"
)

const (
	testAttachmentVolumeID         = "11111111-1111-4111-8111-111111111111"
	testVolumeAttachmentInstanceID = "22222222-2222-4222-8222-222222222222"
)

func volumeAttachmentString(s string) *string {
	return &s
}

func volumeAttachmentTime(t time.Time) *time.Time {
	return &t
}

func TestCreateVolumeAttachmentValidation(t *testing.T) {
	now := time.Now().UTC()

	tests := []struct {
		name    string
		input   persistence.CreateVolumeAttachmentInput
		wantErr bool
	}{
		{
			name: "minimum valid attachment",
			input: persistence.CreateVolumeAttachmentInput{
				VolumeID:   testAttachmentVolumeID,
				InstanceID: testVolumeAttachmentInstanceID,
			},
		},
		{
			name: "complete attachment",
			input: persistence.CreateVolumeAttachmentInput{
				VolumeID:   testAttachmentVolumeID,
				InstanceID: testVolumeAttachmentInstanceID,
				MountPoint: volumeAttachmentString("/mnt/data"),
				Status:     volumeAttachmentString("attached"),
				AttachedAt: volumeAttachmentTime(now),
			},
		},
		{
			name: "maximum string lengths",
			input: persistence.CreateVolumeAttachmentInput{
				VolumeID:   testAttachmentVolumeID,
				InstanceID: testVolumeAttachmentInstanceID,
				MountPoint: volumeAttachmentString(strings.Repeat("m", 255)),
				Status:     volumeAttachmentString(strings.Repeat("s", 50)),
			},
		},
		{
			name: "detached timestamp allowed",
			input: persistence.CreateVolumeAttachmentInput{
				VolumeID:   testAttachmentVolumeID,
				InstanceID: testVolumeAttachmentInstanceID,
				DetachedAt: volumeAttachmentTime(now),
			},
		},
		{
			name: "invalid volume UUID",
			input: persistence.CreateVolumeAttachmentInput{
				VolumeID:   "invalid",
				InstanceID: testVolumeAttachmentInstanceID,
			},
			wantErr: true,
		},
		{
			name: "invalid instance UUID",
			input: persistence.CreateVolumeAttachmentInput{
				VolumeID:   testAttachmentVolumeID,
				InstanceID: "invalid",
			},
			wantErr: true,
		},
		{
			name: "missing volume ID",
			input: persistence.CreateVolumeAttachmentInput{
				InstanceID: testVolumeAttachmentInstanceID,
			},
			wantErr: true,
		},
		{
			name: "missing instance ID",
			input: persistence.CreateVolumeAttachmentInput{
				VolumeID: testAttachmentVolumeID,
			},
			wantErr: true,
		},
		{
			name: "mount point too long",
			input: persistence.CreateVolumeAttachmentInput{
				VolumeID:   testAttachmentVolumeID,
				InstanceID: testVolumeAttachmentInstanceID,
				MountPoint: volumeAttachmentString(strings.Repeat("m", 256)),
			},
			wantErr: true,
		},
		{
			name: "status too long",
			input: persistence.CreateVolumeAttachmentInput{
				VolumeID:   testAttachmentVolumeID,
				InstanceID: testVolumeAttachmentInstanceID,
				Status:     volumeAttachmentString(strings.Repeat("s", 51)),
			},
			wantErr: true,
		},
		{
			name: "zero attached timestamp",
			input: persistence.CreateVolumeAttachmentInput{
				VolumeID:   testAttachmentVolumeID,
				InstanceID: testVolumeAttachmentInstanceID,
				AttachedAt: volumeAttachmentTime(time.Time{}),
			},
			wantErr: true,
		},
		{
			name: "zero detached timestamp",
			input: persistence.CreateVolumeAttachmentInput{
				VolumeID:   testAttachmentVolumeID,
				InstanceID: testVolumeAttachmentInstanceID,
				DetachedAt: volumeAttachmentTime(time.Time{}),
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

func TestUpdateVolumeAttachmentValidation(t *testing.T) {
	now := time.Now().UTC()

	tests := []struct {
		name    string
		input   persistence.UpdateVolumeAttachmentInput
		wantErr bool
	}{
		{
			name: "valid update",
			input: persistence.UpdateVolumeAttachmentInput{
				MountPoint: volumeAttachmentString("/mnt/storage"),
				Status:     volumeAttachmentString("attached"),
				AttachedAt: volumeAttachmentTime(now),
			},
		},
		{
			name:  "clear nullable fields",
			input: persistence.UpdateVolumeAttachmentInput{},
		},
		{
			name: "reverse timestamp order permitted",
			input: persistence.UpdateVolumeAttachmentInput{
				AttachedAt: volumeAttachmentTime(now),
				DetachedAt: volumeAttachmentTime(now.Add(-time.Hour)),
			},
		},
		{
			name: "mount point too long",
			input: persistence.UpdateVolumeAttachmentInput{
				MountPoint: volumeAttachmentString(strings.Repeat("m", 256)),
			},
			wantErr: true,
		},
		{
			name: "status too long",
			input: persistence.UpdateVolumeAttachmentInput{
				Status: volumeAttachmentString(strings.Repeat("s", 51)),
			},
			wantErr: true,
		},
		{
			name: "zero attached timestamp",
			input: persistence.UpdateVolumeAttachmentInput{
				AttachedAt: volumeAttachmentTime(time.Time{}),
			},
			wantErr: true,
		},
		{
			name: "zero detached timestamp",
			input: persistence.UpdateVolumeAttachmentInput{
				DetachedAt: volumeAttachmentTime(time.Time{}),
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

func TestVolumeAttachmentErrorSentinels(t *testing.T) {
	if persistence.ErrVolumeAttachmentNotFound == nil {
		t.Fatal("not-found sentinel is nil")
	}
	if persistence.ErrVolumeAttachmentConflict == nil {
		t.Fatal("conflict sentinel is nil")
	}
	if errors.Is(
		persistence.ErrVolumeAttachmentNotFound,
		persistence.ErrVolumeAttachmentConflict,
	) {
		t.Fatal("error sentinels must be distinct")
	}
}

func TestVolumeAttachmentRepositoryNilDatabase(t *testing.T) {
	repo, err := persistence.NewVolumeAttachmentRepository(nil)
	if err == nil {
		t.Fatal("expected error for nil database")
	}
	if repo != nil {
		t.Fatal("expected nil repository")
	}
}
