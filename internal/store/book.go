package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/paulnopaul/ddia-assist/internal/epub"
	"github.com/paulnopaul/ddia-assist/internal/plan"
)

// Unit statuses (DAT-5: read and studied are in read scope).
const (
	StatusNotStarted = "not_started"
	StatusReading    = "reading"
	StatusRead       = "read"
	StatusStudied    = "studied"
)

var ErrNotFound = errors.New("not found")

type Book struct {
	ID         int64
	Title      string
	SHA256     string
	ImportedAt string
}

type Section struct {
	ID          string
	ParentID    string
	Chapter     int
	Level       int
	Heading     string
	Markdown    string
	Words       int
	IsReference bool
}

type Unit struct {
	ID       int64
	Ord      int
	Chapter  int
	Title    string
	Words    int
	Manual   bool
	Status   string
	ReadAt   string
	Sections []Section // headings only unless loaded with text
}

// InScope reports whether the unit is in read scope (DAT-5).
func (u Unit) InScope() bool { return u.Status == StatusRead || u.Status == StatusStudied }

type Figure struct {
	Name      string
	SectionID string
	Path      string
	Caption   string
}

// CurrentBook returns the imported book, or ErrNotFound.
func (s *Store) CurrentBook(ctx context.Context) (*Book, error) {
	var b Book
	err := s.DB.QueryRowContext(ctx, `SELECT id, title, sha256, imported_at FROM book ORDER BY id LIMIT 1`).
		Scan(&b.ID, &b.Title, &b.SHA256, &b.ImportedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &b, err
}

// SaveBook stores a parsed book, its figures and its unit plan in one transaction.
func (s *Store) SaveBook(ctx context.Context, title, sha string, secs []epub.Section, figs []Figure, units []plan.Unit) (int64, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `INSERT INTO book(title, sha256) VALUES (?, ?)`, title, sha)
	if err != nil {
		return 0, err
	}
	bookID, _ := res.LastInsertId()
	for i, sc := range secs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO section(id, book_id, parent_id, chapter, level, ord, heading, markdown, words, is_reference)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			sc.ID, bookID, nullable(sc.ParentID), sc.Chapter, sc.Level, i, sc.Heading, sc.Markdown, sc.Words, sc.IsReference); err != nil {
			return 0, fmt.Errorf("section %s: %w", sc.ID, err)
		}
		if !sc.IsReference {
			if _, err := tx.ExecContext(ctx, `INSERT INTO section_fts(section_id, heading, markdown) VALUES (?, ?, ?)`, sc.ID, sc.Heading, sc.Markdown); err != nil {
				return 0, err
			}
		}
	}
	for _, f := range figs {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO figure(book_id, section_id, name, path, caption) VALUES (?, ?, ?, ?, ?)`,
			bookID, f.SectionID, f.Name, f.Path, f.Caption); err != nil {
			return 0, fmt.Errorf("figure %s: %w", f.Name, err)
		}
	}
	for i, u := range units {
		if err := insertUnit(ctx, tx, bookID, i, u.Chapter, u.Title, u.Words, false, u.SectionIDs); err != nil {
			return 0, err
		}
	}
	return bookID, tx.Commit()
}

func insertUnit(ctx context.Context, tx *sql.Tx, bookID int64, ord, chapter int, title string, words int, manual bool, ids []string) error {
	res, err := tx.ExecContext(ctx, `INSERT INTO unit(book_id, ord, chapter, title, words, manual) VALUES (?, ?, ?, ?, ?, ?)`,
		bookID, ord, chapter, title, words, manual)
	if err != nil {
		return err
	}
	unitID, _ := res.LastInsertId()
	for j, id := range ids {
		if _, err := tx.ExecContext(ctx, `INSERT INTO unit_section(unit_id, section_id, ord) VALUES (?, ?, ?)`, unitID, id, j); err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO progress(unit_id) VALUES (?)`, unitID)
	return err
}

