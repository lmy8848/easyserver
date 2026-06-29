.PHONY: all build build-web build-server build-dev clean run dev test deps fmt build-linux build-linux-arm64 help

# Build all (production with embedded frontend)
all: build

# Build production binary (with embedded frontend)
build: build-server
	@echo "Build complete: easyserver (production mode)"

# Build frontend
build-web:
	@echo "Building frontend..."
	cd web && npm run build
	@echo "Copying frontend to embed location..."
	rm -rf internal/api/web/dist
	mkdir -p internal/api/web/dist
	cp -a web/dist/. internal/api/web/dist/

# Version injection
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null | tr -d "'" || echo "dev")
LDFLAGS := -X 'easyserver/internal/api.Version=$(VERSION)'
GO_BUILD := go build -buildvcs=false -ldflags "$(LDFLAGS)"

# Build backend (with embedded frontend)
build-server: build-web
	@echo "Building backend..."
	$(GO_BUILD) -o easyserver ./cmd/server

# Build development binary (without embedded frontend)
build-dev:
	@echo "Building backend (dev mode)..."
	$(GO_BUILD) -tags dev -o easyserver ./cmd/server

# Clean build artifacts
clean:
	rm -f easyserver easyserver-linux easyserver-linux-arm64
	rm -rf web/dist
	rm -rf internal/api/web/dist

# Run production
run: build
	./easyserver -config config.yaml

# Run in development mode
dev:
	@echo "Starting backend (with air) and frontend in parallel..."
	bash -c "go run github.com/air-verse/air@latest & PID1=\$$!; cd web && npm run dev & PID2=\$$!; trap \"kill -TERM \$$PID1 \$$PID2 2>/dev/null || true\" EXIT; wait -n"

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
build-linux: build-web
	@echo "Building for Linux amd64..."
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GO_BUILD) -o easyserver-linux ./cmd/server

build-linux-arm64: build-web
	@echo "Building for Linux arm64..."
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 $(GO_BUILD) -o easyserver-linux-arm64 ./cmd/server

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
