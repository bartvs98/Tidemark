-- Stats (step 11): one row per finished check run with what the dashboard showed at its end,
-- plus the outgoing requests made since the previous row. Kept forever (a few hundred bytes a run).
CREATE TABLE run_stats (
  run_id      INTEGER PRIMARY KEY,          -- = check_runs.id
  at          TEXT NOT NULL,                -- run finish, fixed-width UTC (text-orderable)
  duration_ms INTEGER NOT NULL,
  containers  INTEGER NOT NULL,             -- all containers on the host (ignored included)
  ignored     INTEGER NOT NULL,
  checked     INTEGER NOT NULL,             -- non-ignored with a result from a successful check
  up_to_date  INTEGER NOT NULL,             -- checked and class none
  updates     INTEGER NOT NULL,             -- open updates (not reviewed or snoozed)
  attention   INTEGER NOT NULL,             -- open updates at level attention
  breaking    INTEGER NOT NULL,             -- open updates at level breaking
  reviewed    INTEGER NOT NULL,
  snoozed     INTEGER NOT NULL,
  failed      INTEGER NOT NULL,             -- containers whose latest attempt failed
  classes     TEXT NOT NULL,                -- JSON {"patch":n,…}: all updates, open or not; zero keys left out
  failures    TEXT NOT NULL,                -- JSON {"rate-limit":n,…,"other":n}; zero keys left out
  requests    TEXT NOT NULL,                -- JSON {"registry":{"requests":n,"limited":n},"github":{…},"gitlab":{…}}
  github_remaining INTEGER,                 -- last-known GitHub quota at run end; NULL when unknown
  github_limit     INTEGER
);
CREATE INDEX run_stats_at ON run_stats(at);
