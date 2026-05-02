.PHONY: run build test test-short test-cover lint clean tidy

# Go commands
GO := go
BINARY := portfoliolab

# Run the server
run:
	$(GO) run cmd/server/main.go

# Run with example config
run-config:
	$(GO) run cmd/server/main.go --config config/config.example.yaml

# Build binary
build:
	$(GO) build -o $(BINARY) cmd/server/main.go

# Run all tests
test:
	$(GO) test ./...

# Run only unit tests (fast, no DB)
test-short:
	$(GO) test -short ./...

# Run tests with coverage
test-cover:
	$(GO) test -cover ./...

# Run tests with coverage and output to file
test-cover-report:
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html

# Tidy dependencies
tidy:
	$(GO) mod tidy

# Format code
fmt:
	goimports -w .

# Lint (if golangci-lint is installed)
lint:
	@if command -v golangci-lint > /dev/null; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not installed, skipping"; \
	fi

# Check for unused imports and vet issues
vet:
	$(GO) vet ./...

# Clean build artifacts
clean:
	rm -f $(BINARY) coverage.out coverage.html
	rm -rf bin/

# Generate sqlc types
sqlc-generate:
	sqlc generate

# Generate mocks
mocks:
	mockery --all

# Run database migrations up
migrate-up:
	goose sqlite3 data/portfoliolab.db up

# Run database migrations down (rollback last)
migrate-down:
	goose sqlite3 data/portfoliolab.db down

# Default target
.DEFAULT_GOAL := run
