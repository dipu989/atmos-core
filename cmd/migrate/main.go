// cmd/migrate runs all pending SQL migrations in order.
// It tracks applied migrations in a schema_migrations table so it is
// safe to run multiple times — already-applied migrations are skipped.
//
// Usage:
//
//	go run ./cmd/migrate            # apply all pending migrations
//	go run ./cmd/migrate --dry-run  # print pending migrations without applying
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/dipu/atmos-core/config"
	"github.com/dipu/atmos-core/platform/logger"
	_ "github.com/jackc/pgx/v5/stdlib"
	"go.uber.org/zap"
)

func main() {
	dryRun := flag.Bool("dry-run", false, "print pending migrations without applying them")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}
	logger.Init(cfg.App.Env)
	defer logger.Sync()
	log := logger.L()

	db, err := sql.Open("pgx", cfg.DB.DSN())
	if err != nil {
		log.Fatal("failed to open db", zap.Error(err))
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatal("failed to connect to db", zap.Error(err))
	}

	if err := ensureMigrationsTable(db); err != nil {
		log.Fatal("failed to create schema_migrations table", zap.Error(err))
	}

	files, err := filepath.Glob("migrations/*.sql")
	if err != nil || len(files) == 0 {
		log.Fatal("no migration files found in migrations/")
	}
	sort.Strings(files)

	applied, err := appliedMigrations(db)
	if err != nil {
		log.Fatal("failed to query applied migrations", zap.Error(err))
	}

	pending := []string{}
	for _, f := range files {
		name := filepath.Base(f)
		if !applied[name] {
			pending = append(pending, f)
		}
	}

	if len(pending) == 0 {
		log.Info("all migrations are up to date")
		return
	}

	for _, f := range pending {
		name := filepath.Base(f)
		log.Info("migration pending", zap.String("file", name))
		if *dryRun {
			continue
		}
		if err := applyMigration(db, f, name); err != nil {
			log.Fatal("migration failed", zap.String("file", name), zap.Error(err))
		}
		log.Info("migration applied", zap.String("file", name))
	}

	if *dryRun {
		fmt.Printf("\n%d migration(s) would be applied (dry run)\n", len(pending))
	} else {
		log.Info("all migrations applied", zap.Int("count", len(pending)))
	}
}

func ensureMigrationsTable(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			filename   TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`)
	return err
}

func appliedMigrations(db *sql.DB) (map[string]bool, error) {
	rows, err := db.Query(`SELECT filename FROM schema_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		applied[name] = true
	}
	return applied, rows.Err()
}

func applyMigration(db *sql.DB, path, name string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}

	if _, err := tx.Exec(string(content)); err != nil {
		_ = tx.Rollback()
		return err
	}

	if _, err := tx.Exec(`INSERT INTO schema_migrations (filename) VALUES ($1)`, name); err != nil {
		_ = tx.Rollback()
		return err
	}

	return tx.Commit()
}
