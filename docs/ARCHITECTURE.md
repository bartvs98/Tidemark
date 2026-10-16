# Architecture

## Overview

One Go binary that contains the backend, the scheduler and the compiled Angular frontend (embedded with `embed.FS`). It ships as a single small Docker image and talks to the host's Docker daemon through the socket.

```
┌──────────────────────────── tidemark container ─────────────────────────────┐
│                                                                              │
│   Angular SPA (embedded static files)  ◄──── HTTP :8080 ────►  Browser (LAN)  │
│              │ JSON /api/*                                                   │
│   ┌──────────▼──────────┐                                                    │
│   │      HTTP API        │                                                   │
│   └──────────┬──────────┘                                                    │
│              │                                                               │
│   ┌──────────▼──────────────────────────────────────────────┐               │
│   │                    Check service                         │◄── Scheduler  │
│   │  inventory → resolve version → latest → changelog →      │    (cron)     │
│   │  breaking analysis → (AI summary) → persist → notify     │               │
│   └──┬──────────┬───────────┬────────────┬───────────┬──────┘               │
│      │          │           │            │           │                       │
│   docker     registry    changelog     ai         notify       updater       │
│   (socket)   (OCI v2)    (GitHub/      (opt-in)   (Telegram)   (gated,       │
│      │          │         GitLab)                                opt-in)     │
│      │          │           │                                                │
│   ┌──▼──────────▼───────────▼─────── SQLite (/data/tidemark.db) ──────────┐ │
│   └────────────────────────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────────────────────┘
        │                     │                    │
   /var/run/docker.sock   registries          api.github.com, api.telegram.org,
   (via socket-proxy)     (Hub, GHCR, …)      AI provider
```

## Tech stack

| Layer | Choice | Why |
|---|---|---|
| Language | **Go 1.26+** | Fast, small static binary, low memory, great Docker ecosystem |
| Docker | Own minimal read-only Engine API client (`internal/docker`) | Only 2 GET endpoints needed; avoids the SDK dependency tree; same decoder for live and snapshot mode. Pull/create for one-click updates comes in step 11. |
| Registry | `github.com/google/go-containerregistry` | Handles token auth for Hub/GHCR/Quay, tag listing, manifest HEAD, multi-arch indexes |
| Versions | `github.com/Masterminds/semver/v3` + own variant parser | Lenient semver parsing (`v1.2`, `1.2.3-ls45`) |
| HTTP | stdlib `net/http` (Go 1.22+ pattern routing) | No framework needed |
| DB | SQLite via `modernc.org/sqlite` (pure Go, no CGO) | Zero ops, single file, easy cross-compile |
| Migrations | embedded `.sql` files, applied on start | Simple, no extra tooling |
| Scheduler | `github.com/robfig/cron/v3` | Cron expressions |
| Markdown | Rendered in the frontend | Backend stores raw release notes |
| Config | env vars (`TIDEMARK_*`) + optional `config.yaml` + container labels | Labels keep per-container rules next to the compose file |
| Frontend | **Angular 22**: standalone components, signals, zoneless, `HttpClient` + `httpResource` | Small dashboard; it doubles as Angular practice |
| Accessible widgets | `@angular/aria` (headless tabs, menus, listboxes) | Keyboard and ARIA handled for us; we only style |
| UI styling | Tailwind CSS v4 + own design system (`web/src/app/ui`) | Semantic tokens with `light-dark()`; see [DESIGN-SYSTEM.md](DESIGN-SYSTEM.md) |
| Markdown UI | `marked` + DOMPurify | Render release notes safely |
| Image | Multi-stage: `node` build → `golang` build → `gcr.io/distroless/static` | ~20 MB final image, non-root |

## Repository layout

```
/cmd/tidemark/main.go        entrypoint: config, wiring, start HTTP + scheduler
/internal/
  config/                     env + yaml + label parsing
  docker/                     Source interface: live Engine API client + snapshot file reader
  imageref/                   parse image references (registry, repository, tag, digest)
  icons/                      selfh.st icon index, container → icon matching, cached icon serving
  registry/                   tags, digests, auth per registry
  version/                    parse tags, variants, compare, classify patch/minor/major
  resolve/                    running-version resolution chain
  source/                     image → source repo resolution
  changelog/                  GitHub / GitLab / CHANGELOG.md providers
  analysis/                   breaking-change heuristics
  ai/                         (later) summary provider interface + Anthropic impl
  notify/                     (later) notifier interface + Telegram impl
  updater/                    (later) gated pull/recreate/rollback
  store/                      SQLite repository + migrations
  check/                      orchestrates one full check run
  api/                        HTTP handlers + embedded SPA serving
/web/                         Angular workspace
/docs/                        these documents
/scripts/snapshot.sh          export a host's container/image metadata (no env vars) for development
Dockerfile
compose.example.yaml
```

## Check pipeline

One check run (scheduled or manual), with containers processed concurrently (bounded worker pool, e.g. 4):

