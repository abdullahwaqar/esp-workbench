.PHONY: build run clean install help lint fmt test

BINARY_NAME=esp-workbench
BINARY_PATH=./bin/$(BINARY_NAME)
VERSION?=$(shell git describe --tags --always 2>/dev/null || echo "dev")
BUILD_TIME=$(shell date -u '+%Y-%m-%d %H:%M:%S')
GOFLAGS=-ldflags "-X main.Version=$(VERSION) -X 'main.BuildTime=$(BUILD_TIME)'"

## help: Display this help message
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@sed -n 's/^## //p' $(MAKEFILE_LIST) | column -t -s ':' | sed -e 's/^/ /'

## build: Build the binary
build:
	@echo "building $(BINARY_NAME)..."
	@mkdir -p bin
	@go build $(GOFLAGS) -o $(BINARY_PATH) ./cmd/app
	@echo "built: $(BINARY_PATH)"

## run: Build and run the application
run: build
	@$(BINARY_PATH)

## install: Install the binary to GOPATH/bin
install: build
	@echo "installing $(BINARY_NAME)..."
	@go install $(GOFLAGS) ./cmd/app
	@echo "installed to $(GOPATH)/bin/$(BINARY_NAME)"

## clean: Remove build artifacts
clean:
	@echo "cleaning build artifacts..."
	@rm -rf bin/
	@go clean
	@echo "done"

## fmt: Format code with go fmt
fmt:
	@echo "formatting code..."
	@go fmt ./...
	@echo "done"

## lint: Run go vet
lint:
	@echo "running linter..."
	@go vet ./...
	@echo "done"

## test: Run tests
test:
	@echo "running tests..."
	@go test -v ./...
	@echo "done"

## deps: Download and verify dependencies
deps:
	@echo "downloading dependencies..."
	@go mod download
	@go mod verify
	@echo "done"

## tidy: Tidy dependencies
tidy:
	@echo "tidying dependencies..."
	@go mod tidy
	@echo "done"

## all: Format, lint, test, and build
all: fmt lint test build
	@echo "all checks passed"

## dev: Development mode - build with race detector
dev:
	@echo "building with race detector..."
	@mkdir -p bin
	@go build -race $(GOFLAGS) -o $(BINARY_PATH) ./cmd/app
	@echo "built: $(BINARY_PATH) (race detector enabled)"

## check: Run format, lint, and test without building
check: fmt lint test
	@echo "all checks passed"
