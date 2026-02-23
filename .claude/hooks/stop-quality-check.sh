#!/usr/bin/env bash
# Hook: Stop - Advisory checks for uncommitted changes and basic build/vet.
# Never blocks (always exits 0) to avoid infinite retry loops in agent teams.
# The stop_hook_active guard prevents re-entry when Claude retries after feedback.
set -u

# Read hook input from stdin
INPUT=$(cat)

# Guard: if this stop was triggered by a previous hook failure, skip to avoid loops.
# When a Stop hook exits 2, Claude retries and sets stop_hook_active=true.
STOP_HOOK_ACTIVE=$(echo "$INPUT" | grep -oP '"stop_hook_active"\s*:\s*\K(true|false)' 2>/dev/null || echo "false")
if [ "$STOP_HOOK_ACTIVE" = "true" ]; then
    exit 0
fi

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
if ! CGO_ENABLED=0 go build ./cmd/adb/ 2>&1; then
    echo "WARNING: Build failed (advisory only)"
fi

echo "Running vet..."
if ! CGO_ENABLED=0 go vet ./... 2>&1; then
    echo "WARNING: Vet failed (advisory only)"
fi

echo "All checks completed"
exit 0
