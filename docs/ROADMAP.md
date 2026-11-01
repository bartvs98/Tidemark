# Roadmap

Open steps, written out in full, and parked ideas. A step is removed when it merges: what's built is in [FEATURES.md](FEATURES.md), how it got there in the merged PRs.

## Open steps

None right now. The last step was 13 (stats), so the next one is **14**.

## Ideas parked

Not planned. An idea becomes a step when the owner picks it.

### Updating

- **Gated one-click update.** Parked because a useful version would also have to update the compose file, which is out of scope for now. It would pull the new image and recreate the container with identical config, **only when every gate passes**:
  1. The update class is patch or minor (no major, no `0.x` minor bump).
  2. The level is `none` (heuristics and, if enabled, the AI verdict).
  3. The container isn't ignored or held by a rule, and the update isn't snoozed.
  4. The container uses a **floating tag** (`latest`, `1`, `1.4`). A recreate of a pinned tag (`1.4.2`) would be undone by the next `docker compose up`, so for those the UI shows the line to change in the compose file instead.

  Safety: off by default (`TIDEMARK_ENABLE_UPDATES=false`); turning it on needs socket-proxy write access ([ARCHITECTURE → Security](ARCHITECTURE.md#security)). Keep the old image, wait for the healthcheck (or N seconds running) and **roll back** automatically on failure. Log every update to the history.

### Sources and notes

- **Forgejo, Gitea and self-hosted GitLab notes.** Only github.com and gitlab.com are supported now, so images whose source is elsewhere get version jumps but no notes or breaking-change analysis. Forgejo and Gitea have a GitHub-like API (`/api/v1/repos/{owner}/{repo}/releases`), behind a host allow-list in config.
- **Notes links for official images** whose GitHub repo has no releases (postgres, mongo, mariadb, mysql, nginx): link to the project's own release notes.
- **Private registries** (`TIDEMARK_REGISTRY_AUTH_FILE`: a docker `config.json`).

### Notifications and integrations

- **More notifiers**: ntfy, Discord, webhook (the notifier is already an interface).
- **Homepage/Homarr widget** endpoint and **Prometheus metrics**.

### Settings and AI

- **Rules in the UI**: per-container rules on the detail page, or editing `config.yaml` in the settings dialog.
- **Cost per reasoning effort**: record each AI call's effort (new migration), count Gemini's thinking tokens (its `completion_tokens` leaves them out, so usage is undercounted today), and show the average tokens per summary for each level under the effort select, with an approximate price for the two default models.

### Registry requests

- **Incremental tag lists for huge repos.** Every tag list is fetched in full: pages of at most 1,000 tags (GHCR ignores a larger `n`), walked one after another through the `Link` header, because the registry API has no filter, only `n` and `last` ("start after this tag"). `ghcr.io/getarcaneapp/arcane` had 6,341 tags over 7 pages in Nov 2026 (mostly `next-*`, `pr-*`, `sha256-*` signatures), so every check of it costs 7 requests, which is where the daily registry rate limits come from. A refused page makes the check fall back to the stored list ([FEATURES §7](FEATURES.md#7-checks-failures-and-settings), retried once at the end of the run).
  - **The idea:** keep the full list as the source of truth, but when a stored list exists, ask only for what came after the running tag, `GET /v2/<repo>/tags/list?n=1000&last=<running tag>`, merge that into the stored list (`tag_cache`) and do the full walk only when the stored list is older than some limit (e.g. 24 h) or the answer looks wrong.
  - **What `last` does, measured on 1 Nov 2026:** GHCR returns tags in its own order, roughly by first creation (`latest` sat at position 33 of 6,341, `v2.17` near the end; a moved tag keeps its place), so `last=v2.17.1` gave the 42 tags created after it, `v2.17.2` included, in one request without a `Link` header; `last=v2.16.1` gave 173. This is undocumented GHCR behaviour. Docker Hub follows the spec, lexical order: `library/redis` with `last=8.2` starts at `8.2-alpine`.
  - **Pitfalls to design for:** lexical order misses `9` → `10` (`10` sorts before `9`), so on spec-order registries the delta can't be trusted for a new major or a longer number; the running version isn't known yet on `latest` (a label only counts if the tag exists, which needs the list, see [FEATURES §2](FEATURES.md#2-running-version-resolution)), so start from the stored list's newest version in the family, not the label; digest probing and the held-back line need older tags, which the stored list still has; a tag that was deleted upstream stays in the merged list until the next full walk; a delta that comes back with a `Link` header or 1,000 tags means "a lot changed": walk in full.
  - **Where:** `registry.Client.Tags` / `fetchTags` (needs a request that takes `last`: go-containerregistry's `remote.List` and `remote.Lister` have no option for it, so a plain `GET` through a registry transport with token auth, `transport.NewWithContext` with pull scope), `tag_cache` (may need the full walk's time next to the merge time: a new migration, never edit an old one), the request counting in `stats.Requests.Wrap` stays as is. Tests: the in-memory registry in `internal/registry/registry_test.go` returns lexical order, so GHCR's order needs an intercepting handler (`hookedRegistry`).
  - **Related, cheaper for one case:** for a floating tag whose digest moved, the new version could be read from that image's own version label (index → platform manifest → config blob, 2–3 requests) instead of from the tag list.

### Scale

- **Several Docker hosts** via multiple endpoints (out of scope for now, see [FEATURES](FEATURES.md#out-of-scope-for-now)).
