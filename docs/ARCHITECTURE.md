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
  icons/                      selfh.st icon index, container → icon matching, cached icon serving, built-in icons (builtin/, embedded)
  version/                    parse tags (prefix, numbers, variant, pre-release, build), compare, classify
  registry/                   tag lists + manifest digests via go-containerregistry, cached, anonymous
  resolve/                    one container: running version, latest in family (within pin / major hold), update class
  rules/                      per-container rules: labels + config.yaml, merged, validated, fingerprinted
  source/                     image → source repo (label, plausible image label, ghcr, known list, guess)
  changelog/                  GitHub / GitLab releases + CHANGELOG.md, ETag sync into the store, range selection
  analysis/                   breaking-change heuristics: per-line flags, section inheritance, verdict + reasons
  review/                     whether a stored review / snooze still applies (Active)
  ai/                         (later) summary provider interface + Anthropic impl
  notify/                     (later) notifier interface + Telegram impl
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
   - Tag lists are cached 1 h per repo. When the registry rate-limits the request and a stored list exists, that list is used but not re-cached (`registry.StaleError` → `Result.TagsFrom`).
   - For floating tags, the tag's manifest is `HEAD`ed to detect a new image under the same tag.
   - Containers with the same image and the same rules are checked once.
   - Channel tags (`resolve.Channel`) get `channel` in the API, which skips the verdict.
