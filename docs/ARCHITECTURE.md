# Architecture

How Tidemark is built. Behaviour and decision rules are in [FEATURES.md](FEATURES.md); env vars and rules syntax in the [README](../README.md#configuration). Parts marked *later* are planned (see the [ROADMAP](ROADMAP.md)).

## Overview

One Go binary that contains the backend, the scheduler and the compiled Angular frontend (embedded with `embed.FS`). It ships as a single small Docker image and talks to the host's Docker daemon through a socket proxy.

```
┌──────────────────────────── tidemark container ─────────────────────────────┐
│                                                                              │
│   Angular SPA (embedded static files)  ◄──── HTTP :8080 ────►  Browser (LAN)  │
│              │ JSON /api/*                                                   │
│   ┌──────────▼──────────────────────────────┐                                │
│   │ HTTP API: check results + stored notes, │                                │
│   │ sliced running → latest; breaking       │                                │
│   │ analysis computed on read               │                                │
│   └──────────┬──────────────────────────────┘                                │
│              │                                                               │
│   ┌──────────▼──────────────────────────────────────────────┐               │
│   │                    Check service                         │◄── Scheduler  │
│   │  inventory → resolve version → latest → prefetch notes   │    (cron)     │
│   │  (updates only) → persist → notify                       │               │
│   └──┬──────────┬───────────┬────────────┬───────────┬──────┘               │
│      │          │           │            │           │                       │
│   docker     registry    changelog     ai         notify       updater       │
│   (socket)   (OCI v2)    (GitHub/      (opt-in)   (Telegram)   (later,       │
│      │          │         GitLab)                              gated)        │
│      │          │           │                                                │
│   ┌──▼──────────▼───────────▼─────── SQLite (/data/tidemark.db) ──────────┐ │
│   └────────────────────────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────────────────────┘
        │                     │                    │
   /var/run/docker.sock   registries          api.github.com, gitlab.com,
   (via socket-proxy)     (Hub, GHCR, …)      icon CDN, Telegram, AI provider
```

## Tech stack

| Layer | Choice | Why |
|---|---|---|
| Language | **Go 1.26+** | Fast, small static binary, low memory, great Docker ecosystem |
| Docker | Own minimal read-only Engine API client (`internal/docker`) | Only 2 GET endpoints needed; avoids the SDK dependency tree; same decoder for live and snapshot mode. One-click updates (a parked idea) would add pull/create |
| Registry | `github.com/google/go-containerregistry` | Handles anonymous token auth for Hub/GHCR/Quay/lscr, tag listing, manifest HEAD, multi-arch indexes |
| Versions | Own lenient parser (`internal/version`) | Real tags aren't semver: `16-alpine`, `v1.15.0-ls175`, `2026.9.0`, `7.4.2-alpine3.21` |
| HTTP | stdlib `net/http` (Go 1.22+ pattern routing) | No framework needed |
| DB | SQLite via `modernc.org/sqlite` (pure Go, no CGO) | Zero ops, single file, easy cross-compile |
| Migrations | Embedded `.sql` files, applied on start | Simple, no extra tooling |
| Scheduler | `github.com/robfig/cron/v3` | Cron expressions |
| Config | Env vars (`TIDEMARK_*`) + optional rules `config.yaml` + container labels | Labels keep per-container rules next to the compose file |
| Frontend | **Angular 22**: standalone components, signals, zoneless, `HttpClient` (`withFetch()`) + `httpResource`, route params as component inputs (`withComponentInputBinding`) | Small dashboard; it doubles as Angular practice |
| Accessible widgets | `@angular/aria` (headless tabs; menus/listboxes when needed) | Keyboard and ARIA handled for us; we only style |
| UI styling | Tailwind CSS v4 + own design system (`web/src/app/ui`) | Semantic tokens with `light-dark()`; see [DESIGN-SYSTEM.md](DESIGN-SYSTEM.md) |
| Markdown | Backend stores raw release notes; the frontend renders them with `marked` + DOMPurify | Render untrusted notes safely |
| Image | Multi-stage: `node` build → `golang` build → `gcr.io/distroless/static` | ~20 MB final image, non-root |

## Repository layout

```
/cmd/tidemark/main.go        entrypoint: config, wiring, start HTTP + scheduler
/internal/
  config/                     TIDEMARK_* environment variables; the editable settings' keys, shared validation
                              (Normalize) and precedence (env > stored > default)
  settings/                   editable settings: stored values, validation of a change, applied live through
                              appliers wired in main (schedule, GitHub token, icons, notifier, AI provider)
  docker/                     Source interface: live Engine API client + snapshot file reader
  imageref/                   parse image references (registry, repository, tag, digest)
  icons/                      selfh.st icon index, container → icon matching, cached icon serving, built-in icons (builtin/, embedded)
  version/                    parse tags (prefix, numbers, variant, pre-release, build), compare, classify
  registry/                   tag lists + manifest digests via go-containerregistry, cached, anonymous
  resolve/                    one container: running version, latest in family (within pin / major hold), update class
  rules/                      per-container rules: labels + config.yaml, merged, validated, fingerprinted
  source/                     image → source repo (label, plausible image label, ghcr, known list, guess)
  changelog/                  GitHub / GitLab releases + CHANGELOG.md, ETag sync into the store, range selection
  analysis/                   breaking-change heuristics: per-line flags, section inheritance, verdict + reasons
  review/                     whether a stored review / snooze still applies (Active)
  stats/                      outgoing request counters (registry, GitHub, GitLab) as an http.RoundTripper, read per run
  ai/                         AI summaries: prompt + input cap, Provider interface (Anthropic via its SDK,
                              OpenAI-compatible for openai and gemini), stored summaries, auto mode
  notify/                     new-update selection (sent once, again when the level rises or a snooze ends),
                              notifier interface + Telegram, digest schedule
  updater/                    (later) gated pull/recreate/rollback
  store/                      SQLite repository + migrations
  check/                      background check runs over all containers (skips ignored ones), results in memory + SQLite,
                              image-change history
  api/                        HTTP handlers + embedded SPA serving
/web/                         Angular workspace
/web/embed.go                 embeds web/dist/spa into the binary
/docs/                        these documents
/scripts/snapshot.sh          export a host's container/image metadata for development (no container env;
                              only image *_VERSION variables)
/scripts/dev/verify.mjs       visual verification: build, run against a snapshot, screenshot (verify skill)
/scripts/dev/scenarios/       verify scenarios: seeded states and named shots on the demo host
/.github/workflows/image.yml  CI, see Deployment
/.github/workflows/cleanup.yml  deletes old image versions from GHCR, see Deployment
Dockerfile
compose.example.yaml
```

## Check pipeline

One check run covers every container (when runs happen: [FEATURES §7](FEATURES.md#7-checks-failures-and-settings)). Limits: 6 workers, at most 4 concurrent requests per registry host, 2 min per container, 60 s per registry request. The rules file is re-read at the start of each run.

1. **Inventory:** `GET /containers/json?all=1` plus `GET /images/{id}/json` once per image. This gives name, image ref, image ID, `RepoDigests`, labels and Compose project/service.
   - Container inspect is never called, because it exposes environment variables. From the image environment only `*_VERSION` variables are kept.
   - Ignored containers are dropped here, before any registry call.
   - An image deleted since the container was created fails that container with "the container's image is no longer on this host". Any other failed image inspect also fails only that container.
   - If the image was re-tagged after the container started, Docker reports a bare `sha256:…`. Tidemark then uses the image's first `RepoTag`. After a `docker pull` without recreating, the old image has no tag left, so it uses the first `RepoDigest` (`redis@sha256:…`) and checks it like an image pinned by digest.
2. **Resolve the running version** ([FEATURES §2](FEATURES.md#2-running-version-resolution)). This uses `RepoDigests`, because the local image ID isn't the registry's manifest digest. Digest matching only probes tags written like the running tag (`16.10-alpine` for `16-alpine`, not `16.10-alpine3.22`), at the most precise level, newest first, at most 30 HEADs.
3. **Latest version** ([FEATURES §3](FEATURES.md#3-latest-version-lookup)).
   - Tag lists are cached 1 h per repo, digests too (Check now ignores what's cached from before it started: `registry.FreshSince`). When the registry rate-limits the request and a stored list exists, that list is used but not re-cached (`registry.StaleError` → `Result.TagsFrom`).
   - For floating tags, the tag's manifest is `HEAD`ed to detect a new image under the same tag.
   - Containers with the same image and the same rules are checked once.
   - Channel tags (`resolve.Channel`) get `channel` in the API, which skips the verdict.
4. **Changelog prefetch:** an after-check hook in `cmd/tidemark/main.go`, only for images with an update (the hook is capped at 3 min per image; the sync inside it at 1 min). It resolves the source repo and syncs its notes into the store (`changelog.Service.Sync` with the running and latest version; when it refreshes: [FEATURES §4](FEATURES.md#4-changelog-between-versions)). GitHub's first page is a conditional request (ETag), so an unchanged repo costs no quota. The run stores notes but doesn't select a range: the API picks running → latest on read. If nothing was prefetched, the detail endpoint syncs on demand.
5. **Analysis** isn't part of the run. The API computes flags and level from the stored notes on every read (`changelog.Service.Changes` is cached per repo until a sync stores something), so list and detail always agree.
6. **AI summary** (auto mode only, [FEATURES §11](FEATURES.md#11-ai-summaries)): in the same after-check hook, after the sync, `ai.Service.Auto` with its own 2-min cap. It builds the request from the stored notes in range (`ai.RequestFrom`) and calls the provider only when the update has at least `TIDEMARK_AI_AUTO_MIN_RELEASES` releases, the newest of them has notes, there's no current summary (none, or stale: `NotesHash` differs) and the last failure is over 6 h old. Like Summarise, it extends a base when it can: a stored summary with the same repo and from version, under another key, nothing left out by the cap, whose covered releases are exactly the oldest part of the new range (`CoveredHash` of that suffix equals the stored one); the most complete base wins. The extend prompt carries the base as JSON in an `<earlier-summary>` block and only the new releases' notes; the result's verdict is `max(model, base)`. One call per key at a time (singleflight). The verdict is applied on read, like the analysis: `api.Server.applyAI` raises the level to it after `assess`, from memory, for the list, the detail page and notifications alike.
7. **Persist:**
   - Upsert a result per container name. A failed check keeps the last good result (for the same image ref, image ID and rules) and only sets `error`, `error_kind` and `failed_at`; a later success clears them.
   - Record the run (trigger, start/end, images checked, error), and prune results of containers that are gone or now ignored.
   - A successful check whose image ID differs from the container's last `history` row adds a row. The first check adds a "tracking started" row without a previous image. The last row per container is kept in memory, loaded at start.
   - On shutdown (SIGTERM) a running check is cancelled: it hands out no more images, doesn't save checks that were cut short, and records the run with the error `cancelled at shutdown` and the images done so far. The database closes only after the run has written its finish time (up to 30 s).
8. **Prune AI summaries:** an after-run hook (`check.Service.SetAfterRun`, which gets the finished run as `check.Finished`; skipped when the run itself failed or was cancelled) runs steps 8–10 in order. First it prunes AI summaries: `api.Server.AIKeys` lists the key of every update the list would look one up for (the same code path as `applyAI`), and `ai.Service.Prune` deletes the stored summaries outside it, except, per needed key, the best extension base for its repo and from version (skipped when Docker can't be listed; keys being generated are kept).
9. **Run stats:** then the hook stores one `run_stats` row: `api.Server.RunStats` counts what the dashboard shows (the same `updateFor`, rules and reviews), plus the run's duration, the requests counted since the previous row (`stats.Requests.Since`; the registry client and the changelog client go through `stats.Requests.Wrap`) and the last-known GitHub quota (`changelog.Service.LastQuota`, no request; unknown once its reset time has passed). The request baseline only moves once the row is stored. If the counts can't be taken or stored (Docker unreachable, no run id, a failed write), the requests carry over to the next row.
10. **Notify** (when configured): last, the hook calls `notify.Service.Run`, or a digest cron does instead ([FEATURES §10](FEATURES.md#10-notifications)). It reads the current updates through `api.Server.Updates`, the same `updateFor` as the list endpoint, so a message and the dashboard agree on level and review state. It compares them with `notified`, sends one `Batch` through the `Notifier`, logs the attempt in `notifications` and, only when the send worked, records what was sent.

## Data model (SQLite)

File: `$TIDEMARK_DATA_DIR/tidemark.db` (WAL mode). Migrations live in `internal/store/migrations/NNNN_name.sql`, are embedded in the binary and applied in order on start (`schema_migrations` records the version).

```
check_runs      id, trigger (startup|schedule|manual), started_at, finished_at, images, error
results         container_name (PK), image_ref, image_id, running, source, latest, latest_in_major,
                class, floating, new_image, held, held_by, rules (fingerprint), error, error_kind, checked_at, failed_at, tags_from, tag_digest, run_id
tag_cache       repository (PK), tags (JSON), fetched_at
releases        repo, tag, name, body, url, published_at, prerelease   (PK repo+tag)
repos           repo (PK), kind (releases|changelog|none), etag, has_more, fetched_at, error
reviews         container_name (PK), kind (reviewed|snoozed), version, until, created_at
history         id, container_name, image_id, running, prev_image_id, prev_running, detected_at, via (check; oneclick later)
notified        container_name + key (PK; the review key), level (as sent), notified_at
notifications   id, sent_at, notifier (telegram), kind (updates|first|test), items (count), error
settings        key (PK; e.g. schedule, github_token), value (canonical form, secrets in plain text)
ai_summaries    repo + from_version + to_version (PK), provider, model, changes_json, actions_json, verdict, reasons_json,
                releases, omitted, notes_hash, latest, extended_from, covered, covered_hash, input_tokens, output_tokens,
                duration_ms, created_at, error, failed_at
run_stats       run_id (PK; = check_runs.id), at, duration_ms, containers, ignored, checked, up_to_date, updates, attention, breaking,
                reviewed, snoozed, failed, releases_behind, classes (JSON), failures (JSON), requests (JSON), github_remaining, github_limit
ai_calls        id, at, provider, model, trigger (manual|auto), mode (full|extended), repo, from_version, to_version, releases,
                input_tokens, output_tokens, duration_ms, error ('' = success)
```

- **Results are keyed by container name,** because Compose gives a recreated container a new ID. Each result records the image ref, image ID and rules fingerprint it was checked with. Once the container runs a different image (after `pull` + `up`) or its rules change, the stored result is ignored until the next check.
- **Result columns that need explaining** ('' means not set):
  - `checked_at`: the last successful check.
  - `failed_at`: when the attempt that produced `error` ran ('' if the last attempt worked).
  - `error_kind`: classifies `error` as `rate-limit`, `not-found`, `timeout`, `network` or `registry` (`registry.KindOf`).
  - `tags_from`: set when the tag list came from `tag_cache` because the registry rate-limited the request.
  - `tag_digest`: the floating tag's current registry digest, when fetched.
- **Reviews, history and `notified`** are keyed by container name too, and none of them is pruned when a container goes away.
  - `reviews.version` is "read up to" (reviewed) or "until newer than" (snoozed by version); it's '' when snoozed by time, and `until` is set instead. It holds the update's `reviewKey`: the latest version, or a `sha256:…` build digest for a channel tag or an unknown latest. `review.Active` decides on read whether a review still applies.
  - `history.prev_image_id` is '' on a container's first row.
- **`ai_summaries`:** versions are `ai.VersionKey` (numbers, pre-release and build; no prefix or variant), `from_version` is `?` when the running version is unknown. `created_at` is '' while no attempt has succeeded; a failed attempt sets `error` + `failed_at` and keeps an earlier summary. `notes_hash` covers the versions and every release's tag and body in range, before the cap. All rows are loaded into memory at start. A row goes at the end of the first check run whose updates no longer need it (see Check pipeline, Prune AI summaries). The token and duration columns are those of the call that made the summary. `covered` is the releases in range when it was made (sent or not; on an extension the base's plus the new ones) and `covered_hash` hashes their tags and bodies, newest first, so a newer range's oldest releases can be compared with it; `latest` is the latest version as displayed and `extended_from` the base's (`''` for a full summary).
- **`ai_calls`:** one row per provider call, on demand or automatic, success or failure (a call refused before it's sent, AI off or no notes, isn't recorded). Never pruned; the stats page builds on it. `at` is a fixed-width UTC timestamp so text comparison orders it. Tokens are what the provider reports (Anthropic's include cache reads and writes), 0 when it doesn't.
- **`run_stats`:** one row per check run that finished without a run error, never pruned. `at` is the run's finish (fixed-width UTC like `ai_calls.at`). The counts are the dashboard's ([FEATURES §12](FEATURES.md#12-stats)); `releases_behind` sums every update's releases between running and latest in the stored notes (the same `changelog.Service.Changes` as the detail page; 0 for updates without notes). `classes` counts every update per class (`{"patch": n, …}`), `failures` the containers whose latest attempt failed per error kind (`other` when unclassified), both without zero keys; `requests` is `{"registry"|"github"|"gitlab": {requests, limited}}` since the previous row (process-wide, not only this run's; `limited` = 429, or 403 with no quota left). The quota columns are NULL when it was never seen.
- **Tag lists** are cached in memory and in `tag_cache` for 1 h, so restarts don't re-download repos with thousands of tags. Digests are only cached in memory, since HEAD requests are cheap. A manual run skips entries from before its start (a ctx from `registry.FreshSince`), so groups in the same run still share a fetch. Concurrent requests for the same tag list or digest share one request.
- **Registry failures:** a rate-limited tag list (the stored copy, or the error if there's none) is reused for 5 min, so other containers on that repo don't ask the refusing registry again in the same run. A 429 is never retried (go-containerregistry retries other server errors), and digest probing stops at the first rate-limit, timeout, network or registry error.

## HTTP API (v1)

PUT, PATCH, DELETE and POST need the header `X-Tidemark: 1` (403 without; see Security).

```
GET  /api/health                    {status: "ok", version}; `tidemark healthcheck` calls it (Dockerfile HEALTHCHECK)
GET  /api/containers                all containers + their last valid check result + run status; 502 if Docker
                                    can't be reached
GET  /api/containers/{name}         detail by container name (404 if none): container, image digests + OCI labels,
                                    changelog; may wait up to 15 s for a release-note sync
PUT  /api/containers/{name}/review  mark reviewed or snooze: {kind: "reviewed", version} | {kind: "snoozed", until}
                                    | {kind: "snoozed", version}; 200 + the review, 400 bad body, 404 unknown name, 502 Docker unreachable, 501 without a store
DELETE /api/containers/{name}/review   remove it; 204 (also when there was none), 404 unknown name
POST /api/checks                    start a check run unless one is running; always 202 + the current run status
GET  /api/settings                  configuration for the settings dialog (secrets only as set / not set)
PATCH /api/settings                 change editable settings: {name: "value" | null}; 200 + the same body as GET,
                                    403 when not editable, 400 bad body or unknown name, 422 {error, fields: {name: message}},
                                    500 stored but applying failed
POST /api/notifications/test        send a test message; 204, 502 {error} when sending failed, 409 when notifications are off,
                                    501 without a notify service (tests)
POST /api/containers/{name}/summary make the AI summary from the stored notes, extending a base when possible, or in full with
                                    the body {mode: "full"} (unknown fields or modes: 400); waits up to 2 min; 200 + the
                                    detail's `ai` object, 404 unknown name, 409 AI off, 422 nothing to summarise (ignored,
                                    no update or changelog, channel build, no releases in range), 502 {error} the provider
                                    failed (stored too), 501 without an AI service
GET  /api/stats[?range=30d|90d|1y|all]   everything the stats page shows (default 90d; anything else 400; 501 without a store);
                                    with Docker unreachable it still answers, `partial: true`
GET  /api/icons/{ref}[?variant=light|dark]   app icon from the local cache (fetched from the CDN on first use); built-in ones (`tidemark`) from the binary

```

Response shapes (Go structs in `internal/api`, TypeScript types in `web/src/app/dashboard/containers.ts` and `detail/detail-types.ts`):

- **`GET /api/containers`** → `{source, check, containers}`.
  - `source`: `kind` (`docker` | `snapshot`), `location`, `capturedAt` (snapshots).
  - `check`: `running`, `trigger` (`startup` | `schedule` | `manual`), `nextRunAt`, `startedAt`, `finishedAt`, `done`/`total` (images), `error` (the run itself failed), `rulesError` (the config file didn't load).
  - Per container: `id`, `name`, `image` (ref, display, registry, repository, tag, id, created, platform, missing), `state`, `status`, `created`, `stack`, `service`, `icon` (`src` + optional `light`/`dark` URLs), plus:
    - `rules`: only when a rule is set or invalid: values, `from` per rule, `problems`.
    - `review`: only while it applies to the update shown: `kind` (`reviewed` | `snoozed`), `version`, `until`, `at`.
    - `update`: absent while unchecked, stale or ignored. Fields below.
  - `update` fields:
    - Versions: `running`, `source` (`tag` | `label` | `digest` | `env` | `unknown`), `latest`, `latestInMajor`, `class`, `floating`, `newImage`, `held`, `heldBy` (`pin` | `allow-major`).
    - `channel`: the tag, when it's a pre-release channel like `nightly`. `latest` is then the newest stable release and `level` stays `none`.
    - `tagDigest`: the floating tag's current registry digest, when fetched. `reviewKey`: what a review or "snooze until newer" is set against: `latest`, or `tagDigest` for a channel tag or an unknown latest; absent when neither exists.
    - Check state: `checkedAt` (last successful check; absent if none), `error` + `errorKind` + `failedAt` (the latest attempt failed; everything else is the kept result), `tagsFrom` (the tag list was reused from this time because the registry rate-limited it).
    - Verdict: `level` and `reasons` (`[{text, tag?, line?, source?}]`); AI reasons come last with `source: "ai"` and no `line`. Every flagged line is quoted, with its release `tag` and the raw `line`, which equals one of that release's `hits[].line` in the detail response, so the UI can jump to it. The version-jump reason is text only.
    - Without any successful check, `update` is just `error` + `failedAt` with `source: "unknown"`, `class: "none"`, `level: "none"`.
- **`GET /api/containers/{name}`** → `{container, image: {digests, labels}, changelog, history, ai}`.
  - `ai`: absent when AI is off, and for ignored containers, channel builds, updates without a checked result, a changelog or any release in range (unless a summary is stored). `{provider, model, autoMinReleases, releases (in range now), generating, summary?, error?, failedAt?}`; `summary` is `{changes, actions, verdict, reasons: [{text, tag?}], provider, model, releases (sent), omitted, inputTokens, outputTokens, durationMs (0 when unknown), createdAt, stale, extendedFrom?}`; `releases` is the whole range also for an extension.
  - `history`: always an array, newest first: `{at, running, imageId, prevRunning, prevImageId}` (no `prevImageId` on the first entry).
  - `changelog`: absent for ignored containers and when no source repo is found. It holds:
    - `repo`: url, name, and how it was found (`label` | `config` | `image` | `registry` | `known` | `guess`).
    - `kind`: `releases` | `changelog` | `none`.
    - `releases` in range, newest first, each with a `level` and its flagged lines as `hits`: `{line, level, match?, section?}`. `match` is the words that set the level; `section` the flagged heading it inherits from, when that's higher.
    - `partial` (older releases not fetched), `error` (last sync failure), `fetchedAt`.
    - `links`: `compare`, `releases`, `running` (source tree at your version), `inList` (GitHub release list at your version).
- **`GET /api/settings`** → `{version, source, github: {token (bool), rateLimit: {limit, remaining, resetAt} | rateLimitError}, checks: {schedule (absent when off), nextRunAt}, rules: {path, missing, error}, icons, notifications, editable, fields, warnings}`. The quota comes from GitHub's free `/rate_limit` endpoint, cached a minute and updated from the `X-RateLimit-*` headers of every GitHub API response. `rateLimitError` is set when GitHub can't be reached or rejects the token. `notifications` is absent when they're off, otherwise `{notifier: "telegram", level (`all` | `attention` | `breaking`), digest (absent = after each run), publicUrl, last: {at, kind (`updates` | `first` | `test`), items, error}, nextAt}`; `last` is the newest send attempt, absent before the first. `nextAt` is when Tidemark next looks for something to send: the next digest, or after each run the next scheduled check (absent without a schedule). `ai` is absent when AI summaries are off, otherwise `{provider, model (effective), baseUrl (openai only; the OpenAI URL when none is set), effort (effective; absent when none is sent), autoMinReleases, usage: {since (start of this month, server time), calls, failed, extended, inputTokens, outputTokens}}`; `usage` is left out when it can't be read.
  - `editable` (`TIDEMARK_SETTINGS_EDITABLE`), `fields` and `warnings` (stored values that were invalid and skipped; absent when none). `fields` has one entry per editable setting (`schedule`, `githubToken`, `icons`, `iconsUrl`, `telegramBotToken`, `telegramChatId`, `notifyLevel`, `notifyDigest`, `publicUrl`, `aiProvider`, `aiApiKey`, `aiModel`, `aiBaseUrl`, `aiEffort`, `aiAutoMinReleases`): `{value, source (`env` | `stored` | `default`), env (the env var's name)}`. Secret fields (the two tokens and the AI key) have `secret: true` and `set` instead of `value`: a token is never returned (tested).
- **`PATCH /api/settings`** takes a subset of those names; a string sets the value, `null` or `""` removes the stored one (back to the default). Each value goes through the same validation as its env var; a setting whose env var is set can't be changed (field error). Nothing is stored when any field fails. Valid changes are stored in `settings` and applied without a restart: the check cron is replaced (a running check keeps going), the GitHub token swapped (forgetting the old token's rate-limit state), a new icon service started when icons turn on or the URL changes, the notifier rebuilt (a send in progress finishes with the old one), and the AI provider rebuilt (a summary in progress finishes with the old one). The `openai` provider without a model is a field error on `aiModel`.
- **`GET /api/stats`** → `{generatedAt, range, since (absent for all), partial?, behind, upstream, health, ai?, lifetime}` (Go structs in `internal/api/stats.go`, TypeScript in `web/src/app/stats/stats-types.ts`; what each number means: [FEATURES §12](FEATURES.md#12-stats)). "In range" is `at >= now - range`; series are oldest first, arrays never null, durations in ms.
  - `behind`: `containers` (current updates, longest behind first: versions, class, level, review, `releasesBehind`, `behindSince`, `releasesPartial` when the stored notes stop before the running version), `applied` (`total`, `perStack` (`removed` for containers no longer on the host), `perWeek` with zero weeks, keyed by the Monday), `timeToUpdate` (`count`, `medianMs`, and `byClass` / `byLevel`: always patch, minor, major / none, attention, breaking, each `{kind, count, medianMs?}`), `fromMessage` (`{count, medianMs?}`), `now` (the newest `run_stats` row, whatever the range), `tide` (one point per `run_stats` row, with `releasesBehind`; over 180 points, the last row per UTC day, then per week and per month while still over 180), `calendar` (one `{day, checked, upToDate}` per UTC day from the later of the range start and the first `run_stats` row, at most 365 days back, to today; the day's last row with `checked` > 0, `checked` 0 for days without one), `softwareAge` (`count`, `medianMs`, `oldest`: the top 5 current containers by the publish date of their running version's release (`released`, `latest` when up to date); ignores the range).
  - `upstream`: `repos` (per source repo of a current container with notes: releases in range, breaking/attention counts, `lastRelease`, `medianGapDays`, `quiet` when the last release is over 9 months old, `partial` when the range reaches back past its stored notes), `perMonth` (zero months included; "all" at most 24 months back; a repo counts only from the month after its oldest stored release when `repos.has_more`, and months that leave a repo out have `partial`), `biggestJumps` (top 5 applied updates by releases in between, with `class`, `ms` (the time to update or 0) and `partial` like `releasesPartial`).
  - `health`: `runs` (`total`, `failed`, `byTrigger`, `medianMs` and `slowestMs` of runs without an error), `durations` (per run; over 180, the slowest per day, then per week and month), `failing` (now), `failuresByKind`, `requests` + `requestTotals` (`registry`, `github`, `gitlab` and the rate-limited `limitedRegistry`, `limitedNotes` (GitHub + GitLab); `requestsPer` says whether each entry is a day or, over 92 days, a week keyed by its Monday; zeros included), `github` (last-known quota; left out once its reset time has passed).
  - `ai` (absent without any recorded call): `months`, `byModel`, totals (`calls`, `failed`, `manual`, `auto`, `extended`), `medianMs` (successful calls).
  - `lifetime` (ignores the range): `since`, `startedAt` (this process), `containersTracked`, `releasesStored`, `notesWords` (words in all stored notes; counted with the release levels, so bodies are read once per repo fetch), `breakingFlagged`, `fastestUpdate`, `longestBehind`, `longestStreak` (`{runs, from, to}`).
  - Computed per request from the store (check runs and history sorted by time, not id) plus the live container list; release levels are cached per repo until its `repos.fetched_at` changes or a release appears that they don't cover; only then are its bodies read (the rest of the page reads releases without bodies).
- Unknown `/api/*` paths → 404 `{error}`. Icons → 404 when icons are off, the reference is neither built in nor in the selfh.st index, the variant doesn't exist, or the CDN fetch fails.
- Everything else is the SPA: existing files are served directly (content-hashed assets with a one-year `immutable` cache), extensionless paths get `index.html` (client routes, `no-cache`), a missing file with an extension is a 404. A binary built without the frontend answers 503 with a page explaining how to build it.

## Frontend

What the pages show is in [FEATURES §1](FEATURES.md#1-dashboard) and [§6](FEATURES.md#6-container-detail-page); the pieces they're built from in [DESIGN-SYSTEM.md](DESIGN-SYSTEM.md).

- **Layout** (`web/src/app/`): `ui/` holds the design-system pieces, behind the `ui/index.ts` barrel (the charts behind `ui/charts.ts`). Each page has a folder (`dashboard/`, `detail/`, `settings/`, `stats/`, `styleguide/`) with the page at its root and components only that page uses in subfolders (`dashboard/container-tile/`, …). Every component has its own folder with `name.ts` + `name.html` (except `ui/skeleton` and `ui/markdown`, which keep `template: ''`), plus `.css` only when it has styles and `.spec.ts` only when it has tests. A service sits next to its component (`ui/theme-switch/theme.ts`, `ui/toaster/toasts.ts`). Models and helpers are plain files (`dashboard/containers.ts`, `dashboard/format.ts`, `detail/detail-types.ts`).
- **Routes** (`app.routes.ts`, all lazy): `/` dashboard, `/containers/:name` detail, `/stats`, `/styleguide` (design-system showcase; linked from the settings dialog and shipped in production); anything else redirects to `/`.
- **Shell** (`app.html`): `tm-tide-backdrop` behind every page; sticky 64 px header (`h-16`, content `max-w-7xl`) with the logo (links home), the stats link (chart icon), theme switch (stored as `tidemark.theme`) and the settings gear; toaster; footer with Tidemark's version (from `/api/health`) and the selfh.st icons credit (CC BY 4.0).
- **Dashboard** (`dashboard/`):
  - **Data:** polls `/api/containers` every 1.5 s during a run, otherwise every minute. `data` is a `linkedSignal` that keeps the last good response while a refresh fails (the detail page does the same, keyed by container name).
  - **Coming back from a detail page:** the last list and which sections are open live in the root `DashboardState` service, so the page renders at full height at once and the router (`withInMemoryScrolling`) can restore the scroll position. The detail page's "← Containers" calls `location.back()` when it came from `/`.
  - **Grouping:** `groupBySeverity` / `orderStacks` in `containers.ts`. `splitIgnored` and `splitSnoozed` take those containers out first for their own sections. The stack view's collapsed "Up to date" section shares its open state (`currentOpen`) with the severity view's.
  - **Fading:** up-to-date rows and tiles carry `data-current`, and faded ones `data-dimmed` (`dimQuiet`).
  - **Browser storage:** `tidemark.filter`, `tidemark.group`, `tidemark.view` (localStorage) and `tidemark.query` (sessionStorage).
  - **List view:** a `tm-container-table` per section under one sticky `tm-column-bar`.
  - Tooltips are always `[tmTooltip]`, never `title`, except on flagged lines inside `tm-markdown` (rendered HTML, where a directive can't attach).
- **Container detail** (`detail/`): header card with a compact `tm-tide-gauge` and `.tm-severity-frame`; Changes, Details and History tabs (`TM_TABS`); notes via `tm-markdown` in a `tm-clamp`.
  - A quoted reason jumps to its line: switch to Changes, open `details[data-release]`, `TmClamp.reveal` the `[data-flag~=i]` line, scroll, and ring it with `tm-flag-jumped` for 2 s.
  - Review actions sit on the tab row (`detail/review-actions/`: PUT/DELETE, then `reload()`).
  - **AI summary** (`detail/ai-summary/`): POSTs to `…/summary`, then reloads the detail (the level may rise); while `ai.generating` is true (an auto run) it reloads every 4 s.
- **Stats** (`stats/`): one `httpResource` on `/api/stats?range=…`, the last good response kept while another range loads; the range is stored as `tidemark.statsRange` (localStorage). Sections are `tm-section`s with the chart pieces from `ui/charts` (kept out of the `ui` barrel so they stay in the lazy chunks); text helpers in `stats/stats-format.ts`.
- **Settings dialog** (`settings/settings-dialog.ts` + `.html`, a `tm-dialog size="lg" flush` with `.tm-tabs-vertical` pages: General, Checks, Notifications, AI, GitHub; the page is kept between opens): fetches `/api/settings` each time it opens; the Notifications section's **Send test message** posts to `/api/notifications/test` and reloads it.
- Dev: `ng serve` (bound to 127.0.0.1) with a proxy for `/api` → `localhost:8080`.

## Security

- **Docker socket access is root-equivalent**, so Tidemark reaches Docker only through [`tecnativa/docker-socket-proxy`](https://github.com/Tecnativa/docker-socket-proxy) with `CONTAINERS=1 IMAGES=1`, `POST=0` (setup: [README](../README.md#run-it-with-docker); mounting the socket `:ro` doesn't restrict the API). One-click updates ([parked](ROADMAP.md#ideas-parked)) would need `POST=1` and the matching create/start/stop endpoints, as a conscious opt-in (`TIDEMARK_ENABLE_UPDATES`, default off).
- Container inspect is never called and only image `*_VERSION` variables are read (see Check pipeline).
- Secrets (the GitHub and bot tokens, the AI API key) come from env vars or, when settings are editable, from the `settings` table (plain text, like the rest of the data dir, so protect the volume). They're write-only in the UI, never returned by the API and never logged (a settings change logs only the names of the keys). The GitHub token is only sent to api.github.com, the bot token only to api.telegram.org, the AI key only to the configured provider (the Anthropic SDK runs without its environment defaults, so no `ANTHROPIC_*` variable or credential file is picked up). Provider errors are reduced to a short message without the key before they're logged, stored or returned (tested). The bot token is part of Telegram's URL, so send errors are stripped of the URL (and any echo of the token) before they're logged, stored in `notifications.error` or returned (tested).
- No auth in v1, so keep it LAN-only or behind your reverse proxy's auth: anyone who can reach it can start checks, mark updates reviewed or snoozed and send test messages (it never touches containers). That's why settings are read-only unless `TIDEMARK_SETTINGS_EDITABLE=true`: an editable chat ID would let anyone on the network redirect the notifications, and an editable bot token or GitHub token could be swapped for someone else's.
- **Cross-site requests:** API writes (anything but GET/HEAD/OPTIONS under `/api/`) need the header `X-Tidemark: 1`, otherwise 403. A cross-site form or `fetch` can't set it without a CORS preflight, which the server never grants. The SPA adds it in an HTTP interceptor (`csrf-header.interceptor.ts`); scripts calling the API must send it too. Every response carries `X-Frame-Options: DENY`, `Content-Security-Policy: frame-ancestors 'none'` and `X-Content-Type-Options: nosniff`, so other sites can't frame the UI.
- Release notes are untrusted HTML/markdown, so they're sanitised with DOMPurify before rendering. Images in them never load (that would reveal the viewer's IP to whoever hosts them): each one becomes an "Image: alt" link to its URL. For AI summaries they're sent as data in `<release>` delimiters (anything in a body that could close them is escaped), with a system prompt that says never to follow instructions inside them. The answer has to match a JSON schema, is validated and capped (items, length, reason tags must be ones that were sent) and is rendered as plain text, never markdown or HTML.
- **App icons** are fetched by the server only (browsers never contact the CDN), cached under `$TIDEMARK_DATA_DIR/icons`, and only for references listed in the selfh.st index (no arbitrary URLs or paths); built-in icons are embedded and need no request. Served icons get `Content-Security-Policy: default-src 'none'; style-src 'unsafe-inline'; sandbox` and `X-Content-Type-Options: nosniff`, so a malicious icon can't run script on Tidemark's origin. Requests to the CDN do reveal which icons you use; `TIDEMARK_ICONS=off` avoids them.
- Runs as non-root (uid 65532) in a distroless image.

## Deployment and CI

How to run it: [README → Run it with Docker](../README.md#run-it-with-docker), with the full example in `compose.example.yaml`.

CI is `.github/workflows/image.yml`:

- **Pull requests:** vet, gofmt and all tests, then a linux/amd64 image build that isn't pushed.
- **Merges to `main`:** tests only. There's no image between releases.
- **Docs-only changes** (`*.md`, `docs/`, `LICENSE`) skip the workflow, and a newer push to a PR cancels its running build.
- **Release tags** (`v1.2.3`): tests, then an image pushed as `1.2.3`, `1.2`, `1`, `latest` and `sha-<short commit>`, built with `VERSION=1.2.3` (what the footer, the settings dialog and `/api/health` report). The GitHub Release normally exists already (see Releasing); for a bare tag push the run creates one with notes generated from the merged PRs. A pre-release tag (`v1.3.0-rc.1`) only gets its own version and the sha, and its GitHub Release is marked as a pre-release.
- **Single-manifest images:** builds turn off provenance and SBOM, so each image is one manifest carrying its tags and untagged versions can be pruned safely.
- **Cleanup after each release** (`.github/workflows/cleanup.yml`, the release run's last job, or by hand with a dry run as the default): deletes old image versions. Every version with a release tag is kept. Of the rest (sha-only or untagged), the newest 20 stay, and nothing younger than 14 days is deleted.

**Releasing:** SemVer. Major for changes that need action from whoever runs it (config, data, API), minor for features, patch for fixes. The owner decides when to release. Write the notes, then create the release on the merged `main`, which also creates the tag and so starts the release run (it sees the release exists and leaves it alone): `gh release create v1.2.3 --target <main sha> --title v1.2.3 --notes-file notes.md` (add `--prerelease` for `-rc` tags).

**Release notes** are for whoever runs Tidemark, not a PR list: a one-line intro, then **Needs action** (majors only: what to change), **New**, **Improved**, **Fixed**, one plain-language line per change, and the image line (`ghcr.io/bartvs98/tidemark:1.2.3`). Leave out docs, CI, roadmap and refactors. Empty sections are left out.
