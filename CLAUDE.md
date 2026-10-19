# Tidemark: notes for Claude sessions

Self-hosted dashboard that shows, per running Docker container, the running version, the latest version, the changelog in between and whether it's breaking. Prototype built step by step with the owner (The owner).

## Read first

1. [docs/ROADMAP.md](docs/ROADMAP.md): what's done (checked, with verification notes) and what's next. **Continue from the first unchecked step.**
2. [docs/FEATURES.md](docs/FEATURES.md): behaviour and rules (version resolution chain, icon matching, update gates).
3. [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md): stack, package layout, data model, API, config, security.
4. [docs/DESIGN-SYSTEM.md](docs/DESIGN-SYSTEM.md) + [docs/BRAND.md](docs/BRAND.md) before any UI work.

(`docs/IDEAS.md` may exist locally: the owner's personal notes, git-ignored. Everything decided from it is in the docs above.)

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
cd web; npm start                                                              # :4200, proxies /api
```

## How we work (agreed with the owner)

- **One roadmap step at a time.** Briefly state the plan, build it, verify it, then summarise. Ask only for decisions that are genuinely the owner's.
- **Orchestrate steps over subagents** (The owner's default since step 6). Read enough to split the step, ask the owner the product decisions, then fix the contracts (Go signatures, JSON shapes) so parts run in parallel. Brief each agent need-to-know: its own files only, the contract, the relevant gotchas, no commits, no verify runs, and "stop and ask if the contract must change". Answer their questions when they fall within agreed decisions. The main session integrates: re-runs all tests, verifies visually, updates docs, commits.
  - **Small tasks stay inline.** If a task (or part) is quicker to do than to brief, e.g. a few-line fix, one file, or no parallelism to gain, the main session does it itself.
  - **Model per subagent** (set `model` on each spawn): **Opus 5.5** for contract-heavy or tricky parts (Go pipeline, complex Angular state); **Sonnet 5.5** for well-specified parts that follow existing patterns (new `ui/` pieces, tests, styleguide entries) and for the read-only docs-sync check.
- **Each step ends with:** all tests green, docs updated (ROADMAP checkbox + a ✅ verification note, plus FEATURES / ARCHITECTURE / README config tables as relevant), a docs-sync check (next rule) and, when the dashboard looks noticeably different (header, hero, list/tile layout, colours), freshly rendered README images (see Visual verification). Then commit, push the branch and open a PR.
- **Docs stay in sync, every step and every fix.** Docs describe what's built, not what was planned. Before committing, a read-only agent compares the diff against README, FEATURES, ARCHITECTURE (pipeline, data model, API, config, frontend), DESIGN-SYSTEM (+ `/styleguide`), BRAND, ROADMAP (notes on earlier steps that are no longer true) and CLAUDE.md and lists what's now wrong or missing. Fix that in the same commit. Plans for later steps stay, marked "later". (A full audit in Oct 2026 found ~60 drifts, including a documented env var that was never built.)
- **Feature branches + pull requests, never commit to `main` directly.** `main` only changes through merges; images come from release tags (see ARCHITECTURE → Deployment), never from main. Per step or fix: branch off an up-to-date `main` (`step-8-review-state`, `fix-…`, `docs-…`), commit there, push, and open a PR with `gh pr create` (title like the commit subject; body: what changed, how it was verified, anything the owner should look at; ends with the PR attribution line). CI tests and builds the image on the PR without publishing it; check it with `gh pr checks` (docs-only PRs skip CI, so they have no checks). Then hand the owner the link: **The owner reviews and merges (squash)**, so don't merge yourself. Afterwards `git switch main && git pull` and delete the local branch. Small fixes between steps get their own branch and PR. Repo: github.com/bartvs98/Tidemark (private). Commit messages end with `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.
- **UI is built only from design-system pieces**: generic ones in `web/src/app/ui/` (exported from `ui/index.ts`), container composites next to the dashboard (`dashboard/container-update/`, `container-tile/`, …). One folder per component: `name.ts` + `name.html`, `.css` only with styles, `.spec.ts` only with tests. Every piece and state is shown on `/styleguide` and documented in DESIGN-SYSTEM.md. Colour means risk only; status always icon + word; versions/tags/digests in mono.
- **Verify UI changes visually** with the verify skill, in both themes (and phone width when layout changes), and look at the screenshots before reporting.

## Visual verification

Use the **`verify` skill** (`.claude/skills/verify/SKILL.md`), which runs `node scripts/dev/verify.mjs`: it builds, runs the binary against the snapshot and screenshots dashboard + styleguide across browsers, themes, views and widths into `.verify/`, reporting page errors and failed requests. First time: `npm install --prefix scripts/dev`.

To verify rules or other label/config-driven UI without touching the owner's snapshot: copy `snapshots/home.json` to the scratchpad with labels added (don't print its contents), put a `config.yaml` into a copy of an earlier run's data dir, and pass `--snapshot <copy> --data <dir>`. Interactive states (opened sections, other tabs) need a small Puppeteer script against `--serve` (resolve `puppeteer-core` from `scripts/dev`).

