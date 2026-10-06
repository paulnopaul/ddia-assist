-- M4: spaced review of weak concepts (docs/spec/05-review-queue.md).

-- +goose Up
CREATE TABLE review_item (
    concept_id    INTEGER PRIMARY KEY REFERENCES concept(id) ON DELETE CASCADE,
    due_at        TEXT NOT NULL,
    interval_days INTEGER NOT NULL,
    streak        INTEGER NOT NULL DEFAULT 0,
    last_score    INTEGER NOT NULL,
    mastered      INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX review_item_due ON review_item(mastered, due_at);

-- +goose Down
DROP TABLE review_item;
