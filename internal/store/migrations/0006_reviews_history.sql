-- reviews holds one review state per container (by name): reviewed (read the changes up
-- to version) or snoozed (until a time, or until a version newer than version).
-- history records each image change between successful checks, plus a first row when a
-- container is first checked (prev_* empty: tracking started).
CREATE TABLE reviews (
  container_name TEXT PRIMARY KEY,
  kind TEXT NOT NULL,                -- reviewed | snoozed
  version TEXT NOT NULL DEFAULT '',  -- reviewed: read up to; snoozed: until a version newer than this ('' when by time)
  until TEXT NOT NULL DEFAULT '',    -- snoozed by time: RFC3339 ('' otherwise)
  created_at TEXT NOT NULL
);
CREATE TABLE history (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  container_name TEXT NOT NULL,
  image_id TEXT NOT NULL,
  running TEXT NOT NULL DEFAULT '',
  prev_image_id TEXT NOT NULL DEFAULT '',  -- '' on the first row (tracking started)
  prev_running TEXT NOT NULL DEFAULT '',
  detected_at TEXT NOT NULL,
  via TEXT NOT NULL DEFAULT 'check'        -- check; step 11 adds oneclick
);
CREATE INDEX history_container ON history (container_name, id);
