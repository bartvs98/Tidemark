# Tidemark: Brand

<img src="brand/tidemark-icon.svg" width="64" alt="Tidemark icon"> &nbsp; <img src="brand/tidemark-wordmark.svg" height="48" alt="Tidemark wordmark">

**Know what changed before you update.**

## Name

A *tidemark* is the line left behind showing how high the water reached. In Tidemark, the water is where your containers are now and the mark is how far upstream has moved. The gap between them, and what's in it, is the whole product. It also hints at the second job: *marking* an update as reviewed.

Usage: **Tidemark** in prose, `tidemark` for the binary, image, labels (`tidemark.ignore`) and env vars (`TIDEMARK_*`). Never "TideMark" or "Tide Mark".

## Logo

| Asset | File | Use |
|---|---|---|
| Mark | [brand/tidemark-mark.svg](brand/tidemark-mark.svg) | `currentColor`, any background. The app header inlines it two-tone instead (line `fg`, wave `accent`), matching the app icon |
| App icon | [brand/tidemark-icon.svg](brand/tidemark-icon.svg) | Favicon (`web/public/favicon.svg` is a copy), Docker Hub / GitHub avatar. PWA icon later (no web manifest yet) |
| Wordmark | [brand/tidemark-wordmark.svg](brand/tidemark-wordmark.svg) | Docs. Mid-tone colours (deep-400 line, tide-500 wave, deep-500 text), so it reads on light and dark |
| README banner | [brand/banner-dark.png](brand/banner-dark.png), [brand/banner-light.png](brand/banner-light.png) | README header, picked by `prefers-color-scheme`: two-tone mark, wordmark and tagline in Geist over three layers of tide (the backdrop waves, made visible). Rendered with the README screenshot ([brand/screenshot.png](brand/screenshot.png), dashboard in list view grouped by stack, dark mode) by `node scripts/dev/readme-images.mjs`, against the made-up host in `scripts/dev/demo-snapshot.json` (nine popular images in four stacks, two of them fully up to date) so no real server shows up |

The mark has two strokes: a straight **line** (the high-water mark, upstream) above a **wave** (your running state). Both are 2.5 px on a 32 px grid with round caps. Keep them the same weight, never add a third element, and leave at least 4 px clear space at 32 px.

The wordmark uses lowercase live text (`tidemark`) in Geist, so it renders in the fallback font until Geist is installed. Convert it to outlines before using it outside the app.

## Colour

Dark mode is the default because home-lab dashboards usually live in dark mode, but light mode is fully supported.

### Deep (neutrals, slightly blue): dark mode

| Token | Hex | Typical use |
|---|---|---|
| deep-950 | `#070D17` | Dark page background, text on accent (dark) |
| deep-900 | `#0B1726` | Dark surface, icon tile |
| deep-800 | `#13233A` | Dark raised surface (toasts) |
| deep-700 | `#1E3350` | Dark borders |
| deep-600 | `#33496A` | Strong border (dark) |
| deep-500 | `#536785` | Subtle / disabled text (dark) |
| deep-400 | `#7C8DA6` | Secondary text (dark) |
| deep-300 | `#A9B6C8` | — |
| deep-200 | `#CDD6E2` | — |
| deep-100 | `#E6EBF2` | — |
| deep-50  | `#F4F7FA` | Primary text (dark) |

Dark hover `#112038` sits outside the scale, between deep-900 and deep-800.

### Paper (warm neutrals): light mode

Light mode is warm paper with softer ink rather than a cool near-white: easier on the eyes, and the tide water still shows.

| Role | Hex |
|---|---|
| Page background (`canvas`, also the phone browser bar) | `#EFECE6` |
| Surface (cards, tables) | `#F8F6F2` |
| Raised (toasts, popovers) | `#FBFAF7` |
| Hover / pressed / skeleton | `#E5E1D9` |
| Border / strong border | `#D9D3C7` / `#BCB4A5` |
| Primary text | `#1E2531` |
| Secondary text (≥ 4.5:1 on all three backgrounds) | `#58606E` |
| Subtle text (placeholders, arrows, disabled) | `#7F838B` |

