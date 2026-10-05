package store

import (
	"context"
	"database/sql"
)

// afterScore runs inside the transaction that records a concept score.
// The review queue (M4) schedules the concept here.
func (s *Store) afterScore(ctx context.Context, tx *sql.Tx, conceptID int64, score int, fromReview bool) error {
	return nil
}

// afterOverride runs when the user overrides a concept's current score (REV-7).
func (s *Store) afterOverride(ctx context.Context, tx *sql.Tx, conceptID int64, score int) error {
	return nil
}
