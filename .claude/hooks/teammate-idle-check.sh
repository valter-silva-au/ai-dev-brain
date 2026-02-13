#!/usr/bin/env bash
# Hook: TeammateIdle - Advisory check, never blocks idle (exit 0 always)
# Teammates going idle is normal after each turn. Blocking idle with exit 2
# prevents agents from shutting down and causes them to hang indefinitely.
set -uo pipefail

cd "$(git rev-parse --show-toplevel)" 2>/dev/null || exit 0

echo "Running vet (advisory)..."
if ! go vet ./... 2>&1; then
    echo "WARNING: vet issues detected (advisory only)"
fi

exit 0
