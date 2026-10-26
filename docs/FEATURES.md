# Features

How Tidemark behaves and the rules behind its decisions. Scope: one Docker host, ~20 containers, one user, no login (LAN only). Config syntax is in the [README](../README.md#configuration), how it's built in [ARCHITECTURE.md](ARCHITECTURE.md), plans in the [ROADMAP](ROADMAP.md).

Ordinary UI behaviour (Back restores the scroll position, Escape closes things, errors offer a retry) isn't listed here. This page covers what Tidemark decides and why.

## 1. Dashboard

- **Inventory:** every container from the Docker API, running and stopped, with its image, Compose project and service.
- **Hero:** the headline, a summary line and the figures.
  - Headline: "N updates" (open ones, i.e. not reviewed or snoozed), "No open updates" (only reviewed or snoozed ones left), "All up to date", or "Checking done/total" during a run.
  - The Breaking / Attention / Updates figures count open updates only.
  - The summary line also counts reviewed, snoozed, ignored and failed checks, and shows when the last and next check run.
  - In snapshot mode a "Snapshot · date" badge shows.
- **Tide gauge:** beside the hero, drawn like the logo. The line is the latest version; the water rises with the share of checked containers that are up to date. Ignored and unchecked containers don't count (the tooltip says so). All motion stops under reduced motion.
- **Toolbar:** filter All / Updates / Breaking, a search field, group by Stack or Severity, List or Tiles. Choices are remembered per browser; the search lasts for the browser tab only.
  - Filter counts include reviewed updates but not snoozed ones.
  - Search matches name, image or stack and combines with the filter. The hero figures keep counting everything.
- **Grouping by stack:** Compose stacks as sections (standalone containers under "Ungrouped"), stacks with work first. Under "All", stacks with nothing to do (everything up to date or reviewed) go into one collapsed "Up to date · N stacks" section, which opens by itself when it's all there is to show (everything is up to date, or a search only matches there). Closed again, it stays closed until that changes; the open state is shared with the severity grouping.
- **Grouping by severity:** Breaking, Attention, Updates, Check failed (never checked successfully), Not checked yet, then Up to date (collapsed, and open by itself when it's the only group). Each row names its stack.
- **Snoozed and ignored** containers only show under All, in collapsed sections at the bottom ("Snoozed" above "Ignored"). A reviewed one stays in its section ([§9](#9-reviewed-snoozed-and-history)).
- **Per container:** running → latest, how the running version was found, the update class and the risk level, with the reasons on hover. The row or tile links to the detail page.
  - **Quiet when there's nothing to do:** up-to-date, snoozed and ignored containers fade, and come back on hover or focus. Containers with an update have a bold name, so they stand out.
  - **Reviewed** updates keep their place with faded badges and a quiet "Reviewed" marker. **Snoozed** ones say "until 3 Nov" or "until newer than 1.6.0".
  - **Extra badges:** not running, "image missing", "check failed" (no successful check yet) and "rule skipped". A version held back by a rule shows as a quiet line under the version.
  - **A failed refresh** of an earlier result keeps that result (it still counts for the filters) and adds a quiet "refresh failed" note ([§7](#7-checks-failures-and-settings)).
  - A config file that won't load gets a notice at the top.
- **Refreshing:** during a run, results fill in as they arrive. Otherwise the page reloads its data every minute, so scheduled checks show up without a reload. If that reload fails, the last data stays with a "Couldn't refresh" callout.

### App icons

Icons come from [selfh.st/icons](https://selfh.st/icons/) (CC BY 4.0). Tidemark's own icon is built in, so it shows even without the CDN.

- **Matching**, first hit wins:
  1. The image name: exact, an alias (`postgres` → `postgresql`, `docker-socket-proxy` → `docker`), trimmed (`woodpecker-agent` → `woodpecker`), or a unique prefix (`stirling` → `stirling-pdf`).
  2. The image namespace (`vaultwarden/server` → `vaultwarden`).
  3. The Compose service, then the container name.

  Generic words (`server`, `app`, `agent`, …) are skipped. No match gives a letter avatar.
- **Overrides:** the label `tidemark.icon=<reference>` (an unknown reference falls back to matching), `tidemark.icon=none` turns it off.
- **Theme:** near-black icons switch to their light variant on the dark theme, and near-white icons to their dark variant on the light theme.
- **Fetching:** the index is refreshed daily; a failed download (no network yet at boot) is retried after 1, 5 and 15 min. An icon the CDN doesn't have isn't asked for again for an hour.

## 2. Running version resolution

Many images run as `:latest` or a floating tag (`:1`, `:1.4`), so the tag alone often isn't the version. First hit wins:

1. **Tag:** a full version (`1.4.2`, `v1.4.2`, `1.4.2-alpine`).
2. **Version label:** `org.opencontainers.image.version`, `org.label-schema.version`, `version`, or linuxserver's `build_version`. Labels are inherited from base images (`mongo:8.3` reports `24.04`, its Ubuntu base), so a label only counts when it's plausible:
   - the registry has a tag with exactly that version (matching `tag-pattern` if set);
   - with a version tag, the label extends it in the same variant (`16.10-alpine` for `16-alpine`);
   - with a word tag, the label is that word's variant (`latest`: the plain image; `alpine`: an alpine tag);
   - a channel tag (below) never takes a label.

   With a full version tag, a label that only adds a packaging build refines it (`1.20.0` → `1.20.0-ls166`).
3. **Digest match:** a version tag whose registry digest equals the running image's. Up to 30 tags written like the running one are tried, newest first, skipping a tag when a more precise one exists (`17` when `17.6` exists).
4. **Image `*_VERSION` variable** (`PG_VERSION=16.10`). Official images are re-pushed under the same tags, so an older pull's digest stops matching any tag; these images record their version in their environment instead. Only `*_VERSION` variables are ever read. The value must exist as a tag and extend the version tag (`16.10` for `16-alpine`). For a word tag like `latest`, the variable must be named after the image or a known alias (`PG_VERSION` for `postgres`), so an inherited `NODE_VERSION` is ignored.
5. **Partial version tag** (`16-alpine`, `8.3`) as-is.
6. **Unknown.** Tidemark still reports "new image" when the floating tag's digest moved.

The UI shows how the version was found, so you know how much to trust it.

**Channel tags** are a pre-release word as the whole tag (`nightly`, `dev`, `edge`, `beta`, …). They follow builds, not releases, and are usually ahead of the newest stable release, so they're never compared with it. The running version stays unknown, the only update is a new build (the tag's digest moved), and there's no breaking verdict. The newest stable release shows as a quiet line, and the detail page shows its notes for reference.

## 3. Latest version lookup

- Tags come from the registry (Docker Hub, GHCR, Quay, lscr.io, GitLab, any OCI v2 registry), anonymously, so only public images work.
- **Same variant only:** `1.4-alpine` is compared with `x.y-alpine`, never with `x.y` or `x.y-debian`. The words `latest`, `stable`, `release`, `main` and `master` follow the plain (unsuffixed) tags. The highest version wins; on a tie, the running tag's spelling (`v` prefix, variant) wins.
- **Numeric suffixes** sort after the version: `16-3.4` (PostGIS 3.4 on PostgreSQL 16), `4.0.2-20` (Node 20), Debian revisions like `-1`. The running tag's line is preferred: `16-3.4` → `17-3.5`, `4.0.2-20` → `4.0.5-20` (not `-22`). If the newest version doesn't have that line, the highest suffix wins, so a changing revision doesn't block updates. A suffix-only bump (`16-3.4` → `16-3.5`) is a patch.
- **Date tags** (`2025-01-01`) are only compared with other date tags.
- **Pre-releases** (`-rc`, `-beta`, `-alpha`, `-dev`, `nightly`, `canary`, `edge`, …; full list in `internal/version`) are ignored unless the `prerelease` rule is on. They sort by stage (`nightly`/`dev` < `alpha` < `beta` < `pre`/`preview` < `rc`), then by number (`rc.2` < `rc.10`).
- **Update class:** patch, minor, major or digest (a new image under the same tag, or a packaging-build bump like `-ls174` → `-ls175`).
  - A missing number counts as 0 (`16` → `16.4` is minor), and a pre-release → its release is a patch.
  - In calendar versions (first number 2000–2099) a new year is minor, not major: the year says when, not how much changed, so the release notes decide.
- **Safe step:** the newest version within the current major is reported too, so a container with a major update can still show "or 1.9.3 in your major". Not shown while a pin or `allow-major=false` applies.
- "New image, same tag" needs the image's registry digest (`RepoDigests`, set when the image was pulled).

## 4. Changelog between versions

**Source repository** (GitHub or GitLab), first hit wins:

1. The `repo` rule (label or config file). A repo on another host is ignored and detection carries on.
2. The image label `org.opencontainers.image.source` (or `org.label-schema.vcs-url`), but only if it plausibly belongs to the image: same owner as the image namespace, or the repo name shares a word with the image path. Labels are inherited too (images on a linuxserver base claim `linuxserver/docker-baseimage-ubuntu`).
3. `ghcr.io/owner/repo` → `github.com/owner/repo`.
4. A short list of official Docker Hub images (`redis` → `redis/redis`, …).
5. A guess, `docker.io/namespace/name` → `github.com/namespace/name`, marked as guessed with a hint to set `tidemark.repo`.

linuxserver.io images point at their packaging repo (`linuxserver/docker-<app>`), whose releases match the `-ls` tags and include the upstream changes.

**Which releases:** GitHub or GitLab Releases after the running version, up to and including the latest, newest first.

- Drafts are skipped. Pre-releases only show when you run one or the latest is one.
- With an unknown running version, only the latest release shows.
- Release tags match versions ignoring a `v` prefix and the variant. Prefixes like `release-1.2.3`, `release/1.2.3`, `version-1.2.3` and `app@1.2.3` are understood.
- **Fallback** for repos without releases: `CHANGELOG.md` (or `CHANGES.md`, `changelog.md`) from the default branch, split on version headings; a date in the heading becomes the release date.

**Fetching:**

- Notes are stored and refreshed at most every 6 h, or sooner when the running version isn't covered yet. While they don't have the latest version yet (its image is out before its notes were fetched), they're refreshed at most every hour instead, so a new release gets its notes and breaking verdict soon. A repo without any notes stays at 6 h.
- They're fetched right after each check for containers with an update, so the detail page opens instantly. Otherwise the detail page fetches them, waiting up to 15 s.
- Pages are fetched only until the running version is covered, at most 5 × 100 releases. Older notes beyond that show as "not loaded".
- When GitHub's rate limit is hit, no GitHub calls are made until it resets (15 min if GitHub doesn't say), and the stored notes still show.

**Rendering:** sanitised markdown. `#123` links to the issue or PR and `@user` to the profile. Images never load: each becomes an "Image: alt" link, so opening the notes reveals nothing to their hosts.

## 5. Breaking change highlighting

A heuristic pass (`internal/analysis`) reads each release's notes between running and latest. It flags lines and gives an overall level of **none / attention / breaking**:

| Signal | Level |
|---|---|
| Major version bump (or a `0.x` minor bump); needs both versions known | breaking |
| e.g. `BREAKING` (upper case), `breaking change(s)`, `potentially` / `possibly breaking`, a line starting with `Breaking`, `backward(s)-incompatible`, `not backward compatible`, `incompatible changes`, `💥`, conventional-commit `feat!:` / `fix(x)!:` | breaking |
| e.g. `⚠️`, `action required`, `manual step` / `manual intervention`, "read the migration/upgrade guide", `upgrade notes`, "you need to migrate / update your …", "existing users should …", "migration may take", "has been / will be removed", "is deprecated", "no longer supported / available", "drops support", end of life, a line starting with `Removed` / `Deprecated` / `Dropped` / `Important` / `Warning` / `Heads up`, GitHub `[!IMPORTANT]` / `[!WARNING]` / `[!CAUTION]`, a renamed env var / setting / option / endpoint | attention |

Rules that keep it honest (tuned on ~900 real releases):

- **Phrases, not words.** "fix a bug breaking the import", "Migrate mqtt to the new API" and "when it is removed" are everyday release-note language and don't count. "No breaking changes", "non-breaking" and "fix breaking changes in X" are negations.
- **`⚠️` alone is attention,** not breaking: projects use it for recurring reminders.
- **Sections inherit.** Lines under a flagged heading or bold label ("## Breaking changes", "⚠️ **Breaking Changes**", "## Upgrade notes") get its level or higher, until the next heading at the same level or a generic one ("Features", "Bug fixes", …). A flagged section that only says "None" or "N/A" says nothing.
- **Only news counts.** Lines that already appear in the running release's notes (compared without numbers and links) are skipped: some projects repeat boilerplate in every release.
- Code blocks and HTML comments are ignored.
- **It says why.** Each flagged line records the words that set its level, or the heading it inherits from. In the notes those words get a dotted underline, and hovering the line says "Attention · matched “is removed”" or "Breaking · under “⚠ BREAKING CHANGES”".

The level is computed from the stored notes on every request, so the dashboard and the detail page always agree. Without stored notes (e.g. rate limited) only the version jump counts. With AI summaries on, the AI verdict can raise the level, never lower it ([§11](#11-ai-summaries)).

## 6. Container detail page

`/containers/<name>`.

- **Header:** version change and badges, a compact tide gauge with "N releases behind" ("N+" when the list may be cut off), and for breaking or attention a slowly circling border in the level's colour.
- **Verdict callout:** quotes every flagged line in full, breaking first. Each quote's release tag jumps to that line in the notes. Reasons from an AI summary follow, marked "AI" (no jump: they aren't quotes).
- **Summary card** (with AI summaries on, below the callout): the AI summary of the notes in range, or a button to make one ([§11](#11-ai-summaries)).
- **Links:**
  - **Compare** (`…/compare/v1.4.2...v1.6.0`) using the repo's own tag names, only when both versions have a matching tag.
  - **"`<running>` in releases"** (GitHub): the releases page that holds your version, scrolled to it, so you see it next to everything newer.
  - All releases (or the changelog), the source tree at your version, the repo, and each release's own page.
- **Review actions** (only with an update): "Mark reviewed" and "Snooze" (1 day, 1 week, 1 month, until a newer version), or the current state with Undo. While reviewed or snoozed, the border stops circling; the level and callout stay. Rules in [§9](#9-reviewed-snoozed-and-history).
- **Changes tab:** the notes per release, newest first. The newest and the breaking releases are open, attention ones collapsed with their badge. Long notes are capped with "Show all", which says how many flagged lines it hides.
- **Details tab:** the rules and where each came from, then image, running version and how it was found, latest (and what's held back), last successful check, source repo and how it was found, Compose project, image metadata and OCI labels.
- **History tab:** detected image changes, newest first: "1.4.2 → 1.6.0", or "New image" when the version didn't change or is unknown, with "ran 1.4.2 for 12 days", and "Tracking started" at the bottom.
- **Callouts** explain anything missing: a failed check (with the error and a hint), an ignored container, a guessed repo, notes that couldn't be refreshed or weren't found, an unknown running version, a channel build, older releases not loaded.

## 7. Checks, failures and settings

- **When checks run:** at startup, on the schedule (`TIDEMARK_SCHEDULE`) and with **Check now**. One run at a time; a trigger during a run is ignored.
- **Startup** only checks if the last run finished over an hour ago, ended with an error (Docker unreachable, or cut short when Tidemark stopped), or a container lacks a valid result (new, recreated or its rules changed), so restarts don't hammer registries.
- **Check now asks the registries again:** startup and scheduled runs reuse tag lists and digests fetched in the last hour; a manual run doesn't, so a release from a minute ago shows up. Rate-limited registries still fall back as below.
- **Stopping during a run** (an update, a reboot) records it as cancelled: what it checked so far is kept, nothing it didn't get to is marked failed, and no notification goes out for it.
- **A failed check keeps the last good result,** with its level and reasons, plus a quiet "refresh failed" note (e.g. after the host wakes from sleep). A container that never had a successful check shows "check failed" instead.
  - Short reasons: rate limited (429), not found or private (401/403/404), timed out, network error, registry error (5xx). Anything else shows the raw error.
  - A failed check still counts as checked, so it doesn't force a check at startup.
  - Failures are per container and per stage: when GitHub is rate limited, the version info still shows and only the notes are stale.
- **Rate-limited tag lists** fall back to the stored list, so the check still succeeds (the detail page says "Tag list from <time>"). That registry isn't asked again for the repo for 5 min, and a rate limit is never retried straight away.
- **Settings dialog** (gear in the header): read-only unless `TIDEMARK_SETTINGS_EDITABLE=true` (no login, so editing is opt-in). Pages (General, Checks, Notifications, AI, GitHub) behind side tabs, a tab row on phones. It shows whether a GitHub token is set (never the token), GitHub's remaining quota, the schedule and next run, the rules file and whether it loaded, the container source, whether icons are on, the notification settings with a test button ([§10](#10-notifications)), the AI provider, model, reasoning effort, auto threshold and this month's usage ([§11](#11-ai-summaries)), a link to the styleguide, and the version.
  - **Editable settings:** the schedule, the GitHub token, icons on/off and their URL, everything for Telegram (bot token, chat ID, level, digest, links) and everything for AI summaries (provider, API key, model, reasoning effort, base URL, auto threshold) on the AI page. Saving the `openai` provider without a model is refused (it has no default). Each page saves on its own (Save / Discard appear once something changed); an invalid value shows its error under the field and nothing on the page is saved. Changes apply without a restart.
  - **Precedence:** an env var wins and its field is locked ("Set by `TIDEMARK_SCHEDULE`"); otherwise the saved value, then the default. A saved value that's no longer valid is skipped (default used) and shown as a warning on the General page. Startup-only settings (listen address, data dir, Docker host, snapshot, log level, rules file) stay env-only; the rules file is still edited by hand.
  - **Tokens are write-only:** the dialog shows whether one is set, with Replace and Clear, but never the token itself.
  - The quota is updated from every GitHub response, so it's current right after a check. Without a token it's per IP address, shared with other tools on the server, and the dialog says so.

## 8. Rules

Per-container rules (ignore, pin, allow-major, prerelease, tag-pattern, repo) come from container labels or the config file. Syntax: [README → Per-container rules](../README.md#per-container-rules). Code: `internal/rules`.

- **Precedence per rule:** container label > `containers:` entry > `images:` entry > default. Within a section, all matching entries merge rule by rule, most specific first (exact before glob, longer key before shorter). An `images:` key without a registry also matches the same repository on lscr.io, ghcr.io and so on.
- **Ignored containers** aren't checked at all (no registry or GitHub calls), and their stored result is deleted at the next run. They still get an icon, and they're left out of the Updates / Breaking filters.
- **Held back, still shown:** a version kept back by a pin or `allow-major=false` shows quietly ("2.1.0 held back by pin 1.4"). The class and level are those of the version within the rule.
- **The file is re-read at the start of every run.** A file that doesn't load is reported on the dashboard, and the last good file stays in use. Reasons it won't load: bad YAML, an unknown or empty key, two keys for the same container (`app` and `/app`), an image key with a tag or digest. A missing file means no file rules; labels always apply.
- **Invalid values don't break a container:** that rule is skipped and shown as "rule skipped" (`tidemark.allow-major: "maybe" is not true or false`).
- **Edge cases:**
  - A pin above the running version is offered as an update.
  - A pin matching no tag gives no latest version (the newest shows as held back).
  - `allow-major=false` can't apply while the running version is unknown.
  - Pin and `allow-major=false` together both apply.
- **Results follow the rules:** each result records the rules it was checked with. When they change, the result is stale until the next check, and startup checks right away.

## 9. Reviewed, snoozed and history

All three are kept per container name, so a recreated container keeps them. Code: `internal/review`, `internal/check`, `internal/api/review.go`.

- **Reviewed** means "I've read the changes up to the latest version" (say 1.6.0). It ends by itself when a newer version appears, and the update looks new again. A rebuild under the same version keeps it: there are no new notes to read.
- **Snoozed** hides the update until a time (1 day, 1 week, 1 month) or until a version newer than the current latest. A snooze by time ends only by the clock, even if a newer version appears meanwhile.
- **Builds instead of versions:** on a channel tag, or a floating tag whose latest version is unknown, there's no version to review against. Reviewed and "snoozed until newer" are then set against the build (the tag's current registry digest): "Reviewed this build", "Snoozed until a new build".
- A container is either reviewed or snoozed; setting one replaces the other. Both only show while there's an update.
- **History** records a change whenever a container's image ID differs from its previous successful check. So a `pull` + `up` is recorded even when the version stays the same or is unknown. The first successful check records "Tracking started", and a failed check in between doesn't lose the previous image. Ignored containers get no history. History is kept when a container is removed, and a new container with the same name continues it.

## 10. Notifications

A Telegram message when a check finds new updates. Off until a bot token and chat ID are configured, by env vars or in the settings dialog ([README → Notifications](../README.md#notifications)); turning them on, off or changing them applies without a restart. Code: `internal/notify`.

- **Only new updates.** Each update (a container and its latest version, or its new build) is sent once. Tidemark remembers what it sent, so a restart doesn't send it again. It's sent again when:
  - its level rises, say once its release notes arrive and show it's breaking ("level raised");
  - a snooze by time ran out and the update is still there ("snooze ended", once).
- **Never sent:** reviewed and snoozed updates (while that applies), ignored containers, and containers without a successful check.
- **The first message** after notifications are turned on lists every pending update ("5 updates pending"), so nothing is missed. After that, only new ones.
- **One message per check run,** grouped by stack. Stacks with breaking updates come first, and within a stack breaking before attention before the rest. Breaking and attention updates carry 🔴 Breaking or 🟡 Attention; each line shows running → latest and the class, or "new build of `latest`" for a new build under the same tag or a channel tag. Lines sent again end with "level raised" or "snooze ended". Containers without a stack are under "No stack". With `TIDEMARK_PUBLIC_URL` set, container names link to their detail page and the message ends with a link to the dashboard. A message that's too long for Telegram is split between stacks (a single huge stack between lines).
- **Quiet mode:**
  - `TIDEMARK_NOTIFY_LEVEL=attention` or `breaking` leaves out quieter updates. They aren't remembered as sent, so one that later becomes breaking is still new.
  - `TIDEMARK_NOTIFY_DIGEST` (a cron spec) sends one message at that time instead of after each run, with everything new since the last message, judged on the latest check results.
- **A failed send** is retried at the next run (or digest): nothing is marked as sent.
- **Settings dialog:** whether notifications are on, the level, after each check or digest, the link base, the last message (or why it failed), when the next one can go out (the next digest, or the next scheduled check; only if something is new), and a **Send test message** button (it uses the saved settings, so it waits until unsaved changes are saved). When settings are editable, all of these can be changed there ([§7](#7-checks-failures-and-settings)).

## 11. AI summaries

Opt-in: off until a provider is configured, by env vars or on the settings dialog's AI page ([README → AI summaries](../README.md#ai-summaries)). Nothing is sent to a model while it's off. Code: `internal/ai`.

- **Providers:** `anthropic` (default model `claude-haiku-5-5`), `gemini` (default `gemini-flash-latest`) and `openai`: any OpenAI-compatible endpoint (OpenAI, OpenRouter, Ollama, LM Studio) by its base URL, with a model you name. A local server needs no API key.
- **Reasoning effort** (low / medium / high): how long the model thinks first. Default low for Anthropic and Gemini (Gemini's own default thinking added 20–30 s per call, and slow calls ran past the 2-minute limit); for OpenAI-compatible servers nothing is sent unless you pick one, since not every server knows the setting. Changing it, like changing the model, keeps existing summaries.
- **What it writes:** a verdict (none / attention / breaking) with its reasons, plus a few short items on what changed and what you need to do before or after updating (left out when nothing). The verdict is the point: an extra check on whether anything needs flagging; the prompt tells the model so, and the lists are a bonus. All of it is shown as plain text.
- **The verdict only raises.** The level everywhere (dashboard, filters, detail page, notifications) is the higher of the heuristic level ([§5](#5-breaking-change-highlighting)) and the AI verdict; an AI "none" never quiets a heuristic "breaking". A raise is sent as "level raised" like any other ([§10](#10-notifications)). AI reasons follow the quoted lines, marked "AI". Verdicts only count while AI is on.
- **When it runs:**
  - **On demand:** "Summarise N releases" on the detail page (it says which provider and model the notes go to), and "Regenerate" on an existing summary, which always summarises the whole range again.
  - **Automatically** with `TIDEMARK_AI_AUTO_MIN_RELEASES` set: right after a check, for updates with at least that many releases in range, so the verdict is there before the notification goes out. Automatic mode waits while the newest release in range has no notes yet (they usually follow within the hour), and a failed automatic attempt isn't retried for 6 h.
- **One summary per range:** stored per repo and version range (running → latest), shared by containers on the same image, and kept across restarts. It's deleted at the end of the first check run that no longer needs it: after you update, when a newer latest appears, or when the container is gone or ignored. One exception: the most complete summary for the same repo and running version is kept as the base for extending (below). Changing the model doesn't throw summaries away. When the notes in range change afterwards (say the latest release's notes arrived later), the card says so: the summary and its verdict still apply, Regenerate makes a new one, and auto mode does that by itself.
- **Extending instead of starting over:** when a newer release appears on top of a range that was summarised (you run 2.9, there's a summary up to 3.1.0, now 3.2.0 is out), Summarise and automatic mode send the earlier summary plus only the new releases' notes, and the model merges them. The earlier verdict is a floor: those releases are still in range, so the result is never lower, whatever the model says (its reasons are kept then). The footer says "extended from 3.1.0". It falls back to a full summary when the running version changed, the older notes were edited since, the earlier summary had releases left out, or the new notes plus the earlier summary don't fit the input cap.
- **Long ranges:** at most 120,000 characters of notes are sent (each release cut at 20,000). Over that, breaking releases go first, then attention ones, then the rest, newest first; the card says how many were left out.
- **Usage:** every call to the provider is recorded (on demand or automatic, full or extended, tokens in and out, how long it took, whether it worked), and kept when its summary is deleted. The card's footer shows the tokens and time of the call that made it; Settings → AI shows this month's summaries, tokens and failures. Tokens only, no cost: the provider's own dashboard has the bill. The [stats page](#12-stats) breaks usage down per month and model.
- **Failures** (key rejected, rate limited, model not found, the model declined, …) show on the card with Retry; a summary from before stays.
- **Safety:** release notes go to the model as data in delimiters, with an instruction never to follow what's inside them; the answer must match a fixed JSON schema and is checked and capped before it's stored.

## 12. Stats

The chart icon in the header opens `/stats`: how far behind the host is and how that changed, how busy the upstream projects are, how the checks themselves are doing, AI usage, and a few lifetime numbers. A range (30 days, 90 days by default, 1 year, all) applies to everything except Lifetime. Code: `internal/api/stats.go`, `internal/stats`.

- **What's recorded for it:** most numbers come from what Tidemark already stores (update history, release notes, check runs, AI calls). On top of that, each check run that finishes without a run error adds one small row of counts: containers checked, up to date, open updates per level, reviewed, snoozed, failed (per failure kind), releases behind summed over all updates (the update debt), updates per class, the registry, GitHub and GitLab requests made since the previous row (with how many were rate-limited), and the GitHub quota left. These rows are never pruned (a few KB a day). Charts built on them (up to date over time, requests, failures) start filling after the first runs.
- **Counted like the dashboard:** "checked" and "up to date" are the hero gauge's numbers (ignored containers count nowhere; snoozed ones still count as checked), and "updates", "attention" and "breaking" count open updates only (not reviewed or snoozed).
- **Behind & updates:**
  - *Up to date now*: up to date ÷ checked in the newest run, whatever the range. The *Up to date* chart: the same per run in range (runs that checked nothing are left out).
  - *Tide calendar*: one square per UTC day, shaded by up to date ÷ checked at the day's last run (runs that checked nothing don't count); days without such a run stay empty. It follows the range from the first recorded run on, at most a year back ("All" too).
  - *Releases behind* chart: the update debt per run, how far behind rather than how many containers. Updates without stored notes count 0, so it undercounts when notes are missing.
  - *Updates applied*: image changes from the history with a previous image (a pull + recreate), per week and per stack. Containers that are gone count under *Removed containers* (their stack isn't kept), apart from containers without a stack (*Ungrouped*).
  - *Time to update*: from the publish date of the version you moved to (from its stored release notes) to when Tidemark saw the new image. Only updates whose new version has a dated release count, and only for containers still on the host (their source repo is needed). An image seen before its release was published (images are often pushed before the release notes) is left out rather than counted as an instant update; the same goes for "longest behind" below. A rebuild of the same version (a new image under the same tag) counts as applied but not here, nor below.
  - *Time to update by kind*: the same median split by class (patch, minor, major) and by level (calm, attention, breaking), all on one scale, to show whether breaking updates sit longer than patches. The level is the one the stored notes give the jump (the heuristics only, without the AI verdict). Updates whose class isn't patch, minor or major (a digest change, a version that doesn't parse) only count in the overall median.
  - *From message to update* (shown once an applied update matches a Telegram message, [§10](#10-notifications)): from the message about a version to when Tidemark saw that version running. Only the last message per update is kept (a resend when the level rises or a snooze ends replaces it), so it's measured from the last one; a message sent after the update was seen doesn't count.
  - *Longest behind*: the five current updates whose first newer release came out longest ago, with how many releases you're behind (from the stored notes, so containers without notes aren't listed). When the stored notes stop before your version, the count is a minimum ("100+ releases"), here, in *Releases behind* and in *biggest jumps*. The dashboard has the full list of updates.
  - *Software age* (ignores the range): how long ago the release each container runs was published, from the stored notes, up to date or not, so a project that stopped releasing shows too. A *Median software age* tile over every container with a dated running release, and *Oldest software running*: the five oldest, marked "latest" when there's nothing newer. Ignored containers and those without notes are left out.
- **Upstream:** for the source repos of current containers with stored notes: releases per month, split into calm, attention and breaking by the same per-release analysis as the changelog ([§5](#5-breaking-change-highlighting); scored per release, without the detail page's comparison against your running release), the median gap between releases, the latest release (a repo with no release in 9 months is tagged *Quiet*, often the first sign a project has been abandoned, with a count above the table), and *biggest jumps*: the applied updates in range that skipped the most releases at once ("`1.118.2 → 1.132.0`, 14 releases"). Release dates are the project's own, so one fetch can reach back years: a sync stores the newest 100 releases, more when needed to reach your version ([§4](#4-changelog-between-versions)). That reaches back years for a slow project and only weeks for a fast one, so a month only counts the repos whose stored notes cover all of it (the oldest stored release's month is incomplete, unless that's the repo's first release). Months that leave some repos out are faded, with a note. Above the repositories table, the most breaking upstream: the repo with the most releases flagged breaking in range (ties: more attention releases), "grafana/grafana flagged breaking 6 times in 90 days"; hidden when none was. A repo whose range reaches back past its stored notes shows its count as a minimum ("100+ releases"). Undated releases (`CHANGELOG.md` sections) only count in the "All" range.
- **Check health:** check runs per trigger, failed runs (a run error, or cancelled at shutdown), run durations (the median and slowest leave failed runs out; the chart marks them), the containers failing right now with their reason, failures per reason over the range, requests per day, or per week over 92 days (registry, and release notes from GitHub and GitLab together) with the rate-limited ones split the same way ("18 rate limited (16 registry, 2 release notes)"), and the GitHub quota left as last seen (the page makes no request to GitHub, and leaves it out once the quota has reset).
- **AI usage** (shown once any call was made, [§11](#11-ai-summaries)): summaries, failures, manual vs. automatic and extended calls, tokens in and out per month and per model, and the median duration.
- **Lifetime:** only what the ranged sections don't already show with "All": watching since (the first check run), containers tracked (ever in the history), release notes stored (with roughly how many pages they'd fill: words ÷ 300), releases flagged breaking among them, the fastest update, the longest a container was behind before it was updated, the longest run of consecutive checks with no open updates (a run that checked nothing breaks it), and how long this Tidemark process has been running.
- **Docker unreachable:** the page still loads, without the parts that need the current containers (behind, upstream repos, failing now), and says so.

## Out of scope (for now)

- Login, multi-user, multi-host.
- Fully automatic updates without a click (Watchtower-style).
- Updating Compose files on disk.
