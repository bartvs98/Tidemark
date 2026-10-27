# Audit (26 Oct 2026)

A read-only audit after step 13. The [first audit](https://github.com/bartvs98/Tidemark/blob/0f610b1/docs/AUDIT.md) (20 Oct, roadmap H8) covered the MVP; this one looks again at the whole repo, with extra attention on what came after it: notifications, editable settings, AI summaries and stats. It looks for bugs, current shortcomings, bottlenecks and how Tidemark behaves on bigger hosts. Style and taste were out of scope.

Baseline: `go test ./...`, `go vet` and `gofmt` are clean, and `govulncheck` finds no reachable vulnerabilities (one in an imported package that Tidemark doesn't call).

**Severity:** must = wrong results, security or burned quota; should = realistic edge cases, resilience, scaling; nice = hardening and housekeeping. Nothing here is a must. Tick a finding when its fix is merged.

## The scaling picture in one paragraph

Check runs scale fine: they group containers by image, share registry requests and cache tag lists. **The read path doesn't.** Every API read rebuilds everything from scratch: a full Docker inventory (one list call plus one image inspect per distinct image, one after another), the rules for every container, and a breaking-change analysis of every release in range for every update. The dashboard does that once a minute per open tab, every 1.5 s while a check runs, and the end of every run does it three more times. On a 20-container host that's invisible. On a host with 150 containers and 100 images, one open dashboard during a check sends about 65 inspects per second to the socket proxy and re-runs the regexes over every pending release note each time. Parts B and D fix this. Everything else is smaller.

## A. Check runs

- [x] **should: a run cut short by shutdown is recorded as a success.** When Tidemark stops during a run (`docker compose up -d` after a new image, a host reboot), `check.Service.run` still calls `finish(ctx, runID, len(order), "")`. The run row gets a finish time and no error. Three effects:
  - At the next start, `LastFinished` is recent, so the startup check is skipped for up to an hour, although most images weren't checked.
  - The stats page counts it as a successful, unusually short run (runs, median and durations).
  - The after-run hook still runs, on a cancelled context, and logs three warnings.

  **Fix:** when `ctx.Err() != nil` after `wg.Wait()`, record the run with an error ("cancelled at shutdown"), and skip the prune and the after-run hook. A test can cancel the context halfway through a fake registry.
- [x] **should: Check now doesn't look again within the hour.** Tag lists and tag digests are cached for 1 h (`registry.defaultTTL`), and a manual run uses the same caches. Clicking **Check now** right after an upstream release, or a second time within the hour, gives the same answer without asking the registry. FEATURES doesn't say so, so it reads as "nothing new".
  - **Fix:** give manual runs a flag that skips the fresh-cache lookup (`lookup`, and the `tag_cache` age check in `fetchTags`) but keeps singleflight, the per-host slots and the rate-limit fallback. Alternatively, cache digests for a shorter time than tag lists (a `HEAD` is cheap and doesn't count towards Docker Hub's pull limit). Either way, document it in FEATURES §7.
- [x] **should: automatic AI summaries run inside the check workers.** The after-check hook syncs notes (up to 1 min) and then waits for the provider (up to 2 min) while holding one of the 6 workers. The first run after turning on auto mode, or a run after a big upstream release day, can take several minutes, with the progress counter ("Checking 14/80") standing still.
  - **Fix:** queue the AI part, `{key, request}`, and work through the queue after the workers finish, with its own concurrency of 1–2. Do it before the after-run hook, so notifications still see the raised level. The run then reports progress for what it actually checks, and the queue can be bounded or skipped when the run is cancelled.

## B. Docker inventory and the API read path

- [x] **should: every read re-inventories Docker.** `docker.Client.Containers` lists all containers, then inspects each distinct image **sequentially**, with no cache. These all call it:

  | Caller | How often |
  |---|---|
  | `GET /api/containers` | every 60 s per open dashboard; every 1.5 s during a run |
  | `GET /api/containers/{name}` (`findContainer`) | every open; every 4 s while an AI summary is generating |
  | `GET /api/stats` | every open and every range change |
  | After-run hook: `AIKeys`, `RunStats`, `Updates` | 3× after every run |
  | `check.Unchecked` | at startup |

  Image inspect results never change for a given image ID (images are content-addressed). Only the container list changes.
  - **Fix, part 1:** cache `apiImage` by image ID in the client. Drop the IDs that no longer appear in a list, so the cache can't grow. A list then costs one request plus one inspect per *new* image.
  - **Fix, part 2:** share one list between concurrent callers (singleflight), with a short TTL of about 2 s. The 1.5 s poll, the detail page and the hook then reuse it. A check run should keep asking for a fresh list.
  - **Fix, part 3:** let the after-run hook take the inventory once and pass it to `AIKeys`, `RunStats` and `Updates`. Today they also compute `updateFor` for every container three times.
  - The snapshot source re-reads and parses its JSON file on every call too, but it's only for development.
- [x] **should: the breaking-change analysis is recomputed on every read.** `assessUpdate` → `analysis.Assess` runs ~30 regexes over every line of every release in range, for every container with an update, on every poll. `changelog.Service.Changes` is cached, but the analysis on top of it isn't. An update that is 80 releases behind re-analyses all 80 bodies every 1.5 s during a run.
  - **Fix:** cache the `analysis.Result` next to the `Changes` entry (same repo + range key, plus the class), and drop it in the same `forget`. The stats page's `releaseLevels` already does this per release; the two could share one cache.
- [x] **nice: rules are re-parsed per container per call.** `check.Service.Rules` → `rules.File.For` parses the container's labels each time, including `regexp.Compile` for a `tag_pattern` label. A single list request calls it more than once per container (`toContainerJSON`, `updateFor`). Memoise it per (container ID, labels, file version).
- [x] **nice: the freshness check reads every release body.** Before deciding that a repo is fresh, `changelog.Service.Sync` calls `covered` and `hasLatest`, and each loads every stored release of the repo, bodies included. That happens on every detail open, every 4 s during AI generation and for every update in every run. Both only need the tags, so a body-less query (like `AllReleases`, per repo) is enough.

## C. Store and growth

- [x] **should: one connection serialises reads behind writes.** `store.Open` sets `SetMaxOpenConns(1)`. That's right for writes, but WAL is turned on precisely so readers don't wait for the writer, and with one connection they still do. During a run, each `SaveResult`, `SaveReleases` (a transaction per page of 100) and `PutAISummary` queues behind a running dashboard or stats read, and the other way round. The stats page, which reads every table in full, is the longest holder.
  - **Fix:** open two pools on the same file: a writer with 1 connection, and a reader with a few connections opened with `_pragma=query_only(1)`. Route the read methods to the reader. Keep the rule that no query runs while another one's rows are open, which the first audit checked.
- [x] **nice: tables and caches only grow.** By design, `history`, `run_stats` and `ai_calls` are never pruned (the stats page builds on them), and they're small. The tables below are pruned by nothing, though, and no screen needs their old rows:
  - `releases`, `repos` and `tag_cache` for repos and images no current container uses. A tag list can be thousands of tags of JSON, and a releases row holds full note bodies.
  - `notified` rows for versions long past, and `notifications`.
  - In memory: the registry client's `tags` and `digests` maps keep expired entries, including whole tag lists. The changelog service's `locks` map and its `Changes` cache keep an entry for every repo and range ever asked for; the stats page asks for every historical update's range.
  - **Done differently:** `notified` and `notifications` stay unpruned, since the stats page and the first message depend on them; pruned release notes keep counting in the lifetime totals ([ARCHITECTURE → Data model](ARCHITECTURE.md#data-model-sqlite)).
  - **Fix:** an after-run step that deletes `releases`, `repos` and `tag_cache` rows unused for 30 days (unused = no current container resolves to them), and `notified` rows whose key is no longer any container's review key. Evict expired map entries when an entry is stored. Check what the stats page reads first: it uses the stored notes of *current* containers' repos only.
- [x] **nice: the stats page is computed from scratch on every request.** It reads `run_stats`, `check_runs`, `history`, `ai_calls`, `releases` and `notified` in full, re-inventories Docker and runs the analysis for every applied update. That's fine today; with an hourly schedule it's about 9,000 runs a year. The answer only changes when a run finishes, a review changes or notes are stored, so cache the response per range behind a generation counter that those three events bump.
- [x] **nice: there's no way to take a backup.** The data dir holds a WAL-mode database. Copying it while Tidemark runs can give an inconsistent copy, and the image has no shell to run `sqlite3` in. Add a `tidemark backup <path>` subcommand (`VACUUM INTO`, like `tidemark healthcheck`, so it works through `docker exec`) and a line in the README.

## D. Frontend

- [x] **should: polling ignores whether the page is visible.** The dashboard reloads every 60 s (1.5 s during a run) in a background tab, a minimised window or a wall display nobody looks at. Each poll is the full inventory and analysis from B, and every extra tab multiplies it.
  - **Fix:** pause the timer while `document.hidden`, and reload once on `visibilitychange` to visible. Do the same for the AI-summary poll on the detail page.
  - Combined with B, a forgotten tab then costs nothing.
- [x] **nice: sanitised HTML is re-parsed after DOMPurify.** `renderMarkdown` sanitises, then `imagesToLinks` and `underlineMatches` each parse the result again with `template.innerHTML`, edit it, and serialise it back. Re-parsing sanitised markup in another context is the pattern behind mutation-XSS bypasses, which DOMPurify's docs warn about. The edits only add text, http(s) links and spans, so the risk is low.
  - **Fix:** sanitise with `RETURN_DOM_FRAGMENT: true`, do both edits on that fragment, and serialise once at the end.
- [x] **nice: no script CSP.** Responses set `frame-ancestors 'none'` but no `script-src`, on a page that renders untrusted release notes. DOMPurify is the only line of defence there.
  - **Fix:** add `script-src 'self'` with a hash for the one inline script in `index.html` (the theme bootstrap). Angular's `security.autoCsp` in `angular.json` generates that hash at build time.

## E. Build, CI and operations

- [ ] **should: the image is amd64 only, and the README doesn't say so.** `image.yml` builds `linux/amd64` for releases too. Raspberry Pis and many ARM NAS boxes are typical homelab hosts, and there the image doesn't run at all.
  - **Fix:** build `linux/amd64,linux/arm64` for release tags. Cross-compile instead of emulating: run the `node` and `golang` stages on `$BUILDPLATFORM` and set `GOARCH=$TARGETARCH` (the build is `CGO_ENABLED=0`). That keeps the build at about the same billed minutes. PR builds can stay amd64.
- [ ] **should: CI doesn't run the race detector.** The code is concurrent in many places:
  - the worker pool, plus the check service's shared maps and status;
  - singleflight fetches in the registry client;
  - settings appliers that swap services live (`atomic.Pointer` for icons; mutex-guarded setups for the notifier and the AI provider);
  - AI generation flags.

  `go test -race ./...` in the `test` job (Linux has CGO) would catch a regression in any of them. Locally on Windows it needs a C compiler, so CI is the place for it.
- [ ] **nice: dependency updates are manual.** There's no Dependabot or Renovate for `go.mod`, `web/package.json`, the GitHub Actions and the Dockerfile's base images. That's ironic for an update checker, and govulncheck only runs when someone remembers to. A weekly, grouped Dependabot config (one PR per ecosystem) is about 30 lines.
- [ ] **nice: the HTTP server has no idle timeout.** Only `ReadHeaderTimeout` is set. Add `IdleTimeout` (e.g. 2 min). A global `WriteTimeout` would cut off the 2-minute summary request, so leave that out or set it per handler with `http.ResponseController`.

## F. Shortcomings worth knowing

These aren't bugs. Most are already [parked in the roadmap](ROADMAP.md#ideas-parked): private registries, several Docker hosts, more notifiers and rules in the UI (a login is [out of scope](FEATURES.md#out-of-scope-for-now)). Two weren't; the first is parked now too:

- **Release notes only come from github.com and gitlab.com** (`source.ParseURL`). Images whose source is on Codeberg, another Forgejo or Gitea, or a self-hosted GitLab get version jumps but no notes, and so no breaking-change analysis. Forgejo and Gitea have a GitHub-like releases API (`/api/v1/repos/{owner}/{repo}/releases`), so they'd be a small addition to `changelog`, with a host allow-list in config.
- **Arm hosts** can't run the image yet (E).

## Checked and found sound

- **Privacy:** still only `/containers/json?all=1` and `/images/{id}/json`, and only `*_VERSION` env variables are kept.
- **Secrets:** never returned by `GET /api/settings`, never logged, and stripped from provider and Telegram errors before they're stored or returned (all tested).
- **Writes:** need `X-Tidemark`. Bodies are size-limited with strict decoders (review 4 KiB, summary 1 KiB, settings).
- **SQLite:**
  - No method runs a query while another one's rows are open, so neither the single writer connection nor the reader pool can deadlock on itself.
  - Migrations are append-only, and run-end and run-stats writes survive a cancelled context.
- **Notifications:** items are only marked as sent after a successful send. Messages are split at Telegram's 4,096-character limit. A digest or notifier change never cuts off a send in progress.
- **AI:**
  - Release notes go in delimiters, with anything that could close them escaped.
  - Answers are validated against a schema and capped.
  - Auto mode backs off 6 h after a failure.
  - Pruning keeps the summaries that are being generated.
- **GitHub quota:** ETags on the first page, the primary and secondary limits respected, and the coverage fix from the first audit still in place.
- **Frontend:**
  - Every `httpResource` goes through `hasValue()`, and lists are tracked by ID.
  - The only `innerHTML` is DOMPurify output, and remote images never load.
  - A failed poll keeps the last good data.

## Suggested order

Each step is one `fix-…` PR, smallest and most useful first.

1. **Check runs (A):** record a cancelled run as cancelled, and let Check now bypass the caches. Small, with clear tests.
2. **Read path (B, first finding, and D, polling):** the image inspect cache, the shared list, one inventory per after-run hook, and pausing in hidden tabs. This is the biggest scaling win.
3. **Analysis cache (B, the other three findings).**
4. **Store (C):** the reader pool, pruning and map eviction. The backup command can go here or stand alone.
5. **CI (E):** `-race`, arm64 releases and Dependabot.
6. **Frontend hardening (D):** a single DOMPurify pass and a script CSP.
7. **Forgejo/Gitea notes (F):** [parked in the roadmap](ROADMAP.md#sources-and-notes).
