-- A failed check keeps the container's last good result and records the failure next to
-- it: checked_at is the last successful check ('' if there never was one), failed_at the
-- failed attempt since then ('' if the last attempt succeeded).
ALTER TABLE results ADD COLUMN failed_at TEXT NOT NULL DEFAULT '';

-- Until now a failure replaced the result, so an existing error row never succeeded:
-- its checked_at was the failed attempt.
UPDATE results SET failed_at = checked_at, checked_at = '' WHERE error != '';
