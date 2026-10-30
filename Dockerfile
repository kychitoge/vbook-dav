# ==============================================================================
# Build Stage
# ==============================================================================
FROM golang:1.24-alpine AS builder

WORKDIR /src

# Install build dependencies and security certificates
RUN apk add --no-cache ca-certificates tzdata

# Cache Go modules layer
COPY go.mod go.sum ./
RUN go mod download

# Copy application source code
COPY . .

# Compile static binary with optimizations (-s -w strip debug info)
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -extldflags '-static'" \
    -trimpath \
    -o /bin/vbook-dav ./cmd/dav

# ==============================================================================
# Runtime Stage
# ==============================================================================
FROM alpine:3.21

# Import system certificates and timezone data from builder
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=builder /bin/vbook-dav /app/vbook-dav

# Create dedicated non-root user (UID 1000 standard for Linux host volume permissions)
# Keep /app owned by root:root with read/execute-only permissions (0555) to prevent runtime tampering
# Set /data ownership to appuser:appgroup for runtime storage access
RUN addgroup -g 1000 -S appgroup && \
    adduser -u 1000 -S appuser -G appgroup && \
    mkdir -p /data && \
    chown -R appuser:appgroup /data && \
    chmod 0755 /data && \
    chmod 0555 /app && \
    chmod 0555 /app/vbook-dav

# Run container as non-root
USER appuser:appgroup

# Environment defaults
ENV PORT=:8080 \
    DATA_DIR=/data

EXPOSE 8080

VOLUME ["/data"]

# Health check using lightweight native GET /healthz
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -q -O /dev/null http://127.0.0.1:8080/healthz || exit 1

ENTRYPOINT ["/app/vbook-dav"]
