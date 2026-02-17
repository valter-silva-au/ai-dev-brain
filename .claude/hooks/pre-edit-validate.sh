#!/bin/bash
# Prevent editing generated or sensitive files
# Uses grep instead of jq for Windows compatibility
INPUT=$(cat)
FILE_PATH=$(echo "$INPUT" | grep -oP '"file_path"\s*:\s*"[^"]*"' | head -1 | grep -oP ':\s*"\K[^"]+' || echo "")
if [ -z "$FILE_PATH" ]; then
    exit 0
fi
case "$FILE_PATH" in
    vendor/*|*/vendor/*)
        echo "BLOCKED: Do not edit vendor/ files directly. Run 'go mod vendor' instead." >&2
        exit 2
        ;;
    go.sum|*/go.sum)
        echo "BLOCKED: Do not edit go.sum directly. Use 'go mod tidy'." >&2
        exit 2
        ;;
esac
exit 0
