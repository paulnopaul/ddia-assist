package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// ErrOutOfScope means the content belongs to a unit that isn't read yet (MCP-1).
var ErrOutOfScope = errors.New("not read yet")

// inScopeSections lists one book's section IDs readable by the AI client:
// sections of units marked read or studied, plus reference blocks of chapters
// with at least one such unit (DAT-5, MCP-1). It takes scopeArgs(bookID).
const inScopeSections = `
	SELECT us.section_id FROM unit_section us JOIN progress p ON p.unit_id = us.unit_id
	WHERE us.book_id = ? AND p.status IN ('read', 'studied')
	UNION
	SELECT s.id FROM section s WHERE s.book_id = ? AND s.is_reference = 1 AND s.chapter IN (
		SELECT u.chapter FROM unit u JOIN progress p ON p.unit_id = u.id WHERE u.book_id = ? AND p.status IN ('read', 'studied'))`

func scopeArgs(bookID int64, more ...any) []any {
	return append([]any{bookID, bookID, bookID}, more...)
}

// Section returns one of a book's sections if it is in read scope.
func (s *Store) Section(ctx context.Context, bookID int64, id string) (*Section, error) {
	var sc Section
	err := s.DB.QueryRowContext(ctx, `SELECT id, COALESCE(parent_id, ''), chapter, level, heading, markdown, words, is_reference FROM section WHERE book_id = ? AND id = ?`, bookID, id).
		Scan(&sc.ID, &sc.ParentID, &sc.Chapter, &sc.Level, &sc.Heading, &sc.Markdown, &sc.Words, &sc.IsReference)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	ok, err := s.SectionInScope(ctx, bookID, id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrOutOfScope
	}
	return &sc, nil
}

// SectionInScope reports whether id names a section of the book in read scope.
func (s *Store) SectionInScope(ctx context.Context, bookID int64, id string) (bool, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM (`+inScopeSections+`) WHERE section_id = ?`, scopeArgs(bookID, id)...).Scan(&n)
	return n > 0, err
}

// FigureInScope returns one of a book's figures if its section is in read scope.
func (s *Store) FigureInScope(ctx context.Context, bookID int64, name string) (*Figure, error) {
	var f Figure
	err := s.DB.QueryRowContext(ctx, `SELECT name, section_id, path, caption FROM figure WHERE book_id = ? AND name = ?`, bookID, name).
		Scan(&f.Name, &f.SectionID, &f.Path, &f.Caption)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	ok, err := s.SectionInScope(ctx, bookID, f.SectionID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrOutOfScope
	}
	return &f, nil
}

type SearchHit struct {
	SectionID string
	Heading   string
	UnitID    int64
	Snippet   string
}

// Search runs a full-text query over a book's sections in read scope only (MCP-1).
func (s *Store) Search(ctx context.Context, bookID int64, query string, limit int) ([]SearchHit, error) {
	q := ftsQuery(query)
	if q == "" {
		return nil, errors.New("empty query")
	}
	if limit <= 0 || limit > 10 {
		limit = 10
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT f.section_id, f.heading,
		       COALESCE((SELECT unit_id FROM unit_section WHERE book_id = f.book_id AND section_id = f.section_id), 0),
		       snippet(section_fts, 3, '«', '»', ' … ', 24)
		FROM section_fts f
		WHERE section_fts MATCH ? AND f.book_id = ? AND f.section_id IN (`+inScopeSections+`)
		ORDER BY rank LIMIT ?`, append([]any{q, bookID}, scopeArgs(bookID, limit)...)...)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	defer rows.Close()
	var hits []SearchHit
	for rows.Next() {
		var h SearchHit
		if err := rows.Scan(&h.SectionID, &h.Heading, &h.UnitID, &h.Snippet); err != nil {
			return nil, err
		}
		hits = append(hits, h)
	}
	return hits, rows.Err()
}

// ftsQuery turns free text into an FTS5 query of quoted terms, ANDed, so
// user punctuation can't cause syntax errors. A trailing * keeps prefix search.
func ftsQuery(in string) string {
	var terms []string
	for _, f := range strings.Fields(in) {
		prefix := strings.HasSuffix(f, "*")
		f = strings.Trim(f, `*"'`)
		f = strings.ReplaceAll(f, `"`, "")
		if f == "" {
			continue
		}
		t := `"` + f + `"`
		if prefix {
			t += "*"
		}
		terms = append(terms, t)
	}
	return strings.Join(terms, " ")
}

// NextUnit returns the book's first unit with the given status in reading order.
func (s *Store) NextUnit(ctx context.Context, bookID int64, statuses ...string) (*Unit, error) {
	if len(statuses) == 0 {
		return nil, ErrNotFound
	}
	ph := strings.TrimSuffix(strings.Repeat("?,", len(statuses)), ",")
	args := []any{bookID}
	for _, st := range statuses {
		args = append(args, st)
	}
	u, err := scanUnit(s.DB.QueryRowContext(ctx, `SELECT `+unitCols+` FROM unit u JOIN progress p ON p.unit_id = u.id
		WHERE u.book_id = ? AND p.status IN (`+ph+`) ORDER BY u.ord LIMIT 1`, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &u, err
}

// Counts returns how many of a book's units have each status.
func (s *Store) Counts(ctx context.Context, bookID int64) (map[string]int, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT p.status, count(*) FROM progress p JOIN unit u ON u.id = p.unit_id WHERE u.book_id = ? GROUP BY p.status`, bookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[string]int{}
	for rows.Next() {
		var st string
		var n int
		if err := rows.Scan(&st, &n); err != nil {
			return nil, err
		}
		m[st] = n
	}
	return m, rows.Err()
}