4. **Changelog prefetch:** an after-check hook in `cmd/tidemark/main.go`, only for images with an update, capped at 1 min per image. It resolves the source repo and syncs its notes into the store ([FEATURES §4](FEATURES.md#4-changelog-between-versions)). GitHub's first page is a conditional request (ETag), so an unchanged repo costs no quota. The run stores notes but doesn't select a range: the API picks running → latest on read. If nothing was prefetched, the detail endpoint syncs on demand.
5. **Analysis** isn't part of the run. The API computes flags and level from the stored notes on every read (`changelog.Service.Changes` is cached per repo until a sync stores something), so list and detail always agree.
6. **AI** (later, if enabled): summary + verdict, cached per (repo, from, to).
7. **Persist:**
   - Upsert a result per container name. A failed check keeps the last good result (for the same image ref, image ID and rules) and only sets `error`, `error_kind` and `failed_at`; a later success clears them.
   - Record the run (trigger, start/end, images checked, error), and prune results of containers that are gone or now ignored.
   - A successful check whose image ID differs from the container's last `history` row adds a row. The first check adds a "tracking started" row without a previous image. The last row per container is kept in memory, loaded at start.
   - On shutdown (SIGTERM) a running check is cancelled, and the database closes only after the run has written its finish time (up to 30 s).
8. **Notify** (later): compare with the previous run and the reviewed/snoozed state, and send only *new* findings.

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
ai_summaries    repo, from_version, to_version, model, summary_md, verdict, reasons_json   (later)
```

- **Results are keyed by container name,** because Compose gives a recreated container a new ID. Each result records the image ref, image ID and rules fingerprint it was checked with. Once the container runs a different image (after `pull` + `up`) or its rules change, the stored result is ignored until the next check.
- **Result columns that need explaining** ('' means not set):
  - `checked_at`: the last successful check.
  - `failed_at`: when the attempt that produced `error` ran ('' if the last attempt worked).
  - `error_kind`: classifies `error` as `rate-limit`, `not-found`, `timeout`, `network` or `registry` (`registry.KindOf`).
  - `tags_from`: set when the tag list came from `tag_cache` because the registry rate-limited the request.
  - `tag_digest`: the floating tag's current registry digest, when fetched.
- **Reviews and history** are keyed by container name too, and neither is pruned when a container goes away.
  - `reviews.version` is "read up to" (reviewed) or "until newer than" (snoozed by version); it's '' when snoozed by time, and `until` is set instead. It holds the update's `reviewKey`: the latest version, or a `sha256:…` build digest for a channel tag or an unknown latest. `review.Active` decides on read whether a review still applies.
  - `history.prev_image_id` is '' on a container's first row.
- **Tag lists** are cached in memory and in `tag_cache` for 1 h, so restarts don't re-download repos with thousands of tags. Digests are only cached in memory, since HEAD requests are cheap. Concurrent requests for the same tag list or digest share one request.
- **Registry failures:** a rate-limited tag list (the stored copy, or the error if there's none) is reused for 5 min, so other containers on that repo don't ask the refusing registry again in the same run. A 429 is never retried (go-containerregistry retries other server errors), and digest probing stops at the first rate-limit, timeout, network or registry error.

## HTTP API (v1)

PUT, DELETE and POST need the header `X-Tidemark: 1` (403 without; see Security).

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
GET  /api/settings                  read-only configuration for the settings dialog (secrets only as set / not set)
GET  /api/icons/{ref}[?variant=light|dark]   app icon from the local cache (fetched from the CDN on first use); built-in ones (`tidemark`) from the binary

later:
POST /api/containers/{name}/summary   generate AI summary
POST /api/containers/{name}/update    gated one-click update (409 + reasons if a gate fails)
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
    - Verdict: `level` and `reasons` (`[{text, tag?, line?}]`). Every flagged line is quoted, with its release `tag` and the raw `line`, which equals one of that release's `hits[].line` in the detail response, so the UI can jump to it. The version-jump reason is text only.
    - Without any successful check, `update` is just `error` + `failedAt` with `source: "unknown"`, `class: "none"`, `level: "none"`.
- **`GET /api/containers/{name}`** → `{container, image: {digests, labels}, changelog, history}`.
  - `history`: always an array, newest first: `{at, running, imageId, prevRunning, prevImageId}` (no `prevImageId` on the first entry).
  - `changelog`: absent for ignored containers and when no source repo is found. It holds:
    - `repo`: url, name, and how it was found (`label` | `config` | `image` | `registry` | `known` | `guess`).
    - `kind`: `releases` | `changelog` | `none`.
    - `releases` in range, newest first, each with a `level` and its flagged lines as `hits`: `{line, level, match?, section?}`. `match` is the words that set the level; `section` the flagged heading it inherits from, when that's higher.
    - `partial` (older releases not fetched), `error` (last sync failure), `fetchedAt`.
    - `links`: `compare`, `releases`, `running` (source tree at your version), `inList` (GitHub release list at your version).
- **`GET /api/settings`** → `{version, source, github: {token (bool), rateLimit: {limit, remaining, resetAt} | rateLimitError}, checks: {schedule (absent when off), nextRunAt}, rules: {path, missing, error}, icons}`. The quota comes from GitHub's free `/rate_limit` endpoint, cached a minute and updated from the `X-RateLimit-*` headers of every GitHub API response. `rateLimitError` is set when GitHub can't be reached or rejects the token.
- Unknown `/api/*` paths → 404 `{error}`. Icons → 404 when icons are off, the reference is neither built in nor in the selfh.st index, the variant doesn't exist, or the CDN fetch fails.
- Everything else is the SPA: existing files are served directly (content-hashed assets with a one-year `immutable` cache), extensionless paths get `index.html` (client routes, `no-cache`), a missing file with an extension is a 404. A binary built without the frontend answers 503 with a page explaining how to build it.

## Frontend

What the pages show is in [FEATURES §1](FEATURES.md#1-dashboard) and [§6](FEATURES.md#6-container-detail-page); the pieces they're built from in [DESIGN-SYSTEM.md](DESIGN-SYSTEM.md).

- **Layout** (`web/src/app/`): `ui/` holds the design-system pieces, behind the `ui/index.ts` barrel. Each page has a folder (`dashboard/`, `detail/`, `settings/`, `styleguide/`) with the page at its root and components only that page uses in subfolders (`dashboard/container-tile/`, …). Every component has its own folder with `name.ts` + `name.html` (except `ui/skeleton` and `ui/markdown`, which keep `template: ''`), plus `.css` only when it has styles and `.spec.ts` only when it has tests. A service sits next to its component (`ui/theme-switch/theme.ts`, `ui/toaster/toasts.ts`). Models and helpers are plain files (`dashboard/containers.ts`, `dashboard/format.ts`, `detail/detail-types.ts`).
- **Routes** (`app.routes.ts`, all lazy): `/` dashboard, `/containers/:name` detail, `/styleguide` (design-system showcase; linked from the settings dialog and shipped in production); anything else redirects to `/`.
- **Shell** (`app.html`): `tm-tide-backdrop` behind every page; sticky 64 px header (`h-16`, content `max-w-7xl`) with the logo (links home), theme switch (stored as `tidemark.theme`) and the settings gear; toaster; footer with Tidemark's version (from `/api/health`) and the selfh.st icons credit (CC BY 4.0).
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
  - *Later:* AI summary panel, update button with gate status.
- **Settings dialog** (`settings/settings-dialog.ts` + `.html`, a `tm-dialog`): fetches `/api/settings` each time it opens. *Later:* notification test button.
- Dev: `ng serve` (bound to 127.0.0.1) with a proxy for `/api` → `localhost:8080`.

## Security

- **Docker socket access is root-equivalent**, so Tidemark reaches Docker only through [`tecnativa/docker-socket-proxy`](https://github.com/Tecnativa/docker-socket-proxy) with `CONTAINERS=1 IMAGES=1`, `POST=0` (setup: [README](../README.md#run-it-with-docker); mounting the socket `:ro` doesn't restrict the API). One-click updates (later) will need `POST=1` and the matching create/start/stop endpoints, as a conscious opt-in (`TIDEMARK_ENABLE_UPDATES`, default off).
- Container inspect is never called and only image `*_VERSION` variables are read (see Check pipeline).
- Secrets only come from env vars, are never returned by the API, and are never logged. `TIDEMARK_GITHUB_TOKEN` is only sent to api.github.com.
- No auth in v1, so keep it LAN-only or behind your reverse proxy's auth: anyone who can reach it can start checks and mark updates reviewed or snoozed (it never touches containers).
- **Cross-site requests:** API writes (anything but GET/HEAD/OPTIONS under `/api/`) need the header `X-Tidemark: 1`, otherwise 403. A cross-site form or `fetch` can't set it without a CORS preflight, which the server never grants. The SPA adds it in an HTTP interceptor (`csrf-header.interceptor.ts`); scripts calling the API must send it too. Every response carries `X-Frame-Options: DENY`, `Content-Security-Policy: frame-ancestors 'none'` and `X-Content-Type-Options: nosniff`, so other sites can't frame the UI.
- Release notes are untrusted HTML/markdown, so they're sanitised with DOMPurify before rendering. Images in them never load (that would reveal the viewer's IP to whoever hosts them): each one becomes an "Image: alt" link to its URL. For the AI summary (later), they're passed as data, not instructions.
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
