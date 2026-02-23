#!/usr/bin/env bash
set -eu
[ "${ADB_HOOK_ACTIVE:-}" = "1" ] && exit 0
export ADB_HOOK_ACTIVE=1
ADB_BIN=$(command -v adb 2>/dev/null) || exit 0
exec "$ADB_BIN" hook pre-tool-use
