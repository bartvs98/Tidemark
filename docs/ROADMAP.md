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
- [x] **7. Deploy to the home server**: CI image build; runs behind a reverse proxy with its own socket proxy on an internal network and a named data volume (the setup in `compose.example.yaml`). Tuning led to the `*_VERSION` fallback and to channel tags. Known gap: mongo and postgres have no notes (their GitHub repos have no releases; parked below).

## Between MVP and step 8

From the owner's notes after the MVP. One PR each; the release comes last so v1.0.0 includes the rest.

- [x] **H1. License**: MIT (`LICENSE`, README section, `license` in `web/package.json`; the image's licence label comes from the repo via `docker/metadata-action`). The selfh.st icons keep CC BY 4.0.
- [x] **H2. CI tidy-up**: docs-only changes skip CI; superseded PR runs are cancelled (`concurrency` cancels only on PRs).
- [x] **H3. Angular restructure** (PR #6): one folder per component, also for small pieces, and pages with the same layout ([ARCHITECTURE → Frontend](ARCHITECTURE.md#frontend)); no behaviour or visual change.
- [x] **H4. Visible check failures** (PR #8): failures are classified and the short reason shows inline, with a callout on the detail page; a rate-limited tag list falls back to the stored one ([FEATURES §7](FEATURES.md#7-checks-failures-and-settings)). The GHCR 429s seen on one image weren't caused by paging (the library already asks for 1,000 tags per page); the cause is unknown, and the fallback makes it harmless.
- [x] **H5. Releases + v1.0.0**: release tags publish the image and a GitHub Release; merges to `main` only test (about 3 billed minutes per run against 2,000 free a month), and old image versions are cleaned up after each release ([ARCHITECTURE → Deployment and CI](ARCHITECTURE.md#deployment-and-ci)). v1.0.0 released on 19 Oct 2026.
- [x] **H6. Search, full breaking reasons, quota check**: a toolbar search; the callout quotes flagged lines in full and each quote jumps to its line (reasons became `{text, tag, line}`); long flagged notes are capped with "Show all · N flagged lines". The settings quota now updates from every GitHub response and says it's per IP without a token.
- [x] **H7. Third round of notes**: Tidemark's own icon is built in (images named `…/tidemark`, also before the icon index loads) and Docker socket proxies get selfh.st's Docker icon. Tooltips open with a tap on touch screens, one at a time. In the stack view, up-to-date containers in stacks with updates fade, and fully settled stacks move into a collapsed "Up to date · N stacks" section (new `level` input on `tm-section`). The README screenshot is dark mode only.
- [x] **H8. Audit fixes**: the findings of the 20 Oct 2026 audit (pipeline, backend, frontend), fixed in six PRs: changelog quota (#16), Docker inventory (#17), dashboard refresh (#18), version resolution and build reviews (#20), hardening (#21), registry resilience (#22).
- [x] **H9. Fourth round of notes**: Back to the dashboard keeps the scroll position and open sections; loading skeletons match the page again; flagged lines no longer overlap; the detail callout quotes every flagged line (in a clamp); each flagged line says what triggered it (dotted underline + hover), and "when it is removed" no longer counts.
- [x] **H10. Fifth round of notes**: "Up to date" opens by itself when it's all the dashboard shows; segmented counts sit on the label's baseline; the stats page splits rate-limited requests into registry and release notes; `TZ` is documented (the schedule follows it across daylight saving).

## After MVP

- [x] **8. Reviewed / snooze state + update history**: mark an update reviewed or snooze it from the detail page; reviewed updates fade on the dashboard, snoozed ones move to a collapsed section. A History tab lists detected image changes ([FEATURES §9](FEATURES.md#9-reviewed-snoozed-and-history)). New design-system piece: `tm-menu`.
- [x] **9. Telegram notifications**: one message per check run with new updates only (sent once per version, again when the level rises or a time snooze ends; the first message lists everything pending), grouped by stack, breaking first, linked with `TIDEMARK_PUBLIC_URL`. Quiet mode via a minimum level and a digest cron; test button in the settings dialog. The notifier is an interface for ntfy, Discord or webhooks later ([FEATURES §10](FEATURES.md#10-notifications)).
- [x] **9a. Settings pages**: the settings dialog got pages behind side tabs (a tab row on phones): General, Checks, Notifications (one block per provider) and GitHub. New in the design system: `tm-dialog` `size="lg"` + `flush`, and `.tm-tabs-vertical`.
- [x] **9b. Form pieces**: `input[tmInput]`, `tm-password-input`, `tm-select` (a listbox on Angular Aria), `tm-switch` and `tm-field` (label, hint, error, locked), all Signal Forms controls, on `/styleguide` and in [DESIGN-SYSTEM.md](DESIGN-SYSTEM.md#forms).
- [x] **9c. Settings editable in the UI**: opt-in with `TIDEMARK_SETTINGS_EDITABLE=true`. Schedule, GitHub token, icons and all Telegram settings save per page and apply without a restart; an env var wins and locks its field, tokens are write-only, and env vars and the dialog share one validation ([FEATURES §7](FEATURES.md#7-checks-failures-and-settings)). The rules file stays hand-edited (parked below).
- [x] **10. AI changelog summary**: opt-in summaries of the notes in range (what changed, what to do) with a verdict that can only raise the level, everywhere; a newer release extends the stored summary instead of starting over. Anthropic (Haiku 5.5), Gemini or any OpenAI-compatible endpoint (Ollama too); on demand from the detail page or automatically for updates with ≥ N releases; one stored summary per (repo, from, to); an AI settings page ([FEATURES §11](FEATURES.md#11-ai-summaries)). New in the design system: `tm-ai-marker` and the summary card.
- [x] **11. Stats**: a `/stats` page (header chart icon) with a range: behind & updates (up to date and releases behind over time, updates applied, time to update, longest behind), upstream (releases per month by level, cadence, biggest jumps), check health (durations, failures, requests, GitHub quota) and AI usage per month and model, plus lifetime numbers. Each check run now records one row of counts (`run_stats`) and outgoing requests are counted ([FEATURES §12](FEATURES.md#12-stats)). New in the design system: `tm-stat`, `tm-time-chart`, `tm-bar-chart`, `tm-bar-list` (from `ui/charts`).
- [x] **12. Stats, second round**: time to update by class and by level (one card next to Longest behind), a "From message to update" tile (from the last Telegram message), and quiet upstreams (no release in 9 months) tagged in the repositories table. Also fixes from the step 11 review: same-version rebuilds no longer count as slow updates, release counts that run past the stored notes show as "100+", "Up to date now" no longer depends on the range ([FEATURES §12](FEATURES.md#12-stats)). New in the design system: `max` on `tm-bar-list`.
- [x] **13. More stats**: software age (a median tile and the five oldest releases running, up to date or not), a tide calendar (each day's up-to-date ratio as a heatmap), the most breaking upstream above the repositories table, and release notes read in pages in Lifetime ([FEATURES §12](FEATURES.md#12-stats)). New in the design system: `tm-calendar-chart`.

## Ideas parked

- **Gated one-click update** (was step 11; parked because a useful version would also have to update the compose file, which is out of scope for now): pull the new image and recreate the container with identical config, **only when every gate passes**:
  1. The update class is patch or minor (no major, no `0.x` minor bump).
  2. The level is `none` (heuristics and, if enabled, the AI verdict).
  3. The container isn't ignored or held by a rule, and the update isn't snoozed.
  4. The container uses a **floating tag** (`latest`, `1`, `1.4`), so that a pull plus a recreate actually picks up the new version. For a pinned tag (`1.4.2`) in a Compose file, a recreate via the API would be undone by the next `docker compose up`, so for those the UI shows the line to change in the compose file instead of a button.

  Safety: disabled by default (`TIDEMARK_ENABLE_UPDATES=false`); turning it on needs socket-proxy write access ([ARCHITECTURE → Security](ARCHITECTURE.md#security)). The old image is kept; after recreating, wait for the healthcheck (or a running state for N seconds) and **roll back** to the old image automatically if it fails. Every update is logged to the update history.
- More notifiers (ntfy, Discord, webhook)
- Rules in the UI: per-container rules on the detail page, or editing `config.yaml` in the settings dialog (left out of 9c)
- Private registries (`TIDEMARK_REGISTRY_AUTH_FILE`: a docker `config.json`)
- Homepage/Homarr widget-compatible endpoint, Prometheus metrics
- Multi-host via multiple Docker endpoints
- Release-notes links for official images whose GitHub repo has no releases (postgres, mongo, mariadb, mysql, nginx: link to the project's own release notes)
- Cost per reasoning effort: record each AI call's effort (new migration), count Gemini's thinking tokens (its `completion_tokens` leaves them out, so usage is undercounted today), and show your own average tokens per summary for each level under the effort select, with an approximate price for the two default models only.