### Tide (brand accent)

| Token | Hex | Use |
|---|---|---|
| tide-300 | `#5EEAD4` | Accent text and links, accent hover on dark |
| tide-400 | `#2DD4BF` | Accent on dark: primary button bg (dark text on it), focus ring |
| tide-500 | `#14B8A6` | Wordmark only |
| tide-600 | `#0D9488` | Focus ring on light (≥ 3:1, UI only) |
| tide-700 | `#0F766E` | Accent on light: primary button bg (white text on it) |

Outside the palette on light: accent hover `#0B5F58`, and accent text and links `#0C6A62` (≥ 4.5:1 on the paper backgrounds).

### Status

**Colour means risk, nothing else.** Update *class* (patch, minor, major, digest) is shown as a neutral label. Only the *level* (none, attention, breaking) gets colour. A dashboard of 20 containers with a dozen patch updates should look calm, and the one breaking change should stand out.

| Meaning | Dark mode | Light mode | Shown as |
|---|---|---|---|
| Up to date | `#3FB97F` | `#1A6B43` | Small green check + the word "Up to date", no badge |
| Update, no risk | tide-400 | tide-700 | Neutral class badge + tide dot |
| Attention | `#F5B83D` | `#875A00` | Amber badge "Attention" + highlighted lines |
| Breaking | `#F2665E` | `#B5302A` | Coral badge "Breaking" + highlighted lines |
| Unknown / error | deep-400 | secondary text | Muted text, dashed border: "check failed" (never checked successfully), "rule skipped", "image missing" |
| Kept result | — | — | Small muted text + history icon "refresh failed", no badge or colour: it must not hide the result shown |
| Reviewed | — | — | Check icon + "Reviewed" in small muted text, class and level badges faded; no colour of its own (it isn't a risk) |
| Snoozed | — | — | Clock icon on the "Snoozed" section, "until 3 Nov" / "until newer than 1.6.0" under the version, badges faded; no colour |

## Typography

| Role | Font | Notes |
|---|---|---|
| UI | **Geist** (variable, OFL) | 14 px base, 600 for headings |
| Versions, tags, digests, code | **Geist Mono** | Always mono: `1.4.2 → 1.6.0`, `sha256:3f9a…` |

Both are bundled with the app (`@fontsource-variable/geist`, `@fontsource-variable/geist-mono`), not loaded from a CDN, so the app works offline on the LAN.

Type scale (px): 12 · 13 · 14 · 16 · 20 · 24 · 32 · 40. Tabular numerals in tables.

## Motion

Decorative motion is the tide: the tide gauge (the logo as a gauge: line = latest version, water = how much of it you run) and the faint tide behind every page drift in slow loops (seconds to a minute), and a breaking or attention detail page gets a slowly circling border in the level's colour. UI feedback (skeleton shimmer, tooltip fade, toasts, the water easing to a new level) stays short and subtle. Nothing blinks or pulses, and decorative motion stops when the OS asks for reduced motion.

## Voice

Calm, factual, short, like a tide report rather than an alarm. Say what changed, then what to do.

| Do | Don't |
|---|---|
| "3 updates · 1 needs attention" | "⚠️ WARNING: 3 OUTDATED CONTAINERS!" |
| "Breaking: config key `DB_URL` renamed to `DATABASE_URL` in 2.0.0" | "This update may break things" |
| "Couldn't fetch the changelog, GitHub rate limit. Add a GitHub token in Settings to raise it." | "Error 403" |
| "Up to date" | "Congrats! You're all caught up 🎉" |
| "Reviewed up to 1.6.0" | "Dismissed" |
| "No open updates" (only reviewed or snoozed ones left) | "All up to date" (it isn't) |

Running → latest is always written with an arrow: `1.4.2 → 1.6.0`.
