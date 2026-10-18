#!/usr/bin/env node
// Renders the README images into docs/brand/: the banner (dark + light) and a dashboard
// screenshot split diagonally into dark and light mode.
//
//   node scripts/dev/verify.mjs --serve --snapshot scripts/dev/demo-snapshot.json   # in another terminal
//   node scripts/dev/readme-images.mjs [--base http://127.0.0.1:18080] [--view list] [--group severity]
//
// The banner needs no server; skip the screenshot with --banner-only.

import { existsSync, readFileSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import process from 'node:process';
import puppeteer from 'puppeteer-core';

const ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '../..');
const OUT = join(ROOT, 'docs/brand');
const args = process.argv.slice(2);
const opt = (name, fallback) => {
  const i = args.indexOf(`--${name}`);
  return i >= 0 && args[i + 1] ? args[i + 1] : fallback;
};
const base = opt('base', 'http://127.0.0.1:18080');
const view = opt('view', 'list');
const group = opt('group', 'severity');

const chrome = [
  process.env.CHROME_PATH,
  'C:/Program Files/Google/Chrome/Application/chrome.exe',
  'C:/Program Files (x86)/Google/Chrome/Application/chrome.exe',
  '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
  '/usr/bin/google-chrome',
].find((p) => p && existsSync(p));
const browser = await puppeteer.launch({ executablePath: chrome });

// Same wave as web/src/app/ui/tide-gauge.ts (tideWavePath).
function wavePath(period, amp, width, depth = 400) {
  let d = `M0 ${amp} Q${period / 4} ${-amp} ${period / 2} ${amp}`;
  for (let x = period; x <= width + period; x += period / 2) d += ` T${x} ${amp}`;
  return { crest: d, fill: `${d} V${depth} H0Z` };
}

const font = (pkg, file) =>
  'data:font/woff2;base64,' + readFileSync(join(ROOT, 'web/node_modules/@fontsource-variable', pkg, 'files', file)).toString('base64');

const THEMES = {
  dark: { bg: '#0B1726', fg: '#F4F7FA', muted: '#A9B6C8', accent: '#2DD4BF', water: '45 212 191' },
  light: { bg: '#FFFFFF', fg: '#0B1726', muted: '#536785', accent: '#0F766E', water: '15 118 110' },
};

// ── banner ───────────────────────────────────────────────
const W = 1280;
const H = 320;
// Back to front, like the app's backdrop, but visible: [period, amp, top, opacity, shift].
const WAVES = [
  [820, 16, 0, 0.1, 0],
  [620, 12, 26, 0.14, 210],
  [480, 9, 52, 0.2, 90],
];

function bannerHtml(t) {
  const waves = WAVES.map(([period, amp, top, opacity, shift], i) => {
    const { crest, fill } = wavePath(period, amp, W + period);
    const front = i === WAVES.length - 1;
    return `<svg class="wave" style="top:${top}px" viewBox="${shift} 0 ${W} 140" preserveAspectRatio="none">
      <path d="${fill}" fill="rgb(${t.water} / ${opacity})"/>
      ${front ? `<path d="${crest}" fill="none" stroke="rgb(${t.water} / 0.5)" stroke-width="1.5"/>` : ''}
    </svg>`;
  }).join('');
  return `<!doctype html><html><head><style>
    @font-face { font-family: Geist; src: url(${font('geist', 'geist-latin-wght-normal.woff2')}); font-weight: 100 900; }
    html, body { margin: 0; background: transparent; }
    .banner { position: relative; width: ${W}px; height: ${H}px; overflow: hidden; border-radius: 16px;
      background: ${t.bg}; font-family: Geist, sans-serif; }
    .tide { position: absolute; left: 0; right: 0; bottom: 0; height: 120px; }
    .wave { position: absolute; left: 0; width: 100%; height: 140px; }
    .content { position: relative; display: flex; flex-direction: column; align-items: center; padding-top: 58px; }
    .logo { display: flex; align-items: center; gap: 18px; }
    .logo svg { width: 84px; height: 84px; }
    .name { font-size: 76px; font-weight: 600; letter-spacing: -2px; color: ${t.fg}; line-height: 1; padding-bottom: 8px; }
    .tagline { margin-top: 18px; font-size: 24px; font-weight: 450; color: ${t.muted}; letter-spacing: -0.2px; }
  </style></head><body>
    <div class="banner">
      <div class="tide">${waves}</div>
      <div class="content">
        <div class="logo">
          <svg viewBox="0 0 32 32" fill="none" stroke-linecap="round" stroke-width="2.5">
            <path d="M9 11h14" stroke="${t.fg}"/>
            <path d="M5 20.5c1.8-2.6 3.7-2.6 5.5 0s3.7 2.6 5.5 0 3.7-2.6 5.5 0 3.7 2.6 5.5 0" stroke="${t.accent}"/>
          </svg>
          <span class="name">tidemark</span>
        </div>
        <div class="tagline">Know what changed before you update.</div>
      </div>
    </div>
  </body></html>`;
}

