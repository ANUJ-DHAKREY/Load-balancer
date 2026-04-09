.PHONY: build run clean test fmt lint docker-up docker-down backends

BINARY_NAME=loadbalancer
BACKEND_BINARY=example-backend

## build: Build the load balancer binary
build:
	go build -o bin/$(BINARY_NAME) ./cmd/loadbalancer
	go build -o bin/$(BACKEND_BINARY) ./cmd/example-backend

## run: Build and run the load balancer
run: build
	./bin/$(BINARY_NAME) -config config.yaml

## backends: Start 3 example backend servers
backends: build
	./bin/$(BACKEND_BINARY) -port 8081 -name backend-1 &
	./bin/$(BACKEND_BINARY) -port 8082 -name backend-2 &
	./bin/$(BACKEND_BINARY) -port 8083 -name backend-3 &
	@echo "Started 3 backend servers on ports 8081, 8082, 8083"

## test: Run tests
test:
	go test -v -race -count=1 ./...

## test-cover: Run tests with coverage
test-cover:
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

## fmt: Format code
fmt:
	go fmt ./...

## lint: Run go vet
lint:
	go vet ./...

## clean: Remove build artifacts
clean:
	rm -rf bin/ coverage.out coverage.html
	-pkill -f "$(BACKEND_BINARY)" 2>/dev/null || true

## docker-up: Start with Docker Compose
docker-up:
	docker compose up --build -d

## docker-down: Stop Docker Compose
docker-down:
	docker compose down

## help: Show this help
help:
	@echo "Available targets:"
	@grep -E '^## ' Makefile | sed 's/^## /  /'
