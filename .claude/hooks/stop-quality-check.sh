#!/usr/bin/env bash
# Hook: Stop - Checks for uncommitted changes and basic build/vet
set -eu

# Read hook input from stdin
INPUT=$(cat)

# Find the git repo root; if this fails (e.g. broken worktree path), skip all checks
REPO_ROOT=$(git rev-parse --show-toplevel 2>/dev/null) || exit 0
cd "$REPO_ROOT" 2>/dev/null || exit 0

# Verify we're actually in a working git repo
git status --porcelain >/dev/null 2>&1 || exit 0

echo "Checking for uncommitted changes..."
CHANGES=$(git status --porcelain 2>/dev/null || echo "")
if [ -n "$CHANGES" ]; then
    echo "WARNING: Uncommitted changes detected:"
    git status --short 2>/dev/null || true
fi

echo "Running build..."
if ! go build ./cmd/adb/ 2>&1; then
    echo "FAIL: Build failed"
    exit 2
fi

echo "Running vet..."
if ! go vet ./... 2>&1; then
    echo "FAIL: Vet failed"
    exit 2
fi

echo "All checks passed"
exit 0
