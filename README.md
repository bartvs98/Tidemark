<img src="docs/brand/tidemark-wordmark.svg" height="40" alt="Tidemark">

**Know what changed before you update.** A self-hosted dashboard that shows, for every running container, the version you run, the latest version, the changelog in between and whether anything in it is breaking.

Status: early prototype. See [docs/ROADMAP.md](docs/ROADMAP.md).

## Docs

- [Features](docs/FEATURES.md)
- [Architecture](docs/ARCHITECTURE.md)
- [Roadmap](docs/ROADMAP.md)
- [Brand](docs/BRAND.md)
- [Design system](docs/DESIGN-SYSTEM.md) (live at `/styleguide`)

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

### Developing without Docker: snapshots

`scripts/snapshot.sh` exports a Docker host's container and image metadata to JSON. It leaves out environment variables, mounts, networks and non-essential labels. Run it on the server, copy the file into `snapshots/` (git-ignored), then:

```sh
TIDEMARK_DOCKER_SNAPSHOT=snapshots/home.json go run ./cmd/tidemark
```

In PowerShell: `$env:TIDEMARK_DOCKER_SNAPSHOT="snapshots/home.json"; go run ./cmd/tidemark`

Tests:

```sh
go test ./...
cd web && npm test -- --watch=false
```

Single binary with the frontend embedded:

```sh
cd web && npm run build && cd ..
go build -o tidemark ./cmd/tidemark
```

## Running with Docker

```sh
cp compose.example.yaml compose.yaml
docker compose up -d --build
```

Then open http://your-server:8080.

## Configuration

| Env var | Default | |
|---|---|---|
| `TIDEMARK_ADDR` | `:8080` | Listen address |
| `TIDEMARK_DATA_DIR` | `./data` (`/data` in Docker) | SQLite database and icon cache |
| `TIDEMARK_SCHEDULE` | `0 */6 * * *` | When to check for updates (cron); `off` to only check on demand |
| `TIDEMARK_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `TIDEMARK_DOCKER_HOST` | `unix:///var/run/docker.sock` | Docker Engine API, or `tcp://socket-proxy:2375` |
| `TIDEMARK_DOCKER_SNAPSHOT` | none | Use a snapshot file instead of Docker (see Development) |
| `TIDEMARK_ICONS` | `on` | `off` disables app icons |
| `TIDEMARK_CONFIG` | `<data dir>/config.yaml` | Optional rules file, re-read on every check |
| `TIDEMARK_GITHUB_TOKEN` | — | Optional. GitHub fine-grained token with public-repository read access; raises the release-notes limit from 60 to 5000 requests/hour |

Per-container labels: `tidemark.icon=<selfh.st reference>` picks an icon, `tidemark.icon=none` hides it.

Rules, as labels or in the config file (`containers:` by name, `images:` by repository, globs allowed; labels win):

| Label | Effect |
|---|---|
| `tidemark.ignore=true` | Don't check this container |
| `tidemark.pin=1.4` | Only report updates within 1.4.x |
| `tidemark.allow-major=false` | Only report updates within the running major |
| `tidemark.prerelease=true` | Include pre-releases |
| `tidemark.tag-pattern=<regex>` | Only tags matching this count as versions |
| `tidemark.repo=github.com/owner/name` | Source repository for release notes |

```yaml
# /data/config.yaml
containers:
  vaultwarden:
    allow-major: false
images:
  postgres:
    pin: "16"
```

Details in [Features → Rules](docs/FEATURES.md#8-rules--mvp).

More options are added per roadmap step; the full list is in [Architecture → Configuration](docs/ARCHITECTURE.md#configuration).

## Credits

App icons by [selfh.st/icons](https://selfh.st/icons/), licensed [CC BY 4.0](https://creativecommons.org/licenses/by/4.0/).
