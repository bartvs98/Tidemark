-- notified remembers which update (container + review key) was sent, and with which level,
-- so it isn't sent again unless its level rises or a time snooze ran out since.
CREATE TABLE notified (
  container_name TEXT NOT NULL,
  key TEXT NOT NULL,              -- the update's review key: latest version or build digest
  level TEXT NOT NULL,            -- none | attention | breaking, as sent
  notified_at TEXT NOT NULL,
  PRIMARY KEY (container_name, key)
);
-- notifications logs every send attempt (also failed ones and tests).
CREATE TABLE notifications (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  sent_at TEXT NOT NULL,
  notifier TEXT NOT NULL,         -- telegram
  kind TEXT NOT NULL,             -- updates | first | test
  items INTEGER NOT NULL,
  error TEXT NOT NULL DEFAULT ''
);
