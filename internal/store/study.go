package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

type Concept struct {
	ID         int64  `json:"concept_id"`
	UnitID     int64  `json:"unit_id"`
	Name       string `json:"name"`
	Definition string `json:"definition"`
	SectionRef string `json:"section_ref"`
	Score      *int   `json:"score,omitempty"` // current score (DAT-2); nil if never scored
	ScoreID    int64  `json:"-"`
	UnitTitle  string `json:"-"`
}

type ConceptIn struct {
	Name       string `json:"name"`
	Definition string `json:"definition"`
	SectionRef string `json:"section_ref"`
}

// ValidationError is returned for bad input that the caller can fix and retry (MCP-3).
type ValidationError struct{ Problems []string }

func (e *ValidationError) Error() string {
	return "fix and retry: " + strings.Join(e.Problems, "; ")
}

const conceptCols = `cc.concept_id, cc.unit_id, cc.name, cc.definition, cc.section_ref, cc.score, COALESCE(cc.score_id, 0), u.title`

func scanConcepts(rows *sql.Rows) ([]Concept, error) {
	defer rows.Close()
	var out []Concept
	for rows.Next() {
		var c Concept
		var score sql.NullInt64
		if err := rows.Scan(&c.ID, &c.UnitID, &c.Name, &c.Definition, &c.SectionRef, &score, &c.ScoreID, &c.UnitTitle); err != nil {
			return nil, err
		}
		if score.Valid {
			v := int(score.Int64)
			c.Score = &v
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Concepts returns a unit's concepts with their current scores.
func (s *Store) Concepts(ctx context.Context, unitID int64) ([]Concept, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+conceptCols+` FROM concept_current cc JOIN unit u ON u.id = cc.unit_id WHERE cc.unit_id = ? ORDER BY cc.concept_id`, unitID)
	if err != nil {
		return nil, err
	}
	return scanConcepts(rows)
}

// ConceptsInScope returns concepts of read or studied units, weakest first.
func (s *Store) ConceptsInScope(ctx context.Context) ([]Concept, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+conceptCols+` FROM concept_current cc
		JOIN unit u ON u.id = cc.unit_id JOIN progress p ON p.unit_id = u.id
		WHERE p.status IN ('read', 'studied')
		ORDER BY cc.score IS NULL, cc.score, u.ord, cc.concept_id`)
	if err != nil {
		return nil, err
	}
	return scanConcepts(rows)
}

// WeakConcepts returns in-scope concepts whose current score is at most 1.
func (s *Store) WeakConcepts(ctx context.Context, limit int) ([]Concept, error) {
	all, err := s.ConceptsInScope(ctx)
	if err != nil {
		return nil, err
	}
	var out []Concept
	for _, c := range all {
		if c.Score != nil && *c.Score <= 1 && len(out) < limit {
			out = append(out, c)
		}
	}
	return out, nil
}

// SaveConcepts stores a unit's key concepts the first time it is studied.
func (s *Store) SaveConcepts(ctx context.Context, unitID int64, in []ConceptIn) ([]Concept, error) {
	u, err := s.Unit(ctx, unitID, false)
	if err != nil {
		return nil, err
	}
	if !u.InScope() {
		return nil, ErrOutOfScope
	}
	existing, err := s.Concepts(ctx, unitID)
	if err != nil {
		return nil, err
	}
	if len(existing) > 0 {
		return nil, &ValidationError{[]string{fmt.Sprintf("unit %d already has %d concepts; use those (see get_unit)", unitID, len(existing))}}
	}
	var problems []string
	if len(in) < 3 || len(in) > 12 {
		problems = append(problems, fmt.Sprintf("give 3-12 concepts, got %d", len(in)))
	}
	unitSections := map[string]bool{}
	for _, sc := range u.Sections {
		unitSections[sc.ID] = true
	}
	seen := map[string]bool{}
	for i, c := range in {
		if strings.TrimSpace(c.Name) == "" || strings.TrimSpace(c.Definition) == "" {
			problems = append(problems, fmt.Sprintf("concept %d needs a name and a definition", i))
		}
		if !unitSections[c.SectionRef] {
			problems = append(problems, fmt.Sprintf("concept %q: section_ref %q is not a section of unit %d", c.Name, c.SectionRef, unitID))
		}
		if seen[strings.ToLower(c.Name)] {
			problems = append(problems, fmt.Sprintf("duplicate concept %q", c.Name))
		}
		seen[strings.ToLower(c.Name)] = true
	}
	if len(problems) > 0 {
		return nil, &ValidationError{problems}
	}
	err = s.withTx(ctx, func(tx *sql.Tx) error {
		for _, c := range in {
			if _, err := tx.ExecContext(ctx, `INSERT INTO concept(unit_id, name, definition, section_ref) VALUES (?, ?, ?, ?)`,
				unitID, strings.TrimSpace(c.Name), strings.TrimSpace(c.Definition), c.SectionRef); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.Concepts(ctx, unitID)
}

type AnswerIn struct {
	Step      string `json:"step" jsonschema:"explain, probe or apply"`
	TaskShape string `json:"task_shape,omitempty" jsonschema:"for apply: predict_outcome, spot_the_flaw, choose_and_justify or explain_failure"`
	Prompt    string `json:"prompt" jsonschema:"the question or task as asked"`
	Response  string `json:"response" jsonschema:"the user's answer, verbatim"`
	Feedback  string `json:"feedback,omitempty"`
	Score     *int   `json:"score,omitempty" jsonschema:"0-3, for probe and apply answers"`
}

type ScoreIn struct {
	ConceptID int64  `json:"concept_id"`
	Score     int    `json:"score" jsonschema:"0 missing, 1 shaky, 2 solid, 3 could teach it"`
	Note      string `json:"note,omitempty"`
}

type GapIn struct {
	ConceptID   int64  `json:"concept_id,omitempty"`
	Description string `json:"description"`
	SectionRef  string `json:"section_ref" jsonschema:"section ID to reread, e.g. ch06.s04.s02"`
}

type Assessment struct {
	UnitID          int64      `json:"unit_id"`
	ClientSessionID string     `json:"client_session_id,omitempty" jsonschema:"optional idempotency key; repeating it returns the earlier result"`
	Answers         []AnswerIn `json:"answers"`
	ConceptScores   []ScoreIn  `json:"concept_scores"`
	Gaps            []GapIn    `json:"gaps"`
}

var TaskShapes = map[string]bool{"predict_outcome": true, "spot_the_flaw": true, "choose_and_justify": true, "explain_failure": true}

// RecordAssessment validates and stores a study session, marks the unit
// studied and lets the review queue react to the scores (MCP-3, MCP-4, SES-10, SES-12).
// It returns the session ID and whether it already existed (MCP-5).
func (s *Store) RecordAssessment(ctx context.Context, a Assessment) (int64, bool, error) {
	if a.ClientSessionID != "" {
		var id int64
		err := s.DB.QueryRowContext(ctx, `SELECT id FROM session WHERE client_session_id = ?`, a.ClientSessionID).Scan(&id)
		if err == nil {
			return id, true, nil
		}
	}
	u, err := s.Unit(ctx, a.UnitID, false)
	if err != nil {
		return 0, false, err
	}
	if !u.InScope() {
		return 0, false, ErrOutOfScope
	}
	var problems []string
	explains, applies := 0, 0
	for i, an := range a.Answers {
		switch an.Step {
		case "explain":
			explains++
		case "probe":
		case "apply":
			applies++
			if !TaskShapes[an.TaskShape] {
				problems = append(problems, fmt.Sprintf("answer %d: task_shape must be one of predict_outcome, spot_the_flaw, choose_and_justify, explain_failure", i))
			}
		default:
			problems = append(problems, fmt.Sprintf("answer %d: step must be explain, probe or apply", i))
		}
		if an.Score != nil && (*an.Score < 0 || *an.Score > 3) {
			problems = append(problems, fmt.Sprintf("answer %d: score must be 0-3", i))
		}
		if strings.TrimSpace(an.Prompt) == "" {
			problems = append(problems, fmt.Sprintf("answer %d: prompt is empty", i))
		}
	}
	if explains == 0 {
		problems = append(problems, "an explain answer is required")
	}
	if applies > 1 {
		problems = append(problems, "at most one apply answer")
	}

	unitConcepts, err := s.Concepts(ctx, a.UnitID)
	if err != nil {
		return 0, false, err
	}
	if len(unitConcepts) == 0 {
		problems = append(problems, "this unit has no concepts yet; call save_concepts first")
	}
	allowed, err := s.allowedConcepts(ctx, u.Ord)
	if err != nil {
		return 0, false, err
	}
	scored := map[int64]bool{}
	for _, sc := range a.ConceptScores {
		if sc.Score < 0 || sc.Score > 3 {
			problems = append(problems, fmt.Sprintf("concept %d: score must be 0-3", sc.ConceptID))
		}
		if !allowed[sc.ConceptID] {
			problems = append(problems, fmt.Sprintf("concept %d doesn't belong to this unit or an earlier read unit", sc.ConceptID))
		}
		scored[sc.ConceptID] = true
	}
	for _, c := range unitConcepts {
		if !scored[c.ID] {
			problems = append(problems, fmt.Sprintf("concept %d (%s) needs a score (SES-10)", c.ID, c.Name))
		}
	}
	for i, g := range a.Gaps {
		if strings.TrimSpace(g.Description) == "" {
			problems = append(problems, fmt.Sprintf("gap %d: description is empty", i))
		}
		if g.SectionRef == "" {
			problems = append(problems, fmt.Sprintf("gap %d: section_ref is required (MCP-4)", i))
		} else if ok, err := s.SectionInScope(ctx, g.SectionRef); err != nil {
			return 0, false, err
		} else if !ok {
			problems = append(problems, fmt.Sprintf("gap %d: section_ref %q is not a section the user has read", i, g.SectionRef))
		}
		if g.ConceptID != 0 && !allowed[g.ConceptID] {
			problems = append(problems, fmt.Sprintf("gap %d: concept %d is not in scope", i, g.ConceptID))
		}
	}
	if len(problems) > 0 {
		return 0, false, &ValidationError{problems}
	}

	var sessionID int64
	err = s.withTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `INSERT INTO session(kind, unit_id, client_session_id, finished_at) VALUES ('study', ?, ?, datetime('now'))`,
			a.UnitID, nullable(a.ClientSessionID))
		if err != nil {
			return err
		}
		sessionID, _ = res.LastInsertId()
		for i, an := range a.Answers {
			if _, err := tx.ExecContext(ctx, `INSERT INTO answer(session_id, ord, step, task_shape, prompt, response, feedback, score) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
				sessionID, i, an.Step, nullable(an.TaskShape), an.Prompt, an.Response, an.Feedback, an.Score); err != nil {
				return err
			}
		}
		for _, sc := range a.ConceptScores {
			if _, err := tx.ExecContext(ctx, `INSERT INTO concept_score(session_id, concept_id, score, note) VALUES (?, ?, ?, ?)`,
				sessionID, sc.ConceptID, sc.Score, sc.Note); err != nil {
				return err
			}
			if err := s.afterScore(ctx, tx, sc.ConceptID, sc.Score, false); err != nil {
				return err
			}
		}
		for _, g := range a.Gaps {
			var cid any
			if g.ConceptID != 0 {
				cid = g.ConceptID
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO gap(session_id, concept_id, description, section_ref) VALUES (?, ?, ?, ?)`,
				sessionID, cid, g.Description, g.SectionRef); err != nil {
				return err
			}
		}
		_, err = tx.ExecContext(ctx, `UPDATE progress SET status = 'studied', studied_at = datetime('now'), read_at = COALESCE(read_at, datetime('now')) WHERE unit_id = ?`, a.UnitID)
		return err
	})
	return sessionID, false, err
}

// allowedConcepts are concepts of read/studied units up to and including the unit at ord.
func (s *Store) allowedConcepts(ctx context.Context, ord int) (map[int64]bool, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT c.id FROM concept c JOIN unit u ON u.id = c.unit_id JOIN progress p ON p.unit_id = u.id
		WHERE u.ord <= ? AND p.status IN ('read', 'studied')`, ord)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		m[id] = true
	}
	return m, rows.Err()
}

type SessionView struct {
	ID         int64
	Kind       string
	UnitID     int64
	FinishedAt string
	Answers    []AnswerView
	Scores     []ScoreView
	Gaps       []GapView
}

type AnswerView struct {
	Step, TaskShape, Prompt, Response, Feedback, ConceptName string
	Score                                                    *int
}

type ScoreView struct {
	ID          int64
	ConceptID   int64
	ConceptName string
	Score       int
	Override    *int
	Note        string
}

// Effective is the score that counts (DAT-2).
func (v ScoreView) Effective() int {
	if v.Override != nil {
		return *v.Override
	}
	return v.Score
}

type GapView struct {
	ConceptName, Description, SectionRef string
}

// Sessions returns a unit's study sessions, newest first, with everything recorded.
func (s *Store) Sessions(ctx context.Context, unitID int64) ([]SessionView, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, kind, COALESCE(unit_id, 0), COALESCE(finished_at, started_at) FROM session WHERE unit_id = ? ORDER BY id DESC`, unitID)
	if err != nil {
		return nil, err
	}
	var out []SessionView
	for rows.Next() {
		var v SessionView
		if err := rows.Scan(&v.ID, &v.Kind, &v.UnitID, &v.FinishedAt); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, v)
	}
	rows.Close()
	for i := range out {
		if err := s.fillSession(ctx, &out[i]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s *Store) fillSession(ctx context.Context, v *SessionView) error {
	rows, err := s.DB.QueryContext(ctx, `SELECT a.step, COALESCE(a.task_shape, ''), a.prompt, a.response, a.feedback, a.score, COALESCE(c.name, '')
		FROM answer a LEFT JOIN concept c ON c.id = a.concept_id WHERE a.session_id = ? ORDER BY a.ord`, v.ID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var a AnswerView
		var score sql.NullInt64
		if err := rows.Scan(&a.Step, &a.TaskShape, &a.Prompt, &a.Response, &a.Feedback, &score, &a.ConceptName); err != nil {
			rows.Close()
			return err
		}
		if score.Valid {
			n := int(score.Int64)
			a.Score = &n
		}
		v.Answers = append(v.Answers, a)
	}
	rows.Close()
	rows, err = s.DB.QueryContext(ctx, `SELECT cs.id, cs.concept_id, c.name, cs.score, cs.user_override, cs.note
		FROM concept_score cs JOIN concept c ON c.id = cs.concept_id WHERE cs.session_id = ? ORDER BY cs.id`, v.ID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var sv ScoreView
		var ov sql.NullInt64
		if err := rows.Scan(&sv.ID, &sv.ConceptID, &sv.ConceptName, &sv.Score, &ov, &sv.Note); err != nil {
			rows.Close()
			return err
		}
		if ov.Valid {
			n := int(ov.Int64)
			sv.Override = &n
		}
		v.Scores = append(v.Scores, sv)
	}
	rows.Close()
	rows, err = s.DB.QueryContext(ctx, `SELECT COALESCE(c.name, ''), g.description, g.section_ref FROM gap g LEFT JOIN concept c ON c.id = g.concept_id WHERE g.session_id = ? ORDER BY g.id`, v.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var g GapView
		if err := rows.Scan(&g.ConceptName, &g.Description, &g.SectionRef); err != nil {
			return err
		}
		v.Gaps = append(v.Gaps, g)
	}
	return rows.Err()
}

// OverrideScore sets the user's own score on a recorded concept score (SES-11).
func (s *Store) OverrideScore(ctx context.Context, scoreID int64, score int) error {
	if score < 0 || score > 3 {
		return errors.New("score must be 0-3")
	}
	return s.withTx(ctx, func(tx *sql.Tx) error {
		var conceptID int64
		if err := tx.QueryRowContext(ctx, `SELECT concept_id FROM concept_score WHERE id = ?`, scoreID).Scan(&conceptID); err != nil {
			return ErrNotFound
		}
		if _, err := tx.ExecContext(ctx, `UPDATE concept_score SET user_override = ? WHERE id = ?`, score, scoreID); err != nil {
			return err
		}
		var latest int64
		if err := tx.QueryRowContext(ctx, `SELECT MAX(id) FROM concept_score WHERE concept_id = ?`, conceptID).Scan(&latest); err != nil {
			return err
		}
		if latest != scoreID {
			return nil // an older score: history only, the current score is unchanged
		}
		return s.afterOverride(ctx, tx, conceptID, score)
	})
}

// UnitScores returns the average current concept score per unit.
func (s *Store) UnitScores(ctx context.Context) (map[int64]float64, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT unit_id, AVG(score) FROM concept_current WHERE score IS NOT NULL GROUP BY unit_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[int64]float64{}
	for rows.Next() {
		var id int64
		var avg float64
		if err := rows.Scan(&id, &avg); err != nil {
			return nil, err
		}
		m[id] = avg
	}
	return m, rows.Err()
}
