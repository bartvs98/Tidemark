---
name: docs-sync
description: "Read-only check that Tidemark's docs match a change: the doc sections the diff touches, what they link to and /styleguide for UI. Reports what's wrong or missing; doesn't edit. Use before committing a roadmap step or a fix that changes behaviour, config or the API; pass the facts that changed as arguments."
context: fork
model: sonnet
---

# Docs-sync check

You check; you don't edit. The session that ran you fixes what you report.

Facts that changed, from the caller: $ARGUMENTS

## Gather

- The change: `git diff main...HEAD` plus `git diff HEAD` (uncommitted work). Read the code only as far as you need to confirm a fact.
- The doc sections the diff touches, and the sections those link to. Use the doc map in CLAUDE.md to tell where each kind of fact belongs. Don't read whole docs.

## Check

- **Every changed fact is in its doc:** behaviour and decision rules in FEATURES; packages, tables, endpoints, API shapes and CI in ARCHITECTURE; install, env vars and rules syntax in README; UI pieces and tokens in DESIGN-SYSTEM.
- **Each fact lives in one doc:** a fact repeated elsewhere should be a link instead. Flag repeats the change added or left stale.
- **Docs describe what's built:** a finished step is gone from ROADMAP; nothing planned is written as built (ARCHITECTURE may sketch tables and endpoints, marked "later").
- **Nothing contradicts the code:** names, defaults, field names, limits and examples in the touched sections still match.
- **Links and anchors resolve** in the touched sections.
- **UI:** each new or changed piece and state is on `/styleguide` (`web/src/app/styleguide/`) and in DESIGN-SYSTEM.

## Report

Only findings, each with `file:line`, what's wrong or missing and the fix in one line. If everything holds, say so in one line. Don't restate the change or the checks.
