#!/usr/bin/env bash
# Auto-format Go files after editing
# This hook runs after Edit/Write tool on .go files
INPUT=$(cat)
FILE_PATH=$(echo "$INPUT" | grep -o '"file_path"[[:space:]]*:[[:space:]]*"[^"]*"' | head -1 | sed 's/.*"file_path"[[:space:]]*:[[:space:]]*"//;s/"$//')
if [[ "$FILE_PATH" == *.go ]]; then
    gofmt -s -w "$FILE_PATH" 2>/dev/null || true
fi
exit 0
