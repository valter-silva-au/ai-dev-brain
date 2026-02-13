#!/usr/bin/env bash
# Hook: TeammateIdle - Runs tests and lint to verify project health
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

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
