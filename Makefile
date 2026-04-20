# CerbAI Makefile

BINARY_NAME = cerbai
GO = go
GOFMT = gofmt
GOVET = go vet
GOTEST = go test

# Build flags
LDFLAGS = -ldflags "-X main.version=dev -X main.commit=$$(git rev-parse --short HEAD 2>/dev/null || echo unknown) -X main.buildDate=$$(date -u +%Y-%m-%dT%H:%M:%SZ)"

.PHONY: all build test lint clean fmt vet run docker docker-compose-up docker-compose-down perf-build perf-smoke perf-load perf-stress perf-soak perf-all help

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

PERF_COMPOSE = docker compose -f docker-compose.yml -f docker-compose.perf.yml --profile perf

## perf-build: Build mock server and pull k6 image
perf-build:
	$(PERF_COMPOSE) build mock-server
	$(PERF_COMPOSE) pull k6

## perf-up: Start mock-server, redis and cerbai for perf tests
perf-up:
	$(PERF_COMPOSE) up -d mock-server redis cerbai

## perf-down: Stop perf environment
perf-down:
	$(PERF_COMPOSE) down

## perf-smoke: Run k6 smoke test (2 VUs, 1 min)
perf-smoke: perf-up
	$(PERF_COMPOSE) run --rm k6 run /scripts/smoke.js

## perf-load: Run k6 load test (50 VUs, 2 min)
perf-load: perf-up
	$(PERF_COMPOSE) run --rm k6 run /scripts/load.js

## perf-stress: Run k6 stress test (ramp up to 200 VUs)
perf-stress: perf-up
	$(PERF_COMPOSE) run --rm k6 run /scripts/stress.js

## perf-soak: Run k6 soak test (20 VUs, 30 min)
perf-soak: perf-up
	$(PERF_COMPOSE) run --rm k6 run /scripts/soak.js

## perf-all: Run smoke → load → stress tests sequentially
perf-all: perf-up perf-smoke perf-load perf-stress

## help: Show this help message
help:
	@echo "CerbAI Makefile"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@sed -n 's/^## //p' $(MAKEFILE_LIST) | awk -F: '{printf "  %-20s %s\n", $$1, $$2}'
