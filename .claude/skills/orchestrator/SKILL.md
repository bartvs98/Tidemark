---
name: orchestrator
description: How to run a complex Tidemark task as the orchestrator: plan, ask the owner's product decisions up front, write a contract, brief subagents (models, reports) and integrate their parts. Use for work that spans several packages or parts or is big enough to parallelise; simple tasks run inline. Finishing (Done means, Branch → PR) is in CLAUDE.md.
---

# Orchestrating a task

## Plan

Read enough to split the work, ask the owner only the product decisions (batched, up front), briefly state the plan, then build.

## Orchestrate over subagents

- **Contracts first.** Write the Go signatures, JSON shapes and schema to one file in the scratchpad that every brief points to, so parts can run in parallel. Dependent parts (wiring, integration) come after.
- **Need-to-know briefs:** the agent's own files, the contract, the relevant CLAUDE.md gotchas, no commits, no verify runs (GitHub quota), and "if the contract must change, stop and put the question at the top of your report".
- **Short reports:** files changed outside the brief, decisions beyond it, anything in scope left undone or failing, open questions, the test result in one line. Not a restatement of the brief.
- **Model per agent** (set `model`): **Opus 5.5** for contract-heavy or tricky parts (Go pipeline, new state logic); **Sonnet 5.5** for parts that follow existing patterns (new `ui/` pieces, tests, styleguide entries) and for the docs-sync check.
- Answer agents' questions with SendMessage when they're within agreed decisions; take real product choices to the owner.
- **Small parts stay inline** when they're quicker to do than to brief (a few-line fix, one file, nothing to parallelise).
- **The main session integrates:** re-runs all tests, verifies visually (integration screenshots catch cross-part bugs that per-part tests miss), then finishes the task as CLAUDE.md describes (Done means, Branch → PR).
