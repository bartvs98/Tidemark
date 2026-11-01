-- run_stats.stale_tags: checked containers whose result came from an older tag list at the
-- run's end, because the registry rate-limited the refresh (and the end-of-run retry).
ALTER TABLE run_stats ADD COLUMN stale_tags INTEGER NOT NULL DEFAULT 0;
