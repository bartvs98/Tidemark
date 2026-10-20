---
name: verify
description: Build Tidemark, run the real binary against the home-server snapshot and screenshot the UI in Chrome/Firefox, dark/light, list/tiles and desktop/phone widths, reporting page errors and failed requests. Use after any UI or API change before reporting it done, when asked to run, show, screenshot or check the app, or to reproduce a visual or browser-specific bug.
---

# Verify Tidemark visually

The tool is `scripts/dev/verify.mjs` (dev-only, own `package.json`; Puppeteer drives the locally installed browsers).

## Run it

First time in a checkout: `npm install --prefix scripts/dev`

```sh
node scripts/dev/verify.mjs                     # dashboard + styleguide × dark/light × list/tiles, Chrome, 1280px
node scripts/dev/verify.mjs --widths 1280,390   # add phone width (do this for any layout change)
node scripts/dev/verify.mjs --browsers chrome,firefox --pages dashboard --views tiles
node scripts/dev/verify.mjs --no-build          # reuse the last build when only re-shooting
node scripts/dev/verify.mjs --serve             # build + run the server only, for manual poking (stop with Ctrl+C / background it)
node scripts/dev/verify.mjs --data <dir>        # start from a copy of an earlier run's data dir (stored notes: no GitHub calls)
node scripts/dev/verify.mjs --scenario scripts/dev/scenarios/review.json --themes dark   # seeded states, named + cropped shots
node scripts/dev/verify.mjs --help
```

Pages are names (`dashboard`, `styleguide`) or paths. It builds the SPA and the Go binary, starts the server on `127.0.0.1:18080` with `TIDEMARK_DOCKER_SNAPSHOT=snapshots/home.json` (falls back to the test fixture), a fresh data dir and no schedule, waits for the startup update check, then shoots full-page PNGs.

## Then actually look

1. Output is in `.verify/<timestamp>/` (git-ignored): `<browser>-<page>-<view>-<theme>-<width>.png`, `report.json`, `server.log`.
2. **Read the screenshots that matter for the change** (Read tool on the PNG paths). Check both themes, and phone width for layout changes. Don't report a UI change as done without having looked.
3. The summary lists problems: `pageerror`/`console` (exit code 1, fix these), `http` (failed requests: icons the CDN refuses are expected occasionally; anything under `/api/` other than icons is a bug).
4. For a single element, crop by writing a small Puppeteer snippet that reuses `--serve`, or read the full-page PNG.

## Notes

- Takes ~30 s with a build (the update check against real registries is ~10 s of that). Needs network for registry checks and icons.
- In Git Bash, arguments starting with `/` get rewritten into Windows paths; the tool undoes that, but page names avoid the issue.
- Port busy? Pass `--port 18081` (another verify or `--serve` may still be running).
- Chrome/Firefox not at the default Windows paths: set `CHROME_PATH` / `FIREFOX_PATH`.
