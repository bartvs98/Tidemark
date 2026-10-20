# Roadmap

Each step ends in something runnable and verifiable. Finished steps are summarised in a line or two (the details are in the PRs and the other docs); open steps are written out in full. MVP = steps 0–7 plus the brand and design steps B1–B3.

## Brand & design

- [x] **B1. Name & branding**: name (checked for collisions on GitHub / Docker Hub), logo, palette, typography, voice in [BRAND.md](BRAND.md).
- [x] **B2. Design system**: tokens wired into Tailwind v4 `@theme`, the core components, the status language and `/styleguide` ([DESIGN-SYSTEM.md](DESIGN-SYSTEM.md)).
- [x] **B3. Visual refresh** (between steps 6 and 7): dashboard hero with the tide gauge, the faint tide backdrop, one toolbar row with group by stack or severity, quieter up-to-date containers, `tmTooltip` instead of `title`, the detail header card with a compact gauge and the circling severity frame.
- [x] **B4. Settings dialog** (between steps 7 and 8): a gear opens a read-only settings dialog (`GET /api/settings`, tested never to expose the token), the Styleguide link moved into it, and the version shows in the dialog and the footer.

## MVP

- [x] **0. Scaffolding**: Go module, Angular workspace, dev proxy, embedded SPA, `/api/health`, multi-stage Dockerfile, compose example.
- [x] **1. Container inventory**: `internal/docker` (live socket mode tested against a fake Engine API, plus snapshots from `scripts/snapshot.sh`), `GET /api/containers`, dashboard grouped by Compose stack.
- [x] **2. Versions & registries**: `internal/version`, `internal/registry`, `internal/resolve`; every container shows running → latest, the update class and how the version was found.
- [x] **3. Storage & scheduling**: SQLite + migrations, tag cache, check runs, scheduler, `POST /api/checks`, "Check now"; results survive restarts.
- [x] **4. Changelogs**: source repo resolution, GitHub + GitLab releases, CHANGELOG.md fallback, compare/release links, detail page with rendered notes.
- [x] **5. Breaking-change analysis**: phrase-level heuristics tuned on ~900 real releases, per-release flags, highlighted lines, level badges and filters; also the stacked phone-width list view and "Show all" for very long notes.
- [x] **6. Rules**: ignore / pin / allow-major / prerelease / tag-pattern / repo via labels and config file, held-back versions shown, invalid rules skipped and shown, pre-releases ordered by stage and number.
- [x] **7. Deploy to the home server**: CI image build, runs in a stack behind a reverse proxy with its own socket proxy on an internal network, a named data volume and `tidemark.ignore` on itself. Tuning led to the `*_VERSION` fallback in use and to channel tags (`nightly` is no longer compared with stable releases). Known gap: mongo and postgres have no notes (their GitHub repos have no releases; parked below).

## Between MVP and step 8

From the owner's notes after the MVP. One PR each; the release comes last so v1.0.0 includes the rest.

