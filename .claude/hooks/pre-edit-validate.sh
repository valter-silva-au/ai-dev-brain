#!/usr/bin/env bash
# Prevent editing generated or sensitive files
INPUT=$(cat)

# Extract file_path using grep/sed instead of jq for portability
FILE_PATH=$(echo "$INPUT" | grep -o '"file_path"[[:space:]]*:[[:space:]]*"[^"]*"' | head -1 | sed 's/.*"file_path"[[:space:]]*:[[:space:]]*"//;s/"$//')

# Block editing vendor/ files
if [[ "$FILE_PATH" == *vendor/* ]]; then
    echo "BLOCKED: Do not edit vendor/ files directly. Run 'go mod vendor' instead." >&2
    exit 2
fi
# Block editing go.sum directly
if [[ "$FILE_PATH" == *go.sum ]]; then
    echo "BLOCKED: Do not edit go.sum directly. Use 'go mod tidy'." >&2
    exit 2
fi
exit 0
