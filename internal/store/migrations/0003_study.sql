-- M3: concepts and study sessions (docs/spec/02-data-model.md, 04-study-session.md).

-- +goose Up
CREATE TABLE concept (
    id          INTEGER PRIMARY KEY,
    unit_id     INTEGER NOT NULL REFERENCES unit(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    definition  TEXT NOT NULL,
    section_ref TEXT NOT NULL,
    UNIQUE (unit_id, name)
);

CREATE TABLE session (
    id                INTEGER PRIMARY KEY,
    kind              TEXT NOT NULL CHECK (kind IN ('study', 'review')),
    unit_id           INTEGER REFERENCES unit(id) ON DELETE CASCADE,
    client_session_id TEXT UNIQUE,
    started_at        TEXT NOT NULL DEFAULT (datetime('now')),
    finished_at       TEXT
);

CREATE TABLE answer (
    id         INTEGER PRIMARY KEY,
    session_id INTEGER NOT NULL REFERENCES session(id) ON DELETE CASCADE,
    ord        INTEGER NOT NULL,
    step       TEXT NOT NULL CHECK (step IN ('explain', 'probe', 'apply', 'review')),
    task_shape TEXT,
    concept_id INTEGER REFERENCES concept(id) ON DELETE SET NULL,
    prompt     TEXT NOT NULL,
    response   TEXT NOT NULL,
    feedback   TEXT NOT NULL DEFAULT '',
    score      INTEGER CHECK (score BETWEEN 0 AND 3)
);

CREATE TABLE concept_score (
    id            INTEGER PRIMARY KEY,
    session_id    INTEGER NOT NULL REFERENCES session(id) ON DELETE CASCADE,
    concept_id    INTEGER NOT NULL REFERENCES concept(id) ON DELETE CASCADE,
    score         INTEGER NOT NULL CHECK (score BETWEEN 0 AND 3),
    note          TEXT NOT NULL DEFAULT '',
    user_override INTEGER CHECK (user_override BETWEEN 0 AND 3)
);
CREATE INDEX concept_score_concept ON concept_score(concept_id, id);

CREATE TABLE gap (
    id          INTEGER PRIMARY KEY,
    session_id  INTEGER NOT NULL REFERENCES session(id) ON DELETE CASCADE,
    concept_id  INTEGER REFERENCES concept(id) ON DELETE SET NULL,
    description TEXT NOT NULL,
    section_ref TEXT NOT NULL
);

-- DAT-2: a concept's current score is its latest score, override first.
CREATE VIEW concept_current AS
SELECT c.id AS concept_id, c.unit_id, c.name, c.definition, c.section_ref,
       COALESCE(cs.user_override, cs.score) AS score, cs.id AS score_id
FROM concept c
LEFT JOIN concept_score cs ON cs.id = (SELECT MAX(id) FROM concept_score WHERE concept_id = c.id);

-- +goose Down
DROP VIEW concept_current;
DROP TABLE gap;
DROP TABLE concept_score;
DROP TABLE answer;
DROP TABLE session;
DROP TABLE concept;
