.PHONY: all build build-web build-server build-dev clean run dev test deps fmt build-linux help

# Build all (production with embedded frontend)
all: build

# Build production binary (with embedded frontend)
build: build-web build-server
	@echo "Build complete: easyserver (production mode)"

# Build frontend
build-web:
	@echo "Building frontend..."
	cd web && npm run build
	@echo "Copying frontend to embed location..."
	mkdir -p internal/api/web/dist
	cp -r web/dist/* internal/api/web/dist/

# Version injection
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -X easyserver/internal/api.Version=$(VERSION)

# Build backend (with embedded frontend)
build-server:
	@echo "Building backend..."
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o easyserver ./cmd/server

# Build development binary (without embedded frontend)
build-dev:
	@echo "Building backend (dev mode)..."
	CGO_ENABLED=0 go build -tags dev -ldflags "$(LDFLAGS)" -o easyserver ./cmd/server

# Clean build artifacts
clean:
	rm -f easyserver
	rm -rf web/dist
	rm -rf internal/api/web/dist

# Run production
run: build
	./easyserver -config config.yaml

# Run in development mode
dev: build-dev
	@echo "Starting backend on :8080..."
	@echo "Starting frontend on :5173..."
	./easyserver -config config.yaml -dev &
	cd web && npm run dev

# Install dependencies
deps:
	go mod download
	cd web && npm install

# Format code
fmt:
	go fmt ./...
	cd web && npm run lint -- --fix

# Run tests
test:
	go test ./...

# Cross compile for Linux (CGO disabled; terminal PTY unavailable without C cross-compiler)
build-linux:
	@echo "Building for Linux amd64..."
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o easyserver-linux ./cmd/server

build-linux-arm64:
	@echo "Building for Linux arm64..."
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o easyserver-linux-arm64 ./cmd/server

# Help
help:
	@echo "Available commands:"
	@echo "  make build          - Build production binary (with embedded frontend)"
	@echo "  make build-dev      - Build development binary (without frontend)"
	@echo "  make dev            - Run in development mode"
	@echo "  make run            - Build and run production"
	@echo "  make clean          - Clean build artifacts"
	@echo "  make deps           - Install dependencies"
	@echo "  make test           - Run tests"
	@echo "  make build-linux    - Cross compile for Linux amd64"
	@echo "  make build-linux-arm64 - Cross compile for Linux arm64"
