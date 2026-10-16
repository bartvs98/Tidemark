# Tidemark: notes for Claude sessions

Self-hosted dashboard that shows, per running Docker container, the running version, the latest version, the changelog in between and whether it's breaking. Prototype built step by step with the owner (The owner).

## Read first

1. [docs/ROADMAP.md](docs/ROADMAP.md): what's done (checked, with verification notes) and what's next. **Continue from the first unchecked step.**
2. [docs/FEATURES.md](docs/FEATURES.md): behaviour and rules (version resolution chain, icon matching, update gates).
3. [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md): stack, package layout, data model, API, config, security.
4. [docs/DESIGN-SYSTEM.md](docs/DESIGN-SYSTEM.md) + [docs/BRAND.md](docs/BRAND.md) before any UI work.
5. [docs/IDEAS.md](docs/IDEAS.md): the owner's original ideas and answers.

## Stack in one breath

Go 1.26 single binary (`cmd/tidemark`, `internal/*`) embedding an Angular 22 SPA (`web/`, zoneless, signals, `httpResource`, Tailwind v4, Angular Aria). SQLite via `modernc.org/sqlite` (no CGO). Registry access via go-containerregistry. Own Docker Engine API client (read-only).

## Commands

```sh
go test ./...                                   # backend tests
go vet ./... && gofmt -l .                      # must be clean
cd web && npx ng test --watch=false             # frontend tests (Vitest)
cd web && npx ng build                          # builds web/dist/spa, embedded by `go build`
```

Run locally without Docker, against a snapshot of a real host (PowerShell):

```powershell
$env:TIDEMARK_DOCKER_SNAPSHOT = "snapshots/home.json"; go run ./cmd/tidemark   # :8080
cd web; npm start                                                                # :4200, proxies /api
```

## How we work (agreed with the owner)

- **One roadmap step at a time.** Briefly state the plan, build it, verify it, then summarise. Ask only for decisions that are genuinely the owner's.
- **Each step ends with:** all tests green, docs updated (ROADMAP checkbox + a ✅ verification note, plus FEATURES / ARCHITECTURE / README config tables as relevant), then **commit and push** to `origin main` (github.com/bartvs98/Tidemark, private). Commit messages end with `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. Small fixes between steps get their own commit.
- **UI is built only from design-system pieces** in `web/src/app/ui/`. New piece → add it there, export from `ui/index.ts`, show it on `/styleguide`, document it in DESIGN-SYSTEM.md. Colour means risk only; status always icon + word; versions/tags/digests in mono.
- **Verify UI changes visually** in both themes (and phone width when layout changes): build, run the binary against the snapshot, screenshot with headless Chrome (see below), look at the screenshots before reporting.

## Visual verification recipe

Puppeteer is **not** a project dependency. In the session scratchpad: `npm i puppeteer-core@24.10.0`, then launch with `executablePath: 'C:/Program Files/Google/Chrome/Application/chrome.exe'` (Firefox: `browser: 'firefox'`, `C:/Program Files/Mozilla Firefox/firefox.exe`). Run the server with `TIDEMARK_ADDR=127.0.0.1:18080`, `TIDEMARK_DOCKER_SNAPSHOT=snapshots/home.json` and `TIDEMARK_DATA_DIR=<scratchpad>/data` (keeps `./data` clean). Set the theme/view via `localStorage` keys `tidemark.theme` (`dark|light|system`) and `tidemark.view` (`list|tiles`), then reload.

## Gotchas (learned the hard way)

- **`ng serve` must bind `127.0.0.1`** (set in `angular.json`): bound to `[::1]` only, Firefox on Windows waits ~2 s per connection.
- **Angular:** `httpResource.value()` throws in the error state, so go through `hasValue()`. Effects only re-run when a read signal *changes*: depend on objects that change per response, not on a boolean that stays `true` (this broke polling once; there's a regression test).
- **Privacy rules (don't break):** never call Docker's `GET /containers/{id}/json` (it exposes env vars; a test enforces this). From image env only `*_VERSION` variables are read. `snapshots/` is git-ignored and must never be committed; don't print non-essential labels from it.
- **Versions:** image labels can be inherited from base images (`mongo:8.3` says `24.04`), so a label only counts if the registry has that exact tag. Official images are rebuilt under the same tags, so old digests stop matching (hence the `*_VERSION` fallback).
- Docker isn't installed on the dev machine, so the Docker image build is unverified until step 7 (deploy to the home server). The snapshot may be stale: the owner can re-run `scripts/snapshot.sh` on the server.
