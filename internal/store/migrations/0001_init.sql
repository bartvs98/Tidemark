-- Check runs: one row per run, scheduled or manual.
CREATE TABLE check_runs (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    trigger     TEXT    NOT NULL,              -- startup | schedule | manual
    started_at  TEXT    NOT NULL,              -- RFC 3339, UTC
    finished_at TEXT,
    images      INTEGER NOT NULL DEFAULT 0,
    error       TEXT    NOT NULL DEFAULT ''
);

-- Latest result per container. Keyed by name: Compose gives a recreated container a
-- new ID but the same name. image_ref + image_id record what was checked, so a result
-- is ignored once the container runs a different image.
CREATE TABLE results (
    container_name  TEXT PRIMARY KEY,
    image_ref       TEXT    NOT NULL,
    image_id        TEXT    NOT NULL,
    running         TEXT    NOT NULL DEFAULT '',
    source          TEXT    NOT NULL DEFAULT '',
    latest          TEXT    NOT NULL DEFAULT '',
    latest_in_major TEXT    NOT NULL DEFAULT '',
    class           TEXT    NOT NULL DEFAULT '',
    floating        INTEGER NOT NULL DEFAULT 0,
    new_image       INTEGER NOT NULL DEFAULT 0,
    error           TEXT    NOT NULL DEFAULT '',
    checked_at      TEXT    NOT NULL,
    run_id          INTEGER REFERENCES check_runs(id) ON DELETE SET NULL
);

-- Registry tag lists, so restarts don't re-download repositories with thousands of tags.
CREATE TABLE tag_cache (
    repository TEXT PRIMARY KEY,               -- registry/repository, e.g. docker.io/library/redis
    tags       TEXT NOT NULL,                  -- JSON array
    fetched_at TEXT NOT NULL
);
