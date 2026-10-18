# Features

> **Tidemark**: know what changed before you update. See [BRAND.md](BRAND.md).

A self-hosted dashboard that tells you, for every running container: what version you run, what the latest version is, what changed in between, and whether any of it will bite you.

Scope: single Docker host, ~20 containers, single user, no login (LAN only).

Status legend: **MVP** = first usable version · **Later** = on the roadmap, after MVP.

---

## 1. Container inventory — MVP

- List all containers via the Docker API, running and stopped (with their state and Docker's status text, "Up 3 days").
- Per container: name, image reference (`ghcr.io/owner/app:1.4`), image ID, digests, platform and build time, container created time, Compose project & service.
- **Group by Compose stack** using the `com.docker.compose.project` label; standalone containers go in an "Ungrouped" group.
- **List and tile views**, remembered per browser.
- **Group by stack or severity**, remembered per browser. By stack, stacks with something to do come first and fully up-to-date stacks go last. By severity, sections run Breaking, Attention, Updates, Check failed (never checked successfully), Not checked yet, Up to date (collapsed) and Ignored (collapsed), each row naming its stack. A kept result whose refresh failed counts by that result.
- **Up-to-date containers stay quiet**: muted name, greyed icon and flat tiles, so the ones with an update stand out (their names are bold).
- **Tide gauge**: the dashboard opens with a hero ("11 updates", the summary, the Breaking / Attention / Updates / Up to date figures and Check now) beside a gauge drawn like the logo: the line is the latest version, the water how many of the checked containers are up to date (ignored and not-yet-checked ones aren't counted: the summary line says how many are ignored, and hovering the count explains it). Each detail page shows a small gauge with the number of releases you're behind. A very faint tide moves along the bottom of every page. All motion stops when the OS asks for reduced motion.
- **App icons** from [selfh.st/icons](https://selfh.st/icons/) (CC BY 4.0). Matched from the image name first (exact, alias like `postgres` → `postgresql`, trimmed like `woodpecker-agent` → `uptime-kuma`, unique prefix like `stirling` → `stirling-pdf`), then the image namespace (`vaultwarden/server` → `vaultwarden`), then the Compose service and container name; generic words (`server`, `app`, `agent`, …) are skipped. Override with the `tidemark.icon=<reference>` label (an unknown reference falls back to automatic matching), or turn it off for one container with `tidemark.icon=none`. No match gives a letter avatar. The icon index is refreshed daily. Near-black icons swap to their light variant on the dark theme, and near-white icons to their dark variant on the light theme.

## 2. Running version resolution — MVP

Many images run as `:latest` or a floating tag (`:1`, `:1.4`), so the tag alone often isn't the version. Resolution order, first hit wins:

1. Tag is a full version (`1.4.2`, `v1.4.2`, `1.4.2-alpine`) → use it.
2. **Version label**: `org.opencontainers.image.version`, `org.label-schema.version`, `version`, or linuxserver's `build_version` (`Linuxserver.io version:- 1.4.2-ls123 ...`), **only if it's plausible**. Labels are inherited from base images: `mongo:8.3` reports `24.04` (its Ubuntu base). So a label only counts if the registry has a tag with exactly that version (matching the `tag-pattern` rule if set) and, with a version tag, the label is the same variant and extends it (`16.10-alpine` for `16-alpine`). Word tags like `latest` skip that prefix check. With a full version tag, a label that only adds a packaging build refines it (`1.20.0` → `1.20.0-ls166`).
3. **Digest match**: list the registry's tags and find a version tag whose digest equals the running image digest.
4. **Image `*_VERSION` variable** (`PG_VERSION=16.10`, `REDIS_VERSION`). Official images are rebuilt and re-pushed under the same version tags, so an older pull's digest no longer matches any tag; these images record their version in their own environment instead. Only `*_VERSION` variables are ever read. The value must exist as a tag, and must extend a version tag (`16.10` for `16-alpine`); for word tags like `latest` the variable must be named after the image, so a `NODE_VERSION` inherited from a Node base image is ignored.
5. A partial version tag (`16-alpine`, `8.3`) as-is.
6. Unknown → still report "new image" if the floating tag's digest moved.

**Channel tags** (`nightly`, `dev`, `edge`, `beta`, … : a pre-release word as the whole tag) follow builds, not releases, and are usually ahead of the newest stable release. So they're never compared with it: the running version stays unknown, the only update is "new image" when the tag's digest moved ("new build available"), and there's no breaking verdict from stable release notes. The newest stable release is shown as a quiet line ("newest stable v2.36.0"); the detail page shows its notes for reference.

The UI shows *how* the version was determined (tag / label / digest match / `*_VERSION` variable / unknown) so you know how much to trust it.

## 3. Latest version lookup — MVP

- List tags from the registry (Docker Hub, GHCR, Quay, lscr.io, GitLab, generic OCI v2 registries), anonymously: only public images for now.
- Keep only tags that parse as versions **and share the same variant** as the running tag: `1.4-alpine` is only compared with `x.y-alpine`, never with `x.y` or `x.y-debian`. The word tags `latest`, `stable`, `release`, `main` and `master` follow the plain (unsuffixed) family.
- Ignore pre-releases (e.g. `-rc`, `-beta`, `-alpha`, `-dev`, `nightly`, `canary`, `edge`; full list in `internal/version`) unless enabled for that container (`prerelease` rule, §8). Pre-releases order by stage (`nightly`/`dev` < `alpha` < `beta` < `pre`/`preview` < `rc`), then by number (`rc.2` < `rc.10`). A tag that is only such a word (`nightly`) is a channel, see §2.
- Classify the update: **patch / minor / major / digest** (same tag with a new image, or a packaging-build bump like `-ls174` → `-ls175`). A missing component counts as 0 (`16` → `16.4` is minor); a pre-release → its release counts as patch.
- Also report the newest version *within the current major*, so a "major available" container can still show "1.9.3 is a safe step". Not shown while a pin or `allow-major=false` is active (§8).
- "New image, same tag" detection for floating tags needs the image's registry digest (`RepoDigests`, set when the image was pulled).

## 4. Changelog between versions — MVP

- Find the source repository (GitHub or GitLab), first hit wins:
  1. Container label override `tidemark.repo=github.com/owner/repo` (always trusted).
  2. Image label `org.opencontainers.image.source` (or `org.label-schema.vcs-url`), **only if it plausibly belongs to the image**: same owner as the image namespace, or the repo name shares a word with a part of the image path. Labels are inherited from base images (`plex-meta-manager` claims `linuxserver/docker-baseimage-ubuntu`).
  3. `ghcr.io/owner/repo` → `github.com/owner/repo`.
  4. A short list of official Docker Hub images (`redis` → `redis/redis`, `postgres` → `postgres/postgres`, …).
  5. A guess: `docker.io/namespace/name` → `github.com/namespace/name` (shown as "guessed" in the UI).
  The `repo` rule (§8) can also come from the config file (shown as "set in the config file"); a repo on an unsupported host falls back to the steps below it.
- linuxserver.io images point at their packaging repo (`linuxserver/docker-<app>`); its releases match the `-ls` tags and include the upstream changes.
- Fetch **GitHub Releases** (and GitLab Releases), keep the ones after the running version up to and including the latest, newest first. Drafts are skipped; pre-releases only show when you run one or the latest is one (which takes the `prerelease` rule, §8). With an unknown running version, only the latest release is shown.
- Release tags are matched to image versions ignoring a `v` prefix and the variant; prefixed tags like `release-1.2.3`, `release/1.2.3`, `version-1.2.3` and `app@1.2.3` are understood too.
- Fallback when a repo has no releases: `CHANGELOG.md`, `CHANGES.md` or `changelog.md` from the default branch, split into sections by version headings (levels 1–4); a date in the heading becomes the release date.
- Stored in SQLite and refreshed at most every 6 h, or sooner when the running version isn't covered yet; GitHub requests send the ETag, and "304 Not Modified" answers don't count against the rate limit. Pages are fetched only until the running version is covered (max 5 × 100 releases). When GitHub's rate limit is hit, no further GitHub calls are made until it resets (15 min if GitHub doesn't say when), and the stored notes are still shown.
- **Prefetched** after each check for containers with an update, so the detail page opens instantly. Opening a container whose notes weren't prefetched fetches them then (waiting up to 15 s, then showing whatever is stored).
- Rendered as sanitised markdown (DOMPurify): `#123` links to the issue/PR, `@user` to the profile; links open in a new tab without a referrer.

## 5. Breaking change highlighting — MVP

A heuristic pass (`internal/analysis`) over each release's notes between running and latest, producing per-release flags plus an overall level of **none / attention / breaking**:

| Signal | Level |
|---|---|
| Major version bump (or `0.x` minor bump); needs both versions known | breaking |
| e.g. `BREAKING` (upper case), `breaking change(s)`, `potentially` / `possibly breaking`, a line starting with `Breaking`, `backward(s)-incompatible`, `not backward compatible`, `incompatible changes`, `💥`, conventional-commit `feat!:` / `fix(x)!:` | breaking |
| e.g. `⚠️`, `action required`, `manual step` / `manual intervention`, "read the migration/upgrade guide", `upgrade notes`, "you need to migrate / update your …", "existing users should …", "migration may take", "has been / will be removed", "is deprecated", "no longer supported / available", "drops support", end of life, a line starting with `Removed` / `Deprecated` / `Dropped` / `Important` / `Warning` / `Heads up`, GitHub `[!IMPORTANT]` / `[!WARNING]` / `[!CAUTION]`, a renamed env var / setting / option / endpoint | attention |

Rules that keep it honest (tuned on ~900 real releases):

- **Phrases, not words.** "fix a bug breaking the import", "Migrate mqtt to the new API" and "while the entity is removed" are everyday release-note language and don't count. "No breaking changes", "non-breaking", "fix breaking changes in X" and "add a breaking change note" are negations.
- **`⚠️` alone is attention**, not breaking: projects use it for recurring reminders (Vaultwarden's "migrate off bitwardenrs").
- **Sections inherit.** Lines under a flagged heading or bold label ("## Breaking changes", "⚠️ **Breaking Changes**", "## Upgrade notes") get its level (or higher), until the next heading at the same level or a generic one ("Features", "Bug fixes", "Other"…). An empty flagged section ("None", "N/A") says nothing.
- **Only news counts.** Lines that already appear in the running release's notes, compared without numbers and links, are skipped: some projects repeat their history or boilerplate ("read the migration guide (#v3715)") in every release.
- Code blocks and HTML comments are ignored.

The dashboard shows the level next to the update class (reasons in the tooltip) and filters on All / Updates / Breaking. The detail page explains the verdict ("v3.0.0: Redis 8.0 or newer is now required": at most three quoted lines of up to 140 characters, breaking first, then "and N more flagged lines"), highlights flagged lines in the notes, and opens the newest release plus the breaking ones (attention releases stay closed but carry their badge). The level is computed from stored notes on every request (cached until the notes change), so it appears once the notes are fetched and never lags behind the detail page. Without stored notes (e.g. rate limited) only the version jump counts.

## 6. Links to the repo — MVP

- **Compare link**: `https://github.com/owner/repo/compare/v1.4.2...v1.6.0` (uses the repo's own tag names, so a `v` or `release-` prefix is picked up automatically; §4). Only when both versions have a matching repo tag.
- Link to the release page of every version in between, and to the repo's tree at your running version.
- **Your version in the release list** (GitHub): opens `/releases` on the page that holds your version (10 per page, pre-releases included), scrolled to and highlighting its heading via a text fragment, so you see it in relation to everything newer.
- Built in step 4 on the container detail page (`/containers/<name>`), with a Details tab (image, digests, how the version and repo were found, OCI labels).

## 7. Scheduled checks & caching — MVP

- Built-in scheduler (default: every 6 h, configurable cron expression) plus a "Check now" button.
- A failed check (registry unreachable, e.g. right after the host wakes from sleep) keeps the last good result, shown with a quiet "refresh failed" note (time and error on hover, spelled out in the detail page's Details tab); the dashboard summary counts failed checks. A container that never had a successful check shows "check failed". A failed check doesn't force a check at startup.
- Registry calls use `HEAD` on manifests where possible (doesn't count towards Docker Hub's pull rate limit).
- Optional GitHub token (raises the API limit from 60 to 5000 requests/hour). Recommended.
- **Settings dialog** (gear in the header): read-only, since everything is set by env vars. Shows whether a GitHub token is set (never the token), GitHub's remaining quota and reset time (looked up live via GitHub's `/rate_limit`, which doesn't count against it; cached for a minute), the check schedule and next run, the rules file (path, "not found" or "didn't load"), the container source and whether app icons are on, plus a link to the styleguide and Tidemark's version. The version is also in the page footer.

## 8. Rules — MVP

Per container, via container labels or the config file (`internal/rules`):

| Label | Config key | Effect |
|---|---|---|
| `tidemark.ignore=true` | `ignore` | Not checked at all (no registry or GitHub calls, no release notes); a stored result is deleted at the next run. Its app icon still shows. Listed in a collapsed, muted "Ignored" section at the bottom of the dashboard; left out of the Updates / Breaking filters. |
| `tidemark.pin=1.4` | `pin` | Only versions starting with this prefix count as updates (`1.4` → 1.4.x, `16` → 16.x). |
| `tidemark.allow-major=false` | `allow-major` | Only versions within the running major count as updates. |
| `tidemark.prerelease=true` | `prerelease` | Pre-release tags count as updates (and their releases show in the changelog). |
| `tidemark.tag-pattern=^\d+\.\d+\.\d+-alpine$` | `tag-pattern` | Only registry tags matching this regex count as versions (for latest *and* for finding the running version). |
| `tidemark.repo=github.com/owner/repo` | `repo` | Source repository for release notes (§4). |

Booleans accept `true/false/yes/no/1/0` (any case). A pin may start with `v` (`v1.4`). A `repo` on a host other than GitHub or GitLab is silently ignored: the repo is detected as usual. A version that a pin or `allow-major=false` keeps back is still shown, quietly: "2.1.0 held back by pin 1.4", "3.0.0 held back, majors off". The update class and risk level are those of the held-in version.

**Config file** (`TIDEMARK_CONFIG`, default `<data dir>/config.yaml`):

```yaml
containers:            # container name, glob (* ?) allowed
  vaultwarden:
    allow-major: false
  "immich-*":
    ignore: true
images:                # image repository without tag, glob allowed
  postgres:            # matches postgres:16-alpine, docker.io/library/postgres:16
    pin: "16"
  linuxserver/sonarr:  # no registry: also matches lscr.io/… and ghcr.io/…
    prerelease: true
  ghcr.io/foo/bar:
    repo: github.com/foo/bar-src
```

- **Precedence per rule:** container label > `containers:` entry > `images:` entry > default. Within a section all matching entries merge rule by rule, the most specific first (exact before glob, longer key before shorter).
- **Re-read at the start of every check run**, so edits apply on the next check without a restart. A file that doesn't load (bad YAML, unknown key, an empty key, an image key with a tag or digest) is reported on the dashboard; the last good file stays in use and labels always apply. A missing file means no file rules.
- **Invalid values don't break a container:** that rule is skipped and shown as "rule skipped" on the dashboard and in the detail page's Rules card (`tidemark.allow-major: "maybe" is not true or false`).
- **Edge cases:** a pin above the running version is offered as an update; a pin matching no tag gives no latest version (the newest one shows as held back); `allow-major=false` can't apply while the running version is unknown (e.g. `:latest` with no label, digest or `*_VERSION` match); pin and `allow-major=false` together both apply. A version label only counts if a tag matching the `tag-pattern` has that version.
- **Results follow the rules:** each stored result records the rules it was checked with; when they change (a new label means a recreated container), the result is stale until the next check, and startup checks right away when any non-ignored container has no valid result.

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
3. Container isn't ignored or held by a rule, and the update isn't snoozed.
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
