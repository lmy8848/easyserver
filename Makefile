.PHONY: all build build-web build-server build-dev clean run dev test

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

# Build backend (with embedded frontend)
build-server:
	@echo "Building backend..."
	go build -o easyserver ./cmd/server

# Build development binary (without embedded frontend)
build-dev:
	@echo "Building backend (dev mode)..."
	go build -tags dev -o easyserver ./cmd/server

# Clean build artifacts
clean:
	rm -f easyserver easyserver.exe
	rm -rf web/dist
	rm -rf internal/api/web/dist
	rm -rf data

# Run production
run: build
	./easyserver -config config.yaml

# Run in development mode
dev: build-dev
	@echo "Starting backend on :8080..."
	./easyserver -config config.yaml -dev &
	@echo "Starting frontend on :5173..."
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
	cd web && npm test

# Cross compile for Linux
build-linux:
	@echo "Building for Linux..."
	GOOS=linux GOARCH=amd64 CGO_ENABLED=1 go build -o easyserver-linux ./cmd/server

# Help
help:
	@echo "Available commands:"
	@echo "  make build       - Build production binary (with embedded frontend)"
	@echo "  make build-dev   - Build development binary (without frontend)"
	@echo "  make dev         - Run in development mode"
	@echo "  make run         - Build and run production"
	@echo "  make clean       - Clean build artifacts"
	@echo "  make deps        - Install dependencies"
	@echo "  make test        - Run tests"
	@echo "  make build-linux - Cross compile for Linux"
