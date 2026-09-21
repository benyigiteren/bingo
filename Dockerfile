# --- Build Stage ---
FROM golang:alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates

# Set work directory
WORKDIR /app

# Copy dependency files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Compile optimized static Go binary (CGO-free for minimal RAM/CPU overhead)
RUN CGO_ENABLED=0 go build -ldflags="-w -s" -o bingo main.go

# --- Final Runtime Stage ---
FROM alpine:latest

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

# Copy binary and assets from build stage
COPY --from=builder /app/bingo .
COPY --from=builder /app/templates ./templates
COPY --from=builder /app/static ./static

# Ensure runtime directories exist for non-volume runs.
# NOTE: the container intentionally runs as root because named volumes are
# created root-owned; switching to an unprivileged USER would break writes to
# /app/data and /app/uploads on fresh deploys. Privilege containment is
# enforced via compose security_opt (no-new-privileges).
RUN mkdir -p /app/data /app/uploads

# Volumes for persistent data and uploads
VOLUME ["/app/data", "/app/uploads"]

# Expose server port
EXPOSE 8080

# Liveness probe (uses PORT when customized, defaults to 8080)
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://127.0.0.1:${PORT:-8080}/login || exit 1

# Run bingo
CMD ["./bingo"]
