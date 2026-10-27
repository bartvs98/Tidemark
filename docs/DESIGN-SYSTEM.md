# Design system

How the visual rules of [BRAND.md](BRAND.md) are built in code. Every piece and state is live in both themes at **`/styleguide`**. This page says what each piece is for and what to watch out for; the inputs are in each component's `.ts` (typed `input()` declarations), the UI texts in its template.

## Rules

1. **Colour means risk:** only `attention` and `breaking` get colour; patch, minor and major are neutral. The only green in update info is the small "Up to date" check (`tm-badge`'s `accent` and `ok` tones aren't for update info).
2. **Never colour alone:** every status has an icon **and** a word.
3. **Versions, tags, digests and image refs are always mono.**
4. **Build from the pieces.** Something new goes into the design system and onto `/styleguide` first, then into the page. Generic pieces get a folder in `ui/` (exported from `ui/index.ts`); domain composites live next to their page.
5. **Accessibility:** native elements first (`button`, `a`, `table`), Angular Aria for composite widgets (tabs, menus, listboxes), an `aria-label` on every icon-only button, and never remove the focus ring (`:focus-visible`, 2 px `focus` colour).
6. **Motion** stops or turns into a fade under `prefers-reduced-motion`, in every piece that moves.

## Where things live

| What | Where |
|---|---|
| Tokens | [web/src/styles/tokens.css](../web/src/styles/tokens.css) |
| Plain-HTML patterns (`.tm-table`, `.tm-tabs`, `.tm-prose`, `.tm-tooltip`, `.tm-severity-frame`, `.tm-ai-sheen`, …) | [web/src/styles/components.css](../web/src/styles/components.css) |
| Generic components | [web/src/app/ui/](../web/src/app/ui/), one folder per piece: `name.ts` + `name.html`, `.css` only with styles, `.spec.ts` only with tests. Import from the `ui` barrel, **except `ui/markdown/markdown` and `ui/charts`** (they'd land in the main bundle; lazy routes only) |
| Domain pieces | [dashboard/](../web/src/app/dashboard/) (container pieces, text helpers in `format.ts`), [detail/](../web/src/app/detail/), [stats/](../web/src/app/stats/), [settings/](../web/src/app/settings/) |
| Live reference | [web/src/app/styleguide/](../web/src/app/styleguide/) → `/styleguide` |

## Tokens

Tailwind v4 `@theme` variables: each is a CSS variable (`var(--color-surface)`) and a set of utilities (`bg-surface`, `text-surface`, `border-surface`). **Use the semantic tokens, never the `deep-*` / `tide-*` primitives.**

| Group | Tokens |
|---|---|
| Surfaces | `canvas` (page), `surface` (cards), `raised` (toasts), `hover` |
| Lines | `line`, `line-strong` |
| Text | `fg`, `fg-muted`, `fg-subtle` |
| Accent | `accent`, `accent-hover`, `on-accent`, `accent-text`, `accent-soft`, `focus` |
| Water | `water`: translucent tide for the gauge and backdrop |
| Status | `ok`, `attention(-soft/-line)`, `breaking(-soft/-line)` |
| Fonts | `font-sans` Geist, `font-mono` Geist Mono |
| Type | `text-xs` 12 · `sm` 13 · `base` 14 (body) · `lg` 16 · `xl` 20 · `2xl` 24 · `3xl` 32 · `4xl` 40 (dashboard headline, detail name) |
| Radius | `sm` 4 · `md` 6 (controls) · `lg` 10 (cards) · `xl` 14 |
| Elevation | `shadow-raised` (cards), `shadow-overlay` (toasts, popovers) |
| Spacing | Tailwind's default 4 px scale |

**Light and dark:** every colour is written once as `light-dark(<light>, <dark>)`, and the theme switch only sets `<html data-theme="dark|light|system">` (→ `color-scheme`). So there's no `dark:` variant anywhere and new components get both themes for free. Default is dark. The choice is in `localStorage` (`tidemark.theme`), applied by an inline script in `index.html` before first paint. Scrollbars are styled once on `html`; don't restyle them per component.

- **Changing `canvas`?** Also change the two `THEME_COLORS` copies in `ui/theme-switch/theme.ts` (the phone browser bar's `theme-color`).

## Components

| Component | For / watch out |
|---|---|
| `button[tmButton]`, `a[tmButton]` | Styles the native element, so `disabled`, `type`, `href`, `routerLink` keep working. `square` (icon-only) needs an `aria-label`. |
| `tm-badge` | Generic label. Tone `unknown` (dashed) is for failures: "check failed", "rule skipped", "image missing". |
| `tm-ai-marker` | Neutral "AI" badge on model-written text. Not a risk, so no colour. |
| `tm-update-class` | The class as a neutral badge with a tide dot; `none` is the green check + "Up to date". |
| `tm-level` | The risk badge; renders nothing for `none`. The API never sends `unknown`: a never-checked container shows `tm-badge tone="unknown"` "check failed" instead. |
| `tm-version-diff` | `1.4.2 → 1.6.0` in mono, or just the running version. |
| `tm-card` | `flush`, `[tmCardActions]`. **Clips overflow**, so no `tm-menu` or `tm-select` inside. A grid-stretched card's body fills it, so content can centre. |
| `tm-empty-state` | Heading, description, icon; buttons as content. |
| `tm-skeleton` | Loading bar, `aria-hidden`. |
| `Toasts` + `<tm-toaster>` | `inject(Toasts).show(msg, tone, ms)` → id, `dismiss(id)`; `ms <= 0` stays. One toaster in the app shell. |
| `tm-icon` | A Lucide subset; add icons in `icon.ts`. `check` = reviewed, `clock` = snoozed. |
| `tm-callout` | Explains what's missing and what to do next, in context. `breaking` gets `role="alert"`. |
| `tm-clamp` | Caps tall content with a fade and "Show all". `marks` counts matching elements under the fade ("Show all · 2 flagged lines"); call `reveal(el)` before scrolling to a line inside. |
| `tm-markdown` | Untrusted markdown (release notes), sanitised, styled by `.tm-prose`. Images become links and never load. Flagged lines get `.tm-flag-breaking` / `.tm-flag-attention`, a native `title` (`tmTooltip` can't attach to rendered HTML) and `data-flag` indexes (`[data-flag~="2"]`). **Import from `ui/markdown/markdown`, lazy routes only** (~28 KB gz). |
| `tm-app-icon` | An app's logo, or a letter avatar; swaps light/dark variants. Decorative: always show the name next to it. |
| `tm-dialog` | Modal on native `<dialog>` (focus trap, Escape, backdrop). `size="lg"`: one fixed height for tabbed pages; `flush`: no body padding or scrolling. Content is always projected, so wrap what should load only while open in `@if`. Label/value rows: `<dl>` with `div.tm-setting`. |
| `tm-theme-switch` + `Theme` | The header switch; `Theme.isDark()` resolves `system`. |
| `PageVisibility` | Service: `visible()` is false while the tab is hidden. Polls pause on it and reload once when it turns true. |
| `tm-section` | A titled region, such as a stack. `collapsible` + `[(open)]` (closed by default), `muted`, `level` 3 for a sub-section. |
| `tm-menu` | A button with a short list of actions, on Angular Aria. Disabled items stay focusable so their `hint` shows. Absolutely positioned: **not inside a `tm-card`**. |
| `tm-segmented` | Mutually exclusive toggles (view, grouping, theme, a filter with counts). `size="md"` in toolbars. With `iconOnly` the labels become `aria-label` + tooltip. |
| `tm-search` | Search input; Escape clears, public `focus()` (the dashboard binds `/`). |
| `[tmTooltip]` | **Use instead of `title` everywhere.** Hover, focus or tap; one open at a time. Not on a stretched link (it would cover the row). Tests read `data-tooltip`. |
| `tm-tide-gauge` | The logo as a gauge (line = latest, water = `level`). Never amber or red. Dashboard: `hero`, up to date ÷ checked. Detail: `compact`, `max(0.15, 1 − releases behind / 12)`. |
| `tm-tide-backdrop` | The faint tide behind every page, mounted once in `app.html`. |
| `.tm-severity-frame` | `data-level="breaking"` or `"attention"` on a card: border plus two slow circling arcs (the level badge stays). `before:hidden` stills the arcs (reviewed or snoozed). Don't combine with `border-line`, which wins. |
| `tr.tm-row-link` | Row that opens a link: `after:absolute after:inset-0` on one link inside. Things that must stay hoverable get `relative z-10`. |
| `table.tm-table` (+ `tm-table-stack`) | Plain accessible table. `tm-table-stack` stacks rows below 640px via `data-stack` (`full`, `hide`, `indent`); make any `min-w` `sm:` only. |
| `TM_TABS` + `.tm-tabs` | Angular Aria tabs, styled. `tm-tabs-vertical` in a `size="lg" flush` dialog; bind `ngTabList`'s `orientation` to the same breakpoint. |

### Forms

Signal Forms (`form()` + `[formField]`); every control also works standalone with `[(value)]` / `[(checked)]`.

| Piece | For / watch out |
|---|---|
| `tm-field` | Wraps **one** control: label, then the control, then one message (`error` wins over `locked`, which wins over `hint`). Provides `TM_FIELD`, so ids and ARIA are wired without page code. `inline` for switches. |
| `input[tmInput]`, `textarea[tmInput]` | Directive on the native element. `mono` for cron specs, URLs, IDs, tokens. |
| `tm-password-input` | Eye button to show the value. For write-only secrets. |
| `tm-select` | Listbox on Angular Aria (`''` = nothing chosen). Absolutely positioned: not inside a `tm-card`. |
| `tm-switch` | `button role="switch"`. |
| `tm-secret-setting` ([settings/secret-setting/](../web/src/app/settings/secret-setting/)) | A secret in the settings dialog: Set / Not set, Replace, Clear, Undo. Two-way `value`: `undefined` untouched, a string being typed, `null` cleared on Save. |

- **Errors go through `tm-field`'s `error`**: map the API's `fields: {name: message}` onto it. A control's own `invalid` only draws the border once `touched`, and never shows text.
- **Locked fields** (set by an env var): `locked="Set by TIDEMARK_X"` on the field, and disable the control.
- **Secrets are write-only:** never fill an input with a stored value.
- **The settings dialog loads lazily** (`@defer (when settingsOpen())` in `app.html`).

### Charts

Own SVG pieces from `ui/charts` (not the barrel), with shared helpers in `ui/chart-tip/`.

- **Colour means risk here too:** marks are `accent`; `attention` / `breaking` only for levels, `muted` for "other". Segment names show in the legend and readout.
- **Marks:** 2 px lines; bars with 4 px rounded data ends and 2 px gaps; 3–4 recessive gridlines at 1/2/5 × 10ⁿ; axis text `text-xs` `fg-subtle`, tabular numerals; one y axis.
- **Readout:** hover, or keyboard (one Tab stop per chart, arrow keys / Home / End), in the top layer so cards don't clip it.
- **Accessible:** `role="img"` with `label`, plus an `sr-only` data table. Charts redraw at their measured width, so they work at 390 px.

| Piece | For / watch out |
|---|---|
| `tm-stat` | A headline number, no box; pages grid them. |
| `tm-time-chart` | One series over time as a line with a soft area. |
| `tm-bar-chart` | Stacked bars. `faded` for incomplete data (say why in `note` and under the chart); `fill` to grow to a stretched card's height. |
| `tm-calendar-chart` | GitHub-style day grid, accent in four steps; scrolls inside its host on phones. |
| `tm-bar-list` | Ranked rows with a bar; `max` shares one scale between lists side by side. |

## Container pieces

Domain composites in [dashboard/](../web/src/app/dashboard/) that take a `ContainerSummary` (`container`, required); version, update and tile also take `checking` (a skeleton until there's a result). The list, the tiles and the detail header are built from them.

**An `update` has three states.** Use the helpers in `containers.ts` (`hasResult`, `refreshFailed`, `hasUpdate`, `checkFailed`) instead of testing `error`, and `errorKindLabel` / `errorKindHint` in `format.ts` for reasons.

| State | Fields | Shown as |
|---|---|---|
| Result | `checkedAt` | Versions, class and level |
| Result, refresh failed | `checkedAt` + `error` + `failedAt` (+ `errorKind`) | The same plus `tm-refresh-failed`; still counts in filters and counts |
| No result | `error` + `failedAt`, no `checkedAt` | Just the tag and a dashed "check failed" badge |

| Piece | For / watch out |
|---|---|
| `tm-container-version` | Running → latest, plus how the version was found, the held-back line, or a channel build's state. |
| `tm-container-update` | Class + level (reasons on hover, capped; the detail page shows them all), or "Ignored", or "check failed" with the short reason. |
| `tm-refresh-failed` | Quiet note that the result is from an earlier check. No badge, no colour: it must not hide the result. |
| `tm-rule-problems` | Dashed "N rules skipped" badge. No colour. |
| `tm-container-tile` | One container as a tile; the whole tile is a stretched link (`link` overrides it). |
| `tm-container-table` | One group as `tm-card flush` + `table.tm-table tm-table-stack`. `ignored`, `showStack` (severity grouping, Snoozed), `dimQuiet`. |
| `tm-column-bar` | The single sticky column header above all tables. |
| `tm-dashboard-skeleton` | Loading state for `list` or `tiles`. **Keep it in step** with the table and tile spacing. |

**Reviewed / snoozed** (`review` set): class and level badges fade with no colour of their own; reviewed adds "Reviewed", snoozed adds "until …" and tones the row down. `[showReview]="false"` where the review is stated already (the detail header).

**Quiet when up to date:**

- Up to date (a result, class `none`, not ignored): `data-current`, muted name, greyed icon, a flat tile. Containers with an update get a bold name.
- `dimQuiet` (every dashboard section): quiet rows also fade (`data-dimmed`) until hover or focus (`DIM_CLASS` / `DIM_ICON` / `DIM_NAME` in `container-table.ts`). Reviewed ones aren't dimmed.
- Settled stacks go into one collapsed, muted "Up to date" section, each as a `level` 3 sub-section. Snoozed containers go to a collapsed "Snoozed" section above "Ignored".

### Tables across sections

The list view is one `tm-container-table` per stack or severity group, made to read as one table:

- **Shared widths:** every table is `table-fixed` with a `<colgroup>` from `COLUMNS` (`container-table.ts`).
- **One column bar:** `tm-column-bar` (`aria-hidden`) sits above all sections, `sticky top-16`, hidden below `sm`.
- **Headers stay for screen readers:** each table keeps its `<thead>` (`th scope="col"`), `sm:sr-only`.
- **Rows are links:** `tr.tm-row-link`, stretched link on the name.

## Detail and stats pieces

| Piece | For / watch out |
|---|---|
| `tm-review-actions` | Mark reviewed / Snooze, or the status with Undo. Only for a container with an update. Without a `reviewKey`, the version-based actions are disabled with the reason as a tooltip. |
| `tm-ai-summary` | The summary card in every state: empty, generating, summary, stale, error. Wears `.tm-ai-sheen` (teal and neutral only, so it never reads as the severity frame). **All model text is plain-text interpolation, never HTML.** While the server says `generating` it emits `changed` every 4 s (paused while the tab is hidden). |
| `tm-history` | Quiet timeline of version and image changes. |
| `tm-detail-skeleton`, `tm-stats-skeleton` | Page loading states with the same boxes as the loaded page. `role="status"`. |
