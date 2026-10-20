# Audit (20 Oct 2026)

A read-only audit of the whole repo, done after v1.0.0 and step 8. Three agents each covered one area: the check pipeline, the backend shell (API, store, rules, config, icons) and the frontend. The main session then checked every finding against the code, re-ran the agents' tests where they had them, and ran `govulncheck`. Style and taste were out of scope.

The fixes are roadmap step [H8](ROADMAP.md#between-mvp-and-step-8). Each part (A–F) is its own `fix-…` PR; tick the finding here when it's merged. **Severity:** must = wrong results, security or quota burned; should = realistic edge cases, resilience, missing tests; nice = hardening.

## A. Changelog quota (PR #16) ✅

- [x] **must**: A repo whose running version lies beyond the 5 release pages a sync fetches (or whose tags don't parse) never counted as covered. Every detail open and every check re-fetched pages 2–5, so about 15 opens used up the 60/h GitHub quota. It now counts as covered while fresh.
- [x] **nice**: GitHub's secondary rate limit (403/429 with `Retry-After`) wasn't recognised, so the next calls went out straight away. It now pauses GitHub calls.
- [x] **nice**: The release-note prefetch after a check had no deadline; it now stops after 1 min per image.

## B. Version resolution (PR #20) ✅

- [x] **should**: Digest matching only probes tags with the most version parts in the whole tag list, so one old `9.6.24` stops `postgres:latest` from ever matching `17.0`, and the running version stays unknown. `PG_VERSION` is also rejected for word tags, because "pg" isn't part of "postgres". Candidates are now picked by recency, and a tag is dropped only when a more precise one with the same prefix exists; `PG_VERSION` counts for `postgres`.
- [x] **should**: A purely numeric suffix token is dropped (`version.go`, "a purely numeric token is dropped"):
  - postgis `16-3.4` gets `17-3.4` as its latest, although `17-3.5` exists.
  - node-red `4.0.2-20` is offered `4.0.5-18`, which switches its Node runtime.
  - Date tags like `2025-01-01` never show an update.

  The suffix is now a sort key with the running line preferred, and date tags form their own family.
- [x] **should**: For word tags, a version label skips the family check. `foo:alpine` with label `1.2.3` is offered the plain image. `foo:nightly` with a label gets a stable "minor" update, which contradicts the channel-tag rule in [FEATURES §2](FEATURES.md). The label now counts only for the same variant, and never for channel tags.
- [x] **nice**: Calendar versions count as major (Home Assistant `2024.12.4` → `2025.1.0`), so every January is "breaking" by the version jump alone. A year rollover is now minor; the notes decide.
- [x] **decision**: Reviews on channel tags. A new build of a channel tag (`nightly`) should end a review or a snooze by version, because that build *is* the update. A rebuild of the same version (official images every few weeks) keeps the review: there are no new notes to read. Decided with the owner, and extended to floating tags without a known latest version: reviews there are set against the build (the tag's registry digest).

## C. Docker inventory (PR #17) ✅

- [x] **should**: After `docker pull` without recreating, Docker reports the container's image as a bare `sha256:…`, and the old image has no tag left. The check failed with "docker.io/library/sha256: not found". It now falls back to the image's `RepoDigest` and finds the version by digest.
- [x] **nice**: One image inspect failing with anything other than 404 failed the whole run; now only that container's check fails.

## D. Registry resilience (`registry`, `resolve`)

- [ ] **should**: During digest probing, go-containerregistry retries a 429 (after 1 s, then 3 s) for each tag, for up to 30 tags. That can reach the 2-minute container timeout, and the per-host slots stay taken the whole time. Stop probing on the first classified error; consider not retrying 429 at all.
- [ ] **should**: A stale (rate-limited) tag list isn't cached, so every group on the same repo asks the refusing registry again in the same run. Cache it briefly, and add singleflight per key so concurrent workers don't fetch the same tag list twice.

## E. Dashboard refresh (PR #18) ✅

- [x] **should**: One failed poll or reload replaced the whole page with the error card and stopped polling, so the "Check finished" toast never fired. Now the last good data stays, with a "Couldn't refresh" callout (dashboard and detail page), and polling goes on.
- [x] **should**: Scheduled and startup checks never showed up until a reload. The dashboard now refreshes every minute.
- [x] **nice**: "Couldn't start a check" now shows the server's error when there is one.

## F. Hardening (PR #21) ✅

- [x] **should**: When the first icon index download fails (no network yet at boot), there are no icons for 24 h. It now retries after 1, 5 and 15 min.
- [x] **should**: `go.mod` pins Go 1.26.1, which has 24 reachable standard-library vulnerabilities according to govulncheck. The release image isn't affected because it builds with the newest 1.26; bumped to 1.26.10; govulncheck now finds nothing. The 17 vulnerabilities in third-party dependencies aren't reachable from Tidemark's code.
- [x] **nice**: Any icon download error, including a short network blip, blocks that icon for an hour. Only 404/403 answers are cached now.
- [x] **nice**: `POST /api/checks` can be triggered cross-site (a plain form post needs no CORS preflight); require a custom header. No page sets `X-Frame-Options` / `frame-ancestors` or `nosniff`, so the UI can be framed by another site. Writes now need `X-Tidemark: 1`, and every response sets both headers.
- [x] **nice**: Shutdown doesn't wait for a running check before it closes the database, so the run row never gets its finish time. Nothing gets corrupted. Shutdown now waits up to 30 s for it.
- [x] **nice**: Duplicate rule keys after trimming (`app` and `/app`) merge in random order. They are now rejected like empty keys.
- [x] **optional**: Remote images in release notes load from third-party hosts, which reveals the viewer's IP to them. Images now become links (decided with the owner).

## Checked and found sound

- **Privacy:** only `/containers/json?all=1` and `/images/{id}/json` are called, and only `*_VERSION` env variables are kept, the same in `scripts/snapshot.sh`.
- **Label inheritance and rebuilt official images:** handled as described in FEATURES §2, apart from the word-tag gap in B.
- **Check runs:** the worker pool, per-container failure isolation, keeping the last good result, timeouts and the per-host semaphore are all sound.
- **Primary GitHub rate limit:** ETags and the quota display work.
- **API:** routing, error codes, the 4 KiB body limit with a strict decoder, and response shapes match ARCHITECTURE.
- **SPA serving:** there's no path traversal, and the cache headers are right.
- **SQLite:** migrations are ordered and idempotent. WAL, `busy_timeout` and a single connection rule out deadlocks.
- **Rules:** parser precedence and problem reporting match FEATURES §8.
- **Config:** defaults match the README.
- **Icons:** downloads have size limits and timeouts, and there's no SSRF from request input.
- **Frontend:** the markdown bundle is loaded only from lazy routes, and the only `innerHTML` is DOMPurify output. Every `httpResource` goes through `hasValue()`. API types match the backend. Accessibility (stretched links, `aria-pressed`, a native `<dialog>`, live toasts, status as icon + word) is in place, and DESIGN-SYSTEM matches `ui/`.
