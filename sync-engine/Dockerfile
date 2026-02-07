# Build stage
FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s" \
    -o /app/bin/sync-engine \
    ./cmd/server

# Runtime stage
FROM alpine:3.19

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1000 syncflow && \
    adduser -D -u 1000 -G syncflow syncflow

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/bin/sync-engine /app/sync-engine

# Create directories for checkpoints and logs
RUN mkdir -p /app/checkpoints /app/logs && \
    chown -R syncflow:syncflow /app

# Switch to non-root user
USER syncflow

# Expose ports
# 8082: HTTP API
# 8083: WebSocket (if separate)
EXPOSE 8082 8083

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8082/health || exit 1

# Set entrypoint
ENTRYPOINT ["/app/sync-engine"]
