-- settings holds values edited in the settings dialog (TIDEMARK_SETTINGS_EDITABLE=true).
-- An env var still wins over a stored value. Secrets are stored as plain text, like the rest of the data dir.
CREATE TABLE settings (
  key TEXT PRIMARY KEY,   -- config.Key, e.g. schedule, github_token
  value TEXT NOT NULL     -- canonical form (config.Normalize)
);
