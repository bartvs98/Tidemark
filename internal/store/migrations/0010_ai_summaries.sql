-- ai_summaries caches one AI summary per repo and version range (step 10).
-- created_at '' = never succeeded; error/failed_at hold the last failed attempt.
CREATE TABLE ai_summaries (
  repo TEXT NOT NULL, from_version TEXT NOT NULL, to_version TEXT NOT NULL,
  provider TEXT NOT NULL DEFAULT '', model TEXT NOT NULL DEFAULT '',
  changes_json TEXT NOT NULL DEFAULT '[]', actions_json TEXT NOT NULL DEFAULT '[]',
  verdict TEXT NOT NULL DEFAULT 'none', reasons_json TEXT NOT NULL DEFAULT '[]',
  releases INTEGER NOT NULL DEFAULT 0, omitted INTEGER NOT NULL DEFAULT 0,
  notes_hash TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL DEFAULT '',
  error TEXT NOT NULL DEFAULT '', failed_at TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (repo, from_version, to_version)
);
