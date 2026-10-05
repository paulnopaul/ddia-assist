package store

import (
	"context"
	"path/filepath"
	"testing"
)

func open(t *testing.T) *Store {
	t.Helper()
	s, err := Open(context.Background(), filepath.Join(t.TempDir(), "ddia.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestOpen_MigratesAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "ddia.db")
	for i := 0; i < 2; i++ {
		s, err := Open(ctx, path)
		if err != nil {
			t.Fatalf("open #%d: %v", i, err)
		}
		v, err := s.SchemaVersion(ctx)
		if err != nil || v != 1 {
			t.Fatalf("open #%d: schema version = %d, %v; want 1", i, v, err)
		}
		s.Close()
	}
}

func TestOpen_UsesWAL(t *testing.T) {
	var mode string
	if err := open(t).DB.QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if mode != "wal" {
		t.Fatalf("journal_mode = %q, want wal", mode)
	}
}

// DEP-1 requires FTS5 in the cgo-free driver; DAT-1 depends on it.
func TestDriver_DEP1_HasFTS5(t *testing.T) {
	db := open(t).DB
	if _, err := db.Exec(`CREATE VIRTUAL TABLE t USING fts5(body)`); err != nil {
		t.Fatalf("fts5 unavailable: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO t(body) VALUES ('leaderless replication with sloppy quorums')`); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM t WHERE t MATCH 'quorum*'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("match count = %d, %v; want 1", n, err)
	}
}
