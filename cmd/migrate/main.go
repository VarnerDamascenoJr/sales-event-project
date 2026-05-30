package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/varner/sales-event-project/internal/config"
	"github.com/varner/sales-event-project/internal/database"
	"github.com/varner/sales-event-project/internal/observability"
)

const defaultMigrationsDir = "migrations"

func main() {
	cfg := config.Load()
	observability.ConfigureLogger("sales-event-migrate", cfg.AppEnv)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	db, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("connect postgres failed", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	dir := os.Getenv("MIGRATIONS_DIR")
	if dir == "" {
		dir = defaultMigrationsDir
	}

	applied, err := Apply(ctx, db, dir)
	if err != nil {
		slog.Error("apply migrations failed", "error", err)
		os.Exit(1)
	}
	slog.Info("migrations applied", "count", applied)
}

func Apply(ctx context.Context, db *pgxpool.Pool, dir string) (int, error) {
	migrations, err := loadMigrations(dir)
	if err != nil {
		return 0, err
	}
	if len(migrations) == 0 {
		return 0, errNoMigrations
	}

	if _, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version BIGINT PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		return 0, err
	}

	applied := 0
	for _, migration := range migrations {
		alreadyApplied, err := migrationApplied(ctx, db, migration.Version)
		if err != nil {
			return applied, err
		}
		if alreadyApplied {
			continue
		}

		if err := applyMigration(ctx, db, migration); err != nil {
			return applied, err
		}
		applied++
	}

	return applied, nil
}

func loadMigrations(dir string) ([]migration, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	migrations := make([]migration, 0)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".sql" {
			continue
		}

		version, err := parseVersion(entry.Name())
		if err != nil {
			return nil, err
		}

		path := filepath.Join(dir, entry.Name())
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}

		upSQL, err := parseUpSQL(string(content), entry.Name())
		if err != nil {
			return nil, err
		}
		migrations = append(migrations, migration{
			Version: version,
			Name:    entry.Name(),
			UpSQL:   upSQL,
		})
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})
	return migrations, nil
}

func parseVersion(filename string) (int64, error) {
	prefix, _, ok := strings.Cut(filename, "_")
	if !ok {
		return 0, fmt.Errorf("migration %q must start with numeric version and underscore", filename)
	}

	version, err := strconv.ParseInt(prefix, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("migration %q has invalid version: %w", filename, err)
	}
	return version, nil
}

func parseUpSQL(content string, filename string) (string, error) {
	_, afterUp, ok := strings.Cut(content, "-- +goose Up")
	if !ok {
		return "", fmt.Errorf("migration %q is missing -- +goose Up", filename)
	}

	upSQL, _, _ := strings.Cut(afterUp, "-- +goose Down")
	upSQL = strings.TrimSpace(upSQL)
	if upSQL == "" {
		return "", fmt.Errorf("migration %q has empty up section", filename)
	}
	return upSQL, nil
}

func migrationApplied(ctx context.Context, db *pgxpool.Pool, version int64) (bool, error) {
	var exists bool
	err := db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM schema_migrations
			WHERE version = $1
		)
	`, version).Scan(&exists)
	return exists, err
}

func applyMigration(ctx context.Context, db *pgxpool.Pool, migration migration) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if err := execStatements(ctx, tx, migration.UpSQL); err != nil {
		return fmt.Errorf("apply migration %s: %w", migration.Name, err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO schema_migrations (version, name)
		VALUES ($1, $2)
	`, migration.Version, migration.Name); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	slog.Info("migration applied", "version", migration.Version, "name", migration.Name)
	return nil
}

func execStatements(ctx context.Context, tx pgx.Tx, sql string) error {
	statements := splitSQLStatements(sql)
	for _, statement := range statements {
		if _, err := tx.Exec(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func splitSQLStatements(sql string) []string {
	rawStatements := strings.Split(sql, ";")
	statements := make([]string, 0, len(rawStatements))
	for _, statement := range rawStatements {
		statement = strings.TrimSpace(statement)
		if statement == "" {
			continue
		}
		statements = append(statements, statement)
	}
	return statements
}

type migration struct {
	Version int64
	Name    string
	UpSQL   string
}

var errNoMigrations = errors.New("no migrations found")
