<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/brand/banner-dark.png">
  <img src="docs/brand/banner-light.png" alt="Tidemark: know what changed before you update">
</picture>

**Know what changed before you update.** A self-hosted dashboard that shows, for every container on your Docker host, the version you run, the latest version, the changelog in between and whether anything in it is breaking.

- **Real versions, not just tags.** `:latest` and `:16-alpine` are resolved to the actual version via image labels, registry digests or the image's own `*_VERSION` variable, and the UI says how it found out.
- **Same family only.** `1.4-alpine` is compared with other `-alpine` releases, never with `-debian`. Updates are classed as patch / minor / major / new image.
- **Release notes in between,** from GitHub or GitLab releases (or `CHANGELOG.md`), with compare links.
- **Breaking-change flags.** Release notes are scanned for things like "BREAKING", "action required" and removed options. Each update gets a risk level (none / attention / breaking) with the reasons, and the lines that triggered it are highlighted.
- **Reviewed, snoozed, history.** Mark an update as read up to its latest version, or snooze it for a while or until a newer version; a History tab lists the image changes Tidemark has seen per container.
- **Per-container rules** via labels or a config file: ignore, pin to `1.4.x`, no majors, include pre-releases, custom tag pattern, source repo override.
- **Read-only and private.** Tidemark never changes your containers. It talks to Docker through a read-only socket proxy and never reads container environment variables.

![The Tidemark dashboard, dark mode on the left and light mode on the right](docs/brand/screenshot.png)

Status: **early prototype.** The MVP (steps 0–7 and the brand and design steps B1–B3 of the [roadmap](docs/ROADMAP.md)) is done and runs on real images. Only public images are supported.

## Run it with Docker

```sh
cp compose.example.yaml compose.yaml
mkdir -p data && sudo chown 65532:65532 data   # the image runs as a non-root user (uid 65532)
docker compose up -d
```

Images are published to `ghcr.io/bartvs98/tidemark` (linux/amd64) for each release: `latest` is the newest release, `1` and `1.2` follow the newest release in that line, and `1.2.3` is fixed. Releases are listed on the repository's Releases page. While the repository is private, the host needs a one-time `docker login ghcr.io` with a token that has `read:packages`. To build from a checkout instead, replace `image:` with `build: .` and run `docker compose up -d --build`. The compose file also works as a stack in Portainer; the `./data` path is then relative to the stack's directory.

A named volume (`- tidemark-data:/data`) works too and needs no `chown`: Docker fills a new volume from the image, which already gives `/data` to uid 65532. The image has no shell, though, so to use a rules file, mount it separately:

```yaml
    volumes:
      - tidemark-data:/data
      - ./tidemark/config.yaml:/config.yaml:ro
    environment:
      TIDEMARK_CONFIG: /config.yaml
```

Then open http://your-server:8080. Tidemark has no login, so keep it on your LAN or behind a reverse proxy that limits access (the compose file has commented Traefik labels). The first check runs at startup, and then every 6 hours. You can also start one with **Check now**.

