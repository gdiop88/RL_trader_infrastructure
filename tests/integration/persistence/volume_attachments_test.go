package persistence_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gdiop88/RL_trader_infrastructure/internal/config"
	"github.com/gdiop88/RL_trader_infrastructure/internal/database"
	"github.com/gdiop88/RL_trader_infrastructure/internal/persistence"
)

func volumeAttachString(s string) *string {
	return &s
}

func volumeAttachTime(t time.Time) *time.Time {
	return &t
}

func TestVolumeAttachmentCRUD(t *testing.T) {
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

	repo, err := persistence.NewVolumeAttachmentRepository(db)
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	now := time.Now().UTC().Truncate(time.Microsecond)

	var volumeID, secondVolumeID string
	var instanceID, secondInstanceID string
	var attachmentID string
	var attachmentIDs []string

	t.Cleanup(func() {
		cleanupCtx, done := context.WithTimeout(
			context.Background(), 30*time.Second,
		)
		defer done()

		for _, id := range attachmentIDs {
			_, err := db.Pool().Exec(
				cleanupCtx,
				`DELETE FROM volume_attachments WHERE id = $1`,
				id,
			)
			if err != nil {
				t.Errorf("cleanup attachment %s: %v", id, err)
			}
		}

		for _, id := range []string{volumeID, secondVolumeID} {
			if id == "" {
				continue
			}
			_, err := db.Pool().Exec(
				cleanupCtx,
				`DELETE FROM storage_volumes WHERE id = $1`,
				id,
			)
			if err != nil {
				t.Errorf("cleanup volume %s: %v", id, err)
			}
		}

		for _, id := range []string{instanceID, secondInstanceID} {
			if id == "" {
				continue
			}
			_, err := db.Pool().Exec(
				cleanupCtx,
				`DELETE FROM infrastructure_instances WHERE id = $1`,
				id,
			)
			if err != nil {
				t.Errorf("cleanup instance %s: %v", id, err)
			}
		}
	})

	for i, target := range []*string{&volumeID, &secondVolumeID} {
		err := db.Pool().QueryRow(
			ctx,
			`INSERT INTO storage_volumes (name, size_gb)
			 VALUES ($1, $2) RETURNING id`,
			fmt.Sprintf("attachment-volume-%s-%d", suffix, i),
			10+i,
		).Scan(target)
		if err != nil {
			t.Fatalf("create volume fixture: %v", err)
		}
	}

	for i, target := range []*string{&instanceID, &secondInstanceID} {
		err := db.Pool().QueryRow(
			ctx,
			`INSERT INTO infrastructure_instances (provider, name)
			 VALUES ($1, $2) RETURNING id`,
			"integration",
			fmt.Sprintf("volume-instance-%s-%d", suffix, i),
		).Scan(target)
		if err != nil {
			t.Fatalf("create instance fixture: %v", err)
		}
	}

	createFixture := func(
		volume string,
		instance string,
		status string,
	) *persistence.VolumeAttachment {
		t.Helper()

		attachment, err := repo.Create(
			ctx,
			persistence.CreateVolumeAttachmentInput{
				VolumeID:   volume,
				InstanceID: instance,
				Status:     volumeAttachString(status),
			},
		)
		if err != nil {
			t.Fatalf("create attachment fixture: %v", err)
		}

		attachmentIDs = append(attachmentIDs, attachment.ID)
		return attachment
	}

	t.Run("Create", func(t *testing.T) {
		attachment, err := repo.Create(
			ctx,
			persistence.CreateVolumeAttachmentInput{
				VolumeID:   volumeID,
				InstanceID: instanceID,
				MountPoint: volumeAttachString("/mnt/data"),
				Status:     volumeAttachString("attached"),
				AttachedAt: volumeAttachTime(now),
			},
		)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		attachmentID = attachment.ID
		attachmentIDs = append(attachmentIDs, attachment.ID)

		if attachment.ID == "" {
			t.Fatal("expected generated UUID")
		}
		if attachment.VolumeID != volumeID {
			t.Fatalf("unexpected volume ID: %s", attachment.VolumeID)
		}
		if attachment.InstanceID != instanceID {
			t.Fatalf("unexpected instance ID: %s", attachment.InstanceID)
		}
		if attachment.MountPoint == nil ||
			*attachment.MountPoint != "/mnt/data" {
			t.Fatalf("unexpected mount point: %v", attachment.MountPoint)
		}
		if attachment.AttachedAt == nil ||
			!attachment.AttachedAt.Equal(now) {
			t.Fatalf("unexpected attached_at: %v", attachment.AttachedAt)
		}
	})

	t.Run("GetByID", func(t *testing.T) {
		attachment, err := repo.GetByID(ctx, attachmentID)
		if err != nil {
			t.Fatal(err)
		}
		if attachment.ID != attachmentID {
			t.Fatalf("unexpected ID: %s", attachment.ID)
		}
	})

	t.Run("NullableFields", func(t *testing.T) {
		attachment, err := repo.Create(
			ctx,
			persistence.CreateVolumeAttachmentInput{
				VolumeID:   secondVolumeID,
				InstanceID: secondInstanceID,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		attachmentIDs = append(attachmentIDs, attachment.ID)

		if attachment.MountPoint != nil ||
			attachment.Status != nil ||
			attachment.AttachedAt != nil ||
			attachment.DetachedAt != nil {
			t.Fatalf("expected nullable fields: %+v", attachment)
		}
	})

	t.Run("InvalidForeignKeys", func(t *testing.T) {
		missing := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"

		_, err := repo.Create(
			ctx,
			persistence.CreateVolumeAttachmentInput{
				VolumeID:   missing,
				InstanceID: instanceID,
			},
		)
		if err == nil {
			t.Fatal("expected volume foreign-key error")
		}

		_, err = repo.Create(
			ctx,
			persistence.CreateVolumeAttachmentInput{
				VolumeID:   volumeID,
				InstanceID: missing,
			},
		)
		if err == nil {
			t.Fatal("expected instance foreign-key error")
		}
	})

	t.Run("RequiredFields", func(t *testing.T) {
		_, err := db.Pool().Exec(
			ctx,
			`INSERT INTO volume_attachments (instance_id)
			 VALUES ($1)`,
			instanceID,
		)
		if err == nil {
			t.Fatal("expected volume_id NOT NULL error")
		}

		_, err = db.Pool().Exec(
			ctx,
			`INSERT INTO volume_attachments (volume_id)
			 VALUES ($1)`,
			volumeID,
		)
		if err == nil {
			t.Fatal("expected instance_id NOT NULL error")
		}
	})

	t.Run("DuplicatePairAllowed", func(t *testing.T) {
		first := createFixture(volumeID, secondInstanceID, "attached")
		second := createFixture(volumeID, secondInstanceID, "attached")

		if first.ID == second.ID {
			t.Fatal("expected different attachment IDs")
		}
	})

	t.Run("ListByVolume", func(t *testing.T) {
		items, err := repo.List(
			ctx,
			persistence.ListVolumeAttachmentsOptions{
				VolumeID: &volumeID,
			},
		)
		if err != nil {
			t.Fatal(err)
		}

		found := false
		for _, item := range items {
			if item.VolumeID != volumeID {
				t.Fatal("unexpected volume in filtered list")
			}
			if item.ID == attachmentID {
				found = true
			}
		}
		if !found {
			t.Fatal("expected original attachment")
		}
	})

	t.Run("ListByInstance", func(t *testing.T) {
		items, err := repo.List(
			ctx,
			persistence.ListVolumeAttachmentsOptions{
				InstanceID: &instanceID,
			},
		)
		if err != nil {
			t.Fatal(err)
		}

		found := false
		for _, item := range items {
			if item.InstanceID != instanceID {
				t.Fatal("unexpected instance in filtered list")
			}
			if item.ID == attachmentID {
				found = true
			}
		}
		if !found {
			t.Fatal("expected original attachment")
		}
	})

	t.Run("ListByStatus", func(t *testing.T) {
		items, err := repo.List(
			ctx,
			persistence.ListVolumeAttachmentsOptions{
				Status: volumeAttachString("attached"),
			},
		)
		if err != nil {
			t.Fatal(err)
		}

		found := false
		for _, item := range items {
			if item.Status == nil || *item.Status != "attached" {
				t.Fatal("unexpected status")
			}
			if item.ID == attachmentID {
				found = true
			}
		}
		if !found {
			t.Fatal("expected original attachment")
		}
	})

	t.Run("Pagination", func(t *testing.T) {
		items, err := repo.List(
			ctx,
			persistence.ListVolumeAttachmentsOptions{
				VolumeID: &volumeID,
				Limit:    1,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if len(items) != 1 {
			t.Fatalf("expected one item, got %d", len(items))
		}

		next, err := repo.List(
			ctx,
			persistence.ListVolumeAttachmentsOptions{
				VolumeID: &volumeID,
				Limit:    1,
				Offset:   1,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if len(next) != 1 || next[0].ID == items[0].ID {
			t.Fatal("pagination did not advance")
		}
	})

	t.Run("Update", func(t *testing.T) {
		updated, err := repo.Update(
			ctx,
			attachmentID,
			persistence.UpdateVolumeAttachmentInput{
				MountPoint: volumeAttachString("/mnt/storage"),
				Status:     volumeAttachString("mounting"),
				AttachedAt: volumeAttachTime(now.Add(time.Minute)),
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if updated.MountPoint == nil ||
			*updated.MountPoint != "/mnt/storage" {
			t.Fatal("mount point not updated")
		}
		if updated.Status == nil || *updated.Status != "mounting" {
			t.Fatal("status not updated")
		}
	})

	t.Run("UpdateStatus", func(t *testing.T) {
		updated, err := repo.UpdateStatus(
			ctx,
			attachmentID,
			volumeAttachString("attached"),
		)
		if err != nil {
			t.Fatal(err)
		}
		if updated.Status == nil || *updated.Status != "attached" {
			t.Fatal("status not persisted")
		}
	})

	t.Run("UpdateStatusNull", func(t *testing.T) {
		updated, err := repo.UpdateStatus(ctx, attachmentID, nil)
		if err != nil {
			t.Fatal(err)
		}
		if updated.Status != nil {
			t.Fatal("expected NULL status")
		}
	})

	t.Run("TimestampOrderingNotEnforced", func(t *testing.T) {
		updated, err := repo.Update(
			ctx,
			attachmentID,
			persistence.UpdateVolumeAttachmentInput{
				AttachedAt: volumeAttachTime(now),
				DetachedAt: volumeAttachTime(now.Add(-time.Hour)),
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if updated.DetachedAt == nil ||
			!updated.DetachedAt.Before(*updated.AttachedAt) {
			t.Fatal("expected reversed timestamps to persist")
		}
	})

	t.Run("ClearNullableFields", func(t *testing.T) {
		updated, err := repo.Update(
			ctx,
			attachmentID,
			persistence.UpdateVolumeAttachmentInput{},
		)
		if err != nil {
			t.Fatal(err)
		}
		if updated.MountPoint != nil ||
			updated.Status != nil ||
			updated.AttachedAt != nil ||
			updated.DetachedAt != nil {
			t.Fatalf("nullable fields not cleared: %+v", updated)
		}
	})

	t.Run("Detach", func(t *testing.T) {
		updated, err := repo.Detach(ctx, attachmentID, now)
		if err != nil {
			t.Fatal(err)
		}
		if updated.DetachedAt == nil ||
			!updated.DetachedAt.Equal(now) {
			t.Fatal("detach timestamp not persisted")
		}
	})

	t.Run("DetachedExcludedByDefault", func(t *testing.T) {
		items, err := repo.List(
			ctx,
			persistence.ListVolumeAttachmentsOptions{
				VolumeID: &volumeID,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range items {
			if item.ID == attachmentID {
				t.Fatal("detached attachment appeared in default list")
			}
		}
	})

	t.Run("IncludeDetached", func(t *testing.T) {
		items, err := repo.List(
			ctx,
			persistence.ListVolumeAttachmentsOptions{
				VolumeID:        &volumeID,
				IncludeDetached: true,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, item := range items {
			if item.ID == attachmentID {
				found = true
			}
		}
		if !found {
			t.Fatal("expected detached attachment")
		}
	})

	t.Run("GetDetached", func(t *testing.T) {
		item, err := repo.GetByID(ctx, attachmentID)
		if err != nil {
			t.Fatal(err)
		}
		if item.DetachedAt == nil {
			t.Fatal("expected detached timestamp")
		}
	})

	t.Run("RestoreByUpdate", func(t *testing.T) {
		item, err := repo.Update(
			ctx,
			attachmentID,
			persistence.UpdateVolumeAttachmentInput{
				Status: volumeAttachString("attached"),
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if item.DetachedAt != nil {
			t.Fatal("expected detached_at cleared")
		}
	})

	missingID := "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"

	t.Run("GetMissing", func(t *testing.T) {
		_, err := repo.GetByID(ctx, missingID)
		if !errors.Is(err, persistence.ErrVolumeAttachmentNotFound) {
			t.Fatalf("expected not-found error, got %v", err)
		}
	})

	t.Run("UpdateMissing", func(t *testing.T) {
		_, err := repo.Update(
			ctx,
			missingID,
			persistence.UpdateVolumeAttachmentInput{},
		)
		if !errors.Is(err, persistence.ErrVolumeAttachmentNotFound) {
			t.Fatalf("expected not-found error, got %v", err)
		}
	})

	t.Run("UpdateStatusMissing", func(t *testing.T) {
		_, err := repo.UpdateStatus(
			ctx,
			missingID,
			volumeAttachString("attached"),
		)
		if !errors.Is(err, persistence.ErrVolumeAttachmentNotFound) {
			t.Fatalf("expected not-found error, got %v", err)
		}
	})

	t.Run("DetachMissing", func(t *testing.T) {
		_, err := repo.Detach(ctx, missingID, now)
		if !errors.Is(err, persistence.ErrVolumeAttachmentNotFound) {
			t.Fatalf("expected not-found error, got %v", err)
		}
	})

	t.Run("Delete", func(t *testing.T) {
		item := createFixture(
			secondVolumeID,
			instanceID,
			"attached",
		)
		if err := repo.Delete(ctx, item.ID); err != nil {
			t.Fatal(err)
		}

		_, err := repo.GetByID(ctx, item.ID)
		if !errors.Is(err, persistence.ErrVolumeAttachmentNotFound) {
			t.Fatalf("expected deleted record to be missing: %v", err)
		}
	})

	t.Run("DeleteMissing", func(t *testing.T) {
		err := repo.Delete(ctx, missingID)
		if !errors.Is(err, persistence.ErrVolumeAttachmentNotFound) {
			t.Fatalf("expected not-found error, got %v", err)
		}
	})

	t.Run("StringLengthConstraint", func(t *testing.T) {
		_, err := repo.Create(
			ctx,
			persistence.CreateVolumeAttachmentInput{
				VolumeID:   volumeID,
				InstanceID: instanceID,
				MountPoint: volumeAttachString(strings.Repeat("x", 256)),
			},
		)
		if err == nil {
			t.Fatal("expected mount point validation error")
		}
	})
}
