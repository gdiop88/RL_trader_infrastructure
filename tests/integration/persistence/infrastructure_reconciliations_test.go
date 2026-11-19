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

func reconciliationIntegrationString(v string) *string { return &v }
func reconciliationIntegrationBool(v bool) *bool       { return &v }
func reconciliationIntegrationTime(v time.Time) *time.Time {
	return &v
}

func TestInfrastructureReconciliationCRUD(t *testing.T) {
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

	repo, err := persistence.NewInfrastructureReconciliationRepository(db)
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	baseTime := time.Now().UTC().Truncate(time.Microsecond)
	missingID := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"

	var instanceID, secondInstanceID string
	var reconciliationIDs []string

	t.Cleanup(func() {
		cleanupCtx, done := context.WithTimeout(
			context.Background(), 30*time.Second,
		)
		defer done()

		for _, id := range reconciliationIDs {
			_, err := db.Pool().Exec(
				cleanupCtx,
				`DELETE FROM infrastructure_reconciliations WHERE id=$1`,
				id,
			)
			if err != nil {
				t.Errorf("cleanup reconciliation %s: %v", id, err)
			}
		}

		for _, id := range []string{instanceID, secondInstanceID} {
			if id == "" {
				continue
			}
			_, err := db.Pool().Exec(
				cleanupCtx,
				`DELETE FROM infrastructure_instances WHERE id=$1`,
				id,
			)
			if err != nil {
				t.Errorf("cleanup instance %s: %v", id, err)
			}
		}
	})

	for i, target := range []*string{&instanceID, &secondInstanceID} {
		err := db.Pool().QueryRow(
			ctx,
			`INSERT INTO infrastructure_instances (provider, name)
			 VALUES ($1,$2) RETURNING id`,
			"integration",
			fmt.Sprintf("reconciliation-%s-%d", suffix, i),
		).Scan(target)
		if err != nil {
			t.Fatalf("create instance fixture: %v", err)
		}
	}

	createFixture := func(
		instance string,
		checkedAt *time.Time,
	) *persistence.InfrastructureReconciliation {
		t.Helper()

		item, err := repo.Create(
			ctx,
			persistence.CreateInfrastructureReconciliationInput{
				InstanceID: instance,
				CheckedAt:  checkedAt,
			},
		)
		if err != nil {
			t.Fatalf("create reconciliation fixture: %v", err)
		}
		reconciliationIDs = append(reconciliationIDs, item.ID)
		return item
	}

	var primaryID string

	t.Run("CreateAllFields", func(t *testing.T) {
		item, err := repo.Create(
			ctx,
			persistence.CreateInfrastructureReconciliationInput{
				InstanceID:    instanceID,
				DesiredState:  reconciliationIntegrationString("running"),
				ObservedState: reconciliationIntegrationString("stopped"),
				DriftDetected: reconciliationIntegrationBool(true),
				DriftDetails: json.RawMessage(
					`{"reason":"state mismatch","severity":2}`,
				),
				Status:    reconciliationIntegrationString("pending"),
				CheckedAt: reconciliationIntegrationTime(baseTime),
			},
		)
		if err != nil {
			t.Fatal(err)
		}

		primaryID = item.ID
		reconciliationIDs = append(reconciliationIDs, item.ID)

		if item.ID == "" || item.InstanceID != instanceID {
			t.Fatalf("unexpected identity: %+v", item)
		}
		if item.DesiredState == nil || *item.DesiredState != "running" {
			t.Fatal("desired_state mismatch")
		}
		if item.ObservedState == nil || *item.ObservedState != "stopped" {
			t.Fatal("observed_state mismatch")
		}
		if item.DriftDetected == nil || !*item.DriftDetected {
			t.Fatal("drift_detected mismatch")
		}
		if item.Status == nil || *item.Status != "pending" {
			t.Fatal("status mismatch")
		}
		if item.CheckedAt == nil || !item.CheckedAt.Equal(baseTime) {
			t.Fatal("checked_at mismatch")
		}
		if item.ResolvedAt != nil {
			t.Fatal("resolved_at should be NULL")
		}

		var details map[string]any
		if err := json.Unmarshal(item.DriftDetails, &details); err != nil {
			t.Fatal(err)
		}
		if details["reason"] != "state mismatch" {
			t.Fatal("JSONB details mismatch")
		}
	})

	t.Run("GetByID", func(t *testing.T) {
		item, err := repo.GetByID(ctx, primaryID)
		if err != nil {
			t.Fatal(err)
		}
		if item.ID != primaryID || item.InstanceID != instanceID {
			t.Fatal("retrieved reconciliation mismatch")
		}
	})

	t.Run("NullableFields", func(t *testing.T) {
		item := createFixture(secondInstanceID, nil)

		if item.DesiredState != nil ||
			item.ObservedState != nil ||
			item.DriftDetected != nil ||
			item.DriftDetails != nil ||
			item.Status != nil ||
			item.CheckedAt != nil ||
			item.ResolvedAt != nil {
			t.Fatalf("expected nullable fields: %+v", item)
		}
	})

	t.Run("FalseDrift", func(t *testing.T) {
		item, err := repo.Create(
			ctx,
			persistence.CreateInfrastructureReconciliationInput{
				InstanceID:    secondInstanceID,
				DesiredState:  reconciliationIntegrationString("running"),
				ObservedState: reconciliationIntegrationString("running"),
				DriftDetected: reconciliationIntegrationBool(false),
				Status:        reconciliationIntegrationString("healthy"),
				CheckedAt: reconciliationIntegrationTime(
					baseTime.Add(time.Minute),
				),
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		reconciliationIDs = append(reconciliationIDs, item.ID)

		if item.DriftDetected == nil || *item.DriftDetected {
			t.Fatal("expected false drift")
		}
	})

	t.Run("MaximumFieldLengths", func(t *testing.T) {
		item, err := repo.Create(
			ctx,
			persistence.CreateInfrastructureReconciliationInput{
				InstanceID:    secondInstanceID,
				DesiredState:  reconciliationIntegrationString(strings.Repeat("a", 50)),
				ObservedState: reconciliationIntegrationString(strings.Repeat("b", 50)),
				Status:        reconciliationIntegrationString(strings.Repeat("c", 50)),
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		reconciliationIDs = append(reconciliationIDs, item.ID)

		if item.Status == nil || len(*item.Status) != 50 {
			t.Fatal("maximum field length not preserved")
		}
	})

	t.Run("JSONBArray", func(t *testing.T) {
		item, err := repo.Create(
			ctx,
			persistence.CreateInfrastructureReconciliationInput{
				InstanceID:   secondInstanceID,
				DriftDetails: json.RawMessage(`[1,2,3]`),
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		reconciliationIDs = append(reconciliationIDs, item.ID)

		var values []int
		if err := json.Unmarshal(item.DriftDetails, &values); err != nil {
			t.Fatal(err)
		}
		if len(values) != 3 || values[2] != 3 {
			t.Fatal("JSONB array mismatch")
		}
	})

	t.Run("InvalidInstanceForeignKey", func(t *testing.T) {
		_, err := repo.Create(
			ctx,
			persistence.CreateInfrastructureReconciliationInput{
				InstanceID: missingID,
			},
		)
		if err == nil {
			t.Fatal("expected foreign-key violation")
		}
	})

	t.Run("DatabaseNotNull", func(t *testing.T) {
		_, err := db.Pool().Exec(
			ctx,
			`INSERT INTO infrastructure_reconciliations
			 (desired_state) VALUES ('running')`,
		)
		if err == nil {
			t.Fatal("expected NOT NULL violation")
		}
	})

	t.Run("DatabaseFieldLength", func(t *testing.T) {
		_, err := db.Pool().Exec(
			ctx,
			`INSERT INTO infrastructure_reconciliations
			 (instance_id, desired_state) VALUES ($1,$2)`,
			instanceID,
			strings.Repeat("x", 51),
		)
		if err == nil {
			t.Fatal("expected VARCHAR(50) violation")
		}
	})

	t.Run("ApplicationValidation", func(t *testing.T) {
		cases := []persistence.CreateInfrastructureReconciliationInput{
			{InstanceID: "invalid"},
			{
				InstanceID:   instanceID,
				DesiredState: reconciliationIntegrationString(strings.Repeat("a", 51)),
			},
			{
				InstanceID:   instanceID,
				DriftDetails: json.RawMessage(`{invalid}`),
			},
		}

		for i, input := range cases {
			if _, err := repo.Create(ctx, input); err == nil {
				t.Fatalf("case %d: expected validation error", i)
			}
		}
	})

	t.Run("ListByInstance", func(t *testing.T) {
		items, err := repo.List(
			ctx,
			persistence.ListInfrastructureReconciliationsOptions{
				InstanceID: &instanceID,
			},
		)
		if err != nil {
			t.Fatal(err)
		}

		found := false
		for _, item := range items {
			if item.InstanceID != instanceID {
				t.Fatal("unexpected instance in results")
			}
			if item.ID == primaryID {
				found = true
			}
		}
		if !found {
			t.Fatal("primary reconciliation missing")
		}
	})

	t.Run("ListByStatus", func(t *testing.T) {
		status := "pending"
		items, err := repo.List(
			ctx,
			persistence.ListInfrastructureReconciliationsOptions{
				InstanceID: &instanceID,
				Status:     &status,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if len(items) != 1 || items[0].ID != primaryID {
			t.Fatalf("unexpected status results: %+v", items)
		}
	})

	t.Run("ListByDrift", func(t *testing.T) {
		drift := true
		items, err := repo.List(
			ctx,
			persistence.ListInfrastructureReconciliationsOptions{
				InstanceID:    &instanceID,
				DriftDetected: &drift,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if len(items) != 1 || items[0].ID != primaryID {
			t.Fatalf("unexpected drift results: %+v", items)
		}
	})

	t.Run("TimeRangeFiltering", func(t *testing.T) {
		from := baseTime
		to := baseTime

		items, err := repo.List(
			ctx,
			persistence.ListInfrastructureReconciliationsOptions{
				InstanceID: &instanceID,
				From:       &from,
				To:         &to,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if len(items) != 1 || items[0].ID != primaryID {
			t.Fatalf("unexpected time-range results: %+v", items)
		}
	})

	t.Run("PaginationAndOrdering", func(t *testing.T) {
		older := baseTime.Add(-time.Minute)
		newer := baseTime.Add(time.Minute)

		createFixture(instanceID, &older)
		createFixture(instanceID, &newer)

		first, err := repo.List(
			ctx,
			persistence.ListInfrastructureReconciliationsOptions{
				InstanceID: &instanceID,
				Limit:      1,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		second, err := repo.List(
			ctx,
			persistence.ListInfrastructureReconciliationsOptions{
				InstanceID: &instanceID,
				Limit:      1,
				Offset:     1,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if len(first) != 1 || len(second) != 1 ||
			first[0].ID == second[0].ID {
			t.Fatal("pagination failed")
		}
		if first[0].CheckedAt == nil || second[0].CheckedAt == nil ||
			first[0].CheckedAt.Before(*second[0].CheckedAt) {
			t.Fatal("expected descending checked_at ordering")
		}
	})

	t.Run("NullTimestampsLast", func(t *testing.T) {
		createFixture(instanceID, nil)

		items, err := repo.List(
			ctx,
			persistence.ListInfrastructureReconciliationsOptions{
				InstanceID: &instanceID,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if len(items) < 2 || items[len(items)-1].CheckedAt != nil {
			t.Fatal("expected NULL timestamps last")
		}
	})

	t.Run("Update", func(t *testing.T) {
		resolved := baseTime.Add(10 * time.Minute)

		item, err := repo.Update(
			ctx,
			primaryID,
			persistence.UpdateInfrastructureReconciliationInput{
				DesiredState:  reconciliationIntegrationString("running"),
				ObservedState: reconciliationIntegrationString("running"),
				DriftDetected: reconciliationIntegrationBool(false),
				DriftDetails:  json.RawMessage(`{"resolved":true}`),
				Status:        reconciliationIntegrationString("resolved"),
				CheckedAt:     reconciliationIntegrationTime(baseTime),
				ResolvedAt:    &resolved,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if item.InstanceID != instanceID ||
			item.DriftDetected == nil || *item.DriftDetected ||
			item.Status == nil || *item.Status != "resolved" ||
			item.ResolvedAt == nil || !item.ResolvedAt.Equal(resolved) {
			t.Fatalf("update mismatch: %+v", item)
		}
	})

	t.Run("ClearNullableFields", func(t *testing.T) {
		item, err := repo.Update(
			ctx,
			primaryID,
			persistence.UpdateInfrastructureReconciliationInput{},
		)
		if err != nil {
			t.Fatal(err)
		}
		if item.DesiredState != nil ||
			item.ObservedState != nil ||
			item.DriftDetected != nil ||
			item.DriftDetails != nil ||
			item.Status != nil ||
			item.CheckedAt != nil ||
			item.ResolvedAt != nil {
			t.Fatalf("nullable fields not cleared: %+v", item)
		}
		if item.InstanceID != instanceID {
			t.Fatal("update unexpectedly changed instance_id")
		}
	})

	t.Run("GetMissing", func(t *testing.T) {
		_, err := repo.GetByID(ctx, missingID)
		if !errors.Is(
			err,
			persistence.ErrInfrastructureReconciliationNotFound,
		) {
			t.Fatalf("expected not-found error: %v", err)
		}
	})

	t.Run("UpdateMissing", func(t *testing.T) {
		_, err := repo.Update(
			ctx,
			missingID,
			persistence.UpdateInfrastructureReconciliationInput{},
		)
		if !errors.Is(
			err,
			persistence.ErrInfrastructureReconciliationNotFound,
		) {
			t.Fatalf("expected not-found error: %v", err)
		}
	})

	t.Run("Delete", func(t *testing.T) {
		item := createFixture(secondInstanceID, nil)

		if err := repo.Delete(ctx, item.ID); err != nil {
			t.Fatal(err)
		}
		_, err := repo.GetByID(ctx, item.ID)
		if !errors.Is(
			err,
			persistence.ErrInfrastructureReconciliationNotFound,
		) {
			t.Fatalf("expected deleted record to be absent: %v", err)
		}
	})

	t.Run("DeleteMissing", func(t *testing.T) {
		err := repo.Delete(ctx, missingID)
		if !errors.Is(
			err,
			persistence.ErrInfrastructureReconciliationNotFound,
		) {
			t.Fatalf("expected not-found error: %v", err)
		}
	})

	t.Run("InvalidUUID", func(t *testing.T) {
		if _, err := repo.GetByID(ctx, "invalid"); err == nil {
			t.Fatal("expected invalid UUID error")
		}
		if _, err := repo.Update(
			ctx,
			"invalid",
			persistence.UpdateInfrastructureReconciliationInput{},
		); err == nil {
			t.Fatal("expected invalid UUID error")
		}
		if err := repo.Delete(ctx, "invalid"); err == nil {
			t.Fatal("expected invalid UUID error")
		}
	})

	t.Run("InvalidListOptions", func(t *testing.T) {
		badID := "invalid"
		after := baseTime.Add(time.Hour)
		before := baseTime

		cases := []persistence.ListInfrastructureReconciliationsOptions{
			{InstanceID: &badID},
			{Status: reconciliationIntegrationString(strings.Repeat("x", 51))},
			{Limit: -1},
			{Limit: 1001},
			{Offset: -1},
			{From: reconciliationIntegrationTime(time.Time{})},
			{To: reconciliationIntegrationTime(time.Time{})},
			{From: &after, To: &before},
		}

		for i, opts := range cases {
			if _, err := repo.List(ctx, opts); err == nil {
				t.Fatalf("case %d: expected validation error", i)
			}
		}
	})
}
