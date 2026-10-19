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
| Docker | Own minimal read-only Engine API client (`internal/docker`) | Only 2 GET endpoints needed; avoids the SDK dependency tree; same decoder for live and snapshot mode. Pull/create for one-click updates comes in step 11 |
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
/.github/workflows/image.yml  CI, see Deployment
/.github/workflows/cleanup.yml  deletes old image versions from GHCR, see Deployment
Dockerfile
compose.example.yaml
```

## Check pipeline

One check run (startup, scheduled or manual; when it runs: [FEATURES §7](FEATURES.md#7-checks-failures-and-settings)) covers every container: 6 workers, at most 4 concurrent requests per registry host. Each container gets 2 min, each registry request 60 s. The rules file is re-read at the start of each run.

1. **Inventory**: `GET /containers/json?all=1` (stopped containers too) + `GET /images/{id}/json` once per image → name, image ref, image ID, `RepoDigests`, labels, Compose project/service. From the image environment only `*_VERSION` variables are kept. Container inspect is never called, because it exposes environment variables. An image deleted since the container was created gives the error "the container's image is no longer on this host"; a container whose image was re-tagged after it started (Docker then reports a bare `sha256:…`) falls back to the image's first `RepoTag`. Ignored containers are skipped here, before any registry call.
2. **Resolve running version** ([FEATURES §2](FEATURES.md#2-running-version-resolution)). Uses `RepoDigests` from the image, because the local image ID ≠ the registry manifest digest. Digest matching only probes tags written like the running tag (`16.10-alpine` for `16-alpine`, not `16.10-alpine3.22`), at the most precise level, newest first, at most 30 HEADs.
3. **Latest version** ([FEATURES §3](FEATURES.md#3-latest-version-lookup)): tag lists are cached 1 h per repo (when the registry rate-limits the request and a stored list exists, that list is used and not cached: `registry.StaleError` → `Result.TagsFrom`); for floating tags the tag's manifest is `HEAD`ed to detect "new image, same tag". Containers with the same image and the same rules are checked once. Channel tags (`resolve.Channel`) get `channel` in the API, which skips the verdict.
4. **Changelog prefetch**: an after-check hook (`cmd/tidemark/main.go`), only for images with an update: resolve the source repo and sync its notes into the store (refresh rules: [FEATURES §4](FEATURES.md#4-changelog-between-versions)). GitHub's first page is a conditional request (ETag). The run doesn't slice anything: the API selects running → latest from the stored notes on read. If nothing was prefetched, the detail endpoint syncs on demand.
5. **Analysis**: not part of the run. The API computes flags and level from the stored notes on read (`changelog.Service.Changes` is cached per repo until a sync stores something), so list and detail always agree.
6. **AI** (later, if enabled): summary + verdict, cached per (repo, from, to).
7. **Persist**: upsert a result per container name (a failed check keeps the last good result for the same image ref, image ID and rules, and only sets `error`, `error_kind` + `failed_at`; a later success clears them), record the run (trigger, start/end, images checked, error), and prune results of containers that are gone or ignored now. Recording version changes in a history comes later (step 8).
8. **Notify** (later): compare against the previous run + reviewed/snooze state → send only *new* findings.

## Data model (SQLite)

File: `$TIDEMARK_DATA_DIR/tidemark.db` (WAL mode). Migrations live in `internal/store/migrations/NNNN_name.sql`, are embedded in the binary and applied in order on start (`schema_migrations` records the version).

```
check_runs      id, trigger (startup|schedule|manual), started_at, finished_at, images, error
results         container_name (PK), image_ref, image_id, running, source, latest, latest_in_major,
                class, floating, new_image, held, held_by, rules (fingerprint), error, error_kind, checked_at, failed_at, tags_from, run_id
