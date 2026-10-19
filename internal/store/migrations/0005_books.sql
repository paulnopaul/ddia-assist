-- M5: several books (docs/spec/02-data-model.md, DAT-6, DAT-7).
-- Section IDs become unique per book, so section, figure, unit_section and
-- section_fts are rebuilt with the book ID in their keys. Children are created
-- against section_new and the old tables are dropped child-first, so the
-- foreign-key cascades never fire on the copied rows.

-- +goose Up
CREATE TABLE section_new (
    book_id      INTEGER NOT NULL REFERENCES book(id) ON DELETE CASCADE,
    id           TEXT NOT NULL,
    parent_id    TEXT,
    chapter      INTEGER NOT NULL,
    level        INTEGER NOT NULL,
    ord          INTEGER NOT NULL,
    heading      TEXT NOT NULL,
    markdown     TEXT NOT NULL,
    words        INTEGER NOT NULL,
    is_reference INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (book_id, id)
);
INSERT INTO section_new SELECT book_id, id, parent_id, chapter, level, ord, heading, markdown, words, is_reference FROM section;

CREATE TABLE figure_new (
    id         INTEGER PRIMARY KEY,
    book_id    INTEGER NOT NULL REFERENCES book(id) ON DELETE CASCADE,
    section_id TEXT NOT NULL,
    name       TEXT NOT NULL,
    path       TEXT NOT NULL,
    caption    TEXT NOT NULL,
    UNIQUE (book_id, name),
    FOREIGN KEY (book_id, section_id) REFERENCES section_new(book_id, id) ON DELETE CASCADE
);
INSERT INTO figure_new SELECT id, book_id, section_id, name, path, caption FROM figure;

CREATE TABLE unit_section_new (
    unit_id    INTEGER NOT NULL REFERENCES unit(id) ON DELETE CASCADE,
    book_id    INTEGER NOT NULL,
    section_id TEXT NOT NULL,
    ord        INTEGER NOT NULL,
    PRIMARY KEY (unit_id, section_id),
    FOREIGN KEY (book_id, section_id) REFERENCES section_new(book_id, id) ON DELETE CASCADE
);
INSERT INTO unit_section_new SELECT us.unit_id, u.book_id, us.section_id, us.ord FROM unit_section us JOIN unit u ON u.id = us.unit_id;

CREATE VIRTUAL TABLE section_fts_new USING fts5(book_id UNINDEXED, section_id UNINDEXED, heading, markdown);
INSERT INTO section_fts_new(book_id, section_id, heading, markdown)
    SELECT s.book_id, f.section_id, f.heading, f.markdown FROM section_fts f JOIN section s ON s.id = f.section_id;

DROP TABLE section_fts;
DROP TABLE unit_section;
DROP TABLE figure;
DROP TABLE section;
ALTER TABLE section_new RENAME TO section;
ALTER TABLE figure_new RENAME TO figure;
ALTER TABLE unit_section_new RENAME TO unit_section;
ALTER TABLE section_fts_new RENAME TO section_fts;
CREATE INDEX section_book_ord ON section(book_id, ord);
CREATE INDEX unit_section_section ON unit_section(book_id, section_id);
CREATE INDEX unit_book_ord ON unit(book_id, ord);

-- DAT-7: the only book so far is the active one.
INSERT INTO meta(key, value) SELECT 'active_book', CAST(MIN(id) AS TEXT) FROM book HAVING count(*) > 0;

-- +goose Down
-- Only works while section IDs are unique across books, i.e. with one book.
DELETE FROM meta WHERE key = 'active_book';
DROP INDEX unit_book_ord;

CREATE TABLE section_old (
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
INSERT INTO section_old SELECT id, book_id, parent_id, chapter, level, ord, heading, markdown, words, is_reference FROM section;

CREATE TABLE figure_old (
    id         INTEGER PRIMARY KEY,
    book_id    INTEGER NOT NULL REFERENCES book(id) ON DELETE CASCADE,
    section_id TEXT NOT NULL REFERENCES section_old(id) ON DELETE CASCADE,
    name       TEXT NOT NULL UNIQUE,
    path       TEXT NOT NULL,
    caption    TEXT NOT NULL
);
INSERT INTO figure_old SELECT id, book_id, section_id, name, path, caption FROM figure;

CREATE TABLE unit_section_old (
    unit_id    INTEGER NOT NULL REFERENCES unit(id) ON DELETE CASCADE,
    section_id TEXT NOT NULL REFERENCES section_old(id) ON DELETE CASCADE,
    ord        INTEGER NOT NULL,
    PRIMARY KEY (unit_id, section_id)
);
INSERT INTO unit_section_old SELECT unit_id, section_id, ord FROM unit_section;

CREATE VIRTUAL TABLE section_fts_old USING fts5(section_id UNINDEXED, heading, markdown);
INSERT INTO section_fts_old(section_id, heading, markdown) SELECT section_id, heading, markdown FROM section_fts;

DROP TABLE section_fts;
DROP TABLE unit_section;
DROP TABLE figure;
DROP TABLE section;
ALTER TABLE section_old RENAME TO section;
ALTER TABLE figure_old RENAME TO figure;
ALTER TABLE unit_section_old RENAME TO unit_section;
ALTER TABLE section_fts_old RENAME TO section_fts;
CREATE INDEX section_book_ord ON section(book_id, ord);
CREATE INDEX unit_section_section ON unit_section(section_id);
