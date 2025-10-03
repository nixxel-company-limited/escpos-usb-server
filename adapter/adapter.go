package adapter

// Adapter defines the interface for printer communication adapters
type Adapter interface {
	// Open opens the connection to the printer
	Open() error

	// Write sends data to the printer
	Write(data []byte) (int, error)

	// Read reads data from the printer
	Read(buf []byte) (int, error)

	// Close closes the connection to the printer
	Close() error

	// IsOpen returns whether the connection is open
	IsOpen() bool
}
