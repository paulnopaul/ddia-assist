# 02 Data model

SQLite in WAL mode at `/data/ddia.db`. Migrations are embedded in the binary and run on start.

```mermaid
erDiagram
    META ||--o| BOOK : "active_book"
    BOOK ||--o{ SECTION : contains
    BOOK ||--o{ UNIT : "planned into"
    UNIT ||--|{ UNIT_SECTION : spans
    SECTION ||--o{ UNIT_SECTION : "belongs to"
    SECTION ||--o{ FIGURE : has
    UNIT ||--|| PROGRESS : tracks
    UNIT ||--o{ CONCEPT : introduces
    UNIT ||--o{ SESSION : "studied in"
    SESSION ||--o{ ANSWER : records
    SESSION ||--o{ CONCEPT_SCORE : produces
    SESSION ||--o{ GAP : finds
    CONCEPT ||--o{ CONCEPT_SCORE : "scored by"
    CONCEPT ||--o| REVIEW_ITEM : schedules

    BOOK { int id PK  text title  text sha256  datetime imported_at }
    META { text key PK  text value }
    SECTION { int book_id PK  text id PK "ch05.s03.s02"  text parent_id  int level  int ord  text heading  text markdown  int words  bool is_reference }
    FIGURE { int id PK  int book_id FK  text section_id FK  text name  text path  text caption }
    UNIT { int id PK  int book_id FK  int ord  int chapter  text title  int words  bool manual }
    UNIT_SECTION { int unit_id FK  int book_id FK  text section_id FK  int ord }
    PROGRESS { int unit_id PK  text status "not_started|reading|read|studied"  datetime read_at  datetime studied_at }
    CONCEPT { int id PK  int unit_id FK  text name  text definition  text section_ref }
    SESSION { int id PK  text kind "study|review"  int unit_id FK  datetime started_at  datetime finished_at }
    ANSWER { int id PK  int session_id FK  text step "explain|probe|apply|review"  text task_shape  text prompt  text response  text feedback  int score }
    CONCEPT_SCORE { int session_id FK  int concept_id FK  int score "0-3"  text note  int user_override }
    GAP { int id PK  int session_id FK  int concept_id FK  text description  text section_ref }
    REVIEW_ITEM { int concept_id PK  datetime due_at  int interval_days  int streak  int last_score }
```

- **DAT-1** `section_fts` is an FTS5 table over `heading` and `markdown` for non-reference sections.
- **DAT-2** A concept's *current score* is its latest `CONCEPT_SCORE`, with `user_override`
  taking precedence over `score` when it is set.
- **DAT-3** Raw answers (`ANSWER.response`) MUST be stored verbatim. They're the evidence used to audit a score.
- **DAT-4** Deleting a book MUST cascade. No other deletes are exposed.
- **DAT-5** Units count as *in read scope* when their `PROGRESS.status` is `read` or `studied`.
- **DAT-6** Each book's data is separate. A section is keyed by `(book_id, id)`, a figure name is unique
  within its book, and `section_fts` carries the book ID. Units, sessions and concepts keep global integer
  IDs, so a unit number names one unit across all books. A `section_ref` on a concept or gap is read
  within that concept's or session's book. Upgrading a database from before multi-book support MUST keep
  all of its data as book 1.
- **DAT-7** The active book is stored in `meta` under `active_book`. While any book exists, exactly one is
  active. Deleting the active book makes the oldest remaining book active.
