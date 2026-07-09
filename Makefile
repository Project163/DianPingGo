MODULE_NAME := $(shell go list -m)
SERVICE_NAME := server
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "v0.0.0")
BUILD_TIME := $(shell date +%Y-%m-%dT%H:%M:%S%z)
COMMIT_SHA := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

BUILD_DIR := ./build
CMD_DIR := ./cmd/$(SERVICE_NAME)
MAIN_FILE := $(CMD_DIR)/main.go

GO := go
GOBUILD := CGO_ENABLED=0 $(GO) build
GOCLEAN := $(GO) clean
GOTEST := $(GO) test
GOMOD := $(GO) mod
GOGET := $(GO) get

.PHONY: all default help dev build clean test cover lint fmt wire

# Default target
default: help

all: lint test build

## dev: Run the application in development mode with live reloading (requires air)
dev:
	if command -v air >/dev/null 2>&1; then \
		air; \
	else \
		echo "Please install air: go install github.com/air-verse/air@latest"; \
		echo "Run with 'go run'..."; \
		$(GO) run $(MAIN_FILE); \
	fi

## build: Build the application binary
build: clean
	@echo "Building $(SERVICE_NAME) version $(VERSION)..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) -o $(BUILD_DIR)/$(SERVICE_NAME) $(MAIN_FILE)
	@echo "Build completed: $(BUILD_DIR)/$(SERVICE_NAME)"

## clean: Remove build artifacts
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf $(BUILD_DIR)
	@$(GOCLEAN)

## lint: Run linters on the codebase
lint:
	@echo "Running linters..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "Please install golangci-lint: curl -sSfL https://golangci-lint.run/install.sh | sh -s -- -b $$(go env GOPATH)/bin v2.12.2"; \
		exit 1; \
	fi

## fmt: Format the codebase using go fmt
fmt:
	@echo "Formatting code..."
	@$(GO) fmt ./...

pkg ?= ./...
run ?= .

## test: Run tests for the specified package and test pattern
test:
	@echo "Running tests for $(pkg) (matching: $(run))..."
	@$(GOTEST) -v -race -cover -run="$(run)" $(pkg)

## cover: Run tests with coverage and generate an HTML report
cover:
	@echo "Running tests with coverage for $(pkg)..."
	@$(GOTEST) -v -race -coverprofile=coverage.out $(pkg)
	@go tool cover -html=coverage.out -o coverage.html

## wire: Generate dependency injection code with Wire
wire:
	@echo "Generating dependency injection code with Wire..."
	@if command -v wire >/dev/null 2>&1; then \
		wire ./...; \
	else \
		echo "Please install Wire: go install github.com/google/wire/cmd/wire@latest"; \
		exit 1; \
	fi

## help: Show this help message
help:
	@echo "Usage:"
	@sed -n 's/^##//p' $(MAKEFILE_LIST) | column -t -s ':' | sed -e 's/^/ /'