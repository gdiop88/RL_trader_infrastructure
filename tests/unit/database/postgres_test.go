package database_test

import (
	"strings"
	"testing"

	"github.com/gdiop88/RL_trader_infrastructure/internal/database"
)

func TestValidateConnectionString(t *testing.T) {
	tests := []struct {
		name      string
		conn      string
		wantError bool
	}{
		{
			name:      "missing DATABASE_URL",
			conn:      "",
			wantError: true,
		},
		{
			name:      "TLS disabled",
			conn:      "postgres://user:pass@localhost/db?sslmode=disable",
			wantError: true,
		},
		{
			name:      "TLS allow",
			conn:      "postgres://user:pass@localhost/db?sslmode=allow",
			wantError: true,
		},
		{
			name:      "TLS prefer",
			conn:      "postgres://user:pass@localhost/db?sslmode=prefer",
			wantError: true,
		},
		{
			name:      "TLS require",
			conn:      "postgres://user:pass@localhost/db?sslmode=require",
			wantError: true,
		},
		{
			name:      "TLS verify-ca",
			conn:      "postgres://user:pass@localhost/db?sslmode=verify-ca",
			wantError: true,
		},
		{
			name:      "missing sslmode",
			conn:      "postgres://user:pass@localhost/db",
			wantError: true,
		},
		{
			name:      "valid PostgreSQL URI",
			conn:      "postgres://user:pass@localhost/db?sslmode=verify-full",
			wantError: false,
		},
		{
			name:      "valid PostgreSQL alternative URI",
			conn:      "postgresql://user:pass@localhost/db?sslmode=verify-full",
			wantError: false,
		},
		{
			name:      "valid keyword value",
			conn:      "host=localhost user=user password=pass dbname=db sslmode=verify-full",
			wantError: false,
		},
		{
			name:      "quoted keyword value",
			conn:      "host='localhost' user='test user' dbname=db sslmode='verify-full'",
			wantError: false,
		},
		{
			name:      "TLS require keyword value",
			conn:      "host=localhost dbname=db sslmode=require",
			wantError: true,
		},
		{
			name:      "duplicate URI sslmode",
			conn:      "postgres://user:pass@localhost/db?sslmode=verify-full&sslmode=disable",
			wantError: true,
		},
		{
			name:      "duplicate keyword sslmode",
			conn:      "host=localhost sslmode=verify-full sslmode=disable",
			wantError: true,
		},
		{
			name:      "unterminated quoted value",
			conn:      "host=localhost sslmode='verify-full",
			wantError: true,
		},
		{
			name:      "missing hostname",
			conn:      "postgres://user:pass@/db?sslmode=verify-full",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := database.ValidateConnectionString(tt.conn)

			if tt.wantError && err == nil {
				t.Fatal("expected validation error, got nil")
			}

			if !tt.wantError && err != nil {
				t.Fatalf("unexpected validation error: %v", err)
			}
		})
	}
}

func TestConnectMissingDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")

	_, err := database.Connect(t.Context())

	if err == nil {
		t.Fatal("expected missing DATABASE_URL error")
	}

	if !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDatabaseNilSafety(t *testing.T) {
	var db *database.Database

	if err := db.Ping(t.Context()); err == nil {
		t.Fatal("expected Ping error for nil database")
	}

	if db.Pool() != nil {
		t.Fatal("expected nil connection pool")
	}

	// Closing an uninitialized database must not panic.
	db.Close()
}
