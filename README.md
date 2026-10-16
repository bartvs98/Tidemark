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
| `TIDEMARK_DATA_DIR` | `./data` (`/data` in Docker) | Database location |
| `TIDEMARK_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `TIDEMARK_DOCKER_HOST` | `unix:///var/run/docker.sock` | Docker Engine API, or `tcp://socket-proxy:2375` |
| `TIDEMARK_DOCKER_SNAPSHOT` | none | Use a snapshot file instead of Docker (see Development) |

More options are added per roadmap step; the full list is in [Architecture → Configuration](docs/ARCHITECTURE.md#configuration).
