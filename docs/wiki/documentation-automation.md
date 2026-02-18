# Documentation Automation

## Overview

The `/update-docs` skill is a Claude Code skill (defined in `.claude/skills/update-docs/SKILL.md`) that automates documentation updates for adb-managed tasks. The skill was refined in TASK-00033 to address 12 specific gaps identified through comparison with a production CCAAS implementation and analysis of Claude Code's execution characteristics.

## Key Decisions

- **`self` audience type**: Added for solo developer progress tracking -- most tasks in this project are solo, and generating stakeholder/team/customer communications for every update was noise (K-00014)
- **Conditional design.md updates**: Updates to `design.md` only trigger when actual design changes occur; previously the skill always attempted updates even for code-only changes (K-00015)
- **notes.md as update target**: Added `notes.md` as an output target to close the loop between work done and requirements tracked (K-00016)
- **Scope limits**: 3-8 bullets per entry, word count targets per audience -- without these guardrails, Claude Code tends toward exhaustive output that nobody reads (K-00017)
- **Communication template**: Changed from generic "Key Updates / Impact" to concrete "What Was Delivered / Key Numbers / Outstanding Items" to match actual communication files already produced (K-00018)

## Learnings

- When designing Claude Code skills that produce text output, always set explicit scope limits (bullet counts, word targets). Without constraints, AI output inflates to fill available context.
- The `self` audience type is a useful pattern for single-developer projects. It avoids generating unnecessary stakeholder communication artifacts while still capturing progress.
- Aligning skill templates with existing artifact formats (e.g., matching communication templates to what TASK-00021 already produced) prevents format drift across the project.

---
*Sources: TASK-00033*
