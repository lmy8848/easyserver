# Stage 1: Build frontend
FROM node:18-alpine AS frontend
WORKDIR /web
COPY web/package*.json ./
RUN npm ci
COPY web/ .
RUN npm run build

# Stage 2: Build backend
FROM golang:1.23-alpine AS backend
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /web/dist ./web/dist
# Copy dist to embed location
RUN mkdir -p internal/api/web/dist && cp -r web/dist/* internal/api/web/dist/
RUN CGO_ENABLED=1 GOOS=linux go build -o easyserver ./cmd/server

# Stage 3: Final image
FROM debian:bookworm-slim

# Install ca-certificates for HTTPS
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy binary
COPY --from=backend /app/easyserver .

# Create data directory
RUN mkdir -p /app/data

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD curl -f http://localhost:8080/health || exit 1

# Run
ENTRYPOINT ["./easyserver"]
CMD ["-config", "/app/config.yaml"]
