package persistence_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gdiop88/RL_trader_infrastructure/internal/config"
	"github.com/gdiop88/RL_trader_infrastructure/internal/database"
	"github.com/gdiop88/RL_trader_infrastructure/internal/persistence"
)

func volumeIntegrationString(s string) *string {
	return &s
}

func volumeIntegrationBool(b bool) *bool {
	return &b
}

func volumeIntegrationTime(value time.Time) *time.Time {
	return &value
}

func TestStorageVolumeCRUD(t *testing.T) {
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

	repo, err := persistence.NewStorageVolumeRepository(db)
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	now := time.Now().UTC().Truncate(time.Microsecond)

	var volumeIDs []string
	var volumeID string
	var secondVolumeID string

	t.Cleanup(func() {
		cleanupCtx, done := context.WithTimeout(
			context.Background(), 30*time.Second,
		)
		defer done()

		for _, id := range volumeIDs {
			if _, err := db.Pool().Exec(
				cleanupCtx,
				`DELETE FROM storage_volumes WHERE id = $1`,
				id,
			); err != nil {
				t.Errorf("cleanup volume %s: %v", id, err)
			}
		}
	})

	createVolume := func(
		input persistence.CreateStorageVolumeInput,
	) *persistence.StorageVolume {
		t.Helper()

		volume, err := repo.Create(ctx, input)
		if err != nil {
			t.Fatalf("create fixture volume: %v", err)
		}

		volumeIDs = append(volumeIDs, volume.ID)
		return volume
	}

	t.Run("Create", func(t *testing.T) {
		volume, err := repo.Create(
			ctx,
			persistence.CreateStorageVolumeInput{
				ProviderVolumeID: volumeIntegrationString(
					"vol-" + suffix,
				),
				Name: volumeIntegrationString(
					"integration-volume-" + suffix,
				),
				Region:     volumeIntegrationString("nyc1"),
				SizeGB:     100,
				VolumeType: volumeIntegrationString("ssd"),
				Status:     volumeIntegrationString("available"),
				Encrypted:  volumeIntegrationBool(true),
				Metadata: json.RawMessage(
					`{"environment":"integration","version":1}`,
				),
			},
		)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		volumeID = volume.ID
		volumeIDs = append(volumeIDs, volume.ID)

		if volume.ID == "" {
			t.Fatal("expected generated UUID")
		}
		if volume.SizeGB != 100 {
			t.Fatalf("size_gb = %d; want 100", volume.SizeGB)
		}
		if volume.Name == nil ||
			*volume.Name != "integration-volume-"+suffix {
			t.Fatalf("unexpected name: %v", volume.Name)
		}
		if volume.Encrypted == nil || !*volume.Encrypted {
			t.Fatal("expected encrypted=true")
		}
		if volume.CreatedAt.IsZero() {
			t.Fatal("expected generated created_at")
		}
		if volume.DeletedAt != nil {
			t.Fatal("expected deleted_at=NULL")
		}
		if !json.Valid(volume.Metadata) {
			t.Fatalf("invalid returned JSON: %s", volume.Metadata)
		}
	})

	t.Run("GetByID", func(t *testing.T) {
		volume, err := repo.GetByID(ctx, volumeID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if volume.ID != volumeID {
			t.Fatalf("ID = %s; want %s", volume.ID, volumeID)
		}
		if volume.Status == nil || *volume.Status != "available" {
			t.Fatalf("unexpected status: %v", volume.Status)
		}
	})

	t.Run("JSONBPersistence", func(t *testing.T) {
		volume, err := repo.GetByID(ctx, volumeID)
		if err != nil {
			t.Fatal(err)
		}

		var metadata map[string]any
		if err := json.Unmarshal(volume.Metadata, &metadata); err != nil {
			t.Fatalf("unmarshal metadata: %v", err)
		}
		if metadata["environment"] != "integration" {
			t.Fatalf("unexpected metadata: %v", metadata)
		}
		if metadata["version"] != float64(1) {
			t.Fatalf("unexpected version: %v", metadata["version"])
		}
	})

	t.Run("NullableFields", func(t *testing.T) {
		volume := createVolume(
			persistence.CreateStorageVolumeInput{
				SizeGB: 1,
			},
		)

		if volume.ProjectID != nil ||
			volume.ProviderAccountID != nil ||
			volume.ProviderVolumeID != nil ||
			volume.Name != nil ||
			volume.Region != nil ||
			volume.VolumeType != nil ||
			volume.Status != nil ||
			volume.Encrypted != nil ||
			volume.Metadata != nil ||
			volume.DeletedAt != nil {
			t.Fatalf("expected nullable fields to be nil: %+v", volume)
		}
	})

	t.Run("InvalidForeignKeys", func(t *testing.T) {
		invalidUUID := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"

		_, err := repo.Create(
			ctx,
			persistence.CreateStorageVolumeInput{
				ProjectID: volumeIntegrationString(invalidUUID),
				SizeGB:    10,
			},
		)
		if err == nil {
			t.Fatal("expected project foreign-key error")
		}

		_, err = repo.Create(
			ctx,
			persistence.CreateStorageVolumeInput{
				ProviderAccountID: volumeIntegrationString(invalidUUID),
				SizeGB:            10,
			},
		)
		if err == nil {
			t.Fatal("expected account foreign-key error")
		}
	})

	t.Run("DatabaseSizeConstraint", func(t *testing.T) {
		_, err := db.Pool().Exec(
			ctx,
			`INSERT INTO storage_volumes (size_gb) VALUES (0)`,
		)
		if err == nil {
			t.Fatal("expected database CHECK constraint failure")
		}

		_, err = db.Pool().Exec(
			ctx,
			`INSERT INTO storage_volumes (size_gb) VALUES (-1)`,
		)
		if err == nil {
			t.Fatal("expected negative size CHECK failure")
		}
	})

	t.Run("ApplicationSizeValidation", func(t *testing.T) {
		_, err := repo.Create(
			ctx,
			persistence.CreateStorageVolumeInput{SizeGB: 0},
		)
		if err == nil {
			t.Fatal("expected size validation error")
		}
	})

	t.Run("DuplicateProviderVolumeIDAllowed", func(t *testing.T) {
		providerID := "duplicate-" + suffix

		first := createVolume(
			persistence.CreateStorageVolumeInput{
				ProviderVolumeID: volumeIntegrationString(providerID),
				SizeGB:           10,
			},
		)
		second := createVolume(
			persistence.CreateStorageVolumeInput{
				ProviderVolumeID: volumeIntegrationString(providerID),
				SizeGB:           20,
			},
		)

		if first.ID == second.ID {
			t.Fatal("expected distinct IDs")
		}
	})

	t.Run("ListByRegion", func(t *testing.T) {
		volumes, err := repo.List(
			ctx,
			persistence.ListStorageVolumesOptions{
				Region: volumeIntegrationString("nyc1"),
			},
		)
		if err != nil {
			t.Fatal(err)
		}

		found := false
		for _, volume := range volumes {
			if volume.ID == volumeID {
				found = true
			}
		}
		if !found {
			t.Fatal("expected volume in region-filtered list")
		}
	})

	t.Run("ListByType", func(t *testing.T) {
		volumes, err := repo.List(
			ctx,
			persistence.ListStorageVolumesOptions{
				VolumeType: volumeIntegrationString("ssd"),
			},
		)
		if err != nil {
			t.Fatal(err)
		}

		found := false
		for _, volume := range volumes {
			if volume.ID == volumeID {
				found = true
			}
		}
		if !found {
			t.Fatal("expected volume in type-filtered list")
		}
	})

	t.Run("ListByStatus", func(t *testing.T) {
		volumes, err := repo.List(
			ctx,
			persistence.ListStorageVolumesOptions{
				Status: volumeIntegrationString("available"),
			},
		)
		if err != nil {
			t.Fatal(err)
		}

		found := false
		for _, volume := range volumes {
			if volume.ID == volumeID {
				found = true
			}
		}
		if !found {
			t.Fatal("expected volume in status-filtered list")
		}
	})

	t.Run("ListByEncrypted", func(t *testing.T) {
		volumes, err := repo.List(
			ctx,
			persistence.ListStorageVolumesOptions{
				Encrypted: volumeIntegrationBool(true),
			},
		)
		if err != nil {
			t.Fatal(err)
		}

		found := false
		for _, volume := range volumes {
			if volume.ID == volumeID {
				found = true
			}
		}
		if !found {
			t.Fatal("expected encrypted volume")
		}
	})

	t.Run("Pagination", func(t *testing.T) {
		volumes, err := repo.List(
			ctx,
			persistence.ListStorageVolumesOptions{
				Limit: 1,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if len(volumes) > 1 {
			t.Fatalf("expected at most 1 result, got %d", len(volumes))
		}
	})

	t.Run("Update", func(t *testing.T) {
		volume, err := repo.Update(
			ctx,
			volumeID,
			persistence.UpdateStorageVolumeInput{
				ProviderVolumeID: volumeIntegrationString(
					"updated-" + suffix,
				),
				Name:       volumeIntegrationString("updated-volume"),
				Region:     volumeIntegrationString("sfo3"),
				SizeGB:     250,
				VolumeType: volumeIntegrationString("premium-ssd"),
				Status:     volumeIntegrationString("attached"),
				Encrypted:  volumeIntegrationBool(false),
				Metadata:   json.RawMessage(`{"updated":true}`),
			},
		)
		if err != nil {
			t.Fatalf("Update: %v", err)
		}

		if volume.SizeGB != 250 {
			t.Fatalf("size_gb = %d; want 250", volume.SizeGB)
		}
		if volume.Name == nil || *volume.Name != "updated-volume" {
			t.Fatalf("unexpected name: %v", volume.Name)
		}
		if volume.Encrypted == nil || *volume.Encrypted {
			t.Fatal("expected encrypted=false")
		}

		var metadata map[string]any
		if err := json.Unmarshal(volume.Metadata, &metadata); err != nil {
			t.Fatal(err)
		}
		if metadata["updated"] != true {
			t.Fatalf("unexpected metadata: %v", metadata)
		}
	})

	t.Run("UpdateStatus", func(t *testing.T) {
		volume, err := repo.UpdateStatus(
			ctx,
			volumeID,
			volumeIntegrationString("in-use"),
		)
		if err != nil {
			t.Fatal(err)
		}
		if volume.Status == nil || *volume.Status != "in-use" {
			t.Fatalf("unexpected status: %v", volume.Status)
		}
	})

	t.Run("UpdateStatusNull", func(t *testing.T) {
		volume, err := repo.UpdateStatus(ctx, volumeID, nil)
		if err != nil {
			t.Fatal(err)
		}
		if volume.Status != nil {
			t.Fatalf("expected NULL status, got %v", volume.Status)
		}
	})

	t.Run("ClearNullableFields", func(t *testing.T) {
		volume, err := repo.Update(
			ctx,
			volumeID,
			persistence.UpdateStorageVolumeInput{
				SizeGB: 50,
			},
		)
		if err != nil {
			t.Fatal(err)
		}

		if volume.Name != nil ||
			volume.Region != nil ||
			volume.Status != nil ||
			volume.Encrypted != nil ||
			volume.Metadata != nil {
			t.Fatalf("expected nullable fields cleared: %+v", volume)
		}
	})

	t.Run("SoftDelete", func(t *testing.T) {
		volume, err := repo.SoftDelete(ctx, volumeID, now)
		if err != nil {
			t.Fatal(err)
		}
		if volume.DeletedAt == nil {
			t.Fatal("expected deleted_at")
		}
		if !volume.DeletedAt.Equal(now) {
			t.Fatalf(
				"deleted_at = %v; want %v",
				volume.DeletedAt,
				now,
			)
		}
	})

	t.Run("SoftDeletedExcludedByDefault", func(t *testing.T) {
		volumes, err := repo.List(
			ctx,
			persistence.ListStorageVolumesOptions{},
		)
		if err != nil {
			t.Fatal(err)
		}

		for _, volume := range volumes {
			if volume.ID == volumeID {
				t.Fatal("soft-deleted volume appeared in default list")
			}
		}
	})

	t.Run("IncludeDeleted", func(t *testing.T) {
		volumes, err := repo.List(
			ctx,
			persistence.ListStorageVolumesOptions{
				IncludeDeleted: true,
			},
		)
		if err != nil {
			t.Fatal(err)
		}

		found := false
		for _, volume := range volumes {
			if volume.ID == volumeID {
				found = true
			}
		}
		if !found {
			t.Fatal("expected soft-deleted volume")
		}
	})

	t.Run("GetSoftDeleted", func(t *testing.T) {
		volume, err := repo.GetByID(ctx, volumeID)
		if err != nil {
			t.Fatal(err)
		}
		if volume.DeletedAt == nil {
			t.Fatal("expected soft-deleted volume to remain accessible")
		}
	})

	t.Run("RestoreByUpdate", func(t *testing.T) {
		volume, err := repo.Update(
			ctx,
			volumeID,
			persistence.UpdateStorageVolumeInput{
				SizeGB: 75,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if volume.DeletedAt != nil {
			t.Fatal("expected deleted_at cleared")
		}
	})

	t.Run("GetMissing", func(t *testing.T) {
		_, err := repo.GetByID(
			ctx,
			"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
		)
		if !errors.Is(err, persistence.ErrStorageVolumeNotFound) {
			t.Fatalf("expected not-found error, got %v", err)
		}
	})

	t.Run("UpdateMissing", func(t *testing.T) {
		_, err := repo.Update(
			ctx,
			"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
			persistence.UpdateStorageVolumeInput{
				SizeGB: 10,
			},
		)
		if !errors.Is(err, persistence.ErrStorageVolumeNotFound) {
			t.Fatalf("expected not-found error, got %v", err)
		}
	})

	t.Run("UpdateStatusMissing", func(t *testing.T) {
		_, err := repo.UpdateStatus(
			ctx,
			"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
			volumeIntegrationString("available"),
		)
		if !errors.Is(err, persistence.ErrStorageVolumeNotFound) {
			t.Fatalf("expected not-found error, got %v", err)
		}
	})

	t.Run("SoftDeleteMissing", func(t *testing.T) {
		_, err := repo.SoftDelete(
			ctx,
			"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
			now,
		)
		if !errors.Is(err, persistence.ErrStorageVolumeNotFound) {
			t.Fatalf("expected not-found error, got %v", err)
		}
	})

	t.Run("Delete", func(t *testing.T) {
		second := createVolume(
			persistence.CreateStorageVolumeInput{
				Name:   volumeIntegrationString("delete-test-" + suffix),
				SizeGB: 10,
			},
		)
		secondVolumeID = second.ID

		if err := repo.Delete(ctx, secondVolumeID); err != nil {
			t.Fatalf("Delete: %v", err)
		}

		_, err := repo.GetByID(ctx, secondVolumeID)
		if !errors.Is(err, persistence.ErrStorageVolumeNotFound) {
			t.Fatalf("expected deleted volume to be missing: %v", err)
		}
	})

	t.Run("DeleteMissing", func(t *testing.T) {
		err := repo.Delete(
			ctx,
			"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
		)
		if !errors.Is(err, persistence.ErrStorageVolumeNotFound) {
			t.Fatalf("expected not-found error, got %v", err)
		}
	})

	t.Run("StringLengthConstraint", func(t *testing.T) {
		_, err := repo.Create(
			ctx,
			persistence.CreateStorageVolumeInput{
				Name:   volumeIntegrationString(strings.Repeat("x", 151)),
				SizeGB: 10,
			},
		)
		if err == nil {
			t.Fatal("expected name length validation error")
		}
	})
}
