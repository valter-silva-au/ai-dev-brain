#!/usr/bin/env bash
# Hook: TaskCompleted - Verifies tests pass, lint clean, no uncommitted Go changes
set -eu

REPO_ROOT=$(git rev-parse --show-toplevel 2>/dev/null) || exit 0
cd "$REPO_ROOT" 2>/dev/null || exit 0
git status --porcelain >/dev/null 2>&1 || exit 0

echo "Checking for uncommitted Go file changes..."
if git diff --name-only 2>/dev/null | grep -q '\.go$'; then
    echo "FAIL: Uncommitted Go file changes detected"
    git diff --name-only 2>/dev/null | grep '\.go$'
    exit 2
fi

if git diff --cached --name-only 2>/dev/null | grep -q '\.go$'; then
    echo "FAIL: Staged but uncommitted Go file changes detected"
    git diff --cached --name-only 2>/dev/null | grep '\.go$'
    exit 2
fi

echo "Running tests..."
if ! go test ./... -count=1 2>&1; then
    echo "FAIL: Tests failed"
    exit 2
fi

echo "Running lint..."
if ! golangci-lint run 2>&1; then
    echo "FAIL: Lint check failed"
    exit 2
fi

echo "All checks passed"
exit 0
