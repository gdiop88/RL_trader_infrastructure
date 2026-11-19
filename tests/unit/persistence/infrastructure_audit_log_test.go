package persistence_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gdiop88/RL_trader_infrastructure/internal/persistence"
)

const auditLogTestUUID = "11111111-1111-4111-8111-111111111111"

func auditLogTestString(value string) *string {
	return &value
}

func auditLogTestTime(value time.Time) *time.Time {
	return &value
}

func TestCreateInfrastructureAuditLogValidation(t *testing.T) {
	now := time.Now().UTC()

	valid := func() persistence.CreateInfrastructureAuditLogInput {
		return persistence.CreateInfrastructureAuditLogInput{
			Action:     "instance.created",
			OccurredAt: now,
		}
	}

	tests := []struct {
		name    string
		modify  func(*persistence.CreateInfrastructureAuditLogInput)
		wantErr bool
	}{
		{name: "minimum valid"},
		{
			name: "all fields",
			modify: func(in *persistence.CreateInfrastructureAuditLogInput) {
				in.ProjectID = auditLogTestString(auditLogTestUUID)
				in.OperationID = auditLogTestString(auditLogTestUUID)
				in.InstanceID = auditLogTestString(auditLogTestUUID)
				in.ActorReference = auditLogTestString("central-authority")
				in.Action = "instance.provisioned"
				in.ResourceType = auditLogTestString("instance")
				in.ResourceReference = auditLogTestString("instance-001")
				in.Outcome = auditLogTestString("success")
				in.Details = json.RawMessage(`{"provider":"digitalocean"}`)
			},
		},
		{
			name: "maximum lengths",
			modify: func(in *persistence.CreateInfrastructureAuditLogInput) {
				in.ActorReference = auditLogTestString(strings.Repeat("a", 255))
				in.Action = strings.Repeat("b", 100)
				in.ResourceType = auditLogTestString(strings.Repeat("c", 100))
				in.ResourceReference = auditLogTestString(strings.Repeat("d", 255))
				in.Outcome = auditLogTestString(strings.Repeat("e", 50))
			},
		},
		{
			name: "unicode character lengths",
			modify: func(in *persistence.CreateInfrastructureAuditLogInput) {
				in.Action = strings.Repeat("é", 100)
				in.ActorReference = auditLogTestString(strings.Repeat("界", 255))
			},
		},
		{
			name: "empty action permitted by schema",
			modify: func(in *persistence.CreateInfrastructureAuditLogInput) {
				in.Action = ""
			},
		},
		{
			name: "empty optional strings",
			modify: func(in *persistence.CreateInfrastructureAuditLogInput) {
				in.ActorReference = auditLogTestString("")
				in.ResourceType = auditLogTestString("")
				in.ResourceReference = auditLogTestString("")
				in.Outcome = auditLogTestString("")
			},
		},
		{
			name: "JSON object",
			modify: func(in *persistence.CreateInfrastructureAuditLogInput) {
				in.Details = json.RawMessage(`{"event":"created"}`)
			},
		},
		{
			name: "JSON array",
			modify: func(in *persistence.CreateInfrastructureAuditLogInput) {
				in.Details = json.RawMessage(`[1,2,3]`)
			},
		},
		{
			name: "JSON scalar",
			modify: func(in *persistence.CreateInfrastructureAuditLogInput) {
				in.Details = json.RawMessage(`true`)
			},
		},
		{
			name: "JSON null",
			modify: func(in *persistence.CreateInfrastructureAuditLogInput) {
				in.Details = json.RawMessage(`null`)
			},
		},
		{
			name: "invalid project UUID",
			modify: func(in *persistence.CreateInfrastructureAuditLogInput) {
				in.ProjectID = auditLogTestString("invalid")
			},
			wantErr: true,
		},
		{
			name: "invalid operation UUID",
			modify: func(in *persistence.CreateInfrastructureAuditLogInput) {
				in.OperationID = auditLogTestString("invalid")
			},
			wantErr: true,
		},
		{
			name: "invalid instance UUID",
			modify: func(in *persistence.CreateInfrastructureAuditLogInput) {
				in.InstanceID = auditLogTestString("invalid")
			},
			wantErr: true,
		},
		{
			name: "zero occurred_at",
			modify: func(in *persistence.CreateInfrastructureAuditLogInput) {
				in.OccurredAt = time.Time{}
			},
			wantErr: true,
		},
		{
			name: "action too long",
			modify: func(in *persistence.CreateInfrastructureAuditLogInput) {
				in.Action = strings.Repeat("x", 101)
			},
			wantErr: true,
		},
		{
			name: "actor reference too long",
			modify: func(in *persistence.CreateInfrastructureAuditLogInput) {
				in.ActorReference = auditLogTestString(strings.Repeat("x", 256))
			},
			wantErr: true,
		},
		{
			name: "resource type too long",
			modify: func(in *persistence.CreateInfrastructureAuditLogInput) {
				in.ResourceType = auditLogTestString(strings.Repeat("x", 101))
			},
			wantErr: true,
		},
		{
			name: "resource reference too long",
			modify: func(in *persistence.CreateInfrastructureAuditLogInput) {
				in.ResourceReference = auditLogTestString(strings.Repeat("x", 256))
			},
			wantErr: true,
		},
		{
			name: "outcome too long",
			modify: func(in *persistence.CreateInfrastructureAuditLogInput) {
				in.Outcome = auditLogTestString(strings.Repeat("x", 51))
			},
			wantErr: true,
		},
		{
			name: "invalid JSON",
			modify: func(in *persistence.CreateInfrastructureAuditLogInput) {
				in.Details = json.RawMessage(`{"broken":`)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := valid()
			if tt.modify != nil {
				tt.modify(&input)
			}

			err := input.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestListInfrastructureAuditLogsValidation(t *testing.T) {
	now := time.Now().UTC()
	earlier := now.Add(-time.Hour)
	later := now.Add(time.Hour)
	zero := time.Time{}

	tests := []struct {
		name    string
		options persistence.ListInfrastructureAuditLogsOptions
		wantErr bool
	}{
		{name: "defaults"},
		{
			name: "all filters",
			options: persistence.ListInfrastructureAuditLogsOptions{
				ProjectID:         auditLogTestString(auditLogTestUUID),
				OperationID:       auditLogTestString(auditLogTestUUID),
				InstanceID:        auditLogTestString(auditLogTestUUID),
				ActorReference:    auditLogTestString("authority"),
				Action:            auditLogTestString("instance.created"),
				ResourceType:      auditLogTestString("instance"),
				ResourceReference: auditLogTestString("instance-001"),
				Outcome:           auditLogTestString("success"),
				From:              &earlier,
				To:                &later,
				Limit:             100,
				Offset:            10,
			},
		},
		{
			name: "maximum limit",
			options: persistence.ListInfrastructureAuditLogsOptions{
				Limit: 1000,
			},
		},
		{
			name: "equal timestamps",
			options: persistence.ListInfrastructureAuditLogsOptions{
				From: &now,
				To:   &now,
			},
		},
		{
			name: "invalid project UUID",
			options: persistence.ListInfrastructureAuditLogsOptions{
				ProjectID: auditLogTestString("invalid"),
			},
			wantErr: true,
		},
		{
			name: "invalid operation UUID",
			options: persistence.ListInfrastructureAuditLogsOptions{
				OperationID: auditLogTestString("invalid"),
			},
			wantErr: true,
		},
		{
			name: "invalid instance UUID",
			options: persistence.ListInfrastructureAuditLogsOptions{
				InstanceID: auditLogTestString("invalid"),
			},
			wantErr: true,
		},
		{
			name: "action too long",
			options: persistence.ListInfrastructureAuditLogsOptions{
				Action: auditLogTestString(strings.Repeat("x", 101)),
			},
			wantErr: true,
		},
		{
			name: "actor reference too long",
			options: persistence.ListInfrastructureAuditLogsOptions{
				ActorReference: auditLogTestString(strings.Repeat("x", 256)),
			},
			wantErr: true,
		},
		{
			name: "resource type too long",
			options: persistence.ListInfrastructureAuditLogsOptions{
				ResourceType: auditLogTestString(strings.Repeat("x", 101)),
			},
			wantErr: true,
		},
		{
			name: "resource reference too long",
			options: persistence.ListInfrastructureAuditLogsOptions{
				ResourceReference: auditLogTestString(strings.Repeat("x", 256)),
			},
			wantErr: true,
		},
		{
			name: "outcome too long",
			options: persistence.ListInfrastructureAuditLogsOptions{
				Outcome: auditLogTestString(strings.Repeat("x", 51)),
			},
			wantErr: true,
		},
		{
			name: "zero from",
			options: persistence.ListInfrastructureAuditLogsOptions{
				From: &zero,
			},
			wantErr: true,
		},
		{
			name: "zero to",
			options: persistence.ListInfrastructureAuditLogsOptions{
				To: &zero,
			},
			wantErr: true,
		},
		{
			name: "reversed range",
			options: persistence.ListInfrastructureAuditLogsOptions{
				From: &later,
				To:   &earlier,
			},
			wantErr: true,
		},
		{
			name: "negative limit",
			options: persistence.ListInfrastructureAuditLogsOptions{
				Limit: -1,
			},
			wantErr: true,
		},
		{
			name: "limit above maximum",
			options: persistence.ListInfrastructureAuditLogsOptions{
				Limit: 1001,
			},
			wantErr: true,
		},
		{
			name: "negative offset",
			options: persistence.ListInfrastructureAuditLogsOptions{
				Offset: -1,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.options.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestInfrastructureAuditLogErrorSentinels(t *testing.T) {
	if persistence.ErrInfrastructureAuditLogNotFound == nil {
		t.Fatal("not-found sentinel is nil")
	}
	if persistence.ErrInfrastructureAuditLogConflict == nil {
		t.Fatal("conflict sentinel is nil")
	}
	if errors.Is(
		persistence.ErrInfrastructureAuditLogNotFound,
		persistence.ErrInfrastructureAuditLogConflict,
	) {
		t.Fatal("error sentinels must be distinct")
	}
}

func TestInfrastructureAuditLogRepositoryNilDatabase(t *testing.T) {
	repo, err := persistence.NewInfrastructureAuditLogRepository(nil)
	if err == nil || repo != nil {
		t.Fatalf("expected nil repository and error, got %v, %v", repo, err)
	}
}
