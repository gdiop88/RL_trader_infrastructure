package persistence_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gdiop88/RL_trader_infrastructure/internal/persistence"
)

const (
	testAttachmentInstanceID = "11111111-1111-4111-8111-111111111111"
	testAttachmentNetworkID  = "22222222-2222-4222-8222-222222222222"
)

func attachmentString(value string) *string {
	return &value
}

func attachmentBool(value bool) *bool {
	return &value
}

func attachmentTime(value time.Time) *time.Time {
	return &value
}

func TestCreateInstanceNetworkAttachmentValidation(t *testing.T) {
	now := time.Now().UTC()

	tests := []struct {
		name    string
		input   persistence.CreateInstanceNetworkAttachmentInput
		wantErr bool
	}{
		{
			name: "required identifiers only",
			input: persistence.CreateInstanceNetworkAttachmentInput{
				InstanceID: testAttachmentInstanceID,
				NetworkID:  testAttachmentNetworkID,
			},
		},
		{
			name: "complete IPv4 attachment",
			input: persistence.CreateInstanceNetworkAttachmentInput{
				InstanceID:    testAttachmentInstanceID,
				NetworkID:     testAttachmentNetworkID,
				PrivateIP:     attachmentString("10.0.0.10"),
				PublicIP:      attachmentString("203.0.113.10"),
				InterfaceName: attachmentString("eth0"),
				IsPrimary:     attachmentBool(true),
				AttachedAt:    attachmentTime(now),
			},
		},
		{
			name: "IPv6 addresses",
			input: persistence.CreateInstanceNetworkAttachmentInput{
				InstanceID: testAttachmentInstanceID,
				NetworkID:  testAttachmentNetworkID,
				PrivateIP:  attachmentString("fd00::10"),
				PublicIP:   attachmentString("2001:db8::10"),
			},
		},
		{
			name: "nullable primary flag",
			input: persistence.CreateInstanceNetworkAttachmentInput{
				InstanceID: testAttachmentInstanceID,
				NetworkID:  testAttachmentNetworkID,
				IsPrimary:  nil,
			},
		},
		{
			name: "explicit false primary flag",
			input: persistence.CreateInstanceNetworkAttachmentInput{
				InstanceID: testAttachmentInstanceID,
				NetworkID:  testAttachmentNetworkID,
				IsPrimary:  attachmentBool(false),
			},
		},
		{
			name: "maximum interface name length",
			input: persistence.CreateInstanceNetworkAttachmentInput{
				InstanceID:    testAttachmentInstanceID,
				NetworkID:     testAttachmentNetworkID,
				InterfaceName: attachmentString(strings.Repeat("e", 100)),
			},
		},
		{
			name: "timestamps without ordering restriction",
			input: persistence.CreateInstanceNetworkAttachmentInput{
				InstanceID: testAttachmentInstanceID,
				NetworkID:  testAttachmentNetworkID,
				AttachedAt: attachmentTime(now),
				DetachedAt: attachmentTime(now.Add(-time.Hour)),
			},
		},
		{
			name:    "missing required identifiers",
			input:   persistence.CreateInstanceNetworkAttachmentInput{},
			wantErr: true,
		},
		{
			name: "invalid instance UUID",
			input: persistence.CreateInstanceNetworkAttachmentInput{
				InstanceID: "invalid",
				NetworkID:  testAttachmentNetworkID,
			},
			wantErr: true,
		},
		{
			name: "invalid network UUID",
			input: persistence.CreateInstanceNetworkAttachmentInput{
				InstanceID: testAttachmentInstanceID,
				NetworkID:  "invalid",
			},
			wantErr: true,
		},
		{
			name: "invalid private IPv4",
			input: persistence.CreateInstanceNetworkAttachmentInput{
				InstanceID: testAttachmentInstanceID,
				NetworkID:  testAttachmentNetworkID,
				PrivateIP:  attachmentString("999.0.0.1"),
			},
			wantErr: true,
		},
		{
			name: "invalid public IPv6",
			input: persistence.CreateInstanceNetworkAttachmentInput{
				InstanceID: testAttachmentInstanceID,
				NetworkID:  testAttachmentNetworkID,
				PublicIP:   attachmentString("2001:db8::gg"),
			},
			wantErr: true,
		},
		{
			name: "CIDR rejected as individual IP",
			input: persistence.CreateInstanceNetworkAttachmentInput{
				InstanceID: testAttachmentInstanceID,
				NetworkID:  testAttachmentNetworkID,
				PrivateIP:  attachmentString("10.0.0.1/24"),
			},
			wantErr: true,
		},
		{
			name: "empty IP rejected",
			input: persistence.CreateInstanceNetworkAttachmentInput{
				InstanceID: testAttachmentInstanceID,
				NetworkID:  testAttachmentNetworkID,
				PrivateIP:  attachmentString(""),
			},
			wantErr: true,
		},
		{
			name: "null character in IP",
			input: persistence.CreateInstanceNetworkAttachmentInput{
				InstanceID: testAttachmentInstanceID,
				NetworkID:  testAttachmentNetworkID,
				PublicIP:   attachmentString("10.0.0.1\x00"),
			},
			wantErr: true,
		},
		{
			name: "interface name too long",
			input: persistence.CreateInstanceNetworkAttachmentInput{
				InstanceID:    testAttachmentInstanceID,
				NetworkID:     testAttachmentNetworkID,
				InterfaceName: attachmentString(strings.Repeat("e", 101)),
			},
			wantErr: true,
		},
		{
			name: "null character in interface name",
			input: persistence.CreateInstanceNetworkAttachmentInput{
				InstanceID:    testAttachmentInstanceID,
				NetworkID:     testAttachmentNetworkID,
				InterfaceName: attachmentString("eth\x000"),
			},
			wantErr: true,
		},
		{
			name: "zero attached timestamp",
			input: persistence.CreateInstanceNetworkAttachmentInput{
				InstanceID: testAttachmentInstanceID,
				NetworkID:  testAttachmentNetworkID,
				AttachedAt: attachmentTime(time.Time{}),
			},
			wantErr: true,
		},
		{
			name: "zero detached timestamp",
			input: persistence.CreateInstanceNetworkAttachmentInput{
				InstanceID: testAttachmentInstanceID,
				NetworkID:  testAttachmentNetworkID,
				DetachedAt: attachmentTime(time.Time{}),
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

func TestUpdateInstanceNetworkAttachmentValidation(t *testing.T) {
	now := time.Now().UTC()

	tests := []struct {
		name    string
		input   persistence.UpdateInstanceNetworkAttachmentInput
		wantErr bool
	}{
		{
			name:  "clear nullable fields",
			input: persistence.UpdateInstanceNetworkAttachmentInput{},
		},
		{
			name: "valid update",
			input: persistence.UpdateInstanceNetworkAttachmentInput{
				PrivateIP:     attachmentString("192.168.1.10"),
				PublicIP:      attachmentString("198.51.100.10"),
				InterfaceName: attachmentString("ens3"),
				IsPrimary:     attachmentBool(true),
				AttachedAt:    attachmentTime(now),
				DetachedAt:    attachmentTime(now.Add(time.Hour)),
			},
		},
		{
			name: "valid IPv6 update",
			input: persistence.UpdateInstanceNetworkAttachmentInput{
				PrivateIP: attachmentString("fd00::1"),
				PublicIP:  attachmentString("2001:db8::1"),
			},
		},
		{
			name: "invalid private IP",
			input: persistence.UpdateInstanceNetworkAttachmentInput{
				PrivateIP: attachmentString("not-an-ip"),
			},
			wantErr: true,
		},
		{
			name: "invalid public IP",
			input: persistence.UpdateInstanceNetworkAttachmentInput{
				PublicIP: attachmentString("1.2.3.999"),
			},
			wantErr: true,
		},
		{
			name: "interface name too long",
			input: persistence.UpdateInstanceNetworkAttachmentInput{
				InterfaceName: attachmentString(strings.Repeat("x", 101)),
			},
			wantErr: true,
		},
		{
			name: "zero attached timestamp",
			input: persistence.UpdateInstanceNetworkAttachmentInput{
				AttachedAt: attachmentTime(time.Time{}),
			},
			wantErr: true,
		},
		{
			name: "zero detached timestamp",
			input: persistence.UpdateInstanceNetworkAttachmentInput{
				DetachedAt: attachmentTime(time.Time{}),
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

func TestInstanceNetworkAttachmentErrorSentinels(t *testing.T) {
	if persistence.ErrInstanceNetworkAttachmentNotFound == nil {
		t.Fatal("not-found sentinel is nil")
	}

	if persistence.ErrInstanceNetworkAttachmentConflict == nil {
		t.Fatal("conflict sentinel is nil")
	}

	if errors.Is(
		persistence.ErrInstanceNetworkAttachmentNotFound,
		persistence.ErrInstanceNetworkAttachmentConflict,
	) {
		t.Fatal("error sentinels must be distinct")
	}
}

func TestInstanceNetworkAttachmentRepositoryNilDatabase(t *testing.T) {
	repo, err := persistence.NewInstanceNetworkAttachmentRepository(nil)

	if err == nil {
		t.Fatal("expected error for nil database")
	}
	if repo != nil {
		t.Fatal("expected nil repository")
	}
}
