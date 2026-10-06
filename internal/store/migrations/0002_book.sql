-- M1: book, sections, figures, units and progress (docs/spec/02-data-model.md).

-- +goose Up
CREATE TABLE book (
    id          INTEGER PRIMARY KEY,
    title       TEXT NOT NULL,
    sha256      TEXT NOT NULL UNIQUE,
    imported_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE section (
    id           TEXT PRIMARY KEY,
    book_id      INTEGER NOT NULL REFERENCES book(id) ON DELETE CASCADE,
    parent_id    TEXT,
    chapter      INTEGER NOT NULL,
    level        INTEGER NOT NULL,
    ord          INTEGER NOT NULL,
    heading      TEXT NOT NULL,
    markdown     TEXT NOT NULL,
    words        INTEGER NOT NULL,
    is_reference INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX section_book_ord ON section(book_id, ord);

CREATE TABLE figure (
    id         INTEGER PRIMARY KEY,
    book_id    INTEGER NOT NULL REFERENCES book(id) ON DELETE CASCADE,
    section_id TEXT NOT NULL REFERENCES section(id) ON DELETE CASCADE,
    name       TEXT NOT NULL UNIQUE,
    path       TEXT NOT NULL,
    caption    TEXT NOT NULL
);

CREATE TABLE unit (
    id      INTEGER PRIMARY KEY,
    book_id INTEGER NOT NULL REFERENCES book(id) ON DELETE CASCADE,
    ord     INTEGER NOT NULL,
    chapter INTEGER NOT NULL,
    title   TEXT NOT NULL,
    words   INTEGER NOT NULL,
    manual  INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE unit_section (
    unit_id    INTEGER NOT NULL REFERENCES unit(id) ON DELETE CASCADE,
    section_id TEXT NOT NULL REFERENCES section(id) ON DELETE CASCADE,
    ord        INTEGER NOT NULL,
    PRIMARY KEY (unit_id, section_id)
);
CREATE INDEX unit_section_section ON unit_section(section_id);

CREATE TABLE progress (
    unit_id    INTEGER PRIMARY KEY REFERENCES unit(id) ON DELETE CASCADE,
    status     TEXT NOT NULL DEFAULT 'not_started'
               CHECK (status IN ('not_started', 'reading', 'read', 'studied')),
    read_at    TEXT,
    studied_at TEXT
);

-- DAT-1: full-text index over non-reference sections.
CREATE VIRTUAL TABLE section_fts USING fts5(section_id UNINDEXED, heading, markdown);

-- +goose Down
DROP TABLE section_fts;
DROP TABLE progress;
DROP TABLE unit_section;
DROP TABLE unit;
DROP TABLE figure;
DROP TABLE section;
DROP TABLE book;
