package migrations

import (
	"context"
	"embed"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed sql/*.sql
var SQLFiles embed.FS

const (
	AdvisoryLockID = 789123456
	TargetVersion  = 14
)

type Migration struct {
	Version int64
	Name    string
	UpSQL   string
	DownSQL string
}

// LoadMigrations parses all embedded .up.sql and .down.sql files and returns them sorted by version.
func LoadMigrations() ([]Migration, error) {
	entries, err := SQLFiles.ReadDir("sql")
	if err != nil {
		return nil, fmt.Errorf("read embedded sql directory: %w", err)
	}

	upMap := make(map[int64]struct {
		name string
		sql  string
	})
	downMap := make(map[int64]string)

	for _, entry := range entries {
		filename := entry.Name()
		if !strings.HasSuffix(filename, ".sql") {
			continue
		}

		parts := strings.Split(filename, "_")
		if len(parts) < 2 {
			continue
		}

		ver, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			continue
		}

		data, err := SQLFiles.ReadFile("sql/" + filename)
		if err != nil {
			return nil, fmt.Errorf("read file %s: %w", filename, err)
		}

		if strings.HasSuffix(filename, ".up.sql") {
			cleanName := strings.TrimSuffix(strings.TrimPrefix(filename, parts[0]+"_"), ".up.sql")
			upMap[ver] = struct {
				name string
				sql  string
			}{name: cleanName, sql: string(data)}
		} else if strings.HasSuffix(filename, ".down.sql") {
			downMap[ver] = string(data)
		}
	}

	var migrations []Migration
	for ver, up := range upMap {
		migrations = append(migrations, Migration{
			Version: ver,
			Name:    up.name,
			UpSQL:   up.sql,
			DownSQL: downMap[ver],
		})
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	return migrations, nil
}

// EnsureMigrationsTable creates the schema_migrations tracking table if it does not exist.
func EnsureMigrationsTable(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version     BIGINT PRIMARY KEY,
			name        TEXT NOT NULL,
			applied_at  TIMESTAMPTZ NOT NULL DEFAULT now()
		);
	`)
	return err
}

// GetCurrentVersion returns the latest applied migration version in the database.
func GetCurrentVersion(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	if err := EnsureMigrationsTable(ctx, pool); err != nil {
		return 0, err
	}

	var ver *int64
	err := pool.QueryRow(ctx, `SELECT MAX(version) FROM schema_migrations`).Scan(&ver)
	if err != nil || ver == nil {
		return 0, nil
	}
	return *ver, nil
}

// ApplyPending applies all unapplied up migrations in ascending order inside transactions under an advisory lock.
func ApplyPending(ctx context.Context, pool *pgxpool.Pool) (int, error) {
	// 1. Acquire global migration advisory lock
	_, err := pool.Exec(ctx, `SELECT pg_advisory_lock($1)`, AdvisoryLockID)
	if err != nil {
		return 0, fmt.Errorf("acquire migration advisory lock: %w", err)
	}
	defer func() {
		_, _ = pool.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, AdvisoryLockID)
	}()

	if err := EnsureMigrationsTable(ctx, pool); err != nil {
		return 0, fmt.Errorf("ensure migrations table: %w", err)
	}

	// 2. Fetch applied versions
	rows, err := pool.Query(ctx, `SELECT version FROM schema_migrations ORDER BY version ASC`)
	if err != nil {
		return 0, fmt.Errorf("query applied migrations: %w", err)
	}
	defer rows.Close()

	applied := make(map[int64]bool)
	for rows.Next() {
		var v int64
		if err := rows.Scan(&v); err == nil {
			applied[v] = true
		}
	}
	rows.Close()

	allMigrations, err := LoadMigrations()
	if err != nil {
		return 0, err
	}

	var appliedCount int
	for _, m := range allMigrations {
		if applied[m.Version] {
			continue
		}

		log.Printf("==> Applying Migration %06d_%s.up.sql...", m.Version, m.Name)
		tx, err := pool.Begin(ctx)
		if err != nil {
			return appliedCount, fmt.Errorf("begin transaction for migration %d: %w", m.Version, err)
		}

		if strings.TrimSpace(m.UpSQL) != "" {
			if _, err := tx.Exec(ctx, m.UpSQL); err != nil {
				_ = tx.Rollback(ctx)
				return appliedCount, fmt.Errorf("migration %d (%s) failed: %w", m.Version, m.Name, err)
			}
		}

		_, err = tx.Exec(ctx,
			`INSERT INTO schema_migrations (version, name, applied_at)
			 VALUES ($1, $2, $3)
			 ON CONFLICT (version) DO UPDATE SET name = EXCLUDED.name, applied_at = EXCLUDED.applied_at`,
			m.Version, m.Name, time.Now().UTC(),
		)
		if err != nil {
			_ = tx.Rollback(ctx)
			return appliedCount, fmt.Errorf("record migration %d in schema_migrations: %w", m.Version, err)
		}

		if err := tx.Commit(ctx); err != nil {
			return appliedCount, fmt.Errorf("commit migration %d: %w", m.Version, err)
		}

		log.Printf("==> Successfully applied Migration %06d_%s.", m.Version, m.Name)
		appliedCount++
	}

	return appliedCount, nil
}

// RollbackLast rolls back the latest applied migration using its down SQL.
func RollbackLast(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	_, err := pool.Exec(ctx, `SELECT pg_advisory_lock($1)`, AdvisoryLockID)
	if err != nil {
		return 0, fmt.Errorf("acquire migration advisory lock: %w", err)
	}
	defer func() {
		_, _ = pool.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, AdvisoryLockID)
	}()

	currentVer, err := GetCurrentVersion(ctx, pool)
	if err != nil || currentVer == 0 {
		return 0, fmt.Errorf("no migrations to rollback")
	}

	allMigrations, err := LoadMigrations()
	if err != nil {
		return 0, err
	}

	var target *Migration
	for _, m := range allMigrations {
		if m.Version == currentVer {
			target = &m
			break
		}
	}

	if target == nil {
		return 0, fmt.Errorf("migration version %d not found in files", currentVer)
	}

	if strings.TrimSpace(target.DownSQL) == "" {
		return 0, fmt.Errorf("migration %d does not have down SQL definition", currentVer)
	}

	log.Printf("==> Rolling back Migration %06d_%s...", target.Version, target.Name)
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}

	if _, err := tx.Exec(ctx, target.DownSQL); err != nil {
		_ = tx.Rollback(ctx)
		return 0, fmt.Errorf("rollback migration %d failed: %w", target.Version, err)
	}

	if _, err := tx.Exec(ctx, `DELETE FROM schema_migrations WHERE version = $1`, target.Version); err != nil {
		_ = tx.Rollback(ctx)
		return 0, fmt.Errorf("delete from schema_migrations failed: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}

	log.Printf("==> Successfully rolled back Migration %06d_%s.", target.Version, target.Name)
	return target.Version, nil
}

// VerifySchemaVersion asserts that the database schema is at or ahead of expectedVersion.
// Server boot calls this to fail fast if migrations have not been applied.
func VerifySchemaVersion(ctx context.Context, pool *pgxpool.Pool, expectedVersion int64) error {
	currentVer, err := GetCurrentVersion(ctx, pool)
	if err != nil {
		return fmt.Errorf("query schema version: %w", err)
	}

	if currentVer < expectedVersion {
		return fmt.Errorf("FATAL: database schema version %d is behind required version %d (run migrations to update)", currentVer, expectedVersion)
	}

	return nil
}
