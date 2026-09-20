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

# Volumes for persistent data and uploads
VOLUME ["/app/data", "/app/uploads"]

# Expose server port
EXPOSE 8080

# Run bingo
CMD ["./bingo"]
