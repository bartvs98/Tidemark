# Roadmap

Finished steps get one line: the details are in the PRs and the linked docs. Open steps are written out in full. **No step is open right now.** Open fixes are in [AUDIT.md](AUDIT.md).

## MVP

- [x] **B1. Name & branding** ([BRAND.md](BRAND.md)).
- [x] **B2. Design system**: tokens, core components, `/styleguide` ([DESIGN-SYSTEM.md](DESIGN-SYSTEM.md)).
- [x] **0. Scaffolding**: Go module, Angular workspace, embedded SPA, Dockerfile, compose example.
- [x] **1. Container inventory**: own read-only Docker client (live socket or snapshot), dashboard grouped by stack.
- [x] **2. Versions & registries**: running → latest, update class, how the version was found.
- [x] **3. Storage & scheduling**: SQLite, tag cache, scheduler, Check now.
- [x] **4. Changelogs**: GitHub and GitLab releases, `CHANGELOG.md` fallback, detail page.
- [x] **5. Breaking-change analysis**: phrase heuristics, flagged lines, levels and filters ([FEATURES §5](FEATURES.md#5-breaking-change-highlighting)).
- [x] **6. Rules**: ignore, pin, allow-major, prerelease, tag pattern and repo, via labels or a config file.
- [x] **B3. Visual refresh**: hero with the tide gauge, tide backdrop, one toolbar row, detail header with the severity frame.
- [x] **7. Deploy to the home server**: CI image and socket-proxy setup; led to the `*_VERSION` fallback and channel tags.

## After the MVP

- [x] **B4. Settings dialog**: read-only at first (`GET /api/settings`, never exposes tokens).
- [x] **H1–H3. Housekeeping**: MIT licence, CI skips docs-only changes, one folder per Angular component.
- [x] **H4. Visible check failures**: classified reasons inline; a rate-limited tag list falls back to the stored one ([FEATURES §7](FEATURES.md#7-checks-failures-and-settings)).
- [x] **H5. Releases + v1.0.0** (19 Oct 2026): release tags publish the image and a GitHub Release ([ARCHITECTURE → Deployment and CI](ARCHITECTURE.md#deployment-and-ci)).
- [x] **H6. Search and full breaking reasons**: toolbar search, quotes that jump to their line.
- [x] **H7, H9, H10. Owner's notes, rounds 3–5**: settled stacks collapse into "Up to date", tap tooltips, kept scroll position, flagged lines say why, smaller polish.
- [x] **H8. First audit** (20 Oct 2026): fixes in six PRs.
- [x] **8. Reviewed / snoozed + update history** ([FEATURES §9](FEATURES.md#9-reviewed-snoozed-and-history)).
- [x] **9. Telegram notifications** ([FEATURES §10](FEATURES.md#10-notifications)).
- [x] **9a–9c. Editable settings**: settings pages behind side tabs, the form pieces, opt-in editing with `TIDEMARK_SETTINGS_EDITABLE` ([FEATURES §7](FEATURES.md#7-checks-failures-and-settings)).
- [x] **10. AI changelog summaries** ([FEATURES §11](FEATURES.md#11-ai-summaries)).
- [x] **11–13. Stats**: the `/stats` page and two follow-up rounds ([FEATURES §12](FEATURES.md#12-stats)).

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

- **Forgejo, Gitea and self-hosted GitLab notes.** Only github.com and gitlab.com are supported now ([AUDIT F](AUDIT.md#f-shortcomings-worth-knowing)). Forgejo and Gitea have a GitHub-like API (`/api/v1/repos/{owner}/{repo}/releases`), behind a host allow-list in config.
- **Notes links for official images** whose GitHub repo has no releases (postgres, mongo, mariadb, mysql, nginx): link to the project's own release notes.
- **Private registries** (`TIDEMARK_REGISTRY_AUTH_FILE`: a docker `config.json`).

### Notifications and integrations

- **More notifiers**: ntfy, Discord, webhook (the notifier is already an interface).
- **Homepage/Homarr widget** endpoint and **Prometheus metrics**.

### Settings and AI

- **Rules in the UI**: per-container rules on the detail page, or editing `config.yaml` in the settings dialog.
- **Cost per reasoning effort**: record each AI call's effort (new migration), count Gemini's thinking tokens (its `completion_tokens` leaves them out, so usage is undercounted today), and show the average tokens per summary for each level under the effort select, with an approximate price for the two default models.

### Scale

- **Several Docker hosts** via multiple endpoints (out of scope for now, see [FEATURES](FEATURES.md#out-of-scope-for-now)).
