-- Per-container rules: what a pin or major hold keeps back, and the fingerprint of the
-- rules a result was checked with ('' = no rules), so changed rules make it stale.
ALTER TABLE results ADD COLUMN held    TEXT NOT NULL DEFAULT '';
ALTER TABLE results ADD COLUMN held_by TEXT NOT NULL DEFAULT '';  -- pin | allow-major
ALTER TABLE results ADD COLUMN rules   TEXT NOT NULL DEFAULT '';
