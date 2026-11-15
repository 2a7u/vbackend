package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"venus_backend/internal/db"
	"venus_backend/internal/db/migrations"
)

func main() {
	upFlag := flag.Bool("up", false, "Apply all pending versioned migrations")
	downFlag := flag.Bool("down", false, "Rollback the most recent migration")
	versionFlag := flag.Bool("version", false, "Print the current database schema version")
	flag.Parse()

	if !*upFlag && !*downFlag && !*versionFlag {
		fmt.Println("Usage:")
		fmt.Println("  go run ./cmd/migrate -up        (Apply pending migrations)")
		fmt.Println("  go run ./cmd/migrate -down      (Rollback last migration)")
		fmt.Println("  go run ./cmd/migrate -version   (Show current schema version)")
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	dbURL := os.Getenv("DATABASE_URL")
	if strings.TrimSpace(dbURL) == "" {
		dbURL = os.Getenv("TEST_DATABASE_URL")
	}
	if strings.TrimSpace(dbURL) == "" {
		dbURL = "postgres://postgres:postgres@localhost:5432/venus_db?sslmode=disable"
	}

	pool, err := db.Connect(ctx, dbURL)
	if err != nil {
		log.Fatalf("Database connection failed: %v", err)
	}
	defer pool.Close()

	if *versionFlag {
		ver, err := migrations.GetCurrentVersion(ctx, pool)
		if err != nil {
			log.Fatalf("Failed to fetch schema version: %v", err)
		}
		fmt.Printf("Current Database Schema Version: %d (Latest Available: %d)\n", ver, migrations.TargetVersion)
		return
	}

	if *downFlag {
		rolledBack, err := migrations.RollbackLast(ctx, pool)
		if err != nil {
			log.Fatalf("Rollback failed: %v", err)
		}
		fmt.Printf("Successfully rolled back migration version %d.\n", rolledBack)
		return
	}

	if *upFlag {
		applied, err := migrations.ApplyPending(ctx, pool)
		if err != nil {
			log.Fatalf("Migration failed: %v", err)
		}
		fmt.Printf("Migrations complete. Applied %d pending migrations. Target version: %d\n", applied, migrations.TargetVersion)
	}
}
