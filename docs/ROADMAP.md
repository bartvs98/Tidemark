# Roadmap

Each step ends in something runnable and verifiable. MVP = steps B1–7.

## Brand & design

- [x] **B1. Name & branding**: pick a name (checked for collisions on GitHub / Docker Hub), logo mark, colour palette, typography, voice. *Done when:* `docs/BRAND.md` + logo SVGs exist and the working title is replaced everywhere.
- [x] **B2. Design system** (after scaffolding): design tokens (colour, type, spacing, radius, elevation, light/dark) as CSS variables wired into Tailwind v4 `@theme`; core Angular components (button, badge, card, table row, tabs, toast, empty state, skeleton); status language for update class + breaking level; a `/styleguide` route showing everything. *Done when:* the dashboard in step 1 can be built only from design-system pieces.

## MVP

- [x] **0. Scaffolding**: Go module, Angular 22 workspace (`/web`), dev proxy, embedded SPA serving, `/api/health`, multi-stage Dockerfile, compose example. *Done when:* `docker compose up` serves the empty Angular app with a working health call. ⚠️ Binary + embedded SPA verified locally; the Docker image build is unverified until Docker is available (step 1).
- [ ] **1. Container inventory**: `internal/docker`, `GET /api/containers`, dashboard listing containers grouped by Compose stack. *Done when:* the dashboard shows your real containers.
- [ ] **2. Versions & registries**: `internal/version` (parse, variants, compare, classify) with thorough unit tests on real-world tags. `internal/registry` (tags, digest HEAD, auth for Hub/GHCR/Quay/lscr). `internal/resolve` running-version chain. *Done when:* every container shows running → latest + update class + how the version was found.
- [ ] **3. Storage & scheduling**: SQLite + migrations, tag cache, check runs, scheduler, `POST /api/checks`, "Check now" button. *Done when:* results survive restarts and refresh on schedule.
- [ ] **4. Changelogs**: `internal/source` (repo resolution), GitHub + GitLab releases, CHANGELOG.md fallback, compare/release links, detail page with rendered notes. *Done when:* clicking a container shows the releases between running and latest.
- [ ] **5. Breaking-change analysis**: heuristics, per-release flags, highlighted lines, level badges + filters on the dashboard.
- [ ] **6. Rules**: ignore / pin / allow-major / prerelease / tag-pattern / repo override via labels + config file.
- [ ] **7. Deploy to the home server**: build the image, run with socket-proxy, tune against your ~20 real containers (fix version-detection misses).

## After MVP

- [ ] **8. Reviewed / snooze state + update history**
- [ ] **9. Telegram notifications** (notifier interface, new-findings-only diffing, test button)
- [ ] **10. AI changelog summary** (opt-in, cached, verdict can only raise the level)
- [ ] **11. Gated one-click update** (gates, floating-tag check, pull/recreate, healthcheck, rollback)

## Ideas parked

- More notifiers (ntfy, Discord, webhook)
- Homepage/Homarr widget-compatible endpoint, Prometheus metrics
- Multi-host via multiple Docker endpoints
