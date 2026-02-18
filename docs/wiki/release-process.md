# Release Process

## Overview

AI Dev Brain uses `release-please` for automated GitHub releases based on conventional commit messages. The tool analyzes commit history, generates changelogs, and creates GitHub releases. GoReleaser handles the binary build and distribution.

## Key Decisions

- **release-please for automated GitHub releases**: Conventional commits drive version bumps and changelog generation automatically (K-00010)
- **Conventional Commits format**: All commits follow the format `type(scope): description` with optional task ID reference. Types: `feat`, `fix`, `chore`, `refactor`, `docs`, `test`

## Process

1. Merge PRs to `main` using conventional commit messages
2. `release-please` automatically creates or updates a release PR with version bump and changelog
3. Merge the release PR to trigger the release
4. GoReleaser builds binaries for all platforms and attaches them to the GitHub release

## Configuration Files

- `release-please-config.json` -- Release-please configuration
- `.goreleaser.yml` -- GoReleaser build and distribution configuration (if present)

## Related

- See the `release-manager` Claude Code agent for automated release workflows
- See the `/release` skill for preparing releases via Claude Code

---
*Sources: TASK-00021*