tag_cache       repository (PK), tags (JSON), fetched_at
releases        repo, tag, name, body, url, published_at, prerelease   (PK repo+tag)
repos           repo (PK), kind (releases|changelog|none), etag, has_more, fetched_at, error
reviews         container_id, reviewed_version, snoozed_until, snoozed_until_version   (later)
history         container_id, from_version, to_version, detected_at, via (manual|oneclick)  (later)
ai_summaries    repo, from_version, to_version, model, summary_md, verdict, reasons_json   (later)
```

- **Results are keyed by container name**, because Compose gives a recreated container a new ID. Each result records the image ref, image ID and rules fingerprint it was checked with; once the container runs a different image (after `pull` + `up`) or its rules change, the stored result is ignored until the next check.
- **`checked_at` is the last successful check** ('' if none), **`failed_at` the failed attempt `error` comes from** ('' if the last attempt worked). Migration `0004_failed_at` moved `checked_at` to `failed_at` on existing error rows. `error_kind` classifies `error` (`rate-limit` | `not-found` | `timeout` | `network` | `registry`, '' if unclassified; `registry.KindOf`); `tags_from` is set when the tag list came from the stored copy because the registry rate-limited the request, '' otherwise (migration `0005`).
- **Tag lists** are cached in memory and in `tag_cache` for 1 h, so restarts don't re-download repositories with thousands of tags. Digests are only cached in memory: HEAD requests are cheap.

## HTTP API (v1)

```
GET  /api/health                    {status: "ok", version}; `tidemark healthcheck` calls it (Dockerfile HEALTHCHECK)
GET  /api/containers                all containers + their last valid check result + run status; 502 if Docker
                                    can't be reached
GET  /api/containers/{name}         detail by container name (404 if none): container, image digests + OCI labels,
                                    changelog; may wait up to 15 s for a release-note sync
POST /api/checks                    start a check run unless one is running; always 202 + the current run status
GET  /api/settings                  read-only configuration for the settings dialog (secrets only as set / not set)
GET  /api/icons/{ref}[?variant=light|dark]   app icon from the local cache (fetched from the CDN on first use)

