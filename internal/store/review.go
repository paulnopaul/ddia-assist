package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

type DueReview struct {
	ConceptID   int64  `json:"concept_id"`
	Name        string `json:"name"`
	Definition  string `json:"definition"`
	SectionRef  string `json:"section_ref"`
	UnitTitle   string `json:"unit"`
	LastScore   int    `json:"last_score"`
	Streak      int    `json:"streak"`
	LastGap     string `json:"last_gap,omitempty"`
	LastPrompt  string `json:"last_question,omitempty"`
	DueAt       string `json:"due_at"`
	IntervalDay int    `json:"interval_days"`
}

const reviewCols = `ri.concept_id, c.name, c.definition, c.section_ref, u.title, ri.last_score, ri.streak, ri.due_at, ri.interval_days,
	COALESCE((SELECT g.description FROM gap g WHERE g.concept_id = c.id ORDER BY g.id DESC LIMIT 1), ''),
	COALESCE((SELECT a.prompt FROM answer a WHERE a.concept_id = c.id AND a.step = 'review' ORDER BY a.id DESC LIMIT 1), '')`

const reviewFrom = ` FROM review_item ri JOIN concept c ON c.id = ri.concept_id JOIN unit u ON u.id = c.unit_id
	JOIN progress p ON p.unit_id = u.id WHERE p.status IN ('read', 'studied') AND ri.mastered = 0`

func scanReviews(rows *sql.Rows) ([]DueReview, error) {
	defer rows.Close()
	out := []DueReview{}
	for rows.Next() {
		var d DueReview
		if err := rows.Scan(&d.ConceptID, &d.Name, &d.Definition, &d.SectionRef, &d.UnitTitle, &d.LastScore, &d.Streak, &d.DueAt, &d.IntervalDay, &d.LastGap, &d.LastPrompt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// DueReviews returns due items in read scope, oldest due first (REV-5, REV-6).
func (s *Store) DueReviews(ctx context.Context, limit int) ([]DueReview, error) {
	if limit <= 0 {
		limit = 5
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+reviewCols+reviewFrom+` AND ri.due_at <= datetime('now') ORDER BY ri.due_at LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	return scanReviews(rows)
}

// ReviewQueue returns every scheduled (not mastered) item in read scope.
func (s *Store) ReviewQueue(ctx context.Context) ([]DueReview, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+reviewCols+reviewFrom+` ORDER BY ri.due_at`)
	if err != nil {
		return nil, err
	}
	return scanReviews(rows)
}

// DueCount is the number of due review items in read scope.
func (s *Store) DueCount(ctx context.Context) (int, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT count(*)`+reviewFrom+` AND ri.due_at <= datetime('now')`).Scan(&n)
	return n, err
}

type ReviewState struct {
	DueAt    string
	Interval int
	Streak   int
	Mastered bool
}

// ReviewStates maps concept IDs to their queue state.
func (s *Store) ReviewStates(ctx context.Context) (map[int64]ReviewState, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT concept_id, due_at, interval_days, streak, mastered FROM review_item`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[int64]ReviewState{}
	for rows.Next() {
		var id int64
		var st ReviewState
		if err := rows.Scan(&id, &st.DueAt, &st.Interval, &st.Streak, &st.Mastered); err != nil {
			return nil, err
		}
		m[id] = st
	}
	return m, rows.Err()
}

type ReviewIn struct {
	ConceptID int64  `json:"concept_id"`
	TaskShape string `json:"task_shape,omitempty" jsonschema:"predict_outcome, spot_the_flaw, choose_and_justify or explain_failure"`
	Prompt    string `json:"prompt" jsonschema:"the review question as asked"`
	Response  string `json:"response" jsonschema:"the user's answer, verbatim"`
	Score     int    `json:"score" jsonschema:"0-3"`
	Feedback  string `json:"feedback,omitempty"`
}

// RecordReview stores one review answer and reschedules the concept (REV-3, REV-5).
func (s *Store) RecordReview(ctx context.Context, in ReviewIn) (*ReviewState, error) {
	var problems []string
	if in.Score < 0 || in.Score > 3 {
		problems = append(problems, "score must be 0-3")
	}
	if strings.TrimSpace(in.Prompt) == "" {
		problems = append(problems, "prompt is empty")
	}
	if in.TaskShape != "" && !TaskShapes[in.TaskShape] {
		problems = append(problems, "unknown task_shape")
	}
	var unitID int64
	var inScope bool
	err := s.DB.QueryRowContext(ctx, `SELECT c.unit_id, p.status IN ('read', 'studied') FROM concept c JOIN progress p ON p.unit_id = c.unit_id WHERE c.id = ?`, in.ConceptID).
		Scan(&unitID, &inScope)
	if err == sql.ErrNoRows || (err == nil && !inScope) {
		problems = append(problems, fmt.Sprintf("concept %d is not a concept from a read unit", in.ConceptID))
	} else if err != nil {
		return nil, err
	}
	var last string
	_ = s.DB.QueryRowContext(ctx, `SELECT prompt FROM answer WHERE concept_id = ? AND step = 'review' ORDER BY id DESC LIMIT 1`, in.ConceptID).Scan(&last)
	if last != "" && strings.EqualFold(strings.TrimSpace(last), strings.TrimSpace(in.Prompt)) {
		problems = append(problems, "ask a different question than last time (REV-5)")
	}
	if len(problems) > 0 {
		return nil, &ValidationError{problems}
	}
	err = s.withTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `INSERT INTO session(kind, unit_id, finished_at) VALUES ('review', ?, datetime('now'))`, unitID)
		if err != nil {
			return err
		}
		sid, _ := res.LastInsertId()
		if _, err := tx.ExecContext(ctx, `INSERT INTO answer(session_id, ord, step, task_shape, concept_id, prompt, response, feedback, score) VALUES (?, 0, 'review', ?, ?, ?, ?, ?, ?)`,
			sid, nullable(in.TaskShape), in.ConceptID, in.Prompt, in.Response, in.Feedback, in.Score); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO concept_score(session_id, concept_id, score, note) VALUES (?, ?, ?, 'review')`, sid, in.ConceptID, in.Score); err != nil {
			return err
		}
		return s.afterScore(ctx, tx, in.ConceptID, in.Score, true)
	})
	if err != nil {
		return nil, err
	}
	states, err := s.ReviewStates(ctx)
	if err != nil {
		return nil, err
	}
	st := states[in.ConceptID]
	return &st, nil
}

type ChapterProgress struct {
	Chapter              int
	Units, Read, Studied int
}

// ChapterProgress counts units per chapter by status (UI-6).
func (s *Store) ChapterProgress(ctx context.Context) ([]ChapterProgress, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT u.chapter, count(*),
		SUM(p.status IN ('read', 'studied')), SUM(p.status = 'studied')
		FROM unit u JOIN progress p ON p.unit_id = u.id GROUP BY u.chapter ORDER BY u.chapter`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ChapterProgress
	for rows.Next() {
		var c ChapterProgress
		if err := rows.Scan(&c.Chapter, &c.Units, &c.Read, &c.Studied); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
