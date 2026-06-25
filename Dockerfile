# Stage 1: Build frontend
FROM node:18.20-alpine AS frontend
WORKDIR /web
COPY web/package*.json ./
RUN npm ci
COPY web/ .
RUN npm run build

# Stage 2: Build backend
FROM golang:1.25.0-alpine AS backend
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /web/dist ./web/dist
# Copy dist to embed location
RUN mkdir -p internal/api/web/dist && cp -r web/dist/* internal/api/web/dist/
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags "-X easyserver/internal/api.Version=${VERSION}" -o easyserver ./cmd/server

# Stage 3: Final image
FROM debian:bookworm-20260601-slim

# Install ca-certificates for HTTPS and wget for healthcheck
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates wget && rm -rf /var/lib/apt/lists/*

# Create non-root user
RUN groupadd -r easyserver && useradd -r -g easyserver -d /app -s /sbin/nologin easyserver

WORKDIR /app

# Copy binary
COPY --from=backend /app/easyserver .

# Create data directory and set ownership
RUN mkdir -p /app/data && chown -R easyserver:easyserver /app

# Generate a default config.yaml with random secrets at runtime if not provided
# This ensures the container can start even without a mounted config file
RUN printf '%s\n' \
  'server:' \
  '  port: 8080' \
  '  host: 0.0.0.0' \
  '  serve_frontend: true' \
  '  dev_mode: false' \
  'auth:' \
  '  jwt_secret: "change-me-to-a-random-32byte-secret!!!"' \
  '  session_timeout: 24h' \
  '  max_login_attempts: 5' \
  '  lockout_duration: 15m' \
  '  rate_limit: 100' \
  '  rate_interval: 1m' \
  '  session_cleanup_interval: 5m' \
  'monitor:' \
  '  history_retention: 24h' \
  '  collect_interval: 1s' \
  'database:' \
  '  path: "/app/data/easyserver.db"' \
  'audit:' \
  '  enabled: true' \
  '  retention_days: 90' \
  'deploy:' \
  '  encryption_key: "change-me-to-a-random-32-byte-key!!"' \
  'filemanager:' \
  '  base_path: "/app/data"' \
  'notify:' \
  '  enabled: false' \
  > /app/config.yaml && chown easyserver:easyserver /app/config.yaml

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -qO /dev/null http://localhost:8080/health || exit 1

# Run as non-root user
USER easyserver

ENTRYPOINT ["./easyserver"]
CMD ["-config", "/app/config.yaml"]
