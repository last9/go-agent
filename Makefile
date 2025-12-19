.PHONY: help test test-unit test-integration test-integration-ci lint docker-up docker-down docker-ps build clean coverage

# Default target
help:
	@echo "Available targets:"
	@echo "  make test                - Run all tests (unit + integration)"
	@echo "  make test-unit           - Run unit tests only"
	@echo "  make test-integration    - Run integration tests (requires Docker services)"
	@echo "  make test-integration-ci - Run integration tests with race detector"
	@echo "  make lint                - Run golangci-lint"
	@echo "  make docker-up           - Start Docker test services"
	@echo "  make docker-down         - Stop Docker test services"
	@echo "  make docker-ps           - Show Docker test services status"
	@echo "  make build               - Build all packages"
	@echo "  make coverage            - Generate coverage report"
	@echo "  make clean               - Clean test artifacts"

# Run all tests
test: test-unit test-integration

# Run unit tests only (fast)
test-unit:
	@echo "Running unit tests..."
	go test -v -race -short ./...

# Run integration tests (requires Docker services)
test-integration:
	@echo "Running integration tests..."
	@echo "Checking if Docker services are running..."
	@docker-compose -f docker-compose.test.yml ps kafka | grep -q "Up" || (echo "Error: Docker services not running. Run 'make docker-up' first." && exit 1)
	go test -v -tags=integration -timeout=15m ./tests/integration/...

# Run integration tests with race detector (for CI)
test-integration-ci:
	@echo "Running integration tests with race detector..."
	go test -v -race -tags=integration -timeout=10m ./tests/integration/...

# Run linter
lint:
	@echo "Running golangci-lint..."
	@which golangci-lint > /dev/null || (echo "Error: golangci-lint not installed. Install: https://golangci-lint.run/usage/install/" && exit 1)
	golangci-lint run --timeout=5m

# Start Docker test services
docker-up:
	@echo "Starting Docker test services..."
	docker-compose -f docker-compose.test.yml up -d
	@echo "Waiting for services to be healthy..."
	@echo "Waiting for Kafka..."
	@timeout 60 bash -c 'until docker-compose -f docker-compose.test.yml exec -T kafka kafka-topics --bootstrap-server localhost:9092 --list > /dev/null 2>&1; do sleep 2; echo -n "."; done' || (echo "Kafka failed to start" && exit 1)
	@echo ""
	@echo "Waiting for PostgreSQL..."
	@timeout 30 bash -c 'until docker-compose -f docker-compose.test.yml exec -T postgres pg_isready -U test_user -d test_db > /dev/null 2>&1; do sleep 1; echo -n "."; done' || (echo "PostgreSQL failed to start" && exit 1)
	@echo ""
	@echo "Waiting for MySQL..."
	@timeout 30 bash -c 'until docker-compose -f docker-compose.test.yml exec -T mysql mysqladmin ping -h localhost -u test_user -ptest_password > /dev/null 2>&1; do sleep 1; echo -n "."; done' || (echo "MySQL failed to start" && exit 1)
	@echo ""
	@echo "Waiting for Redis..."
	@timeout 30 bash -c 'until docker-compose -f docker-compose.test.yml exec -T redis redis-cli ping > /dev/null 2>&1; do sleep 1; echo -n "."; done' || (echo "Redis failed to start" && exit 1)
	@echo ""
	@echo "✓ All services are healthy and ready!"

# Stop Docker test services
docker-down:
	@echo "Stopping Docker test services..."
	docker-compose -f docker-compose.test.yml down -v

# Show Docker service status
docker-ps:
	docker-compose -f docker-compose.test.yml ps

# Build all packages
build:
	@echo "Building all packages..."
	go build ./...
	@echo "Building examples..."
	cd examples/gin && go build -o ../../bin/example-gin main.go
	cd examples/gin-full && go build -o ../../bin/example-gin-full main.go
	@echo "✓ Build successful"

# Generate coverage report
coverage:
	@echo "Generating coverage report..."
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Clean test artifacts
clean:
	@echo "Cleaning test artifacts..."
	rm -f coverage.out coverage.html
	rm -rf bin/
	go clean -testcache
	@echo "✓ Clean complete"
