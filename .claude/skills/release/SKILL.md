---
name: release
description: Prepare and create a new release with GoReleaser
argument-hint: "<version>"
allowed-tools: Bash, Read, Grep
---

# Release Skill

Prepare and create a new release. Requires a version argument (e.g., "v1.0.0").

## Prerequisites

- The version argument is **required**. If not provided, ask the user for a version number.
- The version should follow semantic versioning (e.g., v1.0.0, v1.2.3-beta.1).

## Steps

1. **Verify clean working tree**
   Run `git status --porcelain`. If there are uncommitted changes, warn the user and stop.

2. **Run full test suite**
   Run `go test ./... -race -count=1`. If any tests fail, report failures and stop.

3. **Validate GoReleaser config**
   Run `goreleaser check` to verify `.goreleaser.yml` is valid.

4. **Show changelog since last tag**
   ```
   git log --oneline $(git describe --tags --abbrev=0 2>/dev/null || echo HEAD~10)..HEAD
   ```
   Display the list of commits that will be included in this release.

5. **Ask for confirmation**
   Present the changelog and ask the user to confirm before proceeding with tagging.

6. **Create annotated tag**
   ```
   git tag -a <version> -m "Release <version>"
   ```

7. **Instruct user to push**
   Tell the user to push the tag to trigger the release workflow:
   ```
   git push origin <version>
   ```
   Note: The GitHub Actions workflow at `.github/workflows/release.yml` will handle the actual release build and publication via GoReleaser.

## Important

- Do NOT push the tag automatically. Let the user decide when to push.
- Do NOT run `goreleaser release` locally. The CI pipeline handles this.