later:
PUT  /api/containers/{id}/review    { version } | { snoozeUntil } | { snoozeUntilVersion }
POST /api/containers/{id}/summary   generate AI summary
POST /api/containers/{id}/update    gated one-click update (409 + reasons if a gate fails)
```

Response shapes (Go structs in `internal/api`, TypeScript types in `web/src/app/dashboard/containers.ts` and `detail/detail-types.ts`):

- **`GET /api/containers`** → `{source, check, containers}`.
  - `source`: `kind` (`docker` | `snapshot`), `location`, `capturedAt` (snapshots).
  - `check`: `running`, `trigger` (`startup` | `schedule` | `manual`), `nextRunAt`, `startedAt`, `finishedAt`, `done`/`total` (images), `error` (the run itself failed), `rulesError` (the config file didn't load).
  - per container: id, name, `image` (ref, display, registry, repository, tag, id, created, platform, missing), state, status, created, stack, service, `icon` (`src` + optional `light`/`dark` variant URLs), `rules` (only when one is set or invalid: values, `from` per rule, `problems`), `update` (absent while unchecked, stale or ignored): running, source (`tag` | `label` | `digest` | `env` | `unknown`), latest, latestInMajor, class, channel (the tag when it's a pre-release channel like `nightly`: `latest` is then only the newest stable release, and level stays `none`), floating, newImage, held, heldBy (`pin` | `allow-major`), checkedAt (last successful check; absent if none succeeded), error + errorKind (`rate-limit` | `not-found` | `timeout` | `network` | `registry`; absent when unclassified) + failedAt (the latest attempt failed; the other fields are still the kept result), tagsFrom (a successful check whose tag list was reused from this time because the registry rate-limited it), level, reasons (`[{text, tag?, line?}]`: a quoted flagged line carries its release `tag` and the raw `line`, equal to one of that release's `hits[].line` in the detail response, so the UI can jump to it; the version-jump and "and N more" reasons are text only). Without any successful check, `update` is just `error` + `failedAt` with `source: "unknown"`, `class: "none"`, `level: "none"`.
- **`GET /api/containers/{name}`** → `{container, image: {digests, labels}, changelog}`; `changelog` is absent for ignored containers and when no source repo is found. It holds `repo` (url, name, how: `label` | `config` | `image` | `registry` | `known` | `guess`), `kind` (`releases` | `changelog` | `none`), `releases` in range (newest first, each with `level` and flagged lines as `hits`), `partial` (older releases not fetched), `error` (last sync failure), `fetchedAt`, and `links`: `compare`, `releases`, `running` (source tree at your version), `inList` (GitHub release list at your version).
- **`GET /api/settings`** → `{version, source, github: {token (bool), rateLimit: {limit, remaining, resetAt} | rateLimitError}, checks: {schedule (absent when off), nextRunAt}, rules: {path, missing, error}, icons}`. The quota comes from GitHub's `/rate_limit` (free, cached a minute and refreshed from the `X-RateLimit-*` headers of every GitHub API response Tidemark gets; `rateLimitError` when GitHub can't be reached or rejects the token).
- Unknown `/api/*` paths → 404 `{error}`. Icons → 404 when icons are off, the reference isn't in the selfh.st index, the variant doesn't exist, or the CDN fetch fails.
- Everything else is the SPA: existing files are served directly (content-hashed assets with a one-year `immutable` cache), extensionless paths get `index.html` (client routes, `no-cache`), a missing file with an extension is a 404. A binary built without the frontend answers 503 with a page explaining how to build it.

## Frontend

What the pages show is in [FEATURES §1](FEATURES.md#1-dashboard) and [§6](FEATURES.md#6-container-detail-page); the pieces they're built from in [DESIGN-SYSTEM.md](DESIGN-SYSTEM.md).

- **Layout** (`web/src/app/`): `ui/` holds the design-system pieces, behind the `ui/index.ts` barrel. Each page has a folder (`dashboard/`, `detail/`, `settings/`, `styleguide/`) with the page at its root and components only that page uses in subfolders (`dashboard/container-tile/`, …). Every component has its own folder with `name.ts` + `name.html` (except `ui/skeleton` and `ui/markdown`, which keep `template: ''`), plus `.css` only when it has styles and `.spec.ts` only when it has tests. A service sits next to its component (`ui/theme-switch/theme.ts`, `ui/toaster/toasts.ts`). Models and helpers are plain files (`dashboard/containers.ts`, `dashboard/format.ts`, `detail/detail-types.ts`).
- **Routes** (`app.routes.ts`, all lazy): `/` dashboard, `/containers/:name` detail, `/styleguide` (design-system showcase; linked from the settings dialog and shipped in production); anything else redirects to `/`.
- **Shell** (`app.html`): `tm-tide-backdrop` behind every page; sticky 64 px header (`h-16`, content `max-w-7xl`) with the logo (links home), theme switch (stored as `tidemark.theme`) and the settings gear; toaster; footer with Tidemark's version (from `/api/health`) and the selfh.st icons credit (CC BY 4.0).
- **Dashboard** (`dashboard/`): hero with a `tm-tide-gauge` (`hero` size, level = up to date ÷ checked). Browser storage keys: `tidemark.filter`, `tidemark.group`, `tidemark.view` (localStorage) and `tidemark.query` (sessionStorage, the search). Grouping is `groupBySeverity` / `orderStacks` in `containers.ts`. Up-to-date rows/tiles carry `data-current`. While a run is in progress the page polls `/api/containers` every 1.5 s. List view is a `tm-container-table` per section under one sticky `tm-column-bar` (`dashboard/container-table/`, `dashboard/column-bar/`); below 640 px rows stack. Tooltips everywhere are `[tmTooltip]`, never `title`.
- **Container detail** (`detail/`): header card with a compact `tm-tide-gauge` and `.tm-severity-frame`; Changes and Details tabs (`TM_TABS`); notes via `tm-markdown` and `tm-clamp`; a quoted reason jumps to its line, switching to the Changes tab first (`details[data-release]` → `[data-flag~=i]`, `TmClamp.reveal`, class `tm-flag-jumped` for 2 s). *Later:* AI summary panel, review/snooze actions, update button with gate status.
- **Settings dialog** (`settings/settings-dialog.ts` + `.html`, a `tm-dialog`): fetches `/api/settings` each time it opens. *Later:* notification test button.
- Dev: `ng serve` (bound to 127.0.0.1) with a proxy for `/api` → `localhost:8080`.

## Security

- **Docker socket access is root-equivalent**, so Tidemark reaches Docker only through [`tecnativa/docker-socket-proxy`](https://github.com/Tecnativa/docker-socket-proxy) with `CONTAINERS=1 IMAGES=1`, `POST=0` (setup: [README](../README.md#run-it-with-docker); mounting the socket `:ro` doesn't restrict the API). One-click updates (later) will need `POST=1` and the matching create/start/stop endpoints, as a conscious opt-in (`TIDEMARK_ENABLE_UPDATES`, default off).
- Container inspect is never called and only image `*_VERSION` variables are read (see Check pipeline).
- Secrets only come from env vars, are never returned by the API, and are never logged. `TIDEMARK_GITHUB_TOKEN` is only sent to api.github.com.
- No auth in v1, so keep it LAN-only or behind your reverse proxy's auth.
- Release notes are untrusted HTML/markdown, so they're sanitised with DOMPurify before rendering. For the AI summary (later), they're passed as data, not instructions.
- **App icons** are fetched by the server only (browsers never contact the CDN), cached under `$TIDEMARK_DATA_DIR/icons`, and only for references listed in the selfh.st index (no arbitrary URLs or paths). Served icons get `Content-Security-Policy: default-src 'none'; style-src 'unsafe-inline'; sandbox` and `X-Content-Type-Options: nosniff`, so a malicious icon can't run script on Tidemark's origin. Requests to the CDN do reveal which icons you use; `TIDEMARK_ICONS=off` avoids them.
- Runs as non-root (uid 65532) in a distroless image.

## Deployment and CI

How to run it: [README → Run it with Docker](../README.md#run-it-with-docker), with the full example in `compose.example.yaml` (it puts `tidemark.ignore=true` on Tidemark itself, because its own image is private and can't be checked).

CI is `.github/workflows/image.yml`:

- **Pull requests:** vet, gofmt and all tests, then a linux/amd64 image build that isn't pushed.
- **Merges to `main`:** tests only. There's no image between releases.
- **Docs-only changes** (`*.md`, `docs/`, `LICENSE`) skip the workflow, and a newer push to a PR cancels its running build.
- **Release tags** (`v1.2.3`): tests, then an image pushed as `1.2.3`, `1.2`, `1`, `latest` and `sha-<short commit>`, built with `VERSION=1.2.3` (what the footer, the settings dialog and `/api/health` report). Then a GitHub Release with notes generated from the merged PRs. A pre-release tag (`v1.3.0-rc.1`) only gets its own version and the sha, and its GitHub Release is marked as a pre-release.
- **Single-manifest images:** builds turn off provenance and SBOM, so each image is one manifest carrying its tags and untagged versions can be pruned safely.
- **Cleanup after each release** (`.github/workflows/cleanup.yml`, the release run's last job, or by hand with a dry run as the default): deletes old image versions. Every version with a release tag is kept. Of the rest (sha-only or untagged), the newest 20 stay, and nothing younger than 14 days is deleted.

**Releasing:** SemVer. Major for changes that need action from whoever runs it (config, data, API), minor for features, patch for fixes. Tag the merged `main` and push the tag: `git tag -a v1.2.3 -m v1.2.3 origin/main && git push origin v1.2.3`. The owner decides when to release.
