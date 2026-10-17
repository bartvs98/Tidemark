# Architecture

## Overview

One Go binary that contains the backend, the scheduler and the compiled Angular frontend (embedded with `embed.FS`). It ships as a single small Docker image and talks to the host's Docker daemon through the socket.

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
│   │  (updates only) → persist → (notify, later)              │               │
│   └──┬──────────┬───────────┬────────────┬───────────┬──────┘               │
│      │          │           │            │           │                       │
│   docker     registry    changelog     ai         notify       updater       │
│   (socket)   (OCI v2)    (GitHub/      (later,    (later,      (later,       │
│      │          │         GitLab)      opt-in)    Telegram)    gated)        │
│      │          │           │                                                │
│   ┌──▼──────────▼───────────▼─────── SQLite (/data/tidemark.db) ──────────┐ │
│   └────────────────────────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────────────────────┘
        │                     │                    │
   /var/run/docker.sock   registries          api.github.com, gitlab.com,
   (via socket-proxy)     (Hub, GHCR, …)      icon CDN; later Telegram, AI
```

## Tech stack

| Layer | Choice | Why |
|---|---|---|
| Language | **Go 1.26+** | Fast, small static binary, low memory, great Docker ecosystem |
| Docker | Own minimal read-only Engine API client (`internal/docker`) | Only 2 GET endpoints needed; avoids the SDK dependency tree; same decoder for live and snapshot mode. Pull/create for one-click updates comes in step 11. |
| Registry | `github.com/google/go-containerregistry` | Handles anonymous token auth for Hub/GHCR/Quay/lscr, tag listing, manifest HEAD, multi-arch indexes |
| Versions | Own lenient parser (`internal/version`) | Real tags aren't semver: `16-alpine`, `v1.15.0-ls175`, `2026.9.0`, `7.4.2-alpine3.21` |
| HTTP | stdlib `net/http` (Go 1.22+ pattern routing) | No framework needed |
| DB | SQLite via `modernc.org/sqlite` (pure Go, no CGO) | Zero ops, single file, easy cross-compile |
| Migrations | embedded `.sql` files, applied on start | Simple, no extra tooling |
| Scheduler | `github.com/robfig/cron/v3` | Cron expressions |
| Markdown | Rendered in the frontend | Backend stores raw release notes |
| Config | env vars (`TIDEMARK_*`) + optional rules `config.yaml` + container labels | Labels keep per-container rules next to the compose file |
| Frontend | **Angular 22**: standalone components, signals, zoneless, `HttpClient` (`withFetch()`) + `httpResource`, route params as component inputs (`withComponentInputBinding`) | Small dashboard; it doubles as Angular practice |
| Accessible widgets | `@angular/aria` (headless tabs; menus/listboxes when needed) | Keyboard and ARIA handled for us; we only style |
| UI styling | Tailwind CSS v4 + own design system (`web/src/app/ui`) | Semantic tokens with `light-dark()`; see [DESIGN-SYSTEM.md](DESIGN-SYSTEM.md) |
| Markdown UI | `marked` + DOMPurify | Render release notes safely |
| Image | Multi-stage: `node` build → `golang` build → `gcr.io/distroless/static` | ~20 MB final image, non-root |

## Repository layout

```
/cmd/tidemark/main.go        entrypoint: config, wiring, start HTTP + scheduler
/internal/
  config/                     TIDEMARK_* environment variables
  docker/                     Source interface: live Engine API client + snapshot file reader
  imageref/                   parse image references (registry, repository, tag, digest)
  icons/                      selfh.st icon index, container → icon matching, cached icon serving
  version/                    parse tags (prefix, numbers, variant, pre-release, build), compare, classify
  registry/                   tag lists + manifest digests via go-containerregistry, cached, anonymous
  resolve/                    one container: running version, latest in family (within pin / major hold), update class
  rules/                      per-container rules: labels + config.yaml, merged, validated, fingerprinted
  source/                     image → source repo (label, plausible image label, ghcr, known list, guess)
  changelog/                  GitHub / GitLab releases + CHANGELOG.md, ETag sync into the store, range selection
  analysis/                   breaking-change heuristics: per-line flags, section inheritance, verdict + reasons
  ai/                         (later) summary provider interface + Anthropic impl
  notify/                     (later) notifier interface + Telegram impl
  updater/                    (later) gated pull/recreate/rollback
  store/                      SQLite repository + migrations
  check/                      background check runs over all containers (skips ignored ones), results in memory + SQLite
  api/                        HTTP handlers + embedded SPA serving