1. **Inventory**: `GET /containers/json` + `GET /images/{id}/json` → name, image ref, local digest(s), labels, compose project. Container inspect is never called, because it exposes environment variables.
2. **Resolve running version**: chain from FEATURES §2. Uses `RepoDigests` from the image, because the local image ID ≠ the registry manifest digest.
3. **Latest version**: list tags (cached ~1 h per repo), filter by variant + pre-release rules, pick the highest. For floating tags, also `HEAD` the tag's manifest to detect digest-only updates.
4. **Changelog**: resolve the source repo → fetch releases (cached forever per tag; the list is refreshed per run) → slice the range from running to latest.
5. **Analysis**: heuristic flags per release → overall level.
6. **AI** (later, if enabled): summary + verdict, cached per (repo, from, to).
7. **Persist**: upsert container state and write a check-run record. Detected version changes go into the history.
8. **Notify** (later): compare against previous run + reviewed/snooze state → send only *new* findings.

Failures are per container and per stage: a GitHub rate-limit hit still shows the version info, with "changelog unavailable: rate limited" in the UI.

## Data model (SQLite)

```
containers      id (docker name), image_ref, compose_project, compose_service,
                running_tag, running_digest, running_version, version_source,
                latest_version, latest_digest, latest_in_major, update_class,
                level, source_repo, last_checked_at, error
releases        repo, tag, version, name, body, published_at, url   (PK repo+tag)
tag_cache       image_repo, tags_json, fetched_at
check_runs      id, started_at, finished_at, trigger, summary_json
reviews         container_id, reviewed_version, snoozed_until, snoozed_until_version   (later)
history         container_id, from_version, to_version, detected_at, via (manual|oneclick)  (later)
ai_summaries    repo, from_version, to_version, model, summary_md, verdict, reasons_json   (later)
```

## HTTP API (v1)

```
GET  /api/containers                list with status, grouped info, level
GET  /api/containers/{id}           detail incl. releases in range + flags + links
POST /api/checks                    trigger a check run (returns run id)
GET  /api/checks/latest             status of last/current run
GET  /api/health
GET  /api/icons/{ref}[?variant=light|dark]   app icon from the local cache (fetched from the CDN on first use)

later:
PUT  /api/containers/{id}/review    { version } | { snoozeUntil } | { snoozeUntilVersion }
POST /api/containers/{id}/summary   generate AI summary
POST /api/containers/{id}/update    gated one-click update (409 + reasons if a gate fails)
GET  /api/settings, PUT /api/settings
```

All other paths serve the Angular SPA (with fallback to `index.html`).

## Frontend

- **Dashboard**: stacks as sections; each container is a row with name, running → latest, update class badge, level badge (breaking / attention), and how the version was determined. Filters: updates only, breaking only.
- **Container detail**: version info, compare/repo links, release list (markdown, newest first) with breaking/attention lines highlighted. Later: AI summary panel, review/snooze actions, update button with gate status.
- **Settings** (later): schedule, tokens set / not set, notification test button.
- Dev: `ng serve` with a proxy for `/api` → `localhost:8080`.

## Configuration

| Env var | Default | Purpose |
|---|---|---|
| `TIDEMARK_ADDR` | `:8080` | HTTP listen address |
| `TIDEMARK_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `TIDEMARK_DOCKER_HOST` | `unix:///var/run/docker.sock` | Docker endpoint (e.g. `tcp://socket-proxy:2375`) |
| `TIDEMARK_DOCKER_SNAPSHOT` | none | Read containers from a `scripts/snapshot.sh` file instead of Docker (development) |
| `TIDEMARK_ICONS` | `on` | `off` disables app icons (no requests to the icon CDN) |
| `TIDEMARK_ICONS_URL` | `https://cdn.jsdelivr.net/gh/selfhst/icons` | selfh.st icons base URL, e.g. a self-hosted mirror |
| `TIDEMARK_SCHEDULE` | `0 */6 * * *` | Check schedule (cron) |
| `TIDEMARK_DATA_DIR` | `/data` | SQLite location |
| `TIDEMARK_GITHUB_TOKEN` | — | Fine-grained PAT, public repo read only |
| `TIDEMARK_REGISTRY_AUTH_FILE` | — | Optional docker `config.json` for private registries |
| `TIDEMARK_TELEGRAM_BOT_TOKEN` / `_CHAT_ID` | — | Later |
| `TIDEMARK_AI_PROVIDER` / `_API_KEY` / `_MODEL` | — | Later, opt-in |
| `TIDEMARK_ENABLE_UPDATES` | `false` | Later, enables one-click updates |

## Security

- **Docker socket access is root-equivalent.** Don't mount the socket directly; run [`tecnativa/docker-socket-proxy`](https://github.com/Tecnativa/docker-socket-proxy) next to it:
  - Read-only phase: `CONTAINERS=1 IMAGES=1`, `POST=0`.
  - With one-click updates: also allow `POST=1` (and the matching create/start/stop endpoints). This is a conscious opt-in.
  - Mounting the socket with `:ro` does **not** restrict the API, so it isn't a substitute.
- Secrets only come from env vars, are never returned by the API, and are never logged.
- No auth in v1, so keep it LAN-only or put it behind your reverse proxy's auth.
- Release notes are untrusted HTML/markdown, so they're sanitised with DOMPurify before rendering. For the AI summary, they're passed as data, not instructions.
- **App icons** are fetched by the server only (browsers never contact the CDN), cached under `$TIDEMARK_DATA_DIR/icons`, and only for references listed in the selfh.st index (no arbitrary URLs or paths). Served SVGs get `Content-Security-Policy: sandbox` so a malicious icon can't run script on Tidemark's origin. Requests to the CDN do reveal which icons you use; `TIDEMARK_ICONS=off` avoids them.
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
