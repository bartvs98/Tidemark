-- Seed for the stats page (scenarios/stats.json), on the demo host. Runs against tidemark.db after
-- the startup check, so the real startup run and its run_stats row are the newest point.
-- All times are relative to 'now'. Seeded ids sit away from the live ones: check_runs and ai_calls
-- from 1000, history negative (the app reads the highest history id per container, which stays real).

-- ── Check runs and run stats: 120 runs over 60 days, twice a day ──────────────
WITH RECURSIVE n(i) AS (SELECT 0 UNION ALL SELECT i + 1 FROM n WHERE i < 119),
runs AS (
  SELECT i,
         (119 - i) * 12 + 1 AS hours_ago,
         3 + (i * 7919) % 5 + CASE WHEN i % 13 = 0 THEN 20 ELSE 0 END AS secs
  FROM n
)
INSERT INTO check_runs (id, trigger, started_at, finished_at, images, error)
SELECT 1000 + i,
       CASE WHEN i = 0 THEN 'startup' WHEN i % 17 = 0 THEN 'manual' ELSE 'schedule' END,
       strftime('%Y-%m-%dT%H:%M:%SZ', 'now', '-' || hours_ago || ' hours'),
       strftime('%Y-%m-%dT%H:%M:%SZ', 'now', '-' || hours_ago || ' hours', '+' || secs || ' seconds'),
       9,
       CASE WHEN i % 41 = 40 THEN 'docker: connection refused' ELSE '' END
FROM runs;

WITH RECURSIVE n(i) AS (SELECT 0 UNION ALL SELECT i + 1 FROM n WHERE i < 119),
runs AS (
  SELECT i,
         (119 - i) * 12 + 1 AS hours_ago,
         3 + (i * 7919) % 5 + CASE WHEN i % 13 = 0 THEN 20 ELSE 0 END AS secs,
         1 + (i / 10) % 4 AS updates
  FROM n
)
INSERT INTO run_stats (run_id, at, duration_ms, containers, ignored, checked, up_to_date, updates, attention, breaking,
                       reviewed, snoozed, failed, releases_behind, classes, failures, requests, github_remaining, github_limit)
SELECT 1000 + i,
       strftime('%Y-%m-%dT%H:%M:%S', 'now', '-' || hours_ago || ' hours', '+' || secs || ' seconds') || '.000000000Z',
       secs * 1000 + (i * 137) % 900,
       9, 0, 9,
       9 - updates,
       updates,
       CASE WHEN updates IN (2, 4) THEN 1 ELSE 0 END,
       CASE WHEN updates >= 3 THEN 1 ELSE 0 END,
       0, 0,
       CASE WHEN i % 29 = 28 THEN 1 ELSE 0 END,
       20 + updates * 12 + (i % 10) * 2,
       printf('{"patch":1,"minor":%d}', updates),
       CASE WHEN i % 29 = 28 THEN '{"rate-limit":1}' WHEN i % 53 = 52 THEN '{"timeout":1}' ELSE '{}' END,
       printf('{"registry":{"requests":%d,"limited":0},"github":{"requests":%d,"limited":%d}}',
              22 + (i * 5) % 14, (i * 3) % 9, CASE WHEN i % 47 = 46 THEN 1 ELSE 0 END),
       4800 - (i % 9) * 20,
       5000
FROM runs;

