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

func auditIntegrationString(v string) *string {
	return &v
}

func auditIntegrationTime(v time.Time) *time.Time {
	return &v
}

func TestInfrastructureAuditLogCRUD(t *testing.T) {
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
		t.Fatalf("connect database: %v", err)
	}
	t.Cleanup(db.Close)

	ctx, cancel := context.WithTimeout(
		context.Background(), 120*time.Second,
	)
	defer cancel()

	repo, err := persistence.NewInfrastructureAuditLogRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	// A unique actor identifies records belonging to this test.
	actor := fmt.Sprintf("audit-integration-%d", time.Now().UnixNano())
	base := time.Now().UTC().Truncate(time.Microsecond)
	missingID := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"

	var createdIDs []string

	t.Cleanup(func() {
		cleanupCtx, done := context.WithTimeout(
			context.Background(), 30*time.Second,
		)
		defer done()

		for _, id := range createdIDs {
			_, err := db.Pool().Exec(
				cleanupCtx,
				`DELETE FROM infrastructure_audit_log WHERE id=$1`,
				id,
			)
			if err != nil {
				t.Errorf("cleanup audit record %s: %v", id, err)
			}
		}
	})

	create := func(
		action string,
		occurredAt time.Time,
	) *persistence.InfrastructureAuditLog {
		t.Helper()

		item, err := repo.Create(
			ctx,
			persistence.CreateInfrastructureAuditLogInput{
				ActorReference: &actor,
				Action:         action,
				OccurredAt:     occurredAt,
			},
		)
		if err != nil {
			t.Fatalf("create fixture: %v", err)
		}
		createdIDs = append(createdIDs, item.ID)
		return item
	}

	var primaryID string

	t.Run("CreateAllFields", func(t *testing.T) {
		item, err := repo.Create(
			ctx,
			persistence.CreateInfrastructureAuditLogInput{
				ActorReference:    &actor,
				Action:            "instance.provisioned",
				ResourceType:      auditIntegrationString("instance"),
				ResourceReference: auditIntegrationString("droplet-001"),
				Outcome:           auditIntegrationString("success"),
				Details: json.RawMessage(
					`{"provider":"digitalocean","attempt":1}`,
				),
				OccurredAt: base,
			},
		)
		if err != nil {
			t.Fatal(err)
		}

		primaryID = item.ID
		createdIDs = append(createdIDs, item.ID)

		if item.ID == "" || item.Action != "instance.provisioned" {
			t.Fatalf("unexpected record: %+v", item)
		}
		if item.ActorReference == nil || *item.ActorReference != actor {
			t.Fatal("actor mismatch")
		}
		if item.ResourceType == nil || *item.ResourceType != "instance" {
			t.Fatal("resource type mismatch")
		}
		if item.ResourceReference == nil ||
			*item.ResourceReference != "droplet-001" {
			t.Fatal("resource reference mismatch")
		}
		if item.Outcome == nil || *item.Outcome != "success" {
			t.Fatal("outcome mismatch")
		}
		if !item.OccurredAt.Equal(base) {
			t.Fatal("occurred_at mismatch")
		}

		var details map[string]any
		if err := json.Unmarshal(item.Details, &details); err != nil {
			t.Fatal(err)
		}
		if details["provider"] != "digitalocean" {
			t.Fatal("JSONB mismatch")
		}
	})

	t.Run("GetByID", func(t *testing.T) {
		item, err := repo.GetByID(ctx, primaryID)
		if err != nil {
			t.Fatal(err)
		}
		if item.ID != primaryID {
			t.Fatal("wrong record returned")
		}
	})

	t.Run("NullableFields", func(t *testing.T) {
		item := create("minimal", base.Add(-time.Minute))

		if item.ProjectID != nil ||
			item.OperationID != nil ||
			item.InstanceID != nil ||
			item.ResourceType != nil ||
			item.ResourceReference != nil ||
			item.Outcome != nil ||
			item.Details != nil {
			t.Fatalf("expected NULL optional fields: %+v", item)
		}
	})

	t.Run("MaximumLengths", func(t *testing.T) {
		item, err := repo.Create(
			ctx,
			persistence.CreateInfrastructureAuditLogInput{
				ActorReference: auditIntegrationString(
					strings.Repeat("a", 255),
				),
				Action: strings.Repeat("b", 100),
				ResourceType: auditIntegrationString(
					strings.Repeat("c", 100),
				),
				ResourceReference: auditIntegrationString(
					strings.Repeat("d", 255),
				),
				Outcome: auditIntegrationString(
					strings.Repeat("e", 50),
				),
				OccurredAt: base,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		createdIDs = append(createdIDs, item.ID)

		if len(item.Action) != 100 {
			t.Fatal("maximum action length not preserved")
		}
	})

	t.Run("UnicodeLengths", func(t *testing.T) {
		item, err := repo.Create(
			ctx,
			persistence.CreateInfrastructureAuditLogInput{
				ActorReference: &actor,
				Action:         strings.Repeat("é", 100),
				OccurredAt:     base,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		createdIDs = append(createdIDs, item.ID)
	})

	t.Run("JSONBArray", func(t *testing.T) {
		item, err := repo.Create(
			ctx,
			persistence.CreateInfrastructureAuditLogInput{
				ActorReference: &actor,
				Action:         "json.array",
				Details:        json.RawMessage(`[1,2,3]`),
				OccurredAt:     base,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		createdIDs = append(createdIDs, item.ID)

		var values []int
		if err := json.Unmarshal(item.Details, &values); err != nil {
			t.Fatal(err)
		}
		if len(values) != 3 {
			t.Fatal("JSONB array mismatch")
		}
	})

	t.Run("ForeignKeyProject", func(t *testing.T) {
		_, err := repo.Create(
			ctx,
			persistence.CreateInfrastructureAuditLogInput{
				ProjectID:  &missingID,
				Action:     "invalid.project",
				OccurredAt: base,
			},
		)
		if err == nil {
			t.Fatal("expected project foreign-key error")
		}
	})

	t.Run("ForeignKeyOperation", func(t *testing.T) {
		_, err := repo.Create(
			ctx,
			persistence.CreateInfrastructureAuditLogInput{
				OperationID: &missingID,
				Action:      "invalid.operation",
				OccurredAt:  base,
			},
		)
		if err == nil {
			t.Fatal("expected operation foreign-key error")
		}
	})

	t.Run("ForeignKeyInstance", func(t *testing.T) {
		_, err := repo.Create(
			ctx,
			persistence.CreateInfrastructureAuditLogInput{
				InstanceID: &missingID,
				Action:     "invalid.instance",
				OccurredAt: base,
			},
		)
		if err == nil {
			t.Fatal("expected instance foreign-key error")
		}
	})

	t.Run("DatabaseActionNotNull", func(t *testing.T) {
		_, err := db.Pool().Exec(
			ctx,
			`INSERT INTO infrastructure_audit_log (occurred_at)
			 VALUES ($1)`,
			base,
		)
		if err == nil {
			t.Fatal("expected action NOT NULL violation")
		}
	})

	t.Run("DatabaseOccurredAtNotNull", func(t *testing.T) {
		_, err := db.Pool().Exec(
			ctx,
			`INSERT INTO infrastructure_audit_log (action)
			 VALUES ('test')`,
		)
		if err == nil {
			t.Fatal("expected occurred_at NOT NULL violation")
		}
	})

	t.Run("DatabaseLengthConstraint", func(t *testing.T) {
		_, err := db.Pool().Exec(
			ctx,
			`INSERT INTO infrastructure_audit_log
			 (action, occurred_at) VALUES ($1,$2)`,
			strings.Repeat("x", 101),
			base,
		)
		if err == nil {
			t.Fatal("expected VARCHAR(100) violation")
		}
	})

	t.Run("ApplicationValidation", func(t *testing.T) {
		cases := []persistence.CreateInfrastructureAuditLogInput{
			{Action: "missing.time"},
			{Action: strings.Repeat("x", 101), OccurredAt: base},
			{Action: "bad.uuid", ProjectID: auditIntegrationString("invalid"), OccurredAt: base},
			{Action: "bad.json", Details: json.RawMessage(`{broken}`), OccurredAt: base},
		}

		for i, input := range cases {
			if _, err := repo.Create(ctx, input); err == nil {
				t.Fatalf("case %d: expected validation error", i)
			}
		}
	})

	t.Run("ListByActor", func(t *testing.T) {
		items, err := repo.List(
			ctx,
			persistence.ListInfrastructureAuditLogsOptions{
				ActorReference: &actor,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if len(items) == 0 {
			t.Fatal("expected audit records")
		}
		for _, item := range items {
			if item.ActorReference == nil || *item.ActorReference != actor {
				t.Fatal("unexpected actor in results")
			}
		}
	})

	t.Run("ListByAction", func(t *testing.T) {
		action := "instance.provisioned"
		items, err := repo.List(
			ctx,
			persistence.ListInfrastructureAuditLogsOptions{
				ActorReference: &actor,
				Action:         &action,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if len(items) != 1 || items[0].ID != primaryID {
			t.Fatalf("unexpected action results: %+v", items)
		}
	})

	t.Run("ListByResource", func(t *testing.T) {
		resourceType := "instance"
		resourceRef := "droplet-001"
		items, err := repo.List(
			ctx,
			persistence.ListInfrastructureAuditLogsOptions{
				ActorReference:    &actor,
				ResourceType:      &resourceType,
				ResourceReference: &resourceRef,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if len(items) != 1 || items[0].ID != primaryID {
			t.Fatalf("unexpected resource results: %+v", items)
		}
	})

	t.Run("ListByOutcome", func(t *testing.T) {
		outcome := "success"
		items, err := repo.List(
			ctx,
			persistence.ListInfrastructureAuditLogsOptions{
				ActorReference: &actor,
				Outcome:        &outcome,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if len(items) != 1 || items[0].ID != primaryID {
			t.Fatalf("unexpected outcome results: %+v", items)
		}
	})

	t.Run("TimeRangeInclusive", func(t *testing.T) {
		action := "instance.provisioned"
		items, err := repo.List(
			ctx,
			persistence.ListInfrastructureAuditLogsOptions{
				ActorReference: &actor,
				Action:         &action,
				From:           auditIntegrationTime(base),
				To:             auditIntegrationTime(base),
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
		paginationActor := actor + "-page"

		for i := 0; i < 3; i++ {
			item, err := repo.Create(
				ctx,
				persistence.CreateInfrastructureAuditLogInput{
					ActorReference: &paginationActor,
					Action:         fmt.Sprintf("page.%d", i),
					OccurredAt:     base.Add(time.Duration(i) * time.Minute),
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			createdIDs = append(createdIDs, item.ID)
		}

		first, err := repo.List(
			ctx,
			persistence.ListInfrastructureAuditLogsOptions{
				ActorReference: &paginationActor,
				Limit:          1,
			},
		)
		if err != nil {
			t.Fatal(err)
		}

		second, err := repo.List(
			ctx,
			persistence.ListInfrastructureAuditLogsOptions{
				ActorReference: &paginationActor,
				Limit:          1,
				Offset:         1,
			},
		)
		if err != nil {
			t.Fatal(err)
		}

		if len(first) != 1 || len(second) != 1 {
			t.Fatal("pagination returned wrong number of records")
		}
		if first[0].Action != "page.2" || second[0].Action != "page.1" {
			t.Fatal("descending timestamp ordering failed")
		}
	})

	t.Run("GetMissing", func(t *testing.T) {
		_, err := repo.GetByID(ctx, missingID)
		if !errors.Is(
			err,
			persistence.ErrInfrastructureAuditLogNotFound,
		) {
			t.Fatalf("expected not-found error: %v", err)
		}
	})

	t.Run("InvalidUUID", func(t *testing.T) {
		if _, err := repo.GetByID(ctx, "invalid"); err == nil {
			t.Fatal("expected UUID validation error")
		}
	})

	t.Run("InvalidListOptions", func(t *testing.T) {
		badID := "invalid"
		later := base.Add(time.Hour)

		cases := []persistence.ListInfrastructureAuditLogsOptions{
			{ProjectID: &badID},
			{OperationID: &badID},
			{InstanceID: &badID},
			{Limit: -1},
			{Limit: 1001},
			{Offset: -1},
			{From: &later, To: &base},
		}

		for i, opts := range cases {
			if _, err := repo.List(ctx, opts); err == nil {
				t.Fatalf("case %d: expected validation error", i)
			}
		}
	})
}