- [x] **H1. License**: MIT (`LICENSE`, README section, `license` in `web/package.json`; the image's licence label comes from the repo via `docker/metadata-action`). The selfh.st icons keep CC BY 4.0.
- [x] **H2. CI tidy-up**: docs-only changes skip CI; superseded PR runs are cancelled (`concurrency` cancels only on PRs).
- [x] **H3. Angular restructure** (PR #6): one folder per component, also for small pieces, and pages with the same layout ([ARCHITECTURE → Frontend](ARCHITECTURE.md#frontend)); no behaviour or visual change.
- [x] **H4. Visible check failures** (PR #8): failures are classified and the short reason shows inline ("refresh failed · rate limited"), the detail page has a callout, and a rate-limited tag list falls back to the stored one ([FEATURES → failed checks](FEATURES.md#7-checks-failures-and-settings)). Portainer's GHCR 429s weren't caused by paging (the library already asks for 1,000 tags per page); the trigger stays unclear, the fallback makes it harmless.
- [x] **H6. Search, full breaking reasons, quota check** (The owner's second round of notes, lands before the v1.0.0 tag): ✅ A search field in the toolbar filters by name, image or stack (`/` focuses it, kept for the browser tab). The "Read before updating" callout quotes flagged lines in full (they were cut at 140 characters), and each quote's tag jumps to the line in the notes (opens the release, expands it, scrolls, briefly rings the line); reasons are now `{text, tag, line}` in the API. Long flagged notes are capped too (homepage's 3.0.0 notes rendered whole), and "Show all · N flagged lines" says what's hidden. The settings quota was checked: it is GitHub's own `/rate_limit` answer, but was up to a minute stale after a check and, without a token, is shared by everything on the server's IP; it now updates from the `X-RateLimit-*` headers of every GitHub response and the dialog says it's per IP. Checked against the home snapshot in Chrome + Firefox, both themes, 1280 and 390 px, plus the jump on homepage and a search, by script. Built by three agents in parallel (Go, detail page, search).
- [x] **H5. Releases + v1.0.0**: a git tag `v1.2.3` publishes `1.2.3`, `1.2`, `1` and `latest` plus a GitHub Release with generated notes, built with that version; merges to `main` only test (the leaner option, chosen after looking at the free tier: about 3 billed minutes per run against 2,000 a month; GHCR storage is currently free). The owner pulls `:latest` (or `:1`) after a release. Old image versions are cleaned up after each release, not on a schedule, because the owner doesn't develop this full time. Built so far: the pipeline and cleanup ([ARCHITECTURE → Deployment and CI](ARCHITECTURE.md#deployment-and-ci); the cleanup's selection filter was tested locally on sample versions), stale merged branches deleted, and GitHub now deletes a PR's branch on merge. ✅ v1.0.0 tagged on 19 Oct 2026 (after H6): the release run published `1.0.0`, `1.0`, `1` and `latest` (plus `sha-eca434c`) with version 1.0.0 in the image labels, created the GitHub Release with generated notes, and the cleanup ran for real (0 to delete; the dry run before it also found 0).

## After MVP

- [x] **8. Reviewed / snooze state + update history**: on the detail page an update can be marked as reviewed (up to the latest version; it looks new again when a newer one appears) or snoozed (1 day, 1 week, 1 month, or until a newer version), via `PUT`/`DELETE /api/containers/{name}/review`. On the dashboard reviewed updates fade and leave the hero figures, snoozed ones move to a collapsed "Snoozed" section. A History tab shows image changes detected per check ("1.4.2 → 1.6.0", "New image", "Tracking started"). Keyed by container name like the results ([FEATURES §9](FEATURES.md#9-reviewed-snoozed-and-history)). New design-system piece: `tm-menu`.
- [ ] **9. Telegram notifications**
  - Telegram bot (`TIDEMARK_TELEGRAM_BOT_TOKEN` + `TIDEMARK_TELEGRAM_CHAT_ID`). One message per check run with **new** updates only (compared with the previous run, respecting reviewed/snooze state), grouped by stack, breaking ones first, with a link to the dashboard.
  - The notifier is an interface, so ntfy, Discord and webhooks can be added later without touching the rest.
  - Quiet mode: only notify for breaking/attention, or a digest once a day. A test button in the settings dialog.
- [ ] **10. AI changelog summary** (opt-in)
  - Bring your own API key (`TIDEMARK_AI_PROVIDER` / `_API_KEY` / `_MODEL`; Anthropic by default, provider behind an interface). Nothing is sent unless enabled.
  - On demand (button) or automatically for updates with ≥ N releases in between: summarise all release notes between running and latest into a short "what changed / what you need to do" list.
  - The model also returns a structured verdict (`none | attention | breaking`) with reasons. **This verdict can only raise the heuristic level, never lower it.**
  - Summaries are cached per (repo, from, to).
- [ ] **11. Gated one-click update**: pull the new image and recreate the container with identical config, **only when every gate passes**:
  1. The update class is patch or minor (no major, no `0.x` minor bump).
  2. The heuristic level is `none` **and** the AI verdict (if enabled) is `none`.
  3. The container isn't ignored or held by a rule, and the update isn't snoozed.
  4. The container uses a **floating tag** (`latest`, `1`, `1.4`), so that a pull plus a recreate actually picks up the new version. For a pinned tag (`1.4.2`) in a Compose file, a recreate via the API would be undone by the next `docker compose up`, so for those the UI shows the line to change in the compose file instead of a button.

  Safety: disabled by default (`TIDEMARK_ENABLE_UPDATES=false`); turning it on needs socket-proxy write access ([ARCHITECTURE → Security](ARCHITECTURE.md#security)). The old image is kept; after recreating, wait for the healthcheck (or a running state for N seconds) and **roll back** to the old image automatically if it fails. Every update is logged to the update history.
- [ ] **12. Stats** (later; brainstorm with the owner first, builds on step 8's history). Ideas so far:
  - *Behind:* how long each container has been behind (age of the running version vs. the latest), updates per container and stack over time, an up-to-date ratio over time (the tide as a history chart).
  - *Upstream:* release cadence per image/repo, breaking vs. calm releases per repo, biggest version jumps.
  - *Check health:* failures per container and reason, check durations, registry and GitHub requests, GitHub quota used.
  - *Lifetime (for fun):* containers ever tracked, updates applied (running version changed), breaking changes caught, release notes read, check runs, longest-behind container ever, fastest update after a release, longest fully-up-to-date streak, mean time to update, Tidemark's own uptime.

## Ideas parked

- More notifiers (ntfy, Discord, webhook)
- Private registries (`TIDEMARK_REGISTRY_AUTH_FILE`: a docker `config.json`)
- Homepage/Homarr widget-compatible endpoint, Prometheus metrics
- Multi-host via multiple Docker endpoints
- Release-notes links for official images whose GitHub repo has no releases (postgres, mongo, mariadb, mysql, nginx: link to the project's own release notes)