/web/                         Angular workspace
/web/embed.go                 embeds web/dist/spa into the binary
/docs/                        these documents
/scripts/snapshot.sh          export a host's container/image metadata for development (no container env;
                              only image *_VERSION variables)
/scripts/dev/verify.mjs       visual verification: build, run against a snapshot, screenshot (verify skill)
Dockerfile
compose.example.yaml
```

## Check pipeline

One check run (startup, scheduled or manual) covers every container: 6 workers, at most 4 concurrent requests per registry host. Each container gets 2 min, each registry request 60 s.

1. **Inventory**: `GET /containers/json?all=1` (stopped containers too) + `GET /images/{id}/json` once per image → name, image ref, image ID, `RepoDigests`, labels, Compose project/service. From the image environment only `*_VERSION` variables are kept. Container inspect is never called, because it exposes environment variables. An image deleted since the container was created gives the error "the container's image is no longer on this host"; a container whose image was re-tagged after it started (Docker then reports a bare `sha256:…`) falls back to the image's first `RepoTag`. Ignored containers (FEATURES §8) are skipped here, before any registry call.
2. **Resolve running version**: chain from FEATURES §2. Uses `RepoDigests` from the image, because the local image ID ≠ the registry manifest digest. Digest matching only probes tags written like the running tag (`16.10-alpine` for `16-alpine`, not `16.10-alpine3.22`), at the most precise level, newest first, at most 30 HEADs.
3. **Latest version**: list tags (cached 1 h per repo), keep stable tags (pre-releases too with the `prerelease` rule) of the same family (same variant), within any pin / major hold, pick the highest, preferring the running tag's spelling (`v` prefix, raw variant) on ties. For floating tags, also `HEAD` the tag's manifest to detect "new image, same tag" updates. Containers with the same image and the same rules are checked once.
4. **Changelog prefetch**: an after-check hook (`cmd/tidemark/main.go`), only for images with an update: resolve the source repo and sync its notes into the store. Notes are refreshed at most every 6 h, sooner when the running version isn't covered yet; GitHub's first page is a conditional request (ETag), further pages are fetched until the running version is covered (max 5 × 100). The run doesn't slice anything: the API selects running → latest from the stored notes on read. If nothing was prefetched, the detail endpoint syncs on demand (waits at most 15 s).
5. **Analysis**: not part of the run. Heuristic flags per release → overall level, computed by the API from the stored notes on read (`changelog.Service.Changes` is cached per repo until a sync stores something), so list and detail always agree.
6. **AI** (later, if enabled): summary + verdict, cached per (repo, from, to).
7. **Persist**: upsert a result per container name, record the run (trigger, start/end, images checked, error), and prune results of containers that are gone or ignored now. Recording detected version changes in a history comes later (FEATURES §9).
8. **Notify** (later): compare against previous run + reviewed/snooze state → send only *new* findings.

Failures are per container and per stage: when GitHub's rate limit is hit, the version info still shows, and the detail page says the notes couldn't be refreshed and shows what was stored.

## Data model (SQLite)

File: `$TIDEMARK_DATA_DIR/tidemark.db` (WAL mode). Migrations live in `internal/store/migrations/NNNN_name.sql`, are embedded in the binary and applied in order on start (`schema_migrations` records the version).

```
check_runs      id, trigger (startup|schedule|manual), started_at, finished_at, images, error
results         container_name (PK), image_ref, image_id, running, source, latest, latest_in_major,
                class, floating, new_image, held, held_by, rules (fingerprint), error, checked_at, run_id
