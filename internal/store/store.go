// Package store owns the SQLite database (docs/spec/02-data-model.md).
package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var embedded embed.FS

// Store wraps the database shared by `ddia serve` and `ddia mcp`.
type Store struct {
	DB         *sql.DB
	migrations *goose.Provider
}

// Open opens (or creates) the database at path in WAL mode and applies
// pending goose migrations from migrations/. WAL lets the stdio MCP process
// and the web server use the same file at once (00-overview.md, Architecture).
func Open(ctx context.Context, path string) (*Store, error) {
	dsn := "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	s, err := newStore(ctx, db)
	if err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func newStore(ctx context.Context, db *sql.DB) (*Store, error) {
	if err := db.PingContext(ctx); err != nil {
		return nil, err
	}
	fsys, err := fs.Sub(embedded, "migrations")
	if err != nil {
		return nil, err
	}
	p, err := goose.NewProvider(goose.DialectSQLite3, db, fsys)
	if err != nil {
		return nil, fmt.Errorf("migrations: %w", err)
	}
	if _, err := p.Up(ctx); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &Store{DB: db, migrations: p}, nil
}

func (s *Store) Close() error { return s.DB.Close() }

// SchemaVersion returns the version of the latest applied migration.
func (s *Store) SchemaVersion(ctx context.Context) (int64, error) {
	return s.migrations.GetDBVersion(ctx)
}
