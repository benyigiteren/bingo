# --- Build Stage ---
FROM golang:1.26-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git

# Set work directory
WORKDIR /app

# Copy dependency files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Compile optimized static Go binary (CGO-free for minimal RAM/CPU overhead)
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o bingo main.go

# --- Final Runtime Stage ---
FROM alpine:latest

# Install CA certificates for secure connections
RUN apk add --no-cache ca-certificates

WORKDIR /app

# Copy binary from build stage
COPY --from=builder /app/bingo .

# Copy templates and static assets
COPY --from=builder /app/templates ./templates
COPY --from=builder /app/static ./static

# Expose server port
EXPOSE 8080

# Run bingo
CMD ["./bingo"]
