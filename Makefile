.PHONY: all build build-web clean dev deps fmt build-linux build-linux-arm64 release check check-go check-web help

# Build all (production with embedded frontend)
all: build

# Version injection
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null | tr -d "'" || echo "dev")
LDFLAGS := -X 'easyserver/internal/api.Version=$(VERSION)'
GO_BUILD := go build -buildvcs=false -ldflags "$(LDFLAGS)"

# Build production binary (with embedded frontend)
build: build-web
	@echo "Building backend..."
	$(GO_BUILD) -o easyserver ./cmd/server
	@echo "Build complete: easyserver"

# Build frontend (used as dependency by build / build-linux*)
build-web:
	@echo "Building frontend..."
	cd web && npm run build
	@echo "Copying frontend to embed location..."
	rm -rf internal/api/web/dist
	mkdir -p internal/api/web/dist
	cp -a web/dist/. internal/api/web/dist/

# Clean build artifacts
clean:
	rm -f easyserver easyserver-linux easyserver-linux-arm64
	rm -rf web/dist
	rm -rf internal/api/web/dist

# Run in development mode (backend via air + frontend via vite, parallel)
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

# Quick local checks — run before committing.
# `check` runs both sides; use `check-go` / `check-web` if you only touched one.
check: check-go check-web

check-go:
	@echo "==> gofmt"
	@out=$$(gofmt -l cmd internal); if [ -n "$$out" ]; then echo "$$out"; exit 1; fi
	@echo "==> go vet"
	@go vet -tags dev ./cmd/... ./internal/...
	@echo "==> go test"
	@go test -tags dev ./cmd/... ./internal/...
	@echo "==> go build"
	@go build -tags dev -o /dev/null ./cmd/server
	@echo "✓ 后端检查通过"

check-web:
	@echo "==> tsc"
	@cd web && npx tsc --noEmit
	@echo "==> eslint"
	@cd web && npx eslint .
	@echo "==> vite build"
	@cd web && npm run build
	@echo "✓ 前端检查通过"

# Cross compile for Linux (CGO disabled; terminal PTY unavailable without C cross-compiler)
build-linux: build-web
	@echo "Building for Linux amd64..."
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GO_BUILD) -o easyserver-linux ./cmd/server

build-linux-arm64: build-web
	@echo "Building for Linux arm64..."
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 $(GO_BUILD) -o easyserver-linux-arm64 ./cmd/server

# Cut a release: create an annotated tag and push it. GitHub Actions then
# builds the binaries and publishes the GitHub Release (notes auto-generated
# via --generate-notes). VERSION optional; defaults to patch-bump of last tag.
# ponytail: naive patch bump only — doesn't read Conventional Commits for
# minor/major; pass VERSION=v1.3.0 explicitly for those.
release:
	@[ -z "$$(git status --porcelain)" ] || { echo "工作区不干净，先提交或暂存"; exit 1; }
	@if [ "$(origin VERSION)" = "command line" ]; then \
		VERSION="$(VERSION)"; \
	else \
		LAST=$$(git describe --tags --abbrev=0 2>/dev/null) || { echo "无历史 tag，请显式指定: make release VERSION=v0.1.0"; exit 1; }; \
		VERSION=$$(echo "$$LAST" | awk -F. '{ printf "v%d.%d.%d", $$1, $$2, $$3+1 }'); \
		echo "未指定 VERSION，patch-bump: $$LAST → $$VERSION"; \
	fi; \
	case "$$VERSION" in v*) ;; *) VERSION="v$$VERSION";; esac; \
	git rev-parse --verify "refs/tags/$$VERSION" >/dev/null 2>&1 && { echo "tag $$VERSION 已存在"; exit 1; } || true; \
	echo "即将发布 $$VERSION"; \
	git tag -a "$$VERSION" -m "Release $$VERSION" && \
	echo "" && \
	echo "✓ 已创建 tag $$VERSION" && \
	echo "  推送: git push origin $$VERSION"

# Help
help:
	@echo "Available commands:"
	@echo "  make build          - Build production binary (with embedded frontend)"
	@echo "  make dev            - Run in development mode (air + vite)"
	@echo "  make clean          - Clean build artifacts"
	@echo "  make deps           - Install dependencies"
	@echo "  make fmt            - Format Go + frontend code"
	@echo "  make check          - Quick local checks (both go + web)"
	@echo "  make check-go       - Backend checks: gofmt/vet/test/build"
	@echo "  make check-web      - Frontend checks: tsc/eslint/build"
	@echo "  make build-linux    - Cross compile for Linux amd64"
	@echo "  make build-linux-arm64 - Cross compile for Linux arm64"
	@echo "  make release        - Cut a release (patch-bump or VERSION=v1.2.3)"