// DeleteBook removes the book and everything that hangs off it (DAT-4).
func (s *Store) DeleteBook(ctx context.Context, id int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM section_fts WHERE section_id IN (SELECT id FROM section WHERE book_id = ?)`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM book WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

const unitCols = `u.id, u.ord, u.chapter, u.title, u.words, u.manual, p.status, COALESCE(p.read_at, '')`

func scanUnit(sc interface{ Scan(...any) error }) (Unit, error) {
	var u Unit
	err := sc.Scan(&u.ID, &u.Ord, &u.Chapter, &u.Title, &u.Words, &u.Manual, &u.Status, &u.ReadAt)
	return u, err
}

// Units lists all units in reading order, with section headings.
func (s *Store) Units(ctx context.Context) ([]Unit, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+unitCols+` FROM unit u JOIN progress p ON p.unit_id = u.id ORDER BY u.ord`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var units []Unit
	idx := map[int64]int{}
	for rows.Next() {
		u, err := scanUnit(rows)
		if err != nil {
			return nil, err
		}
		idx[u.ID] = len(units)
		units = append(units, u)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	srows, err := s.DB.QueryContext(ctx, `SELECT us.unit_id, s.id, COALESCE(s.parent_id, ''), s.chapter, s.level, s.heading, s.words
		FROM unit_section us JOIN section s ON s.id = us.section_id ORDER BY us.unit_id, us.ord`)
	if err != nil {
		return nil, err
	}
	defer srows.Close()
	for srows.Next() {
		var uid int64
		var sc Section
		if err := srows.Scan(&uid, &sc.ID, &sc.ParentID, &sc.Chapter, &sc.Level, &sc.Heading, &sc.Words); err != nil {
			return nil, err
		}
		if i, ok := idx[uid]; ok {
			units[i].Sections = append(units[i].Sections, sc)
		}
	}
	return units, srows.Err()
}

// Unit loads one unit; withText also loads section Markdown.
func (s *Store) Unit(ctx context.Context, id int64, withText bool) (*Unit, error) {
	u, err := scanUnit(s.DB.QueryRowContext(ctx, `SELECT `+unitCols+` FROM unit u JOIN progress p ON p.unit_id = u.id WHERE u.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	text := `''`
	if withText {
		text = `s.markdown`
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT s.id, COALESCE(s.parent_id, ''), s.chapter, s.level, s.heading, `+text+`, s.words
		FROM unit_section us JOIN section s ON s.id = us.section_id WHERE us.unit_id = ? ORDER BY us.ord`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var sc Section
		if err := rows.Scan(&sc.ID, &sc.ParentID, &sc.Chapter, &sc.Level, &sc.Heading, &sc.Markdown, &sc.Words); err != nil {
			return nil, err
		}
		u.Sections = append(u.Sections, sc)
	}
	return &u, rows.Err()
}

// SetStatus changes a unit's progress status and stamps read/studied times.
func (s *Store) SetStatus(ctx context.Context, unitID int64, status string) error {
	q := `UPDATE progress SET status = ? WHERE unit_id = ?`
	switch status {
	case StatusNotStarted, StatusReading:
		// Unreading takes the unit out of read scope again (UI-3, DAT-5).
		q = `UPDATE progress SET status = ?, read_at = NULL, studied_at = NULL WHERE unit_id = ?`
	case StatusRead:
		q = `UPDATE progress SET status = ?, read_at = COALESCE(read_at, datetime('now')) WHERE unit_id = ?`
	case StatusStudied:
		q = `UPDATE progress SET status = ?, read_at = COALESCE(read_at, datetime('now')), studied_at = datetime('now') WHERE unit_id = ?`
	}
	res, err := s.DB.ExecContext(ctx, q, status, unitID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Rename sets a unit's title (ING-12).
func (s *Store) Rename(ctx context.Context, unitID int64, title string) error {
	title = strings.TrimSpace(title)
	if title == "" {
		return errors.New("title must not be empty")
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE unit SET title = ?, manual = 1 WHERE id = ?`, title, unitID)
	return err
}

