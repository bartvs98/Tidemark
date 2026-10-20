# Features

How Tidemark behaves and the rules behind its decisions. Scope: a single Docker host, ~20 containers, one user, no login (LAN only). Configuration syntax (env vars, labels, config file) is in the [README](../README.md#configuration); how it's built is in [ARCHITECTURE.md](ARCHITECTURE.md); planned behaviour is in the [ROADMAP](ROADMAP.md).

## 1. Dashboard

- **Inventory:** all containers via the Docker API, running and stopped (with their state and Docker's status text, "Up 3 days"). Per container: name, image reference, image ID, digests, platform and build time, created time, Compose project and service.
- **Hero:** headline "N updates" (open ones: not reviewed or snoozed) / "No open updates" (only reviewed or snoozed ones left) / "All up to date" / "Checking done/total" (or "N containers" before any successful check), a summary line (containers · stacks · not running · reviewed · snoozed · ignored · checked/next times · "N checks failed", meaning the latest attempt failed, with or without a kept result), the Breaking / Attention / Updates figures (open updates only) and Up to date, **Check now**, and a "Snapshot · date" badge in snapshot mode.
- **Tide gauge:** beside the hero, drawn like the logo: the line is the latest version, the water how many of the checked containers are up to date, with "N/M up to date" over it. Ignored and not-yet-checked containers aren't counted, and hovering the count says so. A very faint tide moves along the bottom of every page. All motion stops when the OS asks for reduced motion.
- **Toolbar** (each choice remembered per browser): filter All / Updates / Breaking with counts (reviewed updates included, snoozed ones not; with nothing open, the empty state points to the snoozed ones under All); a search field; group by Stack or Severity; List or Tiles view.
- **Search:** filters containers by name, image or Compose stack (case-insensitive, also inside the Ignored group) and combines with the filter; the hero figures keep counting everything. `/` focuses it, Escape or the x clears it. The query lasts for the browser tab (so it survives a visit to a detail page), not across sessions. No match: "No containers match …" with "Clear search".
  - **By stack:** Compose stacks (`com.docker.compose.project`) as sections, standalone containers in "Ungrouped"; stacks with something to do first; within them, up-to-date containers fade and lose their surface (full again, colours included, on hover or keyboard focus; list and tiles alike). Under "All", stacks that are fully up to date or reviewed go into one collapsed "Up to date · N stacks" section (each stack under a small heading inside), which opens by itself when a search only matches there; under the Updates and Breaking filters such stacks (with a reviewed update) come last as their own sections.
  - **By severity:** Breaking, Attention, Updates, Check failed (never checked successfully), Not checked yet, Up to date (collapsed), then Snoozed and Ignored (both collapsed), each row naming its stack. A kept result whose refresh failed counts by that result.
  - Snoozed and ignored containers only show under All, in collapsed, muted sections at the bottom ("Snoozed" above "Ignored"); a reviewed one stays in its stack or severity section ([§9](#9-reviewed-snoozed-and-history)).
- **Per container:** running → latest, how the version was determined, the update class and the risk level (reasons in a tooltip, shortened; in full on the detail page). Each row or tile is one link to the detail page. Up-to-date containers stay quiet (muted name, greyed icon, flat tile), so containers with an update (bold name) stand out. A reviewed update has faded badges and a quiet "Reviewed" marker (what and when on hover); a snoozed one says "until 3 Nov" / "until newer than 1.6.0". Tooltips open on hover or keyboard focus, and on touch screens with a tap (tap again or elsewhere to close; a tap on a badge shows its tip instead of opening the detail page). Badges for a non-running state, "image missing", "check failed" (no successful check yet, reason in a tooltip) and "rule skipped"; a version held back by a rule shows as a quiet line under the version change; a kept result whose refresh failed shows as usual plus a quiet "refresh failed" note (time and error on hover) and still counts for the filters. A config file that won't load gets a notice at the top.
- **While a check runs,** the headline shows progress, results fill in as they arrive (skeletons until then), and a toast says when the run finished or failed.
- **Empty states:** no containers; nothing under the filter ("No updates" / "Nothing breaking" + "Show all containers"); can't list containers (+ "Try again").
- On phones (below 640 px), list rows stack: icon and name, then the version change and update badges.

### App icons

From [selfh.st/icons](https://selfh.st/icons/) (CC BY 4.0), plus Tidemark's own icon built in (for images named `…/tidemark`; it works before the index loads and without the CDN). Matched from the image name first (exact, alias like `postgres` → `postgresql` or `docker-socket-proxy` → `docker`, trimmed like `woodpecker-agent` → `uptime-kuma`, unique prefix like `stirling` → `stirling-pdf`), then the image namespace (`vaultwarden/server` → `vaultwarden`), then the Compose service and container name; generic words (`server`, `app`, `agent`, …) are skipped. The `tidemark.icon=<reference>` label overrides it (an unknown reference falls back to automatic matching), `tidemark.icon=none` turns it off. No match gives a letter avatar. The icon index is refreshed daily. Near-black icons swap to their light variant on the dark theme, near-white icons to their dark variant on the light theme.

## 2. Running version resolution

Many images run as `:latest` or a floating tag (`:1`, `:1.4`), so the tag alone often isn't the version. First hit wins:

1. **Tag** is a full version (`1.4.2`, `v1.4.2`, `1.4.2-alpine`).
2. **Version label**: `org.opencontainers.image.version`, `org.label-schema.version`, `version`, or linuxserver's `build_version` (`Linuxserver.io version:- 1.4.2-ls123 ...`), **only if it's plausible**. Labels are inherited from base images (`mongo:8.3` reports `24.04`, its Ubuntu base), so a label only counts if the registry has a tag with exactly that version (matching the `tag-pattern` rule if set) and, with a version tag, the label is the same variant and extends it (`16.10-alpine` for `16-alpine`). Word tags like `latest` skip that prefix check. With a full version tag, a label that only adds a packaging build refines it (`1.20.0` → `1.20.0-ls166`).
3. **Digest match**: a version tag in the registry whose digest equals the running image's digest.
4. **Image `*_VERSION` variable** (`PG_VERSION=16.10`, `REDIS_VERSION`). Official images are rebuilt and re-pushed under the same version tags, so an older pull's digest no longer matches any tag; these images record their version in their own environment instead. Only `*_VERSION` variables are ever read. The value must exist as a tag and must extend a version tag (`16.10` for `16-alpine`); for word tags like `latest` the variable must be named after the image, so a `NODE_VERSION` inherited from a Node base image is ignored.
5. A partial version tag (`16-alpine`, `8.3`) as-is.
6. Unknown: still report "new image" if the floating tag's digest moved.

The UI shows *how* the version was found (tag / label / digest match / `*_VERSION` variable / unknown), so you know how much to trust it.

**Channel tags** (a pre-release word as the whole tag: `nightly`, `dev`, `edge`, `beta`, …) follow builds, not releases, and are usually ahead of the newest stable release. So they're never compared with it: the running version stays unknown, the only update is "new image" when the tag's digest moved ("new build available"), and there's no breaking verdict from stable release notes. The newest stable release is shown as a quiet line ("newest stable v2.36.0"), and the detail page shows its notes for reference.

## 3. Latest version lookup

- Tags come from the registry (Docker Hub, GHCR, Quay, lscr.io, GitLab, generic OCI v2 registries), anonymously, so only public images for now.
- Only tags that parse as versions **and share the running tag's variant** count: `1.4-alpine` is only compared with `x.y-alpine`, never with `x.y` or `x.y-debian`. The word tags `latest`, `stable`, `release`, `main` and `master` follow the plain (unsuffixed) family. The highest wins; on a tie, the running tag's spelling (`v` prefix, raw variant) is preferred.
- Pre-releases (`-rc`, `-beta`, `-alpha`, `-dev`, `nightly`, `canary`, `edge`, …; full list in `internal/version`) are ignored unless the `prerelease` rule is on. They order by stage (`nightly`/`dev` < `alpha` < `beta` < `pre`/`preview` < `rc`), then by number (`rc.2` < `rc.10`). A tag that is only such a word is a channel (§2).
- **Update class:** patch / minor / major / digest (same tag with a new image, or a packaging-build bump like `-ls174` → `-ls175`). A missing component counts as 0 (`16` → `16.4` is minor); a pre-release → its release is a patch.
- The newest version **within the current major** is reported too, so a "major available" container can still show "1.9.3 is a safe step". Not shown while a pin or `allow-major=false` is active.
- "New image, same tag" for floating tags needs the image's registry digest (`RepoDigests`, set when the image was pulled).

## 4. Changelog between versions

- **Source repository** (GitHub or GitLab), first hit wins:
  1. The `repo` rule: a `tidemark.repo` label (always trusted) or the config file (shown as "set in the config file"). A repo on another host falls back to the steps below.
  2. Image label `org.opencontainers.image.source` (or `org.label-schema.vcs-url`), **only if it plausibly belongs to the image**: same owner as the image namespace, or the repo name shares a word with a part of the image path. Labels are inherited from base images (`plex-meta-manager` claims `linuxserver/docker-baseimage-ubuntu`).
  3. `ghcr.io/owner/repo` → `github.com/owner/repo`.
  4. A short list of official Docker Hub images (`redis` → `redis/redis`, `postgres` → `postgres/postgres`, …).
  5. A guess: `docker.io/namespace/name` → `github.com/namespace/name`, shown as "guessed" with a hint to set `tidemark.repo`.
- linuxserver.io images point at their packaging repo (`linuxserver/docker-<app>`); its releases match the `-ls` tags and include the upstream changes.
- **Which releases:** GitHub or GitLab Releases after the running version up to and including the latest, newest first. Drafts are skipped; pre-releases only show when you run one or the latest is one (which needs the `prerelease` rule). With an unknown running version, only the latest release is shown.
- Release tags match image versions ignoring a `v` prefix and the variant; prefixed tags like `release-1.2.3`, `release/1.2.3`, `version-1.2.3` and `app@1.2.3` are understood too.
- **Fallback** when a repo has no releases: `CHANGELOG.md`, `CHANGES.md` or `changelog.md` from the default branch, split into sections by version headings (levels 1–4); a date in the heading becomes the release date.
- **Fetching:** notes are stored in SQLite and refreshed at most every 6 h, or sooner when the running version isn't covered yet. GitHub requests send the ETag, and "304 Not Modified" answers don't count against the rate limit. Pages are fetched only until the running version is covered (at most 5 × 100 releases). When GitHub's rate limit is hit, no further GitHub calls are made until it resets (15 min if GitHub doesn't say when), and the stored notes are still shown.
- **Prefetched** after each check for containers with an update, so the detail page opens instantly. Opening a container whose notes weren't prefetched fetches them then (waiting up to 15 s, then showing whatever is stored).
- Rendered as sanitised markdown (DOMPurify): `#123` links to the issue or PR, `@user` to the profile; links open in a new tab without a referrer.

## 5. Breaking change highlighting

A heuristic pass (`internal/analysis`) over each release's notes between running and latest gives per-release flags and an overall level of **none / attention / breaking**:

| Signal | Level |
|---|---|
| Major version bump (or `0.x` minor bump); needs both versions known | breaking |
| e.g. `BREAKING` (upper case), `breaking change(s)`, `potentially` / `possibly breaking`, a line starting with `Breaking`, `backward(s)-incompatible`, `not backward compatible`, `incompatible changes`, `💥`, conventional-commit `feat!:` / `fix(x)!:` | breaking |
| e.g. `⚠️`, `action required`, `manual step` / `manual intervention`, "read the migration/upgrade guide", `upgrade notes`, "you need to migrate / update your …", "existing users should …", "migration may take", "has been / will be removed", "is deprecated", "no longer supported / available", "drops support", end of life, a line starting with `Removed` / `Deprecated` / `Dropped` / `Important` / `Warning` / `Heads up`, GitHub `[!IMPORTANT]` / `[!WARNING]` / `[!CAUTION]`, a renamed env var / setting / option / endpoint | attention |

Rules that keep it honest (tuned on ~900 real releases):

- **Phrases, not words.** "fix a bug breaking the import", "Migrate mqtt to the new API" and "while the entity is removed" are everyday release-note language and don't count. "No breaking changes", "non-breaking", "fix breaking changes in X" and "add a breaking change note" are negations.
- **`⚠️` alone is attention**, not breaking: projects use it for recurring reminders (Vaultwarden's "migrate off bitwardenrs").
- **Sections inherit.** Lines under a flagged heading or bold label ("## Breaking changes", "⚠️ **Breaking Changes**", "## Upgrade notes") get its level (or higher), until the next heading at the same level or a generic one ("Features", "Bug fixes", "Other", …). An empty flagged section ("None", "N/A") says nothing.
- **Only news counts.** Lines that already appear in the running release's notes, compared without numbers and links, are skipped: some projects repeat their history or boilerplate ("read the migration guide (#v3715)") in every release.
- Code blocks and HTML comments are ignored.

The level is computed from the stored notes on every request, so it appears once the notes are fetched and the dashboard never disagrees with the detail page. Without stored notes (e.g. rate limited) only the version jump counts.

## 6. Container detail page

`/containers/<name>`, titled "`<name>` · Tidemark".

- **Header card:** version change and badges; a compact tide gauge with "N releases behind" ("N+" when the list may be cut off; shown when there's an update, a known running version and releases); for breaking/attention a slowly circling border in the level's colour. A callout explains the verdict ("v3.0.0 Redis 8.0 or newer is now required": at most three quoted lines, in full, breaking first, then "and N more flagged lines"). Each quote's release tag is a button that opens that release, expands its notes, scrolls to the flagged line and briefly rings it.
- **Links:**
  - **Compare** (`https://github.com/owner/repo/compare/v1.4.2...v1.6.0`), using the repo's own tag names, so a `v` or `release-` prefix is picked up automatically. Only when both versions have a matching repo tag.
  - **"`<running>` in releases"** (GitHub): opens `/releases` on the page that holds your version (10 per page, pre-releases included), scrolled to and highlighting its heading via a text fragment, so you see it in relation to everything newer.
  - "All releases" (or "Changelog" for a CHANGELOG.md source), the source tree at your version, the repo, and each release's own page.
- **Review actions** (right of the tabs, above them on phones; only with an update): "Mark reviewed" and a "Snooze" menu (1 day, 1 week, 1 month, Until a newer version), or the current state ("Reviewed up to 1.6.0 · 20 Oct", "Snoozed until 3 Nov") with Undo / Unsnooze. A toast confirms. While reviewed or snoozed, the severity frame stops circling; the level and the callout stay. Rules: [§9](#9-reviewed-snoozed-and-history).
- **Changes tab** (with the release count): the notes per release, newest first. The newest and the breaking ones are open, attention ones collapsed with their badge; flagged lines are highlighted; long notes are capped with "Show all", which says how many flagged lines it hides ("Show all · 2 flagged lines").
- **Callouts** for: a failed check (at the top: "Refresh failed at <time>" / "Check failed at <time>", the error in mono, which result is shown, and a hint for rate limits, timeouts, network and registry errors), ignored container, notes couldn't be refreshed (shows the stored ones), guessed repo (hint to set `tidemark.repo`), no release notes found, running version unknown (latest release only), channel build (the newest stable release's notes for reference, no verdict), older releases not loaded, no source repo found, notes not fetched for ignored containers.
- **Details tab:** a Rules card (each rule, where it came from, skipped ones), then image, running version and how it was found, latest (and what's held back), checked time (the last successful check, or "No successful check yet"; "· refresh failed, see above" when the latest attempt failed; "Tag list from <time>" when the tag list was reused), source repo and how it was found, Compose project/service, status, platform, image build time, image ID, digests and OCI labels.
- **History tab** (with the entry count): the container's detected image changes, newest first: "1.4.2 → 1.6.0", or "New image" with short image IDs when the version didn't change or is unknown, "ran 1.4.2 for 12 days" when the earlier entry is there, and "Tracking started · running 1.4.2" at the bottom. Empty until the first successful check.
- A 404 ("No container named …") and an error state with retry.

## 7. Checks, failures and settings

- Checks run at startup, on the schedule (`TIDEMARK_SCHEDULE`) and with **Check now**; one run at a time, so a trigger during a run is ignored. At startup a check runs only if the last one finished more than 1 hour ago, or a non-ignored container has no valid result (new, recreated, or its rules changed), so restarts don't hammer registries.
- **A failed check** (registry unreachable, e.g. right after the host wakes from sleep) keeps the last good result, with level and reasons still computed from it, plus the quiet "refresh failed" note with a short reason when the failure is known ("refresh failed · rate limited"; full error on hover or keyboard focus). A container that never had a successful check shows "check failed" with the same short reason. Reasons: rate limited (HTTP 429), not found or private (401/403/404), timed out, network error (DNS, refused connection), registry error (5xx); anything else shows the raw error. A rate-limited tag list falls back to the stored list, so the check still succeeds (the detail page says "Tag list from <time>"). go-containerregistry already asks for 1,000 tags per page, so repos with thousands of signature tags (Portainer has ~6,200) take only a few requests. A failed check counts as checked, so it doesn't force a check at startup. Failures are per container and per stage: when GitHub's rate limit is hit, the version info still shows and the detail page says the notes couldn't be refreshed.
- **Settings dialog** (gear in the header): read-only, since everything is set by env vars. Shows whether a GitHub token is set (never the token) with a hint to set `TIDEMARK_GITHUB_TOKEN` without one, GitHub's remaining quota and reset time (looked up live and cached for a minute, but every GitHub API response Tidemark gets updates it from its `X-RateLimit-*` headers, so it's current right after a check; without a token the quota is per IP address, so other tools on the server calling GitHub use it too, and the dialog says so), the check schedule and next run, the rules file (path, "not found" or "didn't load"), the container source (Docker host or snapshot) and whether app icons are on, then a link to the styleguide and Tidemark's version. The version is also in the page footer.

## 8. Rules

Per-container rules (ignore, pin, allow-major, prerelease, tag-pattern, repo) come from container labels or the config file; syntax in [README → Per-container rules](../README.md#per-container-rules). Code: `internal/rules`.

- **Precedence per rule:** container label > `containers:` entry > `images:` entry > default. Within a section all matching entries merge rule by rule, the most specific first (exact before glob, longer key before shorter). An `images:` key without a registry also matches the same repository on lscr.io, ghcr.io and so on.
- **Ignored containers** aren't checked at all (no registry or GitHub calls, no release notes) and a stored result is deleted at the next run. Their app icon still shows. They're left out of the Updates / Breaking filters.
- **Held back, still shown:** a version that a pin or `allow-major=false` keeps back is shown quietly: "2.1.0 held back by pin 1.4", "3.0.0 held back, majors off". The update class and risk level are those of the version within the rule.
- **The file is re-read at the start of every check run.** A file that doesn't load (bad YAML, an unknown key, an empty key, an image key with a tag or digest) is reported on the dashboard; the last good file stays in use and labels always apply. A missing file means no file rules.
- **Invalid values don't break a container:** that rule is skipped and shown as "rule skipped" on the dashboard and in the detail page's Rules card (`tidemark.allow-major: "maybe" is not true or false`). A `repo` on a host other than GitHub or GitLab is silently ignored, and the repo is detected as usual.
- **Edge cases:** a pin above the running version is offered as an update; a pin matching no tag gives no latest version (the newest one shows as held back); `allow-major=false` can't apply while the running version is unknown (e.g. `:latest` with no label, digest or `*_VERSION` match); pin and `allow-major=false` together both apply. A version label only counts if a tag matching the `tag-pattern` has that version.
- **Results follow the rules:** each stored result records the rules it was checked with. When they change (a new label means a recreated container), the result is stale until the next check, and startup checks right away.

## 9. Reviewed, snoozed and history

Per container name (a recreated container keeps them). Code: `internal/review`, `internal/check`, `internal/api/review.go`.

- **Reviewed** means "I've read the changes up to the latest version" (1.6.0). It applies while the latest version isn't newer than 1.6.0 and ends by itself when a newer one appears: the update looks new again. It can't be set when the latest version is unknown (a new image under a floating tag without a version); snoozing by time works there.
- **Snoozed** hides the update until a time (1 day, 1 week, 1 month from now) or until a version newer than the latest at that moment. A snooze by time ends by the clock only, even if a newer version appears meanwhile. After that it's back in its section.
- A container is either reviewed or snoozed; setting one replaces the other. Undo / Unsnooze removes it. It only shows while the container has an update (an ignored container never shows it).
- **History** records a change whenever a container's image ID differs from the one at its previous successful check, so a `pull` + `up` is recorded even when the version stays the same (a rebuilt tag) or is unknown. The first successful check records "Tracking started". A failed check in between doesn't lose the previous image. Ignored containers aren't checked, so they get no history. History is kept when a container is removed; a new container with the same name continues it.

## Out of scope (for now)

- Login, multi-user, multi-host.
- Fully automatic updates without a click (Watchtower-style).
- Updating Compose files on disk.
