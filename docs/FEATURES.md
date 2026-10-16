# Features

> **Tidemark**: know what changed before you update. See [BRAND.md](BRAND.md).

A self-hosted dashboard that tells you, for every running container: what version you run, what the latest version is, what changed in between, and whether any of it will bite you.

Scope: single Docker host, ~20 containers, single user, no login (LAN only).

Status legend: **MVP** = first usable version · **Later** = on the roadmap, after MVP.

---

## 1. Container inventory — MVP

- List all running (and optionally stopped) containers via the Docker API.
- Per container: name, image reference (`ghcr.io/owner/app:1.4`), image digest, created/started time, Compose project & service.
- **Group by Compose stack** using the `com.docker.compose.project` label; standalone containers go in an "Ungrouped" group.
- **List and tile views**, remembered per browser.
- **App icons** from [selfh.st/icons](https://selfh.st/icons/) (CC BY 4.0). Matched from the image name first (exact, alias like `postgres` → `postgresql`, trimmed like `woodpecker-agent` → `uptime-kuma`, unique prefix like `stirling` → `stirling-pdf`), then from the Compose service and container name. Override with the `tidemark.icon=<reference>` label, or turn it off for one container with `tidemark.icon=none`. No match gives a letter avatar. Near-black icons swap to their light variant on the dark theme, and near-white icons to their dark variant on the light theme.

## 2. Running version resolution — MVP

Many images run as `:latest` or a floating tag (`:1`, `:1.4`), so the tag alone often isn't the version. Resolution order, first hit wins:

1. Tag is a full version (`1.4.2`, `v1.4.2`, `1.4.2-alpine`) → use it.
2. Image label `org.opencontainers.image.version`, **only if it's plausible for the tag**. Labels are inherited from base images: `mongo:8.3` reports `24.04` (its Ubuntu base). Reject a label whose version doesn't share the tag's leading numbers, unless the tag is floating (`latest`).
   In practice: a label is only trusted if the registry has a tag with exactly that version.
3. Image label `build_version` (linuxserver.io images: `Linuxserver.io version:- 1.4.2-ls123 ...`), same rule.
4. **Digest match**: list the registry's tags and find a version tag whose digest equals the running image digest.
5. **Image `*_VERSION` variable** (`PG_VERSION=16.10`, `REDIS_VERSION`). Official images are rebuilt and re-pushed under the same version tags, so an older pull's digest no longer matches any tag; these images record their version in their own environment instead. Only `*_VERSION` variables are ever read. The value must exist as a tag, and must extend a version tag (`16.10` for `16-alpine`); for word tags like `latest` the variable must be named after the image, so a `NODE_VERSION` inherited from a Node base image is ignored.
6. A partial version tag (`16-alpine`, `8.3`) as-is.
7. Unknown → still report "new image" if the floating tag's digest moved.

The UI shows *how* the version was determined (tag / label / digest match / unknown) so you know how much to trust it.

## 3. Latest version lookup — MVP

- List tags from the registry (Docker Hub, GHCR, Quay, lscr.io, GitLab, generic OCI v2 registries).
- Keep only tags that parse as versions **and share the same variant** as the running tag: `1.4-alpine` is only compared with `x.y-alpine`, never with `x.y` or `x.y-debian`.
- Ignore pre-releases (`-rc`, `-beta`, `-alpha`, `-dev`, `nightly`) unless enabled for that container.
- Classify the update: **patch / minor / major / digest-only** (same tag, new image).
- Also report the newest version *within the current major*, so a "major available" container can still show "1.9.3 is a safe step".

## 4. Changelog between versions — MVP

- Find the source repository, first hit wins:
  1. Container label override `tidemark.repo=github.com/owner/repo`.
  2. Config file mapping (image → repo).
  3. Image label `org.opencontainers.image.source`.
  4. Known image registries: `ghcr.io/owner/repo` implies `github.com/owner/repo`.
- Fetch **GitHub Releases** (and GitLab Releases) and keep the ones between the running and latest version (exclusive / inclusive).
- Fallback when a repo has no releases: fetch `CHANGELOG.md` and split it by version headings.
- Render release notes as markdown, newest first, collapsible per release.
- Cache releases in SQLite; published releases are immutable, so they only need fetching once.

## 5. Breaking change highlighting — MVP

A heuristic pass over each release, producing per-release flags plus an overall level of **none / attention / breaking**:

| Signal | Level |
|---|---|
| Major version bump (or `0.x` minor bump) | breaking |
| Text matches `BREAKING`, `breaking change`, `⚠️`, `💥`, conventional-commit `feat!:` / `fix!:` | breaking |
| Text matches `migration`, `migrate`, `removed`, `deprecated`, `action required`, `manual step`, `config change`, `env var` renamed | attention |

Matching lines are shown highlighted in the changelog view so you can jump straight to them.

## 6. Links to the repo — MVP

- **Compare link**: `https://github.com/owner/repo/compare/v1.4.2...v1.6.0` (tag prefix `v` auto-detected from the repo's actual tag names).
- Link to the release page of every version in between, and to the repo's tree at your running version.

## 7. Scheduled checks & caching — MVP

- Built-in scheduler (default: every 6 h, configurable cron expression) plus a "Check now" button.
- Registry calls use `HEAD` on manifests where possible (doesn't count towards Docker Hub's pull rate limit).
- Optional GitHub token (raises the API limit from 60 to 5000 requests/hour). Recommended.

## 8. Ignore / pin rules — MVP

Configurable per container via labels (or config file):

| Label | Effect |
|---|---|
| `tidemark.ignore=true` | Hide container from checks |
| `tidemark.allow-major=false` | Never report major updates (still shows "major exists" quietly) |
| `tidemark.prerelease=true` | Include pre-release tags |
| `tidemark.tag-pattern=^\d+\.\d+\.\d+-alpine$` | Explicit regex for which tags count as versions |
| `tidemark.repo=github.com/owner/repo` | Source repo override |

## 9. Reviewed / snooze state — Later

- **Mark as reviewed**: "I've read the changes up to 1.6.0". The badge fades until a version *newer* than 1.6.0 appears.
- **Snooze**: hide an update until a date, or until a version newer than X.
- **Update history**: records detected version changes per container (you ran 1.4.2 until Oct 3, then 1.6.0).

## 10. Telegram notifications — Later

- Telegram bot (bot token + chat ID). One message per check run with **new** updates only (respects reviewed/snooze state), grouped by stack, breaking ones first, with a link to the dashboard.
- Notifier is an interface, so ntfy, Discord and webhooks can be added later without touching the rest.
- Quiet mode: only notify for breaking/attention, or digest once a day.

## 11. AI changelog summary — Later (opt-in)

- Bring your own API key (Anthropic by default; provider behind an interface).
- On demand (button) or automatically for updates with ≥ N releases in between: summarise all release notes between running and latest into a short "what changed / what you need to do" list.
- The model also returns a structured verdict (`none | attention | breaking`) with reasons. **This verdict can only raise the heuristic level, never lower it.**
- Summaries are cached per (repo, from, to); nothing is sent unless enabled.

## 12. One-click update (gated) — Later

Pull the new image and recreate the container with identical config, **only when every gate passes**:

1. Update class is patch or minor (no major, no `0.x` minor bump).
2. Heuristic level is `none` **and** the AI verdict (if enabled) is `none`.
3. Container isn't pinned/ignored, and update isn't snoozed.
4. The container uses a **floating tag** (`latest`, `1`, `1.4`) so that a pull plus a recreate actually picks up the new version. For a pinned tag (`1.4.2`) in a Compose file, recreating via the API would be undone by the next `docker compose up`. For those the UI shows the line to change in the compose file instead of a button.

Safety:
- Disabled by default; turning it on requires Docker socket write access (see Architecture → Security).
- The old image is kept; after recreation, wait for the healthcheck (or a running state for N seconds). If it fails, **roll back** to the old image automatically.
- Every update is logged to the update history.

---

## Explicitly out of scope (for now)

- Login, multi-user, multi-host.
- Fully automatic updates without a click (Watchtower-style).
- Updating Compose files on disk.
