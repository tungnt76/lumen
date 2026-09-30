package store

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

// Schema changes that can't be written as idempotent statements in schema.sql (which runs on
// every start) live in migrations/NNNN_name.sql. Each runs once, in its own transaction, in
// file-name order; applied versions are recorded in schema_migrations. An advisory lock keeps
// two API instances starting at once from applying the same migration twice.

//go:embed migrations/*.sql
var migrationFiles embed.FS

// migrationLock is an arbitrary constant for pg_advisory_xact_lock.
const migrationLock = 7_142_025

func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.db.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version    TEXT PRIMARY KEY,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	names, err := fs.Glob(migrationFiles, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(names)
	for _, name := range names {
		version := strings.TrimSuffix(strings.TrimPrefix(name, "migrations/"), ".sql")
		if err := s.applyMigration(ctx, name, version); err != nil {
			return fmt.Errorf("migration %s: %w", version, err)
		}
	}
	return nil
}

func (s *Store) applyMigration(ctx context.Context, name, version string) error {
	body, err := migrationFiles.ReadFile(name)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, migrationLock); err != nil {
		return err
	}
	var done bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version=$1)`, version).Scan(&done); err != nil {
		return err
	}
	if done {
		return nil
	}
	if _, err := tx.Exec(ctx, string(body)); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, version); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// AppliedMigrations lists the migrations recorded in the database, oldest first.
func (s *Store) AppliedMigrations(ctx context.Context) ([]string, error) {
	rows, err := s.db.Query(ctx, `SELECT version FROM schema_migrations ORDER BY version`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
