# Build stage
FROM golang:1.25-alpine AS builder

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

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o epub-search ./cmd/epub-search

# Runtime stage
FROM alpine:latest

# Create non-root user
RUN addgroup -g 1000 -S goepubgrep && \
    adduser -u 1000 -S goepubgrep -G goepubgrep

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/epub-search /app/epub-search

# Set ownership
RUN chown -R goepubgrep:goepubgrep /app

# Switch to non-root user
USER goepubgrep

# Set entrypoint
ENTRYPOINT ["/app/epub-search"]

# Default command will show help
CMD ["--help"]
