package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

// Config defines the infrastructure service configuration.
type Config struct {
	Service struct {
		Name            string `yaml:"name"`
		Environment     string `yaml:"environment"`
		ShutdownTimeout string `yaml:"shutdown_timeout"`
	} `yaml:"service"`

	Server struct {
		GRPC struct {
			Enabled bool   `yaml:"enabled"`
			Address string `yaml:"address"`
			Port    int    `yaml:"port"`
		} `yaml:"grpc"`

		TLS struct {
			Enabled    bool   `yaml:"enabled"`
			CertFile   string `yaml:"cert_file"`
			KeyFile    string `yaml:"key_file"`
			ClientCA   string `yaml:"client_ca"`
			ClientAuth bool   `yaml:"client_auth"`
		} `yaml:"tls"`
	} `yaml:"server"`

	Database struct {
		Driver      string `yaml:"driver"`
		Host        string `yaml:"host"`
		Port        int    `yaml:"port"`
		Name        string `yaml:"name"`
		User        string `yaml:"user"`
		Password    string `yaml:"password"`
		SSLMode     string `yaml:"sslmode"`
		SSLRootCert string `yaml:"sslrootcert"`
	} `yaml:"database"`

	Secrets struct {
		Provider string `yaml:"provider"`
		Address  string `yaml:"address"`
	} `yaml:"secrets"`

	Provisioning struct {
		Engine                  string `yaml:"engine"`
		MaxConcurrentOperations int    `yaml:"max_concurrent_operations"`
	} `yaml:"provisioning"`

	Providers map[string]struct {
		Enabled bool `yaml:"enabled"`
	} `yaml:"providers"`

	Logging struct {
		Level  string `yaml:"level"`
		Format string `yaml:"format"`
	} `yaml:"logging"`
}

// Load reads, parses, resolves paths, and validates
// the infrastructure service configuration.
func Load(path string) (*Config, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf(
			"configuration path is required",
		)
	}

	configPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf(
			"resolve configuration path: %w",
			err,
		)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf(
			"read configuration: %w",
			err,
		)
	}

	var cfg Config

	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)

	if err := decoder.Decode(&cfg); err != nil {
		return nil, fmt.Errorf(
			"parse configuration: %w",
			err,
		)
	}

	// Resolve TLS certificate paths relative to
	// the configuration file directory.
	configDir := filepath.Dir(configPath)

	cfg.Server.TLS.CertFile = resolvePath(
		configDir,
		cfg.Server.TLS.CertFile,
	)

	cfg.Server.TLS.KeyFile = resolvePath(
		configDir,
		cfg.Server.TLS.KeyFile,
	)

	cfg.Server.TLS.ClientCA = resolvePath(
		configDir,
		cfg.Server.TLS.ClientCA,
	)

	// Resolve the PostgreSQL CA certificate path.
	cfg.Database.SSLRootCert = resolvePath(
		configDir,
		cfg.Database.SSLRootCert,
	)

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf(
			"validate configuration: %w",
			err,
		)
	}

	return &cfg, nil
}

// resolvePath resolves relative paths against the
// configuration directory.
//
// Absolute paths remain absolute.
// Empty paths remain empty.
func resolvePath(baseDir, path string) string {
	if path == "" {
		return ""
	}

	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}

	return filepath.Join(baseDir, path)
}

// Validate checks the configuration for invalid settings.
func (cfg *Config) Validate() error {
	if cfg == nil {
		return fmt.Errorf(
			"configuration is nil",
		)
	}

	// ==========================================
	// Service configuration
	// ==========================================

	if cfg.Service.Name == "" {
		return fmt.Errorf(
			"service.name is required",
		)
	}

	if cfg.Service.Environment == "" {
		return fmt.Errorf(
			"service.environment is required",
		)
	}

	timeout, err := time.ParseDuration(
		cfg.Service.ShutdownTimeout,
	)
	if err != nil || timeout <= 0 {
		return fmt.Errorf(
			"service.shutdown_timeout must be a positive duration",
		)
	}

	// ==========================================
	// gRPC configuration
	// ==========================================

	if cfg.Server.GRPC.Enabled {
		if cfg.Server.GRPC.Address == "" {
			return fmt.Errorf(
				"server.grpc.address is required",
			)
		}

		if cfg.Server.GRPC.Port < 1 ||
			cfg.Server.GRPC.Port > 65535 {
			return fmt.Errorf(
				"server.grpc.port must be between 1 and 65535",
			)
		}

		// ======================================
		// TLS configuration
		// ======================================

		if !cfg.Server.TLS.Enabled {
			return fmt.Errorf(
				"gRPC requires TLS to be enabled",
			)
		}

		if !cfg.Server.TLS.ClientAuth {
			return fmt.Errorf(
				"gRPC requires mutual TLS client authentication",
			)
		}

		if cfg.Server.TLS.CertFile == "" {
			return fmt.Errorf(
				"server.tls.cert_file is required",
			)
		}

		if cfg.Server.TLS.KeyFile == "" {
			return fmt.Errorf(
				"server.tls.key_file is required",
			)
		}

		if cfg.Server.TLS.ClientCA == "" {
			return fmt.Errorf(
				"server.tls.client_ca is required",
			)
		}
	}

	// ==========================================
	// Database configuration
	// ==========================================

	if cfg.Database.Driver != "postgres" {
		return fmt.Errorf(
			"database.driver must be postgres",
		)
	}

	if strings.TrimSpace(cfg.Database.Host) == "" {
		return fmt.Errorf(
			"database.host is required",
		)
	}

	if cfg.Database.Port < 1 ||
		cfg.Database.Port > 65535 {
		return fmt.Errorf(
			"database.port must be between 1 and 65535",
		)
	}

	if strings.TrimSpace(cfg.Database.Name) == "" {
		return fmt.Errorf(
			"database.name is required",
		)
	}

	if strings.TrimSpace(cfg.Database.User) == "" {
		return fmt.Errorf(
			"database.user is required",
		)
	}

	if cfg.Database.Password == "" {
		return fmt.Errorf(
			"database.password is required",
		)
	}

	if cfg.Database.SSLMode != "verify-full" {
		return fmt.Errorf(
			"database.sslmode must be verify-full",
		)
	}

	if strings.TrimSpace(cfg.Database.SSLRootCert) == "" {
		return fmt.Errorf(
			"database.sslrootcert is required",
		)
	}

	// ==========================================
	// Secrets configuration
	// ==========================================

	switch cfg.Secrets.Provider {
	case "none":
		// Development mode does not use a
		// managed secrets provider.

	case "vault":
		if strings.TrimSpace(cfg.Secrets.Address) == "" {
			return fmt.Errorf(
				"secrets.address is required when provider is vault",
			)
		}

	default:
		return fmt.Errorf(
			"secrets.provider must be none or vault",
		)
	}

	// ==========================================
	// Provisioning configuration
	// ==========================================

	if cfg.Provisioning.Engine == "" {
		return fmt.Errorf(
			"provisioning.engine is required",
		)
	}

	if cfg.Provisioning.MaxConcurrentOperations < 1 {
		return fmt.Errorf(
			"provisioning.max_concurrent_operations must be positive",
		)
	}

	// ==========================================
	// Logging configuration
	// ==========================================

	if cfg.Logging.Level == "" {
		return fmt.Errorf(
			"logging.level is required",
		)
	}

	if cfg.Logging.Format == "" {
		return fmt.Errorf(
			"logging.format is required",
		)
	}

	return nil
}
