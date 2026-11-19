package database

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/gdiop88/RL_trader_infrastructure/internal/config"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

// MigrationRunner manages versioned PostgreSQL migrations.
type MigrationRunner struct {
	migrator *migrate.Migrate
}

// NewMigrationRunner initializes migrations using DATABASE_URL.
//
// This function is retained for compatibility with existing
// callers and unit tests.
//
// It does not apply migrations automatically.
func NewMigrationRunner(
	migrationsDir string,
) (*MigrationRunner, error) {
	databaseURL := strings.TrimSpace(
		os.Getenv("DATABASE_URL"),
	)

	return newMigrationRunner(
		migrationsDir,
		databaseURL,
	)
}

// NewMigrationRunnerWithConfig initializes migrations using
// the database configuration loaded from server.yaml.
//
// No DATABASE_URL environment variable is required.
//
// Creating the runner does not execute migrations.
func NewMigrationRunnerWithConfig(
	migrationsDir string,
	cfg *config.Config,
) (*MigrationRunner, error) {
	databaseURL, err := ConnectionString(cfg)
	if err != nil {
		return nil, fmt.Errorf(
			"build migration database configuration: %w",
			err,
		)
	}

	return newMigrationRunner(
		migrationsDir,
		databaseURL,
	)
}

// newMigrationRunner creates the migration engine.
//
// Both environment-based and YAML-based initialization
// use this implementation.
func newMigrationRunner(
	migrationsDir string,
	databaseURL string,
) (*MigrationRunner, error) {
	if err := ValidateConnectionString(databaseURL); err != nil {
		return nil, fmt.Errorf(
			"invalid database configuration: %w",
			err,
		)
	}

	if strings.TrimSpace(migrationsDir) == "" {
		return nil, errors.New(
			"migrations directory is required",
		)
	}

	absoluteDir, err := filepath.Abs(migrationsDir)
	if err != nil {
		return nil, fmt.Errorf(
			"resolve migrations directory: %w",
			err,
		)
	}

	info, err := os.Stat(absoluteDir)
	if err != nil {
		return nil, fmt.Errorf(
			"access migrations directory: %w",
			err,
		)
	}

	if !info.IsDir() {
		return nil, errors.New(
			"migrations path is not a directory",
		)
	}

	sourceURL := (&url.URL{
		Scheme: "file",
		Path:   filepath.ToSlash(absoluteDir),
	}).String()

	databaseConfig, err := url.Parse(databaseURL)
	if err != nil {
		return nil, errors.New(
			"invalid migration database URL",
		)
	}

	if databaseConfig.Scheme != "postgres" &&
		databaseConfig.Scheme != "postgresql" {
		return nil, errors.New(
			"migration runner requires a PostgreSQL URL",
		)
	}

	// Select the registered pgx/v5 migration driver.
	databaseConfig.Scheme = "pgx5"

	migrator, err := migrate.New(
		sourceURL,
		databaseConfig.String(),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"initialize migration runner: %w",
			err,
		)
	}

	return &MigrationRunner{
		migrator: migrator,
	}, nil
}

// Up applies all pending migrations.
//
// ErrNoChange is treated as success.
func (r *MigrationRunner) Up() error {
	if r == nil || r.migrator == nil {
		return errors.New(
			"migration runner is not initialized",
		)
	}

	err := r.migrator.Up()

	if errors.Is(err, migrate.ErrNoChange) {
		return nil
	}

	if err != nil {
		return fmt.Errorf(
			"apply database migrations: %w",
			err,
		)
	}

	return nil
}

// Down rolls back exactly one migration version.
//
// This operation may delete tables and data.
// It must be explicitly requested.
func (r *MigrationRunner) Down() error {
	if r == nil || r.migrator == nil {
		return errors.New(
			"migration runner is not initialized",
		)
	}

	err := r.migrator.Steps(-1)

	if errors.Is(err, migrate.ErrNoChange) {
		return nil
	}

	if err != nil {
		return fmt.Errorf(
			"rollback database migration: %w",
			err,
		)
	}

	return nil
}

// Version returns the current migration version
// and whether the database is in a dirty state.
func (r *MigrationRunner) Version() (
	uint,
	bool,
	error,
) {
	if r == nil || r.migrator == nil {
		return 0, false, errors.New(
			"migration runner is not initialized",
		)
	}

	version, dirty, err := r.migrator.Version()

	if errors.Is(err, migrate.ErrNilVersion) {
		return 0, false, nil
	}

	if err != nil {
		return 0, dirty, fmt.Errorf(
			"read migration version: %w",
			err,
		)
	}

	return version, dirty, nil
}

// Close releases migration source and database resources.
func (r *MigrationRunner) Close() error {
	if r == nil || r.migrator == nil {
		return nil
	}

	sourceErr, databaseErr := r.migrator.Close()

	return errors.Join(
		sourceErr,
		databaseErr,
	)
}
