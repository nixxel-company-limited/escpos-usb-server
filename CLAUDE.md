# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is an ESC/POS USB printer server written in Go. It provides a TCP server that forwards data received over the network to a USB thermal printer. The project is inspired by and aims to be compatible with the Node.js `@node-escpos/driver` package architecture.

**Module:** `github.com/nixxel-company-limited/escpos-usb-server`

## Architecture

The codebase follows a clean adapter pattern with two main packages:

### 1. `adapter` Package
Provides hardware abstraction for printer communication.

- **`Adapter` interface**: Defines the contract for all printer adapters (Open, Write, Read, Close, IsOpen)
- **`USBAdapter`**: Implementation for USB thermal printers using `github.com/google/gousb`
- **Event system**: Supports event listeners for connect, disconnect, data, and close events
- **Auto-detection**: `NewUSBAdapterAuto()` finds the first available USB printer automatically
- **Device discovery**: `FindPrinters()` returns all connected USB printers

Key implementation details:
- Uses printer interface class code `0x07` to identify USB printers
- Handles kernel driver detachment on Linux via `SetAutoDetach(true)`
- Thread-safe with mutex protection on device operations
- Claims USB interface and manages endpoints (in/out) automatically

### 2. `server` Package
TCP server that bridges network connections to printer adapters.

- **Blocking mode**: `Start()` blocks the calling goroutine (like Node.js `tcp.Server.listen()`)
- **Async mode**: `StartAsync()` runs server in background goroutine
- **Multi-client**: Handles concurrent TCP connections, each writing to the same printer
- **Pipe pattern**: Streams data from each TCP connection directly to the adapter's Write method
- **Default port**: 9100 (standard RAW printing port)

The server automatically opens the adapter when started and closes it when stopped.

## Development Commands

### Build
```bash
go build -o escpos-server
```

### Run
```bash
# With default address (localhost:9100)
go run main.go

# With custom address via environment variable
SERVER_ADDRESS=0.0.0.0:9100 go run main.go
```

### Docker

**Build:**
```bash
docker build -t escpos-usb-server .
```

**Run:**
```bash
# Basic run (requires USB device access)
docker run --rm \
  --privileged \
  -v /dev/bus/usb:/dev/bus/usb \
  -p 9100:9100 \
  escpos-usb-server

# With custom address
docker run --rm \
  --privileged \
  -v /dev/bus/usb:/dev/bus/usb \
  -p 9200:9200 \
  -e SERVER_ADDRESS=0.0.0.0:9200 \
  escpos-usb-server

# For specific USB device (more secure than --privileged)
docker run --rm \
  --device=/dev/bus/usb/001/002 \
  -p 9100:9100 \
  escpos-usb-server
```

**Important Docker Notes:**
- Requires `--privileged` or `--device` flag for USB access
- Volume mount `/dev/bus/usb:/dev/bus/usb` needed for USB device visibility
- Multi-stage build uses Alpine Linux for minimal image size
- CGO is enabled (required for gousb/libusb-1.0)
- Both builder and runtime use Alpine to avoid glibc/musl compatibility issues

### Testing
```bash
# Run all tests
go test ./...

# Run tests in specific package
go test ./adapter
go test ./server

# Run specific test
go test ./adapter -run TestNewUSBAdapterAuto

# Run tests with verbose output
go test -v ./...

# Skip tests requiring USB hardware
# Tests automatically skip if no USB printer is detected using t.Skip()
```

### Dependencies
```bash
# Install/update dependencies
go mod tidy

# Add new dependency
go get github.com/package/name
```

## Testing Strategy

All tests use `testify` assertions (`require`, `assert`). **No mocking** is used - tests work with real hardware when available.

**USB adapter tests:**
- Gracefully skip when no USB printer is connected
- Test real device communication when hardware is available
- Pattern: Check for error, call `t.Skip()` if device not found

**Server tests:**
- Use `MockAdapter` for unit tests (implements `Adapter` interface)
- Include integration tests with real `USBAdapter` (skip if unavailable)
- Test both `Start()` (blocking) and `StartAsync()` (non-blocking) modes

## Important Patterns

### Creating a USB Adapter
```go
// Auto-detect first printer
device, err := adapter.NewUSBAdapterAuto()

// Or specify VID/PID
device, err := adapter.NewUSBAdapter(0x04b8, 0x0202)

// Always close when done
defer device.Close()
```

### Using the Server
```go
// Create and start (blocks)
server := server.New(device, "localhost:9100")
err := server.Start() // Blocks until Stop() is called

// Or start async
err := server.StartAsync() // Returns immediately
// ... do other work ...
server.Stop()
```

### Event Listeners
```go
adapter.On(adapter.EventConnect, func(e adapter.Event) {
    log.Println("Printer connected")
})

adapter.On(adapter.EventData, func(e adapter.Event) {
    log.Printf("Writing %d bytes\n", len(e.Data))
})
```

## Configuration

The server address is configured via the `SERVER_ADDRESS` environment variable:
- Default: `localhost:9100`
- Uses Viper for configuration management
- Can be set via environment variable or .env file

Example `.env` file:
```bash
SERVER_ADDRESS=0.0.0.0:9100
```

## Module Path

The module uses: `github.com/nixxel-company-limited/escpos-usb-server`

When adding new packages or doing imports, use this as the base path. All imports should use the full module path, not relative imports.

## Dependencies

Key external dependencies:
- `github.com/google/gousb` - USB device communication (requires CGO and libusb-1.0)
- `github.com/spf13/viper` - Configuration management
- `github.com/stretchr/testify` - Testing assertions