-- ── Release notes for a few repos, so the upstream charts and time to update have data ─────
-- (INSERT OR IGNORE: real notes fetched from GitHub win. The repos rows mark them as fresh, so
-- the verify run makes no GitHub requests for them.)
WITH r(repo, tag, days_ago, body) AS (VALUES
  ('github.com/grafana/grafana', 'v11.5.0', 70, '- Dashboard fixes'),
  ('github.com/grafana/grafana', 'v11.6.0', 55, '- Alerting improvements'),
  ('github.com/grafana/grafana', 'v12.0.0', 40, '## Breaking changes' || char(10) || '- Removed legacy alerting'),
  ('github.com/grafana/grafana', 'v12.1.0', 28, '- New panel options'),
  ('github.com/grafana/grafana', 'v12.2.0', 14, '- Faster queries'),
  ('github.com/grafana/grafana', 'v12.2.1', 5, '- Bug fixes'),
  ('github.com/traefik/traefik', 'v3.5.0', 60, '- New middleware'),
  ('github.com/traefik/traefik', 'v3.6.0', 45, '- Kubernetes gateway updates'),
  ('github.com/traefik/traefik', 'v3.6.1', 36, '- Bug fixes'),
  ('github.com/traefik/traefik', 'v3.6.2', 30, '- Bug fixes'),
  ('github.com/traefik/traefik', 'v3.7.0', 20, '## Breaking changes' || char(10) || '- The old plugin API is removed'),
  ('github.com/traefik/traefik', 'v3.7.1', 12, '- Bug fixes'),
  ('github.com/home-assistant/core', '2026.7.0', 80, '- New integrations'),
  ('github.com/home-assistant/core', '2026.8.0', 50, '## Breaking changes' || char(10) || '- The legacy YAML sensor format is removed'),
  ('github.com/home-assistant/core', '2026.9.0', 22, '- New integrations'),
  ('github.com/home-assistant/core', '2026.10.0', 4, '## Breaking changes' || char(10) || '- Python 3.14 is required'),
  ('github.com/louislam/uptime-kuma', '1.23.15', 65, '- Bug fixes'),
  ('github.com/louislam/uptime-kuma', '1.23.16', 35, '- Bug fixes'),
  ('github.com/louislam/uptime-kuma', '2.0.0', 10, '## Breaking changes' || char(10) || '- The database layout changed; back up before updating'),
  ('github.com/prometheus/prometheus', 'v3.13.0', 58, '- Scrape improvements'),
  ('github.com/prometheus/prometheus', 'v3.14.0', 32, '- Bug fixes'),
  ('github.com/prometheus/prometheus', 'v3.15.0', 9, '- Query engine updates'),
  -- A quiet upstream: no release in over 9 months.
  ('github.com/eclipse/mosquitto', 'v2.0.20', 420, '- Bug fixes'),
  ('github.com/eclipse/mosquitto', 'v2.0.22', 310, '- Bug fixes')
)
INSERT OR IGNORE INTO releases (repo, tag, name, body, url, published_at, prerelease)
SELECT repo, tag, tag, body, 'https://' || repo || '/releases/tag/' || tag,
       strftime('%Y-%m-%dT%H:%M:%SZ', 'now', '-' || days_ago || ' days'), 0
FROM r;

INSERT OR IGNORE INTO repos (repo, kind, has_more, fetched_at)
SELECT DISTINCT repo, 'releases', 0, strftime('%Y-%m-%dT%H:%M:%SZ', 'now') FROM releases;

