# Build stage
FROM golang:1.23-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build both binaries
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o server ./cmd/server
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o migrate ./cmd/migrate

# Runtime stage
FROM alpine:latest

# Install ca-certificates for HTTPS and wget for health checks
RUN apk --no-cache add ca-certificates wget

WORKDIR /root/

# Copy the binaries from builder
COPY --from=builder /app/server .
COPY --from=builder /app/migrate .

# Copy migrations
COPY --from=builder /app/migrations /migrations

# Expose port
EXPOSE 8080

# Health check (only applies when running server)
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Default command runs the server, but can be overridden to run migrations
CMD ["./server"]
