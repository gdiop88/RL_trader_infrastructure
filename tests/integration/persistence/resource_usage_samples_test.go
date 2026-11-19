package persistence_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/gdiop88/RL_trader_infrastructure/internal/config"
	"github.com/gdiop88/RL_trader_infrastructure/internal/database"
	"github.com/gdiop88/RL_trader_infrastructure/internal/persistence"
)

func usageIntegrationString(v string) *string { return &v }
func usageIntegrationInt64(v int64) *int64    { return &v }
func usageIntegrationTime(v time.Time) *time.Time {
	return &v
}

func TestResourceUsageSampleCRUD(t *testing.T) {
	cfg, err := config.Load("../../../configs/server.yaml")
	if err != nil {
		t.Fatalf("load config: %v", err)
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

	repo, err := persistence.NewResourceUsageSampleRepository(db)
	if err != nil {
		t.Fatalf("new repository: %v", err)
	}

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	baseTime := time.Now().UTC().Truncate(time.Microsecond)
	missingID := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"

	var instanceID, secondInstanceID string
	var sampleIDs []string

	t.Cleanup(func() {
		cleanupCtx, done := context.WithTimeout(
			context.Background(), 30*time.Second,
		)
		defer done()

		for _, id := range sampleIDs {
			if _, err := db.Pool().Exec(
				cleanupCtx,
				`DELETE FROM resource_usage_samples WHERE id=$1`,
				id,
			); err != nil {
				t.Errorf("cleanup sample %s: %v", id, err)
			}
		}

		for _, id := range []string{instanceID, secondInstanceID} {
			if id == "" {
				continue
			}
			if _, err := db.Pool().Exec(
				cleanupCtx,
				`DELETE FROM infrastructure_instances WHERE id=$1`,
				id,
			); err != nil {
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
			fmt.Sprintf("usage-sample-%s-%d", suffix, i),
		).Scan(target)
		if err != nil {
			t.Fatalf("create instance fixture: %v", err)
		}
	}

	createSample := func(
		instance string,
		at time.Time,
	) *persistence.ResourceUsageSample {
		t.Helper()
		item, err := repo.Create(
			ctx,
			persistence.CreateResourceUsageSampleInput{
				InstanceID:  instance,
				CollectedAt: at,
			},
		)
		if err != nil {
			t.Fatalf("create sample fixture: %v", err)
		}
		sampleIDs = append(sampleIDs, item.ID)
		return item
	}

	var primaryID string

	t.Run("CreateAllMetrics", func(t *testing.T) {
		item, err := repo.Create(
			ctx,
			persistence.CreateResourceUsageSampleInput{
				InstanceID:     instanceID,
				CollectedAt:    baseTime,
				CPUPercent:     usageIntegrationString("75.125"),
				MemoryUsedMB:   usageIntegrationInt64(8192),
				DiskUsedGB:     usageIntegrationString("125.500"),
				NetworkRXBytes: usageIntegrationInt64(123456),
				NetworkTXBytes: usageIntegrationInt64(654321),
				GPUPercent:     usageIntegrationString("88.250"),
				Metrics:        json.RawMessage(`{"temperature":65,"healthy":true}`),
			},
		)
		if err != nil {
			t.Fatal(err)
		}

		primaryID = item.ID
		sampleIDs = append(sampleIDs, item.ID)

		if item.ID == "" || item.InstanceID != instanceID {
			t.Fatalf("unexpected identity: %+v", item)
		}
		if !item.CollectedAt.Equal(baseTime) {
			t.Fatalf("timestamp mismatch: %v", item.CollectedAt)
		}
		if item.CPUPercent == nil || *item.CPUPercent != "75.125" {
			t.Fatalf("CPU mismatch: %v", item.CPUPercent)
		}
		if item.MemoryUsedMB == nil || *item.MemoryUsedMB != 8192 {
			t.Fatal("memory mismatch")
		}
		if item.DiskUsedGB == nil || *item.DiskUsedGB != "125.500" {
			t.Fatal("disk mismatch")
		}
		if item.NetworkRXBytes == nil || *item.NetworkRXBytes != 123456 {
			t.Fatal("RX mismatch")
		}
		if item.NetworkTXBytes == nil || *item.NetworkTXBytes != 654321 {
			t.Fatal("TX mismatch")
		}
		if item.GPUPercent == nil || *item.GPUPercent != "88.250" {
			t.Fatal("GPU mismatch")
		}

		var metrics map[string]any
		if err := json.Unmarshal(item.Metrics, &metrics); err != nil {
			t.Fatal(err)
		}
		if metrics["healthy"] != true {
			t.Fatal("JSONB metrics mismatch")
		}
	})

	t.Run("GetByID", func(t *testing.T) {
		item, err := repo.GetByID(ctx, primaryID)
		if err != nil {
			t.Fatal(err)
		}
		if item.ID != primaryID || item.InstanceID != instanceID {
			t.Fatal("retrieved sample mismatch")
		}
	})

	t.Run("NullableMetrics", func(t *testing.T) {
		item := createSample(
			secondInstanceID,
			baseTime.Add(time.Minute),
		)
		if item.CPUPercent != nil ||
			item.MemoryUsedMB != nil ||
			item.DiskUsedGB != nil ||
			item.NetworkRXBytes != nil ||
			item.NetworkTXBytes != nil ||
			item.GPUPercent != nil ||
			item.Metrics != nil {
			t.Fatalf("expected null metrics: %+v", item)
		}
	})

	t.Run("ZeroMetrics", func(t *testing.T) {
		item, err := repo.Create(
			ctx,
			persistence.CreateResourceUsageSampleInput{
				InstanceID:     secondInstanceID,
				CollectedAt:    baseTime.Add(2 * time.Minute),
				CPUPercent:     usageIntegrationString("0"),
				MemoryUsedMB:   usageIntegrationInt64(0),
				DiskUsedGB:     usageIntegrationString("0"),
				NetworkRXBytes: usageIntegrationInt64(0),
				NetworkTXBytes: usageIntegrationInt64(0),
				GPUPercent:     usageIntegrationString("0"),
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		sampleIDs = append(sampleIDs, item.ID)

		if item.CPUPercent == nil || *item.CPUPercent != "0.000" ||
			item.MemoryUsedMB == nil || *item.MemoryUsedMB != 0 ||
			item.DiskUsedGB == nil || *item.DiskUsedGB != "0.000" {
			t.Fatalf("zero metrics mismatch: %+v", item)
		}
	})

	t.Run("NumericRounding", func(t *testing.T) {
		item, err := repo.Create(
			ctx,
			persistence.CreateResourceUsageSampleInput{
				InstanceID:  instanceID,
				CollectedAt: baseTime.Add(3 * time.Minute),
				CPUPercent:  usageIntegrationString("12.12345"),
				DiskUsedGB:  usageIntegrationString("1.23456"),
				GPUPercent:  usageIntegrationString("98.76549"),
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		sampleIDs = append(sampleIDs, item.ID)

		if item.CPUPercent == nil || *item.CPUPercent != "12.123" ||
			item.DiskUsedGB == nil || *item.DiskUsedGB != "1.235" ||
			item.GPUPercent == nil || *item.GPUPercent != "98.765" {
			t.Fatalf("numeric rounding mismatch: %+v", item)
		}
	})

	t.Run("NumericMaximum", func(t *testing.T) {
		item, err := repo.Create(
			ctx,
			persistence.CreateResourceUsageSampleInput{
				InstanceID:  secondInstanceID,
				CollectedAt: baseTime.Add(4 * time.Minute),
				CPUPercent:  usageIntegrationString("9999.999"),
				DiskUsedGB:  usageIntegrationString("999999999999999.999"),
				GPUPercent:  usageIntegrationString("9999.999"),
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		sampleIDs = append(sampleIDs, item.ID)

		if item.DiskUsedGB == nil ||
			*item.DiskUsedGB != "999999999999999.999" {
			t.Fatal("numeric maximum not preserved")
		}
	})

	t.Run("BigintLimits", func(t *testing.T) {
		item, err := repo.Create(
			ctx,
			persistence.CreateResourceUsageSampleInput{
				InstanceID:     secondInstanceID,
				CollectedAt:    baseTime.Add(5 * time.Minute),
				MemoryUsedMB:   usageIntegrationInt64(9223372036854775807),
				NetworkRXBytes: usageIntegrationInt64(-9223372036854775808),
				NetworkTXBytes: usageIntegrationInt64(9223372036854775807),
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		sampleIDs = append(sampleIDs, item.ID)

		if item.MemoryUsedMB == nil ||
			*item.MemoryUsedMB != 9223372036854775807 ||
			item.NetworkRXBytes == nil ||
			*item.NetworkRXBytes != -9223372036854775808 {
			t.Fatal("BIGINT values not preserved")
		}
	})

	t.Run("InvalidInstanceForeignKey", func(t *testing.T) {
		_, err := repo.Create(
			ctx,
			persistence.CreateResourceUsageSampleInput{
				InstanceID:  missingID,
				CollectedAt: baseTime,
			},
		)
		if err == nil {
			t.Fatal("expected foreign-key error")
		}
	})

	t.Run("DatabaseRequiredFields", func(t *testing.T) {
		queries := []struct {
			name  string
			query string
			args  []any
		}{
			{
				"missing instance",
				`INSERT INTO resource_usage_samples (collected_at)
				 VALUES ($1)`,
				[]any{baseTime},
			},
			{
				"missing timestamp",
				`INSERT INTO resource_usage_samples (instance_id)
				 VALUES ($1)`,
				[]any{instanceID},
			},
		}

		for _, tc := range queries {
			t.Run(tc.name, func(t *testing.T) {
				_, err := db.Pool().Exec(ctx, tc.query, tc.args...)
				if err == nil {
					t.Fatal("expected NOT NULL error")
				}
			})
		}
	})

	t.Run("DatabaseNumericOverflow", func(t *testing.T) {
		_, err := db.Pool().Exec(
			ctx,
			`INSERT INTO resource_usage_samples
			 (instance_id,collected_at,cpu_percent)
			 VALUES ($1,$2,10000.000)`,
			instanceID,
			baseTime,
		)
		if err == nil {
			t.Fatal("expected NUMERIC(7,3) overflow")
		}
	})

	t.Run("ApplicationValidation", func(t *testing.T) {
		cases := []persistence.CreateResourceUsageSampleInput{
			{InstanceID: "bad", CollectedAt: baseTime},
			{InstanceID: instanceID},
			{
				InstanceID:  instanceID,
				CollectedAt: baseTime,
				CPUPercent:  usageIntegrationString("10000"),
			},
			{
				InstanceID:  instanceID,
				CollectedAt: baseTime,
				Metrics:     json.RawMessage(`{invalid}`),
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
			persistence.ListResourceUsageSamplesOptions{
				InstanceID: &instanceID,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, item := range items {
			if item.InstanceID != instanceID {
				t.Fatal("unexpected instance")
			}
			if item.ID == primaryID {
				found = true
			}
		}
		if !found {
			t.Fatal("primary sample missing")
		}
	})

	t.Run("TimeRangeFiltering", func(t *testing.T) {
		from := baseTime.Add(-time.Second)
		to := baseTime.Add(time.Second)

		items, err := repo.List(
			ctx,
			persistence.ListResourceUsageSamplesOptions{
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

	t.Run("Pagination", func(t *testing.T) {
		first, err := repo.List(
			ctx,
			persistence.ListResourceUsageSamplesOptions{
				InstanceID: &instanceID,
				Limit:      1,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		second, err := repo.List(
			ctx,
			persistence.ListResourceUsageSamplesOptions{
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
		if first[0].CollectedAt.Before(second[0].CollectedAt) {
			t.Fatal("expected descending timestamps")
		}
	})

	t.Run("Update", func(t *testing.T) {
		item, err := repo.Update(
			ctx,
			primaryID,
			persistence.UpdateResourceUsageSampleInput{
				CollectedAt:    baseTime.Add(10 * time.Minute),
				CPUPercent:     usageIntegrationString("50.500"),
				MemoryUsedMB:   usageIntegrationInt64(16384),
				DiskUsedGB:     usageIntegrationString("200.125"),
				NetworkRXBytes: usageIntegrationInt64(500),
				NetworkTXBytes: usageIntegrationInt64(600),
				GPUPercent:     usageIntegrationString("25.250"),
				Metrics:        json.RawMessage(`{"updated":true}`),
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if item.CPUPercent == nil || *item.CPUPercent != "50.500" ||
			item.MemoryUsedMB == nil || *item.MemoryUsedMB != 16384 ||
			item.InstanceID != instanceID {
			t.Fatalf("update mismatch: %+v", item)
		}
	})

	t.Run("ClearNullableMetrics", func(t *testing.T) {
		item, err := repo.Update(
			ctx,
			primaryID,
			persistence.UpdateResourceUsageSampleInput{
				CollectedAt: baseTime,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if item.CPUPercent != nil ||
			item.MemoryUsedMB != nil ||
			item.DiskUsedGB != nil ||
			item.NetworkRXBytes != nil ||
			item.NetworkTXBytes != nil ||
			item.GPUPercent != nil ||
			item.Metrics != nil {
			t.Fatalf("metrics not cleared: %+v", item)
		}
	})

	t.Run("GetMissing", func(t *testing.T) {
		_, err := repo.GetByID(ctx, missingID)
		if !errors.Is(err, persistence.ErrResourceUsageSampleNotFound) {
			t.Fatalf("expected not found: %v", err)
		}
	})

	t.Run("UpdateMissing", func(t *testing.T) {
		_, err := repo.Update(
			ctx,
			missingID,
			persistence.UpdateResourceUsageSampleInput{
				CollectedAt: baseTime,
			},
		)
		if !errors.Is(err, persistence.ErrResourceUsageSampleNotFound) {
			t.Fatalf("expected not found: %v", err)
		}
	})

	t.Run("Delete", func(t *testing.T) {
		item := createSample(
			secondInstanceID,
			baseTime.Add(20*time.Minute),
		)
		if err := repo.Delete(ctx, item.ID); err != nil {
			t.Fatal(err)
		}
		_, err := repo.GetByID(ctx, item.ID)
		if !errors.Is(err, persistence.ErrResourceUsageSampleNotFound) {
			t.Fatalf("expected deleted sample to be absent: %v", err)
		}
	})

	t.Run("DeleteMissing", func(t *testing.T) {
		err := repo.Delete(ctx, missingID)
		if !errors.Is(err, persistence.ErrResourceUsageSampleNotFound) {
			t.Fatalf("expected not found: %v", err)
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

	t.Run("InvalidListOptions", func(t *testing.T) {
		badID := "invalid"
		after := baseTime.Add(time.Hour)
		before := baseTime

		cases := []persistence.ListResourceUsageSamplesOptions{
			{InstanceID: &badID},
			{Limit: -1},
			{Limit: 1001},
			{Offset: -1},
			{From: usageIntegrationTime(time.Time{})},
			{To: usageIntegrationTime(time.Time{})},
			{From: &after, To: &before},
		}

		for i, opts := range cases {
			if _, err := repo.List(ctx, opts); err == nil {
				t.Fatalf("case %d: expected validation error", i)
			}
		}
	})
}