// MergeWithNext merges a unit with the following unit of the same chapter (ING-11).
func (s *Store) MergeWithNext(ctx context.Context, unitID int64) error {
	return s.withTx(ctx, func(tx *sql.Tx) error {
		var ord, chapter int
		var bookID int64
		if err := tx.QueryRowContext(ctx, `SELECT ord, chapter, book_id FROM unit WHERE id = ?`, unitID).Scan(&ord, &chapter, &bookID); err != nil {
			return ErrNotFound
		}
		var nextID int64
		var nextChapter, nextWords int
		var nextTitle string
		err := tx.QueryRowContext(ctx, `SELECT id, chapter, words, title FROM unit WHERE book_id = ? AND ord > ? ORDER BY ord LIMIT 1`, bookID, ord).
			Scan(&nextID, &nextChapter, &nextWords, &nextTitle)
		if err != nil || nextChapter != chapter {
			return errors.New("no following unit in the same chapter")
		}
		var first string
		if err := tx.QueryRowContext(ctx, `SELECT title FROM unit WHERE id = ?`, unitID).Scan(&first); err != nil {
			return err
		}
		title := strings.SplitN(first, " – ", 2)[0] + " – " + lastPart(nextTitle)
		if _, err := tx.ExecContext(ctx, `UPDATE unit_section SET unit_id = ?, ord = ord + 10000 WHERE unit_id = ?`, unitID, nextID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE unit SET words = words + ?, title = ?, manual = 1 WHERE id = ?`, nextWords, title, unitID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM unit WHERE id = ?`, nextID); err != nil {
			return err
		}
		return renumber(ctx, tx, unitID)
	})
}

// SplitAt splits a unit so that sectionID starts a new unit (ING-11).
func (s *Store) SplitAt(ctx context.Context, unitID int64, sectionID string) error {
	return s.withTx(ctx, func(tx *sql.Tx) error {
		var splitOrd int
		if err := tx.QueryRowContext(ctx, `SELECT ord FROM unit_section WHERE unit_id = ? AND section_id = ?`, unitID, sectionID).Scan(&splitOrd); err != nil {
			return errors.New("section is not in this unit")
		}
		var minOrd int
		if err := tx.QueryRowContext(ctx, `SELECT MIN(ord) FROM unit_section WHERE unit_id = ?`, unitID).Scan(&minOrd); err != nil {
			return err
		}
		if splitOrd == minOrd {
			return errors.New("cannot split at the first section")
		}
		var bookID int64
		var ord, chapter int
		if err := tx.QueryRowContext(ctx, `SELECT book_id, ord, chapter FROM unit WHERE id = ?`, unitID).Scan(&bookID, &ord, &chapter); err != nil {
			return err
		}
		var heading string
		var moved int
		if err := tx.QueryRowContext(ctx, `SELECT s.heading, (SELECT COALESCE(SUM(s2.words), 0) FROM unit_section us2 JOIN section s2 ON s2.id = us2.section_id WHERE us2.unit_id = ? AND us2.ord >= ?)
			FROM section s WHERE s.id = ?`, unitID, splitOrd, sectionID).Scan(&heading, &moved); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE unit SET ord = ord + 1 WHERE book_id = ? AND ord > ?`, bookID, ord); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO unit(book_id, ord, chapter, title, words, manual) VALUES (?, ?, ?, ?, ?, 1)`, bookID, ord+1, chapter, heading, moved)
		if err != nil {
			return err
		}
		newID, _ := res.LastInsertId()
		if _, err := tx.ExecContext(ctx, `UPDATE unit_section SET unit_id = ? WHERE unit_id = ? AND ord >= ?`, newID, unitID, splitOrd); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE unit SET words = words - ?, manual = 1 WHERE id = ?`, moved, unitID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO progress(unit_id) VALUES (?)`, newID); err != nil {
			return err
		}
		if err := renumber(ctx, tx, unitID); err != nil {
			return err
		}
		return renumber(ctx, tx, newID)
	})
}

func lastPart(t string) string {
	parts := strings.Split(t, " – ")
	return parts[len(parts)-1]
}

// renumber rewrites unit_section.ord as 0..n-1 in its current order.
func renumber(ctx context.Context, tx *sql.Tx, unitID int64) error {
	rows, err := tx.QueryContext(ctx, `SELECT section_id FROM unit_section WHERE unit_id = ? ORDER BY ord`, unitID)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	for i, id := range ids {
		if _, err := tx.ExecContext(ctx, `UPDATE unit_section SET ord = ? WHERE unit_id = ? AND section_id = ?`, i, unitID, id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) withTx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}
