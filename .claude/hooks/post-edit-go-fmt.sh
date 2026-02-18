#!/usr/bin/env bash
# Auto-format Go files after editing
# Uses grep instead of jq for Windows compatibility
INPUT=$(cat)
FILE_PATH=$(echo "$INPUT" | grep -oP '"file_path"\s*:\s*"[^"]*"' | head -1 | grep -oP ':\s*"\K[^"]+' || echo "")
if [[ "$FILE_PATH" == *.go ]]; then
    gofmt -s -w "$FILE_PATH" 2>/dev/null || true
fi
exit 0
