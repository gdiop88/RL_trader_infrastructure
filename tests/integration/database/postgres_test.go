package database_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/gdiop88/RL_trader_infrastructure/internal/config"
	"github.com/gdiop88/RL_trader_infrastructure/internal/database"
)

func TestPostgreSQLIntegration(t *testing.T) {
	configPath := filepath.Join(
		"..", "..", "..", "configs", "server.yaml",
	)

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("load configuration: %v", err)
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		20*time.Second,
	)
	defer cancel()

	db, err := database.ConnectWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("connect to PostgreSQL: %v", err)
	}
	defer db.Close()

	if err := db.Ping(ctx); err != nil {
		t.Fatalf("ping PostgreSQL: %v", err)
	}

	var databaseName string
	var databaseUser string
	var connectionTest int

	err = db.Pool().QueryRow(
		ctx,
		"SELECT current_database(), current_user, 1",
	).Scan(
		&databaseName,
		&databaseUser,
		&connectionTest,
	)
	if err != nil {
		t.Fatalf("execute PostgreSQL query: %v", err)
	}

	if databaseName != cfg.Database.Name {
		t.Fatalf(
			"unexpected database: got %q, want %q",
			databaseName,
			cfg.Database.Name,
		)
	}

	if databaseUser != cfg.Database.User {
		t.Fatalf(
			"unexpected user: got %q, want %q",
			databaseUser,
			cfg.Database.User,
		)
	}

	if connectionTest != 1 {
		t.Fatalf(
			"unexpected query result: got %d, want 1",
			connectionTest,
		)
	}

	t.Logf(
		"PostgreSQL integration successful: database=%s user=%s",
		databaseName,
		databaseUser,
	)
}
