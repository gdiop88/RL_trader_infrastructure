package database

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gdiop88/RL_trader_infrastructure/internal/config"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	defaultMaxConnections = 10
	defaultMinConnections = 2

	connectionTimeout  = 10 * time.Second
	healthCheckTimeout = 5 * time.Second
)

type Database struct {
	pool *pgxpool.Pool
}

// ConnectionString constructs a PostgreSQL connection URL
// from the database settings loaded from YAML.
//
// Passwords and other credentials are URL-escaped.
// The resulting URL must not be logged.
func ConnectionString(cfg *config.Config) (string, error) {
	if cfg == nil {
		return "", errors.New(
			"database configuration is nil",
		)
	}

	db := cfg.Database

	if db.Driver != "postgres" {
		return "", errors.New(
			"database.driver must be postgres",
		)
	}

	if strings.TrimSpace(db.Host) == "" {
		return "", errors.New(
			"database.host is required",
		)
	}

	if db.Port < 1 || db.Port > 65535 {
		return "", errors.New(
			"database.port must be between 1 and 65535",
		)
	}

	if strings.TrimSpace(db.Name) == "" {
		return "", errors.New(
			"database.name is required",
		)
	}

	if strings.TrimSpace(db.User) == "" {
		return "", errors.New(
			"database.user is required",
		)
	}

	if db.Password == "" {
		return "", errors.New(
			"database.password is required",
		)
	}

	if db.SSLMode != "verify-full" {
		return "", errors.New(
			"database.sslmode must be verify-full",
		)
	}

	if strings.TrimSpace(db.SSLRootCert) == "" {
		return "", errors.New(
			"database.sslrootcert is required",
		)
	}

	// url.UserPassword safely escapes credentials
	// containing special characters.
	databaseURL := &url.URL{
		Scheme: "postgres",
		User: url.UserPassword(
			db.User,
			db.Password,
		),
		Host: fmt.Sprintf(
			"%s:%s",
			db.Host,
			strconv.Itoa(db.Port),
		),
		Path: "/" + db.Name,
	}

	query := databaseURL.Query()
	query.Set("sslmode", db.SSLMode)
	query.Set("sslrootcert", db.SSLRootCert)

	databaseURL.RawQuery = query.Encode()

	connString := databaseURL.String()

	if err := ValidateConnectionString(connString); err != nil {
		return "", fmt.Errorf(
			"invalid PostgreSQL configuration: %w",
			err,
		)
	}

	return connString, nil
}

// ValidateConnectionString verifies the security requirements
// for a managed PostgreSQL connection.
//
// An explicit sslmode=verify-full is required.
// URI and keyword/value formats are supported.
func ValidateConnectionString(connString string) error {
	connString = strings.TrimSpace(connString)

	if connString == "" {
		return errors.New("DATABASE_URL is not configured")
	}

	sslMode, err := extractSSLMode(connString)
	if err != nil {
		return err
	}

	if sslMode != "verify-full" {
		return fmt.Errorf(
			"PostgreSQL requires sslmode=verify-full; got %q",
			sslMode,
		)
	}

	parsedConfig, err := pgconn.ParseConfig(connString)
	if err != nil {
		return fmt.Errorf(
			"parse PostgreSQL configuration: %w",
			err,
		)
	}

	if err := validateTLSConfig(parsedConfig.TLSConfig); err != nil {
		return err
	}

	for _, fallback := range parsedConfig.Fallbacks {
		if err := validateTLSConfig(fallback.TLSConfig); err != nil {
			return fmt.Errorf(
				"insecure PostgreSQL fallback: %w",
				err,
			)
		}
	}

	return nil
}

func validateTLSConfig(tlsConfig *tls.Config) error {
	if tlsConfig == nil {
		return errors.New("PostgreSQL TLS is required")
	}

	if tlsConfig.InsecureSkipVerify {
		return errors.New(
			"PostgreSQL TLS certificate verification must be enabled",
		)
	}

	if tlsConfig.ServerName == "" {
		return errors.New(
			"PostgreSQL TLS hostname verification is required",
		)
	}

	return nil
}

// extractSSLMode reads the explicit sslmode parameter.
//
// For keyword/value strings, PostgreSQL-style quoting and
// escaping are handled by the parser below.
func extractSSLMode(connString string) (string, error) {
	if strings.HasPrefix(connString, "postgres://") ||
		strings.HasPrefix(connString, "postgresql://") {

		parsed, err := url.Parse(connString)
		if err != nil {
			return "", fmt.Errorf(
				"parse PostgreSQL URL: %w",
				err,
			)
		}

		modes := parsed.Query()["sslmode"]

		if len(modes) != 1 {
			return "", errors.New(
				"PostgreSQL URL must specify exactly one sslmode",
			)
		}

		return modes[0], nil
	}

	params, err := parseKeywordValues(connString)
	if err != nil {
		return "", err
	}

	mode, ok := params["sslmode"]
	if !ok {
		return "", errors.New(
			"PostgreSQL connection string must explicitly specify sslmode",
		)
	}

	return mode, nil
}

