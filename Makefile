.PHONY: build test lint vet fmt security docker-build install-local clean help

# Version information
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# Build variables
BINARY_NAME = adb
MAIN_PACKAGE = ./cmd/adb
LDFLAGS = -ldflags "-X main.Version=$(VERSION) -X main.Commit=$(COMMIT) -X main.Date=$(DATE) -s -w"

# Installation directory
INSTALL_DIR = $(HOME)/.local/bin

help: ## Display this help screen
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

build: ## Build the binary with version information
	@echo "Building $(BINARY_NAME) $(VERSION)..."
	go build $(LDFLAGS) -o $(BINARY_NAME) $(MAIN_PACKAGE)
	@echo "Build complete: $(BINARY_NAME)"

test: ## Run tests with race detection
	@echo "Running tests with race detection..."
	go test -race -v ./... -count=1

lint: ## Run golangci-lint
	@echo "Running golangci-lint..."
	@which golangci-lint > /dev/null || (echo "golangci-lint not installed. Install from https://golangci-lint.run/usage/install/" && exit 1)
	golangci-lint run ./...

vet: ## Run go vet
	@echo "Running go vet..."
	go vet ./...

fmt: ## Format code with gofmt
	@echo "Formatting code..."
	gofmt -s -w .
	@echo "Code formatted"

security: ## Run govulncheck for security vulnerabilities
	@echo "Running security checks..."
	@which govulncheck > /dev/null || (echo "govulncheck not installed. Install with: go install golang.org/x/vuln/cmd/govulncheck@latest" && exit 1)
	govulncheck ./...

docker-build: ## Build Docker image
	@echo "Building Docker image..."
	docker build -t $(BINARY_NAME):$(VERSION) -t $(BINARY_NAME):latest .
	@echo "Docker image built: $(BINARY_NAME):$(VERSION)"

install-local: build ## Install binary to ~/.local/bin
	@echo "Installing $(BINARY_NAME) to $(INSTALL_DIR)..."
	@mkdir -p $(INSTALL_DIR)
	@cp $(BINARY_NAME) $(INSTALL_DIR)/$(BINARY_NAME)
	@# macOS (Apple Silicon): cp invalidates the Go linker's ad-hoc code
	@# signature, so the copied binary is SIGKILL'd on exec (exit 137, silent).
	@# Re-sign the installed copy in place to restore a valid ad-hoc signature.
	@if [ "$$(uname -s)" = "Darwin" ] && command -v codesign >/dev/null 2>&1; then \
		codesign --force --sign - $(INSTALL_DIR)/$(BINARY_NAME) && \
		echo "Re-signed (ad-hoc) for macOS"; \
	fi
	@echo "Installed to $(INSTALL_DIR)/$(BINARY_NAME)"
	@echo "Make sure $(INSTALL_DIR) is in your PATH"

clean: ## Clean build artifacts
	@echo "Cleaning..."
	@rm -f $(BINARY_NAME)
	@go clean
	@echo "Clean complete"

all: fmt vet lint test build ## Run fmt, vet, lint, test, and build
