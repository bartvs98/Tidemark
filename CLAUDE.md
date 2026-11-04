# Tidemark: notes for Claude sessions

Self-hosted dashboard that shows, for every container on a Docker host (running or stopped), the running version, the latest version, the changelog in between and whether it's breaking. Built step by step with the owner.

## Doc map

Each fact lives in one doc. Read the section you need, not the whole file.

- [docs/ROADMAP.md](docs/ROADMAP.md): open steps in full and parked ideas by theme. **Continue from the first open step.**
- [docs/FEATURES.md](docs/FEATURES.md): behaviour and decision rules. Read the section for the behaviour you're changing.
- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md): stack, package layout, check pipeline, data model, API shapes, frontend, security, CI and releasing. Read before changing a package, a table or an endpoint.
- [README.md](README.md): for users: install, env vars, rules syntax. The only config reference.
- [docs/DESIGN-SYSTEM.md](docs/DESIGN-SYSTEM.md) (components, tokens) + [docs/BRAND.md](docs/BRAND.md) (colour, type, voice): read before any UI work.

(`docs/IDEAS.md` may exist locally: the owner's git-ignored notes; everything decided from it is in the docs above.)

## Stack in one breath

Go 1.26 single binary (`cmd/tidemark`, `internal/*`) embedding an Angular 22 SPA (`web/`, zoneless, signals, `httpResource`, Tailwind v4, Angular Aria). SQLite via `modernc.org/sqlite` (no CGO). Registry access via go-containerregistry. Own read-only Docker Engine API client.

## Commands

```sh
go test ./...                                   # backend tests
go vet ./... && gofmt -l .                      # must be clean
cd web && npx ng test --watch=false             # frontend tests (Vitest); one file: --include src/app/x/x.spec.ts
cd web && npx ng build                          # builds web/dist/spa, embedded by `go build`
```

Run locally against a snapshot of a real host (`snapshots/home.json`, git-ignored; PowerShell):

```powershell
$env:TIDEMARK_DOCKER_SNAPSHOT = "snapshots/home.json"; go run ./cmd/tidemark   # :8080
cd web; npm start                                                              # :4200, proxies /api
```

## How we work (agreed with the owner)

- **Complex tasks go through the `orchestrator` skill** (planning, contracts, subagents, integration). Simple tasks run inline.
- **The owner's notes** (`docs/IDEAS.md`): check each note against the code, ask the open decisions in one batch, group related notes into one PR, and say in the summary which notes are done. The file is the owner's: don't edit it.
- **UI is built only from design-system pieces** ([DESIGN-SYSTEM.md](docs/DESIGN-SYSTEM.md), including its rules), and each piece and state is on `/styleguide`.
- **Look before you report a UI change:** the **`verify` skill** builds, runs against the snapshot and screenshots. Shoot only what changed.
- **Release only when the owner asks**, with hand-written notes. Only release tags push the image ([ARCHITECTURE → Deployment and CI](docs/ARCHITECTURE.md#deployment-and-ci)); check runs with `gh run list`.

## Done means

Every change, inline or orchestrated:

- All tests green; `go vet` and `gofmt -l .` clean.
- **Docs updated:** remove a finished step from ROADMAP; update FEATURES / ARCHITECTURE / README / DESIGN-SYSTEM as relevant. Docs describe what's built; each fact lives in one doc (link, don't repeat). Plans live in the ROADMAP step (ARCHITECTURE may sketch tables and endpoints, marked "later").
- **Docs-sync check** (roadmap steps, and fixes that change behaviour, config or the API): a read-only Sonnet agent gets the diff, the doc sections touched and the facts changed, checks just those sections plus what they link to (and `/styleguide` for UI) and lists what's wrong or missing. Fix it in the same commit. For small fixes and docs-only PRs, check inline.
- **README images** when the dashboard looks noticeably different (header, hero, list/tile layout, colours): see the `verify` skill.

## Branch → PR

- **Never commit to `main`.** Branch off an up-to-date `main`: `step-14-…`, `fix-…`, `docs-…`. Repo: github.com/bartvs98/Tidemark.
- Commit messages end with `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.
- Push, then `gh pr create`: title like the commit subject; body only what changed, ending with the PR attribution line. How it was verified and what the owner should look at go in the chat summary.
- CI tests and builds the image on the PR without publishing it (`gh pr checks`; docs-only PRs have no checks).
- **The owner reviews and squash-merges; don't merge yourself.** Afterwards: `git switch main && git pull`, delete the local branch.

## Gotchas

- **`ng test --include`** takes a spec path; a folder glob like `src/app/dashboard/**` also matches `.html` files and fails.
- **`ng serve` must bind `127.0.0.1`** (set in `angular.json`); on `[::1]` Firefox on Windows waits ~2 s per connection.
- **GitHub quota:** without `TIDEMARK_GITHUB_TOKEN`, 60 API requests/hour per IP, so don't loop verify runs (the `verify` skill says how to reuse notes). Without notes, levels only reflect version jumps.
- **Migrations are append-only:** never edit an existing `internal/store/migrations/NNNN_*.sql`, even one only on an unmerged branch (the owner runs branches against their own data dir, so it's already applied); add the next number. Test the upgrade with verify `--data` pointing at an older `.verify/<run>/data`.
- **Bundles:** `ui/markdown/markdown` (marked + DOMPurify) is deliberately not in the `ui` barrel; import it directly, from lazy routes only.
- **CSS build (lightningcss):** a declaration with `color-mix(... var())` gets an `@supports` copy of just that declaration; with a `background` shorthand that copy resets `background-size`/`-position`. Use longhands (`background-image`).
- **Angular:** `httpResource.value()` throws in the error state, so go through `hasValue()`. Effects only re-run when a read signal _changes_: depend on objects that change per response, not on a boolean that stays `true` (there's a regression test).
- **Privacy rules (don't break):** never call Docker's `GET /containers/{id}/json` (it exposes env vars; a test enforces this). From image env only `*_VERSION` variables are read. `snapshots/` is git-ignored and must never be committed; don't print non-essential labels from it.
- **Versions:** the label and `*_VERSION` checks in version resolution guard against inherited labels and rebuilt tags; read [FEATURES → Running version resolution](docs/FEATURES.md#2-running-version-resolution) before simplifying them.
- **Snapshot:** may be stale. The owner re-runs the repo's current `scripts/snapshot.sh` on the host and copies it to `snapshots/home.json`. `/api/containers` on the live instance shows what the server decided.
