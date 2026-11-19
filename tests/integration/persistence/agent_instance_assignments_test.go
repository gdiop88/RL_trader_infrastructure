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

func assignmentIntegrationString(s string) *string {
	return &s
}

func TestAgentInstanceAssignmentCRUD(t *testing.T) {
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

	repo, err := persistence.NewAgentInstanceAssignmentRepository(db)
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())

	// Create dedicated foreign-key fixtures.
	// Cleanup runs in reverse dependency order.
	var agentID, instanceID string
	var assignmentIDs []string

	t.Cleanup(func() {
		cleanupCtx, done := context.WithTimeout(
			context.Background(), 30*time.Second,
		)
		defer done()

		for _, id := range assignmentIDs {
			if _, err := db.Pool().Exec(
				cleanupCtx,
				`DELETE FROM agent_instance_assignments WHERE id=$1`,
				id,
			); err != nil {
				t.Errorf("cleanup assignment: %v", err)
			}
		}

		if agentID != "" {
			if _, err := db.Pool().Exec(
				cleanupCtx,
				`DELETE FROM agent_registry WHERE id=$1`,
				agentID,
			); err != nil {
				t.Errorf("cleanup agent: %v", err)
			}
		}

		if instanceID != "" {
			if _, err := db.Pool().Exec(
				cleanupCtx,
				`DELETE FROM infrastructure_instances WHERE id=$1`,
				instanceID,
			); err != nil {
				t.Errorf("cleanup instance: %v", err)
			}
		}
	})

	err = db.Pool().QueryRow(
		ctx,
		`INSERT INTO agent_registry (agent_reference)
		 VALUES ($1) RETURNING id`,
		"assignment-agent-"+suffix,
	).Scan(&agentID)
	if err != nil {
		t.Fatalf("create agent fixture: %v", err)
	}

	err = db.Pool().QueryRow(
		ctx,
		`INSERT INTO infrastructure_instances (provider, name)
		 VALUES ($1, $2) RETURNING id`,
		"integration", "assignment-instance-"+suffix,
	).Scan(&instanceID)
	if err != nil {
		t.Fatalf("create instance fixture: %v", err)
	}

	assignedAt := time.Now().UTC().Truncate(time.Microsecond)

	first, err := repo.Create(
		ctx,
		persistence.CreateAgentInstanceAssignmentInput{
			AgentID:        agentID,
			InstanceID:     instanceID,
			AssignmentRole: assignmentIntegrationString("primary"),
			Status:         "assigned",
			AssignedAt:     &assignedAt,
			Metadata:       json.RawMessage(`{"slot":1}`),
		},
	)
	if err != nil {
		t.Fatalf("create assignment: %v", err)
	}
	assignmentIDs = append(assignmentIDs, first.ID)

	t.Run("Create", func(t *testing.T) {
		if first.ID == "" ||
			first.AgentID != agentID ||
			first.InstanceID != instanceID ||
			first.Status != "assigned" {
			t.Fatal("incorrect created assignment")
		}
		if first.AssignedAt == nil ||
			!first.AssignedAt.Equal(assignedAt) {
			t.Fatal("assigned_at mismatch")
		}
	})

	t.Run("GetByID", func(t *testing.T) {
		got, err := repo.GetByID(ctx, first.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got.ID != first.ID {
			t.Fatal("incorrect assignment")
		}
	})

	t.Run("MetadataPersistence", func(t *testing.T) {
		got, err := repo.GetByID(ctx, first.ID)
		if err != nil {
			t.Fatal(err)
		}

		var metadata map[string]int
		if err := json.Unmarshal(got.Metadata, &metadata); err != nil {
			t.Fatal(err)
		}
		if metadata["slot"] != 1 {
			t.Fatal("metadata mismatch")
		}
	})

	t.Run("InvalidForeignKeys", func(t *testing.T) {
		missing := "33333333-3333-4333-8333-333333333333"

		_, err := repo.Create(
			ctx,
			persistence.CreateAgentInstanceAssignmentInput{
				AgentID:    missing,
				InstanceID: instanceID,
				Status:     "assigned",
			},
		)
		if err == nil {
			t.Fatal("expected agent foreign-key error")
		}

		_, err = repo.Create(
			ctx,
			persistence.CreateAgentInstanceAssignmentInput{
				AgentID:    agentID,
				InstanceID: missing,
				Status:     "assigned",
			},
		)
		if err == nil {
			t.Fatal("expected instance foreign-key error")
		}
	})

	// The schema permits multiple assignments for the same pair.
	second, err := repo.Create(
		ctx,
		persistence.CreateAgentInstanceAssignmentInput{
			AgentID:    agentID,
			InstanceID: instanceID,
			Status:     "pending",
		},
	)
	if err != nil {
		t.Fatalf("create second assignment: %v", err)
	}
	assignmentIDs = append(assignmentIDs, second.ID)

	t.Run("DuplicatePairAllowed", func(t *testing.T) {
		if first.ID == second.ID {
			t.Fatal("expected distinct assignment IDs")
		}
	})

	t.Run("NullableFields", func(t *testing.T) {
		got, err := repo.GetByID(ctx, second.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.AssignmentRole != nil ||
			got.AssignedAt != nil ||
			got.ReleasedAt != nil ||
			len(got.Metadata) != 0 {
			t.Fatal("expected nullable fields to be NULL")
		}
	})

	t.Run("ListByAgent", func(t *testing.T) {
		rows, err := repo.List(
			ctx,
			persistence.ListAgentInstanceAssignmentsOptions{
				AgentID: agentID,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 2 {
			t.Fatalf("expected 2 assignments, got %d", len(rows))
		}
	})

	t.Run("ListByInstanceAndStatus", func(t *testing.T) {
		rows, err := repo.List(
			ctx,
			persistence.ListAgentInstanceAssignmentsOptions{
				InstanceID: instanceID,
				Status:     "pending",
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 || rows[0].ID != second.ID {
			t.Fatal("incorrect filtered results")
		}
	})

	t.Run("Pagination", func(t *testing.T) {
		rows, err := repo.List(
			ctx,
			persistence.ListAgentInstanceAssignmentsOptions{
				AgentID: agentID,
				Limit:   1,
				Offset:  1,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 {
			t.Fatalf("expected one row, got %d", len(rows))
		}
	})

	t.Run("Update", func(t *testing.T) {
		updated, err := repo.Update(
			ctx,
			first.ID,
			persistence.UpdateAgentInstanceAssignmentInput{
				AssignmentRole: assignmentIntegrationString("secondary"),
				Status:         "running",
				AssignedAt:     &assignedAt,
				Metadata:       json.RawMessage(`["cpu","gpu"]`),
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if updated.Status != "running" ||
			updated.AssignmentRole == nil ||
			*updated.AssignmentRole != "secondary" {
			t.Fatal("update mismatch")
		}
		if updated.AgentID != agentID ||
			updated.InstanceID != instanceID {
			t.Fatal("foreign keys unexpectedly changed")
		}

		var values []string
		if err := json.Unmarshal(updated.Metadata, &values); err != nil {
			t.Fatal(err)
		}
		if len(values) != 2 {
			t.Fatal("JSONB array mismatch")
		}
	})

	t.Run("UpdateStatus", func(t *testing.T) {
		got, err := repo.UpdateStatus(ctx, second.ID, "active")
		if err != nil {
			t.Fatal(err)
		}
		if got.Status != "active" {
			t.Fatal("status not updated")
		}
	})

	t.Run("Release", func(t *testing.T) {
		releasedAt := assignedAt.Add(time.Hour)

		got, err := repo.Release(ctx, first.ID, releasedAt)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status != "released" ||
			got.ReleasedAt == nil ||
			!got.ReleasedAt.Equal(releasedAt) {
			t.Fatal("release mismatch")
		}
	})

	t.Run("ClearNullableFields", func(t *testing.T) {
		got, err := repo.Update(
			ctx,
			second.ID,
			persistence.UpdateAgentInstanceAssignmentInput{
				Status: "active",
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if got.AssignmentRole != nil ||
			got.AssignedAt != nil ||
			got.ReleasedAt != nil ||
			len(got.Metadata) != 0 {
			t.Fatal("nullable fields not cleared")
		}
	})

	t.Run("JSONBScalar", func(t *testing.T) {
		got, err := repo.Update(
			ctx,
			second.ID,
			persistence.UpdateAgentInstanceAssignmentInput{
				Status:   "active",
				Metadata: json.RawMessage(`42`),
			},
		)
		if err != nil {
			t.Fatal(err)
		}

		var value int
		if err := json.Unmarshal(got.Metadata, &value); err != nil {
			t.Fatal(err)
		}
		if value != 42 {
			t.Fatal("JSONB scalar mismatch")
		}
	})

	missingID := "44444444-4444-4444-8444-444444444444"

	t.Run("GetMissing", func(t *testing.T) {
		_, err := repo.GetByID(ctx, missingID)
		if !errors.Is(err, persistence.ErrAgentInstanceAssignmentNotFound) {
			t.Fatalf("expected not found, got %v", err)
		}
	})

	t.Run("UpdateMissing", func(t *testing.T) {
		_, err := repo.Update(
			ctx, missingID,
			persistence.UpdateAgentInstanceAssignmentInput{
				Status: "active",
			},
		)
		if !errors.Is(err, persistence.ErrAgentInstanceAssignmentNotFound) {
			t.Fatalf("expected not found, got %v", err)
		}
	})

	t.Run("UpdateStatusMissing", func(t *testing.T) {
		_, err := repo.UpdateStatus(ctx, missingID, "active")
		if !errors.Is(err, persistence.ErrAgentInstanceAssignmentNotFound) {
			t.Fatalf("expected not found, got %v", err)
		}
	})

	t.Run("ReleaseMissing", func(t *testing.T) {
		_, err := repo.Release(ctx, missingID, time.Now())
		if !errors.Is(err, persistence.ErrAgentInstanceAssignmentNotFound) {
			t.Fatalf("expected not found, got %v", err)
		}
	})

	t.Run("Delete", func(t *testing.T) {
		if err := repo.Delete(ctx, second.ID); err != nil {
			t.Fatal(err)
		}
		_, err := repo.GetByID(ctx, second.ID)
		if !errors.Is(err, persistence.ErrAgentInstanceAssignmentNotFound) {
			t.Fatalf("expected deleted row missing, got %v", err)
		}
	})

	t.Run("DeleteMissing", func(t *testing.T) {
		err := repo.Delete(ctx, missingID)
		if !errors.Is(err, persistence.ErrAgentInstanceAssignmentNotFound) {
			t.Fatalf("expected not found, got %v", err)
		}
	})
}
