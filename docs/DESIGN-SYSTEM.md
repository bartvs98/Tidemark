# Design system

The visual rules are in [BRAND.md](BRAND.md). This page covers how they're built in code. To see the pieces live in both themes, run the app and open **`/styleguide`** (a few aren't on it yet, see [below](#not-on-the-styleguide-yet)).

## Where things live

| What | File |
|---|---|
| Tokens (colour, type, radius, elevation) | [web/src/styles/tokens.css](../web/src/styles/tokens.css) |
| Plain-HTML patterns: `.tm-table`, `.tm-table-stack`, `.tm-row-link`, `.tm-tabs`, `.tm-prose` (+ `.tm-flag-breaking` / `.tm-flag-attention`), `.tm-shimmer` | [web/src/styles/components.css](../web/src/styles/components.css) |
| Angular components | [web/src/app/ui/](../web/src/app/ui/) (import from `../ui`, except `ui/markdown`) |
| Container pieces (take a `ContainerSummary`) | [dashboard/update-info.ts](../web/src/app/dashboard/update-info.ts), [dashboard/container-tile.ts](../web/src/app/dashboard/container-tile.ts) |
| Live reference | [web/src/app/styleguide/](../web/src/app/styleguide/) → `/styleguide` |

## Tokens

The tokens are Tailwind v4 `@theme` variables, so each one is both a CSS variable (`var(--color-surface)`) and a set of utilities (`bg-surface`, `text-surface`, `border-surface`).

**Use semantic tokens in components and never primitives.** The `deep-*` and `tide-*` scales exist only to define the semantic tokens.

| Group | Tokens | Utilities, for example |
|---|---|---|
| Surfaces | `canvas` (page), `surface` (cards), `raised` (toasts), `hover` | `bg-canvas`, `bg-surface`, `hover:bg-hover` |
| Lines | `line`, `line-strong` | `border-line` |
| Text | `fg`, `fg-muted`, `fg-subtle` | `text-fg-muted` |
| Accent | `accent`, `accent-hover`, `on-accent`, `accent-text`, `accent-soft`, `focus` | `bg-accent text-on-accent` |
| Status | `ok`, `attention(-soft/-line)`, `breaking(-soft/-line)` | `bg-attention-soft text-attention border-attention-line` |
| Fonts | `--font-sans` Geist, `--font-mono` Geist Mono | `font-mono` for versions, tags, digests |
| Type | `text-xs` 12 · `sm` 13 · `base` 14 · `lg` 16 · `xl` 20 · `2xl` 24 · `3xl` 32 | `text-base` is body text |
| Radius | `sm` 4 · `md` 6 · `lg` 10 · `xl` 14 | `rounded-md` for controls, `rounded-lg` for cards |
| Elevation | `shadow-raised` (cards), `shadow-overlay` (toasts, popovers) | |
| Spacing | Tailwind's default 4 px scale | `gap-2`, `px-4` |

### Light and dark

Every colour is written once as `light-dark(<light>, <dark>)`. The theme switch only sets `<html data-theme="dark|light|system">`, and that maps to `color-scheme`. So there's no `dark:` variant anywhere, and a new component gets both themes for free.

- The default is **dark**. `system` follows the OS.
- The choice is saved in `localStorage` (`tidemark.theme`) and applied by an inline script in `index.html` before first paint, so the page doesn't flash the wrong theme.

## Components

| Component | Use |
|---|---|
| `button[tmButton]`, `a[tmButton]` | Inputs: `variant` (primary, secondary, ghost, danger), `size` (sm, md), `icon`, `square` (icon-only, needs `aria-label`), `loading`. It styles the native element, so `disabled`, `type`, `href` and `routerLink` all still work. |
| `tm-badge` | Inputs: `tone` (neutral, accent, ok, attention, breaking, unknown), `icon`, `mono`. A generic label. `unknown` has a dashed border ("check failed", "rule skipped", "image missing"). |
| `tm-update-class` | `kind`: none, patch, minor, major or digest. `none` is a small green check + "Up to date" (no badge); the others a **neutral** badge with a tide dot: "patch", "minor", "major" or "new image" (digest). |
| `tm-level` | `level`: none, attention, breaking or unknown. Renders **nothing** for `none`; `attention` is an amber "Attention" badge (triangle icon), `breaking` a coral "Breaking" badge (octagon icon), `unknown` a dashed "Unknown" badge. The API never sends `unknown`: a failed check shows a dashed `tm-badge tone="unknown"` "check failed" instead (see `tm-container-update`). |
| `tm-version-diff` | `from`, optional `to`, shown as `1.4.2 → 1.6.0` in mono. Without `to` it shows just the running version. |
| `tm-card` | `heading`, `flush`, and a `[tmCardActions]` slot for header buttons. |
| `tm-empty-state` | `heading` (required), `description`, `icon` (default `box`). Buttons go in as content. |
| `tm-skeleton` | `width` (default `100%`) and `height` (default `1rem`). A highlight sweeps across; with reduced motion it becomes a gentle fade instead. `aria-hidden`. |
| `Toasts` + `<tm-toaster>` | `inject(Toasts).show(msg, tone, ms)` returns an id; `dismiss(id)` closes it. Tones: info (default), ok, attention, breaking, each with its own icon. `ms` defaults to 5000; `ms <= 0` stays until dismissed. Breaking toasts use `role="alert"` and the others `role="status"`. One `<tm-toaster />` lives in the app shell (bottom-right, `aria-live="polite"`). |
| `tm-icon` | `name`, `size` (default 16). A small set of Lucide icons; add new ones in `icon.ts`. `ICON_NAMES` lists them all. |
| `tm-callout` | `tone` (info by default, attention, breaking; `breaking` gets `role="alert"`), optional `heading`, content. Each tone has a fixed icon. Explains what's missing and what to do next, in context. |
| `tm-clamp` | `max` (CSS length, default `40rem`). Caps tall content with a fade and "Show all" / "Show less"; content at most a third taller than the cap is shown whole. Used for long release notes (not for notes with flagged lines, which must stay visible). |
| `tm-markdown` | `markdown`, `githubRepo`, `flags` (`{line, level}[]` from the API). Flagged source lines get a highlighter in the level's colour (`.tm-flag-breaking` / `.tm-flag-attention` on the heading, paragraph or list item). Renders untrusted markdown (release notes) sanitised with DOMPurify, styled by `.tm-prose`. **Import it from `ui/markdown`, not the `ui` barrel**, and only in lazy routes: it pulls in marked + DOMPurify (~28 KB gz). |
| `tm-app-icon` | `icon` (`{src, light?, dark?}`), `name`, `size` (default 24). An app's logo, with a letter avatar when there's no icon or it fails to load. Swaps to the light/dark variant when the logo would disappear on the current theme. Decorative (`aria-hidden`); always show the name next to it. |
| `tm-theme-switch` | System, light or dark, as an icon-only `tm-segmented`. Lives in the header. Backed by the exported `Theme` service: `preference` (signal) and `isDark()` (resolves `system` via `prefers-color-scheme`), which `tm-app-icon` uses. |
| `tm-section` | `heading`, `icon`, `meta`, and a `[tmSectionActions]` slot. A titled group such as a Compose stack, announced as a named region. `collapsible` makes the heading a disclosure button (`aria-expanded`, two-way `[(open)]`, closed by default); `muted` tones the header down. Both are used for the dashboard's "Ignored" section. |
| `tm-segmented` | `options` (`{value, label, icon, count}`), two-way `[(value)]`, `label` (required: the group's `aria-label` on `role="group"`), `iconOnly`. Mutually exclusive toggles such as list/tiles view, theme, or a filter (`count` shows muted after the label: "Updates 11"). Buttons use `aria-pressed`; with `iconOnly` each label becomes the button's `aria-label` and `title`. |
| `tr.tm-row-link` | A table row that opens a link: put `after:absolute after:inset-0` on one link inside (stretched link). Things that must stay hoverable on their own get `relative z-10`. |
| `table.tm-table` | A plain table, which keeps it accessible by default. For tables split across sections, see [Tables across sections](#tables-across-sections). Add `tm-table-stack` (and make any `min-w` `sm:` only) to stack rows below 640px: cells with `data-stack="full"` take the first line, `"hide"` drop out, `"indent"` grows and lines up under the name, the rest share the second line. |
| `TM_TABS` + `.tm-tabs` | [Angular Aria](https://angular.dev/guide/aria/tabs) headless tabs (`ngTabs`, `ngTabList`, `ngTab`, `ngTabPanel`), styled by the class. Keyboard navigation and ARIA come from Aria. |

### Tables across sections

The dashboard's list view is one `tm-section` + `tm-card flush` + `table.tm-table` per Compose stack ([dashboard.html](../web/src/app/dashboard/dashboard.html)). To make them read as one table:

- **Shared widths.** Every table is `table-fixed` with a `<colgroup>` built from one constant (`COLUMNS` in [dashboard.ts](../web/src/app/dashboard/dashboard.ts)), so columns line up across stacks.
- **One column bar.** A single `aria-hidden` header table sits above all sections, `sticky top-14` (just under the app header) and hidden below `sm`.
- **Headers stay for screen readers.** Each table keeps its own `<thead>` (`th scope="col"`), made `sm:sr-only`. Below 640px `tm-table-stack` hides it and stacks the rows.
- **Rows are links.** Each row is `tr.tm-row-link` with a stretched link on the container name.

## Container pieces

Domain composites that take a `ContainerSummary`. They live next to the dashboard because they depend on its API types, not in `ui/`, but they're part of the design system: the list and tile views both use them, and `/styleguide` shows them. All take `container` (required); `tm-container-version`, `tm-container-update` and `tm-container-tile` also take `checking`, which shows a skeleton while there's no result yet.

| Component | Shows |
|---|---|
| `tm-container-version` | Running → latest via `tm-version-diff` (latest only when there's an update; `? → latest` when the running version is unknown). Below it, in small subtle text: a **source hint** when the version didn't come from the tag ("detected from image label", "detected by image digest", "detected from image metadata", "running version unknown"), **"or `X` in your major"** when there's a newer version inside the running major, and the **held line** when a rule keeps a newer version back ("`1.5.0` held back by pin `1.4`" / "… held back, majors off", reason on hover). Ignored or not checked yet: just the tag in mono. |
| `tm-container-update` | `tm-update-class` + `tm-level` (reasons on hover). **Ignored:** subtle eye-off icon + "Ignored" (who ignored it on hover). **Check failed:** dashed `tm-badge tone="unknown"` "check failed" (the error on hover). |
| `tm-rule-problems` | Dashed "rule skipped" / "N rules skipped" badge when rules were dropped for invalid values (the problems on hover). Nothing otherwise. Not a risk, so no colour. |
| `tm-container-tile` | One container in tile view: app icon, name, state badge (when not running), rule-skipped badge, `image:tag` and status, then version and update in the footer. The whole tile is a stretched link to the detail page; keyboard focus outlines the tile. |

Anything with a tooltip inside a stretched-link row or tile gets `relative z-10` so it stays hoverable.

### Not on the styleguide yet

`/styleguide` doesn't show these yet: a `tr.tm-row-link` example, `tm-card` with `heading` / `[tmCardActions]`, `tm-theme-switch` (only in the header), `tm-container-tile`, and the sticky column bar. Its "Stack sections" sample is out of date; [Tables across sections](#tables-across-sections) describes what the dashboard does.

## Rules

1. **Colour means risk.** Patch, minor and major are neutral. Only `attention` and `breaking` get colour. Don't add coloured badges for anything else. (`tm-badge` has `accent` and `ok` tones, but update info doesn't use them; the only green there is the small "Up to date" check.)
2. **Never colour alone.** Every status has an icon **and** a word.
3. **Versions, tags, digests and image refs are always mono.**
4. **Build from the pieces.** If a screen needs something new, add it to the design system and to `/styleguide` first, then use it. Generic pieces go in `ui/` (exported from `ui/index.ts`). Domain composites that take a `ContainerSummary` live next to the dashboard ([Container pieces](#container-pieces)), but are documented here and shown on `/styleguide` all the same.
5. **Accessibility:** use native elements first (`button`, `a`, `table`), use Angular Aria for composite widgets (tabs, menus, listboxes), give every icon-only button an `aria-label`, and never remove the visible focus ring (`:focus-visible`, 2 px `focus` colour).
