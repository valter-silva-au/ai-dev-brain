#!/usr/bin/env bash
# Hook: Stop - Checks for uncommitted changes and basic build/vet
set -euo pipefail

# Read hook input from stdin
INPUT=$(cat)

# Prevent infinite loop: if this hook already triggered a continuation, allow stop
if [ "$(echo "$INPUT" | jq -r '.stop_hook_active')" = "true" ]; then
    exit 0
fi

cd "$(git rev-parse --show-toplevel)"

echo "Checking for uncommitted changes..."
if [ -n "$(git status --porcelain)" ]; then
    echo "WARNING: Uncommitted changes detected:"
    git status --short
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
