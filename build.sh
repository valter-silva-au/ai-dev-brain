#!/usr/bin/env bash
set -euo pipefail

BINARY="adb"
CMD="./cmd/adb/"

VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo "dev")}"
COMMIT="${COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")}"
DATE="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"

LDFLAGS="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}"

case "${1:-build}" in
  build)
    echo "Building ${BINARY} ${VERSION} (${COMMIT})..."
    go build -ldflags="${LDFLAGS}" -o "${BINARY}" "${CMD}"
    echo "Done: ./${BINARY}"
    ;;
  install)
    mkdir -p "${HOME}/.local/bin"
    go build -ldflags="${LDFLAGS}" -o "${HOME}/.local/bin/${BINARY}" "${CMD}"
    echo "Installed ${HOME}/.local/bin/${BINARY} (${VERSION})"
    ;;
  clean)
    rm -f "${BINARY}" "${BINARY}.exe" coverage.out coverage.html
    echo "Cleaned."
    ;;
  *)
    echo "Usage: $0 {build|install|clean}"
    exit 1
    ;;
esac
