package main

import (
	"fmt"
	"log"
	"os"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/rohitkeshwani07/chat/backend/internal/config"
)

func main() {
	logger := log.New(os.Stdout, "[MIGRATE] ", log.LstdFlags)
	logger.Println("Starting database migration...")

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		logger.Fatalf("Failed to load configuration: %v", err)
	}

	// Build database URL
	dbURL := fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		cfg.Database.User,
		cfg.Database.Password,
		cfg.Database.Host,
		cfg.Database.Port,
		cfg.Database.Database,
		cfg.Database.SSLMode,
	)

	// Get migrations path from environment or use default
	migrationsPath := os.Getenv("MIGRATIONS_PATH")
	if migrationsPath == "" {
		migrationsPath = "file:///migrations"
	}

	logger.Printf("Using migrations path: %s", migrationsPath)
	logger.Printf("Connecting to database: %s@%s:%d/%s", cfg.Database.User, cfg.Database.Host, cfg.Database.Port, cfg.Database.Database)

	// Create migration instance
	m, err := migrate.New(migrationsPath, dbURL)
	if err != nil {
		logger.Fatalf("Failed to create migration instance: %v", err)
	}
	defer m.Close()

	// Get command from argument (default: up)
	command := "up"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}

	logger.Printf("Running migration command: %s", command)

	// Execute migration command
	switch command {
	case "up":
		if err := m.Up(); err != nil && err != migrate.ErrNoChange {
			logger.Fatalf("Failed to run up migrations: %v", err)
		}
		logger.Println("Migrations applied successfully")

	case "down":
		if err := m.Down(); err != nil && err != migrate.ErrNoChange {
			logger.Fatalf("Failed to run down migrations: %v", err)
		}
		logger.Println("Migrations rolled back successfully")

	case "version":
		version, dirty, err := m.Version()
		if err != nil {
			logger.Fatalf("Failed to get migration version: %v", err)
		}
		logger.Printf("Current migration version: %d (dirty: %v)", version, dirty)

	case "force":
		if len(os.Args) < 3 {
			logger.Fatalf("Force command requires version argument")
		}
		var version int
		if _, err := fmt.Sscanf(os.Args[2], "%d", &version); err != nil {
			logger.Fatalf("Invalid version number: %v", err)
		}
		if err := m.Force(version); err != nil {
			logger.Fatalf("Failed to force version: %v", err)
		}
		logger.Printf("Forced migration to version %d", version)

	default:
		logger.Fatalf("Unknown command: %s (valid commands: up, down, version, force)", command)
	}

	logger.Println("Migration completed successfully")
}