const page = await browser.newPage();
await page.setViewport({ width: W, height: H, deviceScaleFactor: 2 });
for (const [name, t] of Object.entries(THEMES)) {
  await page.setContent(bannerHtml(t), { waitUntil: 'load' });
  await page.evaluate(() => document.fonts.ready);
  const file = join(OUT, `banner-${name}.png`);
  await (await page.$('.banner')).screenshot({ path: file, omitBackground: true });
  console.log(`✓ ${file}`);
}

// ── split screenshot ─────────────────────────────────────
if (!args.includes('--banner-only')) {
  const SW = 1280;
  const SH = 1000;
  const shots = {};
  await page.setViewport({ width: SW, height: SH, deviceScaleFactor: 2 });
  for (const theme of ['dark', 'light']) {
    await page.goto(base + '/', { waitUntil: 'networkidle0' });
    await page.evaluate(
      (t, v, g) => {
        localStorage.setItem('tidemark.theme', t);
        localStorage.setItem('tidemark.view', v);
        localStorage.setItem('tidemark.group', g);
      },
      theme,
      view,
      group,
    );
    await page.reload({ waitUntil: 'networkidle0' });
    await page.emulateMediaFeatures([{ name: 'prefers-reduced-motion', value: 'reduce' }]);
    // A server that just started runs its first check; wait for the results.
    await page.waitForFunction(() => !document.body.innerText.includes('Checking'), { timeout: 180000 });
    // The screenshot shows a made-up host: drop the "Snapshot" badge.
    await page.evaluate(() =>
      document.querySelectorAll('[data-testid=hero] tm-badge').forEach((b) => b.textContent.trim().startsWith('Snapshot') && b.remove()),
    );
    await new Promise((r) => setTimeout(r, 800));
    shots[theme] = 'data:image/png;base64,' + (await page.screenshot({ encoding: 'base64' }));
  }

  // Dark on the left, light on the right, split along a slight diagonal.
  await page.setContent(
    `<!doctype html><html><head><style>
      html, body { margin: 0; }
      .split { position: relative; width: ${SW}px; height: ${SH}px; overflow: hidden; }
      .split img { position: absolute; inset: 0; width: 100%; height: 100%; }
      .light { clip-path: polygon(56% 0, 100% 0, 100% 100%, 44% 100%); }
      .line { position: absolute; inset: 0; width: 100%; height: 100%; }
    </style></head><body>
      <div class="split">
        <img src="${shots.dark}"><img class="light" src="${shots.light}">
        <svg class="line" viewBox="0 0 ${SW} ${SH}" preserveAspectRatio="none">
          <line x1="${SW * 0.56}" y1="0" x2="${SW * 0.44}" y2="${SH}" stroke="#2DD4BF" stroke-width="2"/>
        </svg>
      </div>
    </body></html>`,
    { waitUntil: 'load' },
  );
  const file = join(OUT, 'screenshot.png');
  await (await page.$('.split')).screenshot({ path: file });
  console.log(`✓ ${file}`);
}

await browser.close();
