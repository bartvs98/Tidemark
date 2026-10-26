-- pruned_releases keeps the lifetime totals of release notes that pruning deleted, so the
-- stats page's lifetime numbers don't drop. One row.
CREATE TABLE pruned_releases (
  id       INTEGER PRIMARY KEY CHECK (id = 1),
  releases INTEGER NOT NULL DEFAULT 0,
  breaking INTEGER NOT NULL DEFAULT 0,
  words    INTEGER NOT NULL DEFAULT 0
);
INSERT INTO pruned_releases (id) VALUES (1);
