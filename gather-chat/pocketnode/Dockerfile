# Build stage
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git gcc musl-dev

# Copy go mod files
COPY go.mod go.sum* ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o pocketnode .

# Runtime stage
FROM alpine:3.19

WORKDIR /app

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Copy the binary
COPY --from=builder /app/pocketnode /app/pocketnode

# Create data directory
RUN mkdir -p /pb_data

# Expose PocketBase port
EXPOSE 8090

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8090/api/health || exit 1
# Note: /api/health is PocketBase's built-in endpoint

# Run PocketBase
ENTRYPOINT ["/app/pocketnode"]
CMD ["serve", "--http=0.0.0.0:8090", "--dir=/pb_data"]
