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

### Scale

- **Several Docker hosts** via multiple endpoints (out of scope for now, see [FEATURES](FEATURES.md#out-of-scope-for-now)).
