package store

import (
	"context"
	"database/sql"
	"fmt"
)

// Review intervals (REV-2, REV-3).
const maxIntervalDays = 60

func firstInterval(score int) int {
	switch score {
	case 0:
		return 1
	case 1:
		return 2
	default:
		return 7
	}
}

// afterScore schedules a concept after it is scored, inside the same transaction.
//
//	study score 0–2: (re)enters the queue with the REV-2 interval (REV-1, REV-4)
//	study score 3:   leaves the queue as it is
//	review score 3:  interval doubles (cap 60 days), streak + 1, mastered at 2 (REV-3, REV-4)
//	review score ≤2: interval resets per REV-2, streak 0
func (s *Store) afterScore(ctx context.Context, tx *sql.Tx, conceptID int64, score int, fromReview bool) error {
	if !fromReview {
		if score >= 3 {
			return nil
		}
		return schedule(ctx, tx, conceptID, firstInterval(score), 0, score, false)
	}
	var interval, streak int
	err := tx.QueryRowContext(ctx, `SELECT interval_days, streak FROM review_item WHERE concept_id = ?`, conceptID).Scan(&interval, &streak)
	if err == sql.ErrNoRows {
		interval, streak = 0, 0
	} else if err != nil {
		return err
	}
	if score < 3 {
		return schedule(ctx, tx, conceptID, firstInterval(score), 0, score, false)
	}
	interval = min(max(interval*2, 1), maxIntervalDays)
	streak++
	return schedule(ctx, tx, conceptID, interval, streak, score, streak >= 2)
}

// afterOverride reschedules when the user overrides a concept's current score (REV-7).
func (s *Store) afterOverride(ctx context.Context, tx *sql.Tx, conceptID int64, score int) error {
	if score >= 3 {
		_, err := tx.ExecContext(ctx, `UPDATE review_item SET mastered = 1, last_score = ? WHERE concept_id = ?`, score, conceptID)
		return err
	}
	return schedule(ctx, tx, conceptID, firstInterval(score), 0, score, false)
}

func schedule(ctx context.Context, tx *sql.Tx, conceptID int64, interval, streak, score int, mastered bool) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO review_item(concept_id, due_at, interval_days, streak, last_score, mastered)
		VALUES (?, datetime('now', ?), ?, ?, ?, ?)
		ON CONFLICT(concept_id) DO UPDATE SET due_at = excluded.due_at, interval_days = excluded.interval_days,
			streak = excluded.streak, last_score = excluded.last_score, mastered = excluded.mastered`,
		conceptID, fmt.Sprintf("+%d days", interval), interval, streak, score, mastered)
	return err
}
