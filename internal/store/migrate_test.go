package store

import (
	"context"
	"database/sql"
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
)

// TestMigrate_DAT6_KeepsSingleBookData upgrades a database written before
// multi-book support and checks that its book, progress, concepts, figures
// and search index survive as the active book 1.
func TestMigrate_DAT6_KeepsSingleBookData(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "ddia.db")
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatal(err)
	}
	fsys, _ := fs.Sub(embedded, "migrations")
	p, err := goose.NewProvider(goose.DialectSQLite3, db, fsys)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.UpTo(ctx, 4); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO book(id, title, sha256) VALUES (1, 'Old Book', 'abc')`,
		`INSERT INTO section(id, book_id, chapter, level, ord, heading, markdown, words) VALUES
			('ch01', 1, 1, 0, 0, 'Chapter 1', 'intro text', 2), ('ch01.s01', 1, 1, 1, 1, 'Logs', 'append only quorumword', 3)`,
		`INSERT INTO section_fts(section_id, heading, markdown) VALUES ('ch01', 'Chapter 1', 'intro text'), ('ch01.s01', 'Logs', 'append only quorumword')`,
		`INSERT INTO figure(book_id, section_id, name, path, caption) VALUES (1, 'ch01.s01', 'f.png', '/data/figures/f.png', 'A log')`,
		`INSERT INTO unit(id, book_id, ord, chapter, title, words) VALUES (7, 1, 0, 1, 'Logs', 5)`,
		`INSERT INTO unit_section(unit_id, section_id, ord) VALUES (7, 'ch01', 0), (7, 'ch01.s01', 1)`,
		`INSERT INTO progress(unit_id, status, read_at) VALUES (7, 'read', datetime('now'))`,
		`INSERT INTO concept(id, unit_id, name, definition, section_ref) VALUES (3, 7, 'Log', 'd', 'ch01.s01')`,
	} {
		if _, err := db.ExecContext(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	s, err := newStore(ctx, db)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	defer s.Close()

	b, err := s.ActiveBook(ctx)
	if err != nil || b.ID != 1 || b.Title != "Old Book" {
		t.Fatalf("DAT-7: active book = %+v, %v", b, err)
	}
	units, err := s.Units(ctx, 1)
	if err != nil || len(units) != 1 || units[0].ID != 7 || len(units[0].Sections) != 2 || units[0].Status != StatusRead {
		t.Fatalf("units = %+v, %v", units, err)
	}
	if sc, err := s.Section(ctx, 1, "ch01.s01"); err != nil || sc.Heading != "Logs" {
		t.Fatalf("section = %+v, %v", sc, err)
	}
	if f, err := s.FigureInScope(ctx, 1, "f.png"); err != nil || f.Path != "/data/figures/f.png" {
		t.Fatalf("figure = %+v, %v", f, err)
	}
	if hits, err := s.Search(ctx, 1, "quorumword", 5); err != nil || len(hits) != 1 || hits[0].UnitID != 7 {
		t.Fatalf("search = %+v, %v", hits, err)
	}
	if cs, err := s.Concepts(ctx, 7); err != nil || len(cs) != 1 {
		t.Fatalf("concepts = %+v, %v", cs, err)
	}
}
