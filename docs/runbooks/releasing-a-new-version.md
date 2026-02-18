# Runbook: Releasing a New Version

## Prerequisites

- You have push access to the `main` branch
- All CI checks pass on the branch being merged
- `go`, `golangci-lint`, and `govulncheck` are installed locally
- You are on a branch with changes ready to merge

## Steps

### 1. Verify the build is clean

```bash
go build -ldflags="-s -w" -o adb ./cmd/adb/
go vet ./...
golangci-lint run
go test ./... -race -count=1
govulncheck ./...
```

All commands must exit with code 0.

### 2. Ensure commits follow conventional format

All commits on the branch must use the Conventional Commits format:

```
feat(core): add knowledge query command
fix(cli): correct argument parsing in exec
chore(deps): update Go to 1.24
```

Include the task ID in the commit body or footer where applicable.

### 3. Create and merge the PR to main

```bash
gh pr create --title "feat: description of changes" --body "TASK-XXXXX: summary"
```

Wait for CI to pass, then merge.

### 4. Wait for release-please to create a release PR

After merging to `main`, the `release-please` GitHub Action will:
- Analyze new commits since the last release
- Determine the version bump (major/minor/patch) from commit types
- Create or update a release PR with the changelog

### 5. Review and merge the release PR

- Verify the version number is correct
- Verify the changelog entries are accurate
- Merge the release PR

### 6. Verify the release

After merging the release PR:
- Check the GitHub Releases page for the new release
- Verify release artifacts (binaries) are attached
- Verify the changelog is correct

## Verification

- [ ] GitHub Release exists with the expected version tag
- [ ] Release binaries are downloadable and functional: `./adb version`
- [ ] Changelog accurately reflects merged changes

## Rollback

If a release contains a critical bug:

1. Create a `fix` commit on a new branch
2. Merge to `main` following the normal PR process
3. `release-please` will create a new patch release automatically

Do not delete GitHub releases or tags -- create a new release instead.

## Troubleshooting

**release-please does not create a PR**: Check that commits follow conventional format. Non-conventional commits are ignored. Verify the `release-please-config.json` configuration.

**CI fails on the release PR**: The release PR only changes `CHANGELOG.md` and version files. If CI fails, check whether a test depends on hardcoded version strings.

**Tag mismatch**: If the release tag does not match the expected version, check `release-please-config.json` for the correct release type and versioning strategy.
