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

// inScopeSections lists section IDs readable by the AI client: sections of
// units marked read or studied, plus reference blocks of chapters with at
// least one such unit (DAT-5, MCP-1).
const inScopeSections = `
	SELECT us.section_id FROM unit_section us JOIN progress p ON p.unit_id = us.unit_id
	WHERE p.status IN ('read', 'studied')
	UNION
	SELECT s.id FROM section s WHERE s.is_reference = 1 AND s.chapter IN (
		SELECT u.chapter FROM unit u JOIN progress p ON p.unit_id = u.id WHERE p.status IN ('read', 'studied'))`

// Section returns one section's text if it is in read scope.
func (s *Store) Section(ctx context.Context, id string) (*Section, error) {
	var sc Section
	err := s.DB.QueryRowContext(ctx, `SELECT id, COALESCE(parent_id, ''), chapter, level, heading, markdown, words, is_reference FROM section WHERE id = ?`, id).
		Scan(&sc.ID, &sc.ParentID, &sc.Chapter, &sc.Level, &sc.Heading, &sc.Markdown, &sc.Words, &sc.IsReference)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	ok, err := s.sectionInScope(ctx, id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrOutOfScope
	}
	return &sc, nil
}

func (s *Store) sectionInScope(ctx context.Context, id string) (bool, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM (`+inScopeSections+`) WHERE section_id = ?`, id).Scan(&n)
	return n > 0, err
}

// SectionInScope reports whether id names a section in read scope.
func (s *Store) SectionInScope(ctx context.Context, id string) (bool, error) {
	return s.sectionInScope(ctx, id)
}

// FigureInScope returns a figure if its section is in read scope.
func (s *Store) FigureInScope(ctx context.Context, name string) (*Figure, error) {
	var f Figure
	err := s.DB.QueryRowContext(ctx, `SELECT name, section_id, path, caption FROM figure WHERE name = ?`, name).
		Scan(&f.Name, &f.SectionID, &f.Path, &f.Caption)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	ok, err := s.sectionInScope(ctx, f.SectionID)
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

// Search runs a full-text query over sections in read scope only (MCP-1).
func (s *Store) Search(ctx context.Context, query string, limit int) ([]SearchHit, error) {
	q := ftsQuery(query)
	if q == "" {
		return nil, errors.New("empty query")
	}
	if limit <= 0 || limit > 10 {
		limit = 10
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT f.section_id, f.heading, COALESCE((SELECT unit_id FROM unit_section WHERE section_id = f.section_id), 0),
		       snippet(section_fts, 2, '«', '»', ' … ', 24)
		FROM section_fts f
		WHERE section_fts MATCH ? AND f.section_id IN (`+inScopeSections+`)
		ORDER BY rank LIMIT ?`, q, limit)
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

// NextUnit returns the first unit with the given status in reading order.
func (s *Store) NextUnit(ctx context.Context, statuses ...string) (*Unit, error) {
	if len(statuses) == 0 {
		return nil, ErrNotFound
	}
	ph := strings.TrimSuffix(strings.Repeat("?,", len(statuses)), ",")
	args := make([]any, len(statuses))
	for i, st := range statuses {
		args[i] = st
	}
	u, err := scanUnit(s.DB.QueryRowContext(ctx, `SELECT `+unitCols+` FROM unit u JOIN progress p ON p.unit_id = u.id
		WHERE p.status IN (`+ph+`) ORDER BY u.ord LIMIT 1`, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &u, err
}

// Counts returns how many units have each status.
func (s *Store) Counts(ctx context.Context) (map[string]int, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT status, count(*) FROM progress GROUP BY status`)
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
