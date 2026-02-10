# =============================================================================
# AI Dev Brain (adb) - Makefile
# =============================================================================
# Developer workflow automation for building, testing, linting, and packaging.
#
# Usage:
#   make          Build the adb binary
#   make help     Show all available targets
# =============================================================================

# --- Variables ---------------------------------------------------------------

BINARY    := adb
MODULE    := github.com/drapaimern/ai-dev-brain
CMD       := ./cmd/adb/

VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT    ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE      ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

LDFLAGS   := -s -w \
             -X main.version=$(VERSION) \
             -X main.commit=$(COMMIT) \
             -X main.date=$(DATE)

# --- Default target ----------------------------------------------------------

.DEFAULT_GOAL := all

# --- Phony targets -----------------------------------------------------------

.PHONY: all build test test-coverage test-property lint vet fmt fmt-check \
        security clean install docker-build docker-run help

# --- Build -------------------------------------------------------------------

all: build ## Build the project (default)

build: ## Compile the adb binary
	go build -ldflags="$(LDFLAGS)" -o $(BINARY) $(CMD)

install: ## Install adb into $GOPATH/bin
	go install -ldflags="$(LDFLAGS)" $(CMD)

# --- Testing -----------------------------------------------------------------

test: ## Run all tests with race detection
	go test ./... -race -count=1

test-coverage: ## Run tests with coverage report (HTML output)
	go test ./... -coverprofile=coverage.out -covermode=atomic
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

test-property: ## Run property-based tests only
	go test ./... -run "TestProperty" -count=1 -v

# --- Code Quality ------------------------------------------------------------

lint: ## Run golangci-lint
	golangci-lint run

vet: ## Run go vet
	go vet ./...

fmt: ## Format all Go source files
	gofmt -s -w .

fmt-check: ## Check formatting (fails if files need formatting)
	@test -z "$$(gofmt -l .)" || { echo "Files need formatting:"; gofmt -l .; exit 1; }

security: ## Run govulncheck for known vulnerabilities
	govulncheck ./...

# --- Docker ------------------------------------------------------------------

docker-build: ## Build Docker image
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg BUILD_DATE=$(DATE) \
		-t $(BINARY):latest \
		-t $(BINARY):$(VERSION) \
		.

docker-run: ## Run adb in a Docker container
	docker run --rm $(BINARY):latest

# --- Cleanup -----------------------------------------------------------------

clean: ## Remove build artifacts
	rm -f $(BINARY) $(BINARY).exe coverage.out coverage.html

# --- Help --------------------------------------------------------------------

help: ## Show this help message
	@echo "AI Dev Brain (adb) - Available targets:"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'
	@echo ""
	@echo "Variables:"
	@echo "  VERSION=$(VERSION)"
	@echo "  COMMIT=$(COMMIT)"
