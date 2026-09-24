# Build stage
FROM golang:1.22-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary
ARG VERSION=dev
ARG COMMIT=unknown
ARG DATE=unknown

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-X main.Version=${VERSION} -X main.Commit=${COMMIT} -X main.Date=${DATE} -s -w" \
    -o adb \
    ./cmd/adb

# Runtime stage
FROM alpine:3.21

# Install runtime dependencies
RUN apk add --no-cache git ca-certificates

# Create non-root user
RUN addgroup -g 1000 adb && \
    adduser -D -u 1000 -G adb adb

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/adb /usr/local/bin/adb

# Change ownership
RUN chown -R adb:adb /app

# Switch to non-root user
USER adb

# Set entrypoint
ENTRYPOINT ["/usr/local/bin/adb"]
CMD ["--help"]
