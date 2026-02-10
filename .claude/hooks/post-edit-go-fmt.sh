#!/bin/bash
# Auto-format Go files after editing
# This hook runs after Edit/Write tool on .go files
INPUT=$(cat)
FILE_PATH=$(echo "$INPUT" | jq -r '.tool_input.file_path // empty')
if [[ "$FILE_PATH" == *.go ]]; then
    gofmt -s -w "$FILE_PATH" 2>/dev/null || true
fi
exit 0