tag_cache       repository (PK), tags (JSON), fetched_at
releases        repo, tag, name, body, url, published_at, prerelease   (PK repo+tag)
repos           repo (PK), kind (releases|changelog|none), etag, has_more, fetched_at, error
reviews         container_id, reviewed_version, snoozed_until, snoozed_until_version   (later)
history         container_id, from_version, to_version, detected_at, via (manual|oneclick)  (later)
ai_summaries    repo, from_version, to_version, model, summary_md, verdict, reasons_json   (later)
```

- **Results are keyed by container name**, because Compose gives a recreated container a new ID. Each result records the image ref and image ID it was checked against; once the container runs a different image (after `pull` + `up`), or its rules changed (the stored rules fingerprint differs), the stored result is ignored until the next check.
- **Tag lists** are cached in memory and in `tag_cache` for 1 h, so restarts don't re-download repositories with thousands of tags. Digests are only cached in memory: HEAD requests are cheap.

## Scheduling

- `TIDEMARK_SCHEDULE` is a cron spec (`0 */6 * * *` by default, also `@daily`, `@every 3h`; prefix `CRON_TZ=Europe/Amsterdam ` for a time zone other than the server's). `off` (any case) disables scheduled checks; an invalid spec stops startup.
- At startup a check runs if the last one finished more than 1 hour ago, or if any non-ignored container has no valid result (new, recreated, or rules changed); otherwise restarts don't hammer registries.
- The rules file is re-read at the start of each run. Containers share a check only when both image and rules match.
- One run at a time: a scheduled or manual trigger while a run is in progress is ignored.

## HTTP API (v1)

```
GET  /api/health                    {status: "ok", version}; `tidemark healthcheck` calls it (Dockerfile HEALTHCHECK)
GET  /api/containers                all containers + their last valid check result + run status; 502 if Docker
                                    can't be reached
GET  /api/containers/{name}         detail by container name (404 if none): container, image digests + OCI labels,
                                    changelog; may wait up to 15 s for a release-note sync
POST /api/checks                    start a check run unless one is running; always 202 + the current run status
GET  /api/icons/{ref}[?variant=light|dark]   app icon from the local cache (fetched from the CDN on first use)

