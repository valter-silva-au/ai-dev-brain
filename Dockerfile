# =============================================================================
# AI Dev Brain (adb) - Multi-stage Docker Build
# =============================================================================
# Produces a minimal Alpine-based image with the adb CLI binary.
# Git is included in the final image because adb uses git worktree operations.
# =============================================================================

# ---------------------------------------------------------------------------
# Stage 1: Build the Go binary
# ---------------------------------------------------------------------------
FROM golang:1.26-alpine AS builder

# Version can be injected at build time: docker build --build-arg VERSION=1.0.0
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

# Git is needed during build for module fetching and test operations
RUN apk add --no-cache git

WORKDIR /src

# Copy dependency manifests first for better layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy the full source tree
COPY . .

# Build a statically-linked binary with version metadata baked in
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${BUILD_DATE}" \
    -o /usr/local/bin/adb \
    ./cmd/adb/

# ---------------------------------------------------------------------------
# Stage 2: Minimal runtime image
# ---------------------------------------------------------------------------
FROM alpine:3.23

ARG VERSION=dev

LABEL maintainer="valter-silva-au"
LABEL org.opencontainers.image.title="ai-dev-brain"
LABEL org.opencontainers.image.description="AI Dev Brain - intelligent developer workflow CLI"
LABEL org.opencontainers.image.version="${VERSION}"
LABEL org.opencontainers.image.source="https://github.com/valter-silva-au/ai-dev-brain"

# Git is required at runtime for worktree operations
RUN apk add --no-cache git ca-certificates

# Run as non-root for security
RUN adduser -D -h /home/adb adb
USER adb
WORKDIR /home/adb

# Copy the compiled binary from the builder stage
COPY --from=builder /usr/local/bin/adb /usr/local/bin/adb

ENTRYPOINT ["adb"]
