#!/usr/bin/env node
// Visual verification for Tidemark: builds the app, runs the real binary against a
// snapshot, and screenshots pages across browsers, themes, views and widths.
//
//   node scripts/dev/verify.mjs                       # dashboard + styleguide, dark+light, list+tiles, chrome, 1280px
//   node scripts/dev/verify.mjs --pages dashboard --views tiles --widths 1280,390 --browsers chrome,firefox
//   node scripts/dev/verify.mjs --no-build            # reuse the last build
//   node scripts/dev/verify.mjs --serve               # build + run the server only (Ctrl+C to stop)
//   node scripts/dev/verify.mjs --data .verify/serve/data   # start from a copy of stored results + notes
//
// Output: .verify/<run>/ with PNGs and report.json (git-ignored). Exit code 1 on page errors.
// Needs a local Chrome (and Firefox for --browsers firefox); override with CHROME_PATH / FIREFOX_PATH.

import { spawn, spawnSync } from 'node:child_process';
import { cpSync, existsSync, mkdirSync, writeFileSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import process from 'node:process';

const ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '../..');
const WIN = process.platform === 'win32';

// ── options ──────────────────────────────────────────────
const args = process.argv.slice(2);
const flag = (name) => args.includes(`--${name}`);
const opt = (name, fallback) => {
  const i = args.indexOf(`--${name}`);
  return i >= 0 && args[i + 1] && !args[i + 1].startsWith('--') ? args[i + 1] : fallback;
};
const list = (name, fallback) => opt(name, fallback).split(',').map((s) => s.trim()).filter(Boolean);

if (flag('help')) {
  console.log(`Usage: node scripts/dev/verify.mjs [options]
  --pages dashboard,styleguide page names or paths (/styleguide)
  --themes dark,light          dark | light | system
  --views list,tiles           dashboard views (only used for /)
  --widths 1280                viewport widths, e.g. 1280,390
  --browsers chrome            chrome,firefox
  --snapshot snapshots/home.json   falls back to internal/docker/testdata/snapshot.json
  --port 18080
  --out .verify/<timestamp>
  --no-build                   skip ng build + go build
  --no-wait                    don't wait for the startup update check to finish
  --data <dir>                 start from a copy of an earlier data dir (results, release notes),
                               e.g. .verify/serve/data: no GitHub requests when the quota is low
  --serve                      only build and run the server until Ctrl+C`);
  process.exit(0);
}

// Pages by name ("dashboard", "styleguide") or path ("/styleguide"). Git Bash rewrites
// arguments starting with "/" into Windows paths ("C:/Program Files/Git/styleguide"); undo that.
const pages = list('pages', 'dashboard,styleguide').map((p) => {
  const mangled = p.match(/^[A-Za-z]:[\\/].*?[\\/]Git([\\/].*)?$/);
  if (mangled) p = (mangled[1] || '/').replace(/\\/g, '/');
  if (p === 'dashboard') return '/';
  return p.startsWith('/') ? p : '/' + p;
});
const themes = list('themes', 'dark,light');
const views = list('views', 'list,tiles');
const widths = list('widths', '1280').map(Number);
const browsers = list('browsers', 'chrome');
const port = Number(opt('port', '18080'));
const stamp = new Date().toISOString().replace(/[:.]/g, '-').slice(0, 19);
const out = resolve(ROOT, opt('out', join('.verify', stamp)));
const base = `http://127.0.0.1:${port}`;

let snapshot = opt('snapshot', 'snapshots/home.json');
if (!existsSync(resolve(ROOT, snapshot))) {
  snapshot = 'internal/docker/testdata/snapshot.json';
  console.log(`(no real snapshot found, using the test fixture ${snapshot})`);
}

const BROWSER_PATHS = {
  chrome: [
    process.env.CHROME_PATH,
    'C:/Program Files/Google/Chrome/Application/chrome.exe',
    'C:/Program Files (x86)/Google/Chrome/Application/chrome.exe',
    '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
    '/usr/bin/google-chrome',
    '/usr/bin/chromium',
  ],
  firefox: [
    process.env.FIREFOX_PATH,
    'C:/Program Files/Mozilla Firefox/firefox.exe',
    '/Applications/Firefox.app/Contents/MacOS/firefox',
    '/usr/bin/firefox',
  ],
};

// ── build ────────────────────────────────────────────────
mkdirSync(out, { recursive: true });
const bin = join(out, WIN ? 'tidemark.exe' : 'tidemark');

function run(cmd, cmdArgs, cwd, label) {
  const t = Date.now();
  // npx is a .cmd shim on Windows and needs a shell; other commands must not get one,
  // because the shell would split paths containing spaces ("My Files").
  const useShell = WIN && cmd === 'npx';
  const r = useShell
    ? spawnSync([cmd, ...cmdArgs].join(' '), { cwd, shell: true, encoding: 'utf8' })
    : spawnSync(cmd, cmdArgs, { cwd, encoding: 'utf8' });
  if (r.status !== 0) {
    console.error(`✗ ${label} failed:\n${r.stdout}\n${r.stderr}`);
    process.exit(1);
  }
  console.log(`✓ ${label} (${((Date.now() - t) / 1000).toFixed(1)}s)`);
}

if (!flag('no-build')) {
  run('npx', ['ng', 'build'], join(ROOT, 'web'), 'ng build');
  run('go', ['build', '-o', bin, './cmd/tidemark'], ROOT, 'go build');
} else if (!existsSync(bin)) {
  run('go', ['build', '-o', bin, './cmd/tidemark'], ROOT, 'go build (no binary in this --out yet)');
}

