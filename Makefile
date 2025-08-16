# Claude Code Open Makefile

# Variables
BINARY_NAME=cco
VERSION=0.3.0
BUILD_DIR=build
MAIN_PACKAGE=.

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=gofmt

# Build flags
BUILD_FLAGS=-ldflags="-s -w -X 'github.com/Davincible/claude-code-open/cmd.Version=$(VERSION)'"

.PHONY: all build clean test coverage fmt lint help install uninstall build-all

all: fmt test build

## build: Build the binary
build:
	$(GOBUILD) $(BUILD_FLAGS) -o $(BINARY_NAME) $(MAIN_PACKAGE)

## build-all: Build binaries for all platforms
build-all: clean
	mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(BUILD_FLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 $(MAIN_PACKAGE)
	GOOS=linux GOARCH=arm64 $(GOBUILD) $(BUILD_FLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 $(MAIN_PACKAGE)
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(BUILD_FLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 $(MAIN_PACKAGE)
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(BUILD_FLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 $(MAIN_PACKAGE)
	GOOS=windows GOARCH=amd64 $(GOBUILD) $(BUILD_FLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe $(MAIN_PACKAGE)

## test: Run tests
test:
	$(GOTEST) -v ./...

## coverage: Run tests with coverage
coverage:
	$(GOTEST) -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

## fmt: Format code
fmt:
	$(GOFMT) -s -w .

## lint: Run linter (requires golangci-lint)
lint:
	@which golangci-lint > /dev/null || (echo "golangci-lint not installed" && exit 1)
	golangci-lint run

## clean: Clean build artifacts
clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html

## deps: Download dependencies
deps:
	$(GOMOD) download
	$(GOMOD) tidy

## install: Install binary to system
install: build
	sudo cp $(BINARY_NAME) /usr/local/bin/$(BINARY_NAME)
	@echo "$(BINARY_NAME) installed to /usr/local/bin"

## uninstall: Remove binary from system
uninstall:
	sudo rm -f /usr/local/bin/$(BINARY_NAME)
	@echo "$(BINARY_NAME) removed from /usr/local/bin"

## dev: Run in development mode with auto-reload
dev:
	@which air > /dev/null || (echo "Installing air..." && go install github.com/cosmtrek/air@latest)
	@echo "Starting development server with hot reload..."
	@echo "The server will start automatically and reload on code changes"
	air

## docker-build: Build Docker image
docker-build:
	docker build -t claude-code-open:$(VERSION) .
	docker tag claude-code-open:$(VERSION) claude-code-open:latest

## docker-run: Run Docker container
docker-run:
	docker run --rm -p 6970:6970 -v ~/.claude-code-open:/root/.claude-code-open claude-code-open:latest

## release: Create release build
release: clean fmt test build-all
	@echo "Release $(VERSION) built successfully"
	@echo "Binaries available in $(BUILD_DIR)/"

## help: Show this help message
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@awk '/^##/{c=substr($$0,3);next}c&&/^[[:alpha:]][[:alnum:]_-]+:/{print substr($$1,1,index($$1,":")-1)":"c}1{c=""}' $(MAKEFILE_LIST) | column -t -s ":"
