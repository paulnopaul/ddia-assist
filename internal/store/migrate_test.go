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
// multi-book support and checks that its book, progress, concepts, sessions,
// scores, review queue, figures and search index survive as the active book 1.
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
		`INSERT INTO session(id, kind, unit_id, finished_at) VALUES (5, 'study', 7, datetime('now'))`,
		`INSERT INTO answer(session_id, ord, step, prompt, response) VALUES (5, 0, 'explain', 'Explain', 'my answer')`,
		`INSERT INTO concept_score(session_id, concept_id, score, user_override) VALUES (5, 3, 1, 2)`,
		`INSERT INTO gap(session_id, concept_id, description, section_ref) VALUES (5, 3, 'missed it', 'ch01.s01')`,
		`INSERT INTO review_item(concept_id, due_at, interval_days, streak, last_score) VALUES (3, datetime('now', '-1 day'), 2, 0, 1)`,
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
	if cs, err := s.Concepts(ctx, 7); err != nil || len(cs) != 1 || cs[0].Score == nil || *cs[0].Score != 2 {
		t.Fatalf("concepts and override = %+v, %v", cs, err)
	}
	sessions, err := s.Sessions(ctx, 7)
	if err != nil || len(sessions) != 1 || len(sessions[0].Answers) != 1 || sessions[0].Answers[0].Response != "my answer" || len(sessions[0].Gaps) != 1 {
		t.Fatalf("sessions = %+v, %v", sessions, err)
	}
	if due, err := s.DueReviews(ctx, 1, 5); err != nil || len(due) != 1 || due[0].LastGap != "missed it" {
		t.Fatalf("review queue = %+v, %v", due, err)
	}
}