// ── server ───────────────────────────────────────────────
// A copied data dir with a check younger than an hour skips the startup check (and its GitHub calls).
const seed = opt('data', '');
if (seed) {
  if (!existsSync(resolve(ROOT, seed))) {
    console.error(`--data: ${seed} doesn't exist`);
    process.exit(1);
  }
  cpSync(resolve(ROOT, seed), join(out, 'data'), { recursive: true });
}

const server = spawn(bin, [], {
  cwd: ROOT,
  env: {
    ...process.env,
    TIDEMARK_ADDR: `127.0.0.1:${port}`,
    TIDEMARK_DOCKER_SNAPSHOT: snapshot,
    TIDEMARK_DATA_DIR: join(out, 'data'),
    TIDEMARK_SCHEDULE: 'off',
  },
  stdio: ['ignore', 'pipe', 'pipe'],
});
let serverLog = '';
server.stdout.on('data', (d) => (serverLog += d));
server.stderr.on('data', (d) => (serverLog += d));
const stop = () => {
  if (!server.killed) server.kill();
};
process.on('exit', stop);
process.on('SIGINT', () => process.exit(130));

async function waitFor(fn, timeoutMs, what) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    try {
      if (await fn()) return true;
    } catch {
      // not ready yet
    }
    await new Promise((r) => setTimeout(r, 250));
  }
  console.warn(`! timed out waiting for ${what}`);
  return false;
}

if (!(await waitFor(async () => (await fetch(`${base}/api/health`)).ok, 15000, 'the server'))) {
  console.error(serverLog);
  process.exit(1);
}
console.log(`✓ server on ${base} (snapshot: ${snapshot}, data: ${join(out, 'data')})`);

if (flag('serve')) {
  console.log('Serving until Ctrl+C. Open the URL above.');
  await new Promise(() => {});
}

if (!flag('no-wait')) {
  const t = Date.now();
  const ok = await waitFor(async () => {
    const body = await (await fetch(`${base}/api/containers`)).json();
    return body.check && !body.check.running && body.check.finishedAt;
  }, 180000, 'the update check');
  if (ok) console.log(`✓ update check finished (${((Date.now() - t) / 1000).toFixed(1)}s)`);
}

// ── screenshots ──────────────────────────────────────────
const { default: puppeteer } = await import('puppeteer-core').catch(() => {
  console.error('puppeteer-core is missing: run `npm install --prefix scripts/dev` once.');
  process.exit(1);
});

// Requests that are supposed to fail: the styleguide shows the letter fallback for a broken icon;
// Chrome asks for /favicon.ico after the JSON page used to set preferences.
const EXPECTED = [/^\/api\/icons\/does-not-exist/, /^\/favicon\.ico$/];

const report = { base, snapshot, out, shots: [], problems: [] };
for (const browserName of browsers) {
  const executablePath = BROWSER_PATHS[browserName]?.find((p) => p && existsSync(p));
  if (!executablePath) {
    report.problems.push({ browser: browserName, error: 'browser not found (set CHROME_PATH / FIREFOX_PATH)' });
    continue;
  }
  const browser = await puppeteer.launch({ browser: browserName, executablePath, headless: true });
  const page = await browser.newPage();
  let current = '';
  page.on('pageerror', (e) => report.problems.push({ shot: current, kind: 'pageerror', error: e.message }));
  page.on('console', (m) => {
    // Failed loads are reported (once, with the URL) by the response handler below.
    if (m.type() === 'error' && current && !m.text().startsWith('Failed to load resource')) {
      report.problems.push({ shot: current, kind: 'console', error: m.text() });
    }
  });
  page.on('response', (r) => {
    const path = r.url().replace(base, '');
    if (r.status() >= 400 && current && !EXPECTED.some((re) => re.test(path))) {
      report.problems.push({ shot: current, kind: 'http', error: `${r.status()} ${path}` });
    }
  });

  for (const width of widths) {
    await page.setViewport({ width, height: 900 });
    for (const theme of themes) {
      for (const path of pages) {
        for (const view of path === '/' ? views : ['-']) {
          const slug = path === '/' ? 'dashboard' : path.replace(/^\//, '').replace(/\//g, '-');
          const name = [browserName, slug, view === '-' ? null : view, theme, width].filter(Boolean).join('-');
          // Set preferences on the origin first (cheap JSON page, not reported), then load the real page.
          current = '';
          await page.goto(`${base}/api/health`);
          await page.evaluate(
            (t, v) => {
              localStorage.setItem('tidemark.theme', t);
              if (v !== '-') localStorage.setItem('tidemark.view', v);
            },
            theme,
            view,
          );
          current = name;
          const t0 = Date.now();
          await page.goto(base + path, { waitUntil: 'domcontentloaded' });
          await page.waitForSelector('main > *', { timeout: 30000 }).catch(() => {});
          await page.waitForNetworkIdle({ idleTime: 400, timeout: 30000 }).catch(() => {});
          const ms = Date.now() - t0;
          const file = join(out, `${name}.png`);
          await page.screenshot({ path: file, fullPage: true });
          report.shots.push({ name, file, ms });
          console.log(`  ${name}.png  (${ms} ms)`);
        }
      }
    }
  }
  await browser.close();
}

writeFileSync(join(out, 'report.json'), JSON.stringify(report, null, 2));
writeFileSync(join(out, 'server.log'), serverLog);
const pageErrors = report.problems.filter((p) => p.kind === 'pageerror' || p.kind === 'console');
console.log(`\n${report.shots.length} screenshots in ${out}`);
for (const p of report.problems) console.log(`  ! ${p.shot ?? p.browser}: ${p.kind ?? ''} ${p.error}`);
stop();
process.exit(pageErrors.length ? 1 : 0);
