# Design system

The visual rules are in [BRAND.md](BRAND.md). This page covers how they're built in code. To see every piece live in both themes, run the app and open **`/styleguide`**.

## Where things live

| What | File |
|---|---|
| Tokens (colour, type, radius, elevation) | [web/src/styles/tokens.css](../web/src/styles/tokens.css) |
| Table, tabs and shimmer styles | [web/src/styles/components.css](../web/src/styles/components.css) |
| Angular components | [web/src/app/ui/](../web/src/app/ui/) (import everything from `../ui`) |
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
| `tm-badge` | Inputs: `tone` (neutral, accent, ok, attention, breaking, unknown), `icon`, `mono`. A generic label. |
| `tm-update-class` | `kind`: none, patch, minor, major or digest. "Up to date" or a **neutral** badge. |
| `tm-level` | `level`: none, attention, breaking or unknown. Renders **nothing** for `none`. |
| `tm-version-diff` | `from` and `to`, shown as `1.4.2 → 1.6.0` in mono. |
| `tm-card` | `heading`, `flush`, and a `[tmCardActions]` slot for header buttons. |
| `tm-empty-state` | `heading`, `description`, `icon`. Buttons go in as content. |
| `tm-skeleton` | `width` and `height`. The shimmer stops when the OS asks for reduced motion. |
| `Toasts` + `<tm-toaster>` | `inject(Toasts).show(msg, tone, ms)`. Breaking toasts use `role="alert"` and the others `role="status"`. |
| `tm-icon` | `name`, `size`. A small set of Lucide icons; add new ones in `icon.ts`. |
| `tm-theme-switch` | System, light or dark. Lives in the header. |
| `table.tm-table` | A plain table with `tr.tm-group` rows for stack headings. Plain HTML means it's accessible by default. |
| `TM_TABS` + `.tm-tabs` | [Angular Aria](https://angular.dev/guide/aria/tabs) headless tabs (`ngTabs`, `ngTabList`, `ngTab`, `ngTabPanel`), styled by the class. Keyboard navigation and ARIA come from Aria. |

## Rules

1. **Colour means risk.** Patch, minor and major are neutral. Only `attention` and `breaking` get colour. Don't add coloured badges for anything else.
2. **Never colour alone.** Every status has an icon **and** a word.
3. **Versions, tags, digests and image refs are always mono.**
4. **Build from the pieces.** If a screen needs something new, add it to `ui/` and to `/styleguide` first, then use it.
5. **Accessibility:** use native elements first (`button`, `a`, `table`), use Angular Aria for composite widgets (tabs, menus, listboxes), give every icon-only button an `aria-label`, and never remove the visible focus ring (`:focus-visible`, 2 px `focus` colour).
