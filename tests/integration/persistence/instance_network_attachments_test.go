package persistence_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/gdiop88/RL_trader_infrastructure/internal/config"
	"github.com/gdiop88/RL_trader_infrastructure/internal/database"
	"github.com/gdiop88/RL_trader_infrastructure/internal/persistence"
)

func attachmentIntegrationString(s string) *string {
	return &s
}

func attachmentIntegrationBool(b bool) *bool {
	return &b
}

func attachmentIntegrationTime(t time.Time) *time.Time {
	return &t
}

func TestInstanceNetworkAttachmentCRUD(t *testing.T) {
	cfg, err := config.Load("../../../configs/server.yaml")
	if err != nil {
		t.Fatalf("load configuration: %v", err)
	}

	connectCtx, connectCancel := context.WithTimeout(
		context.Background(), 15*time.Second,
	)
	defer connectCancel()

	db, err := database.ConnectWithConfig(connectCtx, cfg)
	if err != nil {
		t.Fatalf("connect PostgreSQL: %v", err)
	}
	t.Cleanup(db.Close)

	ctx, cancel := context.WithTimeout(
		context.Background(), 120*time.Second,
	)
	defer cancel()

	repo, err := persistence.NewInstanceNetworkAttachmentRepository(db)
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	now := time.Now().UTC().Truncate(time.Microsecond)

	var instanceID, networkID, secondNetworkID string
	var attachmentIDs []string

	t.Cleanup(func() {
		cleanupCtx, done := context.WithTimeout(
			context.Background(), 30*time.Second,
		)
		defer done()

		for _, id := range attachmentIDs {
			if _, err := db.Pool().Exec(
				cleanupCtx,
				`DELETE FROM instance_network_attachments WHERE id = $1`,
				id,
			); err != nil {
				t.Errorf("cleanup attachment %s: %v", id, err)
			}
		}

		for _, id := range []string{networkID, secondNetworkID} {
			if id == "" {
				continue
			}
			if _, err := db.Pool().Exec(
				cleanupCtx,
				`DELETE FROM network_resources WHERE id = $1`,
				id,
			); err != nil {
				t.Errorf("cleanup network %s: %v", id, err)
			}
		}

		if instanceID != "" {
			if _, err := db.Pool().Exec(
				cleanupCtx,
				`DELETE FROM infrastructure_instances WHERE id = $1`,
				instanceID,
			); err != nil {
				t.Errorf("cleanup instance: %v", err)
			}
		}
	})

	// Create isolated foreign-key fixtures.

	err = db.Pool().QueryRow(
		ctx,
		`INSERT INTO infrastructure_instances (provider, name)
		 VALUES ($1, $2)
		 RETURNING id`,
		"integration",
		"attachment-instance-"+suffix,
	).Scan(&instanceID)
	if err != nil {
		t.Fatalf("create instance fixture: %v", err)
	}

	err = db.Pool().QueryRow(
		ctx,
		`INSERT INTO network_resources (name, cidr)
		 VALUES ($1, $2::cidr)
		 RETURNING id`,
		"attachment-network-"+suffix,
		"10.40.0.0/16",
	).Scan(&networkID)
	if err != nil {
		t.Fatalf("create network fixture: %v", err)
	}

	err = db.Pool().QueryRow(
		ctx,
		`INSERT INTO network_resources (name, cidr)
		 VALUES ($1, $2::cidr)
		 RETURNING id`,
		"attachment-network-secondary-"+suffix,
		"fd00:40::/64",
	).Scan(&secondNetworkID)
	if err != nil {
		t.Fatalf("create second network fixture: %v", err)
	}

	first, err := repo.Create(
		ctx,
		persistence.CreateInstanceNetworkAttachmentInput{
			InstanceID:    instanceID,
			NetworkID:     networkID,
			PrivateIP:     attachmentIntegrationString("10.40.0.10"),
			PublicIP:      attachmentIntegrationString("203.0.113.10"),
			InterfaceName: attachmentIntegrationString("eth0"),
			IsPrimary:     attachmentIntegrationBool(true),
			AttachedAt:    attachmentIntegrationTime(now),
		},
	)
	if err != nil {
		t.Fatalf("create first attachment: %v", err)
	}
	attachmentIDs = append(attachmentIDs, first.ID)

	t.Run("Create", func(t *testing.T) {
		if first.ID == "" {
			t.Fatal("expected generated ID")
		}
		if first.InstanceID != instanceID {
			t.Fatal("instance ID mismatch")
		}
		if first.NetworkID != networkID {
			t.Fatal("network ID mismatch")
		}
		if first.PrivateIP == nil || *first.PrivateIP != "10.40.0.10" {
			t.Fatalf("private IP mismatch: %v", first.PrivateIP)
		}
		if first.PublicIP == nil || *first.PublicIP != "203.0.113.10" {
			t.Fatalf("public IP mismatch: %v", first.PublicIP)
		}
		if first.IsPrimary == nil || !*first.IsPrimary {
			t.Fatal("expected primary attachment")
		}
		if first.AttachedAt == nil || !first.AttachedAt.Equal(now) {
			t.Fatal("attached_at mismatch")
		}
	})

	t.Run("GetByID", func(t *testing.T) {
		got, err := repo.GetByID(ctx, first.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.ID != first.ID {
			t.Fatal("incorrect attachment returned")
		}
	})

	t.Run("InvalidForeignKeys", func(t *testing.T) {
		missingID := "33333333-3333-4333-8333-333333333333"

		_, err := repo.Create(
			ctx,
			persistence.CreateInstanceNetworkAttachmentInput{
				InstanceID: missingID,
				NetworkID:  networkID,
			},
		)
		if err == nil {
			t.Fatal("expected instance foreign-key error")
		}

		_, err = repo.Create(
			ctx,
			persistence.CreateInstanceNetworkAttachmentInput{
				InstanceID: instanceID,
				NetworkID:  missingID,
			},
		)
		if err == nil {
			t.Fatal("expected network foreign-key error")
		}
	})

	second, err := repo.Create(
		ctx,
		persistence.CreateInstanceNetworkAttachmentInput{
			InstanceID: instanceID,
			NetworkID:  secondNetworkID,
			PrivateIP:  attachmentIntegrationString("fd00:40::10"),
			PublicIP:   attachmentIntegrationString("2001:db8::10"),
			IsPrimary:  attachmentIntegrationBool(false),
		},
	)
	if err != nil {
		t.Fatalf("create second attachment: %v", err)
	}
	attachmentIDs = append(attachmentIDs, second.ID)

	t.Run("IPv6Persistence", func(t *testing.T) {
		got, err := repo.GetByID(ctx, second.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.PrivateIP == nil || *got.PrivateIP != "fd00:40::10" {
			t.Fatalf("IPv6 private IP mismatch: %v", got.PrivateIP)
		}
		if got.PublicIP == nil || *got.PublicIP != "2001:db8::10" {
			t.Fatalf("IPv6 public IP mismatch: %v", got.PublicIP)
		}
	})

	t.Run("DefaultPrimaryFalse", func(t *testing.T) {
		got, err := repo.Create(
			ctx,
			persistence.CreateInstanceNetworkAttachmentInput{
				InstanceID: instanceID,
				NetworkID:  networkID,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		attachmentIDs = append(attachmentIDs, got.ID)

		if got.IsPrimary == nil || *got.IsPrimary {
			t.Fatal("expected default is_primary=false")
		}
		if got.PrivateIP != nil || got.PublicIP != nil ||
			got.InterfaceName != nil ||
			got.AttachedAt != nil || got.DetachedAt != nil {
			t.Fatal("expected optional fields to be NULL")
		}
	})

	t.Run("DuplicatePairAllowed", func(t *testing.T) {
		got, err := repo.Create(
			ctx,
			persistence.CreateInstanceNetworkAttachmentInput{
				InstanceID: instanceID,
				NetworkID:  networkID,
			},
		)
		if err != nil {
			t.Fatalf("duplicate instance/network pair rejected: %v", err)
		}
		attachmentIDs = append(attachmentIDs, got.ID)
	})

	t.Run("MultiplePrimaryAllowed", func(t *testing.T) {
		got, err := repo.Create(
			ctx,
			persistence.CreateInstanceNetworkAttachmentInput{
				InstanceID: instanceID,
				NetworkID:  secondNetworkID,
				IsPrimary:  attachmentIntegrationBool(true),
			},
		)
		if err != nil {
			t.Fatalf("second primary attachment rejected: %v", err)
		}
		attachmentIDs = append(attachmentIDs, got.ID)
	})

	t.Run("ListByInstance", func(t *testing.T) {
		rows, err := repo.List(
			ctx,
			persistence.ListInstanceNetworkAttachmentsOptions{
				InstanceID: &instanceID,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 5 {
			t.Fatalf("expected 5 attachments, got %d", len(rows))
		}
	})

	t.Run("ListByNetwork", func(t *testing.T) {
		rows, err := repo.List(
			ctx,
			persistence.ListInstanceNetworkAttachmentsOptions{
				NetworkID: &secondNetworkID,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 2 {
			t.Fatalf("expected 2 attachments, got %d", len(rows))
		}
	})

	t.Run("ListByPrimary", func(t *testing.T) {
		rows, err := repo.List(
			ctx,
			persistence.ListInstanceNetworkAttachmentsOptions{
				InstanceID: &instanceID,
				IsPrimary:  attachmentIntegrationBool(true),
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 2 {
			t.Fatalf("expected 2 primary attachments, got %d", len(rows))
		}
	})

	t.Run("ListByNonPrimary", func(t *testing.T) {
		rows, err := repo.List(
			ctx,
			persistence.ListInstanceNetworkAttachmentsOptions{
				InstanceID: &instanceID,
				IsPrimary:  attachmentIntegrationBool(false),
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 3 {
			t.Fatalf("expected 3 non-primary attachments, got %d", len(rows))
		}
	})

	t.Run("Pagination", func(t *testing.T) {
		rows, err := repo.List(
			ctx,
			persistence.ListInstanceNetworkAttachmentsOptions{
				InstanceID: &instanceID,
				Limit:      2,
				Offset:     1,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 2 {
			t.Fatalf("expected 2 paginated attachments, got %d", len(rows))
		}
	})

	t.Run("Update", func(t *testing.T) {
		updated, err := repo.Update(
			ctx,
			first.ID,
			persistence.UpdateInstanceNetworkAttachmentInput{
				PrivateIP:     attachmentIntegrationString("10.40.0.20"),
				PublicIP:      attachmentIntegrationString("198.51.100.20"),
				InterfaceName: attachmentIntegrationString("ens3"),
				IsPrimary:     attachmentIntegrationBool(false),
				AttachedAt:    attachmentIntegrationTime(now),
			},
		)
		if err != nil {
			t.Fatal(err)
		}

		if updated.PrivateIP == nil ||
			*updated.PrivateIP != "10.40.0.20" {
			t.Fatal("private IP not updated")
		}
		if updated.PublicIP == nil ||
			*updated.PublicIP != "198.51.100.20" {
			t.Fatal("public IP not updated")
		}
		if updated.InterfaceName == nil ||
			*updated.InterfaceName != "ens3" {
			t.Fatal("interface name not updated")
		}
		if updated.IsPrimary == nil || *updated.IsPrimary {
			t.Fatal("primary flag not updated")
		}
		if updated.InstanceID != instanceID ||
			updated.NetworkID != networkID {
			t.Fatal("foreign keys changed unexpectedly")
		}
	})

	t.Run("SetPrimary", func(t *testing.T) {
		got, err := repo.SetPrimary(
			ctx, first.ID, attachmentIntegrationBool(true),
		)
		if err != nil {
			t.Fatal(err)
		}
		if got.IsPrimary == nil || !*got.IsPrimary {
			t.Fatal("expected primary=true")
		}
	})

	t.Run("SetPrimaryNull", func(t *testing.T) {
		got, err := repo.SetPrimary(ctx, first.ID, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got.IsPrimary != nil {
			t.Fatal("expected NULL primary flag")
		}
	})

	t.Run("Detach", func(t *testing.T) {
		detachedAt := now.Add(time.Hour)

		got, err := repo.Detach(ctx, second.ID, detachedAt)
		if err != nil {
			t.Fatal(err)
		}
		if got.DetachedAt == nil ||
			!got.DetachedAt.Equal(detachedAt) {
			t.Fatal("detached_at mismatch")
		}
	})

	t.Run("ClearNullableFields", func(t *testing.T) {
		got, err := repo.Update(
			ctx,
			second.ID,
			persistence.UpdateInstanceNetworkAttachmentInput{},
		)
		if err != nil {
			t.Fatal(err)
		}
		if got.PrivateIP != nil ||
			got.PublicIP != nil ||
			got.InterfaceName != nil ||
			got.IsPrimary != nil ||
			got.AttachedAt != nil ||
			got.DetachedAt != nil {
			t.Fatal("nullable fields were not cleared")
		}
	})

	t.Run("TimestampOrderingNotEnforced", func(t *testing.T) {
		got, err := repo.Update(
			ctx,
			second.ID,
			persistence.UpdateInstanceNetworkAttachmentInput{
				AttachedAt: attachmentIntegrationTime(now),
				DetachedAt: attachmentIntegrationTime(now.Add(-time.Hour)),
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if got.DetachedAt == nil ||
			!got.DetachedAt.Equal(now.Add(-time.Hour)) {
			t.Fatal("timestamp ordering was unexpectedly changed")
		}
	})

	missingID := "44444444-4444-4444-8444-444444444444"

	t.Run("GetMissing", func(t *testing.T) {
		_, err := repo.GetByID(ctx, missingID)
		if !errors.Is(
			err, persistence.ErrInstanceNetworkAttachmentNotFound,
		) {
			t.Fatalf("expected not found, got %v", err)
		}
	})

	t.Run("UpdateMissing", func(t *testing.T) {
		_, err := repo.Update(
			ctx, missingID,
			persistence.UpdateInstanceNetworkAttachmentInput{},
		)
		if !errors.Is(
			err, persistence.ErrInstanceNetworkAttachmentNotFound,
		) {
			t.Fatalf("expected not found, got %v", err)
		}
	})

	t.Run("SetPrimaryMissing", func(t *testing.T) {
		_, err := repo.SetPrimary(
			ctx, missingID, attachmentIntegrationBool(true),
		)
		if !errors.Is(
			err, persistence.ErrInstanceNetworkAttachmentNotFound,
		) {
			t.Fatalf("expected not found, got %v", err)
		}
	})

	t.Run("DetachMissing", func(t *testing.T) {
		_, err := repo.Detach(ctx, missingID, now)
		if !errors.Is(
			err, persistence.ErrInstanceNetworkAttachmentNotFound,
		) {
			t.Fatalf("expected not found, got %v", err)
		}
	})

	t.Run("Delete", func(t *testing.T) {
		if err := repo.Delete(ctx, second.ID); err != nil {
			t.Fatal(err)
		}

		_, err := repo.GetByID(ctx, second.ID)
		if !errors.Is(
			err, persistence.ErrInstanceNetworkAttachmentNotFound,
		) {
			t.Fatalf("expected deleted attachment missing, got %v", err)
		}
	})

	t.Run("DeleteMissing", func(t *testing.T) {
		err := repo.Delete(ctx, missingID)
		if !errors.Is(
			err, persistence.ErrInstanceNetworkAttachmentNotFound,
		) {
			t.Fatalf("expected not found, got %v", err)
		}
	})
}