later:
PUT  /api/containers/{id}/review    { version } | { snoozeUntil } | { snoozeUntilVersion }
POST /api/containers/{id}/summary   generate AI summary
POST /api/containers/{id}/update    gated one-click update (409 + reasons if a gate fails)
GET  /api/settings, PUT /api/settings
```

Response shapes (Go structs in `internal/api`, TypeScript types in `web/src/app/dashboard/containers.ts` and `detail/detail-types.ts`):

- **`GET /api/containers`** → `{source, check, containers}`.
  - `source`: `kind` (`docker` | `snapshot`), `location`, `capturedAt` (snapshots).
  - `check`: `running`, `trigger` (`startup` | `schedule` | `manual`), `nextRunAt`, `startedAt`, `finishedAt`, `done`/`total` (images), `error` (the run itself failed), `rulesError` (the config file didn't load).
  - per container: id, name, `image` (ref, display, registry, repository, tag, id, created, platform, missing), state, status, created, stack, service, `icon` (`src` + optional `light`/`dark` variant URLs), `rules` (only when one is set or invalid: values, `from` per rule, `problems`), `update` (absent while unchecked, stale or ignored): running, source (`tag` | `label` | `digest` | `env` | `unknown`), latest, latestInMajor, class, floating, newImage, held, heldBy (`pin` | `allow-major`), error, checkedAt, level, reasons.
- **`GET /api/containers/{name}`** → `{container, image: {digests, labels}, changelog}`; `changelog` is absent for ignored containers and when no source repo is found. It holds `repo` (url, name, how: `label` | `config` | `image` | `registry` | `known` | `guess`), `kind` (`releases` | `changelog` | `none`), `releases` in range (newest first, each with `level` and flagged lines as `hits`), `partial` (older releases not fetched), `error` (last sync failure), `fetchedAt`, and `links`: `compare`, `releases`, `running` (source tree at your version), `inList` (GitHub release list at your version).
- Unknown `/api/*` paths → 404 `{error}`. Icons → 404 when icons are off, the reference isn't in the selfh.st index, the variant doesn't exist, or the CDN fetch fails.
- Everything else is the SPA: existing files are served directly (content-hashed assets with a one-year `immutable` cache), extensionless paths get `index.html` (client routes, `no-cache`), a missing file with an extension is a 404. A binary built without the frontend answers 503 with a page explaining how to build it.

## Frontend

- **Routes** (`app.routes.ts`, all lazy): `/` dashboard, `/containers/:name` detail, `/styleguide` (design-system showcase; linked in the nav and shipped in production); anything else redirects to `/`.
- **Shell** (`app.html`): sticky header with logo, nav (Dashboard, Styleguide) and theme switch (System / Light / Dark, default dark, stored as `tidemark.theme`); toaster; footer crediting the selfh.st icons (CC BY 4.0).
- **Dashboard** (built): Compose stacks as sections, list and tile views (`tidemark.view`), app icons; each container shows running → latest, how the version was determined, the update class and the risk level (reasons as tooltip). Filter buttons All / Updates / Breaking with counts (`tidemark.filter`); ignored containers only show under All, in a collapsed, muted section at the bottom. Summary line: containers · stacks · not running · updates · checked/next times; a "Snapshot · date" badge in snapshot mode. "Check now" shows progress (`Checking done/total`); while a run is in progress the page polls `/api/containers` every 1.5 s, results fill in (skeletons until then), and a toast says when the run finished or failed. Badges for a non-running state, "image missing", "check failed" (reason as tooltip) and "rule skipped"; a version held back by a rule shows as a quiet line under the version change; a config file that won't load gets a notice at the top. Empty states: no containers, nothing under the filter ("No updates" / "Nothing breaking" + "Show all containers"), and can't list containers (+ "Try again"). Each row/tile is one link to the detail page. List view has a sticky column header bar; below 640px rows stack.
- **Container detail** (built): `/containers/<name>`, page title "`<name>` · Tidemark". Header with version change and badges; links: compare, "`<running>` in releases" (GitHub), "All releases" (or "Changelog" for a CHANGELOG.md source), source at your version, the repo. A callout explains a breaking/attention verdict. **Changes** tab (with the release count): sanitised markdown per release, newest first; the newest and breaking ones open, attention ones collapsed with their badge; flagged lines highlighted; long unflagged notes capped with "Show all". Callouts for: ignored container, notes couldn't be refreshed (shows stored ones), guessed repo (hint to set `tidemark.repo`), no release notes found, running version unknown (latest release only), older releases not loaded, no source repo found, notes not fetched for ignored containers. **Details** tab: Rules card (each rule, where it came from, skipped ones), then image, running version + how it was found, latest (+ held back), checked time, source repo + how it was found, Compose project/service, status, platform, image build time, image ID, digests, OCI labels. A 404 ("No container named …") and an error state with retry.
- **Detail additions** (later): AI summary panel, review/snooze actions, update button with gate status.
- **Settings** (later): schedule, tokens set / not set, notification test button.
- Dev: `ng serve` (bound to 127.0.0.1) with a proxy for `/api` → `localhost:8080`.

## Configuration

| Env var | Default | Purpose |
|---|---|---|
| `TIDEMARK_ADDR` | `:8080` | HTTP listen address |
| `TIDEMARK_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `TIDEMARK_DOCKER_HOST` | `unix:///var/run/docker.sock` | Docker endpoint (e.g. `tcp://socket-proxy:2375`) |
| `TIDEMARK_DOCKER_SNAPSHOT` | none | Read containers from a `scripts/snapshot.sh` file instead of Docker (development); re-read on every request, so a new snapshot needs no restart |
| `TIDEMARK_ICONS` | `on` | `off` disables app icons (no requests to the icon CDN) |
| `TIDEMARK_ICONS_URL` | `https://cdn.jsdelivr.net/gh/selfhst/icons` | selfh.st icons base URL, e.g. a self-hosted mirror |
| `TIDEMARK_SCHEDULE` | `0 */6 * * *` | Check schedule (cron), `off` to only check on demand |
| `TIDEMARK_DATA_DIR` | `./data` (`/data` in the Docker image) | SQLite database, icon cache, default location of `config.yaml` |
| `TIDEMARK_GITHUB_TOKEN` | — | Fine-grained PAT, public repositories read-only; raises GitHub's limit from 60 to 5000 requests/hour |
| `TIDEMARK_CONFIG` | `<TIDEMARK_DATA_DIR>/config.yaml` | Optional rules file (FEATURES §8); missing = no file rules |
| `TIDEMARK_REGISTRY_AUTH_FILE` | — | Later: docker `config.json` for private registries |
| `TIDEMARK_TELEGRAM_BOT_TOKEN` / `_CHAT_ID` | — | Later |
| `TIDEMARK_AI_PROVIDER` / `_API_KEY` / `_MODEL` | — | Later, opt-in |
| `TIDEMARK_ENABLE_UPDATES` | `false` | Later, enables one-click updates |

Only public images are supported; registry requests are anonymous (a private or missing image gives "not found, or private"). An invalid `TIDEMARK_SCHEDULE` or `TIDEMARK_LOG_LEVEL` stops startup; `off` (schedule, icons) is case-insensitive. `TIDEMARK_GITHUB_TOKEN` is only sent to api.github.com.

## Security

- **Docker socket access is root-equivalent.** Don't mount the socket directly; run [`tecnativa/docker-socket-proxy`](https://github.com/Tecnativa/docker-socket-proxy) next to it:
  - Read-only phase: `CONTAINERS=1 IMAGES=1`, `POST=0`.
  - With one-click updates: also allow `POST=1` (and the matching create/start/stop endpoints). This is a conscious opt-in.
  - Mounting the socket with `:ro` does **not** restrict the API, so it isn't a substitute.
- Secrets only come from env vars, are never returned by the API, and are never logged.
- No auth in v1, so keep it LAN-only or put it behind your reverse proxy's auth.
- Release notes are untrusted HTML/markdown, so they're sanitised with DOMPurify before rendering. For the AI summary, they're passed as data, not instructions.
- **App icons** are fetched by the server only (browsers never contact the CDN), cached under `$TIDEMARK_DATA_DIR/icons`, and only for references listed in the selfh.st index (no arbitrary URLs or paths). Served icons get `Content-Security-Policy: default-src 'none'; style-src 'unsafe-inline'; sandbox` and `X-Content-Type-Options: nosniff`, so a malicious icon can't run script on Tidemark's origin. Requests to the CDN do reveal which icons you use; `TIDEMARK_ICONS=off` avoids them.
- Runs as non-root in a distroless image.

## Deployment

```yaml
services:
  socket-proxy:
    image: tecnativa/docker-socket-proxy
    environment: { CONTAINERS: 1, IMAGES: 1, POST: 0 }
    volumes: [ "/var/run/docker.sock:/var/run/docker.sock:ro" ]
  tidemark:
    image: tidemark:latest
    environment:
      TIDEMARK_DOCKER_HOST: tcp://socket-proxy:2375
      TIDEMARK_GITHUB_TOKEN: ${GITHUB_TOKEN}
    volumes: [ "./data:/data" ]
    ports: [ "8080:8080" ]
```
