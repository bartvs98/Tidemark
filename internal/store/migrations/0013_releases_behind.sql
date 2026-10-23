-- Update debt per run: releases between running and latest, summed over all updates
-- (from the stored notes; 0 for updates without). Rows from before it read 0.
ALTER TABLE run_stats ADD COLUMN releases_behind INTEGER NOT NULL DEFAULT 0;
