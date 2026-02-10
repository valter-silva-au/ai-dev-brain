#!/bin/bash
# Prevent editing generated or sensitive files
INPUT=$(cat)
FILE_PATH=$(echo "$INPUT" | jq -r '.tool_input.file_path // empty')
# Block editing vendor/ files
if [[ "$FILE_PATH" == vendor/* ]]; then
    echo "BLOCKED: Do not edit vendor/ files directly. Run 'go mod vendor' instead." >&2
    exit 2
fi
# Block editing go.sum directly
if [[ "$FILE_PATH" == "go.sum" ]]; then
    echo "BLOCKED: Do not edit go.sum directly. Use 'go mod tidy'." >&2
    exit 2
fi
exit 0
