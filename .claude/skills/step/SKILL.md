---
name: step
description: How the owner and Claude build Tidemark: splitting a roadmap step over subagents (contracts, briefs, models), what every step and fix must end with (tests, docs, docs-sync check, README images) and the branch → PR flow. Use when starting a roadmap step or an audit fix, before delegating to subagents, and before committing or opening a PR.
---

# Building a step

## Plan

One roadmap step at a time. Read enough to split it, ask the owner only the product decisions (batched, up front), briefly state the plan, then build.

**Audit fixes:** before fixing a finding from `docs/AUDIT.md`, check it against the code. When it's wrong, or its suggested fix is, report back first. One `fix-…` PR per step of its suggested order.

## Orchestrate over subagents

- **Contracts first.** Write the Go signatures, JSON shapes and schema to one file in the scratchpad that every brief points to, so parts can run in parallel. Dependent parts (wiring, integration) come after.
- **Need-to-know briefs:** the agent's own files, the contract, the relevant CLAUDE.md gotchas, no commits, no verify runs (GitHub quota), and "if the contract must change, stop and put the question at the top of your report".
- **Short reports:** files changed outside the brief, decisions beyond it, anything in scope left undone or failing, open questions, the test result in one line. Not a restatement of the brief.
- **Model per agent** (set `model`): **Opus 5.5** for contract-heavy or tricky parts (Go pipeline, new state logic); **Sonnet 5.5** for parts that follow existing patterns (new `ui/` pieces, tests, styleguide entries) and for the docs-sync check.
- Answer agents' questions with SendMessage when they're within agreed decisions; take real product choices to the owner.
- **Small tasks stay inline** when they're quicker to do than to brief (a few-line fix, one file, nothing to parallelise).
- **The main session integrates:** re-runs all tests, verifies visually (integration screenshots catch cross-part bugs that per-part tests miss), updates the docs, commits.

## Done means

- All tests green; `go vet` and `gofmt -l .` clean.
- **Docs updated:** remove the finished step from ROADMAP; update FEATURES / ARCHITECTURE / README / DESIGN-SYSTEM as relevant. Docs describe what's built; each fact lives in one doc (link, don't repeat). Plans live in the ROADMAP step (ARCHITECTURE may sketch tables and endpoints, marked "later").
- **Docs-sync check** (roadmap steps, and fixes that change behaviour, config or the API): a read-only Sonnet agent gets the diff, the doc sections touched and the facts changed, checks just those sections plus what they link to (and `/styleguide` for UI) and lists what's wrong or missing. Fix it in the same commit. For small fixes and docs-only PRs, check inline.
- **README images** when the dashboard looks noticeably different (header, hero, list/tile layout, colours): see the `verify` skill.

## Branch → PR

- Branch off an up-to-date `main`: `step-14-…`, `fix-…`, `docs-…`. Never commit to `main`.
- Commit messages end with `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.
- Push, then `gh pr create`: title like the commit subject; body only what changed, ending with the PR attribution line. How it was verified and what the owner should look at go in the chat summary.
- CI tests and builds the image on the PR without publishing it (`gh pr checks`; docs-only PRs have no checks).
- **The owner reviews and squash-merges; don't merge yourself.** Afterwards: `git switch main && git pull`, delete the local branch.
