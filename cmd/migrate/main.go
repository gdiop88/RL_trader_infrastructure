package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/gdiop88/RL_trader_infrastructure/internal/config"
	"github.com/gdiop88/RL_trader_infrastructure/internal/database"
)

func main() {
	action := flag.String(
		"action",
		"version",
		"Migration action: version or up",
	)

	configPath := flag.String(
		"config",
		"configs/server.yaml",
		"Path to server configuration",
	)

	migrationsDir := flag.String(
		"migrations",
		"migrations",
		"Path to SQL migrations",
	)

	flag.Parse()

	if *action != "version" && *action != "up" {
		log.Fatal("unsupported migration action")
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("load configuration: %v", err)
	}

	runner, err := database.NewMigrationRunnerWithConfig(
		*migrationsDir,
		cfg,
	)
	if err != nil {
		log.Fatalf("initialize migration runner: %v", err)
	}

	defer func() {
		if err := runner.Close(); err != nil {
			log.Printf("close migration runner: %v", err)
		}
	}()

	if *action == "up" {
		if err := runner.Up(); err != nil {
			log.Fatalf("migration failed: %v", err)
		}
		fmt.Println("Database migration applied successfully")
	}

	version, dirty, err := runner.Version()
	if err != nil {
		log.Fatalf("read migration version: %v", err)
	}

	fmt.Printf(
		"Migration version: %d\nDirty: %t\n",
		version,
		dirty,
	)

	if dirty {
		os.Exit(1)
	}
}
