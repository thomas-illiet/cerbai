# CerbAI Makefile

BINARY_NAME = cerbai
GO = go
GOFMT = gofmt
GOVET = go vet
GOTEST = go test

# Build flags
LDFLAGS = -ldflags "-X main.version=dev -X main.commit=$$(git rev-parse --short HEAD 2>/dev/null || echo unknown) -X main.buildDate=$$(date -u +%Y-%m-%dT%H:%M:%SZ)"

.PHONY: all build test lint clean fmt vet run docker docker-compose-up docker-compose-down help

all: fmt vet build test

## build: Compile the binary
build:
	$(GO) build $(LDFLAGS) -o $(BINARY_NAME) .

## test: Run tests with verbose output
test:
	$(GOTEST) -v ./...

## test-coverage: Run tests with coverage report
test-coverage:
	$(GOTEST) -v -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

## lint: Run go vet
lint: vet

## vet: Run go vet
vet:
	$(GOVET) ./...

## fmt: Format all Go files
fmt:
	$(GOFMT) -s -w .

## check-fmt: Check if files are formatted (CI-friendly)
check-fmt:
	@unformatted=$$($(GOFMT) -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "The following files are not formatted:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi

## run: Build and run with minimal config (requires env vars)
run: build
	@echo "Set required env vars and run ./$(BINARY_NAME)"

## clean: Remove build artifacts
clean:
	rm -f $(BINARY_NAME) coverage.out coverage.html

## docker-build: Build Docker image
docker-build:
	docker build -t $(BINARY_NAME):dev .

## docker-compose-up: Start with Docker Compose
docker-compose-up:
	docker compose up -d

## docker-compose-down: Stop Docker Compose
docker-compose-down:
	docker compose down

## docker-compose-logs: Follow Docker Compose logs
docker-compose-logs:
	docker compose logs -f

## help: Show this help message
help:
	@echo "CerbAI Makefile"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@sed -n 's/^## //p' $(MAKEFILE_LIST) | awk -F: '{printf "  %-20s %s\n", $$1, $$2}'
