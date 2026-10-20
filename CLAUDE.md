# Tidemark: notes for Claude sessions

Self-hosted dashboard that shows, for every container on a Docker host (running or stopped), the running version, the latest version, the changelog in between and whether it's breaking. Built step by step with the owner (The owner).

## Doc map

Each fact lives in one doc. Read the section you need, not the whole file.

- [docs/ROADMAP.md](docs/ROADMAP.md): finished steps (one or two lines each), open steps in full, parked ideas. **Continue from the first unchecked step.**
- [docs/AUDIT.md](docs/AUDIT.md): the 20 Oct 2026 audit's findings with status, worked off as roadmap step H8. Tick a finding there when its fix is merged.
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

Run locally against a snapshot of a real host (PowerShell):

```powershell
$env:TIDEMARK_DOCKER_SNAPSHOT = "snapshots/home.json"; go run ./cmd/tidemark   # :8080
cd web; npm start                                                              # :4200, proxies /api
```

## How we work (agreed with the owner)

- **One roadmap step at a time.** Briefly state the plan, build it, verify it, summarise. Ask only for decisions that are genuinely the owner's, batched up front.
- **Orchestrate steps over subagents.** Read enough to split the step, ask the owner the product decisions, then write the contracts (Go signatures, JSON shapes, schema) to one file in the scratchpad that every brief points to, so parts run in parallel. Brief each agent need-to-know: its own files, the contract, the relevant gotchas, no commits, no verify runs, "stop and ask if the contract must change". Ask for a short report: files changed outside its brief, decisions beyond the brief, anything in scope left undone or failing, open questions, and the test result in one line; not a restatement of the brief or a list of the don'ts it followed (it only fills the main session's context). The main session integrates: re-runs all tests, verifies visually, updates docs, commits.
  - **Small tasks stay inline** when they're quicker to do than to brief (a few-line fix, one file, no parallelism to gain).
  - **Model per subagent** (set `model`): **Opus 5.5** for contract-heavy or tricky parts (Go pipeline, new state logic); **Sonnet 5.5** for parts that follow existing patterns (new `ui/` pieces, tests, styleguide entries) and for the docs-sync check.
- **Each step ends with:** all tests green, docs updated (ROADMAP checkbox + a one- or two-line summary, plus FEATURES / ARCHITECTURE / README as relevant), the docs-sync check and, when the dashboard looks noticeably different (header, hero, list/tile layout, colours), fresh README images. Then commit, push, open a PR.
- **Docs stay in sync, every step and every fix.** Docs describe what's built, not what was planned; each fact lives in one doc (link instead of repeating). Plans for later steps live in the ROADMAP step (ARCHITECTURE may sketch their tables and endpoints, marked "later").
  - **Docs-sync check** (roadmap steps, and fixes that change behaviour, config or the API): a read-only Sonnet agent gets the diff, the list of doc sections touched and the facts changed, checks just those sections plus what they link to (and `/styleguide` for UI), and lists what's wrong or missing. Fix that in the same commit. For small fixes and docs-only PRs, do the check inline.
- **Feature branches + pull requests, never commit to `main`.** Images come from release tags, never from main. Branch off an up-to-date `main` (`step-8-review-state`, `fix-…`, `docs-…`), commit, push, `gh pr create` (title like the commit subject; body: only what changed; ends with the PR attribution line). How it was verified and anything the owner should look at go in the chat summary. CI tests and builds the image on the PR without publishing it (`gh pr checks`; docs-only PRs have no checks). **The owner reviews and merges (squash)**; don't merge yourself. Afterwards `git switch main && git pull` and delete the local branch. Repo: github.com/bartvs98/Tidemark (private). Commit messages end with `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.
- **UI is built only from design-system pieces**: generic ones in `web/src/app/ui/` (exported from `ui/index.ts`), domain composites next to their page (`dashboard/container-update/`, `detail/history/`, …). One folder per component: `name.ts` + `name.html`, `.css` only with styles, `.spec.ts` only with tests. Every piece and state is on `/styleguide` and in DESIGN-SYSTEM.md. Colour means risk only; status always icon + word; versions/tags/digests in mono.

## Visual verification

Use the **`verify` skill** (`node scripts/dev/verify.mjs`): builds, runs the binary against the snapshot, screenshots into `.verify/`, reports page errors and failed requests. First time: `npm install --prefix scripts/dev`. Look at the screenshots before reporting.

- **Shoot only what changed.** Screenshots are the most expensive thing to read. Default: the changed pages at 1280 in both themes, plus 390 when layout changes. Crop to the element when one component changed. Firefox and the full matrix only for CSS or layout-wide changes.
- **States that need data** (rules, reviews, image changes, opened menus, other tabs): write a scenario (`--scenario`, format in [scripts/dev/scenarios/README.md](scripts/dev/scenarios/README.md)): it patches a copy of the snapshot, seeds state through the API and takes named, optionally cropped shots. Commit scenarios worth keeping, on the demo host; for one-off checks against the owner's snapshot keep the file in the scratchpad.
- **README images** (`docs/brand/`) come from `node scripts/dev/readme-images.mjs` against `--serve --snapshot scripts/dev/demo-snapshot.json` (a made-up host, never the owner's snapshot). Re-render them in the same PR when the dashboard looks noticeably different, and look at them before committing.

## Gotchas

- **`ng test --include`** takes a spec path; a folder glob like `src/app/dashboard/**` also matches `.html` files and fails.
- **Puppeteer + Firefox** sometimes dies on the first page with `DiscardedBrowsingContextError`: the driver, not the app; re-run.
- **`ng serve` must bind `127.0.0.1`** (set in `angular.json`); on `[::1]` Firefox on Windows waits ~2 s per connection.
- **GitHub quota:** without `TIDEMARK_GITHUB_TOKEN`, 60 API requests/hour per IP. A verify run with an empty data dir prefetches ~11 repos, so don't loop it; reuse notes with `--data .verify/<earlier run>/data` (a run whose `repos` table has no rate-limit errors). Every run, `--serve` included, writes to a new `.verify/<timestamp>/`. Without notes, levels only reflect version jumps.
- **Bundles:** `ui/markdown/markdown` (marked + DOMPurify) is deliberately not in the `ui` barrel; import it directly, from lazy routes only.
- **CSS build (lightningcss):** a declaration with `color-mix(... var())` gets an `@supports` copy of just that declaration; with a `background` shorthand that copy resets `background-size`/`-position`. Use longhands (`background-image`).
- **Angular:** `httpResource.value()` throws in the error state, so go through `hasValue()`. Effects only re-run when a read signal _changes_: depend on objects that change per response, not on a boolean that stays `true` (there's a regression test).
- **Privacy rules (don't break):** never call Docker's `GET /containers/{id}/json` (it exposes env vars; a test enforces this). From image env only `*_VERSION` variables are read. `snapshots/` is git-ignored and must never be committed; don't print non-essential labels from it.
- **Versions:** image labels can be inherited from base images (`mongo:8.3` says `24.04`), so a label only counts if the registry has that exact tag. Official images are rebuilt under the same tags, so old digests stop matching (hence the `*_VERSION` fallback).
- **Images:** GitHub Actions builds the image and pushes it to `ghcr.io/bartvs98/tidemark` only for releases.
- **Snapshot:** may be stale. The owner re-runs the repo's current `scripts/snapshot.sh` on the server and copies it to `snapshots/home.json`. `/api/containers` on the live instance shows what the server decided.
