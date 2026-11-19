package database_test

import (
	"strings"
	"testing"

	"github.com/gdiop88/RL_trader_infrastructure/internal/database"
)

func TestNewMigrationRunner(t *testing.T) {
	tests := []struct {
		name        string
		databaseURL string
		directory   string
		wantError   string
	}{
		{
			name:        "missing database URL",
			databaseURL: "",
			directory:   "../../../migrations",
			wantError:   "DATABASE_URL",
		},
		{
			name:        "insecure database connection",
			databaseURL: "postgres://user:pass@localhost/db?sslmode=disable",
			directory:   "../../../migrations",
			wantError:   "sslmode=verify-full",
		},
		{
			name:        "missing migrations directory",
			databaseURL: "postgres://user:pass@localhost/db?sslmode=verify-full",
			directory:   "",
			wantError:   "migrations directory is required",
		},
		{
			name:        "nonexistent migrations directory",
			databaseURL: "postgres://user:pass@localhost/db?sslmode=verify-full",
			directory:   "nonexistent-migrations-directory",
			wantError:   "access migrations directory",
		},
		{
			name:        "migration path is a file",
			databaseURL: "postgres://user:pass@localhost/db?sslmode=verify-full",
			directory:   "migrations_test.go",
			wantError:   "migrations path is not a directory",
		},
		{
			name:        "keyword value connection string",
			databaseURL: "host=localhost user=user dbname=db sslmode=verify-full",
			directory:   "../../../migrations",
			wantError:   "migration runner requires a PostgreSQL URL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("DATABASE_URL", tt.databaseURL)

			runner, err := database.NewMigrationRunner(tt.directory)

			if runner != nil {
				defer runner.Close()
			}

			if err == nil {
				t.Fatal("expected an error, got nil")
			}

			if !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf(
					"expected error containing %q, got %q",
					tt.wantError,
					err.Error(),
				)
			}
		})
	}
}

func TestMigrationRunnerNilSafety(t *testing.T) {
	var runner *database.MigrationRunner

	t.Run("Up", func(t *testing.T) {
		if err := runner.Up(); err == nil {
			t.Fatal("expected error for nil runner")
		}
	})

	t.Run("Down", func(t *testing.T) {
		if err := runner.Down(); err == nil {
			t.Fatal("expected error for nil runner")
		}
	})

	t.Run("Version", func(t *testing.T) {
		_, _, err := runner.Version()
		if err == nil {
			t.Fatal("expected error for nil runner")
		}
	})

	t.Run("Close", func(t *testing.T) {
		if err := runner.Close(); err != nil {
			t.Fatalf("unexpected Close error: %v", err)
		}
	})
}
