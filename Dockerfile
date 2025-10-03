# Multi-stage Dockerfile for ESC/POS USB Server
# This build requires CGO for gousb/libusb-1.0 support

# Stage 1: Builder
FROM golang:1.24-bookworm AS builder

# Install build dependencies
# - gcc: Required for CGO compilation
# - libusb-1.0-0-dev: Development libraries for USB support (libusb-1.0)
RUN apt-get update && apt-get install -y --no-install-recommends \
    gcc \
    libusb-1.0-0-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /build

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN go build -o escpos-server .

# Stage 2: Runtime
FROM debian:bookworm-slim

# Install runtime dependencies
# Only libusb runtime library is needed (not -dev)
RUN apt-get update && apt-get install -y --no-install-recommends \
    libusb-1.0-0 \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy binary from builder stage
COPY --from=builder /build/escpos-server .

# Expose the server port
EXPOSE 9100

# Run the server
CMD ["./escpos-server"]
