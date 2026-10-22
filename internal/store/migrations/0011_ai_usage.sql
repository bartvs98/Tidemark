-- Usage of AI summaries (step 10): the cost of the call that made each summary, and one
-- record per provider call (manual or automatic, success or failure). Records are never pruned.
ALTER TABLE ai_summaries ADD COLUMN input_tokens INTEGER NOT NULL DEFAULT 0;
ALTER TABLE ai_summaries ADD COLUMN output_tokens INTEGER NOT NULL DEFAULT 0;
ALTER TABLE ai_summaries ADD COLUMN duration_ms INTEGER NOT NULL DEFAULT 0;

-- Extending a summary when newer releases appear: latest (as displayed when made),
-- extended_from (the base's latest; '' = a full summary), covered (releases in range when
-- made, sent or not) and covered_hash (over those releases' tags + bodies, so a newer range
-- whose oldest part is unchanged can be recognised).
ALTER TABLE ai_summaries ADD COLUMN latest TEXT NOT NULL DEFAULT '';
ALTER TABLE ai_summaries ADD COLUMN extended_from TEXT NOT NULL DEFAULT '';
ALTER TABLE ai_summaries ADD COLUMN covered INTEGER NOT NULL DEFAULT 0;
ALTER TABLE ai_summaries ADD COLUMN covered_hash TEXT NOT NULL DEFAULT '';

CREATE TABLE ai_calls (
  id INTEGER PRIMARY KEY,
  at TEXT NOT NULL,                       -- start of the call
  provider TEXT NOT NULL, model TEXT NOT NULL,
  trigger TEXT NOT NULL,                  -- manual | auto
  mode TEXT NOT NULL DEFAULT 'full',      -- full | extended
  repo TEXT NOT NULL, from_version TEXT NOT NULL, to_version TEXT NOT NULL,
  releases INTEGER NOT NULL DEFAULT 0,    -- releases sent
  input_tokens INTEGER NOT NULL DEFAULT 0, output_tokens INTEGER NOT NULL DEFAULT 0,
  duration_ms INTEGER NOT NULL DEFAULT 0,
  error TEXT NOT NULL DEFAULT ''          -- '' = success; the scrubbed message otherwise
);
CREATE INDEX ai_calls_at ON ai_calls(at);
