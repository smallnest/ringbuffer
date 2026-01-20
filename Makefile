.PHONY: help all build test test-race test-coverage treemap test-verbose lint fmt fmt-check vet clean deps tidy check benchmark docs install-tools version info ci pre-commit update-deps

# Variables
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=gofmt
GOVET=$(GOCMD) vet
COVERAGE_FILE=coverage.out
COVERAGE_HTML=coverage.html

# Colors for terminal output
COLOR_RESET=\033[0m
COLOR_BOLD=\033[1m
COLOR_GREEN=\033[32m
COLOR_YELLOW=\033[33m
COLOR_BLUE=\033[34m

# Default target
all: fmt-check vet test

## help: Display this help message
help:
	@printf "$(COLOR_BOLD)RingBuffer - Makefile Commands$(COLOR_RESET)\n"
	@printf "\n"
	@printf "$(COLOR_BOLD)Usage:$(COLOR_RESET)\n"
	@printf "  make $(COLOR_GREEN)<target>$(COLOR_RESET)\n"
	@printf "\n"
	@printf "$(COLOR_BOLD)Available targets:$(COLOR_RESET)\n"
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## /  /' | sed 's/://'
	@printf "\n"

## build: Build project
build:
	@echo "$(COLOR_BLUE)Building...$(COLOR_RESET)"
	$(GOBUILD) -v ./...

## test: Run all tests
test:
	@echo "$(COLOR_BLUE)Running tests...$(COLOR_RESET)"
	$(GOTEST) -v ./...

## test-short: Run tests in short mode
test-short:
	@echo "$(COLOR_BLUE)Running tests (short mode)...$(COLOR_RESET)"
	$(GOTEST) -short ./...

## test-race: Run tests with race detector
test-race:
	@echo "$(COLOR_BLUE)Running tests with race detector...$(COLOR_RESET)"
	$(GOTEST) -race ./...

## test-coverage: Run tests with coverage report
test-coverage:
	@echo "$(COLOR_BLUE)Running tests with coverage...$(COLOR_RESET)"
	$(GOTEST) -coverprofile=$(COVERAGE_FILE) -covermode=atomic ./...
	@echo "$(COLOR_GREEN)Coverage report generated: $(COVERAGE_FILE)$(COLOR_RESET)"
	$(GOCMD) tool cover -html=$(COVERAGE_FILE) -o $(COVERAGE_HTML)
	@echo "$(COLOR_GREEN)HTML coverage report: $(COVERAGE_HTML)$(COLOR_RESET)"

## treemap: Generate SVG coverage treemap
treemap: test-coverage
	@which go-cover-treemap > /dev/null || (echo "$(COLOR_YELLOW)go-cover-treemap not found. Run 'go install github.com/nikandfor/go-cover-treemap@latest'$(COLOR_RESET)" && exit 1)
	go-cover-treemap -coverprofile $(COVERAGE_FILE) > coverage.svg
	@echo "$(COLOR_GREEN)SVG coverage report: coverage.svg$(COLOR_RESET)"

## test-verbose: Run tests with verbose output
test-verbose:
	@echo "$(COLOR_BLUE)Running tests (verbose)...$(COLOR_RESET)"
	$(GOTEST) -v -count=1 ./...

## benchmark: Run benchmarks
benchmark:
	@echo "$(COLOR_BLUE)Running benchmarks...$(COLOR_RESET)"
	$(GOTEST) -bench=. -benchmem ./...

## lint: Run golangci-lint
lint:
	@echo "$(COLOR_BLUE)Running linter...$(COLOR_RESET)"
	@which golangci-lint > /dev/null || (echo "$(COLOR_YELLOW)golangci-lint not found. Run 'make install-tools'$(COLOR_RESET)" && exit 1)
	golangci-lint run ./...

## fmt: Format all Go files
fmt:
	@echo "$(COLOR_BLUE)Formatting code...$(COLOR_RESET)"
	$(GOFMT) -s -w .
	@echo "$(COLOR_GREEN)Code formatted successfully$(COLOR_RESET)"

## fmt-check: Check if code is formatted
fmt-check:
	@echo "$(COLOR_BLUE)Checking code formatting...$(COLOR_RESET)"
	@test -z "$$($(GOFMT) -l .)" || (echo "$(COLOR_YELLOW)The following files need formatting:$(COLOR_RESET)" && $(GOFMT) -l . && exit 1)
	@echo "$(COLOR_GREEN)All files are properly formatted$(COLOR_RESET)"

## vet: Run go vet
vet:
	@echo "$(COLOR_BLUE)Running go vet...$(COLOR_RESET)"
	$(GOVET) ./...

## check: Run fmt-check, vet, and lint
check: fmt-check vet lint
	@echo "$(COLOR_GREEN)All checks passed!$(COLOR_RESET)"

## clean: Clean build artifacts and test cache
clean:
	@echo "$(COLOR_BLUE)Cleaning...$(COLOR_RESET)"
	$(GOCLEAN)
	rm -f $(COVERAGE_FILE) $(COVERAGE_HTML) coverage.svg
	rm -rf ./bin
	@echo "$(COLOR_GREEN)Clean complete$(COLOR_RESET)"

## deps: Download dependencies
deps:
	@echo "$(COLOR_BLUE)Downloading dependencies...$(COLOR_RESET)"
	$(GOMOD) download
	@echo "$(COLOR_GREEN)Dependencies downloaded$(COLOR_RESET)"

## tidy: Tidy and verify dependencies
tidy:
	@echo "$(COLOR_BLUE)Tidying dependencies...$(COLOR_RESET)"
	$(GOMOD) tidy
	$(GOMOD) verify
	@echo "$(COLOR_GREEN)Dependencies tidied$(COLOR_RESET)"

## install-tools: Install development tools
install-tools:
	@echo "$(COLOR_BLUE)Installing development tools...$(COLOR_RESET)"
	@which golangci-lint > /dev/null || (echo "Installing golangci-lint..." && \
		go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)
	@which go-cover-treemap > /dev/null || (echo "Installing go-cover-treemap..." && \
		go install github.com/nikandfor/go-cover-treemap@latest)
	@echo "$(COLOR_GREEN)Tools installed$(COLOR_RESET)"

## docs: Generate documentation
docs:
	@echo "$(COLOR_BLUE)Generating documentation...$(COLOR_RESET)"
	@echo "Open http://localhost:6060/pkg/github.com/smallnest/ringbuffer/ in your browser"
	godoc -http=:6060

## example: Run example
example:
	@echo "$(COLOR_BLUE)Running example...$(COLOR_RESET)"
	$(GOCMD) run example_test.go

## ci: Run continuous integration checks
ci: deps check test-race test-coverage
	@echo "$(COLOR_GREEN)CI checks passed!$(COLOR_RESET)"

## pre-commit: Run pre-commit checks (fmt, vet, lint, test)
pre-commit: fmt vet lint test
	@echo "$(COLOR_GREEN)Pre-commit checks passed!$(COLOR_RESET)"

## update-deps: Update all dependencies to latest versions
update-deps:
	@echo "$(COLOR_BLUE)Updating dependencies...$(COLOR_RESET)"
	$(GOGET) -u ./...
	$(GOMOD) tidy
	@echo "$(COLOR_GREEN)Dependencies updated$(COLOR_RESET)"

## version: Display Go version
version:
	@$(GOCMD) version

## info: Display project information
info:
	@printf "$(COLOR_BOLD)Project Information$(COLOR_RESET)\n"
	@printf "  Name: RingBuffer\n"
	@printf "  Module: github.com/smallnest/ringbuffer\n"
	@printf "  Go Version: $$($(GOCMD) version | cut -d' ' -f3)\n"
	@printf "  Files: $$($(GOCMD) list ./... | wc -l | tr -d ' ')\n"