-- ── Applied updates: history rows with a previous image ──────────────────────
WITH h(name, prev, running, days_ago) AS (VALUES
  ('homeassistant', '2026.7.0', '2026.8.0', 46),
  ('homeassistant', '2026.8.0', '2026.9.0', 19),
  ('grafana', '11.5.0', '11.6.0', 50),
  ('traefik', 'v3.5.0', 'v3.6.0', 41),
  ('traefik', 'v3.6.0', 'v3.6.1', 34),
  ('traefik', 'v3.6.1', 'v3.6.2', 29),
  ('traefik', 'v3.6.2', 'v3.7.0', 16),
  ('uptime-kuma', '1.23.15', '1.23.16', 30),
  ('prometheus', 'v3.13.0', 'v3.14.0', 28),
  ('prometheus', 'v3.14.0', 'v3.15.0', 7),
  ('nginx', '1.29.3-alpine', '1.30.0-alpine', 52),
  ('nginx', '1.30.0-alpine', '1.31.7-alpine', 11),
  ('mosquitto', '2.0.20', '2.0.22', 24),
  ('vaultwarden', '1.33.2', '1.34.0', 57),
  ('vaultwarden', '1.34.0', '1.37.2', 38),
  ('audiobookshelf', '2.30.0', '2.37.1', 21),
  ('homeassistant', '2026.6.0', '2026.7.0', 3),
  ('grafana', '11.4.0', '11.5.0', 62),
  ('nginx', '1.29.0-alpine', '1.29.3-alpine', 3),
  ('mosquitto', '2.0.18', '2.0.20', 44),
  -- A same-version rebuild: an applied image change, but not an update to time.
  ('mosquitto', '2.0.22', '2.0.22', 6)
)
INSERT INTO history (id, container_name, image_id, running, prev_image_id, prev_running, detected_at, via)
SELECT -(ROW_NUMBER() OVER ()), name,
       'sha256:seed-' || name || '-' || running, running,
       'sha256:seed-' || name || '-' || prev, prev,
       strftime('%Y-%m-%dT%H:%M:%SZ', 'now', '-' || days_ago || ' days', '-' || ((days_ago * 37) % 11) || ' hours'),
       'check'
FROM h;

-- ── Telegram messages for some of those updates (the key is the latest version) ─────────────
WITH m(name, key, days_ago) AS (VALUES
  ('traefik', 'v3.7.0', 18),
  ('prometheus', 'v3.15.0', 8),
  ('homeassistant', '2026.9.0', 21),
  ('vaultwarden', '1.37.2', 45)
)
INSERT OR REPLACE INTO notified (container_name, key, level, notified_at)
SELECT name, key, 'none', strftime('%Y-%m-%dT%H:%M:%SZ', 'now', '-' || days_ago || ' days') FROM m;

-- ── AI calls: 30 over about 45 days ──────────────────────────────────────────
WITH RECURSIVE n(i) AS (SELECT 0 UNION ALL SELECT i + 1 FROM n WHERE i < 29)
INSERT INTO ai_calls (id, at, provider, model, trigger, mode, repo, from_version, to_version, releases,
                      input_tokens, output_tokens, duration_ms, error)
SELECT 1000 + i,
       strftime('%Y-%m-%dT%H:%M:%S', 'now', '-' || ((29 - i) * 36 + 2) || ' hours') || '.000000000Z',
       'anthropic',
       CASE WHEN i % 5 = 0 THEN 'claude-sonnet-4-5' ELSE 'claude-haiku-4-5' END,
       CASE WHEN i % 3 = 0 THEN 'manual' ELSE 'auto' END,
       CASE WHEN i % 7 = 0 THEN 'extended' ELSE 'full' END,
       CASE i % 5 WHEN 0 THEN 'github.com/grafana/grafana' WHEN 1 THEN 'github.com/traefik/traefik'
                  WHEN 2 THEN 'github.com/home-assistant/core' WHEN 3 THEN 'github.com/prometheus/prometheus'
                  ELSE 'github.com/louislam/uptime-kuma' END,
       CASE i % 5 WHEN 0 THEN '11.6.0' WHEN 1 THEN 'v3.6.2' WHEN 2 THEN '2026.8.0' WHEN 3 THEN 'v3.14.0' ELSE '1.23.15' END,
       CASE i % 5 WHEN 0 THEN '12.0.0' WHEN 1 THEN 'v3.7.0' WHEN 2 THEN '2026.9.0' WHEN 3 THEN 'v3.15.0' ELSE '2.0.0' END,
       3 + i % 6,
       CASE WHEN i % 11 = 7 THEN 0 ELSE 6000 + (i * 1237) % 9000 END,
       CASE WHEN i % 11 = 7 THEN 0 ELSE 400 + (i * 173) % 900 END,
       2500 + (i * 911) % 6000,
       CASE WHEN i % 11 = 7 THEN 'provider returned 529: overloaded' ELSE '' END
FROM n;
