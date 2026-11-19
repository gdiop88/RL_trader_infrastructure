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

func agentRegistryTestString(value string) *string {
	return &value
}

func TestAgentRegistryCRUD(t *testing.T) {
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
		t.Fatalf("connect to PostgreSQL: %v", err)
	}
	t.Cleanup(db.Close)

	ctx, cancel := context.WithTimeout(
		context.Background(), 120*time.Second,
	)
	defer cancel()

	repo, err := persistence.NewAgentRegistryRepository(db)
	if err != nil {
		t.Fatalf("create agent registry repository: %v", err)
	}

	var agentIDs []string

	t.Cleanup(func() {
		cleanupCtx, done := context.WithTimeout(
			context.Background(), 30*time.Second,
		)
		defer done()

		for _, id := range agentIDs {
			_, err := db.Pool().Exec(
				cleanupCtx,
				`DELETE FROM agent_registry WHERE id = $1`,
				id,
			)
			if err != nil {
				t.Errorf("cleanup agent %s: %v", id, err)
			}
		}
	})

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	prefix := "integration-agent-" + suffix

	registeredAt := time.Now().UTC().Truncate(time.Microsecond)

	first, err := repo.Create(
		ctx,
		persistence.CreateAgentRegistryInput{
			AgentReference: prefix + "-001",
			AgentType:      agentRegistryTestString("trading"),
			OwnerReference: agentRegistryTestString("integration-owner"),
			Status:         agentRegistryTestString("registered"),
			Capabilities: json.RawMessage(
				`{"architectures":["dqn","ppo"],"gpu":false}`,
			),
			RegisteredAt: &registeredAt,
		},
	)
	if err != nil {
		t.Fatalf("create first agent: %v", err)
	}
	agentIDs = append(agentIDs, first.ID)

	t.Run("Create", func(t *testing.T) {
		if first.ID == "" {
			t.Fatal("expected generated ID")
		}
		if first.AgentReference != prefix+"-001" {
			t.Fatal("agent reference mismatch")
		}
		if first.AgentType == nil || *first.AgentType != "trading" {
			t.Fatal("agent type mismatch")
		}
		if first.RegisteredAt == nil ||
			!first.RegisteredAt.Equal(registeredAt) {
			t.Fatal("registered_at mismatch")
		}
		if first.LastSeenAt != nil {
			t.Fatal("expected NULL last_seen_at")
		}
	})

	t.Run("GetByID", func(t *testing.T) {
		found, err := repo.GetByID(ctx, first.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if found.ID != first.ID {
			t.Fatal("incorrect agent returned")
		}
	})

	t.Run("GetByReference", func(t *testing.T) {
		found, err := repo.GetByReference(
			ctx, first.AgentReference,
		)
		if err != nil {
			t.Fatalf("GetByReference: %v", err)
		}
		if found.ID != first.ID {
			t.Fatal("incorrect agent returned")
		}
	})

	t.Run("JSONBPersistence", func(t *testing.T) {
		found, err := repo.GetByID(ctx, first.ID)
		if err != nil {
			t.Fatalf("reload agent: %v", err)
		}

		var capabilities map[string]any
		if err := json.Unmarshal(
			found.Capabilities, &capabilities,
		); err != nil {
			t.Fatalf("decode capabilities: %v", err)
		}

		if capabilities["gpu"] != false {
			t.Fatal("GPU capability mismatch")
		}

		architectures, ok := capabilities["architectures"].([]any)
		if !ok || len(architectures) != 2 {
			t.Fatal("architecture capabilities mismatch")
		}
	})

	t.Run("DuplicateReference", func(t *testing.T) {
		_, err := repo.Create(
			ctx,
			persistence.CreateAgentRegistryInput{
				AgentReference: first.AgentReference,
			},
		)
		if !errors.Is(
			err, persistence.ErrAgentRegistryConflict,
		) {
			t.Fatalf("expected unique conflict, got %v", err)
		}
	})

	second, err := repo.Create(
		ctx,
		persistence.CreateAgentRegistryInput{
			AgentReference: prefix + "-002",
			AgentType:      agentRegistryTestString("trading"),
			OwnerReference: agentRegistryTestString("integration-owner"),
			Status:         agentRegistryTestString("registered"),
		},
	)
	if err != nil {
		t.Fatalf("create second agent: %v", err)
	}
	agentIDs = append(agentIDs, second.ID)

	t.Run("NullableFields", func(t *testing.T) {
		if second.Capabilities != nil &&
			len(second.Capabilities) != 0 {
			t.Fatal("expected NULL capabilities")
		}
		if second.RegisteredAt != nil {
			t.Fatal("expected NULL registered_at")
		}
		if second.LastSeenAt != nil {
			t.Fatal("expected NULL last_seen_at")
		}
	})

	t.Run("Update", func(t *testing.T) {
		lastSeen := registeredAt.Add(time.Minute)

		updated, err := repo.Update(
			ctx,
			first.ID,
			persistence.UpdateAgentRegistryInput{
				AgentType:      agentRegistryTestString("prediction"),
				OwnerReference: agentRegistryTestString("integration-owner"),
				Status:         agentRegistryTestString("running"),
				Capabilities: json.RawMessage(
					`{"models":["transformer"],"gpu":true}`,
				),
				RegisteredAt: &registeredAt,
				LastSeenAt:   &lastSeen,
			},
		)
		if err != nil {
			t.Fatalf("Update: %v", err)
		}

		if updated.AgentReference != first.AgentReference {
			t.Fatal("immutable reference changed")
		}
		if updated.AgentType == nil ||
			*updated.AgentType != "prediction" {
			t.Fatal("agent type not updated")
		}
		if updated.Status == nil || *updated.Status != "running" {
			t.Fatal("status not updated")
		}

		found, err := repo.GetByID(ctx, first.ID)
		if err != nil {
			t.Fatalf("reload updated agent: %v", err)
		}
		if found.LastSeenAt == nil ||
			!found.LastSeenAt.Equal(lastSeen) {
			t.Fatal("last_seen_at not persisted")
		}
	})

	t.Run("UpdateStatus", func(t *testing.T) {
		updated, err := repo.UpdateStatus(
			ctx, second.ID, agentRegistryTestString("active"),
		)
		if err != nil {
			t.Fatalf("UpdateStatus: %v", err)
		}
		if updated.Status == nil || *updated.Status != "active" {
			t.Fatal("status not updated")
		}

		found, err := repo.GetByID(ctx, second.ID)
		if err != nil {
			t.Fatalf("reload status: %v", err)
		}
		if found.Status == nil || *found.Status != "active" {
			t.Fatal("status not persisted")
		}
	})

	t.Run("SetLastSeen", func(t *testing.T) {
		heartbeat := time.Now().UTC().Truncate(time.Microsecond)

		updated, err := repo.SetLastSeen(
			ctx, second.ID, &heartbeat,
		)
		if err != nil {
			t.Fatalf("SetLastSeen: %v", err)
		}
		if updated.LastSeenAt == nil ||
			!updated.LastSeenAt.Equal(heartbeat) {
			t.Fatal("heartbeat mismatch")
		}
	})

	t.Run("ListByAgentType", func(t *testing.T) {
		agents, err := repo.List(
			ctx,
			persistence.ListAgentRegistryOptions{
				AgentType:      "prediction",
				OwnerReference: "integration-owner",
			},
		)
		if err != nil {
			t.Fatalf("List: %v", err)
		}

		found := false
		for _, agent := range agents {
			if agent.ID == first.ID {
				found = true
			}
		}
		if !found {
			t.Fatal("updated agent missing from filtered list")
		}
	})

	t.Run("ListByStatus", func(t *testing.T) {
		agents, err := repo.List(
			ctx,
			persistence.ListAgentRegistryOptions{
				AgentType:      "trading",
				OwnerReference: "integration-owner",
				Status:         "active",
			},
		)
		if err != nil {
			t.Fatalf("List by status: %v", err)
		}

		found := false
		for _, agent := range agents {
			if agent.ID == second.ID {
				found = true
			}
		}
		if !found {
			t.Fatal("agent missing from status filter")
		}
	})

	t.Run("Pagination", func(t *testing.T) {
		// Create a dedicated group to ensure stable pagination
		// independently of other rows in the database.
		paginationOwner := "pagination-" + suffix

		for i := 0; i < 3; i++ {
			agent, err := repo.Create(
				ctx,
				persistence.CreateAgentRegistryInput{
					AgentReference: fmt.Sprintf(
						"%s-page-%03d", prefix, i,
					),
					OwnerReference: &paginationOwner,
				},
			)
			if err != nil {
				t.Fatalf("create pagination agent: %v", err)
			}
			agentIDs = append(agentIDs, agent.ID)
		}

		agents, err := repo.List(
			ctx,
			persistence.ListAgentRegistryOptions{
				OwnerReference: paginationOwner,
				Limit:          1,
				Offset:         1,
			},
		)
		if err != nil {
			t.Fatalf("paginated List: %v", err)
		}
		if len(agents) != 1 ||
			agents[0].AgentReference != prefix+"-page-001" {
			t.Fatal("pagination returned incorrect agent")
		}
	})

	t.Run("CustomStatus", func(t *testing.T) {
		updated, err := repo.UpdateStatus(
			ctx,
			second.ID,
			agentRegistryTestString("awaiting_provisioning"),
		)
		if err != nil {
			t.Fatalf("custom status: %v", err)
		}
		if updated.Status == nil ||
			*updated.Status != "awaiting_provisioning" {
			t.Fatal("custom status not persisted")
		}
	})

	t.Run("JSONBArray", func(t *testing.T) {
		updated, err := repo.Update(
			ctx,
			second.ID,
			persistence.UpdateAgentRegistryInput{
				Status:       agentRegistryTestString("active"),
				Capabilities: json.RawMessage(`["cpu","memory"]`),
			},
		)
		if err != nil {
			t.Fatalf("JSONB array update: %v", err)
		}

		var capabilities []string
		if err := json.Unmarshal(
			updated.Capabilities, &capabilities,
		); err != nil {
			t.Fatalf("decode JSONB array: %v", err)
		}
		if len(capabilities) != 2 {
			t.Fatal("JSONB array not persisted")
		}
	})

	t.Run("ClearNullableFields", func(t *testing.T) {
		updated, err := repo.Update(
			ctx,
			second.ID,
			persistence.UpdateAgentRegistryInput{},
		)
		if err != nil {
			t.Fatalf("clear nullable fields: %v", err)
		}

		if updated.AgentType != nil ||
			updated.OwnerReference != nil ||
			updated.Status != nil ||
			len(updated.Capabilities) != 0 ||
			updated.RegisteredAt != nil ||
			updated.LastSeenAt != nil {
			t.Fatal("nullable fields were not cleared")
		}
	})

	t.Run("SetNullStatus", func(t *testing.T) {
		updated, err := repo.UpdateStatus(
			ctx, first.ID, nil,
		)
		if err != nil {
			t.Fatalf("clear status: %v", err)
		}
		if updated.Status != nil {
			t.Fatal("expected NULL status")
		}
	})

	t.Run("SetNullLastSeen", func(t *testing.T) {
		updated, err := repo.SetLastSeen(
			ctx, first.ID, nil,
		)
		if err != nil {
			t.Fatalf("clear last seen: %v", err)
		}
		if updated.LastSeenAt != nil {
			t.Fatal("expected NULL last_seen_at")
		}
	})

	t.Run("GetMissing", func(t *testing.T) {
		_, err := repo.GetByID(
			ctx, "22222222-2222-4222-8222-222222222222",
		)
		if !errors.Is(
			err, persistence.ErrAgentRegistryNotFound,
		) {
			t.Fatalf("expected not found, got %v", err)
		}
	})

	t.Run("GetMissingReference", func(t *testing.T) {
		_, err := repo.GetByReference(
			ctx, "missing-agent-"+suffix,
		)
		if !errors.Is(
			err, persistence.ErrAgentRegistryNotFound,
		) {
			t.Fatalf("expected not found, got %v", err)
		}
	})

	t.Run("UpdateMissing", func(t *testing.T) {
		_, err := repo.Update(
			ctx,
			"22222222-2222-4222-8222-222222222222",
			persistence.UpdateAgentRegistryInput{
				Status: agentRegistryTestString("running"),
			},
		)
		if !errors.Is(
			err, persistence.ErrAgentRegistryNotFound,
		) {
			t.Fatalf("expected not found, got %v", err)
		}
	})

	t.Run("UpdateStatusMissing", func(t *testing.T) {
		_, err := repo.UpdateStatus(
			ctx,
			"22222222-2222-4222-8222-222222222222",
			agentRegistryTestString("active"),
		)
		if !errors.Is(
			err, persistence.ErrAgentRegistryNotFound,
		) {
			t.Fatalf("expected not found, got %v", err)
		}
	})

	t.Run("SetLastSeenMissing", func(t *testing.T) {
		_, err := repo.SetLastSeen(
			ctx,
			"22222222-2222-4222-8222-222222222222",
			&registeredAt,
		)
		if !errors.Is(
			err, persistence.ErrAgentRegistryNotFound,
		) {
			t.Fatalf("expected not found, got %v", err)
		}
	})

	t.Run("Delete", func(t *testing.T) {
		if err := repo.Delete(ctx, second.ID); err != nil {
			t.Fatalf("Delete: %v", err)
		}

		_, err := repo.GetByID(ctx, second.ID)
		if !errors.Is(
			err, persistence.ErrAgentRegistryNotFound,
		) {
			t.Fatalf("expected deleted agent not found, got %v", err)
		}
	})

	t.Run("DeleteMissing", func(t *testing.T) {
		err := repo.Delete(
			ctx, "22222222-2222-4222-8222-222222222222",
		)
		if !errors.Is(
			err, persistence.ErrAgentRegistryNotFound,
		) {
			t.Fatalf("expected not found, got %v", err)
		}
	})

	t.Run("MaximumReferenceLength", func(t *testing.T) {
		reference := "agent-" + suffix + "-" +
			strings.Repeat("x", 255-len("agent-")-len(suffix)-1)

		agent, err := repo.Create(
			ctx,
			persistence.CreateAgentRegistryInput{
				AgentReference: reference,
			},
		)
		if err != nil {
			t.Fatalf("create maximum-length reference: %v", err)
		}
		agentIDs = append(agentIDs, agent.ID)

		if len(agent.AgentReference) != 255 {
			t.Fatal("reference length mismatch")
		}
	})
}
