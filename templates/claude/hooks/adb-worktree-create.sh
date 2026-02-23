#!/usr/bin/env bash
set -eu
ADB_BIN=$(command -v adb 2>/dev/null) || exit 0
"$ADB_BIN" worktree-hook create 2>/dev/null || true
exit 0
