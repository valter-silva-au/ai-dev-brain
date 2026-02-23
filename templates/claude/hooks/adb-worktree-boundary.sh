#!/bin/bash
# PreToolUse hook: validates tool paths are within $ADB_WORKTREE_PATH.
# Blocks Edit/Write/Read operations on files outside the worktree boundary.
# Called by Claude Code before executing tool operations.
#
# Environment: ADB_WORKTREE_PATH must be set for enforcement.
# If ADB_WORKTREE_PATH is not set, the hook exits 0 (no enforcement).

# Read tool use event from stdin (JSON).
INPUT=$(cat)

# If no worktree path is set, skip enforcement.
if [ -z "$ADB_WORKTREE_PATH" ]; then
  exit 0
fi

# Extract the file_path from the tool input (handles common patterns).
FILE_PATH=$(echo "$INPUT" | grep -o '"file_path"[[:space:]]*:[[:space:]]*"[^"]*"' | head -1 | sed 's/.*"file_path"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')

# If no file_path found, allow (might be a non-file tool).
if [ -z "$FILE_PATH" ]; then
  exit 0
fi

# Resolve to absolute path for comparison.
RESOLVED=$(realpath -m "$FILE_PATH" 2>/dev/null || echo "$FILE_PATH")
WORKTREE=$(realpath -m "$ADB_WORKTREE_PATH" 2>/dev/null || echo "$ADB_WORKTREE_PATH")

# Check if the resolved path starts with the worktree path.
case "$RESOLVED" in
  "$WORKTREE"/*)
    exit 0
    ;;
  "$WORKTREE")
    exit 0
    ;;
  *)
    echo "BLOCKED: Path $FILE_PATH is outside worktree boundary $ADB_WORKTREE_PATH" >&2
    # Log to event log if adb is available.
    if command -v adb >/dev/null 2>&1; then
      adb worktree-hook violation --path "$FILE_PATH" 2>/dev/null || true
    fi
    exit 2
    ;;
esac