The compose file runs [docker-socket-proxy](https://github.com/Tecnativa/docker-socket-proxy) next to Tidemark with only `CONTAINERS=1 IMAGES=1` and `POST=0`. Don't mount the Docker socket into Tidemark directly: socket access is root-equivalent, and mounting it `:ro` doesn't restrict the API.

Recommended: create a [GitHub fine-grained token](https://github.com/settings/personal-access-tokens) with read access to public repositories only, and put it in a `.env` file next to `compose.yaml` as `GITHUB_TOKEN=…`. Without it, GitHub allows 60 requests an hour, which is enough for a small host but runs out quickly with many updates.

### What Tidemark connects to

| Where | Why |
|---|---|
| Docker Engine API (via the socket proxy) | List containers and images (`GET` only; container inspect is never called) |
| Container registries (Docker Hub, GHCR, Quay, lscr.io, …) | Tag lists and manifest digests, anonymously; `HEAD` where possible, which doesn't count towards Docker Hub's pull limit |
| GitHub / GitLab | Release notes and `CHANGELOG.md` for containers with an update; GitHub's remaining API quota when you open Settings (that lookup doesn't count against it) |
| cdn.jsdelivr.net | App icons from [selfh.st/icons](https://selfh.st/icons/) (`TIDEMARK_ICONS=off` to disable, or `TIDEMARK_ICONS_URL` for a mirror) |

Nothing else: no telemetry, no accounts.

## Configuration

Everything is set by environment variables (the Settings dialog only shows them):

| Env var | Default | |
|---|---|---|
| `TIDEMARK_ADDR` | `:8080` | Listen address |
| `TIDEMARK_DATA_DIR` | `./data` (`/data` in Docker) | SQLite database (check results, release notes, reviews, history), icon cache, default location of `config.yaml` |
| `TIDEMARK_CONFIG` | `<data dir>/config.yaml` | Optional rules file, re-read on every check; a missing file means no file rules |
| `TIDEMARK_SCHEDULE` | `0 */6 * * *` | When to check for updates: cron, `@daily` or `@every 3h`, in the server's time zone unless prefixed with `CRON_TZ=Europe/Amsterdam `. `off` to only check on demand |
| `TIDEMARK_GITHUB_TOKEN` | — | Optional, recommended: a fine-grained token with public repositories read-only; raises GitHub's limit from 60 to 5000 requests/hour. Only sent to api.github.com |
| `TIDEMARK_DOCKER_HOST` | `unix:///var/run/docker.sock` | Docker Engine API, e.g. `tcp://socket-proxy:2375` |
| `TIDEMARK_ICONS` | `on` | `off` disables app icons (no requests to the icon CDN) |
| `TIDEMARK_ICONS_URL` | `https://cdn.jsdelivr.net/gh/selfhst/icons` | Icon base URL, e.g. a self-hosted mirror |
| `TIDEMARK_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `TIDEMARK_DOCKER_SNAPSHOT` | — | Read containers from a `scripts/snapshot.sh` file instead of Docker (development, see [Snapshots](#without-docker-snapshots)) |

An invalid `TIDEMARK_SCHEDULE` or `TIDEMARK_LOG_LEVEL` stops startup. `off` (schedule, icons) is case-insensitive. Registry requests are anonymous, so a private or missing image shows "not found, or private".

### Per-container rules

Set them as container labels, or in the config file. Labels win over the file (full precedence and edge cases: [Features → Rules](docs/FEATURES.md#8-rules)).

| Label | Config key | Effect |
|---|---|---|
| `tidemark.ignore=true` | `ignore` | Don't check this container at all: no registry or GitHub calls. It's listed in a collapsed "Ignored" group |
| `tidemark.pin=1.4` | `pin` | Only versions starting with this prefix count as updates (`1.4` → 1.4.x, `16` → 16.x). May start with `v` |
| `tidemark.allow-major=false` | `allow-major` | Only report updates within the running major |
| `tidemark.prerelease=true` | `prerelease` | Pre-release tags count as updates, and their releases show in the changelog |
| `tidemark.tag-pattern=^\d+\.\d+\.\d+-alpine$` | `tag-pattern` | Only registry tags matching this regex count as versions, both for the latest version and for finding the running one |
| `tidemark.repo=github.com/owner/name` | `repo` | Where to get release notes, when Tidemark can't tell or guesses wrong. Only GitHub and GitLab; another host is ignored |

Booleans accept `true/false/yes/no/1/0` in any case. In the config file, `containers:` matches by container name and `images:` by image repository without a tag; both allow `*` and `?` globs:

```yaml
# data/config.yaml
containers:
  vaultwarden:
    allow-major: false
  "immich-*":
    ignore: true
images:
  postgres:            # matches postgres:16-alpine, docker.io/library/postgres:16, …
    pin: "16"
  linuxserver/sonarr:  # no registry: also matches lscr.io/… and ghcr.io/…
    prerelease: true
  ghcr.io/foo/bar:
    repo: github.com/foo/bar-src
```

Edits apply on the next check, without a restart. A version a rule holds back is still shown, quietly ("18.6 held back by pin 16"). Invalid values are skipped and flagged on the dashboard; a file that doesn't load is reported there too, and the last good one stays in use.

Other labels: `tidemark.icon=<selfh.st reference>` picks an icon, `tidemark.icon=none` hides it.

## Development

Requirements: Go 1.26+, Node 24+.

```sh
# backend on :8080
go run ./cmd/tidemark

# frontend on :4200, proxies /api to :8080 (second terminal)
cd web
npm install
npm start
```

Open http://localhost:4200.

### Without Docker: snapshots

`scripts/snapshot.sh` exports a Docker host's container and image metadata to JSON. It leaves out container environment variables, mounts and networks (from the image environment it keeps only `*_VERSION` variables); from labels it keeps Compose, `tidemark.*` and image metadata labels. Run it on the server, copy the file into `snapshots/` (git-ignored), then:

```sh
TIDEMARK_DOCKER_SNAPSHOT=snapshots/home.json go run ./cmd/tidemark
```

In PowerShell: `$env:TIDEMARK_DOCKER_SNAPSHOT="snapshots/home.json"; go run ./cmd/tidemark`. The file is re-read on every request, so a new snapshot needs no restart.

### Tests and checks

```sh
go test ./...
go vet ./... && gofmt -l .
cd web && npx ng test --watch=false
```

`node scripts/dev/verify.mjs` builds everything, runs the binary against a snapshot and screenshots the dashboard and styleguide in Chrome/Firefox, light/dark, desktop/phone (`--help` for options; `npm install --prefix scripts/dev` once).

### Single binary

```sh
cd web && npx ng build && cd ..
go build -o tidemark ./cmd/tidemark    # embeds web/dist/spa
```

## Docs

- [Features](docs/FEATURES.md): behaviour and decision rules
- [Architecture](docs/ARCHITECTURE.md): stack, pipeline, data model, API, frontend, security, CI
- [Roadmap](docs/ROADMAP.md)
- [Brand](docs/BRAND.md) and [design system](docs/DESIGN-SYSTEM.md) (live at `/styleguide`)

## Credits

App icons by [selfh.st/icons](https://selfh.st/icons/), licensed [CC BY 4.0](https://creativecommons.org/licenses/by/4.0/).

## License

[MIT](LICENSE). The app icons (above) keep their own CC BY 4.0 license.
