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

func resourceReservationIntegrationString(s string) *string {
	return &s
}

func resourceReservationIntegrationInt(n int) *int {
	return &n
}

func resourceReservationIntegrationTime(t time.Time) *time.Time {
	return &t
}

func TestResourceReservationCRUD(t *testing.T) {
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

	repo, err := persistence.NewResourceReservationRepository(db)
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	now := time.Now().UTC().Truncate(time.Microsecond)

	var projectID, secondProjectID string
	var instanceID string
	var reservationID string
	var reservationIDs []string

	t.Cleanup(func() {
		cleanupCtx, done := context.WithTimeout(
			context.Background(), 30*time.Second,
		)
		defer done()

		for _, id := range reservationIDs {
			_, err := db.Pool().Exec(
				cleanupCtx,
				`DELETE FROM resource_reservations WHERE id = $1`,
				id,
			)
			if err != nil {
				t.Errorf("cleanup reservation %s: %v", id, err)
			}
		}

		if instanceID != "" {
			_, err := db.Pool().Exec(
				cleanupCtx,
				`DELETE FROM infrastructure_instances WHERE id = $1`,
				instanceID,
			)
			if err != nil {
				t.Errorf("cleanup instance: %v", err)
			}
		}

		for _, id := range []string{projectID, secondProjectID} {
			if id == "" {
				continue
			}

			_, err := db.Pool().Exec(
				cleanupCtx,
				`DELETE FROM infrastructure_projects WHERE id = $1`,
				id,
			)
			if err != nil {
				t.Errorf("cleanup project %s: %v", id, err)
			}
		}
	})

	for i, target := range []*string{&projectID, &secondProjectID} {
		err := db.Pool().QueryRow(
			ctx,
			`INSERT INTO infrastructure_projects (name, environment)
			 VALUES ($1, $2)
			 RETURNING id`,
			fmt.Sprintf("reservation-project-%s-%d", suffix, i),
			"integration",
		).Scan(target)
		if err != nil {
			t.Fatalf("create project fixture: %v", err)
		}
	}

	err = db.Pool().QueryRow(
		ctx,
		`INSERT INTO infrastructure_instances (provider, name)
		 VALUES ($1, $2)
		 RETURNING id`,
		"integration",
		"reservation-instance-"+suffix,
	).Scan(&instanceID)
	if err != nil {
		t.Fatalf("create instance fixture: %v", err)
	}

	createFixture := func(
		project string,
		kind string,
		status string,
	) *persistence.ResourceReservation {
		t.Helper()

		item, err := repo.Create(
			ctx,
			persistence.CreateResourceReservationInput{
				ProjectID:       project,
				ReservationType: kind,
				Status:          status,
			},
		)
		if err != nil {
			t.Fatalf("create reservation fixture: %v", err)
		}

		reservationIDs = append(reservationIDs, item.ID)
		return item
	}

	missingID := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"

	t.Run("Create", func(t *testing.T) {
		item, err := repo.Create(
			ctx,
			persistence.CreateResourceReservationInput{
				ProjectID:        projectID,
				InstanceID:       &instanceID,
				ReservationType:  "compute",
				VCPUReserved:     resourceReservationIntegrationInt(4),
				MemoryMBReserved: resourceReservationIntegrationInt(8192),
				GPUReserved:      resourceReservationIntegrationInt(1),
				Status:           "pending",
				StartsAt:         resourceReservationIntegrationTime(now),
				ExpiresAt: resourceReservationIntegrationTime(
					now.Add(time.Hour),
				),
			},
		)
		if err != nil {
			t.Fatal(err)
		}

		reservationID = item.ID
		reservationIDs = append(reservationIDs, item.ID)

		if item.ID == "" {
			t.Fatal("expected generated UUID")
		}
		if item.ProjectID != projectID {
			t.Fatal("project ID mismatch")
		}
		if item.InstanceID == nil || *item.InstanceID != instanceID {
			t.Fatal("instance ID mismatch")
		}
		if item.VCPUReserved == nil || *item.VCPUReserved != 4 {
			t.Fatal("vCPU mismatch")
		}
		if item.MemoryMBReserved == nil ||
			*item.MemoryMBReserved != 8192 {
			t.Fatal("memory mismatch")
		}
		if item.GPUReserved == nil || *item.GPUReserved != 1 {
			t.Fatal("GPU mismatch")
		}
		if item.CreatedAt.IsZero() {
			t.Fatal("expected database-generated created_at")
		}
	})

	t.Run("GetByID", func(t *testing.T) {
		item, err := repo.GetByID(ctx, reservationID)
		if err != nil {
			t.Fatal(err)
		}
		if item.ID != reservationID {
			t.Fatal("reservation ID mismatch")
		}
	})

	t.Run("NullableFields", func(t *testing.T) {
		item := createFixture(
			secondProjectID,
			"storage",
			"pending",
		)

		if item.AgentID != nil ||
			item.InstanceID != nil ||
			item.VCPUReserved != nil ||
			item.MemoryMBReserved != nil ||
			item.GPUReserved != nil ||
			item.StartsAt != nil ||
			item.ExpiresAt != nil {
			t.Fatalf("expected nullable fields: %+v", item)
		}
	})

	t.Run("ZeroResources", func(t *testing.T) {
		item, err := repo.Create(
			ctx,
			persistence.CreateResourceReservationInput{
				ProjectID:        secondProjectID,
				ReservationType:  "compute",
				Status:           "pending",
				VCPUReserved:     resourceReservationIntegrationInt(0),
				MemoryMBReserved: resourceReservationIntegrationInt(0),
				GPUReserved:      resourceReservationIntegrationInt(0),
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		reservationIDs = append(reservationIDs, item.ID)

		if item.VCPUReserved == nil || *item.VCPUReserved != 0 ||
			item.MemoryMBReserved == nil || *item.MemoryMBReserved != 0 ||
			item.GPUReserved == nil || *item.GPUReserved != 0 {
			t.Fatal("zero resource quantities not preserved")
		}
	})

	t.Run("InvalidProjectForeignKey", func(t *testing.T) {
		_, err := repo.Create(
			ctx,
			persistence.CreateResourceReservationInput{
				ProjectID:       missingID,
				ReservationType: "compute",
				Status:          "pending",
			},
		)
		if err == nil {
			t.Fatal("expected project foreign-key error")
		}
	})

	t.Run("InvalidAgentForeignKey", func(t *testing.T) {
		_, err := repo.Create(
			ctx,
			persistence.CreateResourceReservationInput{
				ProjectID:       projectID,
				AgentID:         &missingID,
				ReservationType: "compute",
				Status:          "pending",
			},
		)
		if err == nil {
			t.Fatal("expected agent foreign-key error")
		}
	})

	t.Run("InvalidInstanceForeignKey", func(t *testing.T) {
		_, err := repo.Create(
			ctx,
			persistence.CreateResourceReservationInput{
				ProjectID:       projectID,
				InstanceID:      &missingID,
				ReservationType: "compute",
				Status:          "pending",
			},
		)
		if err == nil {
			t.Fatal("expected instance foreign-key error")
		}
	})

	t.Run("RequiredFields", func(t *testing.T) {
		queries := []struct {
			name  string
			query string
		}{
			{
				name: "missing project",
				query: `INSERT INTO resource_reservations
					(reservation_type, status)
					VALUES ('compute', 'pending')`,
			},
			{
				name: "missing type",
				query: `INSERT INTO resource_reservations
					(project_id, status)
					VALUES ($1, 'pending')`,
			},
			{
				name: "missing status",
				query: `INSERT INTO resource_reservations
					(project_id, reservation_type)
					VALUES ($1, 'compute')`,
			},
		}

		for _, tt := range queries {
			t.Run(tt.name, func(t *testing.T) {
				var err error
				if strings.Contains(tt.query, "$1") {
					_, err = db.Pool().Exec(ctx, tt.query, projectID)
				} else {
					_, err = db.Pool().Exec(ctx, tt.query)
				}
				if err == nil {
					t.Fatal("expected NOT NULL constraint error")
				}
			})
		}
	})

	t.Run("DatabaseNonnegativeConstraint", func(t *testing.T) {
		columns := []string{
			"vcpu_reserved",
			"memory_mb_reserved",
			"gpu_reserved",
		}

		for _, column := range columns {
			t.Run(column, func(t *testing.T) {
				query := fmt.Sprintf(
					`INSERT INTO resource_reservations
					 (project_id, reservation_type, status, %s)
					 VALUES ($1, 'compute', 'pending', -1)`,
					column,
				)

				_, err := db.Pool().Exec(ctx, query, projectID)
				if err == nil {
					t.Fatal("expected nonnegative check constraint error")
				}
			})
		}
	})

	t.Run("ApplicationNonnegativeValidation", func(t *testing.T) {
		for _, input := range []persistence.CreateResourceReservationInput{
			{
				ProjectID:       projectID,
				ReservationType: "compute",
				Status:          "pending",
				VCPUReserved:    resourceReservationIntegrationInt(-1),
			},
			{
				ProjectID:        projectID,
				ReservationType:  "compute",
				Status:           "pending",
				MemoryMBReserved: resourceReservationIntegrationInt(-1),
			},
			{
				ProjectID:       projectID,
				ReservationType: "compute",
				Status:          "pending",
				GPUReserved:     resourceReservationIntegrationInt(-1),
			},
		} {
			if _, err := repo.Create(ctx, input); err == nil {
				t.Fatal("expected negative resource validation error")
			}
		}
	})

	t.Run("ListByProject", func(t *testing.T) {
		items, err := repo.List(
			ctx,
			persistence.ListResourceReservationsOptions{
				ProjectID: &projectID,
			},
		)
		if err != nil {
			t.Fatal(err)
		}

		found := false
		for _, item := range items {
			if item.ProjectID != projectID {
				t.Fatal("unexpected project in filtered list")
			}
			if item.ID == reservationID {
				found = true
			}
		}
		if !found {
			t.Fatal("expected original reservation")
		}
	})

	t.Run("ListByInstance", func(t *testing.T) {
		items, err := repo.List(
			ctx,
			persistence.ListResourceReservationsOptions{
				InstanceID: &instanceID,
			},
		)
		if err != nil {
			t.Fatal(err)
		}

		found := false
		for _, item := range items {
			if item.InstanceID == nil ||
				*item.InstanceID != instanceID {
				t.Fatal("unexpected instance")
			}
			if item.ID == reservationID {
				found = true
			}
		}
		if !found {
			t.Fatal("expected original reservation")
		}
	})

	t.Run("ListByType", func(t *testing.T) {
		items, err := repo.List(
			ctx,
			persistence.ListResourceReservationsOptions{
				ReservationType: resourceReservationIntegrationString("compute"),
				ProjectID:       &projectID,
			},
		)
		if err != nil {
			t.Fatal(err)
		}

		found := false
		for _, item := range items {
			if item.ReservationType != "compute" {
				t.Fatal("unexpected reservation type")
			}
			if item.ID == reservationID {
				found = true
			}
		}
		if !found {
			t.Fatal("expected original reservation")
		}
	})

	t.Run("ListByStatus", func(t *testing.T) {
		items, err := repo.List(
			ctx,
			persistence.ListResourceReservationsOptions{
				ProjectID: &projectID,
				Status: resourceReservationIntegrationString(
					"pending",
				),
			},
		)
		if err != nil {
			t.Fatal(err)
		}

		found := false
		for _, item := range items {
			if item.Status != "pending" {
				t.Fatal("unexpected status")
			}
			if item.ID == reservationID {
				found = true
			}
		}
		if !found {
			t.Fatal("expected original reservation")
		}
	})

	t.Run("Pagination", func(t *testing.T) {
		createFixture(projectID, "compute", "pending")

		first, err := repo.List(
			ctx,
			persistence.ListResourceReservationsOptions{
				ProjectID: &projectID,
				Limit:     1,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if len(first) != 1 {
			t.Fatalf("expected one row, got %d", len(first))
		}

		second, err := repo.List(
			ctx,
			persistence.ListResourceReservationsOptions{
				ProjectID: &projectID,
				Limit:     1,
				Offset:    1,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if len(second) != 1 || second[0].ID == first[0].ID {
			t.Fatal("pagination did not advance")
		}
	})

	t.Run("Update", func(t *testing.T) {
		item, err := repo.Update(
			ctx,
			reservationID,
			persistence.UpdateResourceReservationInput{
				ReservationType:  "gpu_compute",
				Status:           "active",
				VCPUReserved:     resourceReservationIntegrationInt(8),
				MemoryMBReserved: resourceReservationIntegrationInt(16384),
				GPUReserved:      resourceReservationIntegrationInt(2),
				StartsAt:         resourceReservationIntegrationTime(now),
				ExpiresAt: resourceReservationIntegrationTime(
					now.Add(2 * time.Hour),
				),
			},
		)
		if err != nil {
			t.Fatal(err)
		}

		if item.ReservationType != "gpu_compute" ||
			item.Status != "active" ||
			item.VCPUReserved == nil ||
			*item.VCPUReserved != 8 ||
			item.GPUReserved == nil ||
			*item.GPUReserved != 2 {
			t.Fatalf("unexpected updated reservation: %+v", item)
		}

		if item.InstanceID != nil {
			t.Fatal("expected instance ID cleared by full update")
		}
	})

	t.Run("UpdateStatus", func(t *testing.T) {
		item, err := repo.UpdateStatus(
			ctx,
			reservationID,
			"fulfilled",
		)
		if err != nil {
			t.Fatal(err)
		}
		if item.Status != "fulfilled" {
			t.Fatal("status not updated")
		}
	})

	t.Run("UpdateStatusValidation", func(t *testing.T) {
		for _, status := range []string{
			"",
			strings.Repeat("s", 51),
		} {
			_, err := repo.UpdateStatus(ctx, reservationID, status)
			if err == nil {
				t.Fatal("expected invalid status error")
			}
		}
	})

	t.Run("ReverseTimestampOrdering", func(t *testing.T) {
		item, err := repo.Update(
			ctx,
			reservationID,
			persistence.UpdateResourceReservationInput{
				ReservationType: "compute",
				Status:          "active",
				StartsAt: resourceReservationIntegrationTime(
					now,
				),
				ExpiresAt: resourceReservationIntegrationTime(
					now.Add(-time.Hour),
				),
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if item.ExpiresAt == nil ||
			item.StartsAt == nil ||
			!item.ExpiresAt.Before(*item.StartsAt) {
			t.Fatal("expected reversed timestamps to persist")
		}
	})

	t.Run("ClearNullableFields", func(t *testing.T) {
		item, err := repo.Update(
			ctx,
			reservationID,
			persistence.UpdateResourceReservationInput{
				ReservationType: "compute",
				Status:          "active",
			},
		)
		if err != nil {
			t.Fatal(err)
		}

		if item.AgentID != nil ||
			item.InstanceID != nil ||
			item.VCPUReserved != nil ||
			item.MemoryMBReserved != nil ||
			item.GPUReserved != nil ||
			item.StartsAt != nil ||
			item.ExpiresAt != nil {
			t.Fatalf("nullable fields not cleared: %+v", item)
		}
	})

	t.Run("CreatedAtPreserved", func(t *testing.T) {
		item, err := repo.GetByID(ctx, reservationID)
		if err != nil {
			t.Fatal(err)
		}

		if item.CreatedAt.IsZero() {
			t.Fatal("created_at should be populated")
		}
	})

	t.Run("GetMissing", func(t *testing.T) {
		_, err := repo.GetByID(ctx, missingID)
		if !errors.Is(err, persistence.ErrResourceReservationNotFound) {
			t.Fatalf("expected not-found error, got %v", err)
		}
	})

	t.Run("UpdateMissing", func(t *testing.T) {
		_, err := repo.Update(
			ctx,
			missingID,
			persistence.UpdateResourceReservationInput{
				ReservationType: "compute",
				Status:          "pending",
			},
		)
		if !errors.Is(err, persistence.ErrResourceReservationNotFound) {
			t.Fatalf("expected not-found error, got %v", err)
		}
	})

	t.Run("UpdateStatusMissing", func(t *testing.T) {
		_, err := repo.UpdateStatus(ctx, missingID, "active")
		if !errors.Is(err, persistence.ErrResourceReservationNotFound) {
			t.Fatalf("expected not-found error, got %v", err)
		}
	})

	t.Run("Delete", func(t *testing.T) {
		item := createFixture(
			secondProjectID,
			"compute",
			"pending",
		)

		if err := repo.Delete(ctx, item.ID); err != nil {
			t.Fatal(err)
		}

		_, err := repo.GetByID(ctx, item.ID)
		if !errors.Is(err, persistence.ErrResourceReservationNotFound) {
			t.Fatalf("expected deleted reservation to be missing: %v", err)
		}
	})

	t.Run("DeleteMissing", func(t *testing.T) {
		err := repo.Delete(ctx, missingID)
		if !errors.Is(err, persistence.ErrResourceReservationNotFound) {
			t.Fatalf("expected not-found error, got %v", err)
		}
	})

	t.Run("InvalidUUID", func(t *testing.T) {
		if _, err := repo.GetByID(ctx, "invalid"); err == nil {
			t.Fatal("expected invalid UUID error")
		}
		if err := repo.Delete(ctx, "invalid"); err == nil {
			t.Fatal("expected invalid UUID error")
		}
	})

	t.Run("InvalidPagination", func(t *testing.T) {
		_, err := repo.List(
			ctx,
			persistence.ListResourceReservationsOptions{
				Limit: -1,
			},
		)
		if err == nil {
			t.Fatal("expected negative limit error")
		}

		_, err = repo.List(
			ctx,
			persistence.ListResourceReservationsOptions{
				Offset: -1,
			},
		)
		if err == nil {
			t.Fatal("expected negative offset error")
		}
	})
}