// parseKeywordValues parses libpq-style keyword/value parameters.
// It supports quoted values, escaped characters, and whitespace.
func parseKeywordValues(input string) (map[string]string, error) {
	result := make(map[string]string)
	i := 0

	for i < len(input) {
		for i < len(input) && isSpace(input[i]) {
			i++
		}

		if i == len(input) {
			break
		}

		start := i

		for i < len(input) &&
			!isSpace(input[i]) &&
			input[i] != '=' {
			i++
		}

		key := input[start:i]

		for i < len(input) && isSpace(input[i]) {
			i++
		}

		if key == "" || i >= len(input) || input[i] != '=' {
			return nil, errors.New(
				"invalid PostgreSQL keyword/value connection string",
			)
		}

		i++

		for i < len(input) && isSpace(input[i]) {
			i++
		}

		var value strings.Builder

		if i < len(input) && input[i] == '\'' {
			i++
			closed := false

			for i < len(input) {
				switch input[i] {
				case '\\':
					i++
					if i >= len(input) {
						return nil, errors.New(
							"invalid escape sequence",
						)
					}
					value.WriteByte(input[i])
					i++

				case '\'':
					i++
					closed = true

				default:
					value.WriteByte(input[i])
					i++
				}

				if closed {
					break
				}
			}

			if !closed {
				return nil, errors.New(
					"unterminated quoted value",
				)
			}

			if i < len(input) && !isSpace(input[i]) {
				return nil, errors.New(
					"invalid characters after quoted value",
				)
			}
		} else {
			for i < len(input) && !isSpace(input[i]) {
				if input[i] == '\\' {
					i++
					if i >= len(input) {
						return nil, errors.New(
							"invalid escape sequence",
						)
					}
				}

				value.WriteByte(input[i])
				i++
			}
		}

		if _, exists := result[key]; exists {
			return nil, fmt.Errorf(
				"duplicate PostgreSQL connection parameter %q",
				key,
			)
		}

		result[key] = value.String()
	}

	return result, nil
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

// Connect initializes the PostgreSQL pool using DATABASE_URL.
//
// Retained for compatibility with existing callers and tests.
func Connect(ctx context.Context) (*Database, error) {
	databaseURL := strings.TrimSpace(
		os.Getenv("DATABASE_URL"),
	)

	return connectWithURL(ctx, databaseURL)
}

// ConnectWithConfig initializes the PostgreSQL pool
// using credentials loaded from server.yaml.
func ConnectWithConfig(
	ctx context.Context,
	cfg *config.Config,
) (*Database, error) {
	databaseURL, err := ConnectionString(cfg)
	if err != nil {
		return nil, err
	}

	return connectWithURL(ctx, databaseURL)
}

// connectWithURL initializes and verifies the PostgreSQL pool.
func connectWithURL(
	ctx context.Context,
	databaseURL string,
) (*Database, error) {
	if err := ValidateConnectionString(databaseURL); err != nil {
		return nil, err
	}

	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf(
			"parse database pool configuration: %w",
			err,
		)
	}

	poolConfig.MaxConns = defaultMaxConnections
	poolConfig.MinConns = defaultMinConnections
	poolConfig.MaxConnLifetime = time.Hour
	poolConfig.MaxConnIdleTime = 30 * time.Minute
	poolConfig.HealthCheckPeriod = time.Minute

	connectCtx, cancel := context.WithTimeout(
		ctx,
		connectionTimeout,
	)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(
		connectCtx,
		poolConfig,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"create PostgreSQL connection pool: %w",
			err,
		)
	}

	if err := pool.Ping(connectCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf(
			"connect to PostgreSQL: %w",
			err,
		)
	}

	return &Database{pool: pool}, nil
}

// Ping checks database connectivity.
func (db *Database) Ping(ctx context.Context) error {
	if db == nil || db.pool == nil {
		return errors.New(
			"PostgreSQL connection is not initialized",
		)
	}

	pingCtx, cancel := context.WithTimeout(
		ctx,
		healthCheckTimeout,
	)
	defer cancel()

	return db.pool.Ping(pingCtx)
}

// Pool returns the underlying pgx connection pool.
func (db *Database) Pool() *pgxpool.Pool {
	if db == nil {
		return nil
	}

	return db.pool
}

// Close releases all PostgreSQL connections.
func (db *Database) Close() {
	if db == nil || db.pool == nil {
		return
	}

	db.pool.Close()
}