The README banner and the split dark/light screenshot in `docs/brand/` come from `node scripts/dev/readme-images.mjs`, run against `--serve --snapshot scripts/dev/demo-snapshot.json` (a made-up host; never the owner's snapshot). Re-render them in the same PR whenever a change makes the dashboard look noticeably different (they went stale once, right after the header changed), and look at the result before committing.

## Gotchas (learned the hard way)

- **Puppeteer + Firefox** sometimes dies on the first page with `DiscardedBrowsingContextError`. That's the driver, not the app: just re-run.
- **`ng serve` must bind `127.0.0.1`** (set in `angular.json`): bound to `[::1]` only, Firefox on Windows waits ~2 s per connection.
- **GitHub quota:** without `TIDEMARK_GITHUB_TOKEN`, GitHub allows 60 API requests/hour per IP. Each verify run starts with an empty data dir and prefetches ~11 repos, so a few runs an hour are fine; don't loop it. When the quota runs low, reuse notes with `--data .verify/<earlier run>/data` (pick a run whose `repos` table has no rate-limit errors; `--serve` reuses `.verify/serve` and overwrites it). Without notes, levels only reflect version jumps.
- **Bundles:** `ui/markdown/markdown` (marked + DOMPurify) is deliberately not in the `ui` barrel; import it directly, from lazy routes only.
- **CSS build (lightningcss):** a declaration using `color-mix(... var())` gets an `@supports` copy containing only that declaration. With a `background` shorthand that copy resets `background-size`/`-position` (this froze the skeleton shimmer for a whole step). Use longhands (`background-image`).
- **Angular:** `httpResource.value()` throws in the error state, so go through `hasValue()`. Effects only re-run when a read signal _changes_: depend on objects that change per response, not on a boolean that stays `true` (this broke polling once; there's a regression test).
- **Privacy rules (don't break):** never call Docker's `GET /containers/{id}/json` (it exposes env vars; a test enforces this). From image env only `*_VERSION` variables are read. `snapshots/` is git-ignored and must never be committed; don't print non-essential labels from it.
- **Versions:** image labels can be inherited from base images (`mongo:8.3` says `24.04`), so a label only counts if the registry has that exact tag. Official images are rebuilt under the same tags, so old digests stop matching (hence the `*_VERSION` fallback).
- **Images:** GitHub Actions builds the image and pushes it to `ghcr.io/bartvs98/tidemark` only for releases.
- **Snapshot:** may be stale. The owner re-runs the repo's current `scripts/snapshot.sh` on the server (an older copy misses the `*_VERSION` variables) and copies it to `snapshots/home.json`. `/api/containers` on the live instance is the quickest way to see what the server decided.
