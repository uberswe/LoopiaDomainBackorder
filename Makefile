.PHONY: build clean test lint run-dropcatch run-available

# Build variables
BINARY_NAME=loopiaDomainGrabber
BUILD_DIR=build
CMD_DIR=cmd/loopiaDomainGrabber

# Go commands
GO=go
GOBUILD=$(GO) build
GOCLEAN=$(GO) clean
GOTEST=$(GO) test
GOGET=$(GO) get
GOLINT=golangci-lint

# Build the application
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME) ./$(CMD_DIR)

# Clean build artifacts
clean:
	@echo "Cleaning..."
	$(GOCLEAN)
	@rm -rf $(BUILD_DIR)

# Run tests
test:
	@echo "Running tests..."
	$(GOTEST) -v ./...

# Run linter
lint:
	@echo "Running linter..."
	$(GOLINT) run

# Install dependencies
deps:
	@echo "Installing dependencies..."
	$(GOGET) -v ./...

# Run dropcatch command
run-dropcatch:
	@echo "Running dropcatch command..."
	$(GO) run ./$(CMD_DIR) dropcatch $(ARGS)

# Run available command
run-available:
	@echo "Running available command..."
	$(GO) run ./$(CMD_DIR) available $(ARGS)

# Default target
all: clean build

# Help target
help:
	@echo "Available targets:"
	@echo "  build          - Build the application"
	@echo "  clean          - Clean build artifacts"
	@echo "  test           - Run tests"
	@echo "  lint           - Run linter"
	@echo "  deps           - Install dependencies"
	@echo "  run-dropcatch  - Run dropcatch command (use ARGS='...' for arguments)"
	@echo "  run-available  - Run available command (use ARGS='...' for arguments)"
	@echo "  all            - Clean and build"
	@echo "  help           - Show this help message"