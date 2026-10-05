# 02 Data model

SQLite in WAL mode at `/data/ddia.db`. Migrations are embedded in the binary and run on start.

```mermaid
erDiagram
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
    SECTION { text id PK "ch05.s03.s02"  int book_id FK  text parent_id  int level  int ord  text heading  text markdown  int words  bool is_reference }
    FIGURE { int id PK  text section_id FK  text path  text caption }
    UNIT { int id PK  int book_id FK  int ord  int chapter  text title  int words  bool manual }
    UNIT_SECTION { int unit_id FK  text section_id FK  int ord }
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
